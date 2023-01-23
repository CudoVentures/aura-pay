package sql_db

import (
	"context"
	"database/sql"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
)

func (tx *DbTx) saveFarmPaymentStatistics(ctx context.Context, farmId int64, amountBtc float64) (int64, error) {
	now := time.Now()

	_, err := tx.ExecContext(ctx, insertFarmPaymentStatistics, farmId, amountBtc, now.UTC(), now.UTC())
	if err != nil {
		return 0, err
	}

	var id int64
	row := tx.QueryRow("SELECT max(id) from farm_payment_statistics")
	row.Scan(&id)

	return id, err
}

func (tx *DbTx) saveCollectionPaymentAllocation(
	ctx context.Context,
	farmId int64,
	farmPaymentId int64,
	collectionId int64,
	collectionAllocationAmount float64,
	cudoGeneralFee float64,
	cudoMaintenanceFee float64,
	farmUnsoldLeftovers float64,
	farmMaintenanceFee float64,
) error {
	now := time.Now()

	_, err := tx.ExecContext(
		ctx,
		insertCollectionPaymentAllocation,
		farmId,
		farmPaymentId,
		collectionId,
		collectionAllocationAmount,
		cudoGeneralFee,
		cudoMaintenanceFee,
		farmUnsoldLeftovers,
		farmMaintenanceFee,
		now.UTC(),
		now.UTC(),
	)

	return err
}

func (tx *DbTx) saveDestinationAddressesWithAmountHistory(ctx context.Context, address string, amountInfo types.AmountInfo, txHash string, farmId, farmPaymentId int64) error {
	now := time.Now()
	if !amountInfo.ThresholdReached {
		txHash = "" // the funds were not sent but accumulated, we keep this record as statistic that they were spread but with empty tx hash
	}
	_, err := tx.ExecContext(ctx, insertDestinationAddressesWithAmountHistory, address, amountInfo.Amount.ToBTC(), txHash, farmId, farmPaymentId, now.Unix(), amountInfo.ThresholdReached, now.UTC(), now.UTC())
	return err

}

// add to this table
func (tx *DbTx) saveNFTInformationHistory(
	ctx context.Context,
	collectionDenomId,
	tokenId string,
	farmPaymentId int64,
	payoutPeriodStart,
	payoutPeriodEnd int64,
	reward btcutil.Amount,
	txHash string,
	maintenanceFee, CudoPartOfMaintenanceFee btcutil.Amount) (int, error) {

	var id int
	now := time.Now()

	if err := tx.QueryRowContext(ctx, insertNFTInformationHistory, collectionDenomId, tokenId, farmPaymentId, payoutPeriodStart,
		payoutPeriodEnd, reward.ToBTC(), txHash, maintenanceFee.ToBTC(), CudoPartOfMaintenanceFee.ToBTC(), now.UTC(), now.UTC()).Scan(&id); err != nil {
		return -1, err
	}

	return id, nil
}

func (tx *DbTx) saveNFTOwnersForPeriodHistory(ctx context.Context, timedOwnedFrom int64, timedOwnedTo int64, totalTimeOwned int64, percentOfTimeOwned float64, owner string, payoutAddress string, reward btcutil.Amount, nftPayoutHistoryId int, sent bool) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertNFTOnwersForPeriodHistory,
		timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned, owner, payoutAddress, reward.ToBTC(), nftPayoutHistoryId, sent, now.UTC(), now.UTC())
	return err
}

func (tx *DbTx) saveRBFTransactionHistory(ctx context.Context, oldTxHash string, newTxHash string, farmSubAccountName string) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertRBFTransactionHistory, oldTxHash, newTxHash, farmSubAccountName, now.UTC(), now.UTC())
	return err
}

func (sdb *SqlDB) SaveTxHashWithStatus(ctx context.Context, txHash, txStatus, farmSubAccountName string, retryCount int) error {
	return saveTxHashWithStatus(ctx, sdb, txHash, txStatus, farmSubAccountName, retryCount)
}

