package services

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewPayService(config *infrastructure.Config, apiRequester ApiRequester, helper Helper, btcNetworkParams *types.BtcNetworkParams) *PayService {
	return &PayService{
		config:           config,
		helper:           helper,
		btcNetworkParams: btcNetworkParams,
		apiRequester:     apiRequester,
	}
}

func (s *PayService) Execute(ctx context.Context, btcClient BtcClient, storage Storage) error {
	farms, err := storage.GetApprovedFarms(ctx)
	if err != nil {
		return err
	}

	for _, farm := range farms {
		if err := s.processFarm(ctx, btcClient, storage, farm); err != nil {
			log.Error().Msgf("processing farm {%s} failed. Error: %s", farm.SubAccountName, err)
			continue
		}
	}

	return nil
}

func (s *PayService) processFarm(ctx context.Context, btcClient BtcClient, storage Storage, farm types.Farm) error {
	log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
	err := validateFarm(farm)
	if err != nil {
		return err
	}

	_, err = btcClient.LoadWallet(farm.SubAccountName)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Farm Wallet: {%s} loaded", farm.SubAccountName)

	defer func() {
		if err := btcClient.UnloadWallet(&farm.SubAccountName); err != nil {
			log.Error().Msgf("Failed to unload wallet %s: %s", farm.SubAccountName, err)
			return
		}

		log.Debug().Msgf("Farm Wallet: {%s} unloaded", farm.SubAccountName)
	}()

	unspentTxsForFarm, err := s.getUnspentTxsForFarm(ctx, btcClient, storage, []string{farm.AddressForReceivingRewardsFromPool})
	if err != nil {
		return err
	}

	if len(unspentTxsForFarm) == 0 {
		log.Info().Msgf("no unspent TXs for farm {{%s}}....skipping this farm", farm.SubAccountName)
		return nil
	}

	// get previoud tx timestamp
	var lastPaymentTimestamp int64
	lastUTXOTransaction, err := storage.GetLastUTXOTransactionByFarmId(ctx, farm.Id)
	if err != nil {
		return err
	}

	// no payment found, so this is the first one. Get the one from foundry
	if lastUTXOTransaction.PaymentTimestamp == 0 {
		if s.config.IsTesting {
			lastPaymentTimestamp = 0
		} else {
			timefarmStarted, err := s.apiRequester.GetFarmStartTime(ctx, farm.SubAccountName)
			if err != nil {
				return nil
			}
			lastPaymentTimestamp = timefarmStarted
		}
	} else {
		lastPaymentTimestamp = lastUTXOTransaction.PaymentTimestamp
	}

	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return err
	}

	defer func() {
		if err := btcClient.WalletLock(); err != nil {
			log.Error().Msgf("Failed to lock wallet %s: %s", farm.SubAccountName, err)
			return
		}

		log.Debug().Msgf("Farm Wallet: {%s} locked", farm.SubAccountName)
	}()

	// for each payment
	for _, unspentTxForFarm := range unspentTxsForFarm {
		txRawResult, err := s.getUnspentTxDetails(ctx, btcClient, unspentTxForFarm)
		if err != nil {
			return err
		}

		// period end is the time the payment was made
		periodEnd := txRawResult.Time

		receivedRewardForFarmBtcDecimal := decimal.NewFromFloat(unspentTxForFarm.Amount)
		totalRewardForFarmAfterCudosFeeBtcDecimal, cudosFeeOfTotalRewardBtcDecimal := s.calculateCudosFeeOfTotalFarmIncome(receivedRewardForFarmBtcDecimal)

		log.Debug().Msgf("-------------------------------------------------")
		log.Debug().Msgf("Processing Unspent TX: %s, Payment period: %d to %d", unspentTxForFarm.TxID, lastPaymentTimestamp, periodEnd)
		log.Debug().Msgf("Total reward for farm \"%s\": %s", farm.SubAccountName, receivedRewardForFarmBtcDecimal)
		log.Debug().Msgf("Cudos part of total farm reward: %s", cudosFeeOfTotalRewardBtcDecimal)
		log.Debug().Msgf("Total reward for farm \"%s\" after cudos fee: %s", farm.SubAccountName, totalRewardForFarmAfterCudosFeeBtcDecimal)

		collections, err := s.apiRequester.GetFarmCollectionsFromHasura(ctx, farm.Id)
		if err != nil {
			return err
		}

		// currently always getting the hash power as set in the farm entity in the Aura Pool Service
		// that way the rewards will be sent proportionally, no matter the current hash power of the farm
		// otherwise there is a case where the farm hash power falls below the registered and the calculations fail
		// if this case is to be handled, it need more work
		currentHashPowerForFarm := farm.TotalHashPower
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

		log.Debug().Msgf("Total hash power for farm %s: %.6f", farm.SubAccountName, currentHashPowerForFarm)

		hourlyMaintenanceFeeInBtcDecimal := s.calculateHourlyMaintenanceFee(farm, currentHashPowerForFarm)

		verifiedDenomIds, err := s.verifyCollectionIds(ctx, collections)
		if err != nil {
			return err
		}

		if len(verifiedDenomIds) == 0 {
			log.Error().Msgf("no verified colletions for farm {%s}", farm.SubAccountName)
			return nil
		}

		log.Debug().Msgf("Verified collections for farm %s: %s", farm.SubAccountName, fmt.Sprintf("%v", verifiedDenomIds))

		farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionsWithNFTs(ctx, verifiedDenomIds)
		if err != nil {
			return err
		}

		farmAuraPoolCollections, err := storage.GetFarmAuraPoolCollections(ctx, farm.Id)
		if err != nil {
			return err
		}

		// make a map for faster getting
		farmAuraPoolCollectionsMap := map[string]types.AuraPoolCollection{}
		for _, farmAuraPoolCollection := range farmAuraPoolCollections {
			farmAuraPoolCollectionsMap[farmAuraPoolCollection.DenomId] = farmAuraPoolCollection
		}

		nonExpiredNFTsCount := s.filterExpiredBeforePeriodNFTs(farmCollectionsWithNFTs, lastPaymentTimestamp)
		log.Debug().Msgf("Non expired NFTs count: %d", nonExpiredNFTsCount)

		if nonExpiredNFTsCount == 0 {
			log.Error().Msgf("all nfts for farm {%s} are expired", farm.SubAccountName)
			return nil
		}

		mintedHashPowerForFarm := sumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)

		log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.SubAccountName, mintedHashPowerForFarm)

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
		log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn: %s, farm: {%s}", rewardForNftOwnersBtcDecimal, rewardToReturnBtcDecimal, farm.SubAccountName)

		var statistics []types.NFTStatistics
		var collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation

		for _, collection := range farmCollectionsWithNFTs {
			_, ok := farmAuraPoolCollectionsMap[collection.Denom.Id]
			if !ok {
				log.Warn().Msgf("aura pool collection not found by denom id {%s}", collection.Denom.Id)
				continue
			}

			auraPoolCollection := farmAuraPoolCollectionsMap[collection.Denom.Id]
			log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)

			var CUDOMaintenanceFeeBtcDecimal decimal.Decimal
			var farmMaintenanceFeeBtcDecimal decimal.Decimal
			var nftRewardsAfterFeesBtcDecimal decimal.Decimal

			for _, nft := range collection.Nfts {
				nftTransferHistory, err := s.getNftTransferHistory(ctx, collection.Denom.Id, nft.Id)
				if err != nil {
					return err
				}

				nftPeriodStart, nftPeriodEnd, err := s.getNftTimestamps(ctx, storage, nft, nftTransferHistory, collection.Denom.Id, periodEnd)
				if err != nil {
					return err
				}

				// nft is minted after this payment period and hsould be skipped currently
				if nftPeriodStart > periodEnd {
					continue
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

				// distribute maintenance fees
				addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, maintenanceFeeBtcDecimal, farm.MaintenanceFeePayoutAddress)
				addPaymentAmountToAddress(destinationAddressesWithAmountBtcDecimal, cudoPartOfMaintenanceFeeBtcDecimal, s.config.CUDOFeePayoutAddress)

				CUDOMaintenanceFeeBtcDecimal = CUDOMaintenanceFeeBtcDecimal.Add(cudoPartOfMaintenanceFeeBtcDecimal)
				farmMaintenanceFeeBtcDecimal = farmMaintenanceFeeBtcDecimal.Add(maintenanceFeeBtcDecimal)
				nftRewardsAfterFeesBtcDecimal = nftRewardsAfterFeesBtcDecimal.Add(rewardForNftAfterFeeBtcDecimal)

				allNftOwnersForTimePeriodWithRewardPercent, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
					ctx, nftTransferHistory, collection.Denom.Id, nft.Id, nftPeriodStart, nftPeriodEnd, nft.Owner, s.config.Network, rewardForNftAfterFeeBtcDecimal)
				if err != nil {
					return err
				}

				statistics = append(statistics, types.NFTStatistics{
					TokenId:                  nft.Id,
					DenomId:                  collection.Denom.Id,
					PayoutPeriodStart:        nftPeriodStart,
					PayoutPeriodEnd:          nftPeriodEnd,
					Reward:                   rewardForNftAfterFeeBtcDecimal,
					MaintenanceFee:           maintenanceFeeBtcDecimal,
					CUDOPartOfMaintenanceFee: cudoPartOfMaintenanceFeeBtcDecimal,
					NFTOwnersForPeriod:       nftOwnersForPeriod,
				})

				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNftAfterFeeBtcDecimal)
				log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, maintenanceFeeBtcDecimal)
				log.Debug().Msgf("CUDO part (%.2f) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", s.config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, cudoPartOfMaintenanceFeeBtcDecimal)

				distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNftAfterFeeBtcDecimal, destinationAddressesWithAmountBtcDecimal)
			}

			// calculate collection's percent of rewards based on hash power
			collectionPartOfFarmDecimal := decimal.NewFromFloat(auraPoolCollection.HashingPower / currentHashPowerForFarm)

			collectionAwardAllocation := totalRewardForFarmAfterCudosFeeBtcDecimal.Mul(collectionPartOfFarmDecimal)
			cudoGeneralFeeForCollection := cudosFeeOfTotalRewardBtcDecimal.Mul(collectionPartOfFarmDecimal)
			CUDOMaintenanceFeeBtcDecimalForCollection := CUDOMaintenanceFeeBtcDecimal
			farmMaintenanceFeeBtcDecimalForCollection := farmMaintenanceFeeBtcDecimal
			farmLeftoverForCollection := collectionAwardAllocation.Sub(nftRewardsAfterFeesBtcDecimal).Sub(CUDOMaintenanceFeeBtcDecimalForCollection).Sub(farmMaintenanceFeeBtcDecimalForCollection)

			var collectionPaymentAllocation types.CollectionPaymentAllocation
			collectionPaymentAllocation.FarmId = farm.Id
			collectionPaymentAllocation.CollectionId = auraPoolCollection.Id
			collectionPaymentAllocation.CollectionAllocationAmount = collectionAwardAllocation
			collectionPaymentAllocation.CUDOGeneralFee = cudoGeneralFeeForCollection
			collectionPaymentAllocation.CUDOMaintenanceFee = CUDOMaintenanceFeeBtcDecimalForCollection
			collectionPaymentAllocation.FarmUnsoldLeftovers = farmLeftoverForCollection
			collectionPaymentAllocation.FarmMaintenanceFee = farmMaintenanceFeeBtcDecimalForCollection

			collectionPaymentAllocationsStatistics = append(collectionPaymentAllocationsStatistics, collectionPaymentAllocation)

			log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn from collection: %s, farm: {%s}, collection: {%d}", nftRewardsAfterFeesBtcDecimal, farmLeftoverForCollection, farm.SubAccountName, collectionPaymentAllocation.CollectionId)
		}

		// return to the farm owner whatever is left
		var distributedNftRewards decimal.Decimal
		for _, nftStat := range statistics {
			distributedNftRewards = distributedNftRewards.Add(nftStat.Reward).Add(nftStat.MaintenanceFee).Add(nftStat.CUDOPartOfMaintenanceFee)
		}

		leftoverNftRewardDistribution := rewardForNftOwnersBtcDecimal.Sub(distributedNftRewards)

		if leftoverNftRewardDistribution.LessThan(decimal.Zero) {
			return fmt.Errorf("distributed NFT awards bigger than the farm reward after cudos fee. NftRewardDistribution: %s, TotalFarmRewardAfterCudosFee: %s", distributedNftRewards, rewardForNftOwnersBtcDecimal)
		}

		if leftoverNftRewardDistribution.GreaterThan(decimal.Zero) {
			addLeftoverRewardToFarmOwner(destinationAddressesWithAmountBtcDecimal, leftoverNftRewardDistribution, farm.LeftoverRewardPayoutAddress)
		}

		if len(destinationAddressesWithAmountBtcDecimal) == 0 {
			return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.SubAccountName)
		}

		var totalAmountToPayToAddressesBtcDecimal decimal.Decimal

		for _, amount := range destinationAddressesWithAmountBtcDecimal {
			totalAmountToPayToAddressesBtcDecimal = totalAmountToPayToAddressesBtcDecimal.Add(amount)
		}

		// check that all of the amount is distributed and no more than it
		if !totalAmountToPayToAddressesBtcDecimal.Equals(receivedRewardForFarmBtcDecimal) {
			return fmt.Errorf("distributed amount doesn't equal total farm rewards. Distributed amount: {%s}, TotalFarmReward: {%s}", totalAmountToPayToAddressesBtcDecimal, receivedRewardForFarmBtcDecimal)
		}

		removeAddressesWithZeroReward(destinationAddressesWithAmountBtcDecimal)

		addressesWithThresholdToUpdateBtcDecimal, addressesWithAmountInfo, err := s.filterByPaymentThreshold(ctx, destinationAddressesWithAmountBtcDecimal, storage, farm.Id)
		if err != nil {
			return err
		}

		log.Debug().Msgf("Destination addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmountBtcDecimal))

		addressesToSendBtc, err := convertAmountToBTC(addressesWithAmountInfo)
		if err != nil {
			return err
		}

		log.Debug().Msgf("Addresses above threshold that will be sent for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(addressesToSendBtc))

		txHash := ""
		if len(addressesToSendBtc) > 0 {
			txHash, err = s.apiRequester.SendMany(ctx, addressesToSendBtc)
			if err != nil {
				return err
			}
			log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash)
		}

		err = storage.UpdateThresholdStatus(ctx, unspentTxForFarm.TxID, periodEnd, addressesWithThresholdToUpdateBtcDecimal, farm.Id)
		if err != nil {
			log.Error().Msgf("Failed to update threshold for tx hash {%s}: %s", txHash, err)
			return err
		}

		if err := storage.SaveStatistics(ctx, receivedRewardForFarmBtcDecimal, collectionPaymentAllocationsStatistics, addressesWithAmountInfo, statistics, txHash, farm.Id, farm.SubAccountName); err != nil {
			log.Error().Msgf("Failed to save statistics for tx hash {%s}: %s", txHash, err)
			return err
		}
		lastPaymentTimestamp = periodEnd
	}

	return nil
}

