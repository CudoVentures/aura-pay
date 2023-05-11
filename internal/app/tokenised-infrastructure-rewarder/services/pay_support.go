package services

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

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
func (s *PayService) verifyCollectionIds(ctx context.Context, collections []types.AuraPoolCollection) ([]string, error) {
	var verifiedCollectionIds []string
	for _, collection := range collections {
		isVerified, err := s.apiRequester.VerifyCollection(ctx, collection.DenomId)
		if err != nil {
			return nil, err
		}
		if isVerified {
			verifiedCollectionIds = append(verifiedCollectionIds, collection.DenomId)
		} else {
			log.Info().Msgf("Collection with denomId %s is not verified", collection.DenomId)
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
func (s *PayService) getNftTimestamps(ctx context.Context, storage Storage, nft types.NFT, mintTimestamp int64, nftTransferHistory []types.NftTransferEvent, denomId string, periodEnd int64) (int64, int64, error) {
	payoutTimes, err := storage.GetPayoutTimesForNFT(ctx, denomId, nft.Id)
	if err != nil {
		return 0, 0, err
	}

	nftPeriodStart, err := s.findCurrentPayoutPeriod(payoutTimes, mintTimestamp)
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
// func (s *PayService) getNftTransferHistory(ctx context.Context, collectionDenomId, nftId string) (types.NftTransferHistory, error) {
// 	// TODO: This oculd be optimized, why fetching all events everytime
// 	nftTransferHistory, err := s.apiRequester.GetNftTransferHistory(ctx, collectionDenomId, nftId, 1) // all transfer events
// 	if err != nil {
// 		return types.NftTransferHistory{}, err
// 	}

// 	// sort in ascending order by timestamp
// 	sort.Slice(nftTransferHistory.Data.NestedData.Events, func(i, j int) bool {
// 		return nftTransferHistory.Data.NestedData.Events[i].Timestamp < nftTransferHistory.Data.NestedData.Events[j].Timestamp
// 	})

// 	return nftTransferHistory, nil
// }

// gets the higher from the last payment timestamp or the mint timestamp
// if there is any payment it is expected that it was done after the mint timestamp
// so if there are payment the last one's timestamp is returned
// otherwise - the mint event timestamp
// it is expected that the inputs are sorted
// TODO: do some checks, or get the max from both
func (s *PayService) findCurrentPayoutPeriod(payoutTimes []types.NFTStatistics, mintTimestamp int64) (int64, error) {
	l := len(payoutTimes)
	if l == 0 { // first time payment - start time is time of minting
		return mintTimestamp, nil
	}
	return payoutTimes[l-1].PayoutPeriodEnd, nil // last time we paid until now
}

// filterByPaymentThreshold filters the destinationAddressesWithAmountsBtcDecimal map by the payment threshold value.
// If the total accumulated amount for an address is greater than or equal to the payment threshold value, the function
// sets the thresholdReached flag to true in the returned addressesToSend map, otherwise, it sets the flag to false.
// The function also updates the accumulated amount for each address based on the amount sent, and returns the
// updated addressesWithThresholdToUpdateBtcDecimal map.
// Returns:
// - map[string]decimal.Decimal: A map with the destination addresses as keys and the updated accumulated amounts as values.
// - map[string]types.AmountInfo: A map with the destination addresses as keys and the amount to send and thresholdReached flag as values.
// - error: An error encountered during the function execution, if any.
func (s *PayService) filterByPaymentThreshold(ctx context.Context, destinationAddressesWithAmountsBtcDecimal map[string]decimal.Decimal, storage Storage, farmId int64) (map[string]decimal.Decimal, map[string]types.AmountInfo, map[string]string, error) {
	thresholdInBtcDecimal := decimal.NewFromFloat(s.config.GlobalPayoutThresholdInBTC)

	addressesWithThresholdToUpdateBtcDecimal := make(map[string]decimal.Decimal)

	addressesToSend := make(map[string]types.AmountInfo)

	var cudosBtcAddressMap = make(map[string]string)

	for address, amountForAddress := range destinationAddressesWithAmountsBtcDecimal {
		// get accumulation for cudos address and add it to current
		amountAccumulatedBtcDecimal, err := storage.GetCurrentAcummulatedAmountForAddress(ctx, address, farmId)

		if err != nil {
			switch err {
			case sql.ErrNoRows:
				log.Info().Msgf("No threshold found, inserting...")
				err = storage.SetInitialAccumulatedAmountForAddress(ctx, address, farmId, 0)
				if err != nil {
					return nil, nil, nil, err
				}
			default:
				return nil, nil, nil, err
			}
		}

		addressToSend := address

		// find btc address if it exists
		if isCudosAddress(address) {
			nftPayoutAddress, err := s.apiRequester.GetPayoutAddressFromNode(ctx, address, s.config.Network)
			if err != nil {
				return nil, nil, nil, err
			}

			cudosBtcAddressMap[address] = nftPayoutAddress

			if nftPayoutAddress != "" {
				addressToSend = nftPayoutAddress
				btcAddressAmount, err := storage.GetCurrentAcummulatedAmountForAddress(ctx, nftPayoutAddress, farmId)
				if err != nil {
					switch err {
					case sql.ErrNoRows:
						log.Info().Msgf("No threshold found, inserting...")
						err = storage.SetInitialAccumulatedAmountForAddress(ctx, nftPayoutAddress, farmId, 0)
						if err != nil {
							return nil, nil, nil, err
						}
					default:
						return nil, nil, nil, err
					}
				}

				amountAccumulatedBtcDecimal = amountAccumulatedBtcDecimal.Add(btcAddressAmount)

				addressesWithThresholdToUpdateBtcDecimal[nftPayoutAddress] = decimal.Zero
			}
		}

		// get accumulation for btc address as well and add it to current
		totalAmountAccumulatedForAddressBtcDecimal := amountAccumulatedBtcDecimal.Add(amountForAddress)
		amountToSendBtcDecimal := totalAmountAccumulatedForAddressBtcDecimal.RoundFloor(8) // up to 1 satoshi

		// if the address was cudos and there is registered btc address for it
		// addressToSend should be set to the btc address and used
		// if not, cudos address will be used
		if totalAmountAccumulatedForAddressBtcDecimal.GreaterThanOrEqual(thresholdInBtcDecimal) && !isCudosAddress(addressToSend) {
			// threshold reached, get amount to send up to 1 satoshi accuracy
			// subtract it from the total amount to reset the threshold with w/e is left
			addressesWithThresholdToUpdateBtcDecimal[address] = totalAmountAccumulatedForAddressBtcDecimal.Sub(amountToSendBtcDecimal)
			// if going to send for this address, use the btc one
			addressesToSend[addressToSend] = types.AmountInfo{Amount: amountToSendBtcDecimal, ThresholdReached: true}
		} else {
			addressesWithThresholdToUpdateBtcDecimal[address] = totalAmountAccumulatedForAddressBtcDecimal
			addressesToSend[address] = types.AmountInfo{Amount: amountToSendBtcDecimal, ThresholdReached: false}
		}
	}

	return addressesWithThresholdToUpdateBtcDecimal, addressesToSend, cudosBtcAddressMap, nil
}

// filterUnspentTransactions filters the given list of unspent transactions by removing transactions that
// have already been processed and change transactions. A change transaction is a transaction that is
// sending funds back to the farm addresses.
// Returns:
// - []btcjson.ListUnspentResult: A filtered list of unspent transactions.
// - error: An error encountered during the function execution, if any.
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

// isChangeTransaction checks if the given unspent transaction is a change transaction. A change transaction
// is a transaction that is sending funds back to the farm addresses.
// Returns:
// - bool: Returns true if the unspent transaction is a change transaction, false otherwise.
func isChangeTransaction(unspentTx btcjson.ListUnspentResult, farmAddresses []string) bool {
	for _, address := range farmAddresses {
		if address == unspentTx.Address {
			return false
		}
	}
	return true
}

// isTransactionProcessed checks if the given unspent transaction has been processed by querying the storage.
// Returns:
// - bool: Returns true if the transaction has been processed, false otherwise.
// - error: An error encountered during the function execution, if any.
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

// convertAmountToBTC converts the amounts in the destinationAddressesWithAmount map from decimal to
// float64 and filters the map to only include the addresses where the threshold has been reached.
// The returned map contains the destination addresses as keys and the amounts in BTC as float64 values.
//
// Parameters:
//   - destinationAddressesWithAmount map[string]types.AmountInfo: A map with the destination addresses as keys
//     and the amount and thresholdReached flag as values.
//
// Returns:
// - map[string]float64: A map with the destination addresses as keys and the amounts in BTC as float64 values.
// - error: An error encountered during the conversion, if any.
func convertAmountToBTC(destinationAddressesWithAmount map[string]types.AmountInfo) (map[string]float64, error) {
	result := make(map[string]float64)
	for k, v := range destinationAddressesWithAmount {
		if v.ThresholdReached {
			amountString := v.Amount.String()
			amountFloat, err := strconv.ParseFloat(amountString, 64)
			if err != nil {
				return nil, err
			}

			result[k] = amountFloat
		}
	}
	return result, nil
}

// addPaymentAmountToAddress adds the specified amount (amountToAdd) to the corresponding address (address) in the
// destinationAddressesWithAmount map.
//
// Parameters:
//   - destinationAddressesWithAmount map[string]decimal.Decimal: A map with the destination addresses as keys
//     and the accumulated amounts as values.
//   - amountToAdd decimal.Decimal: The amount to be added to the specified address.
//   - address string: The address to which the amount will be added.
func addPaymentAmountToAddress(destinationAddressesWithAmount map[string]decimal.Decimal, amountToAdd decimal.Decimal, address string) {
	destinationAddressesWithAmount[address] = destinationAddressesWithAmount[address].Add(amountToAdd)
}

func isCudosAddress(address string) bool {
	return strings.HasPrefix(address, "cudos1")
}

// addLeftoverRewardToFarmOwner adds the leftover reward amount (leftoverReward) to the farm owner's default
// payout address (farmDefaultPayoutAddress) in the destinationAddressesWithAmount map.
// If the farm owner's default payout address already exists in the map, the leftover reward is added to the existing amount.
// Otherwise, a new entry is created with the farm owner's default payout address and the leftover reward as the value.
//
// Parameters:
//   - destinationAddressesWithAmount map[string]decimal.Decimal: A map with the destination addresses as keys
//     and the accumulated amounts as values.
//   - leftoverReward decimal.Decimal: The leftover reward amount to be added.
//   - farmDefaultPayoutAddress string: The farm owner's default payout address.
func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]decimal.Decimal, leftoverReward decimal.Decimal, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = destinationAddressesWithAmount[farmDefaultPayoutAddress].Add(leftoverReward)
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

// validateFarm checks if the provided farm (farm) has valid properties. It returns an error if any of the following
// conditions are not met:
// - The farm must have a non-empty wallet name for rewards.
// - The farm's maintenance fee must be greater than 0.
// - The farm must have a non-empty address for receiving rewards from the pool.
// - The farm must have a non-empty maintenance fee payout address.
// - The farm must have a non-empty leftover reward payout address.
func validateFarm(farm types.Farm) error {
	if farm.RewardsFromPoolBtcWalletName == "" {
		return fmt.Errorf("farm has empty Wallet Name. Farm Id: {%d}", farm.Id)
	}

	if farm.MaintenanceFeeInBtc <= 0 {
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

// loadWallet attempts to load the specified Bitcoin wallet using the given BTC client.
// If the wallet fails to load for 15 consecutive attempts, the function returns an error.
// The function returns a boolean to indicate whether the wallet was successfully loaded or not.
// If the wallet is loaded successfully - nullate the fail counter for the wallet.
// Returns:
// - bool: True if the wallet was successfully loaded, false otherwise.
// - error: An error indicating the reason for the wallet load failure, if any.
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

// unloadWallet attempts to unload the specified Bitcoin wallet (farmName) using the given BTC client.
// If the wallet fails to unload, an error message is logged. If the wallet is successfully unloaded, a debug
// message is logged.
func unloadWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.UnloadWallet(&farmName); err != nil {
		log.Error().Msgf("Failed to unload wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} unloaded", farmName)
}

// lockWallet attempts to lock the specified Bitcoin wallet (farmName) using the given BTC client.
// If the wallet fails to lock, an error message is logged. If the wallet is successfully locked, a debug
// message is logged.
func lockWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.WalletLock(); err != nil {
		log.Error().Msgf("Failed to lock wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} locked", farmName)
}

// getLastUTXOTransactionTimestamp retrieves the timestamp of the last UTXO transaction for the specified farm
// using the provided Storage. If no previous transaction is found, the function returns the farm
// start time.
// Returns:
// - int64: The timestamp of the last UTXO transaction, or the farm start time if no previous transaction is found.
// - error: An error encountered during the function execution, if any.
func (s *PayService) getLastUTXOTransactionTimestamp(ctx context.Context, storage Storage, farm types.Farm) (int64, error) {
	lastUTXOTransaction, err := storage.GetLastUTXOTransactionByFarmId(ctx, farm.Id)
	if err != nil {
		return 0, err
	}

	// no payment found, so this is the first one. Get the one from foundry
	if lastUTXOTransaction.PaymentTimestamp == 0 {
		if s.config.IsTesting {
			return farm.CreatedAt.Unix(), nil
		} else {
			return s.apiRequester.GetFarmStartTime(ctx, farm.SubAccountName)
		}
	} else {
		return lastUTXOTransaction.PaymentTimestamp, nil
	}
}

// getCollectionsWithNftsForFarm retrieves the collections and their respective NFTs for the specified farm.
// The function fetches the collections from BDJUno and verifies them. It then retrieves the NFTs associated with
// the verified collections and the farm's AuraPoolCollections from the storage.
// Returns:
// - []types.Collection: A slice of the collections from the chain (BDJUno) with their associated NFTs.
// - map[string]types.AuraPoolCollection: A map of AuraPoolCollections keyed by their Denom IDs taken from the storage.
// - error: An error encountered during the function execution, if any.
func (s *PayService) getCollectionsWithNftsForFarm(ctx context.Context, storage Storage, farm types.Farm) ([]types.Collection, map[string]types.AuraPoolCollection, error) {
	farmAuraPoolCollections, err := storage.GetFarmAuraPoolCollections(ctx, farm.Id)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	verifiedDenomIds, err := s.verifyCollectionIds(ctx, farmAuraPoolCollections)
	if err != nil {
		return []types.Collection{}, map[string]types.AuraPoolCollection{}, err
	}

	log.Debug().Msgf("Verified collections for farm %s: %s", farm.RewardsFromPoolBtcWalletName, fmt.Sprintf("%v", verifiedDenomIds))
	farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionsWithNFTs(ctx, verifiedDenomIds)
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
			return []types.Collection{}, map[string]types.AuraPoolCollection{}, fmt.Errorf("CUDOS Markets collection not found by denom id {%s}", collection.Denom.Id)
		}
	}

	return farmCollectionsWithNFTs, farmAuraPoolCollectionsMap, nil
}
