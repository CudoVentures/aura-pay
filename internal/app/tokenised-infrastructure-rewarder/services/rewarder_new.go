package services

import (
	"fmt"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
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
		// fetch reward and hash power for farm
		destinationAddressesWithAmount := make(map[string]btcutil.Amount)
		log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
		totalRewardForFarm, err := rpcClient.GetBalance(farm.SubAccountName)
		if err != nil {
			return err
		}
		if totalRewardForFarm == 0 {
			return fmt.Errorf("Reward for farm %s is 0....exiting", farm.SubAccountName)
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
		farmCollectionsWithNFTs, err := requesters.GetFarmCollectionWithNFTs(verifiedDenomIds)
		if err != nil {
			return err
		}
		mintedHashPowerForFarm := SumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)

		for _, collection := range farmCollectionsWithNFTs {
			log.Debug().Msgf("Processing collection with denomId %s..", collection.Denom.Id)
			for _, nft := range collection.Nfts {
				if currentHashPowerForFarm <= mintedHashPowerForFarm {
					rewardForNft, err := CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, float64(totalRewardForFarm))
					if err != nil {
						return err
					}
					allNftOwnersForTimePeriodWithRewardPercent, err := getNftOwnersForTimePeriodWithRewardPercent(nft.Id, collection.Denom.Id, 0, 0) // TODO: Fetch periodStart and end from transfer events
					distributeRewardsToOwnersNew(allNftOwnersForTimePeriodWithRewardPercent, rewardForNft, destinationAddressesWithAmount)
				} else { // the hash power of the farm has increased but the new hash power is not minted to a nft - so return the leftover to a default address
					// the reward for nft owners is the part of mintedHashPowerForFarm from currentHashPowerForFarm as percent from the total reward
					partOfRewardForNftOwners, err := CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, float64(totalRewardForFarm))
					if err != nil {
						return err
					}
					rewardForNft, err := CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, float64(partOfRewardForNftOwners))

					allNftOwnersForTimePeriodWithRewardPercent, err := getNftOwnersForTimePeriodWithRewardPercent(nft.Id, collection.Denom.Id, 0, 0) // TODO: Fetch periodStart and end from transfer events
					distributeRewardsToOwnersNew(allNftOwnersForTimePeriodWithRewardPercent, rewardForNft, destinationAddressesWithAmount)

					leftoverReward, err := CalculatePercent(currentHashPowerForFarm, currentHashPowerForFarm-mintedHashPowerForFarm, float64(totalRewardForFarm))

					//  we calculate the percent of hash power of each nft that has been minted based on the total current minted hash power (ex: 100+100+100...)
					//  then we find what is the reward (out of the totalReward) for the current minted hash power in this way: currentMintedHashPowerAsPercent = current minted hash power as percent from totalCurrentFarmHashPower (ex 1000 from 2000 = 50%)
					//  then we pay the percent for each nft from the currentMintedHashPowerAsPercent from totalReward ( ex: 50% from 2000=1000, for 10 nfts: 10% from 1000)
					//  then after we have finished paying the nft owners take the difference between totalCurrentFarmHashPower - current minted hash power
					//  then we calculate this difference as percent from the totalCurrentFarmHashPower (ex: 40% from 2000)
					//  then we take this percent from the totalReward to the owner of the farm collection to a default payout address provided by him
				}
			}

		}
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
			log.Debug().Msgf("Collection with denomId %s is not verified", collection.Id)
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
