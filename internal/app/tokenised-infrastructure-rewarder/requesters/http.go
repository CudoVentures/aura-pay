package requesters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func NewRequester(config *infrastructure.Config) *Requester {
	return &Requester{config: config}
}

type Requester struct {
	config *infrastructure.Config
}

const (
	StatusCodeOK       = 200
	StatusCodeNotFound = 404
)

func (r *Requester) GetHasuraCollectionNftMintEvents(ctx context.Context, collectionDenomId string) (types.NftMintHistory, error) {
	jsonData := map[string]string{
		"query": fmt.Sprintf(`
		{
			nft_transfer_history(where: {denom_id: {_eq: "%s"}, old_owner: {_eq: "0x0"}}) {
				id
				timestamp
			  }
		}
        `, collectionDenomId),
	}

	jsonValue, _ := json.Marshal(jsonData)
	request, err := http.NewRequestWithContext(ctx, "POST", r.config.HasuraURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return types.NftMintHistory{}, err
	}
	client := &http.Client{Timeout: time.Second * 10}
	response, err := client.Do(request)
	if err != nil {
		log.Error().Msgf("The HTTP request failed with error %s\n", err)
		return types.NftMintHistory{}, nil
	}
	if response.StatusCode != StatusCodeOK {
		return types.NftMintHistory{}, fmt.Errorf("error! Request Failed: %s with StatusCode: %d", response.Status, response.StatusCode)
	}
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)
	if response.StatusCode != StatusCodeOK {
		return types.NftMintHistory{}, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", response.Status, response.StatusCode, string(data))
	}

	if err != nil {
		log.Error().Msgf("Could read data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return types.NftMintHistory{}, err
	}
	var res types.NftMintHistory
	if err := json.Unmarshal(data, &res); err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return types.NftMintHistory{}, err
	}

	return res, nil
}

func (r *Requester) GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error) {
	okStruct, err := r.getFarmDailyDataFromPool(ctx, farmName, sinceTimestamp)
	if err != nil {
		return -1, err
	}

	return okStruct[0].HashrateAccepted, nil
}

func (r *Requester) GetFarmStartTime(ctx context.Context, farmName string) (int64, error) {
	okStruct, err := r.getFarmDailyDataFromPool(ctx, farmName, "0")
	if err != nil {
		return -1, err
	}

	return okStruct[len(okStruct)-1].UnixTime, nil
}

func (r *Requester) getFarmDailyDataFromPool(ctx context.Context, farmName, sinceTimestamp string) (types.FarmHashRate, error) {
	requestString := fmt.Sprintf("/subaccount_hashrate_day/%s", farmName)

	req, err := http.NewRequestWithContext(ctx, "GET", r.config.FoundryPoolAPIBaseURL+requestString, nil)
	if err != nil {
		return types.FarmHashRate{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", r.config.FoundryPoolAPIKey)
	q := req.URL.Query()           // Get a copy of the query values.
	q.Add("start", sinceTimestamp) // Add a new value to the set.
	req.URL.RawQuery = q.Encode()  // Encode and assign back to the original query.

	client := &http.Client{Timeout: time.Second * 10}
	res, err := client.Do(req)
	if err != nil {
		return types.FarmHashRate{}, err
	}

	defer res.Body.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Error().Msgf("Could read farm (%s) total hash power data from foundry, error is: [%s]", farmName, err)
		return types.FarmHashRate{}, err
	}

	if res.StatusCode != StatusCodeOK {
		return types.FarmHashRate{}, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", res.Status, res.StatusCode, string(bytes))
	}

	okStruct := types.FarmHashRate{}

	if err := json.Unmarshal(bytes, &okStruct); err != nil {
		return types.FarmHashRate{}, err
	}

	return okStruct, nil
}

func (r *Requester) getTxsFromHasura(ctx context.Context, txHashes []string) ([]types.HasuraTx, error) {
	hashesString := ""
	for _, hash := range txHashes {
		hashesString += fmt.Sprintf(`"%s",`, hash)
	}
	jsonData := map[string]string{
		"query": fmt.Sprintf(`
            {
				transaction(where: {hash: {_in: %s}}) {
					hash
					block {
					  timestamp
					}
				}
            }
        `, fmt.Sprintf("[%s]", strings.TrimSuffix(hashesString, ","))),
	}

	jsonValue, _ := json.Marshal(jsonData)
	request, err := http.NewRequestWithContext(ctx, "POST", r.config.HasuraURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return []types.HasuraTx{}, err
	}
	client := &http.Client{Timeout: time.Second * 10}
	response, err := client.Do(request)
	if err != nil {
		log.Error().Msgf("The HTTP request failed with error %s\n", err)
		return []types.HasuraTx{}, nil
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return []types.HasuraTx{}, err
	}
	if response.StatusCode != StatusCodeOK {
		return []types.HasuraTx{}, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", response.Status, response.StatusCode, string(data))
	}

	var res types.HasuraTxResult
	if err := json.Unmarshal(data, &res); err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return []types.HasuraTx{}, err
	}

	return res.Data.Transactions, nil
}

