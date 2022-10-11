package main

import (
	"os"

	worker "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// init is invoked before main()
func init() {
	// loads values from .env into the system
	if err := godotenv.Load("../../.env"); err != nil {
		log.Error().Msg("No .env file found")
	}
}

func main() {
	for {
		log.Info().Msg("Application started")
		errorCount := 0
		err := worker.Start()
		if err != nil {
			//todo: unload wallet that has erroed out
			errorCount++
			log.Error().Msgf("Application has encountered an error! Error: %s...Retrying for %s time", err, errorCount) // TODO: https://medium.com/htc-research-engineering-blog/handle-golang-errors-with-stacktrace-1caddf6dab07
		} else {
			log.Info().Msg("Application successfully completed!")
		}

		if errorCount >= 10 {
			log.Error().Msgf("Application has not been able to complete for 10 times in a row..exiting")
			os.Exit(1)
		}
	}

}
