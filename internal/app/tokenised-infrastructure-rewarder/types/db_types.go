package types

import "github.com/btcsuite/btcd/btcutil"

type NFTStatistics struct {
	Id                       string `db:"id"`
	TokenId                  string `db:"token_id"`
	DenomId                  string `db:"denom_id"`
	PayoutPeriodStart        int64  `db:"payout_period_start"`
	PayoutPeriodEnd          int64  `db:"payout_period_end"`
	Reward                   btcutil.Amount
	MaintenanceFee           btcutil.Amount `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee btcutil.Amount `db:"cudo_part_of_maintenance_fee"`
	NFTOwnersForPeriod       []NFTOwnerInformation
	TxHash                   string `db:"tx_hash"`
	CreatedAt                int64  `db:"createdAt"`
	UpdatedAt                int64  `db:"updatedAt"`
}

type NFTOwnerInformation struct {
	TimeOwnedFrom      int64   `db:"time_owned_from"`
	TimeOwnedTo        int64   `db:"time_owned_to"`
	TotalTimeOwned     int64   `db:"total_time_owned"`
	PercentOfTimeOwned float64 `db:"percent_of_time_owned"`
	Owner              string  // not used anywhere?
	PayoutAddress      string  `db:"payout_address"`
	Reward             btcutil.Amount
	CreatedAt          int64 `db:"createdAt"`
	UpdatedAt          int64 `db:"updatedAt"`
}

type TransactionHashWithStatus struct {
	TxHash             string `db:"tx_hash"`
	Status             string `db:"status"`
	TimeSent           int64  `db:"time_sent"`
	FarmSubAccountName string `db:"farm_sub_account_name"`
	RetryCount         int    `db:"retry_count"`
	CreatedAt          int64  `db:"createdAt"`
	UpdatedAt          int64  `db:"updatedAt"`
}

type RBFTransactionHistory struct {
	OldTxHash string `db:"old_tx_hash"`
	NewTxHash string `db:"new_tx_hash"`
	CreatedAt int64  `db:"createdAt"`
	UpdatedAt int64  `db:"updatedAt"`
}

type UTXOTransaction struct {
	TxHash    string `db:"tx_hash"`
	Status    string `db:"status"`
	CreatedAt int64  `db:"createdAt"`
	UpdatedAt int64  `db:"updatedAt"`
}

type AddressThresholdAmountByFarm struct {
	BTCAddress string `db:"btc_address"`
	FarmId     int64  `db:"farm_id"`
	Amount     int64  `db:"amount"`
	CreatedAt  int64  `db:"createdAt"`
	UpdatedAt  int64  `db:"updatedAt"`
}

const (
	TransactionPending   = "Pending"
	TransactionCompleted = "Completed"
	TransactionFailed    = "Failed"
	TransactionReplaced  = "Replaced"
)
