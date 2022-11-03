package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/rs/zerolog/log"
)

func sumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		for _, nft := range collection.Nfts {
			if time.Now().Unix() > nft.DataJson.ExpirationDate {
				log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%d} has expired! Skipping....", collection.Denom.Id, nft.Id, nft.DataJson.ExpirationDate)
				continue
			}
			totalMintedHashPowerForAllCollections += nft.DataJson.HashRateOwned
		}
	}

	return totalMintedHashPowerForAllCollections
}

func calculatePercent(available float64, actual float64, reward btcutil.Amount) btcutil.Amount {
	if available <= 0 || actual <= 0 || reward.ToBTC() <= 0 {
		return btcutil.Amount(0)
	}

	payoutRewardPercent := float64(actual) / float64(available) * 100 // ex 100 from 1000 = 10%
	calculatedReward := float64(reward) * payoutRewardPercent / 100   // ex 10% from 1000 = 100

	// btcutil.Amount is int64 because satoshi is the lowest possible unit (1 satoshi = 0.00000001 bitcoin) and is an int64 in btc core code
	return btcutil.Amount(calculatedReward)
}

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *PayService) calculateNftOwnersForTimePeriodWithRewardPercent(ctx context.Context, nftTransferHistory types.NftTransferHistory, collectionDenomId, nftId string,
	periodStart, periodEnd int64, statistics *types.NFTStatistics, currentNftOwner, payoutAddrNetwork string) (map[string]float64, error) {

	totalPeriodTimeInSeconds := periodEnd - periodStart
	if totalPeriodTimeInSeconds <= 0 {
		return nil, fmt.Errorf("invalid period, start (%d) end (%d)", periodStart, periodEnd)
	}

	var transferHistoryForTimePeriod []types.NftTransferEvent

	// get only those transfer events in the current time period
	for _, transferHistoryElement := range nftTransferHistory.Data.NestedData.Events {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	ownersWithPercentOwnedTime := make(map[string]float64)
	statisticsAdditionalData := types.NFTOwnerInformation{}

	// no transfers for this period, we give the current owner 100%
	if len(transferHistoryForTimePeriod) == 0 {
		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, currentNftOwner, payoutAddrNetwork, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}
		ownersWithPercentOwnedTime[nftPayoutAddress] = 100

		statisticsAdditionalData.TimeOwnedFrom = periodStart
		statisticsAdditionalData.TimeOwnedTo = periodEnd
		statisticsAdditionalData.TotalTimeOwned = periodEnd - periodStart
		statisticsAdditionalData.PayoutAddress = nftPayoutAddress
		statisticsAdditionalData.PercentOfTimeOwned = 100

		statistics.NFTOwnersForPeriod = []types.NFTOwnerInformation{statisticsAdditionalData}

		return ownersWithPercentOwnedTime, nil
	}

	if periodStart < transferHistoryForTimePeriod[0].Timestamp {
		transferHistoryForTimePeriod = append([]types.NftTransferEvent{
			{
				To:        transferHistoryForTimePeriod[0].From,
				From:      transferHistoryForTimePeriod[0].From,
				Timestamp: periodStart,
			},
		}, transferHistoryForTimePeriod...)
	}

	transferHistoryLen := len(transferHistoryForTimePeriod)

	transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, types.NftTransferEvent{
		To:        transferHistoryForTimePeriod[transferHistoryLen-1].To,
		From:      transferHistoryForTimePeriod[transferHistoryLen-1].To,
		Timestamp: periodEnd,
	})

	for i := 0; i < len(transferHistoryForTimePeriod)-1; i++ {

		timeOwned := transferHistoryForTimePeriod[i+1].Timestamp - transferHistoryForTimePeriod[i].Timestamp
		statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
		statisticsAdditionalData.TimeOwnedTo = transferHistoryForTimePeriod[i+1].Timestamp

		statisticsAdditionalData.TotalTimeOwned = timeOwned

		percentOfTimeOwned := roundToPrecision(float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100)
		statisticsAdditionalData.PercentOfTimeOwned = percentOfTimeOwned

		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, transferHistoryForTimePeriod[i].To, payoutAddrNetwork, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}

		statisticsAdditionalData.PayoutAddress = nftPayoutAddress

		ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		statistics.NFTOwnersForPeriod = append(statistics.NFTOwnersForPeriod, statisticsAdditionalData)
	}

	return ownersWithPercentOwnedTime, nil
}

func roundToPrecision(value float64) (result float64) {
	result, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return
}
