package services

import (
	"context"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetryService_Execute(t *testing.T) {
	config := &infrastructure.Config{
		Network:                           "BTC",
		CUDOMaintenanceFeePercent:         50,
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
		if elem.Method == "UpdateTransactionsStatus" && elem.Arguments[3].(string) == "Completed" {
			completedTransactions++
		}
		if elem.Method == "UpdateTransactionsStatus" && elem.Arguments[3].(string) == "Failed" {
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

func setupMockApiRequesterRetryService() *mockAPIRequester {
	apiRequester := &mockAPIRequester{}

	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "farm_1_denom_1").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("BumpFee", mock.Anything, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f884").Return("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f885", nil)
	apiRequester.On("BumpFee", mock.Anything, "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f887").Return("b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f888", nil)

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
		types.TransactionHashWithStatus{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f881",
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		types.TransactionHashWithStatus{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f882",
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		types.TransactionHashWithStatus{
			TxHash:             "b58d7705c8980ad58e9ee981760bdb45f28adad898266b58ebde6dedfc93f883", // skipped
			TimeSent:           1666641098,
			FarmSubAccountName: "farm_sub_account_name_1",
			RetryCount:         0,
		},
		types.TransactionHashWithStatus{
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
	storage.On("UpdateTransactionsStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("SaveRBFTransactionHistory", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("SaveTxHashWithStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	storage.On("SaveRBFTransactionInformation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	return storage
}
