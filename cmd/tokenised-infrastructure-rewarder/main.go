package main

import (
	worker "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

// init is invoked before main()
func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Error().Msg("No .env file found")
	}
}

func main() {
	log.Info().Msg("Application started")
	worker.Start()
}
