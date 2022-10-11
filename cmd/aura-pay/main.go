package main

import (
	"context"

	worker "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
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
	}

	config := infrastructure.NewConfig()
	provider := infrastructure.NewProvider(config)
	requestClient := requesters.NewRequester(config)

	var params *chaincfg.Params
	var minConfirmation int

	if config.IsTesting {
		params = &chaincfg.SigNetParams
		minConfirmation = 1
	} else {
		params = &chaincfg.MainNetParams
		minConfirmation = 6
	}

	payService := services.NewServices(config, requestClient, infrastructure.NewHelper(config), params, minConfirmation)

	worker.Start(ctx, config, payService, provider)
}
