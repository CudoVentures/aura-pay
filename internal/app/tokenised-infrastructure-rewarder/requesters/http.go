package requesters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
)

func GetNFTsByIds(denomId string, tokenIds []int) (NFTCollectionResponse, error) {
	var config = infrastructure.NewConfig()
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	var stringIds []string
	for _, id := range tokenIds {
		stringIds = append(stringIds, strconv.Itoa(id))
	}

	reqBody := GetSpecificNFTsQuery{DenomId: denomId, TokenIds: stringIds}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatal(err)
		return NFTCollectionResponse{}, err
	}

	req, err := http.NewRequest("POST", config.NodeRestUrl+"/nft/nftsByIds", bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Fatal(err)
		return NFTCollectionResponse{}, err
	}

	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	bytes, err := ioutil.ReadAll(res.Body)

	okStruct := NFTCollectionResponse{}

	err = json.Unmarshal(bytes, &okStruct)
	if err != nil {
		log.Fatal(err)
		return NFTCollectionResponse{}, err
	}

	for i := 0; i < len(okStruct.Result.Collection.Nfts); i++ {
		data := DataJson{}
		nft := &okStruct.Result.Collection.Nfts[i]
		err := json.Unmarshal([]byte(nft.Data), &data)
		if err != nil {
			log.Fatal(err)
			return NFTCollectionResponse{}, err
		}
		nft.DataJson = data
		fmt.Printf("test")
	}

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
	if err != nil {
		log.Fatalf("The HTTP request failed with error %s\n", err)
		return NFTData{}, nil
	}
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)

	if err != nil {
		log.Fatalf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return NFTData{}, err
	}
	var res NFTData
	err = json.Unmarshal(data, &res)
	if err != nil {
		log.Fatalf("Could not unmarshall data [%s] from hasura to the specific type, error is: [%s]", data, err)
		return NFTData{}, err
	}
	return res, nil
}
