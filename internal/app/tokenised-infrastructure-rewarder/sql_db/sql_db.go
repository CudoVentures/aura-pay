package sql_db

import (
	"context"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewSqlDB(db *sqlx.DB) *sqlDB {
	return &sqlDB{db: db}
}

func (sdb *sqlDB) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, txHash, farmId string) (retErr error) {
	sql_tx, err := sdb.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %s", err)
	}

	defer func() {
		if retErr != nil {
			if err := sql_tx.Rollback(); err != nil {
				log.Error().Msgf("failed to rollback: %s, during: %s", err, retErr)
			}
		}
	}()

	for address, amount := range destinationAddressesWithAmount {
		if retErr = saveDestionAddressesWithAmountHistory(ctx, sql_tx, address, amount, txHash, farmId); retErr != nil {
			return
		}
	}

	for _, nftStatistic := range statistics {
		if retErr = saveNFTInformationHistory(ctx, sql_tx, nftStatistic.DenomId, nftStatistic.TokenId,
			nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.Reward, txHash,
			nftStatistic.MaintenanceFee, nftStatistic.CUDOPartOfMaintenanceFee); retErr != nil {
			return
		}
		for _, ownersForPeriod := range nftStatistic.NFTOwnersForPeriod {
			if retErr = saveNFTOwnersForPeriodHistory(ctx, sql_tx, nftStatistic.DenomId, nftStatistic.TokenId,
				ownersForPeriod.TimeOwnedFrom, ownersForPeriod.TimeOwnedTo, ownersForPeriod.TotalTimeOwned,
				ownersForPeriod.PercentOfTimeOwned, ownersForPeriod.Owner, ownersForPeriod.PayoutAddress, ownersForPeriod.Reward); retErr != nil {
				return
			}
		}
	}

	if retErr = saveTxHashWithStatus(ctx, sql_tx, txHash, types.TransactionPending); retErr != nil {
		return
	}

	if retErr = sql_tx.Commit(); retErr != nil {
		retErr = fmt.Errorf("failed to commit transaction: %s", retErr)
		return
	}

	return nil
}

func (sdb *sqlDB) UpdateTransactionsStatus(ctx context.Context, txHashesToMarkCompleted []string, status string) error {
	sql_tx, err := sdb.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %s", err)
	}

	if retErr := updateTxHasheshWithStatus(ctx, sql_tx, txHashesToMarkCompleted, status); retErr != nil {
		return fmt.Errorf("failed to commit transaction: %s", retErr)
	}

	if retErr := sql_tx.Commit(); retErr != nil {
		return fmt.Errorf("failed to commit transaction: %s", retErr)
	}

	return nil
}

type sqlDB struct {
	db *sqlx.DB
}
