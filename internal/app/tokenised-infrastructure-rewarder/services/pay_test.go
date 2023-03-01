package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/sql_db"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcessPayment(t *testing.T) {
	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            20,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	s := NewPayService(config, setupMockApiRequester(t), &mockHelper{}, btcNetworkParams)
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClient(), setupMockStorage()))
}

func TestProcessFarm(t *testing.T) {
	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            20,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	s := NewPayService(config, setupMockApiRequester(t), &mockHelper{}, btcNetworkParams)

	farms, err := setupMockStorage().GetApprovedFarms(context.Background())
	require.Equal(t, err, nil, "Get farms returned error")

	require.NoError(t, s.processFarm(context.Background(), setupMockBtcClient(), setupMockStorage(), farms[0]))
}

func TestPayService_ProcessPayment_Threshold(t *testing.T) {
	skipDBTests(t)

	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            2,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
		DbDriverName:               "postgres",
		DbUser:                     "postgresUser",
		DbPassword:                 "mysecretpassword",
		DbHost:                     "127.0.0.1",
		DbPort:                     "5432",
		DbName:                     "aura-pay-test-db",
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	dbStorage, sqlxDB := setupInMemoryStorage(config)
	defer func() {
		tearDownDatabase(sqlxDB)
	}()

	err := dbStorage.UpdateThresholdStatus(context.Background(), "3", 1, map[string]decimal.Decimal{}, 1)
	if err != nil {
		panic(err)
	}

	mockAPIRequester := setupMockApiRequester(t)
	// cudo_maintenance_fee_payout_addr and maintenance_fee_payout_address_1 are below threshold of 0.01 with values 5.928e-05
	mockAPIRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1": 3,
		"nft_minter_payout_addr":           0.1973688,
		"nft_owner_2_payout_addr":          0.55251264,
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	s := NewPayService(config, mockAPIRequester, &mockHelper{}, btcNetworkParams)

	require.NoError(t, s.Execute(context.Background(), setupMockBtcClient(), dbStorage))
	processTx1, _ := dbStorage.GetUTXOTransaction(context.Background(), "1")
	require.Equal(t, true, processTx1.Processed)
	processTx2, _ := dbStorage.GetUTXOTransaction(context.Background(), "2")
	require.Equal(t, true, processTx2.Processed)
	processTx4, _ := dbStorage.GetUTXOTransaction(context.Background(), "4")
	require.Equal(t, true, processTx4.Processed)

	amountAccumulatedBTC, _ := dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "maintenance_fee_payout_address_1", 1)
	require.Equal(t, float64(5928), amountAccumulatedBTC)
	amountAccumulatedBTC, _ = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "cudo_maintenance_fee_payout_address_1", 1)
	require.Equal(t, float64(10210010), amountAccumulatedBTC)
	amountAccumulatedBTC, _ = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "nft_minter_payout_addr", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)
	amountAccumulatedBTC, _ = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "nft_owner_2_payout_addr", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)
	amountAccumulatedBTC, _ = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "leftover_reward_payout_address_1", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)
}

