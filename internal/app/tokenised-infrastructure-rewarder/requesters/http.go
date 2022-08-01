package requesters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

func GetNFTsByIds(denomId string, tokenIds []int) (NFTCollectionResponse, error) {
	var config = infrastructure.NewConfig()
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	reqBody := GetSpecificNFTsQuery{denomId: denomId, NFTsIds: tokenIds}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatal(err)
		return NFTCollectionResponse{}, err
	}

	req, err := http.NewRequest("POST", config.NodeURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Fatal(err)
		return NFTCollectionResponse{}, err
	}
	req.Header.Add("Accept", "application/json")
	res, err := client.Do(req)
	bytes, err := ioutil.ReadAll(res.Body)
	okStruct := NFTCollectionResponse{}
	err = json.Unmarshal(bytes, &okStruct)
	return okStruct, err
}

func GetAllNonExpiredNFTsFromHasura() (NFTData, error) {
	var config = infrastructure.NewConfig()
	jsonData := map[string]string{
		"query": fmt.Sprintf(`
            {
                nfts_by_expiration_date(args: {expiration_date: "%s"}) {
                    id,
					denom_id,
                    data_json
                }
            }
        `, strconv.FormatInt(time.Now().UTC().Unix(), 10)), // possibly refactor with ntp server time
	}
	jsonValue, _ := json.Marshal(jsonData)
	request, err := http.NewRequest("POST", config.HasuraURL, bytes.NewBuffer(jsonValue))
	client := &http.Client{Timeout: time.Second * 10}
	response, err := client.Do(request)
	defer response.Body.Close()
	if err != nil {
		log.Fatalf("The HTTP request failed with error %s\n", err)
		return NFTData{}, nil
	}
	data, _ := ioutil.ReadAll(response.Body)
	var res NFTData
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return NFTData{}, err
	}
	return res, nil
}