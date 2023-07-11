package main

import (
	"context"
	"sync"

	worker "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	services "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func main() {
	runService(context.Background())
}

func runService(ctx context.Context) {
	if err := godotenv.Load(".env"); err != nil {
		log.Error().Msgf("No .env file found: %s", err)
		return
	}

	ctx, ctxCancel := context.WithCancel(ctx)

	config := infrastructure.NewConfig()
	provider := infrastructure.NewProvider(config)
	requestClient := requesters.NewRequester(config)

	var btcNetworkParams types.BtcNetworkParams
	mutex := sync.Mutex{}
	if config.IsTesting {
		btcNetworkParams.ChainParams = &chaincfg.SigNetParams
		btcNetworkParams.MinConfirmations = 1
	} else {
		btcNetworkParams.ChainParams = &chaincfg.MainNetParams
		btcNetworkParams.MinConfirmations = 6
	}

	retryService := services.NewRetryService(config, requestClient, infrastructure.NewHelper(config), &btcNetworkParams)

	go worker.Start(ctx, ctxCancel, config, retryService, provider, &mutex, config.WorkerProcessIntervalPayment)

	payService := services.NewPayService(config, requestClient, infrastructure.NewHelper(config), &btcNetworkParams)

	worker.Start(ctx, ctxCancel, config, payService, provider, &mutex, config.WorkerProcessIntervalRetry)
}
