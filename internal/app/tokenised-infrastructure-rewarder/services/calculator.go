package services

import (
	"context"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/shopspring/decimal"
)

// if the nft has been owned by two or more people you need to split this reward for each one of them based on the time of ownership
// so a method that returns each nft owner for the time period with the time he owned it as percent
// use this percent to calculate how much each one should get from the total reward
func (s *PayService) calculateNftOwnersForTimePeriodWithRewardPercent(ctx context.Context, nftTransferHistory types.NftTransferHistory,
	collectionDenomId, nftId string, periodStart, periodEnd int64, currentNftOwner, payoutAddrNetwork string, rewardForNftAfterFeeBtcDecimal decimal.Decimal) (map[string]float64, []types.NFTOwnerInformation, error) {

	totalPeriodTimeInSeconds := periodEnd - periodStart
	// tx time is block time
	// many transactions can have the same timestamps
	// so 0 time between last payment tx and current is a valid case
	if totalPeriodTimeInSeconds < 0 {
		return nil, nil, fmt.Errorf("invalid period, start (%d) end (%d)", periodStart, periodEnd)
	}

	var transferHistoryForTimePeriod []types.NftTransferEvent

	// get only those transfer events in the current time period
	for _, transferHistoryElement := range nftTransferHistory.Data.NestedData.Events {
		if transferHistoryElement.Timestamp >= periodStart && transferHistoryElement.Timestamp <= periodEnd {
			transferHistoryForTimePeriod = append(transferHistoryForTimePeriod, transferHistoryElement)
		}
	}

	ownersWithPercentOwnedTime := make(map[string]float64)
	// no transfers for this period, we give the current owner 100%
	if len(transferHistoryForTimePeriod) == 0 {
		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, currentNftOwner, payoutAddrNetwork, nftId, collectionDenomId)
		if err != nil {
			return nil, nil, err
		}
		ownersWithPercentOwnedTime[nftPayoutAddress] = 100

		statisticsAdditionalData := types.NFTOwnerInformation{}
		statisticsAdditionalData.TimeOwnedFrom = periodStart
		statisticsAdditionalData.TimeOwnedTo = periodEnd
		statisticsAdditionalData.TotalTimeOwned = periodEnd - periodStart
		statisticsAdditionalData.PayoutAddress = nftPayoutAddress
		statisticsAdditionalData.PercentOfTimeOwned = 100
		statisticsAdditionalData.Owner = currentNftOwner
		statisticsAdditionalData.Reward = rewardForNftAfterFeeBtcDecimal

		return ownersWithPercentOwnedTime,
			[]types.NFTOwnerInformation{{
				TimeOwnedFrom:      periodStart,
				TimeOwnedTo:        periodEnd,
				TotalTimeOwned:     periodEnd - periodStart,
				PayoutAddress:      nftPayoutAddress,
				PercentOfTimeOwned: 100,
				Owner:              currentNftOwner,
				Reward:             rewardForNftAfterFeeBtcDecimal,
			}},
			nil
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
	nftOwnersInformation := []types.NFTOwnerInformation{}
	for i := 0; i < len(transferHistoryForTimePeriod)-1; i++ {
		timeOwned := transferHistoryForTimePeriod[i+1].Timestamp - transferHistoryForTimePeriod[i].Timestamp
		percentOfTimeOwned := float64(timeOwned) / float64(totalPeriodTimeInSeconds) * 100

		nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, transferHistoryForTimePeriod[i].To, payoutAddrNetwork, nftId, collectionDenomId)
		if err != nil {
			return nil, nil, err
		}

		calculatedReward := rewardForNftAfterFeeBtcDecimal.Mul(decimal.NewFromFloat(percentOfTimeOwned / 100))
		totalCalculatedReward = totalCalculatedReward.Add(calculatedReward)
		ownersWithPercentOwnedTime[nftPayoutAddress] += percentOfTimeOwned

		nftOwnersInformation = append(nftOwnersInformation, types.NFTOwnerInformation{
			PercentOfTimeOwned: percentOfTimeOwned,
			TotalTimeOwned:     timeOwned,
			TimeOwnedFrom:      transferHistoryForTimePeriod[i].Timestamp,
			TimeOwnedTo:        transferHistoryForTimePeriod[i+1].Timestamp,
			PayoutAddress:      nftPayoutAddress,
			Owner:              transferHistoryForTimePeriod[i].To,
			Reward:             calculatedReward,
		})
	}

	nftRewardDistributionleftovers := rewardForNftAfterFeeBtcDecimal.Sub(totalCalculatedReward)

	if nftRewardDistributionleftovers.LessThan(decimal.Zero) {
		return nil, nil, fmt.Errorf("calculated NFT reward distribution is greater than the total given. CalculatedForOwnerDistribution: %s, TotalGiventoDistribute: %s", totalCalculatedReward, rewardForNftAfterFeeBtcDecimal)
	}

	lastOwnerIndex := len(nftOwnersInformation) - 1
	nftOwnersInformation[lastOwnerIndex].Reward = nftOwnersInformation[lastOwnerIndex].Reward.Add(nftRewardDistributionleftovers)

	return ownersWithPercentOwnedTime, nftOwnersInformation, nil
}

