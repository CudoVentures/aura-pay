package services

import (
	"fmt"
	"log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

func ProcessPaymentForFarms(farms []types.Farm) error {
	// also check if the funds have come
	for _, farm := range farms {
		for _, collection := range farm.Collections {
			// Poll Foundry API if reward has been payed for the collection
			// if not return
			destinationAddressesWithAmount := make(map[btcutil.Address]btcutil.Amount)
			testRNG := 0
			for _, nft := range collection.Nfts {
				// logging + track payment progress in DB
				nftPayoutAddress, err := getPayoutAddressesFromChain(nft.Owner, collection.Denom.Id, nft.Id, testRNG)
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
			payRewards(farm.BTCWallet, destinationAddressesWithAmount)
		}
	}
	return nil
}

func payRewards(walletName string, destinationAddressesWithAmount map[btcutil.Address]btcutil.Amount) (*chainhash.Hash, error) {
	rpcClient, err := infrastructure.InitBtcRpcClient()
	if err != nil {
		return nil, err
	}
	defer rpcClient.Shutdown()

	txHash, err := rpcClient.SendMany(walletName, destinationAddressesWithAmount)

	return txHash, nil
}

// also handle special edge case where address is changed and you have to pay him only for the time he owned it
func calculatePayoutAmount(nftHashRate string, totalHashRate string) (btcutil.Amount, error) {
	amountInSatoshi, err := btcutil.NewAmount(0.0001)
	if err != nil {
		return -1, err
	}
	return amountInSatoshi, nil
}

func getPayoutAddressesFromChain(ownerAddress string, denomId string, tokenId string, test int) (btcutil.Address, error) {
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
	var fakedAddress string
	if test == 0 || test == 1 {
		fakedAddress = "tb1qntsxw6tlkczpueqtpmpza9kutajarctn6aee0l"

	} else {
		fakedAddress = "tb1qqpacwhsdcr4x6vt9hj228ha43kanpch2n74y5c"
	}
	addr, err := btcutil.DecodeAddress(fakedAddress, &chaincfg.SigNetParams)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return addr, nil

}
