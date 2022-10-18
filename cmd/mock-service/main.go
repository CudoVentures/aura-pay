package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/farms", getFarmsHandler())
	r.HandleFunc("/subaccount_hashrate_day/{subAccountName}", getSubAccountHashrateDayHandler())

	log.Info().Msg(fmt.Sprintf("Listening on port: %d", listeningPort))
	srv := &http.Server{
		Handler:      r,
		Addr:         fmt.Sprintf(":%d", listeningPort),
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal().Err(fmt.Errorf("error while listening: %s", err)).Send()
	}
}

func getFarmsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(farms); err != nil {
			log.Error().Err(err).Send()
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func getSubAccountHashrateDayHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)
		subAccountName, ok := vars["subAccountName"]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if hashRate, ok := farmsHashrates[subAccountName]; ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			if err := json.NewEncoder(w).Encode(types.FarmHashRateElement{
				HashrateAccepted: hashRate,
			}); err != nil {
				log.Error().Err(err).Send()
				w.WriteHeader(http.StatusBadRequest)
			}

			return
		}

		w.WriteHeader(http.StatusBadRequest)
	}
}

var farms = []types.Farm{
	{
		Id:                                 1,
		SubAccountName:                     "aura_pool_test_wallet_1",
		AddressForReceivingRewardsFromPool: "tb1qlglum94l090lr73xjmeu97dcmclyut6x5kvmcs",
		LeftoverRewardPayoutAddress:        "tb1qp2erqptyzj5nj42n55vdhdat9h8qk7khjk6ryc",
		MaintenanceFeePayoutdAddress:       "tb1qw6ssuxe3jp8sjwn5yt9rkun26sxu54yjm62fz8",
		MonthlyMaintenanceFeeInBTC:         "0.0001",
		Collections: []types.Collection{
			{
				Denom: types.Denom{Id: "testdenom1"},
				Nfts:  []types.NFT{},
			},
		},
	},
	{
		Id:                                 2,
		SubAccountName:                     "aura_pool_test_wallet_2",
		AddressForReceivingRewardsFromPool: "tb1qlglum94l090lr73xjmeu97dcmclyut6x5kvmcs",
		LeftoverRewardPayoutAddress:        "tb1qp2erqptyzj5nj42n55vdhdat9h8qk7khjk6ryc",
		MaintenanceFeePayoutdAddress:       "tb1qw6ssuxe3jp8sjwn5yt9rkun26sxu54yjm62fz8",
		MonthlyMaintenanceFeeInBTC:         "0.0001",
		Collections: []types.Collection{
			{
				Denom: types.Denom{Id: "testdenom2"},
				Nfts:  []types.NFT{},
			},
		},
	},
	{
		Id:                                 3,
		SubAccountName:                     "aura_pool_test_wallet_3",
		AddressForReceivingRewardsFromPool: "tb1qlglum94l090lr73xjmeu97dcmclyut6x5kvmcs",
		LeftoverRewardPayoutAddress:        "tb1qp2erqptyzj5nj42n55vdhdat9h8qk7khjk6ryc",
		MaintenanceFeePayoutdAddress:       "tb1qw6ssuxe3jp8sjwn5yt9rkun26sxu54yjm62fz8",
		MonthlyMaintenanceFeeInBTC:         "0.0001",
		Collections: []types.Collection{
			{
				Denom: types.Denom{Id: "testdenom3"},
				Nfts:  []types.NFT{},
			},
		},
	},
}

var farmsHashrates = map[string]float64{
	"aura_pool_test_wallet_1": 1200.1,
	"aura_pool_test_wallet_2": 0,
	"aura_pool_test_wallet_3": 99999.9999,
}

const listeningPort = 8080
