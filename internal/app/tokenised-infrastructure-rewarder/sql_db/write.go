package sql_db

import (
	"github.com/jmoiron/sqlx"
)

func SetPayoutTimesForNFT(tx *sqlx.Tx, nftId string, payoutTime int64, payoutAmount float64) {
	tx.MustExec("INSERT INTO nft_payout_times (token_id, time, amount) VALUES ($1, $2, $3)", nftId, payoutTime, payoutAmount)

}
