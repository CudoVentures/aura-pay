package infrastructure

import (
	"fmt"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewProvider(config *Config) *Provider {
	return &Provider{config: *config}
}

type Provider struct {
	config Config
}

func (p *Provider) InitBtcRpcClient() (*rpcclient.Client, error) {
	connCfg := &rpcclient.ConnConfig{
		Host:         p.config.BitcoinNodeUrl + ":" + p.config.BitcoinNodePort + "/",
		User:         p.config.BitcoinNodeUserName,
		Pass:         p.config.BitcoinNodePassword,
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default,
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

func (p *Provider) InitDBConnection() (*sqlx.DB, error) {

	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		p.config.DbHost, p.config.DbPort, p.config.DbUser, p.config.DbPassword, p.config.DbName)

	db, err := sqlx.Connect(p.config.DbDriverName, psqlInfo)
	if err != nil {
		return nil, err
	}

	log.Debug().Msgf("Successfull connection to database: host: %s, port: %s, dbName: %s", p.config.DbHost, p.config.DbPort, p.config.DbName)

	return db, nil
}
