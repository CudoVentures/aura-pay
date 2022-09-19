package types

import "github.com/btcsuite/btcd/btcutil"

type NFTPayoutTime struct {
	TokenId string `db:"token_id"`
	Time    int64
	Amount  string
}

type NFTStatistics struct {
	TokenId           string
	PayoutPeriodStart int64
	PayoutPeriodEnd   int64
	RewardForNFT      btcutil.Amount
	AdditionalData    []StatisticsAdditionalData
}

type StatisticsAdditionalData struct {
	TimeOwnedFrom      int64
	TimeOwnedTo        int64
	TotalTimeOwned     int64
	PercentOfTimeOwned float64
	Owner              string
	PayoutAddress      string
	Reward             btcutil.Amount
}
