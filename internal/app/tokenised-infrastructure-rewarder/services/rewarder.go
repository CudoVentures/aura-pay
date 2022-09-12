package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/rs/zerolog/log"
)

func ProcessPayment() error {
	// bitcoin rpc client init
	rpcClient, err := infrastructure.InitBtcRpcClient()
	if err != nil {
		return err
	}
	defer rpcClient.Shutdown()

	farms, err := requesters.GetFarms()
	if err != nil {
		return err
	}

	for _, farm := range farms {
		log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
		destinationAddressesWithAmount := make(map[string]btcutil.Amount)
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

		hasHashPowerIncreased, leftoverAmount, err := HasHashPowerIncreased(currentHashPowerForFarm, mintedHashPowerForFarm)
		if err != nil {
			return err
		}
		log.Debug().Msgf("hasHashPowerIncreased : %s, leftoverAmount: ", hasHashPowerIncreased, leftoverAmount)

		rewardForNftOwners := totalRewardForFarm
		if hasHashPowerIncreased {
			rewardForNftOwners, err = CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, float64(totalRewardForFarm))
		}
		log.Debug().Msgf("Reward for nft owners : %s", rewardForNftOwners)

		for _, collection := range farmCollectionsWithNFTs {
			log.Debug().Msgf("Processing collection with denomId %s..", collection.Denom.Id)
			for _, nft := range collection.Nfts {
				if time.Now().Unix() > nft.DataJson.ExpirationDate {
					log.Debug().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%s} has expired! Skipping....", collection.Denom.Id, nft.Id, nft.DataJson.ExpirationDate)
					continue
				}
				rewardForNft, err := CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, float64(rewardForNftOwners))
				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNft)
				if err != nil {
					return err
				}

				allNftOwnersForTimePeriodWithRewardPercent, err := calculateNftOwnersForTimePeriodWithRewardPercent(collection.Denom.Id, nft.Id, 0, 0)
				if err != nil {
					return err
				}
				distributeRewardsToOwnersNew(allNftOwnersForTimePeriodWithRewardPercent, rewardForNft, destinationAddressesWithAmount)
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

		//TODO:how to get the correct tx?
		payRewards("bf4961e4259c9d9c7bdf4862fdeeb0337d06479737c2c63e4af360913b11277f", uint32(1), farm.BTCWallet, destinationAddressesWithAmount)

	}

	return nil
}

func HasHashPowerIncreased(currentHashPowerForFarm float64, mintedHashPowerForFarm float64) (bool, float64, error) {
	if currentHashPowerForFarm > mintedHashPowerForFarm {
		leftOverAmount := currentHashPowerForFarm - mintedHashPowerForFarm
		return true, leftOverAmount, nil
	}

	return false, -1, nil
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
		collectionHashPower += nft.DataJson.HashRateOwned
	}
	return collectionHashPower
}

func distributeRewardsToOwnersNew(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)    // TODO: Change this to normal float64 percent as MULF64 is rounding
		if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
			// log to statistics here if we are doing accumulation send for an nft
			destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount

		} else {
			destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
		}
	}
}

func payRewards(inputTxId string, inputTxVout uint32, walletName string, destinationAddressesWithAmount map[string]btcutil.Amount) (*chainhash.Hash, error) {
	rpcClient, err := infrastructure.InitBtcRpcClient()
	if err != nil {
		return nil, err
	}
	defer rpcClient.Shutdown()

	// todo: add as params and fetch it from pool
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
