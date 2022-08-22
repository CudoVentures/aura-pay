package types

type Farm struct {
	BTCAddress  string       `json:"BTCAddress"`
	Collections []Collection `json:"Collections"`
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
	ExpirationDate int64  `json:"expirationDate"`
	MachineId      string `json:"machineId"`
	HashRateId     string `json:"hashRateId"`
	// possibly others as well
}

type Data struct {
	ExpirationDate int64  `json:"expirationDate"`
	MachineId      string `json:"machineId"`
	HashRateId     string `json:"hashRateId"`
}

type Transaction struct {
	TxId               string `json:"txid"`
	SourceAddress      string `json:"source_address"`
	DestinationAddress string `json:"destination_address"`
	Amount             int64  `json:"amount"`
	UnsignedTx         string `json:"unsignedtx"`
	SignedTx           string `json:"signedtx"`
}
