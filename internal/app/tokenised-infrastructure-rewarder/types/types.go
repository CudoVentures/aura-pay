package types

type Farm struct {
	Id                      string       `json:"id"`
	SubAccountName          string       `json:"sub_account_name"`
	BTCWallet               string       `json:"btc_wallet"`
	DefaultBTCPayoutAddress string       `json:"default_btc_payout_address"`
	Collections             []Collection `json:"collections"`
}

type Collection struct {
	Denom              Denom `json:"denom"`
	Nfts               []NFT `json:"nfts"`
	HashRate           int64 `json:"hash_rate"`
	HashRateAtCreation int64 `json:"hash_rate_at_creation"`
}

type Denom struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Schema  string `json:"schema"`
	Creator string `json:"creator"`
	Symbol  string `json:"symbol"`
}

type NFT struct {
	Id       string      `json:"id"`
	Name     string      `json:"name"`
	Uri      string      `json:"uri"`
	Data     string      `json:"data"`
	DataJson DataJsonNFT `json:"data_json"` // TODO! : Fix this by having an unified json data type on both the node,bdjuno/hasura and rewarder
	Owner    string      `json:"owner"`
}

type DataJsonNFT struct { // hasura response
	ExpirationDate                    int64   `json:"expiration_date"`
	HashRateOwned                     float64 `json:"hash_rate_owned"`
	TotalCollectionHashRateWhenMinted int64   `json:"total_collection_hash_rate_when_minted"`
	// possibly others as well
}

type DataJsonCollection struct { // hasura response
	FarmId string `json:"farm_id"`
	Owner  string `json:"owner"`
}

type Data struct {
	ExpirationDate                    int64 `json:"expiration_date"`
	HashRateOwned                     int64 `json:"hash_rate_owned"`
	TotalCollectionHashRateWhenMinted int64 `json:"total_collection_hash_rate_when_minted"`
}
