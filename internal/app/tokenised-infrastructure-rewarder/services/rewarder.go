package services

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/sql_db"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func ProcessPayment(config *infrastructure.Config) error {
	// bitcoin rpc client init
	rpcClient, err := infrastructure.InitBtcRpcClient(config)
	if err != nil {
		return err
	}
	defer rpcClient.Shutdown()

	db, err := sqlx.Connect(fmt.Sprintf("%s", config.DbDriverName), fmt.Sprintf("user=%s dbname=%s sslmode=disable", config.DbUserNameWithPassword, config.DbName))
	if err != nil {
		return err
	}

	farms, err := requesters.GetFarms()
	if err != nil {
		return err
	}

	for _, farm := range farms {
		log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
		destinationAddressesWithAmount := make(map[string]btcutil.Amount)
		var statistics []types.NFTStatistics
		totalRewardForFarm, err := rpcClient.GetBalance(farm.SubAccountName)
		if err != nil {
			return err
		}
		if totalRewardForFarm == 0 {
			return fmt.Errorf("Reward for farm %s is 0....skipping this farm", farm.SubAccountName)
		}
		log.Debug().Msgf("Total reward for farm %s: %s", farm.SubAccountName, totalRewardForFarm)
		collections, err := requesters.GetFarmCollectionsFromHasura(farm.SubAccountName)
		if err != nil {
			return err
		}
		currentHashPowerForFarm, err := requesters.GetFarmTotalHashPowerFromPoolToday(farm.SubAccountName, time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
		if err != nil {
			return err
		}
		log.Debug().Msgf("Total hash power for farm %s: %s", farm.SubAccountName, currentHashPowerForFarm)
		verifiedDenomIds, err := verifyCollectionIds(collections)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Verified collections for farm %s: %s", farm.SubAccountName, fmt.Sprintf("%v", verifiedDenomIds))
		farmCollectionsWithNFTs, err := requesters.GetFarmCollectionWithNFTs(verifiedDenomIds)
		if err != nil {
			return err
		}
		mintedHashPowerForFarm := SumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)
		log.Debug().Msgf("Minted hash for farm %s: %s", farm.SubAccountName, mintedHashPowerForFarm)

		hasHashPowerIncreased, leftoverAmount := HasHashPowerIncreased(currentHashPowerForFarm, mintedHashPowerForFarm)
		log.Debug().Msgf("hasHashPowerIncreased : %s, leftoverAmount: ", hasHashPowerIncreased, leftoverAmount)

		rewardForNftOwners := totalRewardForFarm
		if hasHashPowerIncreased {
			rewardForNftOwners, err = CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, float64(totalRewardForFarm))
		}
		log.Debug().Msgf("Reward for nft owners : %s", rewardForNftOwners)

		for _, collection := range farmCollectionsWithNFTs {
			log.Debug().Msgf("Processing collection with denomId %s..", collection.Denom.Id)
			for _, nft := range collection.Nfts {
				if time.Now().Unix() > nft.Data.ExpirationDate {
					log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%s} has expired! Skipping....", collection.Denom.Id, nft.Id, nft.Data.ExpirationDate)
					continue
				}
				var nftStatistics types.NFTStatistics
				nftStatistics.TokenId = nft.Id

				rewardForNft, err := CalculatePercent(mintedHashPowerForFarm, nft.Data.HashRateOwned, float64(rewardForNftOwners))
				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNft)
				if err != nil {
					return err
				}
				nftStatistics.RewardForNFT = rewardForNft

				nftTransferHistory, err := getNftTransferHistory(collection.Denom.Id, nft.Id)
				if err != nil {
					return err
				}
				payoutTimes, err := sql_db.GetPayoutTimesForNFT(db, nft.Id)
				if err != nil {
					return err
				}
				periodStart, periodEnd, err := findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
				nftStatistics.PayoutPeriodStart = periodStart
				nftStatistics.PayoutPeriodEnd = periodEnd

				allNftOwnersForTimePeriodWithRewardPercent, err := calculateNftOwnersForTimePeriodWithRewardPercent(nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, nftStatistics)
				if err != nil {
					return err
				}
				distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNft, destinationAddressesWithAmount, nftStatistics)

				tx := db.MustBegin()
				sql_db.SetPayoutTimesForNFT(tx, nft.Id, time.Now().Unix(), rewardForNft.ToBTC())
				tx.Commit()
			}
		}

		if hasHashPowerIncreased {
			leftoverReward, err := CalculatePercent(currentHashPowerForFarm, leftoverAmount, float64(totalRewardForFarm))
			if err != nil {
				return err
			}
			addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, leftoverReward, farm.DefaultBTCPayoutAddress)
			log.Debug().Msgf("Leftover reward with for farm with Id {%s} amount {%s} is added for return to the farm admin with address {%s}", farm.SubAccountName, leftoverReward, farm.DefaultBTCPayoutAddress)
		}

		if len(destinationAddressesWithAmount) == 0 {
			return fmt.Errorf("No addresses found to pay for Farm {%s}", farm.SubAccountName)
		}
		log.Debug().Msgf("Destionation addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmount))

		txHash, err := payRewards("bf4961e4259c9d9c7bdf4862fdeeb0337d06479737c2c63e4af360913b11277f", uint32(1), farm.BTCWallet, destinationAddressesWithAmount, rpcClient)
		// NFTStatistics - save nft statistics - object from above
		// Farm Statistics - save everything about the farm - including addresses
		sql_tx := db.MustBegin()
		saveStatistics(txHash, destinationAddressesWithAmount, statistics, sql_tx, farm.Id)
		sql_tx.Commit()
	}

	return nil
}

