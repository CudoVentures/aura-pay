package main

import (
	"fmt"

	rewarder "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder"
)

func main() {
	fmt.Println("hello world")
	// service to fetch the NFTS that have expiryDate and filter them expiryDate >= currentTime
	// once fetched, fetch the same NFTS from the blockchain and ensure that the data is correct and if not - reject them
	// from the NFTS get the hash ids and machine ids and connect to the pool and obtain the reward
	// for each nft connect to a bitcoin node and initiate transfer of funds from cudo account to main
	rewarder.PayRewards()
}
