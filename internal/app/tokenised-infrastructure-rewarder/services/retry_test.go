package services

import (
	"context"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRetryService_Execute(t *testing.T) {
	config := &infrastructure.Config{
		Network:                           "BTC",
		CUDOMaintenanceFeePercent:         50,
		CUDOFeeOnAllBTC:                   2,
		CUDOFeePayoutAddress:              "cudo_maintenance_fee_payout_addr",
		RBFTransactionRetryDelayInSeconds: 10,
		RBFTransactionRetryMaxCount:       2,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	s := NewRetryService(config, setupMockApiRequesterRetryService(), &mockHelper{}, btcNetworkParams)
	mockStorageService := setupMockStorageRetryService()
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClientRetryService(), mockStorageService))

	completedTransactions := 0
	expectedCompletedTransactions := 1
	repalcedTransactions := 0
	expectedReplacedTransactions := 2
	failedTransactions := 0
	expectedFailedTransactions := 1
	for _, elem := range mockStorageService.Calls {
		if elem.Method == "UpdateTransactionsStatus" && elem.Arguments[2].(string) == "Completed" {
			completedTransactions++
		}
		if elem.Method == "UpdateTransactionsStatus" && elem.Arguments[2].(string) == "Failed" {
			failedTransactions++
		}
		if elem.Method == "SaveRBFTransactionInformation" && elem.Arguments[2].(string) == "Replaced" {
			repalcedTransactions++
		}

	}

	assert.Equal(t, expectedCompletedTransactions, completedTransactions)
	assert.Equal(t, expectedReplacedTransactions, repalcedTransactions)
	assert.Equal(t, expectedFailedTransactions, failedTransactions)

}

func TestRetryService_Execute_With_Database(t *testing.T) {
	skipDBTests(t)

	config := &infrastructure.Config{
		Network:                     "BTC",
		CUDOMaintenanceFeePercent:   50,
		CUDOFeeOnAllBTC:             2,
		CUDOFeePayoutAddress:        "cudo_maintenance_fee_payout_address_1",
		GlobalPayoutThresholdInBTC:  0.01,
		DbDriverName:                "postgres",
		DbUser:                      "postgresUser",
		DbPassword:                  "mysecretpassword",
		DbHost:                      "127.0.0.1",
		DbPort:                      "5432",
		DbName:                      "aura-pay-test-db",
		RBFTransactionRetryMaxCount: 2,
	}

	btcNetworkParams := &types.BtcNetworkParams{
		ChainParams:      &chaincfg.MainNetParams,
		MinConfirmations: 6,
	}

	dbStorage, sqlxDB := setupInMemoryStorage(config)
	defer func() {
		tearDownDatabaseRBF(sqlxDB)
	}()

	seedDatabase(dbStorage)

	s := NewRetryService(config, setupMockApiRequesterRetryService(), &mockHelperRetry{}, btcNetworkParams)
	require.NoError(t, s.Execute(context.Background(), setupMockBtcClientRetryService(), dbStorage))
	// fetch from db and check:
	confirmedTx, _ := dbStorage.GetTxHashesByStatus(context.Background(), types.TransactionCompleted)
	assert.Equal(t, 2, len(confirmedTx))
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f881", confirmedTx[0].TxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f882", confirmedTx[1].TxHash)

	failedTx, _ := dbStorage.GetTxHashesByStatus(context.Background(), types.TransactionFailed)
	assert.Equal(t, 2, len(failedTx))
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f883", failedTx[0].TxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f886", failedTx[1].TxHash)

	replacedTx, _ := dbStorage.GetTxHashesByStatus(context.Background(), types.TransactionReplaced)
	assert.Equal(t, 2, len(replacedTx))
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884", replacedTx[0].TxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887", replacedTx[1].TxHash)

	newPendingTx, _ := dbStorage.GetTxHashesByStatus(context.Background(), types.TransactionPending)
	assert.Equal(t, 2, len(newPendingTx))
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f885", newPendingTx[0].TxHash)
	assert.Equal(t, 1, newPendingTx[0].RetryCount)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f888", newPendingTx[1].TxHash)
	assert.Equal(t, 1, newPendingTx[1].RetryCount)

	var rbfHistory []types.RBFTransactionHistory
	err := sqlxDB.SelectContext(context.Background(), &rbfHistory, `SELECT * FROM rbf_transaction_history`)
	if err != nil {
		panic(err)
	}
	assert.Equal(t, 2, len(rbfHistory))
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884", rbfHistory[0].OldTxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887", rbfHistory[1].OldTxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f885", rbfHistory[0].NewTxHash)
	assert.Equal(t, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f888", rbfHistory[1].NewTxHash)

}

