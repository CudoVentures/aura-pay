package tokenised_infrastructure_rewarder

import (
	"context"
	"time"

	_ "github.com/lib/pq"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/sql_db"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func Start(ctx context.Context, config *infrastructure.Config, payService payService, provider provider) {
	log.Info().Msg("Application started")

	retry := func(err error) {
		log.Error().Msgf("retry error: %s", err)

		ticker := time.NewTicker(config.WorkerFailureRetryDelay)
		defer ticker.Stop()

		select {
		case <-ticker.C:
		case <-ctx.Done():
		}
	}

	errorCount := 0

	for ctx.Err() == nil && errorCount < config.WorkerMaxErrorsCount {

		var processingError error

		rpcClient, err := provider.InitBtcRpcClient()
		if err != nil {
			retry(err)
			continue
		}
		defer rpcClient.Shutdown()

		db, err := provider.InitDBConnection()
		if err != nil {
			retry(err)
			continue
		}
		defer db.Close()

		for processingError == nil {
			ticker := time.NewTicker(config.WorkerProcessInterval)
			defer ticker.Stop()

			select {
			case <-ticker.C:
				processingError = payService.ProcessPayment(ctx, rpcClient, sql_db.NewSqlDB(db))
			case <-ctx.Done():
				return
			}
		}

		// TODO: https://medium.com/htc-research-engineering-blog/handle-golang-errors-with-stacktrace-1caddf6dab07

		if processingError != nil {
			errorCount++
			log.Error().Msgf("Application has encountered an error! Error: %s...Retrying for %d time", processingError, errorCount)
		}
	}

	log.Error().Msgf("Application has not been able to complete for %d times in a row..exiting", errorCount)
}

type provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}

type payService interface {
	ProcessPayment(ctx context.Context, btcClient services.BtcClient, storage services.Storage) error
}
