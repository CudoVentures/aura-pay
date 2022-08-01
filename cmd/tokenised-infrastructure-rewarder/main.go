package main

import (
	"fmt"
	worker "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
	"github.com/joho/godotenv"
	"log"
)

// init is invoked before main()
func init() {
	// loads values from .env into the system
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}
}

func main() {
	fmt.Println("Application started")
	worker.Start()
}
