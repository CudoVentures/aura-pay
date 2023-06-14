package services

import (
	"context"
	"encoding/json"
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

// Processes all approved farms by iterating through them and calling the processFarm function for each farm.
// In case of an error while processing a farm,
// the function logs the error message
// and sends an email notification (limited to once per half hour) to inform about the failure.
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

/*
processFarm function processes a single farm by performing a series of steps:

1. Validate the farm.
2. Load the farm wallet.
3. Get unspent transactions for the farm wallet.
4. Get the last payment timestamp for the farm.
5. Unlock the farm wallet.
6. Process each unspent transaction for the farm.
7. Lock the farm wallet after processing.
*/
func (s *PayService) processFarm(ctx context.Context, btcClient BtcClient, storage Storage, farm types.Farm) error {
	log.Debug().Msgf("Processing farm with name %s..", farm.RewardsFromPoolBtcWalletName)
	err := validateFarm(farm)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Check for loaded wallets...")
	rawMessage, err := btcClient.RawRequest("listwallets", []json.RawMessage{})
	if err != nil {
		return err
	}

	loadedWalletsNames := []string{}
	err = json.Unmarshal(rawMessage, &loadedWalletsNames)
	if err != nil {
		return err
	}

	if len(loadedWalletsNames) > 0 {
		log.Debug().Msgf("Loaded wallets found. Unloading...")
		for _, loadedWalletName := range loadedWalletsNames {
			unloadWallet(btcClient, loadedWalletName)
		}
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

/*
This function processes an unspent transaction for a specific farm
and calculates the reward distribution for NFT owners, the farm owner, and the Cudos fee.
It processes each collection within the farm and sends the rewards accordingly.

 1. Retrieve the details of an unspent transaction using getUnspentTxDetails().
    Needed to get the timestamp of the transaction.
 2. Calculate the period end, total reward for the farm, Cudos fee of total farm income, and total reward after Cudos fee.
 3. Compute the total hash power for the farm and the hourly maintenance fee based on the farm's hash power.
 4. Get verified collections and their minted NFTs for the farm.
 5. Filter out expired NFTs.
 6. Compute the minted hash power for the farm by summing up the minted hash power
    of all the minted nfts in all the collections.
 7. Calculate the reward for NFT owners and the leftovers by minted hash power.
 8. Initialize a map to store destination addresses with their corresponding amounts in Bitcoin.
 9. Add the Cudos fee on total farm income and the leftover reward to the farm owner to the map.
 10. Loop through all collections and process each collection and their minted nfts.
    Update the map of destination addresses with amounts and append the statistics to their respective variables.
 11. Send the rewards and update the statistics.

Return the period end so it can be used as period start in the next transaction if there is any.
*/
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

	currentHashPowerForFarm := farm.TotalHashPower
	log.Debug().Msgf("Total hash power for farm %s: %.6f", farm.RewardsFromPoolBtcWalletName, currentHashPowerForFarm)
	hourlyMaintenanceFeeInBtcDecimal := s.calculateHourlyMaintenanceFee(farm, currentHashPowerForFarm)

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

	rewardForNftOwnersBtcDecimal := calculateRewardByPercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarmAfterCudosFeeBtcDecimal)
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
	if err := s.sendRewards(
		ctx,
		btcClient,
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
	); err != nil {
		return 0, err
	}

	log.Debug().Msgf("Sends completed...")

	return periodEnd, nil
}

/*
Processes a collection within a farm, calculates reward and maintenance fee allocations for each NFT,
and distributes these amounts to their respective addresses.
It returns a CollectionProcessResult object containing relevant statistics and payment allocation information.

 1. Loop through each NFT in the collection and process the NFT. If the NFT is processed successfully,
    update the maintenance fee and rewards variables, and append the NFT's statistics to the array.
 2. Distribute maintenance fees to the farm and CUDO fee payout addresses, and distribute rewards to the NFT owners.
 3. Calculate the collection's percentage of rewards based on its hash power.
 4. Compute the collection award allocation, CUDO general fee for the collection,
    CUDO maintenance fee for the collection, farm maintenance fee for the collection, and farm leftover for the collection.
 5. Create a CollectionPaymentAllocation object to store information about the collection's payment allocations.
 6. Return a CollectionProcessResult object containing CUDO maintenance fee, farm maintenance fee,
    NFT rewards after fees, collection payment allocation, and NFT statistics.
*/
func (s *PayService) processCollection(
	ctx context.Context,
	storage Storage,
	farm types.Farm,
	collection types.Collection,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	rewardForNftOwnersBtcDecimal decimal.Decimal,
	mintedHashPowerForFarm, currentHashPowerForFarm float64,
	totalRewardForFarmAfterCudosFeeBtcDecimal, cudosFeeOfTotalRewardBtcDecimal, hourlyMaintenanceFeeInBtcDecimal decimal.Decimal,
	periodStart, periodEnd int64,
	farmAuraPoolCollectionsMap map[string]types.AuraPoolCollection,
) (CollectionProcessResult, error) {
	log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)
	log.Debug().Msgf("Getting collection transfer events..")
	nftTransferEvents, err := s.apiRequester.GetDenomNftTransferHistory(ctx, collection.Denom.Id, periodStart, periodEnd)

	if err != nil {
		return CollectionProcessResult{}, err
	}
	log.Debug().Msgf("Done!")

	nftTransferEventsMap := make(map[string][]types.NftTransferEvent)
	for _, nftTransferEvent := range nftTransferEvents {
		nftTransferEventsMap[nftTransferEvent.TokenId] = append(nftTransferEventsMap[nftTransferEvent.TokenId], nftTransferEvent)
	}
	log.Debug().Msgf("Getting collection mint history from BDJuno..")
	hasuraNftMintHistory, err := s.apiRequester.GetHasuraCollectionNftMintEvents(ctx, collection.Denom.Id)
	if err != nil {
		return CollectionProcessResult{}, err
	}
	log.Debug().Msgf("Done!")

	ahsuraNftMintEventsMap := make(map[string]types.HasuraNftMintEvent)
	for _, hasuraNftMintEvent := range hasuraNftMintHistory.Data.History {
		ahsuraNftMintEventsMap[fmt.Sprint(hasuraNftMintEvent.TokenId)] = hasuraNftMintEvent
	}

	var CUDOMaintenanceFeeBtcDecimal decimal.Decimal
	var farmMaintenanceFeeBtcDecimal decimal.Decimal
	var nftRewardsAfterFeesBtcDecimal decimal.Decimal

	var nftStatistics []types.NFTStatistics

	for _, nft := range collection.Nfts {
		nftHasuraMintEvent, ok := ahsuraNftMintEventsMap[nft.Id]
		var mintTimestamp int64
		if !ok {
			log.Debug().Msgf("Mint event for NFT with id {{%s}} was not foun in BDJuno. Getting it from chain..", nft.Id)
			chainMintEventTimestamp, err := s.apiRequester.GetChainNftMintTimestamp(ctx, collection.Denom.Id, nft.Id)
			if err != nil {
				return CollectionProcessResult{}, err
			}
			log.Debug().Msgf("Done!")
			mintTimestamp = chainMintEventTimestamp
		} else {
			mintTimestamp = nftHasuraMintEvent.Timestamp
		}

		nftProcessResult, processed, err := s.processNft(
			ctx,
			storage,
			farm,
			collection,
			nft,
			mintTimestamp,
			nftTransferEventsMap[nft.Id],
			destinationAddressesWithAmountBtcDecimal,
			rewardForNftOwnersBtcDecimal,
			mintedHashPowerForFarm,
			hourlyMaintenanceFeeInBtcDecimal,
			periodStart,
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

/*
Processes an NFT within a collection, calculates its reward and maintenance fee,
and returns an NftProcessResult object containing relevant statistics and payment allocation information.

 1. Retrieve the NFT transfer history for the given NFT from BDJuno.
 2. Determine the NFT's period start and end times based on its transfer history and mint timestamp.
    Period start should be the lower of the last payment to that nft or the mint time.
    Period end should be the lower of the current payment timestamp and the nft expiration timestamp.
 3. If the NFT's period start time is after the payment period end, skip the current NFT as it doesn't need to be processed.
 4. Calculate the total reward for the NFT based on its percentage of hash power within the farm.
 5. Adjust the reward for the NFT based on its mint time. This step ensures that if the NFT was minted after the last payment,
    only the part of the reward after the mint is considered for the NFT.
 6. Calculate the maintenance fee, CUDO's part of the maintenance fee, and the reward for the NFT after fees.
 7. Calculate the reward percentages for each owner during the NFT's period.
    This step takes into account the NFT's transfer history and calculates the rewards based on the ownership duration.
 8. Return an NftProcessResult object containing CUDO's part of the maintenance fee, the maintenance fee,
    the reward for the NFT after fees, the reward percentages for all owners during the period,
    the owners for the period, and the NFT's period start and end times.
*/
func (s *PayService) processNft(
	ctx context.Context,
	storage Storage,
	farm types.Farm,
	collection types.Collection,
	nft types.NFT,
	mintTimestamp int64,
	nftTransferHistory []types.NftTransferEvent,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	rewardForNftOwnersBtcDecimal decimal.Decimal,
	mintedHashPowerForFarm float64,
	hourlyMaintenanceFeeInBtcDecimal decimal.Decimal,
	lastPaymentTimestamp int64,
	periodEnd int64,
) (NftProcessResult, bool, error) {
	nftPeriodStart, nftPeriodEnd, err := s.getNftTimestamps(ctx, storage, nft, mintTimestamp, nftTransferHistory, collection.Denom.Id, periodEnd)
	if err != nil {
		return NftProcessResult{}, false, err
	}

	// nft is minted after this payment period and hsould be skipped currently
	if nftPeriodStart > periodEnd {
		return NftProcessResult{}, false, nil
	}

	// first calculate nft parf ot the farm as percent of hash power
	totalRewardForNftBtcDecimal := calculateRewardByPercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwnersBtcDecimal)
	// if nft was minted after the last payment, part of the reward before the mint is still for the farm
	rewardForNftBtcDecimal := calculatePercentByTime(lastPaymentTimestamp, periodEnd, nftPeriodStart, nftPeriodEnd, totalRewardForNftBtcDecimal)

	maintenanceFeeBtcDecimal, cudoPartOfMaintenanceFeeBtcDecimal, rewardForNftAfterFeeBtcDecimal, err := s.calculateMaintenanceFeeForNFT(
		nftPeriodStart,
		nftPeriodEnd,
		hourlyMaintenanceFeeInBtcDecimal,
		rewardForNftBtcDecimal,
	)

	if err != nil {
		return NftProcessResult{}, false, err
	}

	ownersCudosAddressWithPercentOwnedTime, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
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
		AllNftOwnersForTimePeriodWithRewardPercent: ownersCudosAddressWithPercentOwnedTime,
		NftOwnersForPeriod:                         nftOwnersForPeriod,
		NftPeriodStart:                             nftPeriodStart,
		NftPeriodEnd:                               nftPeriodEnd,
	}, true, nil
}

/*
This function is responsible for distributing rewards to the destination addresses,
ensuring that the total reward amount is correctly distributed,
and filtering addresses based on the payment threshold.
Additionally, it updates the threshold status for each address and saves the reward statistics.

 1. Calculate any leftover rewards that were not distributed to NFT owners.
 2. If there are any leftover rewards, add them to the farm owner's payout address.
 3. Check if there are any addresses in the destinationAddressesWithAmountBtcDecimal map to pay rewards.
    If not, return an error, since this is not a valid case.
    At least the farms address should be present in the map.
    TODO: Is there a case where the total payed amount to the farm is under the threshold?
 4. Verify that the total amount to be distributed is equal to the amount of rewards distributed.
    If there's a mismatch, return an error.
 5. Remove any addresses with zero rewards from the destination addresses list.
 6. Filter the payments based on the payment threshold.
    Update the addresses that have reached the payment threshold.
 7. Convert the reward amounts to floats with 8 decimals (BTC type).
 8. Send the rewards to the destination addresses.
    If the transaction is successful, store the transaction hash.
 9. Update the threshold statuses for the addresses.
 10. Save the statistics for the rewards, NFT allocations, and payment allocations.
*/
func (s *PayService) sendRewards(
	ctx context.Context,
	btcClient BtcClient,
	storage Storage,
	farm types.Farm,
	unspentTxForFarm btcjson.ListUnspentResult,
	periodEnd int64,
	receivedRewardForFarmBtcDecimal, rewardForNftOwnersBtcDecimal, totalRewardForFarmAfterCudosFeeBtcDecimal decimal.Decimal,
	destinationAddressesWithAmountBtcDecimal map[string]decimal.Decimal,
	statistics []types.NFTStatistics,
	collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation,
) error {
	// distributing nft owners rewards
	for _, nftStatistics := range statistics {
		// distribute maintenance fees
		addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, nftStatistics.MaintenanceFee, farm.MaintenanceFeePayoutAddress)
		addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, nftStatistics.CUDOPartOfMaintenanceFee, s.config.CUDOMaintenanceFeePayoutAddress)

		// add cudos addresses to payout addresses so they can be saved in the db with the cudos address
		// on send the btc address will be gotten if possible
		for _, nftOwnersForPeriod := range nftStatistics.NFTOwnersForPeriod {
			addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, nftOwnersForPeriod.Reward, nftOwnersForPeriod.Owner)
		}

	}

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
	if err := checkTotalAmountToDistribute(receivedRewardForFarmBtcDecimal, destinationAddressesWithAmountBtcDecimal); err != nil {
		return err
	}

	removeAddressesWithZeroReward(destinationAddressesWithAmountBtcDecimal)

	log.Debug().Msgf("Filtering payments by payment threshold...")
	addressesWithThresholdToUpdateBtcDecimal, addressesWithAmountInfo, cudosBtcAddressMap, err := s.filterByPaymentThreshold(ctx, destinationAddressesWithAmountBtcDecimal, storage, farm.Id)
	if err != nil {
		return err
	}

	// add btc addresses to owner infos
	for i := 0; i < len(statistics); i++ {
		for j := 0; j < len(statistics[i].NFTOwnersForPeriod); j++ {
			if btcAddress, ok := cudosBtcAddressMap[statistics[i].NFTOwnersForPeriod[j].Owner]; ok {
				statistics[i].NFTOwnersForPeriod[j].PayoutAddress = btcAddress
			}
		}
	}

	log.Debug().Msgf("Destination addresses with amount for farm {%s}: {%s}", farm.RewardsFromPoolBtcWalletName, fmt.Sprint(destinationAddressesWithAmountBtcDecimal))
	addressesToSendBtc, err := convertAmountToBTC(addressesWithAmountInfo)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Addresses above threshold that will be sent for farm {%s}: {%s}", farm.RewardsFromPoolBtcWalletName, fmt.Sprint(addressesToSendBtc))

	// check if total amount to send plus leftovers equals account balance
	walletBalance, err := btcClient.GetBalance("*")
	if err != nil {
		return err
	}

	var totalBalanceDistributed decimal.Decimal
	for _, amount := range addressesToSendBtc {
		totalBalanceDistributed = totalBalanceDistributed.Add(decimal.NewFromFloat(amount))
	}
	for _, amount := range addressesWithThresholdToUpdateBtcDecimal {
		totalBalanceDistributed = totalBalanceDistributed.Add(amount)
	}

	if !totalBalanceDistributed.LessThanOrEqual(decimal.NewFromFloat(walletBalance.ToBTC())) {
		return fmt.Errorf("total balance distributed {%s} is not equal to wallet balance {%s}", totalBalanceDistributed, walletBalance)
	}

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
