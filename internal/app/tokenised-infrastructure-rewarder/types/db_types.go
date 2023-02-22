package types

import (
	"time"

	"github.com/shopspring/decimal"
)

type Farm struct {
	Id             int64  `db:"id"`
	Name           string `db:"name"`
	Description    string `db:"description"`
	SubAccountName string `db:"sub_account_name"`
	// Location                           string  `db:"location"`
	TotalHashPower                     float64 `db:"total_farm_hashrate"`
	AddressForReceivingRewardsFromPool string  `db:"address_for_receiving_rewards_from_pool"`
	LeftoverRewardPayoutAddress        string  `db:"leftover_reward_payout_address"`
	MaintenanceFeePayoutAddress        string  `db:"maintenance_fee_payout_address"`
	MaintenanceFeeInBtc                float64 `db:"maintenance_fee_in_btc"`
	// Manufacturers                      []uint8 `db:"manufacturers"`
	// MinerTypes                         []uint8 `db:"miner_types"`
	// EnergySource                       []uint8 `db:"energy_source"`
	// Status                             string  `db:"status"`
	// Images                             [][]uint8 `db:"images"`
	// ProfileImage                       []uint8 `db:"profile_img"`
	// CoverImage                         []uint8 `db:"cover_img"`
	// PrimaryAccountOwnerName            string  `db:"primary_account_owner_name"`
	// PrimaryAccountOwnerEmail           string  `db:"primary_account_owner_email"`
	// CreatorId                          int     `db:"creator_id"`
	// DeletedAt                          int64   `db:"deleted_at"`
	// CreatedAt                          int64   `db:"created_at"`
	// UpdatedAt                          int64   `db:"updated_at"`
	// ResaleFarmRoyaltiesCudosAddress    string  `db:"resale_farm_royalties_cudos_address"`
	// CudosMintNftRoyaltiesPercent       float64 `db:"cudos_mint_nft_royalties_percent"`
	// CudosResaleNftRoyaltiesPercent     float64 `db:"cudos_resale_nft_royalties_percent"`
}

type NFTStatistics struct {
	Id                       string          `db:"id"`
	TokenId                  string          `db:"token_id"`
	DenomId                  string          `db:"denom_id"`
	FarmPaymentId            int64           `db:"farm_payment_id"`
	PayoutPeriodStart        int64           `db:"payout_period_start"`
	PayoutPeriodEnd          int64           `db:"payout_period_end"`
	Reward                   decimal.Decimal `db:"reward"`
	MaintenanceFee           decimal.Decimal `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee decimal.Decimal `db:"cudo_part_of_maintenance_fee"`
	NFTOwnersForPeriod       []NFTOwnerInformation
	TxHash                   string    `db:"tx_hash"`
	CreatedAt                time.Time `db:"createdAt"`
	UpdatedAt                time.Time `db:"updatedAt"`
}

type NFTStatisticsRepo struct {
	Id                       string `db:"id"`
	TokenId                  string `db:"token_id"`
	DenomId                  string `db:"denom_id"`
	FarmPaymentId            int64  `db:"farm_payment_id"`
	PayoutPeriodStart        int64  `db:"payout_period_start"`
	PayoutPeriodEnd          int64  `db:"payout_period_end"`
	Reward                   string `db:"reward"`
	MaintenanceFee           string `db:"maintenance_fee"`
	CUDOPartOfMaintenanceFee string `db:"cudo_part_of_maintenance_fee"`
	NFTOwnersForPeriod       []NFTOwnerInformationRepo
	TxHash                   string    `db:"tx_hash"`
	CreatedAt                time.Time `db:"createdAt"`
	UpdatedAt                time.Time `db:"updatedAt"`
}

type NFTOwnerInformation struct {
	TimeOwnedFrom      int64           `db:"time_owned_from"`
	TimeOwnedTo        int64           `db:"time_owned_to"`
	TotalTimeOwned     int64           `db:"total_time_owned"`
	PercentOfTimeOwned float64         `db:"percent_of_time_owned"`
	Owner              string          `db:"owner"`
	PayoutAddress      string          `db:"payout_address"`
	Reward             decimal.Decimal `db:"reward"`
	CreatedAt          time.Time       `db:"createdAt"`
	UpdatedAt          time.Time       `db:"updatedAt"`
}

type NFTOwnerInformationRepo struct {
	TimeOwnedFrom      int64     `db:"time_owned_from"`
	TimeOwnedTo        int64     `db:"time_owned_to"`
	TotalTimeOwned     int64     `db:"total_time_owned"`
	PercentOfTimeOwned float64   `db:"percent_of_time_owned"`
	Owner              string    `db:"owner"`
	PayoutAddress      string    `db:"payout_address"`
	Reward             string    `db:"reward"`
	CreatedAt          time.Time `db:"createdAt"`
	UpdatedAt          time.Time `db:"updatedAt"`
}

type TransactionHashWithStatus struct {
	Id                 string    `db:"id"`
	FarmPaymentId      int64     `db:"farm_payment_id"`
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
	FarmId             string    `db:"farm_id"`
	OldTxHash          string    `db:"old_tx_hash"`
	NewTxHash          string    `db:"new_tx_hash"`
	FarmSubAccountName string    `db:"farm_sub_account_name"`
	CreatedAt          time.Time `db:"createdAt"`
	UpdatedAt          time.Time `db:"updatedAt"`
}

type UTXOTransaction struct {
	Id        string    `db:"id"`
	FarmId    string    `db:"farm_id"`
	TxHash    string    `db:"tx_hash"`
	Processed bool      `db:"processed"`
	CreatedAt time.Time `db:"createdAt"`
	UpdatedAt time.Time `db:"updatedAt"`
}

type AddressThresholdAmountByFarm struct {
	Id         string    `db:"id"`
	BTCAddress string    `db:"btc_address"`
	FarmId     string    `db:"farm_id"`
	AmountBTC  string    `db:"amount_btc"`
	CreatedAt  time.Time `db:"createdAt"`
	UpdatedAt  time.Time `db:"updatedAt"`
}

type FarmPayment struct {
	Id        string          `db:"id"`
	FarmId    int64           `db:"farm_id"`
	AmountBTC decimal.Decimal `db:"amount_btc"`
	CreatedAt time.Time       `db:"createdAt"`
	UpdatedAt time.Time       `db:"updatedAt"`
}

type CollectionPaymentAllocation struct {
	Id                         int64           `db:"id"`
	FarmId                     int64           `db:"farm_id"`
	FarmPaymentId              int64           `db:"farm_payment_id"`
	CollectionId               int64           `db:"collection_id"`
	CollectionAllocationAmount decimal.Decimal `db:"collection_allocation_amount_btc"`
	CUDOGeneralFee             decimal.Decimal `db:"cudo_general_fee_btc"`
	CUDOMaintenanceFee         decimal.Decimal `db:"cudo_maintenance_fee_btc"`
	FarmUnsoldLeftovers        decimal.Decimal `db:"farm_unsold_leftover_btc"`
	FarmMaintenanceFee         decimal.Decimal `db:"farm_maintenance_fee_btc"`
	CreatedAt                  time.Time       `db:"createdAt"`
	UpdatedAt                  time.Time       `db:"updatedAt"`
}

type AuraPoolCollection struct {
	Id           int64   `db:"id"`
	DenomId      string  `db:"denom_id"`
	HashingPower float64 `db:"hashing_power"`
}

const (
	TransactionPending   = "Pending"
	TransactionCompleted = "Completed"
	TransactionFailed    = "Failed"
	TransactionReplaced  = "Replaced"
)
