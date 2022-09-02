package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
)

func Start() {
	// get nfts
	// requesters.GetAllNonExpiredNFTsFromHasura()
	// _, err := services.GetNonExpiredNFTs()
	// if err != nil {
	// 	log.Fatalln(err)
	// }

	// type NFT struct {
	// 	Id       string   `json:"id"`
	// 	Name     string   `json:"name"`
	// 	Uri      string   `json:"uri"`
	// 	Data     string   `json:"data"`
	// 	DataJson DataJson `json:"data_json"` // TODO! : Fix this by having an unified json data type on both the node,bdjuno/hasura and rewarder
	// 	Owner    string   `json:"owner"`
	// }

	NftOne := types.NFT{Id: "1", Name: "testName", Owner: "cudos1tr9jp0eqza9tvdvqzgyff9n3kdfew8uzhcyuwq", DataJson: types.DataJson{}}
	// NftTwo := types.NFT{Id: "test", Name: "testName", Uri: "", DataJson: types.DataJson{}}
	// NftThree := types.NFT{Id: "test", Name: "testName", Uri: "", DataJson: types.DataJson{}}

	Collection := types.Collection{Denom: types.Denom{Id: "test"}, Nfts: []types.NFT{NftOne}}
	testFarm := types.Farm{Id: "test", Name: "test", BTCWallet: "testwallet2", Collections: []types.Collection{Collection}}

	services.ProcessPaymentForFarms([]types.Farm{testFarm})

}
