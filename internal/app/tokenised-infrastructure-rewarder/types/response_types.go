package types

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
	HashrateAccepted float64 `json:"hashrateAccepted"`
	HashrateStale    int64   `json:"hashrateStale"`
	HashrateRejected int64   `json:"hashrateRejected"`
}

type CollectionResponse struct {
	Height string           `json:"height"`
	Result CollectionResult `json:"result"`
}

type CollectionResult struct {
	Collections []Collection `json:"collections"`
}

type CollectionData struct {
	Data struct {
		DenomsByDataProperty []struct {
			Id       string             `json:"id"`
			DataJson DataJsonCollection `json:"data_json"`
		} `json:"denoms_by_data_property"`
	} `json:"data"`
}

type DataJsonCollection struct { // hasura response
	FarmId string `json:"farm_id"`
	Owner  string `json:"owner"`
}
