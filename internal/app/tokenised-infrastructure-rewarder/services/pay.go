package services

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
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
	farms, err := s.apiRequester.GetFarms(ctx)
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

	_, err := btcClient.LoadWallet(farm.SubAccountName)
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

	totalRewardForFarm, transactionIdsToMarkProcessed, err := s.GetTotalRewardForFarm(ctx, btcClient, storage, []string{farm.AddressForReceivingRewardsFromPool})
	if err != nil {
		return err
	}
	if totalRewardForFarm == 0 {
		log.Info().Msgf("reward for farm {{%s}} is 0....skipping this farm", farm.SubAccountName)
		return nil
	}
	log.Debug().Msgf("Total reward for farm %s: %s", farm.SubAccountName, totalRewardForFarm)

	collections, err := s.apiRequester.GetFarmCollectionsFromHasura(ctx, farm.SubAccountName)
	if err != nil {
		return err
	}

	var currentHashPowerForFarm float64
	if s.config.IsTesting {
		currentHashPowerForFarm = 1200 // hardcoded for testing & QA
	} else {
		currentHashPowerForFarm, err = s.apiRequester.GetFarmTotalHashPowerFromPoolToday(ctx, farm.SubAccountName, time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
		if err != nil {
			return err
		}

		if currentHashPowerForFarm <= 0 {
			return fmt.Errorf("invalid hash power (%f) for farm (%s)", currentHashPowerForFarm, farm.SubAccountName)
		}
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

	nonExpiredNFTsCount := s.filterExpiredNFTs(farmCollectionsWithNFTs)

	fmt.Printf("nonExpiredNFTsCount: %d", nonExpiredNFTsCount)
	if nonExpiredNFTsCount == 0 {
		log.Error().Msgf("all nfts for farm {%s} are expired", farm.SubAccountName)
		return nil
	}

	fmt.Printf("farmCollectionsWithNFTs: %+v", farmCollectionsWithNFTs)

	mintedHashPowerForFarm := sumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)
	log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.SubAccountName, mintedHashPowerForFarm)

	rewardForNftOwners := calculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarm)
	leftoverHashPower := currentHashPowerForFarm - mintedHashPowerForFarm // if hash power increased or not all of it is used as NFTs
	var rewardToReturn btcutil.Amount

	destinationAddressesWithAmount := make(map[string]btcutil.Amount)

	// return to the farm owner whatever is left
	if leftoverHashPower > 0 {
		rewardToReturn = calculatePercent(currentHashPowerForFarm, leftoverHashPower, totalRewardForFarm)
		addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, rewardToReturn, farm.LeftoverRewardPayoutAddress)
	}
	log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn: %s, farm: {%s}", rewardForNftOwners, rewardToReturn, farm.SubAccountName)

	statistics := []types.NFTStatistics{}

	for _, collection := range farmCollectionsWithNFTs {
		log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)
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

			maintenanceFee, cudoPartOfMaintenanceFee, rewardForNftAfterFee := s.calculateMaintenanceFeeForNFT(periodStart, periodEnd, hourlyMaintenanceFeeInSatoshis, rewardForNft)
			payMaintenanceFeeForNFT(destinationAddressesWithAmount, maintenanceFee, farm.MaintenanceFeePayoutdAddress)
			payMaintenanceFeeForNFT(destinationAddressesWithAmount, cudoPartOfMaintenanceFee, s.config.CUDOMaintenanceFeePayoutAddress)
			log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNftAfterFee)
			log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, maintenanceFee)
			log.Debug().Msgf("CUDO part (%.2f) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", s.config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, cudoPartOfMaintenanceFee)
			nftStatistics.Reward = rewardForNftAfterFee
			nftStatistics.MaintenanceFee = maintenanceFee
			nftStatistics.CUDOPartOfMaintenanceFee = cudoPartOfMaintenanceFee

			allNftOwnersForTimePeriodWithRewardPercent, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
				ctx, nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, &nftStatistics, nft.Owner, s.config.Network)
			if err != nil {
				return err
			}

			statistics = append(statistics, nftStatistics)

			distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNftAfterFee, destinationAddressesWithAmount, &nftStatistics)
		}
	}

	if len(destinationAddressesWithAmount) == 0 {
		return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.SubAccountName)
	}

	removeAddressesWithZeroReward(destinationAddressesWithAmount)
	addressesWithThresholdToUpdate, addressesWithAmountInfo, err := s.filterByPaymentThreshold(ctx, destinationAddressesWithAmount, storage, farm.Id)
	if err != nil {
		return err
	}

	log.Debug().Msgf("Destination addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmount))

	addressesToSend := convertAmountToBTC(addressesWithAmountInfo)

	log.Debug().Msgf("Addresses { above threshold that will be sent for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(addressesToSend))

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

	err = storage.UpdateThresholdStatuses(ctx, transactionIdsToMarkProcessed, addressesWithThresholdToUpdate, farm.Id)
	if err != nil {
		return err
	}

	txHash := ""
	if len(addressesToSend) > 0 {
		txHash, err = s.apiRequester.SendMany(ctx, addressesToSend, farm.SubAccountName, totalRewardForFarm)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash)
	}

	if err := storage.SaveStatistics(ctx, addressesWithAmountInfo, statistics, txHash, strconv.Itoa(farm.Id), farm.SubAccountName); err != nil {
		log.Error().Msgf("Failed to save statistics for tx hash {%s}: %s", txHash, err)
	}

	return nil
}

