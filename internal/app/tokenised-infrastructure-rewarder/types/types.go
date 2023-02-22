package types

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/shopspring/decimal"
)

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
	Id       string      `json:"id"`
	Name     string      `json:"name"`
	Uri      string      `json:"uri"`
	Data     string      `json:"data"`
	DataJson NFTDataJson `json:"data_json"`
	Owner    string      `json:"owner"`
}

type NFTDataJson struct {
	ExpirationDate int64   `json:"expiration_date"`
	HashRateOwned  float64 `json:"hash_rate_owned"`
}

type BtcNetworkParams struct {
	ChainParams      *chaincfg.Params
	MinConfirmations int
}

type AmountInfo struct {
	Amount           decimal.Decimal
	ThresholdReached bool
}
