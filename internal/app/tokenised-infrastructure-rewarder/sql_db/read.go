package sql_db

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/jmoiron/sqlx"
)

func GetPayoutTimesForNFT(db *sqlx.DB, nftId string) ([]types.NFTPayoutTime, error) {
	payoutTimes := []types.NFTPayoutTime{}
	err := db.Select(&payoutTimes, "SELECT * FROM nft_payout_times WHERE token_id=$1 ORDER BY payout_time_at ASC", nftId)
	if err != nil {
		return nil, err
	}
	return payoutTimes, nil
}
