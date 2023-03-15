package services

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/rs/zerolog/log"
)

func NewPayService(config *infrastructure.Config, apiRequester ApiRequester, helper Helper, btcNetworkParams *types.BtcNetworkParams) *PayService {
	return &PayService{
		config:                    config,
		helper:                    helper,
		btcNetworkParams:          btcNetworkParams,
		apiRequester:              apiRequester,
		lastEmailTimestamp:        0,
		btcWalletOpenFailsPerFarm: make(map[string]int),
	}
}

func (s *PayService) Execute(ctx context.Context, btcClient BtcClient, storage Storage) error {
	farms, err := storage.GetApprovedFarms(ctx)
	if err != nil {
		return err
	}

	for _, farm := range farms {
		if err := s.processFarm(ctx, btcClient, storage, farm); err != nil {
			msg := fmt.Sprintf("processing farm {%s} failed. Error: %s", farm.RewardsFromPoolBtcWalletName, err)
			// send email only once per half hour
			if s.helper.Unix() >= s.lastEmailTimestamp+int64(time.Minute.Seconds()*30) {
				s.helper.SendMail(msg)
				s.lastEmailTimestamp = s.helper.Unix()
			}
			log.Error().Msg(msg)
			continue
		}
	}

	return nil
}

func (s *PayService) processFarm(ctx context.Context, btcClient BtcClient, storage Storage, farm types.Farm) error {
	log.Debug().Msgf("Processing farm with name %s..", farm.RewardsFromPoolBtcWalletName)
	err := validateFarm(farm)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Loading farm wallet...")
	loaded, err := s.loadWallet(btcClient, farm.RewardsFromPoolBtcWalletName)
	if err != nil {
		return err
	}

	if !loaded {
		return nil
	}
	defer unloadWallet(btcClient, farm.RewardsFromPoolBtcWalletName)

	log.Debug().Msgf("Getting unspent transactions for farm wallet...")
	unspentTxsForFarm, err := s.getUnspentTxsForFarm(ctx, btcClient, storage, []string{farm.AddressForReceivingRewardsFromPool})
	if err != nil {
		return err
	}

	if len(unspentTxsForFarm) == 0 {
		log.Info().Msgf("no unspent TXs for farm {{%s}} with address {{%s}}....skipping this farm", farm.RewardsFromPoolBtcWalletName, farm.AddressForReceivingRewardsFromPool)
		return nil
	}

	log.Debug().Msgf("Getting the last payment timestamp for farm...")
	// get previoud tx timestamp
	lastPaymentTimestamp, err := s.getLastUTXOTransactionTimestamp(ctx, storage, farm)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Unlocking farm wallet...")
	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return err
	}
	defer lockWallet(btcClient, farm.RewardsFromPoolBtcWalletName)

	// for each payment
	log.Debug().Msgf("Processing unspent transactions for farm...")
	for _, unspentTxForFarm := range unspentTxsForFarm {
		lastProcessedPaymentTimestamp, err := s.processFarmUnspentTx(ctx, btcClient, storage, farm, unspentTxForFarm, lastPaymentTimestamp)
		if err != nil {
			return err
		}
		lastPaymentTimestamp = lastProcessedPaymentTimestamp
	}

	log.Debug().Msgf("Processing farm finished successfully")
	return nil
}

