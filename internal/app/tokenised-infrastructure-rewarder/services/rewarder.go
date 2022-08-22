package services

import (
	"log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// should use the fetched reward from the mining_pool service and pay it to the nft owner
// Farm
// 	[]Collection
// 		[]NFT

// For every farm
// 	For every collection
// 	      Poll Foundry API if reward has been payed for the collection
// 		    For every nft in the collection
// 				check that all nfts and their hash rate is equal to the 				total mining power of the collection
// 				transfer % of the reward to the payout address using  BTC full node

func ProcessPaymentForFarms(farms []types.Farm) {
	for _, farm := range farms {
		for _, collection := range farm.Collections {
			for _, nft := range collection.Nfts {
				
			}
		}
	}
}

func PayRewards(sendToAddress string, amount float64) (*chainhash.Hash, error) {
	rpcClient, err := infrastructure.InitBtcRpcClient()
	defer rpcClient.Shutdown()
	rpcClient.wallet

	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	addr, err := btcutil.DecodeAddress(sendToAddress, &chaincfg.SigNetParams)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	amountInSatoshi, err := btcutil.NewAmount(amount)
	txHash, err := rpcClient.SendToAddress(addr, amountInSatoshi, subtractFee=true)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return txHash, nil
}