func (s *PayService) GetTotalRewardForFarm(ctx context.Context, btcClient BtcClient, storage Storage, farmAddresses []string) (btcutil.Amount, []string, error) {
	var totalAmountBTC float64
	var transactionIdsToMarkAsProcessed []string // to be marked as processed at the end of the loop
	unspentTransactions, err := btcClient.ListUnspent()
	if err != nil {
		return 0, nil, err
	}

	validUnspentTransactions, err := filterUnspentTransactions(ctx, unspentTransactions, storage, farmAddresses)
	if err != nil {
		return 0, nil, err
	}

	for _, elem := range validUnspentTransactions {
		totalAmountBTC += elem.Amount
		transactionIdsToMarkAsProcessed = append(transactionIdsToMarkAsProcessed, elem.TxID)
	}
	totalAmountSatoshish, err := btcutil.NewAmount(totalAmountBTC)
	if err != nil {
		return 0, nil, err
	}
	return totalAmountSatoshish, transactionIdsToMarkAsProcessed, nil
}

func filterUnspentTransactions(ctx context.Context, transactions []btcjson.ListUnspentResult, storage Storage, farmAddresses []string) ([]btcjson.ListUnspentResult, error) {
	var validTransactions []btcjson.ListUnspentResult
	for _, unspentTx := range transactions {
		isTransactionProcessed, err := isTransactionProcessed(ctx, unspentTx, storage)
		if err != nil {
			return nil, err
		}
		if !isTransactionProcessed && !isChangeTransaction(unspentTx, farmAddresses) {
			validTransactions = append(validTransactions, unspentTx)
		}
	}

	return validTransactions, nil
}

func isChangeTransaction(unspentTx btcjson.ListUnspentResult, farmAddresses []string) bool {
	for _, address := range farmAddresses {
		if address == unspentTx.Address {
			return false
		}
	}
	return true
}

func isTransactionProcessed(ctx context.Context, unspentTx btcjson.ListUnspentResult, storage Storage) (bool, error) {
	transaction, err := storage.GetUTXOTransaction(ctx, unspentTx.TxID)
	switch err {
	case nil:
		return transaction.Processed == true, nil
	case sql.ErrNoRows:
		return false, nil // not found thus not processed
	default:
		return false, err
	}
}

