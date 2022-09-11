package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/rs/zerolog/log"
)

func Start() error {

	farms := getDummyData() // replace with call to backend once it is done
	log.Info().Msgf("Farms fetched from backend: %s", farms)
	err := services.ProcessPaymentForFarms([]types.Farm{farms})
	return err
	// return nil
}

func getDummyData() types.Farm {
	NftOne := types.NFT{Id: "1", Name: "testName", Owner: "cudos1tr9jp0eqza9tvdvqzgyff9n3kdfew8uzhcyuwq", DataJson: types.DataJsonNFT{}}
	NftTwo := types.NFT{Id: "test", Name: "testName", Uri: "", DataJson: types.DataJsonNFT{}}
	NftThree := types.NFT{Id: "test", Name: "testName", Uri: "", DataJson: types.DataJsonNFT{}}

	Collection := types.Collection{Denom: types.Denom{Id: "test"}, Nfts: []types.NFT{NftOne, NftTwo, NftThree}}
	testFarm := types.Farm{Id: "test", SubAccountName: "test", BTCWallet: "testwallet2", Collections: []types.Collection{Collection}}
	return testFarm
}
