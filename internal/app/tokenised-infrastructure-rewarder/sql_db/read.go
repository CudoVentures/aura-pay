package sql_db

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/jmoiron/sqlx"
)

func GetPayoutTimesForNFT(db *sqlx.DB, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	payoutTimes := []types.NFTStatistics{}
	err := db.Select(&payoutTimes, "SELECT * FROM statistics_nft_payout_history WHERE denom_id=$1 and token_id=$2 ORDER BY payout_period_end ASC", collectionDenomId, nftId)
	if err != nil {
		return nil, err
	}
	return payoutTimes, nil
}