func (s *PayService) processFarmUnspentTx(
	ctx context.Context,
	btcClient BtcClient,
	storage Storage,
	farm types.Farm,
	unspentTxForFarm btcjson.ListUnspentResult,
	lastPaymentTimestamp int64,
) (int64, error) {
	txRawResult, err := s.getUnspentTxDetails(ctx, btcClient, unspentTxForFarm)
	if err != nil {
		return 0, err
	}

	// period end is the time the payment was made
	periodEnd := txRawResult.Time

	receivedRewardForFarmBtcDecimal := decimal.NewFromFloat(unspentTxForFarm.Amount)
	totalRewardForFarmAfterCudosFeeBtcDecimal, cudosFeeOfTotalRewardBtcDecimal := s.calculateCudosFeeOfTotalFarmIncome(receivedRewardForFarmBtcDecimal)

	log.Debug().Msgf("-------------------------------------------------")
	log.Debug().Msgf("Processing Unspent TX: %s, Payment period: %d to %d", unspentTxForFarm.TxID, lastPaymentTimestamp, periodEnd)
	log.Debug().Msgf("Total reward for farm \"%s\": %s", farm.RewardsFromPoolBtcWalletName, receivedRewardForFarmBtcDecimal)
	log.Debug().Msgf("Cudos part of total farm reward: %s", cudosFeeOfTotalRewardBtcDecimal)
	log.Debug().Msgf("Total reward for farm \"%s\" after cudos fee: %s", farm.RewardsFromPoolBtcWalletName, totalRewardForFarmAfterCudosFeeBtcDecimal)

	// currently always getting the hash power as set in the farm entity in the Aura Pool Service
	// that way the rewards will be sent proportionally, no matter the current hash power of the farm
	// otherwise there is a case where the farm hash power falls below the registered and the calculations fail
	// if this case is to be handled, it need more work
	currentHashPowerForFarm := farm.TotalHashPower
	log.Debug().Msgf("Total hash power for farm %s: %.6f", farm.RewardsFromPoolBtcWalletName, currentHashPowerForFarm)
	hourlyMaintenanceFeeInBtcDecimal := s.calculateHourlyMaintenanceFee(farm, currentHashPowerForFarm)
	// var currentHashPowerForFarm float64
	// if s.config.IsTesting {
	// 	currentHashPowerForFarm = farm.TotalHashPower // for testing & QA
	// } else {
	// 	// used to get current hash from FOUNDRY //
	//  // set the date to the period begin?
	// 	currentHashPowerForFarm, err = s.apiRequester.GetFarmTotalHashPowerFromPoolToday(ctx, farm.SubAccountName,
	// 		time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
	// 	if err != nil {
	// 		return err
	// 	}

	// 	if currentHashPowerForFarm <= 0 {
	// 		return fmt.Errorf("invalid hash power (%f) for farm (%s)", currentHashPowerForFarm, farm.SubAccountName)
	// 	}
	// }

	farmCollectionsWithNFTs, farmAuraPoolCollectionsMap, err := s.getCollectionsWithNftsForFarm(ctx, storage, farm)
	if err != nil {
		return 0, err
	}

	if len(farmCollectionsWithNFTs) == 0 {
		log.Error().Msgf("no verified colletions for farm {%s}", farm.RewardsFromPoolBtcWalletName)
		return 0, nil
	}

	nonExpiredNFTsCount := s.filterExpiredBeforePeriodNFTs(farmCollectionsWithNFTs, lastPaymentTimestamp)
	log.Debug().Msgf("Non expired NFTs count: %d", nonExpiredNFTsCount)

	if nonExpiredNFTsCount == 0 {
		log.Error().Msgf("all nfts for farm {%s} are expired", farm.RewardsFromPoolBtcWalletName)
		return 0, nil
	}

	mintedHashPowerForFarm := sumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)

	log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.RewardsFromPoolBtcWalletName, mintedHashPowerForFarm)

	rewardForNftOwnersBtcDecimal := calculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarmAfterCudosFeeBtcDecimal)
	leftoverHashPower := currentHashPowerForFarm - mintedHashPowerForFarm // if hash power increased or not all of it is used as NFTs
	var rewardToReturnBtcDecimal decimal.Decimal

	destinationAddressesWithAmountBtcDecimal := make(map[string]decimal.Decimal)

	// add cudos fee on total farm income
	addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, cudosFeeOfTotalRewardBtcDecimal, s.config.CUDOFeePayoutAddress)

	// return to the farm owner whatever is left
	if leftoverHashPower > 0 {
		rewardToReturnBtcDecimal = totalRewardForFarmAfterCudosFeeBtcDecimal.Sub(rewardForNftOwnersBtcDecimal)
		addLeftoverRewardToFarmOwner(destinationAddressesWithAmountBtcDecimal, rewardToReturnBtcDecimal, farm.LeftoverRewardPayoutAddress)
	}
	log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn: %s, farm: {%s}", rewardForNftOwnersBtcDecimal, rewardToReturnBtcDecimal, farm.RewardsFromPoolBtcWalletName)

	var statistics []types.NFTStatistics
	var collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation

	for _, collection := range farmCollectionsWithNFTs {
		collectionProcessResult, err := s.processCollection(
			ctx,
			storage,
			farm,
			collection,
			destinationAddressesWithAmountBtcDecimal,
			rewardForNftOwnersBtcDecimal,
			mintedHashPowerForFarm,
			currentHashPowerForFarm,
			totalRewardForFarmAfterCudosFeeBtcDecimal,
			cudosFeeOfTotalRewardBtcDecimal,
			hourlyMaintenanceFeeInBtcDecimal,
			lastPaymentTimestamp,
			periodEnd,
			farmAuraPoolCollectionsMap,
		)
		if err != nil {
			return 0, err
		}

		collectionPaymentAllocationsStatistics = append(collectionPaymentAllocationsStatistics, collectionProcessResult.CollectionPaymentAllocation)
		statistics = append(statistics, collectionProcessResult.NftStatistics...)
	}

	log.Debug().Msgf("All collections processed. Starting the send process...")
	if s.sendRewards(
		ctx,
		storage,
		farm,
		unspentTxForFarm,
		periodEnd,
		receivedRewardForFarmBtcDecimal,
		rewardForNftOwnersBtcDecimal,
		totalRewardForFarmAfterCudosFeeBtcDecimal,
		destinationAddressesWithAmountBtcDecimal,
		statistics,
		collectionPaymentAllocationsStatistics,
	) != nil {
		return 0, err
	}

	log.Debug().Msgf("Sends completed...")

	return periodEnd, nil
}

