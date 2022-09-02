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
			testRNG := 0
			for _, nft := range collection.Nfts {
				// logging + track payment progress in DB
				nftPayoutAddress, err := requesters.GetPayoutAddressFromNode(nft.Owner, Network, nft.Id, collection.Denom.Id)
				if err != nil {
					return err
				}
				payoutAmount, err := calculatePayoutAmount(nft.DataJson.HashRate, nft.DataJson.TotalHashRate)
				if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
					destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount

				} else {
					destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
				}
				testRNG += 1
			}
			if len(destinationAddressesWithAmount) == 0 {
				return fmt.Errorf("No addresses found to pay for Farm %q and Collection %q", farm.Name, collection.Denom.Id)
			}
			payRewards("bf4961e4259c9d9c7bdf4862fdeeb0337d06479737c2c63e4af360913b11277f", uint32(1), farm.BTCWallet, destinationAddressesWithAmount)
		}
	}
	return nil
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

// also handle special edge case where address is changed and you have to pay him only for the time he owned it
func calculatePayoutAmount(nftHashRate string, totalHashRate string) (btcutil.Amount, error) {
	amountInSatoshis, err := btcutil.NewAmount(0.0001)
	if err != nil {
		return -1, err
	}
	return amountInSatoshis, nil
}

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
