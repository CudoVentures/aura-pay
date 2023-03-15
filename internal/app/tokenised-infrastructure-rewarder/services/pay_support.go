package services

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// gets the details for a single unspent transaction from the BTC node
// this is needed for the timestamp of the TX
func (s *PayService) getUnspentTxDetails(ctx context.Context, btcClient BtcClient, unspentResult btcjson.ListUnspentResult) (btcjson.TxRawResult, error) {
	txHash, err := chainhash.NewHashFromStr(unspentResult.TxID)
	if err != nil {
		return btcjson.TxRawResult{}, err
	}

	txRawResult, err := btcClient.GetRawTransactionVerbose(txHash)
	if err != nil {
		return btcjson.TxRawResult{}, err
	}

	return *txRawResult, nil
}

// gets all the unspent transactions for the farm wallet
// the farm wallet must fisrst be loaded
func (s *PayService) getUnspentTxsForFarm(ctx context.Context, btcClient BtcClient, storage Storage, farmAddresses []string) ([]btcjson.ListUnspentResult, error) {
	unspentTransactions, err := btcClient.ListUnspent()
	if err != nil {
		return nil, err
	}

	validUnspentTransactions, err := filterUnspentTransactions(ctx, unspentTransactions, storage, farmAddresses)
	if err != nil {
		return nil, err
	}

	return validUnspentTransactions, nil
}

// tries to get the collection from BDJuno
// it also check there if it is verified
// basically if a collection is not verified (minted), it does not exist on the chain
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