func (s *PayService) processCollection(
	ctx context.Context,
	storage Storage,
	farm types.Farm,
	collection types.Collection,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	rewardForNftOwnersBtcDecimal decimal.Decimal,
	mintedHashPowerForFarm, currentHashPowerForFarm float64,
	totalRewardForFarmAfterCudosFeeBtcDecimal, cudosFeeOfTotalRewardBtcDecimal, hourlyMaintenanceFeeInBtcDecimal decimal.Decimal,
	lastPaymentTimestamp, periodEnd int64,
	farmAuraPoolCollectionsMap map[string]types.AuraPoolCollection,
) (CollectionProcessResult, error) {
	log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)

	var CUDOMaintenanceFeeBtcDecimal decimal.Decimal
	var farmMaintenanceFeeBtcDecimal decimal.Decimal
	var nftRewardsAfterFeesBtcDecimal decimal.Decimal

	var nftStatistics []types.NFTStatistics

	for _, nft := range collection.Nfts {
		nftProcessResult, processed, err := s.processNft(
			ctx,
			storage,
			farm,
			collection,
			nft,
			destinationAddressesWithAmountBtcDecimal,
			rewardForNftOwnersBtcDecimal,
			mintedHashPowerForFarm,
			hourlyMaintenanceFeeInBtcDecimal,
			lastPaymentTimestamp,
			periodEnd,
		)
		if err != nil {
			return CollectionProcessResult{}, err
		}

		// this is for all the cases that there is no error, but nft was not processed
		// i.e nft is expired, or minted after the period end
		if !processed {
			continue
		}

		CUDOMaintenanceFeeBtcDecimal = CUDOMaintenanceFeeBtcDecimal.Add(nftProcessResult.CudoPartOfMaintenanceFeeBtcDecimal)
		farmMaintenanceFeeBtcDecimal = farmMaintenanceFeeBtcDecimal.Add(nftProcessResult.MaintenanceFeeBtcDecimal)
		nftRewardsAfterFeesBtcDecimal = nftRewardsAfterFeesBtcDecimal.Add(nftProcessResult.RewardForNftAfterFeeBtcDecimal)

		nftStatistics = append(nftStatistics, types.NFTStatistics{
			TokenId:                  nft.Id,
			DenomId:                  collection.Denom.Id,
			PayoutPeriodStart:        nftProcessResult.NftPeriodStart,
			PayoutPeriodEnd:          nftProcessResult.NftPeriodEnd,
			Reward:                   nftProcessResult.RewardForNftAfterFeeBtcDecimal,
			MaintenanceFee:           nftProcessResult.MaintenanceFeeBtcDecimal,
			CUDOPartOfMaintenanceFee: nftProcessResult.CudoPartOfMaintenanceFeeBtcDecimal,
			NFTOwnersForPeriod:       nftProcessResult.NftOwnersForPeriod,
		})

		log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, nftProcessResult.RewardForNftAfterFeeBtcDecimal)
		log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, nftProcessResult.MaintenanceFeeBtcDecimal)
		log.Debug().Msgf("CUDO part (%.2f) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", s.config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, nftProcessResult.CudoPartOfMaintenanceFeeBtcDecimal)

		// distribute maintenance fees
		addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, nftProcessResult.MaintenanceFeeBtcDecimal, farm.MaintenanceFeePayoutAddress)
		addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, nftProcessResult.CudoPartOfMaintenanceFeeBtcDecimal, s.config.CUDOFeePayoutAddress)
		distributeRewardsToOwners(nftProcessResult.AllNftOwnersForTimePeriodWithRewardPercent, nftProcessResult.RewardForNftAfterFeeBtcDecimal, destinationAddressesWithAmountBtcDecimal)
	}

	// calculate collection's percent of rewards based on hash power
	auraPoolCollection := farmAuraPoolCollectionsMap[collection.Denom.Id]
	collectionPartOfFarmDecimal := decimal.NewFromFloat(auraPoolCollection.HashingPower / currentHashPowerForFarm)

	collectionAwardAllocation := totalRewardForFarmAfterCudosFeeBtcDecimal.Mul(collectionPartOfFarmDecimal)
	cudoGeneralFeeForCollection := cudosFeeOfTotalRewardBtcDecimal.Mul(collectionPartOfFarmDecimal)
	CUDOMaintenanceFeeBtcDecimalForCollection := CUDOMaintenanceFeeBtcDecimal
	farmMaintenanceFeeBtcDecimalForCollection := farmMaintenanceFeeBtcDecimal
	farmLeftoverForCollection := collectionAwardAllocation.Sub(nftRewardsAfterFeesBtcDecimal).Sub(CUDOMaintenanceFeeBtcDecimalForCollection).Sub(farmMaintenanceFeeBtcDecimalForCollection)

	collectionPaymentAllocation := types.CollectionPaymentAllocation{
		FarmId:                     farm.Id,
		CollectionId:               auraPoolCollection.Id,
		CollectionAllocationAmount: collectionAwardAllocation,
		CUDOGeneralFee:             cudoGeneralFeeForCollection,
		CUDOMaintenanceFee:         CUDOMaintenanceFeeBtcDecimalForCollection,
		FarmUnsoldLeftovers:        farmLeftoverForCollection,
		FarmMaintenanceFee:         farmMaintenanceFeeBtcDecimalForCollection,
	}

	log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn from collection: %s, farm: {%s}, collection: {%d}", nftRewardsAfterFeesBtcDecimal, farmLeftoverForCollection, farm.RewardsFromPoolBtcWalletName, auraPoolCollection.Id)

	return CollectionProcessResult{
		CUDOMaintenanceFeeBtcDecimal:  CUDOMaintenanceFeeBtcDecimal,
		FarmMaintenanceFeeBtcDecimal:  farmMaintenanceFeeBtcDecimal,
		NftRewardsAfterFeesBtcDecimal: nftRewardsAfterFeesBtcDecimal,
		CollectionPaymentAllocation:   collectionPaymentAllocation,
		NftStatistics:                 nftStatistics,
	}, nil
}

