package coingecko

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// GetTokensPrices queries the remote APIs to get the token prices of all the tokens having the given ids
func GetBtcPrice(currency string, ids []string) (float64, error) {
	var prices []MarketTicker

	resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd")
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	bz, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error while reading response body: %s", err)
	}

	err = json.Unmarshal(bz, &prices)
	if err != nil {
		return 0, fmt.Errorf("error while unmarshaling response body: %s", err)
	}

	return prices[0].CurrentPrice, nil
}