// for each collection
// for each nft of that collection
// check if it's expiration date is before the period start
// if it is, it shouldn't receive any rewards for that payment
// if it is not expired - add it to the list that is to be returned
func (s *PayService) filterExpiredBeforePeriodNFTs(farmCollectionsWithNFTs []types.Collection, periodStart int64) int {
	nonExpiredNFTsCount := 0
	for i := 0; i < len(farmCollectionsWithNFTs); i++ {
		var nonExpiredNFTs []types.NFT
		for j := 0; j < len(farmCollectionsWithNFTs[i].Nfts); j++ {
			currentNft := farmCollectionsWithNFTs[i].Nfts[j]
			if periodStart > currentNft.DataJson.ExpirationDate {

				log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%d} has expired before period start! Skipping....", farmCollectionsWithNFTs[i].Denom.Id,
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

// calculates the period start and end for a given nft
// gets the payout times entries for that nft
// the period start for the nft is the bigger one from the bigger from the last payout time, or nft mint time
// the period end is the smaller from the expiration date and the given period end
func (s *PayService) getNftTimestamps(ctx context.Context, storage Storage, nft types.NFT, nftTransferHistory types.NftTransferHistory, denomId string, periodEnd int64) (int64, int64, error) {
	payoutTimes, err := storage.GetPayoutTimesForNFT(ctx, denomId, nft.Id)
	if err != nil {
		return 0, 0, err
	}

	nftPeriodStart, err := s.findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
	if err != nil {
		return 0, 0, err
	}

	var nftPeriodEnd int64
	// nft expired before within this period
	if nft.DataJson.ExpirationDate < periodEnd {
		nftPeriodEnd = nft.DataJson.ExpirationDate
	} else {
		nftPeriodEnd = periodEnd
	}

	return nftPeriodStart, nftPeriodEnd, nil
}

// gets the nft transfer history entries from BDJuno
// sorts them
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

// gets the higher from the last payment timestamp or the mint timestamp
// if there is any payment it is expected that it was done after the mint timestamp
// so if there are payment the last one's timestamp is returned
// otherwise - the mint event timestamp
// it is expected that the inputs are sorted
// TODO: do some checks, or get the max from both
func (s *PayService) findCurrentPayoutPeriod(payoutTimes []types.NFTStatistics, nftTransferHistory types.NftTransferHistory) (int64, error) {
	l := len(payoutTimes)
	if l == 0 { // first time payment - start time is time of minting
		return nftTransferHistory.Data.NestedData.Events[0].Timestamp, nil
	}
	return payoutTimes[l-1].PayoutPeriodEnd, nil // last time we paid until now
}

func (s *PayService) filterByPaymentThreshold(ctx context.Context, destinationAddressesWithAmountsBtcDecimal map[string]decimal.Decimal, storage Storage, farmId int64) (map[string]decimal.Decimal, map[string]types.AmountInfo, error) {
	thresholdInBtcDecimal := decimal.NewFromFloat(s.config.GlobalPayoutThresholdInBTC)

	addressesWithThresholdToUpdateBtcDecimal := make(map[string]decimal.Decimal)

	addressesToSend := make(map[string]types.AmountInfo)

	for key := range destinationAddressesWithAmountsBtcDecimal {
		amountAccumulatedBtcDecimal, err := storage.GetCurrentAcummulatedAmountForAddress(ctx, key, farmId)
		if err != nil {
			switch err {
			case sql.ErrNoRows:
				log.Info().Msgf("No threshold found, inserting...")
				err = storage.SetInitialAccumulatedAmountForAddress(ctx, key, farmId, 0)
				if err != nil {
					return nil, nil, err
				}
			default:
				return nil, nil, err
			}
		}

		totalAmountAccumulatedForAddressBtcDecimal := destinationAddressesWithAmountsBtcDecimal[key].Add(amountAccumulatedBtcDecimal)
		amountToSendBtcDecimal := totalAmountAccumulatedForAddressBtcDecimal.RoundFloor(8) // up to 1 satoshi

		if totalAmountAccumulatedForAddressBtcDecimal.GreaterThanOrEqual(thresholdInBtcDecimal) {
			// threshold reached, get amount to send up to 1 satoshi accuracy
			// subtract it from the total amount to reset the threshold with w/e is left
			addressesWithThresholdToUpdateBtcDecimal[key] = totalAmountAccumulatedForAddressBtcDecimal.Sub(amountToSendBtcDecimal)
			addressesToSend[key] = types.AmountInfo{Amount: amountToSendBtcDecimal, ThresholdReached: true}
		} else {
			addressesWithThresholdToUpdateBtcDecimal[key] = totalAmountAccumulatedForAddressBtcDecimal
			addressesToSend[key] = types.AmountInfo{Amount: amountToSendBtcDecimal, ThresholdReached: false}
		}
	}

	return addressesWithThresholdToUpdateBtcDecimal, addressesToSend, nil
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
		return transaction.Processed, nil
	case sql.ErrNoRows:
		return false, nil // not found thus not processed
	default:
		return false, err
	}
}

// removeAddressesWithZeroReward utilised in case we had a maintenance fee greater than the nft reward.
// keys with 0 value were added in order to have statistics even for 0 reward
// and in order to avoid sending them as 0 - just remove them but still keep statistic
func removeAddressesWithZeroReward(destinationAddressesWithAmount map[string]decimal.Decimal) {
	for key := range destinationAddressesWithAmount {
		if destinationAddressesWithAmount[key].IsZero() {
			delete(destinationAddressesWithAmount, key)
		}
	}
}

// Converts decimals to BTC so it can accepted by the RPC interface
func convertAmountToBTC(destinationAddressesWithAmount map[string]types.AmountInfo) (map[string]float64, error) {
	result := make(map[string]float64)
	for k, v := range destinationAddressesWithAmount {
		if v.ThresholdReached {
			amountString := v.Amount.RoundFloor(8).String()
			amountFloat, err := strconv.ParseFloat(amountString, 64)
			if err != nil {
				return nil, err
			}

			result[k] = amountFloat
		}
	}
	return result, nil
}

func addPaymentAmountToAddress(destinationAddressesWithAmount map[string]decimal.Decimal, maintenanceFeeAmountbtcDecimal decimal.Decimal, farmMaintenanceFeePayoutAddress string) {
	destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] = destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress].Add(maintenanceFeeAmountbtcDecimal)
}

