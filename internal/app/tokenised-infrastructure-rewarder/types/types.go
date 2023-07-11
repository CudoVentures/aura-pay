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

type BtcWalletTransaction struct {
	Amount            float64                       `json:"amount"`
	Fee               float64                       `json:"fee"`
	Confirmations     int64                         `json:"confirmations"`
	Trusted           bool                          `json:"trusted"`
	Txid              string                        `json:"txid"`
	WalletConflicts   []string                      `json:"walletconflicts"`
	Time              uint64                        `json:"time"`
	Timereceived      uint64                        `json:"timereceived"`
	Bip125Replaceable string                        `json:"bip125-replaceable"`
	ReplacedByTxid    string                        `json:"replaced_by_txid"`
	ReplacesTxid      string                        `json:"replaces_txid"`
	Details           []BtcWalletTransactionDetails `json:"details"`
	Hex               string                        `json:"hex"`
}

type BtcWalletTransactionDetails struct {
	Address   string  `json:"address"`
	Category  string  `json:"category"`
	Amount    float64 `json:"amount"`
	Vout      uint64  `json:"vout"`
	Fee       float64 `json:"fee"`
	Abandoned bool    `json:"abandoned"`
}
