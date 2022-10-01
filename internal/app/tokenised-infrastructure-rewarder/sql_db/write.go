package sql_db

import (
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
)

func SetPayoutTimesForNFT(tx *sqlx.Tx, collectionDenomId string, nftId string, payoutTime int64, payoutAmount float64) {
	tx.MustExec("INSERT INTO nft_payout_times (denom_id, token_id, payout_time_at, amount, \"createdAt\", \"updatedAt\") VALUES ($1, $2, $3, $4, $5, $6)", collectionDenomId, nftId, payoutTime, payoutAmount, time.Now().UTC(), time.Now().UTC())
}

func SaveDestionAddressesWithAmountHistory(tx *sqlx.Tx, address string, amount btcutil.Amount, txHash string, farmId string) {
	tx.MustExec("INSERT INTO statistics_destination_addresses_with_amount (address, amount, tx_hash, farm_id, time) VALUES ($1, $2, $3, $4, $5)", address, amount, txHash, farmId, time.Now().Unix())
}

func SaveNftInformationHistory(tx *sqlx.Tx, collectionDenomId string, tokenId string, payoutPeriodStart int64, payoutPeriodEnd int64, reward btcutil.Amount, txHash string) {
	tx.MustExec("INSERT INTO statistics_nft_payout_history (denom_id, token_id, payout_period_start, payout_period_end, reward, tx_hash) VALUES ($1, $2, $3, $4, $5, $6)", tokenId, payoutPeriodStart, payoutPeriodEnd, reward, txHash)

}

func SaveNFTOwnersForPeriodHistory(tx *sqlx.Tx, timedOwnedFrom int64, timedOwnedTo int64, totalTimeOwned int64, percentOfTimeOwned float64, owner string, payoutAddress string, reward btcutil.Amount) {
	tx.MustExec("INSERT INTO statistics_nft_owners_payout_history (time_owned_from, time_owned_to, total_time_owned, percent_of_time_owned ,owner, payout_address, reward ) VALUES ($1, $2, $3, $4, $5, $6, S7)",
		timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned, owner, payoutAddress, reward)
}
