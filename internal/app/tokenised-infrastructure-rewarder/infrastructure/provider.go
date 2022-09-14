package infrastructure

import (
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/rs/zerolog/log"
)

func InitBtcRpcClient(config *Config) (*rpcclient.Client, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         config.BitcoinNodeUrl + ":" + config.BitcoinNodePort + "/",
		User:         config.BitcoinNodeUserName,
		Pass:         config.BitcoinNodePassword,
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("rpcClient initiated with host: %s", connCfg.Host)

	return client, err
}
