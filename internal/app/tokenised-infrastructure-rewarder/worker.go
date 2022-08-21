package tokenised_infrastructure_rewarder

import "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"

func Start() {
	// requesters.GetAllNonExpiredNFTsFromHasura()
	// _, err := services.GetNonExpiredNFTs()
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	services.PayRewards("tb1q2t3jkecxjgfa4e7hj79ajn0er2wc65vy3vy0j9", 0.000100000)
	// a := btcutil.Amount(0)
	// fmt.Println("Zero Satoshi:", a)

	// a = btcutil.Amount(1e8)
	// fmt.Println("100,000,000 Satoshis:", a)

	// a = btcutil.Amount(1e5)
	// fmt.Println("100,000 Satoshis:", a)

	// test := 0.0000100000
	// test2, _ := btcutil.NewAmount(test)
	// a = btcutil.Amount(test2)
	// fmt.Println("test:", a)
}
