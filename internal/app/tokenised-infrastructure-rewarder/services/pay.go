package services

import (
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

func NewServices(apiRequester ApiRequester, provider Provider, helper Helper) *services {
	return &services{apiRequester: apiRequester, provider: provider, helper: helper}
}

type services struct {
	apiRequester ApiRequester
	provider     Provider
	helper       Helper
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
		log.Debug().Msgf("Total hash power for farm %s: %.6f", farm.SubAccountName, currentHashPowerForFarm)

		dailyFeeInSatoshis, err := s.CalculateDailyMaintenanceFee(farm, currentHashPowerForFarm)
		if err != nil {
			return err
		}

		verifiedDenomIds, err := s.verifyCollectionIds(collections)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Verified collections for farm %s: %s", farm.SubAccountName, fmt.Sprintf("%v", verifiedDenomIds))

		farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionWithNFTs(verifiedDenomIds)
		if err != nil {
			return err
		}

		s.filterExpiredNFTs(farmCollectionsWithNFTs)

		mintedHashPowerForFarm := s.SumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)
		log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.SubAccountName, mintedHashPowerForFarm)

		rewardForNftOwners := s.CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarm)
		leftoverAmountForFarmOwner := currentHashPowerForFarm - mintedHashPowerForFarm // if hash power increased or not all of it is used as NFTs
		log.Debug().Msgf("rewardForNftOwners : %s, leftoverAmountSatoshis: %.6f", rewardForNftOwners, leftoverAmountForFarmOwner)

		for _, collection := range farmCollectionsWithNFTs {
			log.Debug().Msgf("Processing collection with denomId {{%s}}..", collection.Denom.Id)
			for _, nft := range collection.Nfts {
				var nftStatistics types.NFTStatistics
				nftStatistics.TokenId = nft.Id
				nftStatistics.DenomId = collection.Denom.Id

				nftTransferHistory, err := s.getNftTransferHistory(collection.Denom.Id, nft.Id)
				if err != nil {
					return err
				}
				payoutTimes, err := sql_db.GetPayoutTimesForNFT(db, collection.Denom.Id, nft.Id)
				if err != nil {
					return err
				}
				periodStart, periodEnd, err := s.findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
				if err != nil {
					return err
				}

				nftStatistics.PayoutPeriodStart = periodStart
				nftStatistics.PayoutPeriodEnd = periodEnd

				rewardForNft := s.CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwners)

				maintenanceFee, cudoPartOfMaintenanceFee, rewardForNftAfterFee := s.calculateMaintenanceFeeForNFT(periodStart, periodEnd, dailyFeeInSatoshis, rewardForNft, destinationAddressesWithAmount, farm)
				s.payMaintenanceFeeForNFT(destinationAddressesWithAmount, maintenanceFee, farm.MaintenanceFeePayoutdAddress)
				s.payMaintenanceFeeForNFT(destinationAddressesWithAmount, cudoPartOfMaintenanceFee, config.CUDOMaintenanceFeePayoutAddress)
				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNftAfterFee)
				log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, maintenanceFee)
				log.Debug().Msgf("CUDO part (%s) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, cudoPartOfMaintenanceFee)
				nftStatistics.Reward = rewardForNftAfterFee
				nftStatistics.MaintenanceFee = maintenanceFee
				nftStatistics.CUDOPartOfMaintenanceFee = cudoPartOfMaintenanceFee

				allNftOwnersForTimePeriodWithRewardPercent, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
					nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, nftStatistics, nft.Owner)
				if err != nil {
					return err
				}
				s.distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNftAfterFee, destinationAddressesWithAmount, nftStatistics)
			}
		}

		if leftoverAmountForFarmOwner > 0 {
			rewardToReturn := s.CalculatePercent(currentHashPowerForFarm, leftoverAmountForFarmOwner, totalRewardForFarm)
			s.addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, rewardToReturn, farm.LeftoverRewardPayoutAddress)
			log.Debug().Msgf("Leftover reward with for farm with Id {%s} amount {%s} is added for return to the farm admin with address {%s}", farm.SubAccountName, rewardToReturn, farm.LeftoverRewardPayoutAddress)

		}

		if len(destinationAddressesWithAmount) == 0 {
			return fmt.Errorf("no addresses found to pay for Farm {%s}", farm.SubAccountName)
		}

		// utilised in case we had a maintenance fee greater then the nft reward.
		// keys with 0 value were added in order to have statistics even for 0 reward
		// and in order to avoid sending them as 0 - just remove them but still keep statistic
		for key := range destinationAddressesWithAmount {
			if destinationAddressesWithAmount[key] == 0 {
				delete(destinationAddressesWithAmount, key)
			}
		}

		log.Debug().Msgf("Destionation addresses with amount for farm {%s}: {%s}", farm.SubAccountName, fmt.Sprint(destinationAddressesWithAmount))
		txHash, err := s.payRewards(farm.AddressForReceivingRewardsFromPool, destinationAddressesWithAmount, rpcClient, totalRewardForFarm, farm.SubAccountName, config)
		log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash.String())

		if err != nil {
			return err
		}

		err = rpcClient.UnloadWallet(&farm.SubAccountName)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Farm Wallet: {%s} unloaded", farm.SubAccountName)

		// uncomment once backend is up and running
		// sql_tx := db.MustBegin()
		// s.saveStatistics(txHash, destinationAddressesWithAmount, statistics, sql_tx, farm.Id)
		// sql_tx.Commit()
		fmt.Print(statistics)

	}

	return nil
}

