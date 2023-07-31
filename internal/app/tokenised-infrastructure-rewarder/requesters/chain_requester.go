package requesters

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func (r *Requester) GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network string) (string, error) {

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	requestString := fmt.Sprintf("/CudoVentures/cudos-node/addressbook/address/%s/cudosmarkets/cudosmarkets", cudosAddress)

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
		return "", nil
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
	log.Debug().Msgf("Getting period start block height...")
	for lastCheckedStartTime > periodStart {
		estimatedHeight := periodStartHeight - (lastCheckedStartTime-periodStart)/5 - 1
		if estimatedHeight <= 0 {
			periodStartHeight = 0
			break
		}
		log.Debug().Msgf("Estimated height: %d", estimatedHeight)
		block, err := r.getBlockAtHeight(ctx, estimatedHeight)
		if err != nil {
			return 0, 0, err
		}

		log.Debug().Msgf("Block time: %s", block.Header.Time)
		lastCheckedStartTime, err = convertTimeToTimestamp(block.Header.Time)
		if err != nil {
			return 0, 0, err
		}

		periodStartHeight = estimatedHeight
	}
	log.Debug().Msgf("Found period start height: %d", periodStartHeight)

	// do the same for period end
	periodEndHeight := periodStartHeight
	lastCheckedEndTime := lastCheckedStartTime
	// get the block before the period start
	// repeat until fount height before the period start
	log.Debug().Msgf("Getting period end block height...")
	for lastCheckedEndTime < periodEnd {
		estimatedEndHeight := periodEndHeight + (periodEnd-lastCheckedEndTime)/5 + 1
		if estimatedEndHeight >= currentBlockHeight {
			periodEndHeight = currentBlockHeight
			break
		}
		log.Debug().Msgf("Estimated height: %d", estimatedEndHeight)

		block, err := r.getBlockAtHeight(ctx, estimatedEndHeight)
		if err != nil {
			return 0, 0, err
		}

		log.Debug().Msgf("Block time: %s", block.Header.Time)
		lastCheckedEndTime, err = convertTimeToTimestamp(block.Header.Time)
		if err != nil {
			return 0, 0, err
		}

		periodEndHeight = estimatedEndHeight
	}
	log.Debug().Msgf("Found period end height: %d", periodEndHeight)

	return periodStartHeight, periodEndHeight, nil
}

func (r *Requester) getLatestBlock(ctx context.Context) (types.Block, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRestUrl+"/cosmos/base/tendermint/v1beta1/blocks/latest", nil)
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
	request, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/cosmos/base/tendermint/v1beta1/blocks/%d", r.config.NodeRestUrl, height), nil)
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

func (r *Requester) GetChainNftMintTimestamp(ctx context.Context, denomId, tokenId string) (int64, error) {
	marketplaceModuletxs, err := r.getTxsByEvents(ctx, "marketplace_mint_nft.denom_id=%27"+tokenId+"%27%20AND%20marketplace_mint_nft.denom_id=%27"+denomId+"%27")
	if err != nil {
		return 0, err
	}

	if len(marketplaceModuletxs) != 1 {
		return 0, fmt.Errorf("error! Expected 1 mint tx for token %s, got %d", tokenId, len(marketplaceModuletxs))
	}

	parsedHeight, err := strconv.ParseInt(marketplaceModuletxs[0].Height, 10, 64)
	if err != nil {
		return 0, err
	}

	block, err := r.getBlockAtHeight(ctx, parsedHeight)
	if err != nil {
		return 0, err
	}

	timestamp, err := convertTimeToTimestamp(block.Header.Time)
	if err != nil {
		return 0, err
	}

	return timestamp, nil
}

func (r *Requester) getTxsByEvents(ctx context.Context, query string) ([]types.Tx, error) {
	var txs []types.Tx
	paginationLimit := 100
	page := 1
	shouldFetch := true

	for shouldFetch {
		//fetch batch
		requestString := "/tx_search?query=%22" + query + "%22" + "&per_page=" + fmt.Sprint(paginationLimit) + "&page=" + fmt.Sprint(page)
		request, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRPCUrl+requestString, nil)
		if err != nil {
			return []types.Tx{}, err
		}

		bytes, err := r.makeRequest(ctx, request)
		if err != nil {
			return []types.Tx{}, err
		}
		var res types.TxQueryResponse
		if err := json.Unmarshal(bytes, &res); err != nil {
			log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", bytes, err)
			return []types.Tx{}, err
		}

		//add fetched to total
		for _, tx := range res.Result.Txs {
			txs = append(txs, tx)
		}

		// all pages fetched
		total, err := strconv.Atoi(res.Result.Total)
		if err != nil {
			return []types.Tx{}, err
		}

		if len(txs) == total {
			shouldFetch = false
		}

		page++
	}

	return txs, nil
}

