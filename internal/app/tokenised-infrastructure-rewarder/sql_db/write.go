package sql_db

import (
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
)

func SaveDestionAddressesWithAmountHistory(tx *sqlx.Tx, address string, amount btcutil.Amount, txHash string, farmId string) {
	tx.MustExec("INSERT INTO statistics_destination_addresses_with_amount (address, amount, tx_hash, farm_id, payout_time, \"createdAt\", \"updatedAt\") VALUES ($1, $2, $3, $4, $5, $6, $7)", address, amount, txHash, farmId, time.Now().Unix(), time.Now().UTC(), time.Now().UTC())
}

func SaveNftInformationHistory(tx *sqlx.Tx, collectionDenomId string, tokenId string, payoutPeriodStart int64, payoutPeriodEnd int64, reward btcutil.Amount, txHash string, maintenanceFee btcutil.Amount, cudo_part_of_maintenance_fee btcutil.Amount) {
	tx.MustExec("INSERT INTO statistics_nft_payout_history (denom_id, token_id, payout_period_start, payout_period_end, reward, tx_hash, maintenance_fee, cudo_part_of_maintenance_fee, \"createdAt\", \"updatedAt\") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)", collectionDenomId, tokenId, payoutPeriodStart, payoutPeriodEnd, reward, txHash, maintenanceFee, cudo_part_of_maintenance_fee, time.Now().UTC(), time.Now().UTC())

}

// type NFTStatistics struct {
// 	TokenId                  string
// 	DenomId                  string
// 	PayoutPeriodStart        int64
// 	PayoutPeriodEnd          int64
// 	Reward                   btcutil.Amount
// 	MaintenanceFee           btcutil.Amount
// 	CUDOPartOfMaintenanceFee btcutil.Amount
// 	NFTOwnersForPeriod       []NFTOwnerInformation
// }

func SaveNFTOwnersForPeriodHistory(tx *sqlx.Tx, collectionDenomId string, tokenId string, timedOwnedFrom int64, timedOwnedTo int64, totalTimeOwned int64, percentOfTimeOwned float64, owner string, payoutAddress string, reward btcutil.Amount) {
	tx.MustExec("INSERT INTO statistics_nft_owners_payout_history (denom_id, token_id, time_owned_from, time_owned_to, total_time_owned, percent_of_time_owned ,owner, payout_address, reward, \"createdAt\", \"updatedAt\" ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
		collectionDenomId, tokenId, timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned, owner, payoutAddress, reward, time.Now().UTC(), time.Now().UTC())
}
