package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *PayService) calculateNftOwnersForTimePeriodWithRewardPercent(ctx context.Context, nftTransferHistory types.NftTransferHistory,
	collectionDenomId, nftId string, periodStart, periodEnd int64, statistics *types.NFTStatistics, currentNftOwner, payoutAddrNetwork string, rewardForNftAfterFeeBtcDecimal decimal.Decimal) (map[string]float64, error) {

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
		statisticsAdditionalData.Reward = rewardForNftAfterFeeBtcDecimal

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

	var totalCalculatedReward decimal.Decimal

	for i := 0; i < len(transferHistoryForTimePeriod)-1; i++ {

		timeOwned := transferHistoryForTimePeriod[i+1].Timestamp - transferHistoryForTimePeriod[i].Timestamp
		statisticsAdditionalData.TimeOwnedFrom = transferHistoryForTimePeriod[i].Timestamp
		statisticsAdditionalData.TimeOwnedTo = transferHistoryForTimePeriod[i+1].Timestamp

		statisticsAdditionalData.TotalTimeOwned = timeOwned

		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100
		statisticsAdditionalData.PercentOfTimeOwned = percentOfTimeOwned

		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, transferHistoryForTimePeriod[i].To, payoutAddrNetwork, nftId, collectionDenomId)
		if err != nil {
			return nil, err
		}

		statisticsAdditionalData.PayoutAddress = nftPayoutAddress
		statisticsAdditionalData.Owner = transferHistoryForTimePeriod[i].To

		calculatedReward := rewardForNftAfterFeeBtcDecimal.Mul(decimal.NewFromFloat(percentOfTimeOwned / 100))
		statisticsAdditionalData.Reward = calculatedReward
		totalCalculatedReward = totalCalculatedReward.Add(calculatedReward)

		ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		statistics.NFTOwnersForPeriod = append(statistics.NFTOwnersForPeriod, statisticsAdditionalData)
	}

	nftRewardDistributionleftovers := rewardForNftAfterFeeBtcDecimal.Sub(totalCalculatedReward)

	if nftRewardDistributionleftovers.LessThan(decimal.Zero) {
		return nil, fmt.Errorf("calculated NFT reward distribution is greater than the total given. CalculatedForOwnerDistribution: %s, TotalGiventoDistribute: %s", totalCalculatedReward, rewardForNftAfterFeeBtcDecimal)
	}

	lastOwnerIndex := len(statistics.NFTOwnersForPeriod) - 1
	statistics.NFTOwnersForPeriod[lastOwnerIndex].Reward = statistics.NFTOwnersForPeriod[lastOwnerIndex].Reward.Add(nftRewardDistributionleftovers)

	return ownersWithPercentOwnedTime, nil
}

func (s *PayService) calculateHourlyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) decimal.Decimal {
	currentYear, currentMonth, _ := s.helper.Date()
	periodLength := s.helper.DaysIn(currentMonth, currentYear)

	mtFeeInBtc := decimal.NewFromFloat(farm.MaintenanceFeeInBtc)

	btcFeePerOneHashPowerBtcDecimal := mtFeeInBtc.Div(decimal.NewFromFloat(currentHashPowerForFarm))
	dailyFeeInBtcDecimal := btcFeePerOneHashPowerBtcDecimal.Div(decimal.NewFromInt(int64(periodLength)))
	hourlyFeeInBtcDecimal := dailyFeeInBtcDecimal.Div(decimal.NewFromInt(24))

	return hourlyFeeInBtcDecimal
}

func (s *PayService) calculateMaintenanceFeeForNFT(periodStart int64,
	periodEnd int64,
	hourlyFeeInBtcDecimal decimal.Decimal,
	rewardForNftBtcDecimal decimal.Decimal) (decimal.Decimal, decimal.Decimal, decimal.Decimal) {

	periodInHoursToPayFor := float64(periodEnd-periodStart) / float64(3600) // period for which we are paying the MT fee

	nftMaintenanceFeeForPayoutPeriodBtcDecimal := hourlyFeeInBtcDecimal.Mul(decimal.NewFromFloat(periodInHoursToPayFor)) // the fee for the period
	if nftMaintenanceFeeForPayoutPeriodBtcDecimal.GreaterThan(rewardForNftBtcDecimal) {                                  // if the fee is greater - it has higher priority then the users reward
		nftMaintenanceFeeForPayoutPeriodBtcDecimal = rewardForNftBtcDecimal
		rewardForNftBtcDecimal = decimal.Zero
	} else {
		rewardForNftBtcDecimal = rewardForNftBtcDecimal.Sub(nftMaintenanceFeeForPayoutPeriodBtcDecimal)
	}

	partOfMaintenanceFeeForCudoBtcDecimal := nftMaintenanceFeeForPayoutPeriodBtcDecimal.Mul(decimal.NewFromFloat(s.config.CUDOMaintenanceFeePercent / 100)) // ex 10% from 1000 = 100
	nftMaintenanceFeeForPayoutPeriodBtcDecimal = nftMaintenanceFeeForPayoutPeriodBtcDecimal.Sub(partOfMaintenanceFeeForCudoBtcDecimal)

	return nftMaintenanceFeeForPayoutPeriodBtcDecimal, partOfMaintenanceFeeForCudoBtcDecimal, rewardForNftBtcDecimal
}

func (s *PayService) calculateCudosFeeOfTotalFarmIncome(totalFarmIncomeBtcDecimal decimal.Decimal) (decimal.Decimal, decimal.Decimal) {

	farmIncomeCudosFeeBtcDecimal := totalFarmIncomeBtcDecimal.Mul(decimal.NewFromFloat(s.config.CUDOFeeOnAllBTC / 100)) // ex 10% = 0.1 * total
	farmIncomeAfterCudosFeeBtcDecimal := totalFarmIncomeBtcDecimal.Sub(farmIncomeCudosFeeBtcDecimal)

	return farmIncomeAfterCudosFeeBtcDecimal, farmIncomeCudosFeeBtcDecimal
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

func calculatePercent(available float64, actual float64, reward decimal.Decimal) decimal.Decimal {
	if available <= 0 || actual <= 0 || reward.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	payoutRewardPercent := decimal.NewFromFloat(actual).Div(decimal.NewFromFloat(available))
	calculatedReward := reward.Mul(payoutRewardPercent)

	// btcutil.Amount is int64 because satoshi is the lowest possible unit (1 satoshi = 0.00000001 bitcoin) and is an int64 in btc core code
	return calculatedReward
}

func calculatePercentByTime(timestampPrevPayment, timestampMint, timestampCurrentPayment int64, totalRewardForPeriod decimal.Decimal) decimal.Decimal {
	if timestampMint <= timestampPrevPayment {
		return totalRewardForPeriod
	}

	timeMinted := timestampCurrentPayment - timestampMint
	wholePeriod := timestampCurrentPayment - timestampPrevPayment
	percentOfPeriodMitned := decimal.NewFromInt(timeMinted).Div(decimal.NewFromInt(wholePeriod))

	return totalRewardForPeriod.Mul(percentOfPeriodMitned)
}
