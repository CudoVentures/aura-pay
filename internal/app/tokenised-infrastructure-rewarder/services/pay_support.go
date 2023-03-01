package services

import (
	"context"
	"database/sql"
	"sort"
	"strconv"

	"github.com/btcsuite/btcd/chaincfg/chainhash"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

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

// returns last payment time for this nft or nft mint time
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
		return transaction.Processed == true, nil
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
