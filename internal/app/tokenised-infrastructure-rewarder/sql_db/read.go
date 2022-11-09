package sql_db

import (
	"context"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func (sdb *SqlDB) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	payoutTimes := []types.NFTStatistics{}
	if err := sdb.db.SelectContext(ctx, &payoutTimes, selectNFTPayoutHistory, collectionDenomId, nftId); err != nil {
		return nil, err
	}
	return payoutTimes, nil
}

func (sdb *SqlDB) GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error) {
	txHashesWithStatus := []types.TransactionHashWithStatus{}
	if err := sdb.db.SelectContext(ctx, &txHashesWithStatus, selectTxHashStatus, status); err != nil {
		return nil, err
	}
	return txHashesWithStatus, nil
}

func (sdb *SqlDB) GetCurrentAcummulatedAmountForAddress(ctx context.Context, address string, farmId int) (int64, error) {
	result := types.AddressThresholdAmountByFarm{}
	if err := sdb.db.SelectContext(ctx, &result, selectThresholdByAddress, address, farmId); err != nil {
		return 0, err
	}
	return result.Amount, nil

}

const selectNFTPayoutHistory = `SELECT * FROM statistics_nft_payout_history WHERE denom_id=$1 and token_id=$2 ORDER BY payout_period_end ASC`
const selectTxHashStatus = `SELECT * FROM statistics_tx_hash_status WHERE status=$1 ORDER BY time_sent ASC`
const selectThresholdByAddress = `SELECT * FROM threshold_amounts WHERE address=$1 AND farm_id=$2`