func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]decimal.Decimal, leftoverReward decimal.Decimal, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = destinationAddressesWithAmount[farmDefaultPayoutAddress].Add(leftoverReward)
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount decimal.Decimal, destinationAddressesWithAmount map[string]decimal.Decimal) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.Mul(decimal.NewFromFloat(percentFromReward / 100))
		destinationAddressesWithAmount[nftPayoutAddress] = destinationAddressesWithAmount[nftPayoutAddress].Add(payoutAmount)
	}
}

func validateFarm(farm types.Farm) error {
	if farm.RewardsFromPoolBtcWalletName == "" {
		return fmt.Errorf("farm has empty Wallet Name. Farm Id: {%d}", farm.Id)
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

// try to load the wallet. If it fails, increase counter
// if it fails 15 times, throw error that will result in err on the terminal and email sent
func (s *PayService) loadWallet(btcClient BtcClient, farmName string) (bool, error) {
	_, err := btcClient.LoadWallet(farmName)
	if err != nil {
		s.btcWalletOpenFailsPerFarm[farmName]++
		if s.btcWalletOpenFailsPerFarm[farmName] >= 15 {
			s.btcWalletOpenFailsPerFarm[farmName] = 0
			return false, fmt.Errorf("failed to load wallet %s for 15 times", farmName)
		}

		log.Warn().Msgf("Failed to load wallet %s for %d consecutive times: %s", farmName, s.btcWalletOpenFailsPerFarm[farmName], err)
		return false, nil
	}

	s.btcWalletOpenFailsPerFarm[farmName] = 0
	log.Debug().Msgf("Farm Wallet: {%s} loaded", farmName)

	return true, nil
}

func unloadWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.UnloadWallet(&farmName); err != nil {
		log.Error().Msgf("Failed to unload wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} unloaded", farmName)
}

func lockWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.WalletLock(); err != nil {
		log.Error().Msgf("Failed to lock wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} locked", farmName)
}

func (s *PayService) getLastUTXOTransactionTimestamp(ctx context.Context, storage Storage, farm types.Farm) (int64, error) {
	lastUTXOTransaction, err := storage.GetLastUTXOTransactionByFarmId(ctx, farm.Id)
	if err != nil {
		return 0, err
	}

	// no payment found, so this is the first one. Get the one from foundry
	if lastUTXOTransaction.PaymentTimestamp == 0 {
		if s.config.IsTesting {
			return 0, nil
		} else {
			return s.apiRequester.GetFarmStartTime(ctx, farm.SubAccountName)
		}
	} else {
		return lastUTXOTransaction.PaymentTimestamp, nil
	}
}

func (s *PayService) getCollectionsWithNftsForFarm(ctx context.Context, storage Storage, farm types.Farm) ([]types.Collection, map[string]types.AuraPoolCollection, error) {
	collections, err := s.apiRequester.GetFarmCollectionsFromHasura(ctx, farm.Id)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	verifiedDenomIds, err := s.verifyCollectionIds(ctx, collections)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	log.Debug().Msgf("Verified collections for farm %s: %s", farm.RewardsFromPoolBtcWalletName, fmt.Sprintf("%v", verifiedDenomIds))

	farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionsWithNFTs(ctx, verifiedDenomIds)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	farmAuraPoolCollections, err := storage.GetFarmAuraPoolCollections(ctx, farm.Id)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	// make a map for faster getting
	farmAuraPoolCollectionsMap := map[string]types.AuraPoolCollection{}
	for _, farmAuraPoolCollection := range farmAuraPoolCollections {
		farmAuraPoolCollectionsMap[farmAuraPoolCollection.DenomId] = farmAuraPoolCollection
	}

	for _, collection := range farmCollectionsWithNFTs {
		_, ok := farmAuraPoolCollectionsMap[collection.Denom.Id]
		if !ok {
			return []types.Collection{}, map[string]types.AuraPoolCollection{}, fmt.Errorf("aura pool collection not found by denom id {%s}", collection.Denom.Id)
		}
	}

	return farmCollectionsWithNFTs, farmAuraPoolCollectionsMap, nil
}