func saveStatistics(txHash *chainhash.Hash, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, sql_tx *sqlx.Tx, farmId string) {
	for address, amount := range destinationAddressesWithAmount {
		sql_db.SaveDestionAddressesWithAmountHistory(sql_tx, address, amount, txHash.String(), farmId)
	}

	for _, nftStatistic := range statistics {
		sql_db.SaveNftInformationHistory(sql_tx, nftStatistic.TokenId, nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.RewardForNFT, txHash.String())
		for _, ownersForPeriod := range nftStatistic.NFTOwnersForPeriod {
			sql_db.SaveNFTOwnersForPeriodHistory(sql_tx, ownersForPeriod.TimeOwnedFrom, ownersForPeriod.TimeOwnedTo, ownersForPeriod.TotalTimeOwned, ownersForPeriod.PercentOfTimeOwned, ownersForPeriod.Owner, ownersForPeriod.PayoutAddress, ownersForPeriod.Reward)
		}
	}
}

func getNftTransferHistory(collectionDenomId, nftId string) (types.NftTransferHistory, error) {
	nftTransferHistory, err := requesters.GetNftTransferHistory(collectionDenomId, nftId, 0) // all transfer events
	if err != nil {
		return nil, err
	}

	// sort in ascending order by timestamp
	sort.Slice(nftTransferHistory, func(i, j int) bool {
		return nftTransferHistory[i].Timestamp < nftTransferHistory[j].Timestamp
	})

	return nftTransferHistory, nil
}

func findCurrentPayoutPeriod(payoutTimes []types.NFTPayoutTime, nftTransferHistory types.NftTransferHistory) (int64, int64, error) {
	if len(payoutTimes) == 0 { // first time payment - start time is time of minting, end time is now
		return nftTransferHistory[0].Timestamp, time.Now().Unix(), nil
	}

	if len(payoutTimes) == 1 {
		return payoutTimes[0].Time, time.Now().Unix(), nil
	}

	l := len(payoutTimes)

	return payoutTimes[l-2].Time, payoutTimes[l-1].Time, nil

}

func HasHashPowerIncreased(currentHashPowerForFarm float64, mintedHashPowerForFarm float64) (bool, float64) {
	if currentHashPowerForFarm > mintedHashPowerForFarm {
		leftOverAmount := currentHashPowerForFarm - mintedHashPowerForFarm
		return true, leftOverAmount
	}

	return false, -1
}

