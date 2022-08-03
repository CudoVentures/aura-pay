package tokenised_infrastructure_rewarder

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"log"
)

func Start() {
	// requesters.GetAllNonExpiredNFTsFromHasura()
	_, err := services.GetNonExpiredNFTs()
	if err != nil {
		log.Fatalln(err)
	}
}
