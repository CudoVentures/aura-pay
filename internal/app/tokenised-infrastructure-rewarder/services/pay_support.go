package services

import (
	"context"
	"database/sql"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/rs/zerolog/log"
	"sort"
)

func (s *PayService) getTotalRewardForFarm(ctx context.Context, btcClient BtcClient, storage Storage, farmAddresses []string) (btcutil.Amount, []string, error) {
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

func (s *PayService) filterByPaymentThreshold(ctx context.Context, destinationAddressesWithAmounts map[string]btcutil.Amount, storage Storage, farmId int) (map[string]btcutil.Amount, map[string]types.AmountInfo, error) {
	thresholdInSatoshis, err := btcutil.NewAmount(s.config.GlobalPayoutThresholdInBTC)
	if err != nil {
		return nil, nil, err
	}

	addressesWithThresholdToUpdate := make(map[string]btcutil.Amount)

	addressesToSend := make(map[string]types.AmountInfo)

	for key := range destinationAddressesWithAmounts {
		amountAccumulatedBTC, err := storage.GetCurrentAcummulatedAmountForAddress(ctx, key, farmId)
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
		amountAccumulatedSatoshis := btcutil.Amount(amountAccumulatedBTC)
		if destinationAddressesWithAmounts[key]+amountAccumulatedSatoshis >= thresholdInSatoshis {
			addressesWithThresholdToUpdate[key] = 0 // threshold reached, reset it to 0 and update it later in DB
			amountToSend := destinationAddressesWithAmounts[key] + amountAccumulatedSatoshis
			addressesToSend[key] = types.AmountInfo{Amount: amountToSend, ThresholdReached: true}
		} else {
			addressesWithThresholdToUpdate[key] += destinationAddressesWithAmounts[key] + amountAccumulatedSatoshis
			addressesToSend[key] = types.AmountInfo{Amount: destinationAddressesWithAmounts[key], ThresholdReached: false}
		}
	}

	return addressesWithThresholdToUpdate, addressesToSend, nil
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

func payMaintenanceFeeForNFT(destinationAddressesWithAmount map[string]btcutil.Amount, maintenanceFeeAmount btcutil.Amount, farmMaintenanceFeePayoutAddress string) {
	destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] += maintenanceFeeAmount
}

func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]btcutil.Amount, leftoverReward btcutil.Amount, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] += leftoverReward
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)
		destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount
	}
}
