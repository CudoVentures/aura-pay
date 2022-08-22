package infrastructure

import (
	"log"

	"github.com/btcsuite/btcd/rpcclient"
)

func InitBtcRpcClient() (*rpcclient.Client, error) {
	config := NewConfig()
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