func (r *Requester) GetFarmCollectionsFromHasura(ctx context.Context, farmId int64) (types.CollectionData, error) {
	jsonData := map[string]string{
		"query": fmt.Sprintf(`
            {
                denoms_by_data_property(args: {property_name: "farm_id", property_value: "%d"}) {
                    id,
                    data_json
                }
            }
        `, farmId),
	}
	jsonValue, _ := json.Marshal(jsonData)
	request, err := http.NewRequestWithContext(ctx, "POST", r.config.HasuraURL, bytes.NewBuffer(jsonValue))
	if err != nil {
		return types.CollectionData{}, err
	}
	client := &http.Client{Timeout: time.Second * 10}
	response, err := client.Do(request)
	if err != nil {
		log.Error().Msgf("The HTTP request failed with error %s\n", err)
		return types.CollectionData{}, nil
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return types.CollectionData{}, err
	}
	if response.StatusCode != StatusCodeOK {
		return types.CollectionData{}, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", response.Status, response.StatusCode, string(data))
	}

	var res types.CollectionData
	if err := json.Unmarshal(data, &res); err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return types.CollectionData{}, err
	}

	return res, nil
}

func (r *Requester) VerifyCollection(ctx context.Context, denomId string) (bool, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	requestString := fmt.Sprintf("/CudoVentures/cudos-node/marketplace/collection_by_denom_id/%s", denomId)

	req, err := http.NewRequestWithContext(ctx, "GET", r.config.NodeRestUrl+requestString, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, err
	}

	if res.StatusCode != StatusCodeOK {
		return false, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", res.Status, res.StatusCode, string(bytes))
	}

	okStruct := struct {
		Collection struct {
			ID       string `json:"id"`
			DenomID  string `json:"denomId"`
			Verified bool   `json:"verified"`
			Owner    string `json:"owner"`
		} `json:"Collection"`
	}{}

	if err := json.Unmarshal(bytes, &okStruct); err != nil {
		return false, err
	}

	return okStruct.Collection.Verified, nil
}

func (r *Requester) GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	reqBody := struct {
		DenomIds []string `json:"denom_ids"`
	}{DenomIds: denomIds}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.config.NodeRestUrl+"/nft/collectionsByDenomIds", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != StatusCodeOK {
		return nil, fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", res.Status, res.StatusCode, string(bytes))
	}

	okStruct := types.CollectionResponse{}

	if err := json.Unmarshal(bytes, &okStruct); err != nil {
		return nil, err
	}

	for i := 0; i < len(okStruct.Result.Collections); i++ {
		var parsedNfts []types.NFT
		for j := 0; j < len(okStruct.Result.Collections[i].Nfts); j++ {
			nft := okStruct.Result.Collections[i].Nfts[j]
			var nftDataJson types.NFTDataJson
			err := json.Unmarshal([]byte(nft.Data), &nftDataJson)
			if err != nil || nftDataJson.ExpirationDate == 0 || nftDataJson.HashRateOwned == 0 {
				// log.Warn().Msgf("Failed to parse NFT dataJson field. Skipping. NFT: %s", nft)
				continue
			}
			nft.DataJson = nftDataJson
			parsedNfts = append(parsedNfts, nft)
		}

		okStruct.Result.Collections[i].Nfts = parsedNfts
	}

	return okStruct.Result.Collections, nil
}

