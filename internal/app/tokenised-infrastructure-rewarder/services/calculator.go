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

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *PayService) calculateNftOwnersForTimePeriodWithRewardPercent(ctx context.Context, nftTransferHistory types.NftTransferHistory,
	collectionDenomId, nftId string, periodStart, periodEnd int64, statistics *types.NFTStatistics, currentNftOwner, payoutAddrNetwork string, rewardForNftAfterFee btcutil.Amount) (map[string]float64, error) {

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
		statisticsAdditionalData.Owner = currentNftOwner
		statisticsAdditionalData.Reward = rewardForNftAfterFee

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
		statisticsAdditionalData.Owner = transferHistoryForTimePeriod[i].To
		statisticsAdditionalData.Reward = rewardForNftAfterFee.MulF64(percentOfTimeOwned / 100)

		ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		statistics.NFTOwnersForPeriod = append(statistics.NFTOwnersForPeriod, statisticsAdditionalData)
	}

	return ownersWithPercentOwnedTime, nil
}

func (s *PayService) calculateHourlyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) (btcutil.Amount, error) {
	currentYear, currentMonth, _ := s.helper.Date()
	periodLength := s.helper.DaysIn(currentMonth, currentYear)

	mtFeeInSatoshis, err := btcutil.NewAmount(farm.MaintenanceFeeInBtc)
	if err != nil {
		return -1, err
	}
	feePerOneHashPower := btcutil.Amount(float64(mtFeeInSatoshis) / currentHashPowerForFarm)
	dailyFeeInSatoshis := int(feePerOneHashPower) / periodLength
	hourlyFeeInSatoshis := dailyFeeInSatoshis / 24
	return btcutil.Amount(hourlyFeeInSatoshis), nil
}

func (s *PayService) calculateMaintenanceFeeForNFT(periodStart int64,
	periodEnd int64,
	hourlyFeeInSatoshis btcutil.Amount,
	rewardForNft btcutil.Amount) (btcutil.Amount, btcutil.Amount, btcutil.Amount) {

	periodInHoursToPayFor := (periodEnd - periodStart) / 3600                                              // period for which we are paying the MT fee
	nftMaintenanceFeeForPayoutPeriod := btcutil.Amount(periodInHoursToPayFor * int64(hourlyFeeInSatoshis)) // the fee for the period
	if nftMaintenanceFeeForPayoutPeriod > rewardForNft {                                                   // if the fee is greater - it has higher priority then the users reward
		nftMaintenanceFeeForPayoutPeriod = rewardForNft
		rewardForNft = 0
	} else {
		rewardForNft -= nftMaintenanceFeeForPayoutPeriod
	}

	partOfMaintenanceFeeForCudo := btcutil.Amount(float64(nftMaintenanceFeeForPayoutPeriod) * s.config.CUDOMaintenanceFeePercent / 100) // ex 10% from 1000 = 100
	nftMaintenanceFeeForPayoutPeriod -= partOfMaintenanceFeeForCudo

	return nftMaintenanceFeeForPayoutPeriod, partOfMaintenanceFeeForCudo, rewardForNft
}

func (s *PayService) calculateCudosFeeOfTotalFarmIncome(totalFarmIncome btcutil.Amount) (btcutil.Amount, btcutil.Amount) {

	farmIncomeCudosFee := totalFarmIncome.MulF64(s.config.CUDOFeeOnAllBTC / 100) // ex 10% = 0.1 * total
	farmIncomeAfterCudosFee := totalFarmIncome - farmIncomeCudosFee

	return farmIncomeAfterCudosFee, farmIncomeCudosFee
}

func sumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		totalMintedHashPowerForAllCollections += sumMintedHashPowerForCollection(collection)
	}

	return totalMintedHashPowerForAllCollections
}

func sumMintedHashPowerForCollection(collection types.Collection) float64 {
	var totalMintedHashPowerForCollection float64

	for _, nft := range collection.Nfts {
		if time.Now().Unix() > nft.DataJson.ExpirationDate {
			log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%d} has expired! Skipping....", collection.Denom.Id, nft.Id, nft.DataJson.ExpirationDate)
			continue
		}
		totalMintedHashPowerForCollection += nft.DataJson.HashRateOwned
	}

	return totalMintedHashPowerForCollection
}

func roundToPrecision(value float64) (result float64) {
	result, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return
}

func calculatePercent(available float64, actual float64, reward btcutil.Amount) btcutil.Amount {
	if available <= 0 || actual <= 0 || reward.ToBTC() <= 0 {
		return btcutil.Amount(0)
	}

	payoutRewardPercent := actual / available
	calculatedReward := reward.MulF64(payoutRewardPercent)

	// btcutil.Amount is int64 because satoshi is the lowest possible unit (1 satoshi = 0.00000001 bitcoin) and is an int64 in btc core code
	return calculatedReward
}