func (s *PayService) filterByPaymentThreshold(ctx context.Context, destinationAddressesWithAmounts map[string]btcutil.Amount, storage Storage, farmId int) (map[string]int64, map[string]types.AmountInfo, error) {
	thresholdInSatoshis, err := btcutil.NewAmount(s.config.GlobalPayoutThresholdInBTC)
	if err != nil {
		return nil, nil, err
	}

	addressesWithThresholdToUpdate := make(map[string]int64)

	addressesToSend := make(map[string]types.AmountInfo)

	for key := range destinationAddressesWithAmounts {
		amountAccumulated, err := storage.GetCurrentAcummulatedAmountForAddress(ctx, key, farmId)
		if err != nil {
			switch err {
			case sql.ErrNoRows:
				log.Info().Msgf("No threshold found, inserting...")
				err = storage.SetInitialAccumulatedAmountForAddress(ctx, nil, key, farmId, 0)
			default:
				return nil, nil, err
			}
		}
		amountAccumulatedSatoshis := btcutil.Amount(amountAccumulated)
		if destinationAddressesWithAmounts[key]+amountAccumulatedSatoshis >= thresholdInSatoshis {
			addressesWithThresholdToUpdate[key] = 0 // threshold reached, reset it to 0 and update it later in DB
			amountToSend := destinationAddressesWithAmounts[key] + amountAccumulatedSatoshis
			addressesToSend[key] = types.AmountInfo{Amount: amountToSend, ThresholdReached: true}
		} else {
			addressesWithThresholdToUpdate[key] += int64(destinationAddressesWithAmounts[key]) + amountAccumulated
			addressesToSend[key] = types.AmountInfo{Amount: destinationAddressesWithAmounts[key], ThresholdReached: false}
		}
	}

	return addressesWithThresholdToUpdate, addressesToSend, nil
}

// removeAddressesWithZeroReward utilised in case we had a maintenance fee greater than the nft reward.
// keys with 0 value were added in order to have statistics even for 0 reward
// and in order to avoid sending them as 0 - just remove them but still keep statistic
func removeAddressesWithZeroReward(destinationAddressesWithAmount map[string]btcutil.Amount) {
	for key := range destinationAddressesWithAmount {
		if destinationAddressesWithAmount[key] == 0 {
			delete(destinationAddressesWithAmount, key)
		}

	}
}

// Converts Satoshi to BTC so it can accepted by the RPC interface
func convertAmountToBTC(destinationAddressesWithAmount map[string]types.AmountInfo) map[string]float64 {
	result := make(map[string]float64)
	for k, v := range destinationAddressesWithAmount {
		if v.ThresholdReached {
			result[k] = v.Amount.ToBTC()
		}
	}
	return result
}

func (s *PayService) filterExpiredNFTs(farmCollectionsWithNFTs []types.Collection) int {
	nonExpiredNFTsCount := 0
	now := s.helper.Unix()
	for i := 0; i < len(farmCollectionsWithNFTs); i++ {
		var nonExpiredNFTs []types.NFT
		for j := 0; j < len(farmCollectionsWithNFTs[i].Nfts); j++ {
			currentNft := farmCollectionsWithNFTs[i].Nfts[j]
			if now > currentNft.DataJson.ExpirationDate {
				log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%d} has expired! Skipping....", farmCollectionsWithNFTs[i].Denom.Id,
					currentNft.Id, currentNft.DataJson.ExpirationDate)
				continue
			}
			nonExpiredNFTs = append(nonExpiredNFTs, currentNft)
		}
		farmCollectionsWithNFTs[i].Nfts = nonExpiredNFTs
		nonExpiredNFTsCount += len(nonExpiredNFTs)
	}

	return nonExpiredNFTsCount
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

func payMaintenanceFeeForNFT(destinationAddressesWithAmount map[string]btcutil.Amount, maintenanceFeeAmount btcutil.Amount, farmMaintenanceFeePayoutAddress string) {
	destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] += maintenanceFeeAmount
}

func (s *PayService) calculateHourlyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) (btcutil.Amount, error) {
	currentYear, currentMonth, _ := s.helper.Date()
	periodLength := s.helper.DaysIn(currentMonth, currentYear)
	mtFeeInBTC, err := strconv.ParseFloat(farm.MonthlyMaintenanceFeeInBTC, 64)
	if err != nil {
		return -1, err
	}
	mtFeeInSatoshis, err := btcutil.NewAmount(mtFeeInBTC)
	if err != nil {
		return -1, err
	}
	feePerOneHashPower := btcutil.Amount(float64(mtFeeInSatoshis) / currentHashPowerForFarm)
	dailyFeeInSatoshis := int(feePerOneHashPower) / periodLength
	hourlyFeeInSatoshis := dailyFeeInSatoshis / 24
	return btcutil.Amount(hourlyFeeInSatoshis), nil
}

