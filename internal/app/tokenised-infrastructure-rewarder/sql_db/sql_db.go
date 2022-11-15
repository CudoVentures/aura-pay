package sql_db

import (
	"context"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewSqlDB(db *sqlx.DB) *SqlDB {
	return &SqlDB{db: db}
}

func (sdb *SqlDB) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, txHash, farmId string, farmSubAccountName string) (retErr error) {
	sqlTx, err := sdb.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %s", err)
	}

	defer func() {
		if retErr != nil {
			if err := sqlTx.Rollback(); err != nil {
				log.Error().Msgf("failed to rollback: %s, during: %s", err, retErr)
			}
		}
	}()

	for address, amount := range destinationAddressesWithAmount {
		if retErr = saveDestinationAddressesWithAmountHistory(ctx, sqlTx, address, amount, txHash, farmId); retErr != nil {
			return
		}
	}

	for _, nftStatistic := range statistics {
		if retErr = saveNFTInformationHistory(ctx, sqlTx, nftStatistic.DenomId, nftStatistic.TokenId,
			nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.Reward, txHash,
			nftStatistic.MaintenanceFee, nftStatistic.CUDOPartOfMaintenanceFee); retErr != nil {
			return
		}
		for _, ownersForPeriod := range nftStatistic.NFTOwnersForPeriod {
			if retErr = saveNFTOwnersForPeriodHistory(ctx, sqlTx, nftStatistic.DenomId, nftStatistic.TokenId,
				ownersForPeriod.TimeOwnedFrom, ownersForPeriod.TimeOwnedTo, ownersForPeriod.TotalTimeOwned,
				ownersForPeriod.PercentOfTimeOwned, ownersForPeriod.Owner, ownersForPeriod.PayoutAddress, ownersForPeriod.Reward); retErr != nil {
				return
			}
		}
	}

	if retErr = sdb.SaveTxHashWithStatus(ctx, sqlTx, txHash, types.TransactionPending, farmSubAccountName, 0); retErr != nil {
		return
	}

	if retErr = sqlTx.Commit(); retErr != nil {
		retErr = fmt.Errorf("failed to commit transaction: %s", retErr)
		return
	}

	return nil
}

func (sdb *SqlDB) SaveRBFTransactionInformation(ctx context.Context,
	oldTxHash string,
	oldTxStatus string,
	newRBFTxHash string,
	newRBFTXStatus string,
	farmSubAccountName string,
	retryCount int) error {

	sqlTx, err := sdb.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %s", err)
	}

	defer func() {
		if err := sqlTx.Rollback(); err != nil {
			log.Error().Msgf("failed to rollback: %s, during: %s", err, err)
		}
	}()

	// update old tx status
	if retErr := sdb.UpdateTransactionsStatus(ctx, sqlTx, []string{oldTxHash}, oldTxStatus); retErr != nil {
		return fmt.Errorf("failed to updateTxHashesWithStatus: %s", retErr)
	}

	// link replaced transaction with the tx that replaced it
	if retErr := sdb.SaveRBFTransactionHistory(ctx, sqlTx, oldTxHash, newRBFTxHash, farmSubAccountName); retErr != nil {
		return fmt.Errorf("failed to saveRBFTransactionHistory: %s", retErr)
	}

	// save the new tx with status, new timestamp, and retryCount of old one + 1
	if retErr := sdb.SaveTxHashWithStatus(ctx, sqlTx, newRBFTxHash, newRBFTXStatus, farmSubAccountName, retryCount); retErr != nil {
		return fmt.Errorf("failed to saveTxHashWithStatus: %s", retErr)
	}

	return nil

}

type SqlDB struct {
	db *sqlx.DB
}

func (sdb *SqlDB) UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]int64, farmId int) (retErr error) {
	sqlTx, err := sdb.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %s", err)
	}

	defer func() {
		if retErr != nil {
			if err := sqlTx.Rollback(); err != nil {
				log.Error().Msgf("failed to rollback: %s, during: %s", err, retErr)
			}
		}
	}()

	if retErr := sdb.markUTXOsAsProcessed(ctx, sqlTx, processedTransactions); retErr != nil {
		return fmt.Errorf("failed to commit transaction: %s", retErr)
	}

	for address, amount := range addressesWithThresholdToUpdate {
		if retErr := sdb.updateCurrentAcummulatedAmountForAddress(ctx, sqlTx, address, farmId, amount); retErr != nil {
			return fmt.Errorf("failed to commit transaction: %s", retErr)
		}
		return nil
	}

	return nil
}
