package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcessPayment(t *testing.T) {
	config := &infrastructure.Config{
		Network:                         "BTC",
		CUDOMaintenanceFeePercent:       50,
		CUDOMaintenanceFeePayoutAddress: "cudo_maintenance_fee_payout_addr",
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
			Id:                           1,
			SubAccountName:               "farm_1",
			LeftoverRewardPayoutAddress:  "leftover_reward_payout_address_1",
			MaintenanceFeePayoutdAddress: "maintenance_fee_payout_address_1",
			MonthlyMaintenanceFeeInBTC:   "1",
		},
		{
			Id:                           2,
			SubAccountName:               "farm_2",
			LeftoverRewardPayoutAddress:  "leftover_reward_payout_address_2",
			MaintenanceFeePayoutdAddress: "maintenance_fee_payout_address_2",
			MonthlyMaintenanceFeeInBTC:   "0.01",
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

	apiRequester.On("SendMany", mock.Anything, map[string]float64{
		"cudo_maintenance_fee_payout_addr": 5.928e-05,
		"maintenance_fee_payout_address_1": 5.928e-05,
		"leftover_reward_payout_address_1": 4,
		"nft_minter_payout_addr":           0.2631688,
		"nft_owner_2_payout_addr":          0.73671264,
	}, "farm_1", btcutil.Amount(500000000)).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	return apiRequester
}

func setupMockBtcClient() *mockBtcClient {
	btcClient := &mockBtcClient{}

	btcClient.On("GetBalance", mock.Anything).Return(btcutil.Amount(500000000), nil).Once()

	btcClient.On("LoadWallet", "farm_1").Return(&btcjson.LoadWalletResult{}, nil).Once()
	btcClient.On("LoadWallet", "farm_2").Return(&btcjson.LoadWalletResult{}, errors.New("faield to load wallet")).Once()
	btcClient.On("UnloadWallet", mock.Anything).Return(nil)
	btcClient.On("WalletPassphrase", mock.Anything, mock.Anything).Return(nil)
	btcClient.On("WalletLock").Return(nil)

	return btcClient
}

func (mbc *mockBtcClient) GetBalance(account string) (btcutil.Amount, error) {
	args := mbc.Called(account)
	return args.Get(0).(btcutil.Amount), args.Error(1)
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

type mockBtcClient struct {
	mock.Mock
}

func setupMockStorage() *mockStorage {
	storage := &mockStorage{}

	storage.On("GetPayoutTimesForNFT", mock.Anything, mock.Anything, mock.Anything).Return([]types.NFTStatistics{}, nil)
	storage.On("SaveStatistics", mock.Anything, map[string]btcutil.Amount{
		"cudo_maintenance_fee_payout_addr": 5928,
		"maintenance_fee_payout_address_1": 5928,
		"leftover_reward_payout_address_1": 400000000,
		"nft_minter_payout_addr":           26316880,
		"nft_owner_2_payout_addr":          73671264,
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
		"farm_1_denom_1_nft_owner_2_tx_hash", "1").Return(nil)

	return storage
}

func (ms *mockStorage) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	args := ms.Called(ctx, collectionDenomId, nftId)
	return args.Get(0).([]types.NFTStatistics), args.Error(1)
}

func (ms *mockStorage) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]btcutil.Amount, statistics []types.NFTStatistics, txHash, farmId string) error {
	args := ms.Called(ctx, destinationAddressesWithAmount, statistics, txHash, farmId)
	return args.Error(0)
}

type mockStorage struct {
	mock.Mock
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