func (*services) filterExpiredNFTs(farmCollectionsWithNFTs []types.Collection) {
	for i := 0; i < len(farmCollectionsWithNFTs); i++ {
		var nonExpiredNFTs []types.NFT
		for j := 0; j < len(farmCollectionsWithNFTs[i].Nfts); j++ {
			currentNft := farmCollectionsWithNFTs[i].Nfts[j]
			if time.Now().Unix() > currentNft.DataJson.ExpirationDate {
				log.Info().Msgf("Nft with denomId {%s} and tokenId {%s} and expirationDate {%d} has expired! Skipping....", farmCollectionsWithNFTs[i].Denom.Id,
					currentNft.Id, currentNft.DataJson.ExpirationDate)
				continue
			}
			nonExpiredNFTs = append(nonExpiredNFTs, currentNft)
		}
		farmCollectionsWithNFTs[i].Nfts = nonExpiredNFTs
	}
}

func (s *services) calculateMaintenanceFeeForNFT(periodStart int64,
	periodEnd int64,
	dailyFeeInSatoshis btcutil.Amount,
	rewardForNft btcutil.Amount,
	destinationAddressesWithAmount map[string]btcutil.Amount,
	farm types.Farm) (btcutil.Amount, btcutil.Amount, btcutil.Amount) {

	totalPayoutDays := (periodStart - periodEnd) / 3600 / 24                                        // period for which we are paying the MT fee
	nftMaintenanceFeeForPayoutPeriod := btcutil.Amount(totalPayoutDays * int64(dailyFeeInSatoshis)) // the fee for the period
	if nftMaintenanceFeeForPayoutPeriod > rewardForNft {                                            // if the fee is greater - it has higher priority then the users reward
		nftMaintenanceFeeForPayoutPeriod = rewardForNft
		rewardForNft = 0
	} else {
		rewardForNft -= nftMaintenanceFeeForPayoutPeriod
	}

	partOfMaintenanceFeeForCudo := btcutil.Amount(float64(nftMaintenanceFeeForPayoutPeriod) * 10 / 100) // ex 10% from 1000 = 100
	nftMaintenanceFeeForPayoutPeriod -= partOfMaintenanceFeeForCudo

	return nftMaintenanceFeeForPayoutPeriod, partOfMaintenanceFeeForCudo, rewardForNft
}

func (s *services) payMaintenanceFeeForNFT(destinationAddressesWithAmount map[string]btcutil.Amount, maintenanceFeeAmount btcutil.Amount, farmMaintenanceFeePayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress]; ok {
		destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] += maintenanceFeeAmount
	} else {
		destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] = maintenanceFeeAmount
	}
}