func (s *PayService) processNft(
	ctx context.Context,
	storage Storage,
	farm types.Farm,
	collection types.Collection,
	nft types.NFT,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	rewardForNftOwnersBtcDecimal decimal.Decimal,
	mintedHashPowerForFarm float64,
	hourlyMaintenanceFeeInBtcDecimal decimal.Decimal,
	lastPaymentTimestamp int64,
	periodEnd int64,
) (NftProcessResult, bool, error) {
	nftTransferHistory, err := s.getNftTransferHistory(ctx, collection.Denom.Id, nft.Id)
	if err != nil {
		return NftProcessResult{}, false, err
	}

	nftPeriodStart, nftPeriodEnd, err := s.getNftTimestamps(ctx, storage, nft, nftTransferHistory, collection.Denom.Id, periodEnd)
	if err != nil {
		return NftProcessResult{}, false, err
	}

	// nft is minted after this payment period and hsould be skipped currently
	if nftPeriodStart > periodEnd {
		return NftProcessResult{}, false, nil
	}

	// first calculate nft parf ot the farm as percent of hash power
	totalRewardForNftBtcDecimal := calculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwnersBtcDecimal)

	// if nft was minted after the last payment, part of the reward before the mint is still for the farm
	rewardForNftBtcDecimal := calculatePercentByTime(lastPaymentTimestamp, periodEnd, nftPeriodStart, nftPeriodEnd, totalRewardForNftBtcDecimal)
	maintenanceFeeBtcDecimal, cudoPartOfMaintenanceFeeBtcDecimal, rewardForNftAfterFeeBtcDecimal := s.calculateMaintenanceFeeForNFT(
		nftPeriodStart,
		nftPeriodEnd,
		hourlyMaintenanceFeeInBtcDecimal,
		rewardForNftBtcDecimal,
	)

	allNftOwnersForTimePeriodWithRewardPercent, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
		ctx,
		nftTransferHistory,
		collection.Denom.Id,
		nft.Id,
		nftPeriodStart,
		nftPeriodEnd,
		nft.Owner,
		s.config.Network,
		rewardForNftAfterFeeBtcDecimal,
	)

	if err != nil {
		return NftProcessResult{}, false, err
	}

	return NftProcessResult{
		CudoPartOfMaintenanceFeeBtcDecimal:         cudoPartOfMaintenanceFeeBtcDecimal,
		MaintenanceFeeBtcDecimal:                   maintenanceFeeBtcDecimal,
		RewardForNftAfterFeeBtcDecimal:             rewardForNftAfterFeeBtcDecimal,
		AllNftOwnersForTimePeriodWithRewardPercent: allNftOwnersForTimePeriodWithRewardPercent,
		NftOwnersForPeriod:                         nftOwnersForPeriod,
		NftPeriodStart:                             nftPeriodStart,
		NftPeriodEnd:                               nftPeriodEnd,
	}, true, nil
}

