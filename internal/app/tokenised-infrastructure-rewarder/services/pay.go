package services

import (
	"context"
	"fmt"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
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

	totalRewardForFarm, transactionIdsToMarkProcessed, err := s.getTotalRewardForFarm(
		ctx,
		btcClient,
		storage,
		[]string{farm.AddressForReceivingRewardsFromPool})
	if err != nil {
		return err
	}

	if totalRewardForFarm == 0 {
		log.Info().Msgf("reward for farm {{%s}} is 0....skipping this farm", farm.SubAccountName)
		return nil
	}

	var farmPaymentStatistics types.FarmPayment
	farmPaymentStatistics.FarmId = farm.Id
	farmPaymentStatistics.AmountBTC = totalRewardForFarm

	totalRewardForFarmAfterCudosFee, cudosFeeOfTotalReward := s.calculateCudosFeeOfTotalFarmIncome(totalRewardForFarm)

	log.Debug().Msgf("Total reward for farm \"%s\": %s", farm.SubAccountName, totalRewardForFarm)
	log.Debug().Msgf("Cudos part of total farm reward: %s", cudosFeeOfTotalReward)
	log.Debug().Msgf("Total reward for farm \"%s\" after cudos fee: %s", farm.SubAccountName, totalRewardForFarmAfterCudosFee)

	collections, err := s.apiRequester.GetFarmCollectionsFromHasura(ctx, farm.Id)
	if err != nil {
		return err
	}

	var currentHashPowerForFarm float64
	if s.config.IsTesting {
		currentHashPowerForFarm = 1200 // hardcoded for testing & QA
	} else {
		// // used to get current hash from FOUNDRY //
		// currentHashPowerForFarm, err = s.apiRequester.GetFarmTotalHashPowerFromPoolToday(ctx, farm.SubAccountName,
		// 	time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
		// if err != nil {
		// 	return err
		// }

		// if currentHashPowerForFarm <= 0 {
		// 	return fmt.Errorf("invalid hash power (%f) for farm (%s)", currentHashPowerForFarm, farm.SubAccountName)
		// }
		currentHashPowerForFarm = farm.TotalHashPower
	}

	log.Debug().Msgf("Total hash power for farm %s: %.6f", farm.SubAccountName, currentHashPowerForFarm)

	hourlyMaintenanceFeeInSatoshis, err := s.calculateHourlyMaintenanceFee(farm, currentHashPowerForFarm)
	if err != nil {
		return err
	}

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

	nonExpiredNFTsCount := s.filterExpiredNFTs(farmCollectionsWithNFTs)
	log.Debug().Msgf("Non expired NFTs count: %d", nonExpiredNFTsCount)

	if nonExpiredNFTsCount == 0 {
		log.Error().Msgf("all nfts for farm {%s} are expired", farm.SubAccountName)
		return nil
	}

	mintedHashPowerForFarm := sumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)

	log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.SubAccountName, mintedHashPowerForFarm)

	rewardForNftOwners := calculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarmAfterCudosFee)
	leftoverHashPower := currentHashPowerForFarm - mintedHashPowerForFarm // if hash power increased or not all of it is used as NFTs
	var rewardToReturn btcutil.Amount

	destinationAddressesWithAmount := make(map[string]btcutil.Amount)

	// add cudos fee on total farm income
	addPaymentAmountToAddress(destinationAddressesWithAmount, cudosFeeOfTotalReward, s.config.CUDOFeePayoutAddress)

	// return to the farm owner whatever is left
	if leftoverHashPower > 0 {
		rewardToReturn = totalRewardForFarmAfterCudosFee - rewardForNftOwners
		addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, rewardToReturn, farm.LeftoverRewardPayoutAddress)
	}
	log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn: %s, farm: {%s}", rewardForNftOwners, rewardToReturn, farm.SubAccountName)

	var statistics []types.NFTStatistics
	var collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation

	for _, collection := range farmCollectionsWithNFTs {
		log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)

		var CUDOMaintenanceFee btcutil.Amount
		var farmMaintenanceFee btcutil.Amount
		var nftRewardsAfterFees btcutil.Amount

		for _, nft := range collection.Nfts {
			var nftStatistics types.NFTStatistics
			nftStatistics.TokenId = nft.Id
			nftStatistics.DenomId = collection.Denom.Id

			nftTransferHistory, err := s.getNftTransferHistory(ctx, collection.Denom.Id, nft.Id)
			if err != nil {
				return err
			}

			payoutTimes, err := storage.GetPayoutTimesForNFT(ctx, collection.Denom.Id, nft.Id)
			if err != nil {
				return err
			}

			periodStart, periodEnd, err := s.findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
			if err != nil {
				return err
			}

			nftStatistics.PayoutPeriodStart = periodStart
			nftStatistics.PayoutPeriodEnd = periodEnd

			rewardForNft := calculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwners)

			maintenanceFee, cudoPartOfMaintenanceFee, rewardForNftAfterFee := s.calculateMaintenanceFeeForNFT(periodStart,
				periodEnd, hourlyMaintenanceFeeInSatoshis, rewardForNft)
			addPaymentAmountToAddress(destinationAddressesWithAmount, maintenanceFee, farm.MaintenanceFeePayoutAddress)
			addPaymentAmountToAddress(destinationAddressesWithAmount, cudoPartOfMaintenanceFee, s.config.CUDOFeePayoutAddress)
			log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s",
				collection.Denom.Id, nft.Id, rewardForNftAfterFee)
			log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s",
				collection.Denom.Id, nft.Id, maintenanceFee)
			log.Debug().Msgf("CUDO part (%.2f) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s",
				s.config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, cudoPartOfMaintenanceFee)
			nftStatistics.Reward = rewardForNftAfterFee
			nftStatistics.MaintenanceFee = maintenanceFee
			nftStatistics.CUDOPartOfMaintenanceFee = cudoPartOfMaintenanceFee

			CUDOMaintenanceFee += cudoPartOfMaintenanceFee
			farmMaintenanceFee += maintenanceFee
			nftRewardsAfterFees += rewardForNftAfterFee

			allNftOwnersForTimePeriodWithRewardPercent, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
				ctx, nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, &nftStatistics, nft.Owner, s.config.Network, rewardForNftAfterFee)
			if err != nil {
				return err
			}

			statistics = append(statistics, nftStatistics)

			distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNftAfterFee, destinationAddressesWithAmount)
		}

		// calculate collection's percent of rewards based on hash power
		_, ok := farmAuraPoolCollectionsMap[collection.Denom.Id]
		if !ok {
			return fmt.Errorf("aura pool collection not found by denom id {%s}", collection.Denom.Id)
		}
		auraPoolCollection := farmAuraPoolCollectionsMap[collection.Denom.Id]

		collectionPartOfFarm := auraPoolCollection.HashingPower / currentHashPowerForFarm

		collectionAwardAllocation := totalRewardForFarm.MulF64(collectionPartOfFarm)
		cudoGeneralFeeForCollection := cudosFeeOfTotalReward.MulF64(collectionPartOfFarm)
		farmLeftoverForCOllection := collectionAwardAllocation - cudoGeneralFeeForCollection - farmMaintenanceFee - nftRewardsAfterFees

		var collectionPaymentAllocation types.CollectionPaymentAllocation
		collectionPaymentAllocation.FarmId = farm.Id
		collectionPaymentAllocation.CollectionId = auraPoolCollection.Id
		collectionPaymentAllocation.CollectionAllocationAmount = collectionAwardAllocation
		collectionPaymentAllocation.CUDOGeneralFee = cudoGeneralFeeForCollection
		collectionPaymentAllocation.CUDOMaintenanceFee = CUDOMaintenanceFee
		collectionPaymentAllocation.FarmUnsoldLeftovers = farmLeftoverForCOllection
		collectionPaymentAllocation.FarmMaintenanceFee = farmMaintenanceFee

		collectionPaymentAllocationsStatistics = append(collectionPaymentAllocationsStatistics, collectionPaymentAllocation)
	}

	if len(destinationAddressesWithAmount) == 0 {
		return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.SubAccountName)
	}

	var totalAmountToPayToAddresses btcutil.Amount

	for _, amount := range destinationAddressesWithAmount {
		totalAmountToPayToAddresses += amount
	}

	// check that all of the amount is distributed and no more than it
	if totalAmountToPayToAddresses != totalRewardForFarm {
		return fmt.Errorf("distributed amount doesn't equal total farm rewards. Distributed amount: {%s}, TotalFarmReward: {%s}", totalAmountToPayToAddresses, totalRewardForFarm)
	}

	removeAddressesWithZeroReward(destinationAddressesWithAmount)

	addressesWithThresholdToUpdate, addressesWithAmountInfo, err := s.filterByPaymentThreshold(ctx, destinationAddressesWithAmount, storage, farm.Id)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Destination addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmount))

	addressesToSend := convertAmountToBTC(addressesWithAmountInfo)

	log.Debug().Msgf("Addresses above threshold that will be sent for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(addressesToSend))

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

	txHash := ""
	if len(addressesToSend) > 0 {
		txHash, err = s.apiRequester.SendMany(ctx, addressesToSend)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash)
	}

	err = storage.UpdateThresholdStatuses(ctx, transactionIdsToMarkProcessed, addressesWithThresholdToUpdate, farm.Id)
	if err != nil {
		log.Error().Msgf("Failed to update threshold for tx hash {%s}: %s", txHash, err)
		return err
	}

	if err := storage.SaveStatistics(ctx, farmPaymentStatistics, collectionPaymentAllocationsStatistics, addressesWithAmountInfo, statistics, txHash, farm.Id, farm.SubAccountName); err != nil {
		log.Error().Msgf("Failed to save statistics for tx hash {%s}: %s", txHash, err)
		return err
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

	SaveStatistics(ctx context.Context, farmPaymentStatistics types.FarmPayment, collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash string, farmId int64, farmSubAccountName string) error

	GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error)

	UpdateTransactionsStatus(ctx context.Context, txHashesToMarkCompleted []string, status string) error

	SaveTxHashWithStatus(ctx context.Context, txHash, status, farmSubAccountName string, retryCount int) error

	SaveRBFTransactionInformation(ctx context.Context, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName string, retryCount int) error

	GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error)

	GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int64) (float64, error)

	UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]btcutil.Amount, farmId int64) error

	SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount int) error

	GetFarmAuraPoolCollections(ctx context.Context, farmId int64) ([]types.AuraPoolCollection, error)
}

type Helper interface {
	DaysIn(m time.Month, year int) int
	Unix() int64
	Date() (year int, month time.Month, day int)
	SendMail(message string, to []string) error
}
