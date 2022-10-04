package services

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/sql_db"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

func NewServices(apiRequester ApiRequester, provider Provider) *services {
	return &services{apiRequester: apiRequester, provider: provider}
}

type services struct {
	apiRequester ApiRequester
	provider     Provider
}

// missing:
func (s *services) ProcessPayment(config *infrastructure.Config) error {
	// bitcoin rpc client init
	rpcClient, err := s.provider.InitBtcRpcClient()
	if err != nil {
		return err
	}
	defer rpcClient.Shutdown()

	db, err := s.provider.InitDBConnection()
	if err != nil {
		return err
	}

	farms, err := s.apiRequester.GetFarms()
	if err != nil {
		return err
	}

	for _, farm := range farms {
		log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
		destinationAddressesWithAmount := make(map[string]btcutil.Amount)
		var statistics []types.NFTStatistics
		_, err := rpcClient.LoadWallet(farm.SubAccountName)
		log.Debug().Msgf("Farm Wallet: {%s} loaded", farm.SubAccountName)
		if err != nil {
			return err
		}
		totalRewardForFarm, err := rpcClient.GetBalance("*") // returns the total balance in satoshis
		if err != nil {
			return err
		}
		if totalRewardForFarm == 0 {
			return fmt.Errorf("reward for farm %s is 0....skipping this farm", farm.SubAccountName)
		}
		log.Debug().Msgf("Total reward for farm %s: %s", farm.SubAccountName, totalRewardForFarm)
		collections, err := s.apiRequester.GetFarmCollectionsFromHasura(farm.SubAccountName)
		if err != nil {
			return err
		}

		var currentHashPowerForFarm float64
		if config.IsTesting {
			currentHashPowerForFarm = 1200 // hardcoded for testing & QA
		} else {
			currentHashPowerForFarm, err = s.apiRequester.GetFarmTotalHashPowerFromPoolToday(farm.SubAccountName, time.Now().AddDate(0, 0, -1).UTC().Format("2006-09-23"))
			if err != nil {
				return err
			}
		}
		log.Debug().Msgf("Total hash power for farm %s: %s", farm.SubAccountName, currentHashPowerForFarm)

		verifiedDenomIds, err := s.verifyCollectionIds(collections)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Verified collections for farm %s: %s", farm.SubAccountName, fmt.Sprintf("%v", verifiedDenomIds))

		farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionWithNFTs(verifiedDenomIds)
		if err != nil {
			return err
		}
		mintedHashPowerForFarm := s.SumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)
		log.Debug().Msgf("Minted hash for farm %s: %s", farm.SubAccountName, mintedHashPowerForFarm)

		hasHashPowerIncreased, leftoverAmount := s.hasHashPowerIncreased(currentHashPowerForFarm, mintedHashPowerForFarm)
		log.Debug().Msgf("hasHashPowerIncreased : %s, leftoverAmount: ", hasHashPowerIncreased, leftoverAmount)

		rewardForNftOwners := totalRewardForFarm
		if hasHashPowerIncreased {
			rewardForNftOwners = s.CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarm)
			if err != nil {
				return err
			}
		}
		log.Debug().Msgf("Reward for nft owners : %s", rewardForNftOwners)

		for _, collection := range farmCollectionsWithNFTs {
			log.Debug().Msgf("Processing collection with denomId %s..", collection.Denom.Id)
			for _, nft := range collection.Nfts {
				if time.Now().Unix() > nft.DataJson.ExpirationDate {
					log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%s} has expired! Skipping....", collection.Denom.Id, nft.Id, nft.DataJson.ExpirationDate)
					continue
				}
				var nftStatistics types.NFTStatistics
				nftStatistics.TokenId = nft.Id
				nftStatistics.DenomId = collection.Denom.Id

				rewardForNft := s.CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwners)
				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNft)
				if err != nil {
					return err
				}
				nftStatistics.RewardForNFT = rewardForNft

				nftTransferHistory, err := s.getNftTransferHistory(collection.Denom.Id, nft.Id)
				if err != nil {
					return err
				}
				payoutTimes, err := sql_db.GetPayoutTimesForNFT(db, nft.Id)
				if err != nil {
					return err
				}
				periodStart, periodEnd, err := s.findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
				if err != nil {
					return err
				}
				nftStatistics.PayoutPeriodStart = periodStart
				nftStatistics.PayoutPeriodEnd = periodEnd

				allNftOwnersForTimePeriodWithRewardPercent, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, nftStatistics, nft.Owner)
				if err != nil {
					return err
				}
				s.distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNft, destinationAddressesWithAmount, nftStatistics)

				tx := db.MustBegin()
				sql_db.SetPayoutTimesForNFT(tx, collection.Denom.Id, nft.Id, time.Now().Unix(), rewardForNft.ToBTC())
				tx.Commit()
			}
		}

		err = rpcClient.UnloadWallet(&farm.SubAccountName)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Farm Wallet: {%s} unloaded", farm.SubAccountName)

		if hasHashPowerIncreased {
			leftoverReward := s.CalculatePercent(currentHashPowerForFarm, leftoverAmount, totalRewardForFarm)
			if err != nil {
				return err
			}
			s.addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, leftoverReward, farm.DefaultBTCPayoutAddress)
			log.Debug().Msgf("Leftover reward with for farm with Id {%s} amount {%s} is added for return to the farm admin with address {%s}", farm.SubAccountName, leftoverReward, farm.DefaultBTCPayoutAddress)
		}

		if len(destinationAddressesWithAmount) == 0 {
			return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.SubAccountName)
		}
		log.Debug().Msgf("Destionation addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmount))
		txHash, err := s.payRewards(farm.MiningPoolBTCAddress, destinationAddressesWithAmount, rpcClient, totalRewardForFarm, farm.SubAccountName)
		if err != nil {
			return err
		}

		sql_tx := db.MustBegin()
		s.saveStatistics(txHash, destinationAddressesWithAmount, statistics, sql_tx, farm.Id)
		sql_tx.Commit()

	}

	return nil
}

