package types

import (
	"time"

	"github.com/btcsuite/btcd/btcutil"
)

type NFTStatistics struct {
	Id                       string         `db:"id"`
	TokenId                  string         `db:"token_id"`
	DenomId                  string         `db:"denom_id"`
	PayoutPeriodStart        int64          `db:"payout_period_start"`
	PayoutPeriodEnd          int64          `db:"payout_period_end"`
	Reward                   btcutil.Amount `db:"reward"`
	MaintenanceFee           btcutil.Amount `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee btcutil.Amount `db:"cudo_part_of_maintenance_fee"`
	CUDOPartOfReward         btcutil.Amount `db:"cudo_part_of_reward"`
	NFTOwnersForPeriod       []NFTOwnerInformation
	TxHash                   string    `db:"tx_hash"`
	CreatedAt                time.Time `db:"createdAt"`
	UpdatedAt                time.Time `db:"updatedAt"`
}

type NFTStatisticsRepo struct {
	Id                       string  `db:"id"`
	TokenId                  string  `db:"token_id"`
	DenomId                  string  `db:"denom_id"`
	PayoutPeriodStart        int64   `db:"payout_period_start"`
	PayoutPeriodEnd          int64   `db:"payout_period_end"`
	Reward                   float64 `db:"reward"`
	MaintenanceFee           float64 `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee float64 `db:"cudo_part_of_maintenance_fee"`
	CUDOPartOfReward         float64 `db:"cudo_part_of_reward"`
	NFTOwnersForPeriod       []NFTOwnerInformationRepo
	TxHash                   string    `db:"tx_hash"`
	CreatedAt                time.Time `db:"createdAt"`
	UpdatedAt                time.Time `db:"updatedAt"`
}

type NFTOwnerInformation struct {
	TimeOwnedFrom      int64          `db:"time_owned_from"`
	TimeOwnedTo        int64          `db:"time_owned_to"`
	TotalTimeOwned     int64          `db:"total_time_owned"`
	PercentOfTimeOwned float64        `db:"percent_of_time_owned"`
	Owner              string         `db:"owner"`
	PayoutAddress      string         `db:"payout_address"`
	Reward             btcutil.Amount `db:"reward"`
	CreatedAt          time.Time      `db:"createdAt"`
	UpdatedAt          time.Time      `db:"updatedAt"`
}

type NFTOwnerInformationRepo struct {
	TimeOwnedFrom      int64     `db:"time_owned_from"`
	TimeOwnedTo        int64     `db:"time_owned_to"`
	TotalTimeOwned     int64     `db:"total_time_owned"`
	PercentOfTimeOwned float64   `db:"percent_of_time_owned"`
	Owner              string    `db:"owner"`
	PayoutAddress      string    `db:"payout_address"`
	Reward             float64   `db:"reward"`
	CreatedAt          time.Time `db:"createdAt"`
	UpdatedAt          time.Time `db:"updatedAt"`
}

type TransactionHashWithStatus struct {
	Id                 string    `db:"id"`
	TxHash             string    `db:"tx_hash"`
	Status             string    `db:"status"`
	TimeSent           int64     `db:"time_sent"`
	FarmSubAccountName string    `db:"farm_sub_account_name"`
	RetryCount         int       `db:"retry_count"`
	CreatedAt          time.Time `db:"createdAt"`
	UpdatedAt          time.Time `db:"updatedAt"`
}

type RBFTransactionHistory struct {
	Id                 string    `db:"id"`
	OldTxHash          string    `db:"old_tx_hash"`
	NewTxHash          string    `db:"new_tx_hash"`
	FarmSubAccountName string    `db:"farm_sub_account_name"`
	CreatedAt          time.Time `db:"createdAt"`
	UpdatedAt          time.Time `db:"updatedAt"`
}

type UTXOTransaction struct {
	Id        string    `db:"id"`
	TxHash    string    `db:"tx_hash"`
	Processed bool      `db:"processed"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

type AddressThresholdAmountByFarm struct {
	Id         string    `db:"id"`
	BTCAddress string    `db:"btc_address"`
	FarmId     int64     `db:"farm_id"`
	AmountBTC  float64   `db:"amount_btc"`
	CreatedAt  time.Time `db:"createdAt"`
	UpdatedAt  time.Time `db:"updatedAt"`
}

const (
	TransactionPending   = "Pending"
	TransactionCompleted = "Completed"
	TransactionFailed    = "Failed"
	TransactionReplaced  = "Replaced"
)
