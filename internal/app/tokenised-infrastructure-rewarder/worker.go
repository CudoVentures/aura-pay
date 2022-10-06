package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
)

func Start() error {

	// TODO: add a while(true) to have the app running all the time
	// add new logic for maintenance fee calculation
	// add logic that handles the start of the month for the reward
	config := infrastructure.NewConfig()
	provider := infrastructure.NewProvider(config)
	requestClient := requesters.NewRequester(*config)
	payService := services.NewServices(requestClient, provider)
	err := payService.ProcessPayment(config)
	return err
}