func (s *services) saveStatistics(txHash *chainhash.Hash, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, sql_tx *sqlx.Tx, farmId string) {
	for address, amount := range destinationAddressesWithAmount {
		sql_db.SaveDestionAddressesWithAmountHistory(sql_tx, address, amount, txHash.String(), farmId)
	}

	for _, nftStatistic := range statistics {
		sql_db.SaveNftInformationHistory(sql_tx, nftStatistic.DenomId, nftStatistic.TokenId, nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.RewardForNFT, txHash.String())
		for _, ownersForPeriod := range nftStatistic.NFTOwnersForPeriod {
			sql_db.SaveNFTOwnersForPeriodHistory(sql_tx, nftStatistic.DenomId, nftStatistic.TokenId, ownersForPeriod.TimeOwnedFrom, ownersForPeriod.TimeOwnedTo, ownersForPeriod.TotalTimeOwned, ownersForPeriod.PercentOfTimeOwned, ownersForPeriod.Owner, ownersForPeriod.PayoutAddress, ownersForPeriod.Reward)
		}
	}
}

func (s *services) getNftTransferHistory(collectionDenomId, nftId string) (types.NftTransferHistory, error) {
	nftTransferHistory, err := s.apiRequester.GetNftTransferHistory(collectionDenomId, nftId, 1) // all transfer events
	if err != nil {
		return types.NftTransferHistory{}, err
	}

	// sort in ascending order by timestamp
	sort.Slice(nftTransferHistory.Data.NestedData.Events, func(i, j int) bool {
		return nftTransferHistory.Data.NestedData.Events[i].Timestamp < nftTransferHistory.Data.NestedData.Events[j].Timestamp
	})

	return nftTransferHistory, nil
}

func (s *services) findCurrentPayoutPeriod(payoutTimes []types.NFTPayoutTime, nftTransferHistory types.NftTransferHistory) (int64, int64, error) {
	if len(payoutTimes) == 0 { // first time payment - start time is time of minting, end time is now
		return nftTransferHistory.Data.NestedData.Events[0].Timestamp, time.Now().Unix(), nil
	}

	if len(payoutTimes) == 1 {
		return payoutTimes[0].PayoutTimeAt, time.Now().Unix(), nil
	}

	l := len(payoutTimes)

	return payoutTimes[l-2].PayoutTimeAt, payoutTimes[l-1].PayoutTimeAt, nil

}

func (s *services) hasHashPowerIncreased(currentHashPowerForFarm float64, mintedHashPowerForFarm float64) (bool, float64) {
	if currentHashPowerForFarm > mintedHashPowerForFarm {
		leftOverAmount := currentHashPowerForFarm - mintedHashPowerForFarm
		return true, leftOverAmount
	}

	return false, -1
}