func TestPayService_ProcessPayment_Mint_Between_Payments(t *testing.T) {
	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            20,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	mockAPIRequester := setupMockApiRequester(t)

	// minted at 1/2 of the period between payments
	farm1Denom1Nft1TransferHistoryJSON := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_minter",
						"from": "0x0",
						"timestamp": 1665820278
					}
				]
			}
		}
	}`

	var farm1Denom1Nft1TransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Nft1TransferHistoryJSON), &farm1Denom1Nft1TransferHistory))

	// call it once to clear mock
	mockAPIRequester.GetNftTransferHistory(context.Background(), "farm_1_denom_1", "1", 0)
	mockAPIRequester.On("GetNftTransferHistory", mock.Anything, "farm_1_denom_1", "1", mock.Anything).Return(farm1Denom1Nft1TransferHistory, nil).Once()

	collectionAllocationAmount := decimal.NewFromFloat(4)
	leftoverAmount := decimal.NewFromFloat(3)
	cudoMaintenanceFee := decimal.NewFromFloat(1.25012768)
	nftMinterAmount, _ := decimal.NewFromString("1.9997446236559112")
	cudoPartOfReward := decimal.NewFromFloat(1.25)
	cudoPartOfMaintenanceFee, _ := decimal.NewFromString("0.0001276881720444")
	maintenanceFeeAddress1Amount, _ := decimal.NewFromString("0.0001276881720444")

	// maintenance_fee_payout_address_1 is below threshold of 0.01 with values 5.928e-05
	mockAPIRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1":      leftoverAmount.InexactFloat64(),
		"cudo_maintenance_fee_payout_address_1": cudoMaintenanceFee.InexactFloat64(),
		"nft_minter_payout_addr":                nftMinterAmount.RoundFloor(8).InexactFloat64(),
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	storage := setupMockStorage()

	storage.On("SaveStatistics", mock.Anything,
		mock.MatchedBy(func(farmPayment types.FarmPayment) bool {
			return farmPayment.AmountBTC.Equal(decimal.NewFromFloat(6.25))
		}),
		mock.MatchedBy(func(amountInfoMap map[string]types.AmountInfo) bool {
			return amountInfoMap["leftover_reward_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["nft_minter_payout_addr"].ThresholdReached == true &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["maintenance_fee_payout_address_1"].ThresholdReached == false &&

				amountInfoMap["leftover_reward_payout_address_1"].Amount.Equals(leftoverAmount.RoundFloor(8)) &&
				amountInfoMap["nft_minter_payout_addr"].Amount.Equals(nftMinterAmount.RoundFloor(8)) &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].Amount.Equals(cudoMaintenanceFee.RoundFloor(8)) &&
				amountInfoMap["maintenance_fee_payout_address_1"].Amount.Equals(maintenanceFeeAddress1Amount.RoundFloor(8))
		}),
		mock.MatchedBy(func(collectionAllocations []types.CollectionPaymentAllocation) bool {
			collectionPartOfFarm := decimal.NewFromFloat(0.8)

			return collectionAllocations[0].FarmId == 1 &&
				collectionAllocations[0].CollectionId == 1 &&
				collectionAllocations[0].CollectionAllocationAmount.Equals(collectionAllocationAmount) &&
				collectionAllocations[0].CUDOGeneralFee.Equals(cudoPartOfReward.Mul(collectionPartOfFarm)) &&
				collectionAllocations[0].CUDOMaintenanceFee.Equals(cudoPartOfMaintenanceFee) &&
				collectionAllocations[0].FarmMaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				collectionAllocations[0].FarmUnsoldLeftovers.Equals(collectionAllocationAmount.Sub(nftMinterAmount).Sub(cudoPartOfMaintenanceFee).Sub(maintenanceFeeAddress1Amount))
		}),
		mock.MatchedBy(func(nftStatistics []types.NFTStatistics) bool {
			nftStatistic := nftStatistics[0]
			nftOwnerStat1 := nftStatistic.NFTOwnersForPeriod[0]

			nftStatisticCorrect := nftStatistic.TokenId == "1" &&
				nftStatistic.DenomId == "farm_1_denom_1" &&
				nftStatistic.PayoutPeriodStart == 1665820278 &&
				nftStatistic.PayoutPeriodEnd == 1666641078 &&
				nftStatistic.Reward.Equals(nftMinterAmount) &&
				nftStatistic.MaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				nftStatistic.CUDOPartOfMaintenanceFee.Equals(cudoPartOfMaintenanceFee)

			nftOwnerStat1Correct := nftOwnerStat1.TimeOwnedFrom == 1665820278 &&
				nftOwnerStat1.TimeOwnedTo == 1666641078 &&
				nftOwnerStat1.TotalTimeOwned == 820800 &&
				nftOwnerStat1.PercentOfTimeOwned == 100 &&
				nftOwnerStat1.PayoutAddress == "nft_minter_payout_addr" &&
				nftOwnerStat1.Owner == "nft_minter" &&
				nftOwnerStat1.Reward.Equals(nftMinterAmount)

			return nftStatisticCorrect && nftOwnerStat1Correct
		}),
		"farm_1_denom_1_nft_owner_2_tx_hash",
		int64(1),
		"farm_1",
	).Return(nil)

	s := NewPayService(config, mockAPIRequester, &mockHelper{}, btcNetworkParams)
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClient(), storage))
}

func TestPayService_ProcessPayment_Expiration_Between_Payments(t *testing.T) {
	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            20,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	mockAPIRequester := setupMockApiRequester(t)

	mockAPIRequester.GetFarmCollectionsWithNFTs(context.Background(), []string{"farm_1_denom_1"})
	// expires 1/2 through the period between payments
	farm1Denom1Data := `
	[
		{
			"denom": {
				"id": "farm_1_denom_1"
			},
			"nfts": [
				{
					"id": "1",
					"data_json": {
						"expiration_date": 1665820278,
						"hash_rate_owned": 960
					}
				},
				{
					"id": "2",
					"data_json": {
						"expiration_date": 1643089013,
						"hash_rate_owned": 1000
					}
				}
			]
		}
	]
	`

	var farm1Denom1CollectionWithNFTs []types.Collection
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Data), &farm1Denom1CollectionWithNFTs))

	mockAPIRequester.On("GetFarmCollectionsWithNFTs", mock.Anything, []string{"farm_1_denom_1"}).Return(farm1Denom1CollectionWithNFTs, nil).Once()

	// minted at 1/2 of the period between payments
	farm1Denom1Nft1TransferHistoryJSON := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_minter",
						"from": "0x0",
						"timestamp": 0
					}
				]
			}
		}
	}`

	var farm1Denom1Nft1TransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Nft1TransferHistoryJSON), &farm1Denom1Nft1TransferHistory))

	// call it once to clear mock
	mockAPIRequester.GetNftTransferHistory(context.Background(), "farm_1_denom_1", "1", 0)
	mockAPIRequester.On("GetNftTransferHistory", mock.Anything, "farm_1_denom_1", "1", mock.Anything).Return(farm1Denom1Nft1TransferHistory, nil).Once()

	collectionAllocationAmount := decimal.NewFromFloat(4)
	leftoverAmount := decimal.NewFromFloat(3)
	cudoMaintenanceFee := decimal.NewFromFloat(1.25012768)
	nftMinterAmount, _ := decimal.NewFromString("1.9997446236559112")
	cudoPartOfReward := decimal.NewFromFloat(1.25)
	cudoPartOfMaintenanceFee, _ := decimal.NewFromString("0.0001276881720444")
	maintenanceFeeAddress1Amount, _ := decimal.NewFromString("0.0001276881720444")

	// maintenance_fee_payout_address_1 is below threshold of 0.01 with values 5.928e-05
	mockAPIRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1":      leftoverAmount.InexactFloat64(),
		"cudo_maintenance_fee_payout_address_1": cudoMaintenanceFee.InexactFloat64(),
		"nft_minter_payout_addr":                nftMinterAmount.RoundFloor(8).InexactFloat64(),
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	storage := setupMockStorage()

	storage.On("SaveStatistics", mock.Anything,
		mock.MatchedBy(func(farmPayment types.FarmPayment) bool {
			return farmPayment.AmountBTC.Equal(decimal.NewFromFloat(6.25))
		}),
		mock.MatchedBy(func(amountInfoMap map[string]types.AmountInfo) bool {
			return amountInfoMap["leftover_reward_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["nft_minter_payout_addr"].ThresholdReached == true &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["maintenance_fee_payout_address_1"].ThresholdReached == false &&

				amountInfoMap["leftover_reward_payout_address_1"].Amount.Equals(leftoverAmount.RoundFloor(8)) &&
				amountInfoMap["nft_minter_payout_addr"].Amount.Equals(nftMinterAmount.RoundFloor(8)) &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].Amount.Equals(cudoMaintenanceFee.RoundFloor(8)) &&
				amountInfoMap["maintenance_fee_payout_address_1"].Amount.Equals(maintenanceFeeAddress1Amount.RoundFloor(8))
		}),
		mock.MatchedBy(func(collectionAllocations []types.CollectionPaymentAllocation) bool {
			collectionPartOfFarm := decimal.NewFromFloat(0.8)

			return collectionAllocations[0].FarmId == 1 &&
				collectionAllocations[0].CollectionId == 1 &&
				collectionAllocations[0].CollectionAllocationAmount.Equals(collectionAllocationAmount) &&
				collectionAllocations[0].CUDOGeneralFee.Equals(cudoPartOfReward.Mul(collectionPartOfFarm)) &&
				collectionAllocations[0].CUDOMaintenanceFee.Equals(cudoPartOfMaintenanceFee) &&
				collectionAllocations[0].FarmMaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				collectionAllocations[0].FarmUnsoldLeftovers.Equals(collectionAllocationAmount.Sub(nftMinterAmount).Sub(cudoPartOfMaintenanceFee).Sub(maintenanceFeeAddress1Amount))
		}),
		mock.MatchedBy(func(nftStatistics []types.NFTStatistics) bool {
			nftStatistic := nftStatistics[0]
			nftOwnerStat1 := nftStatistic.NFTOwnersForPeriod[0]

			nftStatisticCorrect := nftStatistic.TokenId == "1" &&
				nftStatistic.DenomId == "farm_1_denom_1" &&
				nftStatistic.PayoutPeriodStart == 1664999478 &&
				nftStatistic.PayoutPeriodEnd == 1666641078 &&
				nftStatistic.Reward.Equals(nftMinterAmount) &&
				nftStatistic.MaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				nftStatistic.CUDOPartOfMaintenanceFee.Equals(cudoPartOfMaintenanceFee)

			nftOwnerStat1Correct := nftOwnerStat1.TimeOwnedFrom == 1664999478 &&
				nftOwnerStat1.TimeOwnedTo == 1666641078 &&
				nftOwnerStat1.TotalTimeOwned == 820800 &&
				nftOwnerStat1.PercentOfTimeOwned == 100 &&
				nftOwnerStat1.PayoutAddress == "nft_minter_payout_addr" &&
				nftOwnerStat1.Owner == "nft_minter" &&
				nftOwnerStat1.Reward.Equals(nftMinterAmount)

			return nftStatisticCorrect && nftOwnerStat1Correct
		}),
		"farm_1_denom_1_nft_owner_2_tx_hash",
		int64(1),
		"farm_1",
	).Return(nil)

	s := NewPayService(config, mockAPIRequester, &mockHelper{}, btcNetworkParams)
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClient(), storage))
}

