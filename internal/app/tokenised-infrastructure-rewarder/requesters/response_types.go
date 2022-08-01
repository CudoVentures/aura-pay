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
	Id    string   `json:"id"`
	Name  string   `json:"name"`
	Uri   string   `json:"uri"`
	Data  DataJson `json:"data"`
	Owner string   `json:"owner"`
}

type DataJson struct {
	ExpirationDate int64 `json:"expirationDate"`
	// possibly others as well
}
