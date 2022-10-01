package services

import (
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
)

const NETWORK = "btc"

func (s *services) SumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		for _, nft := range collection.Nfts {
			totalMintedHashPowerForAllCollections += nft.DataJson.HashRateOwned
		}
	}

	return totalMintedHashPowerForAllCollections
}

func (s *services) CalculatePercent(available float64, actual float64, reward btcutil.Amount) btcutil.Amount {
	payoutRewardPercent := float64(actual) / float64(available) * 100 // ex 100 from 1000 = 10%
	calculatedReward := float64(reward) * payoutRewardPercent / 100   // ex 10% from 1000 = 100
	rewardInSatoshis := btcutil.Amount(calculatedReward)              // btcutil.Amount is int64 because satoshi is the lowest possible unit (1 satoshi = 0.00000001 bitcoin) and is an int64 in btc core code
	return rewardInSatoshis
}

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *services) calculateNftOwnersForTimePeriodWithRewardPercent(nftTransferHistory types.NftTransferHistory, collectionDenomId string, nftId string, periodStart int64, periodEnd int64, statistics types.NFTStatistics, currentNftOwner string) (map[string]float64, error) {

	ownersWithPercentOwnedTime := make(map[string]float64)
	totalPeriodTimeInSeconds := periodEnd - periodStart
	var transferHistoryForTimePeriod []types.NftTransferEvent
	var cudosAddress string

	// get only those transfer events in the current time period
	for _, transferHistoryElement := range nftTransferHistory.Data.NestedData.Events {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	// no transfers for this period, we give the current owner 100%
	if len(transferHistoryForTimePeriod) == 0 {
		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(currentNftOwner, NETWORK, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}
		ownersWithPercentOwnedTime[nftPayoutAddress] = 100
		return ownersWithPercentOwnedTime, nil
	}

	containInitialMintTx := transferHistoryForTimePeriod[0].From == "0x0" // "0x0" means no transfers and thus the from address is empty; only the to address is populated with the receiver addr
	for i := 0; i < len(transferHistoryForTimePeriod); i++ {
		var timeOwned int64
		statisticsAdditionalData := types.NFTOwnerInformation{}
		if containInitialMintTx {
			cudosAddress = transferHistoryForTimePeriod[i].To
			if len(transferHistoryForTimePeriod) == 1 {
				timeOwned = periodEnd - transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedTo = periodEnd
			} else {
				timeOwned = transferHistoryForTimePeriod[i+1].Timestamp - transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedTo = transferHistoryForTimePeriod[i+1].Timestamp
			}
		} else {
			cudosAddress = transferHistoryForTimePeriod[i].From
			if i == 0 {
				timeOwned = transferHistoryForTimePeriod[i].Timestamp - periodStart
				statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedTo = periodStart
			} else {
				timeOwned = transferHistoryForTimePeriod[i].Timestamp - transferHistoryForTimePeriod[i-1].Timestamp
				statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
				statisticsAdditionalData.TimeOwnedTo = transferHistoryForTimePeriod[i-1].Timestamp
			}

		}

		if i == len(transferHistoryForTimePeriod)-1 && len(transferHistoryForTimePeriod) > 1 {
			timeOwned += (periodEnd - transferHistoryForTimePeriod[i].Timestamp)
		}

		statisticsAdditionalData.TotalTimeOwned = timeOwned

		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
		statisticsAdditionalData.PercentOfTimeOwned = percentOfTimeOwned

		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(cudosAddress, NETWORK, nftId, collectionDenomId)
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