func (s *services) CalculateDailyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) (btcutil.Amount, error) {
	currentYear, currentMonth, _ := time.Now().Date()
	periodLength := s.helper.DaysIn(currentMonth, currentYear)
	totalFeeBTC := farm.MonthlyMaintenanceFeeInBTC / currentHashPowerForFarm
	dailyFeeBTC := totalFeeBTC / float64(periodLength)
	dailyFeeInSatoshis, err := btcutil.NewAmount(dailyFeeBTC)
	if err != nil {
		return -1, err
	}
	return dailyFeeInSatoshis, nil
}

func (s *services) saveStatistics(txHash *chainhash.Hash, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, sql_tx *sqlx.Tx, farmId string) {
	for address, amount := range destinationAddressesWithAmount {
		sql_db.SaveDestionAddressesWithAmountHistory(sql_tx, address, amount, txHash.String(), farmId)
	}

	for _, nftStatistic := range statistics {
		sql_db.SaveNftInformationHistory(sql_tx, nftStatistic.DenomId, nftStatistic.TokenId,
			nftStatistic.PayoutPeriodStart, nftStatistic.PayoutPeriodEnd, nftStatistic.Reward, txHash.String(),
			nftStatistic.MaintenanceFee, nftStatistic.CUDOPartOfMaintenanceFee)
		for _, ownersForPeriod := range nftStatistic.NFTOwnersForPeriod {
			sql_db.SaveNFTOwnersForPeriodHistory(sql_tx, nftStatistic.DenomId, nftStatistic.TokenId,
				ownersForPeriod.TimeOwnedFrom, ownersForPeriod.TimeOwnedTo, ownersForPeriod.TotalTimeOwned,
				ownersForPeriod.PercentOfTimeOwned, ownersForPeriod.Owner, ownersForPeriod.PayoutAddress, ownersForPeriod.Reward)
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

func (s *services) findCurrentPayoutPeriod(payoutTimes []types.NFTStatistics, nftTransferHistory types.NftTransferHistory) (int64, int64, error) {
	l := len(payoutTimes)
	if l == 0 { // first time payment - start time is time of minting, end time is now
		return nftTransferHistory.Data.NestedData.Events[0].Timestamp, time.Now().Unix(), nil
	}
	return payoutTimes[l-1].PayoutPeriodEnd, time.Now().Unix(), nil // last time we paid until now
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

func (s *services) payRewards(miningPoolBTCAddress string, destinationAddressesWithAmount map[string]btcutil.Amount, rpcClient *rpcclient.Client, totalRewardForFarm btcutil.Amount, farmName string, config *infrastructure.Config) (*chainhash.Hash, error) {
	var outputVouts []int
	for i := 0; i < len(destinationAddressesWithAmount); i++ {
		outputVouts = append(outputVouts, i)
	}

	var params *chaincfg.Params
	var minConfirmation int

	if config.IsTesting {
		params = &chaincfg.SigNetParams
		minConfirmation = 1
	} else {
		params = &chaincfg.MainNetParams
		minConfirmation = 6
	}
	address, err := btcutil.DecodeAddress(miningPoolBTCAddress, params)
	if err != nil {
		return nil, err
	}

	unspentTxsForAddress, err := rpcClient.ListUnspentMinMaxAddresses(minConfirmation, 99999999, []btcutil.Address{address})
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
	transformedAddressesWithAmount, err := s.transformAddressesWithAmount(destinationAddressesWithAmount, params)
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
	if err != nil || !isSigned {
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

func (s *services) transformAddressesWithAmount(destinationAddressesWithAmount map[string]btcutil.Amount, params *chaincfg.Params) (map[btcutil.Address]btcutil.Amount, error) {
	result := make(map[btcutil.Address]btcutil.Amount)

	for address, amount := range destinationAddressesWithAmount {
		addr, err := btcutil.DecodeAddress(address, params)
		if err != nil {
			return nil, err
		}
		result[addr] = amount
	}

	return result, nil
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

type Helper interface {
	DaysIn(m time.Month, year int) int
}