func (s *PayService) getNftTransferHistory(ctx context.Context, collectionDenomId, nftId string) (types.NftTransferHistory, error) {
	// TODO: This oculd be optimized, why fetching all events everytime
	nftTransferHistory, err := s.apiRequester.GetNftTransferHistory(ctx, collectionDenomId, nftId, 1) // all transfer events
	if err != nil {
		return types.NftTransferHistory{}, err
	}

	// sort in ascending order by timestamp
	sort.Slice(nftTransferHistory.Data.NestedData.Events, func(i, j int) bool {
		return nftTransferHistory.Data.NestedData.Events[i].Timestamp < nftTransferHistory.Data.NestedData.Events[j].Timestamp
	})

	return nftTransferHistory, nil
}

func (s *PayService) findCurrentPayoutPeriod(payoutTimes []types.NFTStatistics, nftTransferHistory types.NftTransferHistory) (int64, int64, error) {
	l := len(payoutTimes)
	if l == 0 { // first time payment - start time is time of minting, end time is now
		return nftTransferHistory.Data.NestedData.Events[0].Timestamp, s.helper.Unix(), nil
	}
	return payoutTimes[l-1].PayoutPeriodEnd, s.helper.Unix(), nil // last time we paid until now
}

func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]btcutil.Amount, leftoverReward btcutil.Amount, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] += leftoverReward
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func (s *PayService) verifyCollectionIds(ctx context.Context, collections types.CollectionData) ([]string, error) {
	var verifiedCollectionIds []string
	for _, collection := range collections.Data.DenomsByDataProperty {
		isVerified, err := s.apiRequester.VerifyCollection(ctx, collection.Id)
		if err != nil {
			return nil, err
		}

		if isVerified {
			verifiedCollectionIds = append(verifiedCollectionIds, collection.Id)
		} else {
			log.Info().Msgf("Collection with denomId %s is not verified", collection.Id)
		}
	}

	return verifiedCollectionIds, nil
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount, statistics *types.NFTStatistics) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)
		destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount
		addPaymentAmountToStatistics(payoutAmount, nftPayoutAddress, statistics)
	}
}

func addPaymentAmountToStatistics(amount btcutil.Amount, payoutAddress string, nftStatistics *types.NFTStatistics) {
	// bug here if no nft owners for time period
	for i := 0; i < len(nftStatistics.NFTOwnersForPeriod); i++ {
		additionalData := nftStatistics.NFTOwnersForPeriod[i]
		if additionalData.PayoutAddress == payoutAddress {
			additionalData.Reward = amount
		}
	}
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

	GetFarmCollectionsFromHasura(ctx context.Context, farmId string) (types.CollectionData, error)

	GetFarms(ctx context.Context) ([]types.Farm, error)

	VerifyCollection(ctx context.Context, denomId string) (bool, error)

	GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error)

	SendMany(ctx context.Context, destinationAddressesWithAmount map[string]float64, walletName string, walletBalance btcutil.Amount) (string, error)

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
	GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error)

	SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash, farmId, farmSubAccountName string) error

	GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error)

	UpdateTransactionsStatus(ctx context.Context, tx *sqlx.Tx, txHashesToMarkCompleted []string, status string) error

	SaveTxHashWithStatus(ctx context.Context, tx *sqlx.Tx, txHash string, status string, farmId string, retryCount int) error

	SaveRBFTransactionHistory(ctx context.Context, tx *sqlx.Tx, oldTxHash string, newTxHash string, farmId string) error

	SaveRBFTransactionInformation(
		ctx context.Context,
		oldTxHash string,
		oldTxStatus string,
		newRBFTxHash string,
		newRBFTXStatus string,
		farmSubAccountName string,
		retryCount int) error

	GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error)

	GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int) (int64, error)

	UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]int64, farmId int) error

	SetInitialAccumulatedAmountForAddress(ctx context.Context, tx *sqlx.Tx, address string, farmId int, amount int) error
}

type Helper interface {
	DaysIn(m time.Month, year int) int
	Unix() int64
	Date() (year int, month time.Month, day int)
}