func validateFarm(farm types.Farm) error {
	if farm.SubAccountName == "" {
		return fmt.Errorf("farm has empty Sub Account Name. Farm Id: {%d}", farm.Id)
	}

	i := farm.MaintenanceFeeInBtc
	if i <= 0 {
		return fmt.Errorf("farm has maintenance fee set below 0. Farm Id: {%d}", farm.Id)
	}

	if farm.AddressForReceivingRewardsFromPool == "" {
		return fmt.Errorf("farm has no AddressForReceivingRewardsFromPool, farm Id: {%d}", farm.Id)
	}
	if farm.MaintenanceFeePayoutAddress == "" {
		return fmt.Errorf("farm has no MaintenanceFeePayoutAddress, farm Id: {%d}", farm.Id)
	}
	if farm.LeftoverRewardPayoutAddress == "" {
		return fmt.Errorf("farm has no LeftoverRewardPayoutAddress, farm Id: {%d}", farm.Id)
	}

	return nil
}

type PayService struct {
	config           *infrastructure.Config
	helper           Helper
	btcNetworkParams *types.BtcNetworkParams
	apiRequester     ApiRequester
}

type ApiRequester interface {
	GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error)

	GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error)

	GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error)

	GetFarmStartTime(ctx context.Context, farmName string) (int64, error)

	GetFarmCollectionsFromHasura(ctx context.Context, farmId int64) (types.CollectionData, error)

	VerifyCollection(ctx context.Context, denomId string) (bool, error)

	GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error)

	SendMany(ctx context.Context, destinationAddressesWithAmount map[string]float64) (string, error)

	BumpFee(ctx context.Context, txId string) (string, error)
}

type Provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}

type BtcClient interface {
	LoadWallet(walletName string) (*btcjson.LoadWalletResult, error)

	UnloadWallet(walletName *string) error

	WalletPassphrase(passphrase string, timeoutSecs int64) error

	WalletLock() error

	GetRawTransactionVerbose(txHash *chainhash.Hash) (*btcjson.TxRawResult, error)

	ListUnspent() ([]btcjson.ListUnspentResult, error)
}

type Storage interface {
	GetApprovedFarms(ctx context.Context) ([]types.Farm, error)

	GetPayoutTimesForNFT(ctx context.Context, collectionDenomId, nftId string) ([]types.NFTStatistics, error)

	SaveStatistics(ctx context.Context, receivedRewardForFarmBtcDecimal decimal.Decimal, collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash string, farmId int64, farmSubAccountName string) error

	GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error)

	UpdateTransactionsStatus(ctx context.Context, txHashesToMarkCompleted []string, status string) error

	SaveTxHashWithStatus(ctx context.Context, txHash, status, farmSubAccountName string, retryCount int) error

	SaveRBFTransactionInformation(ctx context.Context, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName string, retryCount int) error

	GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error)

	GetLastUTXOTransactionByFarmId(ctx context.Context, farmId int64) (types.UTXOTransaction, error)

	GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int64) (decimal.Decimal, error)

	UpdateThresholdStatus(ctx context.Context, processedTransactions string, paymentTimestamp int64, addressesWithThresholdToUpdateBtcDecimal map[string]decimal.Decimal, farmId int64) error

	SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount int) error

	GetFarmAuraPoolCollections(ctx context.Context, farmId int64) ([]types.AuraPoolCollection, error)
}

type Helper interface {
	DaysIn(m time.Month, year int) int
	Unix() int64
	Date() (year int, month time.Month, day int)
	SendMail(message string, to []string) error
}