func (s *services) addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]btcutil.Amount, leftoverReward btcutil.Amount, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] += leftoverReward
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func (s *services) verifyCollectionIds(collections types.CollectionData) ([]string, error) {
	var verifiedCollectionIds []string
	for _, collection := range collections.Data.DenomsByDataProperty {
		isVerified, err := s.apiRequester.VerifyCollection(collection.Id)
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

func (s *services) sumCollectionHashPower(collectionNFTs []types.NFT) float64 {
	var collectionHashPower float64
	for _, nft := range collectionNFTs {
		collectionHashPower += nft.DataJson.HashRateOwned
	}
	return collectionHashPower
}

func (s *services) distributeRewardsToOwners(ownersWithPercentOwned map[string]float64, nftPayoutAmount btcutil.Amount, destinationAddressesWithAmount map[string]btcutil.Amount, statistics types.NFTStatistics) {
	for nftPayoutAddress, percentFromReward := range ownersWithPercentOwned {
		payoutAmount := nftPayoutAmount.MulF64(percentFromReward / 100)    // TODO: Change this to normal float64 percent as MULF64 is rounding
		if _, ok := destinationAddressesWithAmount[nftPayoutAddress]; ok { // if the address is already there then increment the amount it will receive for its next nft
			destinationAddressesWithAmount[nftPayoutAddress] += payoutAmount
		} else {
			destinationAddressesWithAmount[nftPayoutAddress] = payoutAmount
		}
		s.addPaymentAmountToStatistics(payoutAmount, nftPayoutAddress, statistics)
	}
}

func (s *services) addPaymentAmountToStatistics(amount btcutil.Amount, payoutAddress string, nftStatistics types.NFTStatistics) {
	for i := 0; i < len(nftStatistics.NFTOwnersForPeriod); i++ {
		additionalData := nftStatistics.NFTOwnersForPeriod[i]
		if additionalData.PayoutAddress == payoutAddress {
			additionalData.Reward = amount
		}
	}
}

func (s *services) payRewards(miningPoolBTCAddress string, destinationAddressesWithAmount map[string]btcutil.Amount, rpcClient *rpcclient.Client, totalRewardForFarm btcutil.Amount, farmName string) (*chainhash.Hash, error) {
	var outputVouts []int
	for i := 0; i < len(destinationAddressesWithAmount); i++ {
		outputVouts = append(outputVouts, i)
	}

	// todo: add if else config for testnet, signet and mainnet
	addr, err := btcutil.DecodeAddress(miningPoolBTCAddress, &chaincfg.SigNetParams)
	if err != nil {
		return nil, err
	}
	unspentTxsForAddress, err := rpcClient.ListUnspentMinMaxAddresses(6, 99999999, []btcutil.Address{addr})
	if err != nil {
		return nil, err
	}
	if len(unspentTxsForAddress) > 1 {
		return nil, fmt.Errorf("farm {%s} has more then one unspent transaction", farmName)
	}

	err = EnsureTotalRewardIsEqualToAmountBeingSent(destinationAddressesWithAmount, totalRewardForFarm, farmName)
	if err != nil {
		return nil, err
	}

	inputTx := unspentTxsForAddress[0]
	if inputTx.Amount != totalRewardForFarm.ToBTC() {
		err = fmt.Errorf("input tx with hash {%s} has different amount (%v) then the total reward ({%v}) for farm {%s} ", inputTx.TxID, inputTx.Amount, totalRewardForFarm.ToBTC(), farmName)
		return nil, err

	}

	txInput := btcjson.TransactionInput{Txid: inputTx.TxID, Vout: inputTx.Vout}
	inputs := []btcjson.TransactionInput{txInput}

	isWitness := false
	transformedAddressesWithAmount, err := s.transformAddressesWithAmount(destinationAddressesWithAmount)
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

func EnsureTotalRewardIsEqualToAmountBeingSent(destinationAddressesWithAmount map[string]btcutil.Amount, totalRewardForFarm btcutil.Amount, farmName string) error {
	var totalRewardToSend btcutil.Amount

	for _, v := range destinationAddressesWithAmount {
		totalRewardToSend += v
	}
	if totalRewardToSend != totalRewardForFarm {
		return fmt.Errorf("mismatch between totalRewardAForFarm {%s} and totalRewardToSend {%s}. Farm name {%s}", totalRewardForFarm, totalRewardToSend, farmName)

	}

	return nil

}

func (s *services) transformAddressesWithAmount(destinationAddressesWithAmount map[string]btcutil.Amount) (map[btcutil.Address]btcutil.Amount, error) {
	result := make(map[btcutil.Address]btcutil.Amount)

	for address, amount := range destinationAddressesWithAmount {
		// todo: add test param for signet, testnet and mainnet
		addr, err := btcutil.DecodeAddress(address, &chaincfg.SigNetParams)
		if err != nil {
			return nil, err
		}
		result[addr] = amount
	}

	return result, nil
}

func (s *services) findMatchingUTXO(rpcClient *rpcclient.Client, txId string, vout uint32) (btcjson.ListUnspentResult, error) {
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

type ApiRequester interface {
	GetPayoutAddressFromNode(cudosAddress string, network string, tokenId string, denomId string) (string, error)

	GetNftTransferHistory(collectionDenomId string, nftId string, fromTimestamp int64) (types.NftTransferHistory, error)

	GetFarmTotalHashPowerFromPoolToday(farmName string, sinceTimestamp string) (float64, error)

	GetFarmCollectionsFromHasura(farmId string) (types.CollectionData, error)

	GetFarms() ([]types.Farm, error)

	VerifyCollection(denomId string) (bool, error)

	GetFarmCollectionWithNFTs(denomIds []string) ([]types.Collection, error)
}

type Provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}
