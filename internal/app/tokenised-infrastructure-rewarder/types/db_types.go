package types

import "github.com/btcsuite/btcd/btcutil"

type NFTPayoutTime struct {
	Id           int    `db:"id"`
	TokenId      string `db:"token_id"`
	PayoutTimeAt int64  `db:"payout_time_at"`
	Amount       string
	CreatedAt    int64 `db:"createdAt"`
	UpdatedAt    int64 `db:"updatedAt"`
}

type NFTStatistics struct {
	TokenId            string
	DenomId            string
	PayoutPeriodStart  int64
	PayoutPeriodEnd    int64
	RewardForNFT       btcutil.Amount
	NFTOwnersForPeriod []NFTOwnerInformation
}

type NFTOwnerInformation struct {
	TimeOwnedFrom      int64
	TimeOwnedTo        int64
	TotalTimeOwned     int64
	PercentOfTimeOwned float64
	Owner              string
	PayoutAddress      string
	Reward             btcutil.Amount
}
