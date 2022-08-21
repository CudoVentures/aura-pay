package services

import (
	"log"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
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

func PayRewards(sendToAddress string, amount float64) (*chainhash.Hash, error) {
	rpcClient, err := initBtcRpcClient()
	defer rpcClient.Shutdown()
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
	txHash, err := rpcClient.SendToAddress(addr, amountInSatoshi)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return txHash, nil
}

func initBtcRpcClient() (*rpcclient.Client, error) {
	config := infrastructure.NewConfig()
	connCfg := &rpcclient.ConnConfig{
		Host:         config.BitcoinNodeUrl + ":" + config.BitcoinNodePort + "/",
		User:         config.BitcoinNodeUserName,
		Pass:         config.BitcoinNodePassword,
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	blockCount, err := client.GetBlockCount()
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	log.Printf("Block count: %d", blockCount)

	return client, err
}