func TestPayService_ProcessPayment_NFT_Minted_After_Payment_Period(t *testing.T) {
	config := &infrastructure.Config{
		Network:                    "BTC",
		CUDOMaintenanceFeePercent:  50,
		CUDOFeeOnAllBTC:            20,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	mockAPIRequester := setupMockApiRequester(t)

	mockAPIRequester.GetFarmCollectionsWithNFTs(context.Background(), []string{"farm_1_denom_1"})
	// expires 1/2 through the period between payments
	farm1Denom1Data := `
	[
		{
			"denom": {
				"id": "farm_1_denom_1"
			},
			"nfts": [
				{
					"id": "1",
					"data_json": {
						"expiration_date": 1965820278,
						"hash_rate_owned": 960
					}
				},
				{
					"id": "2",
					"data_json": {
						"expiration_date": 1643089013,
						"hash_rate_owned": 1000
					}
				}
			]
		}
	]
	`

	var farm1Denom1CollectionWithNFTs []types.Collection
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Data), &farm1Denom1CollectionWithNFTs))

	mockAPIRequester.On("GetFarmCollectionsWithNFTs", mock.Anything, []string{"farm_1_denom_1"}).Return(farm1Denom1CollectionWithNFTs, nil).Once()

	// minted at 1/2 of the period between payments
	farm1Denom1Nft1TransferHistoryJSON := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_minter",
						"from": "0x0",
						"timestamp": 1964820278
					}
				]
			}
		}
	}`

	var farm1Denom1Nft1TransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Nft1TransferHistoryJSON), &farm1Denom1Nft1TransferHistory))

	// call it once to clear mock
	mockAPIRequester.GetNftTransferHistory(context.Background(), "farm_1_denom_1", "1", 0)
	mockAPIRequester.On("GetNftTransferHistory", mock.Anything, "farm_1_denom_1", "1", mock.Anything).Return(farm1Denom1Nft1TransferHistory, nil).Once()

	collectionAllocationAmount := decimal.NewFromFloat(4)
	leftoverAmount := decimal.NewFromFloat(5)
	cudoMaintenanceFee := decimal.NewFromFloat(1.25)
	cudoPartOfReward := decimal.NewFromFloat(1.25)
	cudoPartOfMaintenanceFee, _ := decimal.NewFromString("0")
	maintenanceFeeAddress1Amount, _ := decimal.NewFromString("0")

	// maintenance_fee_payout_address_1 is below threshold of 0.01 with values 5.928e-05
	mockAPIRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1":      5,
		"cudo_maintenance_fee_payout_address_1": 1.25,
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	storage := setupMockStorage()

	storage.On("SaveStatistics", mock.Anything,
		mock.MatchedBy(func(farmPayment types.FarmPayment) bool {
			return farmPayment.AmountBTC.Equal(decimal.NewFromFloat(6.25))
		}),
		mock.MatchedBy(func(amountInfoMap map[string]types.AmountInfo) bool {
			return amountInfoMap["leftover_reward_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["leftover_reward_payout_address_1"].Amount.Equals(leftoverAmount) &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].Amount.Equals(cudoMaintenanceFee)
		}),
		mock.MatchedBy(func(collectionAllocations []types.CollectionPaymentAllocation) bool {
			collectionPartOfFarm := decimal.NewFromFloat(0.8)

			return collectionAllocations[0].FarmId == 1 &&
				collectionAllocations[0].CollectionId == 1 &&
				collectionAllocations[0].CollectionAllocationAmount.Equals(collectionAllocationAmount) &&
				collectionAllocations[0].CUDOGeneralFee.Equals(cudoPartOfReward.Mul(collectionPartOfFarm)) &&
				collectionAllocations[0].CUDOMaintenanceFee.Equals(cudoPartOfMaintenanceFee) &&
				collectionAllocations[0].FarmMaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				collectionAllocations[0].FarmUnsoldLeftovers.Equals(collectionAllocationAmount.Sub(cudoPartOfMaintenanceFee).Sub(maintenanceFeeAddress1Amount))
		}),
		mock.MatchedBy(func(nftStatistics []types.NFTStatistics) bool {
			return len(nftStatistics) == 0
		}),
		"farm_1_denom_1_nft_owner_2_tx_hash",
		int64(1),
		"farm_1",
	).Return(nil)

	s := NewPayService(config, mockAPIRequester, &mockHelper{}, btcNetworkParams)
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClient(), storage))
}

func tearDownDatabase(sqlxDB *sqlx.DB) {
	_, err := sqlxDB.Exec("TRUNCATE TABLE utxo_transactions")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE threshold_amounts")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE statistics_destination_addresses_with_amount")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE statistics_nft_owners_payout_history CASCADE ")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE statistics_nft_payout_history CASCADE")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE statistics_tx_hash_status")
	if err != nil {
		panic(err)
	}
}

func setupMockApiRequester(t *testing.T) *mockAPIRequester {
	apiRequester := &mockAPIRequester{}

	farms := []types.Farm{
		{
			Id:                                 1,
			SubAccountName:                     "farm_1",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_1",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_1",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_1",
			MaintenanceFeeInBtc:                1,
		},
		{
			Id:                                 2,
			SubAccountName:                     "farm_2",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_2",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_2",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_2",
			MaintenanceFeeInBtc:                0.01,
		},
	}

	apiRequester.On("GetFarms", mock.Anything).Return(farms, nil)

	farm1Data := `
	{
		"data": {
			"denoms_by_data_property": [
				{
					"id": "farm_1_denom_1",
					"data_json": {
						"farm_id": "farm_1",
						"owner": "farm_1_denom_1_owner"
					}
				}
			]
		}
	}
	`

	var farm1CollectionData types.CollectionData
	require.NoError(t, json.Unmarshal([]byte(farm1Data), &farm1CollectionData))

	apiRequester.On("GetFarmCollectionsFromHasura", mock.Anything, int64(1)).Return(farm1CollectionData, nil).Once()

	apiRequester.On("GetFarmTotalHashPowerFromPoolToday", mock.Anything, "farm_1", mock.Anything).Return(1200.0, nil).Once()

	apiRequester.On("VerifyCollection", mock.Anything, "farm_1_denom_1").Return(true, nil)

	farm1Denom1Data := `
	[
		{
			"denom": {
				"id": "farm_1_denom_1"
			},
			"nfts": [
				{
					"id": "1",
					"data_json": {
						"expiration_date": 1919101878,
						"hash_rate_owned": 960
					}
				},
				{
					"id": "2",
					"data_json": {
						"expiration_date": 1643089013,
						"hash_rate_owned": 1000
					}
				}
			]
		}
	]
	`

	var farm1Denom1CollectionWithNFTs []types.Collection
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Data), &farm1Denom1CollectionWithNFTs))

	apiRequester.On("GetFarmCollectionsWithNFTs", mock.Anything, []string{"farm_1_denom_1"}).Return(farm1Denom1CollectionWithNFTs, nil).Once()

	farm1Denom1Nft1TransferHistoryJSON := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_minter",
						"from": "0x0",
						"timestamp": 1664999478
					},
					{
						"to": "nft_owner_2",
						"from": "nft_minter",
						"timestamp": 1665431478
					}
				]
			}
		}
	}`

	var farm1Denom1Nft1TransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(farm1Denom1Nft1TransferHistoryJSON), &farm1Denom1Nft1TransferHistory))

	apiRequester.On("GetNftTransferHistory", mock.Anything, "farm_1_denom_1", "1", mock.Anything).Return(farm1Denom1Nft1TransferHistory, nil).Once()

	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_minter", "BTC", "1", "farm_1_denom_1").Return("nft_minter_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "farm_1_denom_1").Return("nft_owner_2_payout_addr", nil)

	// maintenance_fee_payout_address_1 is below threshold of 0.01 with values 5.928e-05
	apiRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1":      1,
		"cudo_maintenance_fee_payout_address_1": 1.25025537,
		"nft_minter_payout_addr":                1.05249717,
		"nft_owner_2_payout_addr":               2.94699207,
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	return apiRequester
}

