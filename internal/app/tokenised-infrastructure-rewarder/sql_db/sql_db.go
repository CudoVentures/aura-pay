package sql_db

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewSqlDB(db *sqlx.DB) *SqlDB {
	return &SqlDB{db}
}

func (sdb *SqlDB) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash, farmId, farmSubAccountName string) (retErr error) {

	return sdb.ExecuteTx(ctx, func(tx *DbTx) error {

		for address, amountInfo := range destinationAddressesWithAmount {
			if err := tx.saveDestinationAddressesWithAmountHistory(ctx, address, amountInfo, txHash, farmId); err != nil {
				return err
			}
		}

		for _, nftStatistic := range statistics {
			var nftPayoutHistoryId int
			var err error
			if nftPayoutHistoryId, err = tx.saveNFTInformationHistory(ctx, nftStatistic.DenomId, nftStatistic.TokenId,
				nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.Reward, txHash,
				nftStatistic.MaintenanceFee, nftStatistic.CUDOPartOfMaintenanceFee); err != nil {
				return err
			}

			for _, ownerForPeriod := range nftStatistic.NFTOwnersForPeriod {
				isSent := fundsHaveBeenSent(destinationAddressesWithAmount, ownerForPeriod)
				if err := tx.saveNFTOwnersForPeriodHistory(ctx,
					ownerForPeriod.TimeOwnedFrom, ownerForPeriod.TimeOwnedTo, ownerForPeriod.TotalTimeOwned,
					ownerForPeriod.PercentOfTimeOwned, ownerForPeriod.Owner, ownerForPeriod.PayoutAddress, ownerForPeriod.Reward, nftPayoutHistoryId, isSent); err != nil {
					return err
				}
			}
		}

		if txHash != "" {
			if err := saveTxHashWithStatus(ctx, tx, txHash, types.TransactionPending, farmSubAccountName, 0); err != nil {
				return err
			}
		}

		return nil
	})
}

func fundsHaveBeenSent(destinationAddressesWithAmount map[string]types.AmountInfo, ownerInfo types.NFTOwnerInformation) bool {
	for address, info := range destinationAddressesWithAmount {
		if address == ownerInfo.PayoutAddress {
			if info.ThresholdReached {
				return true
			}
		}
	}
	return false
}

func (sdb *SqlDB) SaveRBFTransactionInformation(ctx context.Context, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName string, retryCount int) error {

	return sdb.ExecuteTx(ctx, func(tx *DbTx) error {
		// update old tx status
		if retErr := updateTransactionsStatus(ctx, tx, []string{oldTxHash}, oldTxStatus); retErr != nil {
			return fmt.Errorf("failed to updateTxHashesWithStatus: %s", retErr)
		}

		// link replaced transaction with the tx that replaced it
		if retErr := tx.saveRBFTransactionHistory(ctx, oldTxHash, newRBFTxHash, farmSubAccountName); retErr != nil {
			return fmt.Errorf("failed to saveRBFTransactionHistory: %s", retErr)
		}

		// save the new tx with status, new timestamp, and retryCount of old one + 1
		if retErr := saveTxHashWithStatus(ctx, tx, newRBFTxHash, newRBFTXStatus, farmSubAccountName, retryCount); retErr != nil {
			return fmt.Errorf("failed to saveTxHashWithStatus: %s", retErr)
		}

		return nil
	})
}

func (sdb *SqlDB) UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]btcutil.Amount, farmId int) (retErr error) {

	return sdb.ExecuteTx(ctx, func(tx *DbTx) error {
		if retErr = tx.markUTXOsAsProcessed(ctx, processedTransactions); retErr != nil {
			return fmt.Errorf("failed to commit transaction: %s", retErr)
		}

		for address, amount := range addressesWithThresholdToUpdate {
			if retErr = tx.updateCurrentAcummulatedAmountForAddress(ctx, address, farmId, amount); retErr != nil {
				return fmt.Errorf("failed to commit transaction: %s", retErr)
			}
		}

		return nil
	})
}

func (sdb *SqlDB) ExecuteTx(ctx context.Context, callback func(*DbTx) error) (retErr error) {
	tx, err := sdb.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			if err := tx.Rollback(); err != nil {
				log.Error().Err(fmt.Errorf("error while executing tx: %s\nerror while making rollback: %s", retErr, err)).Send()
			}
		}
	}()

	if retErr = callback(&DbTx{tx}); retErr != nil {
		return
	}

	if retErr = tx.Commit(); retErr != nil {
		return
	}

	return nil
}

type SqlDB struct {
	*sqlx.DB
}
type DbTx struct {
	*sqlx.Tx
}