// calculateHourlyMaintenanceFee calculates the hourly maintenance fee for a farm
// the farm maintenance fee is given in BTC on early basis
// split that into hourly fee
func (s *PayService) calculateHourlyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) decimal.Decimal {
	currentYear, currentMonth, _ := s.helper.Date()
	periodLength := s.helper.DaysIn(currentMonth, currentYear)

	mtFeeInBtc := decimal.NewFromFloat(farm.MaintenanceFeeInBtc)

	btcFeePerOneHashPowerBtcDecimal := mtFeeInBtc.Div(decimal.NewFromFloat(currentHashPowerForFarm))
	dailyFeeInBtcDecimal := btcFeePerOneHashPowerBtcDecimal.Div(decimal.NewFromInt(int64(periodLength)))
	hourlyFeeInBtcDecimal := dailyFeeInBtcDecimal.Div(decimal.NewFromInt(24))

	return hourlyFeeInBtcDecimal
}

// calculate the hours that the period consists of
// calculate fee based on the hourly fee multiplied by the period hours
// if the fee is bigger than the nft reward, reduce it to the nft reward and set the reward to zero
// else reduce the nft reward by the fee
// finally distribute the maintenance fee between aura and farm
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

// calculates the cudos/aura fee from the total farm payment before maintenance fees
// the fee is taken from the payment service env
func (s *PayService) calculateCudosFeeOfTotalFarmIncome(totalFarmIncomeBtcDecimal decimal.Decimal) (decimal.Decimal, decimal.Decimal) {

	farmIncomeCudosFeeBtcDecimal := totalFarmIncomeBtcDecimal.Mul(decimal.NewFromFloat(s.config.CUDOFeeOnAllBTC / 100)) // ex 10% = 0.1 * total
	farmIncomeAfterCudosFeeBtcDecimal := totalFarmIncomeBtcDecimal.Sub(farmIncomeCudosFeeBtcDecimal)

	return farmIncomeAfterCudosFeeBtcDecimal, farmIncomeCudosFeeBtcDecimal
}

// calculates the total hash power distributed to the collections
// used for checks - it shouldn't exceed that of the farm
func sumMintedHashPowerForAllCollections(collections []types.Collection) float64 {
	var totalMintedHashPowerForAllCollections float64

	for _, collection := range collections {
		totalMintedHashPowerForAllCollections += sumMintedHashPowerForCollection(collection)
	}

	return totalMintedHashPowerForAllCollections
}

