package sql_db

import (
	"context"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func (sdb *sqlDB) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	payoutTimes := []types.NFTStatistics{}
	if err := sdb.db.SelectContext(ctx, &payoutTimes, selectNFTPayoutHistory, collectionDenomId, nftId); err != nil {
		return nil, err
	}
	return payoutTimes, nil
}

func (sdb *sqlDB) GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error) {
	txHashesWithStatus := []types.TransactionHashWithStatus{}
	if err := sdb.db.SelectContext(ctx, &txHashesWithStatus, selectTxHashStatus, status); err != nil {
		return nil, err
	}
	return txHashesWithStatus, nil
}

const selectNFTPayoutHistory = `SELECT * FROM statistics_nft_payout_history WHERE denom_id=$1 and token_id=$2 ORDER BY payout_period_end ASC`
const selectTxHashStatus = `SELECT * FROM statistics_tx_hash_status WHERE status=$1 ORDER BY time_sent ASC`
