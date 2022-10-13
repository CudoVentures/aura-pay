package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

func NewServices(config *infrastructure.Config, apiRequester ApiRequester, helper Helper, btcNetworkParams *types.BtcNetworkParams) *services {
	return &services{
		config:           config,
		helper:           helper,
		btcNetworkParams: btcNetworkParams,
		apiRequester:     apiRequester,
	}
}

type services struct {
	config           *infrastructure.Config
	helper           Helper
	btcNetworkParams *types.BtcNetworkParams
	apiRequester     ApiRequester
}

// missing:
func (s *services) ProcessPayment(ctx context.Context, btcClient BtcClient, storage Storage) error {
	farms, err := s.apiRequester.GetFarms(ctx)
	if err != nil {
		return err
	}

	for _, farm := range farms {
		log.Debug().Msgf("Processing farm with name %s..", farm.SubAccountName)
		destinationAddressesWithAmount := make(map[string]btcutil.Amount)
		var statistics []types.NFTStatistics
		if _, err := btcClient.LoadWallet(farm.SubAccountName); err != nil {
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

		totalRewardForFarm, err := btcClient.GetBalance("*") // returns the total balance in satoshis
		if err != nil {
			return err
		}
		if totalRewardForFarm == 0 {
			log.Info().Msgf("reward for farm %s is 0....skipping this farm", farm.SubAccountName)
			continue
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

		dailyFeeInSatoshis, err := s.calculateDailyMaintenanceFee(farm, currentHashPowerForFarm)
		if err != nil {
			return err
		}

		verifiedDenomIds, err := s.verifyCollectionIds(ctx, collections)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Verified collections for farm %s: %s", farm.SubAccountName, fmt.Sprintf("%v", verifiedDenomIds))

		farmCollectionsWithNFTs, err := s.apiRequester.GetFarmCollectionWithNFTs(ctx, verifiedDenomIds)
		if err != nil {
			return err
		}

		filterExpiredNFTs(farmCollectionsWithNFTs)

		mintedHashPowerForFarm := s.SumMintedHashPowerForAllCollections(farmCollectionsWithNFTs)
		log.Debug().Msgf("Minted hash for farm %s: %.6f", farm.SubAccountName, mintedHashPowerForFarm)

		rewardForNftOwners := s.CalculatePercent(currentHashPowerForFarm, mintedHashPowerForFarm, totalRewardForFarm)
		leftoverHashPower := currentHashPowerForFarm - mintedHashPowerForFarm // if hash power increased or not all of it is used as NFTs
		var rewardToReturn btcutil.Amount
		// return to the farm owner whatever is left
		if leftoverHashPower > 0 {
			rewardToReturn := s.CalculatePercent(currentHashPowerForFarm, leftoverHashPower, totalRewardForFarm)
			addLeftoverRewardToFarmOwner(destinationAddressesWithAmount, rewardToReturn, farm.LeftoverRewardPayoutAddress)
		}
		log.Debug().Msgf("rewardForNftOwners : %s, rewardToReturn: %s, farm: {%s}", rewardForNftOwners, rewardToReturn, farm.SubAccountName)

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
				periodStart, periodEnd, err := findCurrentPayoutPeriod(payoutTimes, nftTransferHistory)
				if err != nil {
					return err
				}

				nftStatistics.PayoutPeriodStart = periodStart
				nftStatistics.PayoutPeriodEnd = periodEnd

				rewardForNft := s.CalculatePercent(mintedHashPowerForFarm, nft.DataJson.HashRateOwned, rewardForNftOwners)

				maintenanceFee, cudoPartOfMaintenanceFee, rewardForNftAfterFee := calculateMaintenanceFeeForNFT(periodStart, periodEnd, dailyFeeInSatoshis, rewardForNft, destinationAddressesWithAmount, farm)
				payMaintenanceFeeForNFT(destinationAddressesWithAmount, maintenanceFee, farm.MaintenanceFeePayoutdAddress)
				payMaintenanceFeeForNFT(destinationAddressesWithAmount, cudoPartOfMaintenanceFee, s.config.CUDOMaintenanceFeePayoutAddress)
				log.Debug().Msgf("Reward for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, rewardForNftAfterFee)
				log.Debug().Msgf("Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", collection.Denom.Id, nft.Id, maintenanceFee)
				log.Debug().Msgf("CUDO part (%.6f) of Maintenance fee for nft with denomId {%s} and tokenId {%s} is %s", s.config.CUDOMaintenanceFeePercent, collection.Denom.Id, nft.Id, cudoPartOfMaintenanceFee)
				nftStatistics.Reward = rewardForNftAfterFee
				nftStatistics.MaintenanceFee = maintenanceFee
				nftStatistics.CUDOPartOfMaintenanceFee = cudoPartOfMaintenanceFee

				allNftOwnersForTimePeriodWithRewardPercent, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(
					ctx, nftTransferHistory, collection.Denom.Id, nft.Id, periodStart, periodEnd, nftStatistics, nft.Owner, s.config.Network)
				if err != nil {
					return err
				}

				distributeRewardsToOwners(allNftOwnersForTimePeriodWithRewardPercent, rewardForNftAfterFee, destinationAddressesWithAmount, nftStatistics)
			}
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
		txHash, err := s.payRewards(farm.AddressForReceivingRewardsFromPool, destinationAddressesWithAmount, totalRewardForFarm, farm.SubAccountName, btcClient)
		if err != nil {
			return err
		}
		log.Debug().Msgf("Tx sucessfully sent! Tx Hash {%s}", txHash.String())

		if err := storage.SaveStatistics(ctx, destinationAddressesWithAmount, statistics, txHash.String(), farm.Id); err != nil {
			log.Error().Msgf("Failed to save statistics: %s", txHash.String(), err)
		}
	}

	return nil
}

func filterExpiredNFTs(farmCollectionsWithNFTs []types.Collection) {
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

func calculateMaintenanceFeeForNFT(periodStart int64,
	periodEnd int64,
	dailyFeeInSatoshis btcutil.Amount,
	rewardForNft btcutil.Amount,
	destinationAddressesWithAmount map[string]btcutil.Amount,
	farm types.Farm) (btcutil.Amount, btcutil.Amount, btcutil.Amount) {

	//TODO: What happens if the payout comes in less then 24h? Probably it is better to calculate it hourly?
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

func payMaintenanceFeeForNFT(destinationAddressesWithAmount map[string]btcutil.Amount, maintenanceFeeAmount btcutil.Amount, farmMaintenanceFeePayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress]; ok {
		destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] += maintenanceFeeAmount
	} else {
		destinationAddressesWithAmount[farmMaintenanceFeePayoutAddress] = maintenanceFeeAmount
	}
}

func (s *services) calculateDailyMaintenanceFee(farm types.Farm, currentHashPowerForFarm float64) (btcutil.Amount, error) {
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

func (s *services) getNftTransferHistory(ctx context.Context, collectionDenomId, nftId string) (types.NftTransferHistory, error) {
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

func findCurrentPayoutPeriod(payoutTimes []types.NFTStatistics, nftTransferHistory types.NftTransferHistory) (int64, int64, error) {
	l := len(payoutTimes)
	if l == 0 { // first time payment - start time is time of minting, end time is now
		return nftTransferHistory.Data.NestedData.Events[0].Timestamp, time.Now().Unix(), nil
	}
	return payoutTimes[l-1].PayoutPeriodEnd, time.Now().Unix(), nil // last time we paid until now
}

func addLeftoverRewardToFarmOwner(destinationAddressesWithAmount map[string]btcutil.Amount, leftoverReward btcutil.Amount, farmDefaultPayoutAddress string) {
	if _, ok := destinationAddressesWithAmount[farmDefaultPayoutAddress]; ok {
		// log to statistics here if we are doing accumulation send for an nft
		destinationAddressesWithAmount[farmDefaultPayoutAddress] += leftoverReward
	} else {
		destinationAddressesWithAmount[farmDefaultPayoutAddress] = leftoverReward
	}
}

func (s *services) verifyCollectionIds(ctx context.Context, collections types.CollectionData) ([]string, error) {
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

func (s *services) payRewards(miningPoolBTCAddress string, destinationAddressesWithAmount map[string]btcutil.Amount, totalRewardForFarm btcutil.Amount, farmName string, btcClient BtcClient) (*chainhash.Hash, error) {
	var outputVouts []int
	for i := 0; i < len(destinationAddressesWithAmount); i++ {
		outputVouts = append(outputVouts, i)
	}

	address, err := btcutil.DecodeAddress(miningPoolBTCAddress, s.btcNetworkParams.ChainParams)
	if err != nil {
		return nil, err
	}
	unspentTxsForAddress, err := btcClient.ListUnspentMinMaxAddresses(s.btcNetworkParams.MinConfirmations, 99999999, []btcutil.Address{address})
	if err != nil {
		return nil, err
	}
	if len(unspentTxsForAddress) > 1 {
		return nil, fmt.Errorf("farm {%s} has more then one unspent transaction", farmName)
	}

	// err = EnsureTotalRewardIsEqualToAmountBeingSent(destinationAddressesWithAmount, totalRewardForFarm, farmName)
	// if err != nil {
	// 	return nil, err
	// }

	inputTx := unspentTxsForAddress[0]
	if inputTx.Amount != totalRewardForFarm.ToBTC() {
		return nil, fmt.Errorf("input tx with hash {%s} has different amount (%v) then the total reward ({%v}) for farm {%s} ", inputTx.TxID, inputTx.Amount, totalRewardForFarm.ToBTC(), farmName)
	}

	txInput := btcjson.TransactionInput{Txid: inputTx.TxID, Vout: inputTx.Vout}
	inputs := []btcjson.TransactionInput{txInput}

	isWitness := false
	transformedAddressesWithAmount, err := transformAddressesWithAmount(destinationAddressesWithAmount, s.btcNetworkParams.ChainParams)
	if err != nil {
		return nil, err
	}

	rawTx, err := btcClient.CreateRawTransaction(inputs, transformedAddressesWithAmount, nil)
	if err != nil {
		return nil, err
	}

	res, err := btcClient.FundRawTransaction(rawTx, btcjson.FundRawTransactionOpts{SubtractFeeFromOutputs: outputVouts}, &isWitness)
	if err != nil {
		return nil, err
	}

	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return nil, err
	}

	signedTx, isSigned, err := btcClient.SignRawTransactionWithWallet(res.Transaction)
	if err != nil {
		return nil, err
	}

	if !isSigned {
		return nil, fmt.Errorf("failed to sign transaction: %+v", res.Transaction)
	}
	err = btcClient.WalletLock()
	if err != nil {
		return nil, err
	}

	txHash, err := btcClient.SendRawTransaction(signedTx, false)
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

func transformAddressesWithAmount(destinationAddressesWithAmount map[string]btcutil.Amount, params *chaincfg.Params) (map[btcutil.Address]btcutil.Amount, error) {
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
	GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error)

	GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error)

	GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error)

	GetFarmCollectionsFromHasura(ctx context.Context, farmId string) (types.CollectionData, error)

	GetFarms(ctx context.Context) ([]types.Farm, error)

	VerifyCollection(ctx context.Context, denomId string) (bool, error)

	GetFarmCollectionWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error)
}

type Provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}

type BtcClient interface {
	GetBalance(account string) (btcutil.Amount, error)

	LoadWallet(walletName string) (*btcjson.LoadWalletResult, error)

	UnloadWallet(walletName *string) error

	ListUnspentMinMaxAddresses(minConf, maxConf int, addrs []btcutil.Address) ([]btcjson.ListUnspentResult, error)

	CreateRawTransaction(inputs []btcjson.TransactionInput, amounts map[btcutil.Address]btcutil.Amount, lockTime *int64) (*wire.MsgTx, error)

	FundRawTransaction(tx *wire.MsgTx, opts btcjson.FundRawTransactionOpts, isWitness *bool) (*btcjson.FundRawTransactionResult, error)

	SignRawTransactionWithWallet(tx *wire.MsgTx) (*wire.MsgTx, bool, error)

	SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error)

	ListUnspent() ([]btcjson.ListUnspentResult, error)

	WalletPassphrase(passphrase string, timeoutSecs int64) error

	WalletLock() error
}

type Storage interface {
	GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error)

	SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, txHash, farmId string) error
}

type Helper interface {
	DaysIn(m time.Month, year int) int
}
