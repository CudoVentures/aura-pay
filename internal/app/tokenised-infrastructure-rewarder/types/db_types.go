package types

type NFTPayoutTime struct {
	TokenId string `db:"token_id"`
	Time    int64
	Amount  string
}