func setupMockBtcClient() *mockBtcClient {
	btcClient := &mockBtcClient{}

	// btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{
	// 	{TxID: "1", Amount: 2.425, Address: "address_for_receiving_reward_from_pool_1"},
	// 	{TxID: "2", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
	// 	{TxID: "3", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
	// 	{TxID: "4", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
	// }, nil).Once()

	btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{
		{TxID: "1", Amount: 6.25, Address: "address_for_receiving_reward_from_pool_1"},
	}, nil).Once()

	btcClient.On("LoadWallet", "farm_1").Return(&btcjson.LoadWalletResult{}, nil).Once()
	btcClient.On("LoadWallet", "farm_2").Return(&btcjson.LoadWalletResult{}, errors.New("failed to load wallet")).Once()
	btcClient.On("UnloadWallet", mock.Anything).Return(nil)
	btcClient.On("WalletPassphrase", mock.Anything, mock.Anything).Return(nil)
	btcClient.On("WalletLock").Return(nil)
	btcClient.On("GetRawTransactionVerbose", mock.Anything).Return(&btcjson.TxRawResult{Time: 1666641078}, nil).Once()

	return btcClient
}

func (mbc *mockBtcClient) LoadWallet(walletName string) (*btcjson.LoadWalletResult, error) {

	args := mbc.Called(walletName)
	return args.Get(0).(*btcjson.LoadWalletResult), args.Error(1)
}

