package types

type Farm struct {
	Id          string       `json:"id"`
	Name        string       `json:"name"`
	BTCWallet   string       `json:"btc_wallet"`
	Collections []Collection `json:"collections"`
}

type Collection struct {
	Denom Denom `json:"denom"`
	Nfts  []NFT `json:"nfts"`
}

type Denom struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Schema  string `json:"schema"`
	Creator string `json:"creator"`
}

type NFT struct {
	Id       string   `json:"id"`
	Name     string   `json:"name"`
	Uri      string   `json:"uri"`
	Data     string   `json:"data"`
	DataJson DataJson `json:"data_json"` // TODO! : Fix this by having an unified json data type on both the node,bdjuno/hasura and rewarder
	Owner    string   `json:"owner"`
}

type DataJson struct { // hasura response
	ExpirationDate int64  `json:"expiration_date"`
	HashRate       string `json:"hash_rate"`
	TotalHashRate  string `json:"total_hash_rate"`
	// possibly others as well
}

type Data struct {
	ExpirationDate int64  `json:"expiration_date"`
	HashRate       string `json:"hash_rate"`
	TotalHashRate  string `json:"total_hash_rate"`
}