func seedDatabase(dbStorage Storage) {
	err := dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f881",
		types.TransactionPending, "farm_sub_account_name_1", 0)
	if err != nil {
		panic(err)
	}
	err = dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f882",
		types.TransactionPending, "farm_sub_account_name_1", 0)
	if err != nil {
		panic(err)
	}
	err = dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f883",
		types.TransactionPending, "farm_sub_account_name_1", 2)
	if err != nil {
		panic(err)
	}
	err = dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884",
		types.TransactionPending, "farm_sub_account_name_1", 0)
	if err != nil {
		panic(err)
	}
	err = dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f886",
		types.TransactionPending, "farm_sub_account_name_1", 2)
	if err != nil {
		panic(err)
	}
	err = dbStorage.SaveTxHashWithStatus(context.Background(), "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887",
		types.TransactionPending, "farm_sub_account_name_1", 0)
	if err != nil {
		panic(err)
	}
}

func setupMockApiRequesterRetryService() *mockAPIRequester {
	apiRequester := &mockAPIRequester{}

	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "farm_1_denom_1").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("BumpFee", mock.Anything, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884").Return(
		"b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f885", nil)
	apiRequester.On("BumpFee", mock.Anything, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887").Return(
		"b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f888", nil)

	return apiRequester
}

func setupMockBtcClientRetryService() *mockBtcClient {
	btcClient := &mockBtcClient{}

	confirmedTxHash1 := &btcjson.TxRawResult{Confirmations: 5}
	arg1, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f881")
	btcClient.On("GetRawTransactionVerbose", arg1).Return(confirmedTxHash1, nil)

	confirmedTxHash2 := &btcjson.TxRawResult{Confirmations: 3}
	arg2, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f882")
	btcClient.On("GetRawTransactionVerbose", arg2).Return(confirmedTxHash2, nil)

	unconfirmedTxHash1 := &btcjson.TxRawResult{Confirmations: 0}
	arg3, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f883")
	btcClient.On("GetRawTransactionVerbose", arg3).Return(unconfirmedTxHash1, nil)

	unconfirmedTxHash2 := &btcjson.TxRawResult{Confirmations: 0}
	arg4, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884")
	btcClient.On("GetRawTransactionVerbose", arg4).Return(unconfirmedTxHash2, nil)

	failedTransactionHash := &btcjson.TxRawResult{Confirmations: 0}
	arg5, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f886")
	btcClient.On("GetRawTransactionVerbose", arg5).Return(failedTransactionHash, nil)

	unconfirmedTxHash3 := &btcjson.TxRawResult{Confirmations: 0}
	arg6, _ := chainhash.NewHashFromStr("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887")
	btcClient.On("GetRawTransactionVerbose", arg6).Return(unconfirmedTxHash3, nil)

	btcClient.On("LoadWallet", "farm_sub_account_name_1").Return(&btcjson.LoadWalletResult{}, nil)

	btcClient.On("UnloadWallet", mock.Anything).Return(nil)

	btcClient.On("WalletPassphrase", mock.Anything, mock.Anything).Return(nil)

	btcClient.On("WalletLock").Return(nil)

	return btcClient

}

func setupMockStorageRetryService() *mockStorage {
	storage := &mockStorage{}

	var uncomfirmedTransactions = []types.TransactionHashWithStatus{
		{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f881",
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f882",
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f883", // skipped
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884",
			TimeSent:           10,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
	}

	uncomfirmedTransactions = append(uncomfirmedTransactions, types.TransactionHashWithStatus{
		TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f886",
		TimeSent:           10,
		FarmSubAccountName: "farm_sub_account_name_1",
		RetryCount:         2,
	})

	uncomfirmedTransactions = append(uncomfirmedTransactions, types.TransactionHashWithStatus{
		TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887",
		TimeSent:           10,
		FarmSubAccountName: "farm_sub_account_name_1",
		RetryCount:         0,
	})

	storage.On("GetTxHashesByStatus", mock.Anything, types.TransactionPending).Return(uncomfirmedTransactions, nil)
	storage.On("UpdateTransactionsStatus", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("SaveTxHashWithStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("SaveRBFTransactionInformation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("GetApprovedFarms", mock.Anything).Return(nil, nil)

	return storage
}

func tearDownDatabaseRBF(sqlxDB *sqlx.DB) {
	_, err := sqlxDB.Exec("TRUNCATE TABLE statistics_tx_hash_status")
	if err != nil {
		panic(err)
	}
	_, err = sqlxDB.Exec("TRUNCATE TABLE rbf_transaction_history")
	if err != nil {
		panic(err)
	}
}

type mockHelperRetry struct {
}

func (_ *mockHelperRetry) DaysIn(m time.Month, year int) int {
	panic("not used")
}

func (_ *mockHelperRetry) Date() (year int, month time.Month, day int) {
	panic("not used")
}

func (_ *mockHelperRetry) Unix() int64 {
	return 4132020742
}

func (_ *mockHelperRetry) SendMail(message string) error {
	return nil
}
