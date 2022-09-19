package sql_db

import (
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"
)

func SetPayoutTimesForNFT(tx *sqlx.Tx, nftId string, payoutTime int64, payoutAmount float64) {
	tx.MustExec("INSERT INTO nft_payout_times (token_id, time, amount) VALUES ($1, $2, $3)", nftId, payoutTime, payoutAmount)

}

func SaveDestionAddressesWithAmountHistory(tx *sqlx.Tx, address string, amount btcutil.Amount, txHash string, farmId string) {
	tx.MustExec("INSERT INTO statistics_destination_addresses_with_amount (address, amount, txHash, farmId, time) VALUES ($1, $2, $3, $4, $5)", address, amount, txHash, farmId, time.Now().Unix())
}

func SaveNftInformationHistory(tx *sqlx.Tx, tokenId string, payoutPeriodStart int64, payoutPeriodEnd int64, reward btcutil.Amount, txHash string) {
	tx.MustExec("INSERT INTO statistics_destination_addresses_with_amount (tokenId, payoutPeriodStart, payoutPeriodEnd, reward, txHash) VALUES ($1, $2, $3, $4, $5)", tokenId, payoutPeriodStart, payoutPeriodEnd, reward, txHash)

}

func SaveNFTOwnersForPeriodHistory(tx *sqlx.Tx, timedOwnedFrom int64, timedOwnedTo int64, totalTimeOwned int64, percentOfTimeOwned float64, owner string, payoutAddress string, reward btcutil.Amount) {
	tx.MustExec("INSERT INTO statistics_destination_addresses_with_amount (timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned,owner, payoutAddress, reward ) VALUES ($1, $2, $3, $4, $5, $6, S7)",
		timedOwnedFrom, timedOwnedTo, totalTimeOwned, percentOfTimeOwned, owner, payoutAddress, reward)
}
