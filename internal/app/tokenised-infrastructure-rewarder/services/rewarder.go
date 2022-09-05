package services

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
)

const Network = "BTC"

//TODO:
// Integrate with foundry
// Save data in the sql for statistics
// Test the new payout code

func ProcessPaymentForFarms(farms []types.Farm) error {
	// also check if the funds have come
	for _, farm := range farms {
		rpcClient, err := infrastructure.InitBtcRpcClient()
		if err != nil {
			return err
		}
		defer rpcClient.Shutdown()

		totalRewardForFarm, err := rpcClient.GetBalance(farm.SubAccountName)
		if err != nil {
			return err
		}
		if totalRewardForFarm == 0 {
			return fmt.Errorf("Farm with name %s balance is 0..skipping this farm", farm.SubAccountName)
		}

		totalHashPowerForFarm, err := requesters.GetFarmTotalHashPowerFromPoolToday(farm.SubAccountName, time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
		if err != nil {
			return err
		}
		for _, collection := range farm.Collections {
			destinationAddressesWithAmount := make(map[string]btcutil.Amount)
			totalRewardForCollection, err := calculatePayout(collection.HashRate, totalHashPowerForFarm, collection.HashRateAtCreation, totalRewardForFarm)
			if err != nil {
				return err
			}
			for _, nft := range collection.Nfts {
				// logging + track payment progress in DB
				nftPayoutAmount, err := calculatePayout(nft.DataJson.HashRateOwned, collection.HashRate, nft.DataJson.TotalCollectionHashRateWhenMinted, totalRewardForCollection)
				if err != nil {
					return err
				}
				allNftOwnersForTimePeriodWithRewardPercent, err := getNftOwnersForTimePeriodWithRewardPercent(nft.Id, collection.Denom.Id, 0, 0)
				distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, nftPayoutAmount, destinationAddressesWithAmount)
			}
			if len(destinationAddressesWithAmount) == 0 {
				return fmt.Errorf("No addresses found to pay for Farm %q and Collection %q", farm.SubAccountName, collection.Denom.Id)
			}
			// how and where to fetch tx input id/vout for payment? Get the first available that matches the current balance?
			payRewards("bf4961e4259c9d9c7bdf4862fdeeb0337d06479737c2c63e4af360913b11277f", uint32(1), farm.BTCWallet, destinationAddressesWithAmount)
		}
	}
	return nil
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)
		if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
			// log to statistics here if we are doing accumulation send for an nft
			destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount

		} else {
			destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
		}
	}
}

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func getNftOwnersForTimePeriodWithRewardPercent(nftId string, collectionDenomId string, periodStart int64, periodEnd int64) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart
	var transferHistoryForTimePeriod []types.NftTransferHistoryElement

	nftTransferHistory, err := requesters.GetNftTransferHistory(collectionDenomId, nftId, periodStart)
	if err != nil {
		return nil, err
	}

	for _, transferHistoryElement := range nftTransferHistory {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	// sort in ascending order by timestamp
	sort.Slice(transferHistoryForTimePeriod, func(i, j int) bool {
		return transferHistoryForTimePeriod[i].Timestamp < transferHistoryForTimePeriod[j].Timestamp
	})

	for i := 0; i < len(transferHistoryForTimePeriod); i++ {
		var timeOwned int64

		if i == 0 {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - periodStart
		} else {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - transferHistoryForTimePeriod[i-1].Timestamp
		}

		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
		nftPayoutAddress, err := requesters.GetPayoutAddressFromNode(transferHistoryForTimePeriod[i].From, Network, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}

		if _, ok := ownersWithPercentOwnedTime[nftPayoutAddress]; ok { // if the nft has been bought, sold and bought again by the same owner in the same period - accumulate
			ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		} else {
			ownersWithPercentOwnedTime[nftPayoutAddress] = percentOfTimeOwned
		}
	}

	return ownersWithPercentOwnedTime, nil
}