// calculates the minted hash power for a collection
// that means the minted nfts from that collection
func sumMintedHashPowerForCollection(collection types.Collection) float64 {
	var totalMintedHashPowerForCollection float64

	for _, nft := range collection.Nfts {
		totalMintedHashPowerForCollection += nft.DataJson.HashRateOwned
	}

	return totalMintedHashPowerForCollection
}

// given total hash power and allocated hash power for the given payment (nft, collection)
// calculate the reward as percent of the total
func calculateRewardByPercent(availableHashPower float64, actualHashPower float64, reward decimal.Decimal) decimal.Decimal {
	if availableHashPower <= 0 || actualHashPower <= 0 || reward.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	payoutRewardPercent := decimal.NewFromFloat(actualHashPower).Div(decimal.NewFromFloat(availableHashPower))
	calculatedReward := reward.Mul(payoutRewardPercent)

	// btcutil.Amount is int64 because satoshi is the lowest possible unit (1 satoshi = 0.00000001 bitcoin) and is an int64 in btc core code
	return calculatedReward
}

// given period and nft valid end time
// calculate the reward it should take
// this is used when nft that is minted or expired in the middle of a payment period exists
func calculatePercentByTime(timestampPrevPayment, timestampCurrentPayment, nftStartTime, nftEndTime int64, totalRewardForPeriod decimal.Decimal) decimal.Decimal {
	if nftStartTime <= timestampPrevPayment && nftEndTime >= timestampCurrentPayment {
		return totalRewardForPeriod
	}

	timeMinted := nftEndTime - nftStartTime
	wholePeriod := timestampCurrentPayment - timestampPrevPayment
	percentOfPeriodMitned := decimal.NewFromInt(timeMinted).Div(decimal.NewFromInt(wholePeriod))

	return totalRewardForPeriod.Mul(percentOfPeriodMitned)
}

// during calculations of the nft fees and rewards there are some inaccuracies
// sum nft rewards and fees for each nft
// and check that they are not bigger than the total reward for all nfts of the farm
// return it so it van be distributed
func calculateLeftoverNftRewardDistribution(rewardForNftOwnersBtcDecimal decimal.Decimal, statistics []types.NFTStatistics) (decimal.Decimal, error) {
	// return to the farm owner whatever is left
	var distributedNftRewards decimal.Decimal
	for _, nftStat := range statistics {
		distributedNftRewards = distributedNftRewards.Add(nftStat.Reward).Add(nftStat.MaintenanceFee).Add(nftStat.CUDOPartOfMaintenanceFee)
	}

	leftoverNftRewardDistribution := rewardForNftOwnersBtcDecimal.Sub(distributedNftRewards)

	if leftoverNftRewardDistribution.LessThan(decimal.Zero) {
		return decimal.Decimal{}, fmt.Errorf("distributed NFT awards bigger than the farm reward after cudos fee. NftRewardDistribution: %s, TotalFarmRewardAfterCudosFee: %s", distributedNftRewards, rewardForNftOwnersBtcDecimal)
	}

	return leftoverNftRewardDistribution, nil
}

// sum all amounts for all addresses taht will be sent
// they should equal exactly the total farm reward or something went wrong during calculation
func checkTotalAmountToDistribute(receivedRewardForFarmBtcDecimal decimal.Decimal, destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal) error {
	var totalAmountToPayToAddressesBtcDecimal decimal.Decimal

	for _, amount := range destinationAddressesWithAmountBtcDecimal {
		totalAmountToPayToAddressesBtcDecimal = totalAmountToPayToAddressesBtcDecimal.Add(amount)
	}

	if !totalAmountToPayToAddressesBtcDecimal.Equals(receivedRewardForFarmBtcDecimal) {
		return fmt.Errorf("distributed amount doesn't equal total farm rewards. Distributed amount: {%s}, TotalFarmReward: {%s}", totalAmountToPayToAddressesBtcDecimal, receivedRewardForFarmBtcDecimal)
	}

	return nil
}
