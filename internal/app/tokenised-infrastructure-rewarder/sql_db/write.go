package sql_db

import (
	"context"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
)

func saveTxHashWithStatus(ctx context.Context, tx *sqlx.Tx, txHash string, txStatus string, farmId string, retryCount int) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertTxHashWithStatus, txHash, txStatus, farmId, retryCount, now.Unix(), now.UTC(), now.UTC())
	return err
}

func updateTxHashesWithStatus(ctx context.Context, tx *sqlx.Tx, txHashes []string, txStatus string) error {
	qry, args, err := sqlx.In(updateTxHashesWithStatusQuery, types.TransactionCompleted, txHashes)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, qry, args...); err != nil {
		return err
	}
	return nil
}

func saveDestionAddressesWithAmountHistory(ctx context.Context, tx *sqlx.Tx, address string, amount btcutil.Amount, txHash string, farmId string) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertDestinationAddressesWithAmountHistory, address, amount, txHash, farmId, now.Unix(), now.UTC(), now.UTC())
	return err
}

func saveNFTInformationHistory(ctx context.Context, tx *sqlx.Tx, collectionDenomId, tokenId string,
	payoutPeriodStart, payoutPeriodEnd int64, reward btcutil.Amount, txHash string,
	maintenanceFee, CudoPartOfMaintenanceFee btcutil.Amount) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertNFTInformationHistory, collectionDenomId, tokenId, payoutPeriodStart,
		payoutPeriodEnd, reward, txHash, maintenanceFee, CudoPartOfMaintenanceFee, now.UTC(), now.UTC())
	return err
}

func saveNFTOwnersForPeriodHistory(ctx context.Context, tx *sqlx.Tx, collectionDenomId string, tokenId string, timedOwnedFrom int64,
	timedOwnedTo int64, totalTimeOwned int64, percentOfTimeOwned float64, owner string, payoutAddress string, reward btcutil.Amount) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertNFTOnwersForPeriodHistory, collectionDenomId, tokenId,
		timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned, owner, payoutAddress, reward, now.UTC(), now.UTC())
	return err
}

func saveRBFTransactionHistory(ctx context.Context, tx *sqlx.Tx, old_tx_hash string, new_tx_hash string, farm_id string) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, insertRBFTransactionHistory, old_tx_hash, new_tx_hash,
		farm_id, now.UTC(), now.UTC())
	return err
}

const (
	insertTxHashWithStatus = `INSERT INTO statistics_tx_hash_status 
	(tx_hash, status, time_sent, farm_id, retry_count, createdAt, updatedAt) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	insertRBFTransactionHistory = `INSERT INTO rbf_transaction_history 
	(old_tx_hash, new_tx_hash, farm_id, createdAt, updatedAt) VALUES ($1, $2, $3, $4, $5)`

	insertDestinationAddressesWithAmountHistory = `INSERT INTO statistics_destination_addresses_with_amount 
		(address, amount, tx_hash, farm_id, payout_time, createdAt, updatedAt) VALUES ($1, $2, $3, $4, $5, $6, $7)`

	insertNFTInformationHistory = `INSERT INTO statistics_nft_payout_history (denom_id, token_id, payout_period_start, 
		payout_period_end, reward, tx_hash, maintenance_fee, cudo_part_of_maintenance_fee, createdAt, updatedAt) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	insertNFTOnwersForPeriodHistory = `INSERT INTO statistics_nft_owners_payout_history (denom_id, token_id, time_owned_from, time_owned_to, 
		total_time_owned, percent_of_time_owned ,owner, payout_address, reward, createdAt, updatedAt) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	updateTxHashesWithStatusQuery = `UPDATE statistics_tx_hash_status SET status=? where tx_hash IN (?)`
)