func (mbc *mockBtcClient) UnloadWallet(walletName *string) error {
	args := mbc.Called(walletName)
	return args.Error(0)
}

func (mbc *mockBtcClient) WalletPassphrase(passphrase string, timeoutSecs int64) error {
	args := mbc.Called(passphrase, timeoutSecs)
	return args.Error(0)
}

func (mbc *mockBtcClient) WalletLock() error {
	args := mbc.Called()
	return args.Error(0)
}

func (mbc *mockBtcClient) GetRawTransactionVerbose(txHash *chainhash.Hash) (*btcjson.TxRawResult, error) {
	args := mbc.Called(txHash)
	return args.Get(0).(*btcjson.TxRawResult), args.Error(1)
}

type mockBtcClient struct {
	mock.Mock
}

func (mbc *mockBtcClient) ListUnspent() ([]btcjson.ListUnspentResult, error) {
	args := mbc.Called()
	return args.Get(0).([]btcjson.ListUnspentResult), args.Error(1)
}

func setupMockStorage() *mockStorage {
	storage := &mockStorage{}

	leftoverAmount := decimal.NewFromFloat(1)
	nftMinterAmount, _ := decimal.NewFromString("1.05249717034521640000268817204304")
	nftOwner2Amount, _ := decimal.NewFromString("2.94699207696660599999731182795696")
	cudoPartOfReward, _ := decimal.NewFromString("1.25")
	cudoPartOfMaintenanceFee, _ := decimal.NewFromString("0.0002553763440888")
	maintenanceFeeAddress1Amount, _ := decimal.NewFromString("0.0002553763440888")

	amount, _ := decimal.NewFromString("3.9994892473118224")

	storage.On("GetPayoutTimesForNFT", mock.Anything, mock.Anything, mock.Anything).Return([]types.NFTStatistics{}, nil)
	storage.On("SaveStatistics", mock.Anything,
		mock.MatchedBy(func(farmPayment types.FarmPayment) bool {
			return farmPayment.AmountBTC.Equal(decimal.NewFromFloat(6.25))
		}),
		mock.MatchedBy(func(amountInfoMap map[string]types.AmountInfo) bool {

			return amountInfoMap["leftover_reward_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["nft_minter_payout_addr"].ThresholdReached == true &&
				amountInfoMap["nft_owner_2_payout_addr"].ThresholdReached == true &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].ThresholdReached == true &&
				amountInfoMap["maintenance_fee_payout_address_1"].ThresholdReached == false &&

				amountInfoMap["leftover_reward_payout_address_1"].Amount.Equals(leftoverAmount.RoundFloor(8)) &&
				amountInfoMap["nft_minter_payout_addr"].Amount.Equals(nftMinterAmount.RoundFloor(8)) &&
				amountInfoMap["nft_owner_2_payout_addr"].Amount.Equals(nftOwner2Amount.RoundFloor(8)) &&
				amountInfoMap["cudo_maintenance_fee_payout_address_1"].Amount.Equals(cudoPartOfMaintenanceFee.Add(cudoPartOfReward).RoundFloor(8)) &&
				amountInfoMap["maintenance_fee_payout_address_1"].Amount.Equals(maintenanceFeeAddress1Amount.RoundFloor(8))
		}),
		mock.MatchedBy(func(collectionAllocations []types.CollectionPaymentAllocation) bool {
			collectionPartOfFarm := decimal.NewFromFloat(0.8)

			return collectionAllocations[0].FarmId == 1 &&
				collectionAllocations[0].CollectionId == 1 &&
				collectionAllocations[0].CollectionAllocationAmount.Equals(decimal.NewFromFloat(4)) &&
				collectionAllocations[0].CUDOGeneralFee.Equals(cudoPartOfReward.Mul(collectionPartOfFarm)) &&
				collectionAllocations[0].CUDOMaintenanceFee.Equals(cudoPartOfMaintenanceFee) &&
				collectionAllocations[0].FarmUnsoldLeftovers.Equals(collectionAllocations[0].CollectionAllocationAmount.Sub(nftMinterAmount).Sub(nftOwner2Amount).Sub(cudoPartOfMaintenanceFee).Sub(maintenanceFeeAddress1Amount)) &&
				collectionAllocations[0].FarmMaintenanceFee.Equals(maintenanceFeeAddress1Amount)
		}),
		mock.MatchedBy(func(nftStatistics []types.NFTStatistics) bool {
			nftStatistic := nftStatistics[0]
			nftOwnerStat1 := nftStatistic.NFTOwnersForPeriod[0]
			nftOwnerStat2 := nftStatistic.NFTOwnersForPeriod[1]

			nftStatisticCorrect := nftStatistic.TokenId == "1" &&
				nftStatistic.DenomId == "farm_1_denom_1" &&
				nftStatistic.PayoutPeriodStart == 1664999478 &&
				nftStatistic.PayoutPeriodEnd == 1666641078 &&
				nftStatistic.Reward.Equals(amount) &&
				nftStatistic.MaintenanceFee.Equals(maintenanceFeeAddress1Amount) &&
				nftStatistic.CUDOPartOfMaintenanceFee.Equals(cudoPartOfMaintenanceFee)

			nftOwnerStat1Correct := nftOwnerStat1.TimeOwnedFrom == 1664999478 &&
				nftOwnerStat1.TimeOwnedTo == 1665431478 &&
				nftOwnerStat1.TotalTimeOwned == 432000 &&
				nftOwnerStat1.PercentOfTimeOwned == 26.31578947368421 &&
				nftOwnerStat1.PayoutAddress == "nft_minter_payout_addr" &&
				nftOwnerStat1.Owner == "nft_minter" &&
				nftOwnerStat1.Reward.Equals(nftMinterAmount)

			nftOwnerStat2Correct := nftOwnerStat2.TimeOwnedFrom == 1665431478 &&
				nftOwnerStat2.TimeOwnedTo == 1666641078 &&
				nftOwnerStat2.TotalTimeOwned == 1209600 &&
				nftOwnerStat2.PercentOfTimeOwned == 73.68421052631578 &&
				nftOwnerStat2.PayoutAddress == "nft_owner_2_payout_addr" &&
				nftOwnerStat2.Owner == "nft_owner_2" &&
				nftOwnerStat2.Reward.Equals(nftOwner2Amount)

			return nftStatisticCorrect && nftOwnerStat1Correct && nftOwnerStat2Correct
		}),
		"farm_1_denom_1_nft_owner_2_tx_hash",
		int64(1),
		"farm_1",
	).Return(nil)
	storage.On("GetUTXOTransaction", mock.Anything, "1").Return(types.UTXOTransaction{TxHash: "1", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "2").Return(types.UTXOTransaction{TxHash: "2", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "3").Return(types.UTXOTransaction{TxHash: "3", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "4").Return(types.UTXOTransaction{TxHash: "4", Processed: false}, nil)

	storage.On("GetCurrentAcummulatedAmountForAddress", mock.Anything, mock.Anything, mock.Anything).Return(decimal.Zero, nil)

	storage.On("UpdateThresholdStatus", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	storage.On("GetApprovedFarms", mock.Anything).Return([]types.Farm{
		{
			Id:                                 1,
			SubAccountName:                     "farm_1",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_1",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_1",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_1",
			MaintenanceFeeInBtc:                1,
			TotalHashPower:                     1200,
		},
		{
			Id:                                 2,
			SubAccountName:                     "farm_2",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_2",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_2",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_2",
			MaintenanceFeeInBtc:                0.01,
			TotalHashPower:                     1200,
		},
	}, nil)

	storage.On("GetFarmAuraPoolCollections", mock.Anything, int64(1)).Return(
		[]types.AuraPoolCollection{
			{
				Id:           1,
				DenomId:      "farm_1_denom_1",
				HashingPower: 960,
			},
		}, nil)

	storage.On("GetLastUTXOTransactionByFarmId", mock.Anything, int64(1)).Return(
		types.UTXOTransaction{
			Id:               "1",
			FarmId:           "1",
			PaymentTimestamp: 1664999478,
		}, nil)

	return storage
}

func setupInMemoryStorage(c *infrastructure.Config) (Storage, *sqlx.DB) {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		c.DbHost, c.DbPort, c.DbUser, c.DbPassword, c.DbName)

	db, err := sqlx.Connect(c.DbDriverName, psqlInfo)
	if err != nil {
		panic(err)
	}
	storage := sql_db.NewSqlDB(db)
	return storage, db
}

func (ms *mockStorage) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	args := ms.Called(ctx, collectionDenomId, nftId)
	return args.Get(0).([]types.NFTStatistics), args.Error(1)
}

func (ms *mockStorage) SaveStatistics(ctx context.Context, farmPaymentStatistics types.FarmPayment, collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash string, farmId int64, farmSubAccountName string) error {
	args := ms.Called(ctx, farmPaymentStatistics, destinationAddressesWithAmount, collectionPaymentAllocationsStatistics, statistics, txHash, farmId, farmSubAccountName)
	return args.Error(0)
}

func (ms *mockStorage) UpdateTransactionsStatus(ctx context.Context, txHashesToMarkCompleted []string, status string) error {
	args := ms.Called(ctx, txHashesToMarkCompleted, status)
	return args.Error(0)
}

func (ms *mockStorage) SaveTxHashWithStatus(ctx context.Context, txHash, status, farmSubAccountName string, retryCount int) error {
	args := ms.Called(ctx, txHash, status, farmSubAccountName, retryCount)
	return args.Error(0)
}

func (ms *mockStorage) GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error) {
	args := ms.Called(ctx, status)
	return args.Get(0).([]types.TransactionHashWithStatus), args.Error(1)
}

func (ms *mockStorage) SaveRBFTransactionInformation(ctx context.Context, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName string, retryCount int) error {
	args := ms.Called(ctx, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName, retryCount)
	return args.Error(0)
}

type mockStorage struct {
	mock.Mock
}

func (ms *mockStorage) GetLastUTXOTransactionByFarmId(ctx context.Context, farmId int64) (types.UTXOTransaction, error) {
	args := ms.Called(ctx, farmId)
	return args.Get(0).(types.UTXOTransaction), args.Error(1)
}

func (ms *mockStorage) GetFarmAuraPoolCollections(ctx context.Context, farmId int64) ([]types.AuraPoolCollection, error) {
	args := ms.Called(ctx, farmId)
	return args.Get(0).([]types.AuraPoolCollection), args.Error(1)
}

func (ms *mockStorage) GetApprovedFarms(ctx context.Context) ([]types.Farm, error) {
	args := ms.Called(ctx)
	return args.Get(0).([]types.Farm), args.Error(1)
}

func (ms *mockStorage) GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error) {
	args := ms.Called(ctx, txId)
	return args.Get(0).(types.UTXOTransaction), args.Error(1)
}

func (ms *mockStorage) GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int64) (decimal.Decimal, error) {
	args := ms.Called(ctx, key, farmId)
	return args.Get(0).(decimal.Decimal), args.Error(1)
}

func (ms *mockStorage) UpdateThresholdStatus(ctx context.Context, processedTransaction string, paymentTimestamp int64, addressesWithThresholdToUpdate map[string]decimal.Decimal, farmId int64) error {
	args := ms.Called(ctx, processedTransaction, addressesWithThresholdToUpdate)
	return args.Error(0)
}

func (ms *mockStorage) SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount int) error {
	args := ms.Called(ctx, address, farmId, amount)
	return args.Error(0)
}

func (_ *mockHelper) DaysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (_ *mockHelper) Unix() int64 {
	return 1666641078
}

func (_ *mockHelper) Date() (year int, month time.Month, day int) {
	return 2022, time.October, 24
}

func (_ *mockHelper) SendMail(message string, to []string) error {
	return nil
}

type mockHelper struct {
}

func skipDBTests(t *testing.T) {
	if os.Getenv("EXECUTE_DB_TEST") == "" || os.Getenv("EXECUTE_DB_TEST") == "false" {
		t.Skip("Skipping DB Tests in this env")
	}
}