func payRewards(inputTxId string, inputTxVout uint32, walletName string, destinationAddressesWithAmount map[string]btcutil.Amount) (*chainhash.Hash, error) {
	rpcClient, err := infrastructure.InitBtcRpcClient()
	if err != nil {
		return nil, err
	}
	defer rpcClient.Shutdown()

	// todo: add as params and fetch it from pool
	var outputVouts []int
	for i := 0; i < len(destinationAddressesWithAmount); i++ {
		outputVouts = append(outputVouts, i)
	}

	txInput := btcjson.TransactionInput{Txid: inputTxId, Vout: inputTxVout}
	inputs := []btcjson.TransactionInput{txInput}
	isWitness := false
	transformedAddressesWithAmount, err := transformAddressesWithAmount(destinationAddressesWithAmount)
	if err != nil {
		return nil, err
	}

	rawTx, err := rpcClient.CreateRawTransaction(inputs, transformedAddressesWithAmount, nil)
	if err != nil {
		return nil, err
	}

	res, err := rpcClient.FundRawTransaction(rawTx, btcjson.FundRawTransactionOpts{SubtractFeeFromOutputs: outputVouts}, &isWitness)
	if err != nil {
		return nil, err
	}

	signedTx, isSigned, err := rpcClient.SignRawTransactionWithWallet(res.Transaction)
	if err != nil || isSigned == false {
		return nil, err
	}

	txHash, err := rpcClient.SendRawTransaction(signedTx, false)
	if err != nil {
		return nil, err
	}

	return txHash, nil
}

func transformAddressesWithAmount(destinationAddressesWithAmount map[string]btcutil.Amount) (map[btcutil.Address]btcutil.Amount, error) {
	result := make(map[btcutil.Address]btcutil.Amount)

	for address, amount := range destinationAddressesWithAmount {
		addr, err := btcutil.DecodeAddress(address, &chaincfg.SigNetParams)
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		result[addr] = amount
	}

	return result, nil
}

func findMatchingUTXO(rpcClient *rpcclient.Client, txId string, vout uint32) (btcjson.ListUnspentResult, error) {
	unspentTxs, err := rpcClient.ListUnspent()
	if err != nil {
		return btcjson.ListUnspentResult{}, err
	}
	var matchedUTXO btcjson.ListUnspentResult
	for _, unspentTx := range unspentTxs {
		if unspentTx.TxID == txId && unspentTx.Vout == vout {
			matchedUTXO = unspentTx
		} else {
			err = errors.New("No matching UTXO found!")
			return btcjson.ListUnspentResult{}, err
		}
	}
	return matchedUTXO, nil
}

func calculatePayout(hashRate int64, totalHashRate int64, staticHashRate int64, totalReward btcutil.Amount) (btcutil.Amount, error) {

	var payoutRewardPercent float64
	// handle case where the collection hash power has decreased
	if totalHashRate < staticHashRate {
		// take hashPower as percent of staticHashRate
		payoutRewardPercent = float64(hashRate) / float64(staticHashRate) * 100
	} else {
		// totalHashRate is the same or increased - then we do nothing as the percent is the same or has proportionally decreased in terms of total hash power
		payoutRewardPercent = float64(hashRate) / float64(totalHashRate) * 100
	}

	result := totalReward.MulF64(payoutRewardPercent / 100)

	return result, nil

	// result := float64(totalReward) * payoutRewardPercent / 100
	// result := payoutRewardPercent / 100 * float64(totalReward)
	// returns float64 of percent of totalReward
	// possible problems: what is the foundry tx denomination - in satoshis?
	// think about if we can lose precision here as float64 is  53-bit precision..maybe use math/big type Float with more precision
	// or this: https://github.com/shopspring/decimal
}

// func calculatePayoutAmountTest(nftHashRate string, totalHashRate string, ownedFromTimestamp string, ownedToTimestamp string) (btcutil.Amount, error) {
// 	amountInSatoshis, err := btcutil.NewAmount(0.0001)
// 	if err != nil {
// 		return -1, err
// 	}
// 	return amountInSatoshis, nil
// }

// todo remove once testing is done
// func getPayoutAddressesFromChain(ownerAddress string, denomId string, tokenId string, test int) (string, error) {
// rpc call to cudos node for address once its merged
// http://127.0.0.1:1317/CudoVentures/cudos-node/addressbook/address/cudos1dgv5mmf4r0w3rgxxd3sy5mw3gnnxmgxmuvnqxw/BTC/1@testdenom
// result:
// {
// 	"address": {
// 	  "network": "BTC",
// 	  "label": "1@testdenom",
// 	  "value": "myval",
// 	  "creator": "cudos1dgv5mmf4r0w3rgxxd3sy5mw3gnnxmgxmuvnqxw"
// 	}
//   }
// var fakedAddress string
// if test == 0 || test == 1 {
// 	fakedAddress = "tb1qntsxw6tlkczpueqtpmpza9kutajarctn6aee0l"

// } else {
// 	fakedAddress = "tb1qqpacwhsdcr4x6vt9hj228ha43kanpch2n74y5c"
// }
// // addr, err := btcutil.DecodeAddress(fakedAddress, &chaincfg.SigNetParams)
// // if err != nil {
// // 	log.Fatal(err)
// // 	return nil, err
// // }
// return fakedAddress, nil

// }
