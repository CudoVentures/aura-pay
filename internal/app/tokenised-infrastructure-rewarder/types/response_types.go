package types

type NFTData struct {
	Data struct {
		NftsByExpirationDate []struct {
			Id       int      `json:"id"`
			DenomId  string   `json:"denom_id"`
			DataJson DataJson `json:"data_json"`
		} `json:"nfts_by_expiration_date"`
	} `json:"data"`
}

type NFTCollectionResponse struct {
	Height string `json:"height"`
	Result struct {
		Collection Collection `json:"collection"`
	} `json:"result"`
}
