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
	Id          string `json:"id"`
	Name        string `json:"name"`
	Schema      string `json:"schema"`
	Creator     string `json:"creator"`
	Symbol      string `json:"symbol"`
	Traits      string `json:"traits"`
	Minter      string `json:"minter"`
	Description string `json:"description"`
	Data        string `json:"data"`
}

type NFT struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Uri      string `json:"uri"`
	Data     string `json:"data"`
	DataJson NFTDataJson
	Owner    string `json:"owner"`
}

type NFTDataJson struct {
	ExpirationDate int64   `json:"expiration_date"`
	HashRateOwned  float64 `json:"hash_rate_owned"`
}
