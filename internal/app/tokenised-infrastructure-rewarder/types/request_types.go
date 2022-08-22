package types

type GetSpecificNFTsQuery struct {
	DenomId  string   `json:"denom_id"`
	TokenIds []string `json:"token_ids"`
}
