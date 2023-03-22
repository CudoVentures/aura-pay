package requesters

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func (r *Requester) GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error) {

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// cudos1tr9jp0eqza9tvdvqzgyff9n3kdfew8uzhcyuwq/BTC/1@test
	//requestString := fmt.Sprintf("/CudoVentures/cudos-node/addressbook/address/%s/%s/%s@%s", cudosAddress, network, tokenId, denomId) // TODO: Use this once this is fixed in the aura platform

	requestString := fmt.Sprintf("/CudoVentures/cudos-node/addressbook/address/%s/aurapool/aurapool", cudosAddress)

	req, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRestUrl+requestString, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	if res.StatusCode == StatusCodeNotFound {
		return "", fmt.Errorf("address not found in the node addressbook: %s", cudosAddress)
	}

	if res.StatusCode != StatusCodeOK {
		return "", fmt.Errorf("error! Request Failed: %s with StatusCode: %d, Error: %s", res.Status, res.StatusCode, string(bytes))
	}

	okStruct := types.MappedAddress{}

	if err := json.Unmarshal(bytes, &okStruct); err != nil {
		return "", err
	}

	return okStruct.Address.Value, nil

}

// very rough estimation of a block height before the period start
// and a block height after the period end
// we need just any heights to filter the events by it
// since node querying is very slow
func (r *Requester) getPeriodBlockBorders(ctx context.Context, periodStart, periodEnd int64) (int64, int64, error) {
	// get the current block
	currentBlock, err := r.getLatestBlock(ctx)
	if err != nil {
		return 0, 0, err
	}
	var currentBlockHeight int64
	currentBlockHeight, err = strconv.ParseInt(currentBlock.Header.Height, 10, 64)
	if err != nil {
		return 0, 0, err
	}

	// estimate block height before period start
	// based on blok time of 5s
	// if the actual block time is higher (can't be lower because of settings)
	// it will estimate block a bit further than period start, but that is okay
	periodStartHeight := currentBlockHeight
	lastCheckedStartTime := time.Now().Unix()
	// get the block before the period start
	// repeat until fount height before the period start
	for lastCheckedStartTime > periodStart {
		estimatedHeight := periodStartHeight - (lastCheckedStartTime-periodStart)/5
		if estimatedHeight <= 0 {
			periodStartHeight = 0
			break
		}

		block, err := r.getBlockAtHeight(ctx, estimatedHeight)
		if err != nil {
			fmt.Println(err)
			return 0, 0, err
		}
		lastCheckedStartTime, err = convertTimeToTimestamp(block.Header.Time)
		if err != nil {
			return 0, 0, err
		}

		periodStartHeight = estimatedHeight
	}

	// do the same for period end
	periodEndHeight := periodStartHeight
	lastCheckedEndTime := lastCheckedStartTime
	// get the block before the period start
	// repeat until fount height before the period start
	for lastCheckedEndTime < periodEnd {
		estimatedEndHeight := periodEndHeight + (periodEnd-lastCheckedEndTime)/5
		if estimatedEndHeight >= currentBlockHeight {
			periodEndHeight = currentBlockHeight
			break
		}

		block, err := r.getBlockAtHeight(ctx, estimatedEndHeight)
		if err != nil {
			return 0, 0, err
		}

		lastCheckedEndTime, err = convertTimeToTimestamp(block.Header.Time)
		if err != nil {
			return 0, 0, err
		}

		periodEndHeight = estimatedEndHeight
	}

	return periodStartHeight, periodEndHeight, nil
}

func (r *Requester) getLatestBlock(ctx context.Context) (types.Block, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRestUrl+"/blocks/latest", nil)
	if err != nil {
		return types.Block{}, err
	}

	bytes, err := r.makeRequest(ctx, request)
	if err != nil {
		return types.Block{}, err
	}

	blockResponse := types.GetBlockResponse{}
	if err := json.Unmarshal(bytes, &blockResponse); err != nil {
		return types.Block{}, err
	}

	return blockResponse.Block, nil
}

func (r *Requester) getBlockAtHeight(ctx context.Context, height int64) (types.Block, error) {
	fmt.Printf("%s/blocks/%d", r.config.NodeRestUrl, height)
	request, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/blocks/%d", r.config.NodeRestUrl, height), nil)
	if err != nil {
		return types.Block{}, err
	}

	bytes, err := r.makeRequest(ctx, request)
	if err != nil {
		return types.Block{}, err
	}

	blockResponse := types.GetBlockResponse{}
	if err := json.Unmarshal(bytes, &blockResponse); err != nil {
		return types.Block{}, err
	}

	return blockResponse.Block, nil
}

