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
	"github.com/btcsuite/btcd/btcutil"
	"github.com/jmoiron/sqlx"

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
		CUDOFeeOnAllBTC:            2,
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
		CUDOFeeOnAllBTC:            2,
		CUDOFeePayoutAddress:       "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC: 0.01,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	s := NewPayService(config, setupMockApiRequester(t), &mockHelper{}, btcNetworkParams)

	farms, err := s.apiRequester.GetFarms(context.Background())
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

	err := dbStorage.UpdateThresholdStatuses(context.Background(), []string{"3"}, map[string]btcutil.Amount{}, 1)
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

	amountAccumulatedBTC, err := dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "maintenance_fee_payout_address_1", 1)
	require.Equal(t, float64(5928), amountAccumulatedBTC)
	amountAccumulatedBTC, err = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "cudo_maintenance_fee_payout_address_1", 1)
	require.Equal(t, float64(10210010), amountAccumulatedBTC)
	amountAccumulatedBTC, err = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "nft_minter_payout_addr", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)
	amountAccumulatedBTC, err = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "nft_owner_2_payout_addr", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)
	amountAccumulatedBTC, err = dbStorage.GetCurrentAcummulatedAmountForAddress(context.Background(), "leftover_reward_payout_address_1", 1)
	require.Equal(t, float64(0), amountAccumulatedBTC)

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
			Id:                                 "1",
			SubAccountName:                     "farm_1",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_1",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_1",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_1",
			MaintenanceFeeInBtc:                "1",
		},
		{
			Id:                                 "2",
			SubAccountName:                     "farm_2",
			AddressForReceivingRewardsFromPool: "address_for_receiving_reward_from_pool_2",
			LeftoverRewardPayoutAddress:        "leftover_reward_payout_address_2",
			MaintenanceFeePayoutAddress:        "maintenance_fee_payout_address_2",
			MaintenanceFeeInBtc:                "0.01",
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

	apiRequester.On("GetFarmCollectionsFromHasura", mock.Anything, "1").Return(farm1CollectionData, nil).Once()

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
						"hash_rate_owned": "1000"
					}
				},
				{
					"id": "2",
					"data_json": {
						"expiration_date": 1643089013,
						"hash_rate_owned": "1000"
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

	// maintenance_fee_payout_address_1 is below threshold of 0.01 with values 5.928e-05
	apiRequester.On("SendMany", mock.Anything, map[string]float64{
		"leftover_reward_payout_address_1":      4,
		"cudo_maintenance_fee_payout_address_1": 0.10210010,
		"nft_minter_payout_addr":                0.2631688,
		"nft_owner_2_payout_addr":               0.73671264,
	}).Return("farm_1_denom_1_nft_owner_2_tx_hash", nil).Once()

	return apiRequester
}

func setupMockBtcClient() *mockBtcClient {
	btcClient := &mockBtcClient{}

	btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{
		{TxID: "1", Amount: 1.277040816, Address: "address_for_receiving_reward_from_pool_1"},
		{TxID: "2", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
		{TxID: "3", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
		{TxID: "4", Amount: 1.275, Address: "address_for_receiving_reward_from_pool_1"},
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

	leftoverAmount := btcutil.Amount(400000000)
	nftMinterAmount := btcutil.Amount(26316880)
	nftOwner2Amount := btcutil.Amount(73671264)
	cudoPartOfReward := btcutil.Amount(10204082)
	cudoPartOfMaintenanceFee := btcutil.Amount(5928)
	maintenanceFeeAddress1Amount := btcutil.Amount(5928)

	storage.On("GetPayoutTimesForNFT", mock.Anything, mock.Anything, mock.Anything).Return([]types.NFTStatistics{}, nil)
	storage.On("SaveStatistics", mock.Anything,
		map[string]types.AmountInfo{
			"leftover_reward_payout_address_1":      {Amount: leftoverAmount, ThresholdReached: true},
			"nft_minter_payout_addr":                {Amount: nftMinterAmount, ThresholdReached: true},
			"nft_owner_2_payout_addr":               {Amount: nftOwner2Amount, ThresholdReached: true},
			"cudo_maintenance_fee_payout_address_1": {Amount: cudoPartOfMaintenanceFee + cudoPartOfReward, ThresholdReached: true},
			"maintenance_fee_payout_address_1":      {Amount: maintenanceFeeAddress1Amount, ThresholdReached: false},
		},

		[]types.NFTStatistics{
			{
				TokenId:                  "1",
				DenomId:                  "farm_1_denom_1",
				PayoutPeriodStart:        1664999478,
				PayoutPeriodEnd:          1666641078,
				Reward:                   0.99988144,
				MaintenanceFee:           maintenanceFeeAddress1Amount,
				CUDOPartOfMaintenanceFee: cudoPartOfMaintenanceFee,
				CUDOPartOfReward:         cudoPartOfReward,
				NFTOwnersForPeriod: []types.NFTOwnerInformation{
					{
						TimeOwnedFrom:      1664999478,
						TimeOwnedTo:        1665431478,
						TotalTimeOwned:     432000,
						PercentOfTimeOwned: 26.32,
						PayoutAddress:      "nft_minter_payout_addr",
						Owner:              "nft_minter",
						Reward:             nftMinterAmount,
					},
					{
						TimeOwnedFrom:      1665431478,
						TimeOwnedTo:        1666641078,
						TotalTimeOwned:     1209600,
						PercentOfTimeOwned: 73.68,
						PayoutAddress:      "nft_owner_2_payout_addr",
						Owner:              "nft_owner_2",
						Reward:             nftOwner2Amount,
					},
				},
			},
		},
		"farm_1_denom_1_nft_owner_2_tx_hash", "1", "farm_1").Return(nil)
	storage.On("GetUTXOTransaction", mock.Anything, "1").Return(types.UTXOTransaction{TxHash: "1", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "2").Return(types.UTXOTransaction{TxHash: "2", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "3").Return(types.UTXOTransaction{TxHash: "3", Processed: false}, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "4").Return(types.UTXOTransaction{TxHash: "4", Processed: false}, nil)

	storage.On("GetCurrentAcummulatedAmountForAddress", mock.Anything, mock.Anything, mock.Anything).Return(float64(0), nil)

	storage.On("UpdateThresholdStatuses", mock.Anything, mock.Anything, mock.Anything).Return(nil)
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

func (ms *mockStorage) SaveStatistics(ctx context.Context, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash, farmId string, farmSubAccountName string) error {
	args := ms.Called(ctx, destinationAddressesWithAmount, statistics, txHash, farmId, farmSubAccountName)
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

func (ms *mockStorage) GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error) {
	args := ms.Called(ctx, txId)
	return args.Get(0).(types.UTXOTransaction), args.Error(1)
}

func (ms *mockStorage) GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int) (float64, error) {
	args := ms.Called(ctx, key, farmId)
	return args.Get(0).(float64), args.Error(1)
}

func (ms *mockStorage) UpdateThresholdStatuses(ctx context.Context, processedTransactions []string, addressesWithThresholdToUpdate map[string]btcutil.Amount, farmId int) error {
	args := ms.Called(ctx, processedTransactions, addressesWithThresholdToUpdate)
	return args.Error(0)
}

func (ms *mockStorage) SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId, amount int) error {
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