func saveTxHashWithStatus(ctx context.Context, sqlExec SqlExecutor, txHash, txStatus, farmSubAccountName string, retryCount int) error {
	now := time.Now()
	_, err := sqlExec.ExecContext(ctx, insertTxHashWithStatus, txHash, txStatus, now.Unix(), farmSubAccountName, retryCount, now.UTC(), now.UTC())
	return err
}

func (sdb *SqlDB) UpdateTransactionsStatus(ctx context.Context, txHashes []string, txStatus string) error {
	return updateTransactionsStatus(ctx, sdb, txHashes, txStatus)
}

func updateTransactionsStatus(ctx context.Context, sqlExec SqlExecutor, txHashes []string, txStatus string) error {
	for _, hash := range txHashes {
		_, err := sqlExec.ExecContext(ctx, updateTxHashesWithStatusQuery, txStatus, hash)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tx *DbTx) updateCurrentAcummulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount btcutil.Amount) error {
	_, err := tx.ExecContext(ctx, updateThresholdAmounts, amount.ToBTC(), address, farmId)
	return err
}

func (tx *DbTx) markUTXOsAsProcessed(ctx context.Context, tx_hashes []string) error {
	var UTXOMaps []map[string]interface{}
	for _, hash := range tx_hashes {
		m := map[string]interface{}{
			"tx_hash":   hash,
			"processed": true,
			"createdAt": time.Now().UTC(),
			"updatedAt": time.Now().UTC(),
		}
		UTXOMaps = append(UTXOMaps, m)
	}

	_, err := tx.NamedExecContext(ctx, insertUTXOWithStatus, UTXOMaps)
	return err
}

func (sdb *SqlDB) SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount int) error {
	_, err := sdb.ExecContext(ctx, insertInitialThresholdAmount, address, farmId, amount, time.Now().UTC(), time.Now().UTC())
	return err

}

const (
	insertUTXOWithStatus = `INSERT INTO utxo_transactions (tx_hash, processed, "createdAt", "updatedAt")
	   VALUES (:tx_hash, :processed, :createdAt, :updatedAt)`

	insertTxHashWithStatus = `INSERT INTO statistics_tx_hash_status
	(tx_hash, status, time_sent, farm_sub_account_name, retry_count, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4, $5, $6, $7)`

	insertRBFTransactionHistory = `INSERT INTO rbf_transaction_history
	(old_tx_hash, new_tx_hash, farm_sub_account_name, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4, $5)`

	insertDestinationAddressesWithAmountHistory = `INSERT INTO statistics_destination_addresses_with_amount
		(address, amount_btc, tx_hash, farm_id, farm_payment_id, payout_time, threshold_reached, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	insertNFTInformationHistory = `INSERT INTO statistics_nft_payout_history (denom_id, token_id, farm_payment_id, payout_period_start,
		payout_period_end, reward, tx_hash, maintenance_fee, cudo_part_of_maintenance_fee, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`

	insertNFTOnwersForPeriodHistory = `INSERT INTO statistics_nft_owners_payout_history (time_owned_from, time_owned_to,
		total_time_owned, percent_of_time_owned ,owner, payout_address, reward, nft_payout_history_id, sent, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	updateTxHashesWithStatusQuery = `UPDATE statistics_tx_hash_status SET status=$1 where tx_hash=$2`

	updateThresholdAmounts = `UPDATE threshold_amounts SET amount_btc=$1 where btc_address=$2 and farm_id=$3`

	insertInitialThresholdAmount = `INSERT INTO threshold_amounts
	(btc_address, farm_id, amount_btc, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4, $5)`

	insertFarmPaymentStatistics = `INSERT INTO farm_payment_statistics
	(farm_id, amount_btc, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4)`

	insertCollectionPaymentAllocation = `INSERT INTO collection_payment_allocations
	(farm_id, farm_payment_id, collection_id, collection_allocation_amount_btc, cudo_general_fee_btc, cudo_maintenance_fee_btc, farm_unsold_leftover_btc, farm_maintenance_fee_btc, "createdAt", "updatedAt") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
)

type SqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
