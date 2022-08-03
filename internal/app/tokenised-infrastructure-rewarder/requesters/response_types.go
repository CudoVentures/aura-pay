package requesters

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
		Collection struct {
			Denom struct {
				Id      string `json:"id"`
				Name    string `json:"name"`
				Schema  string `json:"schema"`
				Creator string `json:"creator"`
			} `json:"denom"`
			Nfts []NFT `json:"nfts"`
		} `json:"collection"`
	} `json:"result"`
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
