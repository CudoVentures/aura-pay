package types

import "github.com/btcsuite/btcd/btcutil"

type NFTStatistics struct {
	TokenId                  string `db:"token_id"`
	DenomId                  string `db:"denom_id"`
	PayoutPeriodStart        int64  `db:"payout_period_start"`
	PayoutPeriodEnd          int64  `db:"payout_period_end"`
	Reward                   btcutil.Amount
	MaintenanceFee           btcutil.Amount `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee btcutil.Amount `db:"cudo_part_of_maintenance_fee"`
	NFTOwnersForPeriod       []NFTOwnerInformation
}

type NFTOwnerInformation struct {
	TimeOwnedFrom      int64   `db:"time_owned_from"`
	TimeOwnedTo        int64   `db:"time_owned_to"`
	TotalTimeOwned     int64   `db:"total_time_owned"`
	PercentOfTimeOwned float64 `db:"percent_of_time_owned"`
	Owner              string
	PayoutAddress      string `db:"payout_address"`
	Reward             btcutil.Amount
}
