package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/rs/zerolog/log"
)

func Start() error {

	// TODO:
	// Implement methods + expose methods in the node - done
	// Statistics
	// Test config replacement for foundry
	// Test all of this

	farms := getDummyData() // replace with call to backend once it is done
	log.Info().Msgf("Farms fetched from backend: %s", farms)
	config := infrastructure.NewConfig()
	requestClient := requesters.NewRequester(*config)
	payService := services.NewServices(requestClient)
	err := payService.ProcessPayment(config)
	return err
	// return nil
}

func getDummyData() types.Farm {

	Collection := types.Collection{Denom: types.Denom{Id: "test"}, Nfts: []types.NFT{}}
	testFarm := types.Farm{Id: "test", SubAccountName: "test", BTCWallet: "testwallet2", Collections: []types.Collection{Collection}}
	return testFarm
}