func (s *PayService) sendRewards(
	ctx context.Context,
	storage Storage,
	farm types.Farm,
	unspentTxForFarm btcjson.ListUnspentResult,
	periodEnd int64,
	receivedRewardForFarmBtcDecimal, rewardForNftOwnersBtcDecimal, totalRewardForFarmAfterCudosFeeBtcDecimal decimal.Decimal,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	statistics []types.NFTStatistics,
	collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation,
) error {
	log.Debug().Msgf("Calculating leftover rewards...")
	// return to the farm owner whatever is left
	leftoverNftRewardDistribution, err := calculateLeftoverNftRewardDistribution(rewardForNftOwnersBtcDecimal, statistics)
	if err != nil {
		return err
	}

	if leftoverNftRewardDistribution.GreaterThan(decimal.Zero) {
		addLeftoverRewardToFarmOwner(destinationAddressesWithAmountBtcDecimal, leftoverNftRewardDistribution, farm.LeftoverRewardPayoutAddress)
	}

	if len(destinationAddressesWithAmountBtcDecimal) == 0 {
		return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.RewardsFromPoolBtcWalletName)
	}

	// check that all of the amount is distributed and no more than it
	log.Debug().Msgf("Checking if total amount given is the same as distributed...")
	if checkTotalAmountToDistribute(totalRewardForFarmAfterCudosFeeBtcDecimal, destinationAddressesWithAmountBtcDecimal) != nil {
		return err
	}

	log.Debug().Msgf("Removing addressed with zero reward from the send list...")
	removeAddressesWithZeroReward(destinationAddressesWithAmountBtcDecimal)

	log.Debug().Msgf("Filtering payments by payment threshold...")
	addressesWithThresholdToUpdateBtcDecimal, addressesWithAmountInfo, err := s.filterByPaymentThreshold(ctx, destinationAddressesWithAmountBtcDecimal, storage, farm.Id)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Destination addresses with amount for farm {%s}: {%s}", farm.RewardsFromPoolBtcWalletName, fmt.Sprint(destinationAddressesWithAmountBtcDecimal))
	addressesToSendBtc, err := convertAmountToBTC(addressesWithAmountInfo)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Addresses above threshold that will be sent for farm {%s}: {%s}", farm.RewardsFromPoolBtcWalletName, fmt.Sprint(addressesToSendBtc))

	txHash := ""
	if len(addressesToSendBtc) > 0 {
		if txHash, err = s.apiRequester.SendMany(ctx, addressesToSendBtc); err != nil {
			return err
		}
		log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash)
	}

	log.Debug().Msgf("Updating threshold statuses...")
	if err := storage.UpdateThresholdStatus(ctx, unspentTxForFarm.TxID, periodEnd, addressesWithThresholdToUpdateBtcDecimal, farm.Id); err != nil {
		log.Error().Msgf("Failed to update threshold for tx hash {%s}: %s", txHash, err)
		return err
	}

	log.Debug().Msgf("Saving statistics...")
	if err := storage.SaveStatistics(ctx, receivedRewardForFarmBtcDecimal, collectionPaymentAllocationsStatistics, addressesWithAmountInfo, statistics, txHash, farm.Id, farm.RewardsFromPoolBtcWalletName); err != nil {
		log.Error().Msgf("Failed to save statistics for tx hash {%s}: %s", txHash, err)
		return err
	}

	return nil
}
