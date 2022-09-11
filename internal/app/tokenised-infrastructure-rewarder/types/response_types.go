package types

type NFTData struct {
	Data struct {
		NftsByExpirationDate []struct {
			Id       int         `json:"id"`
			DenomId  string      `json:"denom_id"`
			DataJson DataJsonNFT `json:"data_json"`
		} `json:"nfts_by_expiration_date"`
	} `json:"data"`
}

type CollectionData struct {
	Data struct {
		DenomsByDataProperty []struct {
			Id       string             `json:"id"`
			DataJson DataJsonCollection `json:"data_json"`
		} `json:"denoms_by_data_property"`
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

type FarmHashRate []FarmHashRateElement

type FarmHashRateElement struct {
	Time             string  `json:"time"`
	UnixTime         int64   `json:"unixTime"`
	AverageWorkers   float64 `json:"averageWorkers"`
	HashrateAccepted int64   `json:"hashrateAccepted"`
	HashrateStale    int64   `json:"hashrateStale"`
	HashrateRejected int64   `json:"hashrateRejected"`
}

type CollectionResponse struct {
	Height string           `json:"height"`
	Result CollectionResult `json:"result"`
}

type CollectionResult struct {
	Collection CollectionClass `json:"collection"`
}

type CollectionClass struct {
	Denom Denom `json:"denom"`
	Nfts  []NFT `json:"nfts"`
}