// SendMany Issues a curl request to the btc node to send funds to many addresses:
// curl --user myusername --data-binary '{"jsonrpc": "1.0", "id": "curltest", "method": "sendmany", "params": ["", {"bc1q09vm5lfy0j5reeulh4x5752q25uqqvz34hufdl":0.01,"bc1q02ad21edsxd23d32dfgqqsz4vv4nmtfzuklhy3":0.02}, 6, "testing"]}' -H 'content-type: text/plain;' http://127.0.0.1:8332/
func (r *Requester) SendMany(ctx context.Context, destinationAddressesWithAmount map[string]float64) (string, error) {

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	bytes, err := json.Marshal(destinationAddressesWithAmount)
	if err != nil {
		return "", err
	}
	escapedDestinationAddresses := string(bytes)

	subractFeeFromAddresses := []string{}
	for k := range destinationAddressesWithAmount {
		subractFeeFromAddresses = append(subractFeeFromAddresses, k)
	}

	bytes, err = json.Marshal(subractFeeFromAddresses)
	if err != nil {
		return "", err
	}
	escapedSubractFeeFromAddressesString := string(bytes)

	formatedString := fmt.Sprintf("{\"jsonrpc\": \"1.0\", \"id\": \"curl\", \"method\": \"sendmany\", \"params\": [\"\", %s, 6, \"\", %s, true]}", escapedDestinationAddresses, escapedSubractFeeFromAddressesString)

	body := strings.NewReader(formatedString)
	endPointToCall := fmt.Sprintf("http://%s:%s", r.config.BitcoinNodeUrl, r.config.BitcoinNodePort)
	req, err := http.NewRequestWithContext(ctx, "POST", endPointToCall, body)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(r.config.BitcoinNodeUserName, r.config.BitcoinNodePassword)
	req.Header.Set("Content-Type", "text/plain;")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	bytes, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != StatusCodeOK {
		return "", fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", resp.Status, resp.StatusCode, string(bytes))
	}

	okStruct := struct {
		TxHash string `json:"result"`
		Err    string `json:"error"`
	}{}

	if err := json.Unmarshal(bytes, &okStruct); err != nil {
		return "", err
	}

	if okStruct.Err != "" {
		return "", fmt.Errorf(okStruct.Err)
	}

	return okStruct.TxHash, nil
}

func (r *Requester) BumpFee(ctx context.Context, txId string) (string, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	optionals := "{\"replaceable\": true}" // marks the tx as BIP-125 once again
	formatedString := fmt.Sprintf("{\"jsonrpc\": \"1.0\", \"id\": \"curl\", \"method\": \"bumpfee\", \"params\": [%s, %s]}", txId, optionals)

	body := strings.NewReader(formatedString)
	endPointToCall := fmt.Sprintf("http://%s:%s", r.config.BitcoinNodeUrl, r.config.BitcoinNodePort)
	log.Debug().Msgf("Trying to bump fee with request %s", endPointToCall)

	req, err := http.NewRequestWithContext(ctx, "POST", endPointToCall, body)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(r.config.BitcoinNodeUserName, r.config.BitcoinNodePassword)
	req.Header.Set("Content-Type", "text/plain;")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != StatusCodeOK {
		return "", fmt.Errorf("error! Request Failed: %s with StatusCode: %d. Error: %s", resp.Status, resp.StatusCode, string(bts))
	}

	okStruct := struct {
		TxHash      string   `json:"txid"`
		Errors      []string `json:"errors"`
		OriginalFee string   `json:"origfee"`
		NewFee      string   `json:"fee"`
	}{}

	if err := json.Unmarshal(bts, &okStruct); err != nil {
		return "", err
	}

	if len(okStruct.Errors) != 0 {
		errs := strings.Join(okStruct.Errors[:], ",")
		return "", fmt.Errorf(errs)
	}

	return okStruct.TxHash, nil
}
