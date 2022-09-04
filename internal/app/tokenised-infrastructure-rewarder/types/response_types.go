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

type MappedAddress struct {
	Address Address `json:"address"`
}

type Address struct {
	Network string `json:"network"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Creator string `json:"creator"`
}

type NftTransferHistory []NftTransferHistoryElement

type NftTransferHistoryElement struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Timestamp int64  `json:"timestamp"`
}
