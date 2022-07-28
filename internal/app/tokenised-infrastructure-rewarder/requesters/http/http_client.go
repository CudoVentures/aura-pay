package http

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func GetNFTsByIds(ids []int) error {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	reqBody := GetAllNFTsQuery{}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("POST", "http://example.com/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Accept", "application/json")
	res, err := client.Do(req)
	bytes, err := ioutil.ReadAll(res.Body)
	okStruct := NFT{}
	err = json.Unmarshal(bytes, &okStruct)
	return err
}
