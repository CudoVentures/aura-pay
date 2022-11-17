package services

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"testing"
	"time"

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

func setupMockApiRequester(t *testing.T) *mockAPIRequester {
	apiRequester := &mockAPIRequester{}

	farms := []types.Farm{
		{
			Id:                                 1,
			SubAccountName:                     "farm_1",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_1",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_1",
			MaintenanceFeePayoutdAddress:       "maintenance_fee_payout_address_1",
			MonthlyMaintenanceFeeInBTC:         "1",
		},
		{
			Id:                                 2,
			SubAccountName:                     "farm_2",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_2",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_2",
			MaintenanceFeePayoutdAddress:       "maintenance_fee_payout_address_2",
			MonthlyMaintenanceFeeInBTC:         "0.01",
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

	apiRequester.On("GetFarmCollectionsFromHasura", mock.Anything, "farm_1").Return(farm1CollectionData, nil).Once()

	apiRequester.On("GetFarmTotalHashPowerFromPoolToday", mock.Anything, "farm_1", mock.Anything).Return(5000.0, nil).Once()

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
						"hash_rate_owned": 1000
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

	// TODO: Verify that values are correct

	// cudo_maintenance_fee_payout_addr and maintenance_fee_payout_address_1 are below threshold of 0.01 with values 5.928e-05
	apiRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1": 4,
		"nft_minter_payout_addr":           0.2631688,
		"nft_owner_2_payout_addr":          0.73671264,
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	return apiRequester
}

func setupMockBtcClient() *mockBtcClient {
	btcClient := &mockBtcClient{}

	btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{
		btcjson.ListUnspentResult{TxID: "1", Amount: 1.25, Address: "address_for_receiving_reward_from_pool_1"},
		btcjson.ListUnspentResult{TxID: "2", Amount: 1.25, Address: "address_for_receiving_reward_from_pool_1"},
		btcjson.ListUnspentResult{TxID: "3", Amount: 1.25, Address: "address_for_receiving_reward_from_pool_1"},
		btcjson.ListUnspentResult{TxID: "4", Amount: 1.25, Address: "address_for_receiving_reward_from_pool_1"},
	}, nil).Once()

	btcClient.On("LoadWallet", "farm_1").Return(&btcjson.LoadWalletResult{}, nil).Once()
	btcClient.On("LoadWallet", "farm_2").Return(&btcjson.LoadWalletResult{}, errors.New("failed to load wallet")).Once()
	btcClient.On("UnloadWallet", mock.Anything).Return(nil)
	btcClient.On("WalletPassphrase", mock.Anything, mock.Anything).Return(nil)
	btcClient.On("WalletLock").Return(nil)
	btcClient.On("GetRawTransactionVerbose").Return(nil)

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

	storage.On("GetPayoutTimesForNFT", mock.Anything, mock.Anything, mock.Anything).Return([]types.NFTStatistics{}, nil)
	storage.On("SaveStatistics", mock.Anything,
		map[string]types.AmountInfo{
			"leftover_reward_payout_address_1":      {Amount: 400000000, ThresholdReached: true},
			"nft_minter_payout_addr":                {Amount: 26316880, ThresholdReached: true},
			"nft_owner_2_payout_addr":               {Amount: 73671264, ThresholdReached: true},
			"cudo_maintenance_fee_payout_address_1": {Amount: 5928, ThresholdReached: false},
			"maintenance_fee_payout_address_1":      {Amount: 5928, ThresholdReached: false},
		},

		[]types.NFTStatistics{
			{
				TokenId:                  "1",
				DenomId:                  "farm_1_denom_1",
				PayoutPeriodStart:        1664999478,
				PayoutPeriodEnd:          1666641078,
				Reward:                   99988144,
				MaintenanceFee:           5928,
				CUDOPartOfMaintenanceFee: 5928,
				NFTOwnersForPeriod: []types.NFTOwnerInformation{
					{
						TimeOwnedFrom:      1664999478,
						TimeOwnedTo:        1665431478,
						TotalTimeOwned:     432000,
						PercentOfTimeOwned: 26.32,
						PayoutAddress:      "nft_minter_payout_addr",
					},
					{
						TimeOwnedFrom:      1665431478,
						TimeOwnedTo:        1666641078,
						TotalTimeOwned:     1209600,
						PercentOfTimeOwned: 73.68,
						PayoutAddress:      "nft_owner_2_payout_addr",
					},
				},
			},
		},
		"farm_1_denom_1_nft_owner_2_tx_hash", "1", "farm_1").Return(nil)
	storage.On("GetUTXOTransaction", mock.Anything, "1").Return(types.UTXOTransaction{TxHash: "1", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "2").Return(types.UTXOTransaction{TxHash: "2", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "3").Return(types.UTXOTransaction{TxHash: "3", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "4").Return(types.UTXOTransaction{TxHash: "4", Processed: false}, nil)

	storage.On("GetCurrentAcummulatedAmountForAddress", mock.Anything, mock.Anything, mock.Anything).Return(int64(0), nil)

	storage.On("UpdateThresholdStatuses", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	return storage
}

func (ms *mockStorage) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	args := ms.Called(ctx, collectionDenomId, nftId)
	return args.Get(0).([]types.NFTStatistics), args.Error(1)
}

func (ms *mockStorage) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash, farmId string, farmSubAccountName string) error {
	args := ms.Called(ctx, destinationAddressesWithAmount, statistics, txHash, farmId, farmSubAccountName)
	return args.Error(0)
}

func (ms *mockStorage) UpdateTransactionsStatus(ctx context.Context, tx *sqlx.Tx, txHashesToMarkCompleted []string, status string) error {
	args := ms.Called(ctx, tx, txHashesToMarkCompleted, status)
	return args.Error(0)
}

func (ms *mockStorage) SaveTxHashWithStatus(ctx context.Context, tx *sqlx.Tx, txHash string, status string, farmSubAccountName string, retryCount int) error {
	args := ms.Called(ctx, tx, txHash, status, farmSubAccountName, retryCount)
	return args.Error(0)
}

func (ms *mockStorage) SaveRBFTransactionHistory(ctx context.Context, tx *sqlx.Tx, oldTxHash string, newTxHash string, farmSubAccountName string) error {
	args := ms.Called(ctx, tx, oldTxHash, newTxHash, farmSubAccountName)
	return args.Error(0)
}

func (ms *mockStorage) GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error) {
	args := ms.Called(ctx, status)
	return args.Get(0).([]types.TransactionHashWithStatus), args.Error(1)
}

func (ms *mockStorage) SaveRBFTransactionInformation(ctx context.Context,
	oldTxHash string,
	oldTxStatus string,
	newRBFTxHash string,
	newRBFTXStatus string,
	farmSubAccountName string,
	retryCount int) error {
	args := ms.Called(ctx, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName, retryCount)
	return args.Error(0)
}

type mockStorage struct {
	mock.Mock
}

func (ms *mockStorage) GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error) {
	args := ms.Called(ctx, txId)
	return args.Get(0).(types.UTXOTransaction), args.Error(1)
}

func (ms *mockStorage) GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int) (int64, error) {
	args := ms.Called(ctx, key, farmId)
	return args.Get(0).(int64), args.Error(1)
}

func (ms *mockStorage) UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]int64, farmId int) error {
	args := ms.Called(ctx, processedTransactions, addressesWithThresholdToUpdate)
	return args.Error(0)
}

func (ms *mockStorage) SetInitialAccumulatedAmountForAddress(ctx context.Context, tx *sqlx.Tx, address string, farmId int, amount int) error {
	args := ms.Called(ctx, tx, address, farmId, amount)
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

type mockHelper struct {
}
