package tokenised_infrastructure_rewarder

import "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"

func Start() {
	requesters.GetAllNonExpiredNFTsFromHasura()
}
