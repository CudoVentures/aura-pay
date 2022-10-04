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
	if err := godotenv.Load("../../../.env"); err != nil {
		log.Error().Msg("No .env file found")
	}
}

func main() {
	log.Info().Msg("Application started")
	err := worker.Start()
	if err != nil {
		log.Error().Msgf("Application has encountered an error! Error: %s", err) // TODO: https://medium.com/htc-research-engineering-blog/handle-golang-errors-with-stacktrace-1caddf6dab07
		os.Exit(1)
	}
	log.Info().Msg("Application successfully exited!")

}
