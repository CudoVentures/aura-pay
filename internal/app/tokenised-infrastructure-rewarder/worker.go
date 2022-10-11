package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
)

func Start() error {

	config := infrastructure.NewConfig()
	helper := infrastructure.NewHelper(config)
	provider := infrastructure.NewProvider(config)
	requestClient := requesters.NewRequester(*config)
	payService := services.NewServices(requestClient, provider, helper)
	err := payService.ProcessPayment(config)
	return err
}
