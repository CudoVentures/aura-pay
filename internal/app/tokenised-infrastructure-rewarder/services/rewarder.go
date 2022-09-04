package services

import (
	"errors"
	"fmt"
	"log"

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

func ProcessPaymentForFarms(farms []types.Farm) error {
	// also check if the funds have come
	for _, farm := range farms {
		for _, collection := range farm.Collections {
			// Poll Foundry API if reward has been payed for the collection
			// if not return
			destinationAddressesWithAmount := make(map[string]btcutil.Amount)
			totalRewardForCollection, err := btcutil.NewAmount(0.0001) // where do we take this - from incoming tx or does foundry api give us this info?
			for _, nft := range collection.Nfts {
				// logging + track payment progress in DB
				if err != nil {
					return err
				}
				nftPayoutAmount, err := calculatePayoutAmount(nft.DataJson.HashRateOwned, collection.TotalCollectionHashRate, nft.DataJson.TotalCollectionHashRateWhenMinted, totalRewardForCollection)
				if err != nil {
					return err
				}
				allNftOwnersForTimePeriodWithRewardPercent, err := getNftOwnersForTimePeriodWithRewardPercent(nft.Id, collection.Denom.Id, 0, 0)
				err = distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, nftPayoutAmount, destinationAddressesWithAmount)

			}
			if len(destinationAddressesWithAmount) == 0 {
				return fmt.Errorf("No addresses found to pay for Farm %q and Collection %q", farm.Name, collection.Denom.Id)
			}
			payRewards("bf4961e4259c9d9c7bdf4862fdeeb0337d06479737c2c63e4af360913b11277f", uint32(1), farm.BTCWallet, destinationAddressesWithAmount)
		}
	}
	return nil
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount) error {

	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)
		if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
			// log to statistics here if we are doing accumulation send for an nft
			destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount

		} else {
			destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
		}
	}

	return nil
}

func getNftOwnersForTimePeriodWithRewardPercent(nftId string, collectionDenomId string, periodStart int64, periodEnd int64) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart

	nftTransferHistory, err := requesters.GetNftTransferHistory(collectionDenomId, nftId, periodStart)
	if err != nil {
		return nil, err
	}

	for _, transferHistoryElement := range nftTransferHistory {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			timeOwned := transferHistoryElement.Timestamp - periodStart
			percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
			nftPayoutAddress, err := requesters.GetPayoutAddressFromNode(transferHistoryElement.From, Network, nftId, collectionDenomId)
			if err != nil {
				return nil, err
			}
			if _, ok := ownersWithPercentOwnedTime[nftPayoutAddress]; ok {
				ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

			} else {
				ownersWithPercentOwnedTime[nftPayoutAddress] = percentOfTimeOwned
			}
		}
	}

	return ownersWithPercentOwnedTime, nil

	// ownedFromTimestamp := 1234 // TODO: Once events are fetchable from the chain, use them and calculate it from there
	// ownedToTimestamp := 5678
	// handle case where the user has been an owner for some period and then the same nft changed hands and has been owned by someone else for the remainder of the period
	// payoutAmount represents 100% of the reward for this nft for the given time period
	// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
	// so a method that returns each nft owner for the time period with the time he owned it as percent
	// use this percent to calculate how much each one should get from the total reward

	// parse the transfer events and find each owner that fits between periodStart and periodEnd
	// for each one of them find percent of the time (periodStart, periodEnd) that he has hold it and return it alongside his address
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

func calculatePayoutAmount(nftHashRate int64, totalCollectionHashRate int64, nftHashRateAtTimeOfMinting int64, totalReward btcutil.Amount) (btcutil.Amount, error) {

	var payoutRewardPercent float64
	// handle case where the collection hash power has decreased
	if totalCollectionHashRate < nftHashRateAtTimeOfMinting {
		// take nft.HashPower as percent of nft.TotalHashPower
		payoutRewardPercent = float64(nftHashRate) / float64(nftHashRateAtTimeOfMinting) * 100
	} else {
		// totalHashRate is the same or increased - then we do nothing as the percent is the same or has proportionally decreased in terms of total hash power
		payoutRewardPercent = float64(nftHashRate) / float64(totalCollectionHashRate) * 100
	}

	// result := float64(totalReward) * payoutRewardPercent / 100
	// result := payoutRewardPercent / 100 * float64(totalReward)
	// returns float64 of percent of totalReward
	// possible problems: what is the foundry tx denomination - in satoshis?
	// think about if we can lose precision here as float64 is  53-bit precision..maybe use math/big type Float with more precision
	// or this: https://github.com/shopspring/decimal
	result := totalReward.MulF64(payoutRewardPercent / 100)

	return result, nil
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