func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]btcutil.Amount, leftoverReward btcutil.Amount, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] += leftoverReward
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func verifyCollectionIds(collections types.CollectionData) ([]string, error) {
	var verifiedCollectionIds []string
	for _, collection := range collections.Data.DenomsByDataProperty {
		isVerified, err := requesters.VerifyCollection(collection.Id)
		if err != nil {
			return nil, err
		}

		if isVerified {
			verifiedCollectionIds = append(verifiedCollectionIds, collection.Id)
		} else {
			log.Error().Msgf("Collection with denomId %s is not verified", collection.Id)
		}
	}

	return verifiedCollectionIds, nil
}

func sumCollectionHashPower(collectionNFTs []types.NFT) float64 {
	var collectionHashPower float64
	for _, nft := range collectionNFTs {
		collectionHashPower += nft.Data.HashRateOwned
	}
	return collectionHashPower
}

func distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount, statistics types.NFTStatistics) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)    // TODO: Change this to normal float64 percent as MULF64 is rounding
		if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
			destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount
		} else {
			destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
		}
		addPaymentAmountToStatistics(payoutAmount, nftPayoutAddress, statistics)
	}
}

func addPaymentAmountToStatistics(amount btcutil.Amount, payoutAddress string, nftStatistics types.NFTStatistics) {
	for i := 0; i < len(nftStatistics.NFTOwnersForPeriod); i++ {
		additionalData := nftStatistics.NFTOwnersForPeriod[i]
		if additionalData.PayoutAddress == payoutAddress {
			additionalData.Reward = amount
		}
	}
}

func payRewards(inputTxId string, inputTxVout uint32, walletName string, destinationAddressesWithAmount map[string]btcutil.Amount, rpcClient *rpcclient.Client) (*chainhash.Hash, error) {
	var outputVouts []int
	for i := 0; i < len(destinationAddressesWithAmount); i++ {
		outputVouts = append(outputVouts, i)
	}

	txInput := btcjson.TransactionInput{Txid: inputTxId, Vout: inputTxVout}
	inputs := []btcjson.TransactionInput{txInput}
	isWitness := false
	transformedAddressesWithAmount, err := transformAddressesWithAmount(destinationAddressesWithAmount)
	if err != nil {
		return nil, err
	}

	rawTx, err := rpcClient.CreateRawTransaction(inputs, transformedAddressesWithAmount, nil)
	if err != nil {
		return nil, err
	}

	res, err := rpcClient.FundRawTransaction(rawTx, btcjson.FundRawTransactionOpts{SubtractFeeFromOutputs: outputVouts}, &isWitness)
	if err != nil {
		return nil, err
	}

	signedTx, isSigned, err := rpcClient.SignRawTransactionWithWallet(res.Transaction)
	if err != nil || isSigned == false {
		return nil, err
	}

	txHash, err := rpcClient.SendRawTransaction(signedTx, false)
	if err != nil {
		return nil, err
	}

	return txHash, nil
}

func transformAddressesWithAmount(destinationAddressesWithAmount map[string]btcutil.Amount) (map[btcutil.Address]btcutil.Amount, error) {
	result := make(map[btcutil.Address]btcutil.Amount)

	for address, amount := range destinationAddressesWithAmount {
		addr, err := btcutil.DecodeAddress(address, &chaincfg.SigNetParams)
		if err != nil {
			return nil, err
		}
		result[addr] = amount
	}

	return result, nil
}

func findMatchingUTXO(rpcClient *rpcclient.Client, txId string, vout uint32) (btcjson.ListUnspentResult, error) {
	unspentTxs, err := rpcClient.ListUnspent()
	if err != nil {
		return btcjson.ListUnspentResult{}, err
	}
	var matchedUTXO btcjson.ListUnspentResult
	for _, unspentTx := range unspentTxs {
		if unspentTx.TxID == txId && unspentTx.Vout == vout {
			matchedUTXO = unspentTx
		} else {
			err = errors.New("No matching UTXO found!")
			return btcjson.ListUnspentResult{}, err
		}
	}
	return matchedUTXO, nil
}
