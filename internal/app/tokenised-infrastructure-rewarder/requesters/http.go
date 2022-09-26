package requesters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func NewRequester(config infrastructure.Config) *Requester {
	return &Requester{config: config}
}

type Requester struct {
	config infrastructure.Config
}

func (r *Requester) GetPayoutAddressFromNode(cudosAddress string, network string, tokenId string, denomId string) (string, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// cudos1tr9jp0eqza9tvdvqzgyff9n3kdfew8uzhcyuwq/BTC/1@test
	requestString := fmt.Sprintf("/CudoVentures/cudos-node/addressbook/address/%s/%s/%s@%s", cudosAddress, network, tokenId, denomId)

	req, err := http.NewRequest("GET", r.config.NodeRestUrl+requestString, nil)
	if err != nil {
		log.Error().Msg(err.Error())
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Error().Msg(err.Error())
		return "", err
	}
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := types.MappedAddress{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return "", err
	}

	return okStruct.Address.Value, nil

}

func (r *Requester) GetNftTransferHistory(collectionDenomId string, nftId string, fromTimestamp int64) (types.NftTransferHistory, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	requestString := fmt.Sprintf("/transfer-events?denom=%s&nft=%s", collectionDenomId, nftId)

	req, err := http.NewRequest("GET", r.config.HasuraActionsURL+requestString, nil)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := types.NftTransferHistory{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	return okStruct, nil
}

func (r *Requester) GetFarmTotalHashPowerFromPoolToday(farmName string, sinceTimestamp string) (float64, error) {
	requestString := fmt.Sprintf("/subaccount_hashrate_day/%s", farmName)

	req, err := http.NewRequest("GET", r.config.FoundryPoolAPIBaseURL+requestString, nil)
	if err != nil {
		log.Error().Msg(err.Error())
		return -1, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", r.config.FoundryPoolAPIKey)
	q := req.URL.Query()           // Get a copy of the query values.
	q.Add("start", sinceTimestamp) // Add a new value to the set.
	req.URL.RawQuery = q.Encode()  // Encode and assign back to the original query.

	client := &http.Client{Timeout: time.Second * 10}
	res, err := client.Do(req)
	if err != nil {
		log.Error().Msg(err.Error())
		return -1, err
	}
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := types.FarmHashRate{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return -1, err
	}

	return okStruct[0].HashrateAccepted, nil
}

func (r *Requester) GetFarmCollectionsFromHasura(farmId string) (types.CollectionData, error) {
	jsonData := map[string]string{
		"query": fmt.Sprintf(`
            {
                denoms_by_data_property(args: {property_name: "farm_id", property_value: "%s"}) {
                    id,
                    data_json
                }
            }
        `, farmId),
	}
	jsonValue, _ := json.Marshal(jsonData)
	request, err := http.NewRequest("POST", r.config.HasuraURL, bytes.NewBuffer(jsonValue))
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
	var res types.CollectionData
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Error().Msgf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return types.CollectionData{}, err
	}
	return res, nil
}

func (r *Requester) GetFarms() ([]types.Farm, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	requestString := "getController/GetAllFarms" // change once you know what it is

	req, err := http.NewRequest("GET", r.config.AuraPoolBackEndUrl+requestString, nil)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := []types.Farm{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	if r.config.IsTesting { //TODO: Remove once backend is up
		Collection := types.Collection{Denom: types.Denom{Id: "test"}, Nfts: []types.NFT{}}
		testFarm := types.Farm{Id: "test", SubAccountName: "test", BTCWallet: "testwallet2", Collections: []types.Collection{Collection}}
		return []types.Farm{testFarm}, nil
	}

	return okStruct, nil

}

func (r *Requester) VerifyCollection(denomId string) (bool, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	requestString := fmt.Sprintf("/CudoVentures/cudos-node/marketplace/collection_by_denom_id/%s", denomId)

	req, err := http.NewRequest("GET", r.config.NodeRestUrl+requestString, nil)
	if err != nil {
		log.Error().Msg(err.Error())
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Error().Msg(err.Error())
		return false, err
	}
	bytes, err := ioutil.ReadAll(res.Body) // Generated by https://quicktype.io

	okStruct := struct {
		Collection struct {
			ID       string `json:"id"`
			DenomID  string `json:"denomId"`
			Verified bool   `json:"verified"`
			Owner    string `json:"owner"`
		} `json:"Collection"`
	}{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return false, err
	}

	return okStruct.Collection.Verified, nil
}

func (r *Requester) GetFarmCollectionWithNFTs(denomIds []string) ([]types.Collection, error) {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	var idsArray []string
	for _, id := range denomIds {
		idsArray = append(idsArray, id)
	}

	reqBody := struct {
		DenomIds []string `json:"denom_ids"`
	}{DenomIds: idsArray}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	req, err := http.NewRequest("POST", r.config.NodeRestUrl+"/nft/collectionsByDenomIds", bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := types.CollectionResponse{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Error().Msg(err.Error())
		return nil, err
	}

	return okStruct.Result.Collections, nil
}