func (r *Requester) GetDenomNftTransferHistory(ctx context.Context, collectionDenomId string, periodStart, periodEnd int64) ([]types.NftTransferEvent, error) {
	// estimate block heights
	log.Debug().Msgf("Getting block borders for period %d - %d", periodStart, periodEnd)
	periodStartHeight, periodEndHeight, err := r.getPeriodBlockBorders(ctx, periodStart, periodEnd)
	if err != nil {
		return []types.NftTransferEvent{}, err
	}
	log.Debug().Msgf("Got block borders for period %d - %d: %d - %d", periodStart, periodEnd, periodStartHeight, periodEndHeight)
	log.Debug().Msgf("Getting buy nft txs for period %d - %d", periodStart, periodEnd)
	var allTxs []types.Tx
	marketplaceModuletxs, err := r.getTxsByEvents(ctx, "buy_nft.denom_id=%27"+collectionDenomId+"%27%20AND%20tx.height%3E"+fmt.Sprint(periodStartHeight)+"%20AND%20tx.height%3C"+fmt.Sprint(periodEndHeight))
	if err != nil {
		return []types.NftTransferEvent{}, err
	}
	log.Debug().Msgf("Done!")
	log.Debug().Msgf("Getting buy nft txs for period %d - %d", periodStart, periodEnd)
	allTxs = append(allTxs, marketplaceModuletxs...)
	nftModuleTxs, err := r.getTxsByEvents(ctx, "transfer_nft.denom_id=%27"+collectionDenomId+"%27%20AND%20tx.height%3E"+fmt.Sprint(periodStartHeight)+"%20AND%20tx.height%3C"+fmt.Sprint(periodEndHeight))
	if err != nil {
		return []types.NftTransferEvent{}, err
	}
	log.Debug().Msgf("Done!")
	allTxs = append(allTxs, nftModuleTxs...)

	var txHashes []string
	for _, tx := range allTxs {
		txHashes = append(txHashes, tx.Hash)
	}

	log.Debug().Msgf("Getting txs from hasura for period %d - %d", periodStart, periodEnd)
	hasuraTxs, err := r.getTxsFromHasura(ctx, txHashes)
	if err != nil {
		return []types.NftTransferEvent{}, err
	}
	log.Debug().Msgf("Done!")
	hasuraTxHashmap := make(map[string]types.HasuraTx)
	for _, tx := range hasuraTxs {
		hasuraTxHashmap[tx.Hash] = tx
	}

	var transferEvents []types.NftTransferEvent

	for _, tx := range allTxs {
		var txTimestamp int64
		hasuraTx, ok := hasuraTxHashmap[tx.Hash]
		if !ok {
			log.Debug().Msgf("Could not find tx [%s] in hasura. Fetching from chain...", tx.Hash)
			heightInt, err := strconv.ParseInt(tx.Height, 10, 64)
			if err != nil {
				return []types.NftTransferEvent{}, err
			}
			block, err := r.getBlockAtHeight(ctx, heightInt)
			if err != nil {
				return []types.NftTransferEvent{}, err
			}
			txTimestamp, err = convertTimeToTimestamp(block.Header.Time)
			if err != nil {
				return nil, err
			}
			log.Debug().Msgf("Done!")
		} else {
			txTimestamp, err = convertTimeToTimestamp(hasuraTx.Block.Time)
			if err != nil {
				return nil, err
			}
		}

		if err != nil {
			return []types.NftTransferEvent{}, err
		}
		for _, event := range tx.TxResult.Events {
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
	timeLayout := "2006-01-02T15:04:05"
	lastInd := strings.LastIndex(timeString, ".")

	t, err := time.Parse(timeLayout, timeString[:lastInd])
	if err != nil {
		return 0, err
	}

	return t.Unix(), nil
}
