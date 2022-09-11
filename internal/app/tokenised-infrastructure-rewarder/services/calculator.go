package services

import (
	"sort"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
)

func SumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		for _, nft := range collection.Nfts {
			totalMintedHashPowerForAllCollections += nft.DataJson.HashRateOwned
		}
	}

	return totalMintedHashPowerForAllCollections
}

func CalculatePercent(available float64, actual float64, reward float64) (btcutil.Amount, error) {
	payoutRewardPercent := float64(available) / float64(actual) * 100 // ex 100 from 1000 = 10%
	calculatedReward := float64(reward) * payoutRewardPercent / 100   // ex 10% from 1000 = 100
	btcReward, err := btcutil.NewAmount(calculatedReward)
	if err != nil {
		return -1, err
	}
	return btcReward, nil
}

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func GetNftOwnersForTimePeriodWithRewardPercent(nftId string, collectionDenomId string, periodStart int64, periodEnd int64) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart
	var transferHistoryForTimePeriod []types.NftTransferHistoryElement

	nftTransferHistory, err := requesters.GetNftTransferHistory(collectionDenomId, nftId, periodStart)
	if err != nil {
		return nil, err
	}

	for _, transferHistoryElement := range nftTransferHistory {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	// sort in ascending order by timestamp
	sort.Slice(transferHistoryForTimePeriod, func(i, j int) bool {
		return transferHistoryForTimePeriod[i].Timestamp < transferHistoryForTimePeriod[j].Timestamp
	})

	for i := 0; i < len(transferHistoryForTimePeriod); i++ {
		var timeOwned int64

		if i == 0 {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - periodStart
		} else {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - transferHistoryForTimePeriod[i-1].Timestamp
		}

		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
		nftPayoutAddress, err := requesters.GetPayoutAddressFromNode(transferHistoryForTimePeriod[i].From, Network, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}

		if _, ok := ownersWithPercentOwnedTime[nftPayoutAddress]; ok { // if the nft has been bought, sold and bought again by the same owner in the same period - accumulate
			ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		} else {
			ownersWithPercentOwnedTime[nftPayoutAddress] = percentOfTimeOwned
		}
	}

	return ownersWithPercentOwnedTime, nil
}