func (r *Requester) getTxsByEvents(ctx context.Context, query string) ([]types.TxResponse, error) {
	var txResponses []types.TxResponse
	paginationLimit := 100
	page := 0
	shouldFetch := true

	for shouldFetch {
		//fetch batch
		requestString := fmt.Sprintf("/cosmos/tx/v1beta1/txs?events=%s&pagination.limit=%d&order_by=ORDER_BY_DESC&pagination.offset=%d", query, paginationLimit, page*paginationLimit)
		fmt.Println(requestString)
		request, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRestUrl+requestString, nil)
		if err != nil {
			return []types.TxResponse{}, err
		}

		bytes, err := r.makeRequest(ctx, request)
		if err != nil {
			return []types.TxResponse{}, err
		}
		var res types.TxQueryResponse
		if err := json.Unmarshal(bytes, &res); err != nil {
			log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", bytes, err)
			return []types.TxResponse{}, err
		}

		//add fetched to total
		txResponses = append(txResponses, res.TxResponses...)

		// all pages fetched
		total, err := strconv.Atoi(res.Pagination.Total)
		if err != nil {
			return []types.TxResponse{}, err
		}

		fmt.Println(total)
		fmt.Println(len(txResponses))
		if len(txResponses) == total {
			shouldFetch = false
		}

		page++
	}

	return txResponses, nil
}

func (r *Requester) GetDenomNftTransferHistory(ctx context.Context, collectionDenomId string, periodStart, periodEnd int64) ([]types.NftTransferEvent, error) {
	// estimate block heights
	// periodStartHeight, periodEndHeight, err := r.getPeriodBlockBorders(ctx, periodStart, periodEnd)
	// if err != nil {
	// 	return []types.NftTransferEvent{}, err
	// }

	// txResponses, err := r.getTxsByEvents(ctx, "buy_nft.denom_id%3D%27"+collectionDenomId+"%27", periodStart, periodEnd)
	var allTxResponses []types.TxResponse
	txResponses, err := r.getTxsByEvents(ctx, "message.module=%27"+"nft"+"%27%26tx.height%3E1000")
	if err != nil {
		return []types.NftTransferEvent{}, err
	}
	allTxResponses = append(allTxResponses, txResponses...)

	// txResponses, err = r.getTxsByEvents(ctx, "transfer_nft.denom_id%3D%27"+collectionDenomId+"%27", periodStart, periodEnd)
	// if err != nil {
	// 	return []types.NftTransferEvent{}, err
	// }
	// allTxResponses = append(allTxResponses, txResponses...)

	var transferEvents []types.NftTransferEvent

	for _, txResponse := range allTxResponses {
		txTimestamp, err := convertTimeToTimestamp(txResponse.Timestamp)
		if err != nil {
			return []types.NftTransferEvent{}, err
		}
		for _, event := range txResponse.Events {
			if event.Type == "buy_nft" {
				var transferEvent types.NftTransferEvent
				transferEvent.Timestamp = txTimestamp
				transferEvent.DenomId = collectionDenomId
				for _, attr := range event.Attributes {
					if string(attr.Key) == "owner" {
						transferEvent.From = string(attr.Value)
					}
					if string(attr.Key) == "buyer" {
						transferEvent.To = string(attr.Value)
					}
					if string(attr.Key) == "token_id" {
						transferEvent.TokenId = string(attr.Value)
					}
				}
				transferEvents = append(transferEvents, transferEvent)
				continue
			}

			if event.Type == "transfer_nft" {
				var transferEvent types.NftTransferEvent
				transferEvent.Timestamp = txTimestamp
				transferEvent.DenomId = collectionDenomId

				for _, attr := range event.Attributes {
					if string(attr.Key) == "from" {
						transferEvent.From = string(attr.Value)
					}
					if string(attr.Key) == "to" {
						transferEvent.To = string(attr.Value)
					}
					if string(attr.Key) == "token_id" {
						transferEvent.TokenId = string(attr.Value)
					}
				}

				transferEvents = append(transferEvents, transferEvent)
				continue
			}
		}
	}

	var filteredTransferEvents []types.NftTransferEvent
	//filter by period start and finish
	for _, transferEvent := range transferEvents {
		if transferEvent.Timestamp >= periodStart && transferEvent.Timestamp <= periodEnd {
			filteredTransferEvents = append(filteredTransferEvents, transferEvent)
		}
	}

	sort.Slice(filteredTransferEvents, func(i, j int) bool {
		return filteredTransferEvents[i].Timestamp < filteredTransferEvents[j].Timestamp
	})

	return filteredTransferEvents, nil
}

func (r *Requester) makeRequest(ctx context.Context, request *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: time.Second * 10}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error! Request Failed: %s with StatusCode: %d", response.Status, response.StatusCode)
	}

	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func convertTimeToTimestamp(timeString string) (int64, error) {
	timeLayout := "2006-01-02T15:04:05Z"

	t, err := time.Parse(timeLayout, timeString)
	if err != nil {
		return 0, err
	}

	return t.Unix(), nil
}
