package types

type Farm struct {
	Id                      string       `json:"id"`
	SubAccountName          string       `json:"sub_account_name"`
	BTCWallet               string       `json:"btc_wallet"`
	DefaultBTCPayoutAddress string       `json:"default_btc_payout_address"`
	Collections             []Collection `json:"collections"`
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
	Symbol  string `json:"symbol"`
}

type NFT struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Uri   string `json:"uri"`
	Data  Data   `json:"data"`
	Owner string `json:"owner"`
}

type Data struct { // hasura response
	ExpirationDate                    int64   `json:"expiration_date"`
	HashRateOwned                     float64 `json:"hash_rate_owned"`
	TotalCollectionHashRateWhenMinted int64   `json:"total_collection_hash_rate_when_minted"`
	// possibly others as well
}
