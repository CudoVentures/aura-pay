package tokenised_infrastructure_rewarder

import (
	"context"
	"fmt"
	"sync"
	"time"

	_ "github.com/lib/pq"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	services "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/sql_db"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func Start(ctx context.Context, config *infrastructure.Config, s service, provider provider, mutex *sync.Mutex, interval time.Duration) {
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

	for ctx.Err() == nil {
		func() {
			var processingError error

			rpcClient, err := provider.InitBtcRpcClient()
			if err != nil {
				retry(err)
				return
			}
			defer rpcClient.Shutdown()

			db, err := provider.InitDBConnection()
			if err != nil {
				retry(err)
				return
			}
			defer db.Close()

			for processingError == nil {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()

				select {
				case <-ticker.C:
					mutex.Lock()
					processingError = s.Execute(ctx, rpcClient, sql_db.NewSqlDB(db))
					mutex.Unlock()
				case <-ctx.Done():
					return
				}
			}

			// TODO: https://medium.com/htc-research-engineering-blog/handle-golang-errors-with-stacktrace-1caddf6dab07

			if processingError != nil {
				errorCount++
				errorEncountered(config, processingError, errorCount)
				if errorCount >= config.ServiceMaxErrorCount {
					maxErrorCountReached(config, err)
					return
				}
			}
		}()
	}
}

var mSendMail = sendMail

func maxErrorCountReached(config *infrastructure.Config, err error) {
	message := fmt.Sprintf("Application has exceeded the ServiceMaxErrorCount: {%d} and needs manual intervention!\n Error: {%s}", config.ServiceMaxErrorCount, err)
	log.Error().Msg(message)
	mSendMail(config, message)
}

func errorEncountered(config *infrastructure.Config, processingError error, errorCount int) {
	message := fmt.Sprintf("Application has encountered an error! Error: %s...Retrying for %d time", processingError, errorCount)
	log.Error().Msg(message)
	mSendMail(config, message)
}

func sendMail(config *infrastructure.Config, message string) {
	h := infrastructure.NewHelper(config)
	err := h.SendMail(message)
	if err != nil {
		panic(err)
	}
}

type provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}

type service interface {
	Execute(ctx context.Context, btcClient services.BtcClient, storage services.Storage) error
}
