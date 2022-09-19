package services

import (
	"sort"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
)

const NETWORK = "BTC"

func (s *services) SumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		for _, nft := range collection.Nfts {
			totalMintedHashPowerForAllCollections += nft.Data.HashRateOwned
		}
	}

	return totalMintedHashPowerForAllCollections
}

func (s *services) CalculatePercent(available float64, actual float64, reward float64) (btcutil.Amount, error) {
	payoutRewardPercent := float64(available) / float64(actual) * 100 // ex 100 from 1000 = 10%
	calculatedReward := float64(reward) * payoutRewardPercent / 100   // ex 10% from 1000 = 100
	btcReward, err := btcutil.NewAmount(calculatedReward)
	if err != nil {
		return -1, err
	}
	return btcReward, nil

	// possible problems: what is the foundry tx denomination - in satoshis?
	// think about if we can lose precision here as float64 is  53-bit precision..maybe use math/big type Float with more precision
	// or this: https://github.com/shopspring/decimal
}

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *services) GetNftOwnersForTimePeriodWithRewardPercent(nftId string, collectionDenomId string, periodStart int64, periodEnd int64) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart
	var transferHistoryForTimePeriod []types.NftTransferHistoryElement

	nftTransferHistory, err := s.apiRequester.GetNftTransferHistory(collectionDenomId, nftId, periodStart)
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
		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(transferHistoryForTimePeriod[i].From, NETWORK, nftId, collectionDenomId)
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

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *services) calculateNftOwnersForTimePeriodWithRewardPercent(nftTransferHistory types.NftTransferHistory, collectionDenomId string, nftId string, periodStart int64, periodEnd int64, statistics types.NFTStatistics) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart
	var transferHistoryForTimePeriod []types.NftTransferHistoryElement

	for _, transferHistoryElement := range nftTransferHistory {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	for i := 0; i < len(transferHistoryForTimePeriod); i++ {
		var timeOwned int64
		statisticsAdditionalData := types.NFTOwnerInformation{}
		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(transferHistoryForTimePeriod[i].From, NETWORK, nftId, collectionDenomId)
		if i == 0 {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - periodStart
			statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
			statisticsAdditionalData.TimeOwnedTo = periodStart
		} else {
			timeOwned = transferHistoryForTimePeriod[i].Timestamp - transferHistoryForTimePeriod[i-1].Timestamp
			statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
			statisticsAdditionalData.TimeOwnedTo = transferHistoryForTimePeriod[i-1].Timestamp
		}
		statisticsAdditionalData.TotalTimeOwned = timeOwned

		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
		statisticsAdditionalData.PercentOfTimeOwned = percentOfTimeOwned

		if err != nil {
			return nil, err
		}
		statisticsAdditionalData.PayoutAddress = nftPayoutAddress

		if _, ok := ownersWithPercentOwnedTime[nftPayoutAddress]; ok { // if the nft has been bought, sold and bought again by the same owner in the same period - accumulate
			ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		} else {
			ownersWithPercentOwnedTime[nftPayoutAddress] = percentOfTimeOwned
		}
		statistics.NFTOwnersForPeriod = append(statistics.NFTOwnersForPeriod, statisticsAdditionalData)
	}

	return ownersWithPercentOwnedTime, nil
}
