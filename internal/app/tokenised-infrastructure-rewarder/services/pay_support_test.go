package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetUnspentTxDetails_Success(t *testing.T) {
	ctx := context.Background()
	txID := "3a7e47e76d63e7f9a1e0b8d8f1f0f0c200a15a19c8e2e0a2a1a0f64e9d6ab8f1"
	unspentResult := btcjson.ListUnspentResult{TxID: txID}

	expectedHash, _ := chainhash.NewHashFromStr(txID)
	expectedTxRawResult := btcjson.TxRawResult{
		Txid: txID,
		Time: 1234567890,
	}

	btcClient := new(mockBtcClient)
	btcClient.On("GetRawTransactionVerbose", expectedHash).Return(&expectedTxRawResult, nil).Once()
	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	txRawResult, err := payService.getUnspentTxDetails(ctx, btcClient, unspentResult)

	assert.NoError(t, err)
	assert.Equal(t, expectedTxRawResult, txRawResult)
	btcClient.AssertExpectations(t)
}

func TestGetUnspentTxDetails_InvalidTxID(t *testing.T) {
	ctx := context.Background()
	unspentResult := btcjson.ListUnspentResult{TxID: "invalid_tx_id"}

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	_, err := payService.getUnspentTxDetails(ctx, nil, unspentResult)

	assert.Error(t, err)
}

func TestGetUnspentTxDetails_GetRawTransactionVerboseError(t *testing.T) {
	ctx := context.Background()
	txID := "3a7e47e76d63e7f9a1e0b8d8f1f0f0c200a15a19c8e2e0a2a1a0f64e9d6ab8f1"
	unspentResult := btcjson.ListUnspentResult{TxID: txID}

	expectedHash, _ := chainhash.NewHashFromStr(txID)
	expectedError := errors.New("get_raw_transaction_error")

	btcClient := new(mockBtcClient)
	btcClient.On("GetRawTransactionVerbose", expectedHash).Return(&btcjson.TxRawResult{}, expectedError).Once()

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	_, err := payService.getUnspentTxDetails(ctx, btcClient, unspentResult)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	btcClient.AssertExpectations(t)
}

func TestGetUnspentTxsForFarm_Success(t *testing.T) {
	ctx := context.Background()
	farmAddresses := []string{"address1", "address2"}

	unspentTransactions := []btcjson.ListUnspentResult{
		{TxID: "tx1", Address: "address1"},
		{TxID: "tx2", Address: "address2"},
		{TxID: "tx3", Address: "address3"},
	}

	filteredUnspentTransactions := []btcjson.ListUnspentResult{
		{TxID: "tx1", Address: "address1"},
		{TxID: "tx2", Address: "address2"},
	}

	utxo1 := types.UTXOTransaction{
		Id:               "tx1",
		PaymentTimestamp: int64(1234567890),
	}

	utxo2 := types.UTXOTransaction{
		Id:               "tx2",
		PaymentTimestamp: int64(1234567890),
	}

	btcClient := new(mockBtcClient)
	btcClient.On("ListUnspent").Return(unspentTransactions, nil)

	storage := new(mockStorage)
	storage.On("GetUTXOTransaction", mock.Anything, "tx1").Return(utxo1, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "tx2").Return(utxo2, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "tx3").Return(types.UTXOTransaction{}, nil)

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	validUnspentTxs, err := payService.getUnspentTxsForFarm(ctx, btcClient, storage, farmAddresses)

	assert.NoError(t, err)
	assert.Equal(t, filteredUnspentTransactions, validUnspentTxs)
	btcClient.AssertExpectations(t)
	storage.AssertExpectations(t)
}

func TestGetUnspentTxsForFarm_ListUnspentError(t *testing.T) {
	ctx := context.Background()
	farmAddresses := []string{"address1", "address2"}

	expectedError := errors.New("list_unspent_error")

	btcClient := new(mockBtcClient)
	btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{}, expectedError)

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	_, err := payService.getUnspentTxsForFarm(ctx, btcClient, nil, farmAddresses)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	btcClient.AssertExpectations(t)
}

func TestGetUnspentTxsForFarm_StorageError(t *testing.T) {
	ctx := context.Background()
	farmAddresses := []string{"address1", "address2"}

	unspentTransactions := []btcjson.ListUnspentResult{
		{TxID: "tx1", Address: "address1"},
		{TxID: "tx2", Address: "address2"},
		{TxID: "tx3", Address: "address3"},
	}

	expectedError := errors.New("storage_error")

	btcClient := new(mockBtcClient)
	btcClient.On("ListUnspent").Return(unspentTransactions, nil)

	storage := new(mockStorage)
	storage.On("GetUTXOTransaction", mock.Anything, "tx1").Return(types.UTXOTransaction{}, expectedError)

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	_, err := payService.getUnspentTxsForFarm(ctx, btcClient, storage, farmAddresses)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	btcClient.AssertExpectations(t)
}

func TestGetUnspentTxsForFarm_NoValidUnspentTransactions(t *testing.T) {
	ctx := context.Background()
	farmAddresses := []string{"address1", "address2"}

	unspentTransactions := []btcjson.ListUnspentResult{
		{TxID: "tx1", Address: "address3"},
		{TxID: "tx2", Address: "address4"},
	}

	utxo1 := types.UTXOTransaction{
		Id:               "tx1",
		PaymentTimestamp: int64(1234567890),
	}

	utxo2 := types.UTXOTransaction{
		Id:               "tx2",
		PaymentTimestamp: int64(1234567890),
	}

	btcClient := new(mockBtcClient)
	btcClient.On("ListUnspent").Return(unspentTransactions, nil)

	storage := new(mockStorage)
	storage.On("GetUTXOTransaction", mock.Anything, "tx1").Return(utxo1, nil)
	storage.On("GetUTXOTransaction", mock.Anything, "tx2").Return(utxo2, nil)

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	validUnspentTxs, err := payService.getUnspentTxsForFarm(ctx, btcClient, storage, farmAddresses)

	assert.NoError(t, err)
	assert.Empty(t, validUnspentTxs)
	btcClient.AssertExpectations(t)
	storage.AssertExpectations(t)
}

func TestGetUnspentTxsForFarm_EmptyUnspentTransactions(t *testing.T) {
	ctx := context.Background()
	farmAddresses := []string{"address1", "address2"}

	btcClient := new(mockBtcClient)
	btcClient.On("ListUnspent").Return([]btcjson.ListUnspentResult{}, nil)

	payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

	validUnspentTxs, err := payService.getUnspentTxsForFarm(ctx, btcClient, nil, farmAddresses)

	assert.NoError(t, err)
	assert.Empty(t, validUnspentTxs)
	btcClient.AssertExpectations(t)
}

func TestVerifyCollectionIds(t *testing.T) {
	ctx := context.Background()

	collections := []types.CudosMarketsCollection{
		{
			Id:      1,
			DenomId: "collection1",
		},
		{
			Id:      2,
			DenomId: "collection2",
		},
	}

	apiRequester := new(mockAPIRequester)
	apiRequester.On("VerifyCollection", mock.Anything, "collection1").Return(true, nil)
	apiRequester.On("VerifyCollection", mock.Anything, "collection2").Return(false, nil)

	payService := NewPayService(&infrastructure.Config{}, apiRequester, &mockHelper{}, &types.BtcNetworkParams{})

	verifiedCollectionIds, err := payService.verifyCollectionIds(ctx, collections)

	assert.NoError(t, err)
	assert.Equal(t, []string{"collection1"}, verifiedCollectionIds)
	apiRequester.AssertExpectations(t)
}

func TestVerifyCollectionIds_ErrorDuringVerification(t *testing.T) {
	ctx := context.Background()

	collections := []types.CudosMarketsCollection{
		{
			Id:      1,
			DenomId: "collection1",
		},
		{
			Id:      2,
			DenomId: "collection2",
		},
	}

	apiRequester := new(mockAPIRequester)
	apiRequester.On("VerifyCollection", mock.Anything, "collection1").Return(false, errors.New("verification error"))

	payService := NewPayService(&infrastructure.Config{}, apiRequester, &mockHelper{}, &types.BtcNetworkParams{})

	_, err := payService.verifyCollectionIds(ctx, collections)

	assert.Error(t, err)
	apiRequester.AssertExpectations(t)
}

func TestFilterExpiredBeforePeriodNFTs(t *testing.T) {
	testCases := []struct {
		name                  string
		farmCollections       []types.Collection
		nonExpiredCollections []types.Collection
		periodStart           int64
		expectedNonExpired    int
	}{
		{
			name: "All NFTs are valid",
			farmCollections: []types.Collection{
				{
					Denom: types.Denom{
						Id: "collection1",
					},
					Nfts: []types.NFT{
						{Id: "nft1", DataJson: types.NFTDataJson{ExpirationDate: 100}},
						{Id: "nft2", DataJson: types.NFTDataJson{ExpirationDate: 200}},
					},
				},
				{
					Denom: types.Denom{
						Id: "collection2",
					},
					Nfts: []types.NFT{
						{Id: "nft3", DataJson: types.NFTDataJson{ExpirationDate: 300}},
					},
				},
			},
			nonExpiredCollections: []types.Collection{
				{
					Denom: types.Denom{
						Id: "collection1",
					},
					Nfts: []types.NFT{
						{Id: "nft1", DataJson: types.NFTDataJson{ExpirationDate: 100}},
						{Id: "nft2", DataJson: types.NFTDataJson{ExpirationDate: 200}},
					},
				},
				{
					Denom: types.Denom{
						Id: "collection2",
					},
					Nfts: []types.NFT{
						{Id: "nft3", DataJson: types.NFTDataJson{ExpirationDate: 300}},
					},
				},
			},
			periodStart:        50,
			expectedNonExpired: 3,
		},
		{
			name: "Some NFTs are expired",
			farmCollections: []types.Collection{
				{
					Denom: types.Denom{
						Id: "collection1",
					},
					Nfts: []types.NFT{
						{Id: "nft1", DataJson: types.NFTDataJson{ExpirationDate: 100}},
						{Id: "nft2", DataJson: types.NFTDataJson{ExpirationDate: 200}},
					},
				},
				{
					Denom: types.Denom{
						Id: "collection2",
					},
					Nfts: []types.NFT{
						{Id: "nft3", DataJson: types.NFTDataJson{ExpirationDate: 30}},
					},
				},
			},
			nonExpiredCollections: []types.Collection{
				{
					Denom: types.Denom{
						Id: "collection1",
					},
					Nfts: []types.NFT{
						{Id: "nft1", DataJson: types.NFTDataJson{ExpirationDate: 100}},
						{Id: "nft2", DataJson: types.NFTDataJson{ExpirationDate: 200}},
					},
				},
			},
			periodStart:        50,
			expectedNonExpired: 2,
		},
		{
			name: "All NFTs are expired",
			farmCollections: []types.Collection{
				{
					Denom: types.Denom{
						Id: "collection1",
					},
					Nfts: []types.NFT{
						{Id: "nft1", DataJson: types.NFTDataJson{ExpirationDate: 10}},
						{Id: "nft2", DataJson: types.NFTDataJson{ExpirationDate: 20}},
					},
				},
				{
					Denom: types.Denom{
						Id: "collection2",
					},
					Nfts: []types.NFT{
						{Id: "nft3", DataJson: types.NFTDataJson{ExpirationDate: 30}},
					},
				},
			},
			nonExpiredCollections: []types.Collection{},
			periodStart:           50,
			expectedNonExpired:    0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

			nonExpiredNFTsCount := payService.filterExpiredBeforePeriodNFTs(tc.farmCollections, tc.periodStart)
			assert.Equal(t, tc.expectedNonExpired, nonExpiredNFTsCount)

			// Ensure that expired NFTs are removed from the collections
			for i, collection := range tc.farmCollections {
				for j, nft := range collection.Nfts {
					assert.True(t, nft.DataJson.ExpirationDate >= tc.periodStart)
					assert.Equal(t, tc.nonExpiredCollections[i].Nfts[j], nft)
				}
			}
		})
	}
}

func TestGetNftTimestamps(t *testing.T) {
	testCases := []struct {
		name               string
		payoutTimes        []types.NFTStatistics
		nftTransferHistory []types.NftTransferEvent
		mintTimestamp      int64
		denomId            string
		periodEnd          int64
		nft                types.NFT
		expectedStart      int64
		expectedEnd        int64
		expectedErr        error
	}{
		{
			name: "Normal scenario",
			payoutTimes: []types.NFTStatistics{
				{PayoutPeriodEnd: 1000},
				{PayoutPeriodEnd: 2000},
			},
			nftTransferHistory: []types.NftTransferEvent{{Timestamp: 1500}},
			mintTimestamp:      int64(1500),
			denomId:            "denom1",
			periodEnd:          2500,
			nft:                types.NFT{DataJson: types.NFTDataJson{ExpirationDate: 3000}},
			expectedStart:      2000,
			expectedEnd:        2500,
			expectedErr:        nil,
		},
		{
			name: "Expired NFT",
			payoutTimes: []types.NFTStatistics{
				{PayoutPeriodEnd: 1000},
				{PayoutPeriodEnd: 2000},
			},
			nftTransferHistory: []types.NftTransferEvent{{Timestamp: 1500}},
			denomId:            "denom1",
			periodEnd:          2500,
			nft:                types.NFT{DataJson: types.NFTDataJson{ExpirationDate: 2300}},
			expectedStart:      2000,
			expectedEnd:        2300,
			expectedErr:        nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStorage := &mockStorage{}
			mockStorage.On("GetPayoutTimesForNFT", mock.Anything, tc.denomId, mock.Anything).Return(tc.payoutTimes, nil)

			payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

			start, end, err := payService.getNftTimestamps(context.Background(), mockStorage, tc.nft, tc.mintTimestamp, tc.nftTransferHistory, tc.denomId, tc.periodEnd)

			assert.Equal(t, tc.expectedErr, err)
			assert.Equal(t, tc.expectedStart, start)
			assert.Equal(t, tc.expectedEnd, end)
		})
	}
}

func TestConvertAmountToBTC(t *testing.T) {
	// Arrange
	destinationAddressesWithAmount := map[string]types.AmountInfo{
		"address1": {
			Amount:           decimal.NewFromFloat(124.124124126),
			ThresholdReached: true,
		},
		"address2": {
			Amount:           decimal.NewFromFloat(0.123),
			ThresholdReached: true,
		},
		"address3": {
			Amount:           decimal.NewFromFloat(12412323.1),
			ThresholdReached: true,
		},
	}

	// Act
	result, err := convertAmountToBTC(destinationAddressesWithAmount)

	//Assert
	require.NoError(t, err)
	require.Equal(t, map[string]float64{
		"address1": 124.124124126,
		"address2": 0.123,
		"address3": 12412323.1,
	}, result, "Amounts are not equal to the given up to 8th digit")
}

type GetCurrentAccumulatedAmountForAddressCalls struct {
	result decimal.Decimal
	err    error
}

func TestFilterByPaymentThreshold(t *testing.T) {

	testCases := []struct {
		desc                                       string
		destinationAddressesWithAmountsBtcDecimal  map[string]decimal.Decimal
		getCurrentAccumulatedAmountForAddressCalls map[string]GetCurrentAccumulatedAmountForAddressCalls
		setInitialAccumulatedAmountForAddressCalls map[string]error
		farmId                                     int64
		expectedResult                             map[string]types.AmountInfo
		expectedError                              error
	}{
		{
			desc: "threshold not reached for both addresses",
			destinationAddressesWithAmountsBtcDecimal: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.0004),
				"address2": decimal.NewFromFloat(0.0006),
			},
			getCurrentAccumulatedAmountForAddressCalls: map[string]GetCurrentAccumulatedAmountForAddressCalls{
				"address1": {decimal.NewFromFloat(0.0005), nil},
				"address2": {decimal.NewFromFloat(0.0002), nil},
			},
			setInitialAccumulatedAmountForAddressCalls: map[string]error{"address2": nil},
			expectedResult: map[string]types.AmountInfo{
				"address1": {Amount: decimal.NewFromFloat(0.0009), ThresholdReached: false},
				"address2": {Amount: decimal.NewFromFloat(0.0008), ThresholdReached: false},
			},
			expectedError: nil,
		},
		{
			desc: "threshold reached for one address",
			destinationAddressesWithAmountsBtcDecimal: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.0006),
				"address2": decimal.NewFromFloat(0.0004),
			},
			getCurrentAccumulatedAmountForAddressCalls: map[string]GetCurrentAccumulatedAmountForAddressCalls{
				"address1": {decimal.NewFromFloat(0.0005), nil},
				"address2": {decimal.NewFromFloat(0.0002), nil},
			},
			setInitialAccumulatedAmountForAddressCalls: map[string]error{"address2": nil},
			expectedResult: map[string]types.AmountInfo{
				"address1": {Amount: decimal.NewFromFloat(0.0011), ThresholdReached: true},
				"address2": {Amount: decimal.NewFromFloat(0.0006), ThresholdReached: false},
			},
			expectedError: nil,
		},
		{
			desc: "error when getting accumulated amount",
			destinationAddressesWithAmountsBtcDecimal: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.0006),
			},
			getCurrentAccumulatedAmountForAddressCalls: map[string]GetCurrentAccumulatedAmountForAddressCalls{
				"address1": {decimal.Decimal{}, errors.New("error getting accumulated amount")},
			},
			setInitialAccumulatedAmountForAddressCalls: map[string]error{"address2": nil},
			expectedResult: nil,
			expectedError:  errors.New("error getting accumulated amount"),
		},
		{
			desc: "error when setting initial accumulated amount",
			destinationAddressesWithAmountsBtcDecimal: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.0006),
			},
			getCurrentAccumulatedAmountForAddressCalls: map[string]GetCurrentAccumulatedAmountForAddressCalls{
				"address1": {decimal.Decimal{}, sql.ErrNoRows},
			},
			setInitialAccumulatedAmountForAddressCalls: map[string]error{"address1": errors.New("error setting initial accumulated amount")},
			expectedResult: nil,
			expectedError:  errors.New("error setting initial accumulated amount"),
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctx := context.Background()
			config := infrastructure.Config{GlobalPayoutThresholdInBTC: 0.001}
			mockStorage := mockStorage{}

			for address, result := range tC.getCurrentAccumulatedAmountForAddressCalls {
				mockStorage.On("GetCurrentAcummulatedAmountForAddress", mock.Anything, address, mock.Anything).Return(result.result, result.err).Once()
			}
			for address, err := range tC.setInitialAccumulatedAmountForAddressCalls {
				mockStorage.On("SetInitialAccumulatedAmountForAddress", mock.Anything, address, mock.Anything, mock.Anything).Return(err).Once()
			}
			payService := NewPayService(&config, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})

			_, addressesToSend, _, err := payService.filterByPaymentThreshold(ctx, tC.destinationAddressesWithAmountsBtcDecimal, &mockStorage, tC.farmId)

			if (err == nil && tC.expectedError != nil) || (err != nil && tC.expectedError == nil) || (err != nil && tC.expectedError != nil && err.Error() != tC.expectedError.Error()) {
				t.Errorf("Expected error %v, but got %v", tC.expectedError, err)
			}

			if !reflect.DeepEqual(addressesToSend, tC.expectedResult) {
				t.Errorf("Expected result %v, but got %v", tC.expectedResult, addressesToSend)
			}
		})
	}
}

func TestFindCurrentPayoutPeriod(t *testing.T) {
	tests := []struct {
		name               string
		payoutTimes        []types.NFTStatistics
		mintTimestamp      int64
		nftTransferHistory []types.NftTransferEvent
		expectedStart      int64
	}{
		{
			name:          "first_payout",
			payoutTimes:   []types.NFTStatistics{},
			mintTimestamp: int64(100),
			nftTransferHistory: []types.NftTransferEvent{
				{Timestamp: 100},
			},
			expectedStart: 100,
		},
		{
			name: "subsequent_payout",
			payoutTimes: []types.NFTStatistics{
				{PayoutPeriodEnd: 200},
				{PayoutPeriodEnd: 300},
			},
			mintTimestamp:      int64(100),
			nftTransferHistory: []types.NftTransferEvent{},
			expectedStart:      300,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})
			start, err := payService.findCurrentPayoutPeriod(tc.payoutTimes, tc.mintTimestamp)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStart, start)
		})
	}
}

func TestFilterUnspentTransactions(t *testing.T) {
	var emptyList []btcjson.ListUnspentResult

	tests := []struct {
		name                string
		unspentTransactions []btcjson.ListUnspentResult
		storage             *mockStorage
		farmAddresses       []string
		expectedResult      []btcjson.ListUnspentResult
	}{
		{
			name: "valid_transaction",
			unspentTransactions: []btcjson.ListUnspentResult{
				{
					TxID:    "validTx1",
					Address: "address1",
					Amount:  0.01,
				},
			},
			storage: func() *mockStorage {
				ms := new(mockStorage)
				ms.On("GetUTXOTransaction", mock.Anything, "validTx1").Return(types.UTXOTransaction{Processed: false}, nil)
				return ms
			}(),
			farmAddresses: []string{"address1", "address2"},
			expectedResult: []btcjson.ListUnspentResult{
				{
					TxID:    "validTx1",
					Address: "address1",
					Amount:  0.01,
				},
			},
		},
		{
			name: "processed_transaction",
			unspentTransactions: []btcjson.ListUnspentResult{
				{
					TxID:    "processedTx1",
					Address: "address1",
					Amount:  0.01,
				},
			},
			storage: func() *mockStorage {
				ms := new(mockStorage)
				ms.On("GetUTXOTransaction", mock.Anything, "processedTx1").Return(types.UTXOTransaction{Processed: true}, nil)
				return ms
			}(),
			farmAddresses:  []string{"address1", "address2"},
			expectedResult: emptyList,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			result, err := filterUnspentTransactions(ctx, tc.unspentTransactions, tc.storage, tc.farmAddresses)

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestRemoveAddressesWithZeroReward(t *testing.T) {
	tests := []struct {
		name                 string
		destinationAddresses map[string]decimal.Decimal
		expectedResult       map[string]decimal.Decimal
	}{
		{
			name: "remove_zero_reward_addresses",
			destinationAddresses: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address2": decimal.NewFromFloat(0),
				"address3": decimal.NewFromFloat(0.05),
				"address4": decimal.NewFromFloat(0),
			},
			expectedResult: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address3": decimal.NewFromFloat(0.05),
			},
		},
		{
			name: "no_addresses_to_remove",
			destinationAddresses: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address3": decimal.NewFromFloat(0.05),
			},
			expectedResult: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address3": decimal.NewFromFloat(0.05),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			removeAddressesWithZeroReward(tc.destinationAddresses)
			assert.Equal(t, tc.expectedResult, tc.destinationAddresses)
		})
	}
}

func TestAddPaymentAmountToAddress(t *testing.T) {
	tests := []struct {
		name           string
		initialAmounts map[string]decimal.Decimal
		amountToAdd    decimal.Decimal
		address        string
		expectedResult map[string]decimal.Decimal
	}{
		{
			name: "add_amount_to_existing_address",
			initialAmounts: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address2": decimal.NewFromFloat(0.05),
			},
			amountToAdd: decimal.NewFromFloat(0.02),
			address:     "address1",
			expectedResult: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.03),
				"address2": decimal.NewFromFloat(0.05),
			},
		},
		{
			name: "add_amount_to_new_address",
			initialAmounts: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address2": decimal.NewFromFloat(0.05),
			},
			amountToAdd: decimal.NewFromFloat(0.03),
			address:     "address3",
			expectedResult: map[string]decimal.Decimal{
				"address1": decimal.NewFromFloat(0.01),
				"address2": decimal.NewFromFloat(0.05),
				"address3": decimal.NewFromFloat(0.03),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addPaymentAmountToAddress(tc.initialAmounts, tc.amountToAdd, tc.address)
			assert.Equal(t, tc.expectedResult, tc.initialAmounts)
		})
	}
}

func TestValidateFarm(t *testing.T) {
	tests := []struct {
		name        string
		inputFarm   types.Farm
		expectError bool
	}{
		{
			name: "valid_farm",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "wallet1",
				MaintenanceFeeInBtc:                0.01,
				AddressForReceivingRewardsFromPool: "address1",
				MaintenanceFeePayoutAddress:        "address2",
				LeftoverRewardPayoutAddress:        "address3",
			},
			expectError: false,
		},
		{
			name: "empty_wallet_name",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "",
				MaintenanceFeeInBtc:                0.01,
				AddressForReceivingRewardsFromPool: "address1",
				MaintenanceFeePayoutAddress:        "address2",
				LeftoverRewardPayoutAddress:        "address3",
			},
			expectError: true,
		},
		{
			name: "negative_maintenance_fee",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "wallet1",
				MaintenanceFeeInBtc:                -0.01,
				AddressForReceivingRewardsFromPool: "address1",
				MaintenanceFeePayoutAddress:        "address2",
				LeftoverRewardPayoutAddress:        "address3",
			},
			expectError: true,
		},
		{
			name: "empty_AddressForReceivingRewardsFromPool",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "wallet1",
				MaintenanceFeeInBtc:                0.01,
				AddressForReceivingRewardsFromPool: "",
				MaintenanceFeePayoutAddress:        "address2",
				LeftoverRewardPayoutAddress:        "address3",
			},
			expectError: true,
		},
		{
			name: "empty_MaintenanceFeePayoutAddress",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "wallet1",
				MaintenanceFeeInBtc:                0.01,
				AddressForReceivingRewardsFromPool: "address1",
				MaintenanceFeePayoutAddress:        "",
				LeftoverRewardPayoutAddress:        "address3",
			},
			expectError: true,
		},
		{
			name: "empty_LeftoverRewardPayoutAddress",
			inputFarm: types.Farm{
				Id:                                 1,
				RewardsFromPoolBtcWalletName:       "wallet1",
				MaintenanceFeeInBtc:                0.01,
				AddressForReceivingRewardsFromPool: "address1",
				MaintenanceFeePayoutAddress:        "address2",
				LeftoverRewardPayoutAddress:        "",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFarm(tc.inputFarm)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadWallet(t *testing.T) {
	tests := []struct {
		name            string
		farmName        string
		loadWalletError error
		failsPerFarm    map[string]int
		expectSuccess   bool
		expectError     bool
	}{
		{
			name:            "successful_load",
			farmName:        "farm1",
			loadWalletError: nil,
			failsPerFarm:    map[string]int{},
			expectSuccess:   true,
			expectError:     false,
		},
		{
			name:            "failed_once",
			farmName:        "farm1",
			loadWalletError: errors.New("failed to load wallet"),
			failsPerFarm:    map[string]int{"farm1": 0},
			expectSuccess:   false,
			expectError:     false,
		},
		{
			name:            "failed_15_times",
			farmName:        "farm1",
			loadWalletError: errors.New("failed to load wallet"),
			failsPerFarm:    map[string]int{"farm1": 14},
			expectSuccess:   false,
			expectError:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockBtcClient := &mockBtcClient{}
			mockBtcClient.On("LoadWallet", tc.farmName).Return(&btcjson.LoadWalletResult{}, tc.loadWalletError)

			payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester{}, &mockHelper{}, &types.BtcNetworkParams{})
			payService.btcWalletOpenFailsPerFarm = tc.failsPerFarm

			success, err := payService.loadWallet(mockBtcClient, tc.farmName)

			assert.Equal(t, tc.expectSuccess, success)
			if tc.expectError {
				assert.Error(t, err)
				assert.Equal(t, fmt.Errorf("failed to load wallet %s for 15 times", tc.farmName), err)
			} else {
				assert.NoError(t, err)
			}

			mockBtcClient.AssertExpectations(t)
		})
	}
}

func TestGetLastUTXOTransactionTimestamp(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name                                       string
		farm                                       types.Farm
		mockGetLastUTXOTransactionByFarmIdResponse types.UTXOTransaction
		mockGetLastUTXOTransactionByFarmIdError    error
		mockGetFarmStartTimeResponse               int64
		mockGetFarmStartTimeError                  error
		expectedResult                             int64
		expectedError                              error
		shouldCallGetFarmStartTime                 bool
	}{
		{
			name: "get_last_utxo_transaction_timestamp_success",
			farm: types.Farm{Id: 1, SubAccountName: "test_farm"},
			mockGetLastUTXOTransactionByFarmIdResponse: types.UTXOTransaction{PaymentTimestamp: 1628923421},
			mockGetLastUTXOTransactionByFarmIdError:    nil,
			expectedResult:                             1628923421,
			expectedError:                              nil,
			shouldCallGetFarmStartTime:                 false,
		},
		{
			name: "get_farm_start_time_success",
			farm: types.Farm{Id: 1, SubAccountName: "test_farm"},
			mockGetLastUTXOTransactionByFarmIdResponse: types.UTXOTransaction{PaymentTimestamp: 0},
			mockGetLastUTXOTransactionByFarmIdError:    nil,
			mockGetFarmStartTimeResponse:               1628923421,
			mockGetFarmStartTimeError:                  nil,
			expectedResult:                             1628923421,
			expectedError:                              nil,
			shouldCallGetFarmStartTime:                 true,
		},
		{
			name: "storage_error",
			farm: types.Farm{Id: 1, SubAccountName: "test_farm"},
			mockGetLastUTXOTransactionByFarmIdResponse: types.UTXOTransaction{},
			mockGetLastUTXOTransactionByFarmIdError:    errors.New("storage_error"),
			expectedResult:                             0,
			expectedError:                              errors.New("storage_error"),
			shouldCallGetFarmStartTime:                 false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockStorage := &mockStorage{}
			mockAPIRequester := &mockAPIRequester{}

			mockStorage.On("GetLastUTXOTransactionByFarmId", ctx, tc.farm.Id).Return(tc.mockGetLastUTXOTransactionByFarmIdResponse, tc.mockGetLastUTXOTransactionByFarmIdError)
			mockAPIRequester.On("GetFarmStartTime", ctx, tc.farm.SubAccountName).Return(tc.mockGetFarmStartTimeResponse, tc.mockGetFarmStartTimeError)

			payService := NewPayService(&infrastructure.Config{}, mockAPIRequester, &mockHelper{}, &types.BtcNetworkParams{})

			result, err := payService.getLastUTXOTransactionTimestamp(ctx, mockStorage, tc.farm)
			assert.Equal(t, tc.expectedResult, result)
			assert.Equal(t, tc.expectedError, err)

			mockStorage.AssertExpectations(t)

			if tc.shouldCallGetFarmStartTime {
				mockAPIRequester.AssertExpectations(t)
			}
		})
	}
}

func TestGetCollectionsWithNftsForFarm(t *testing.T) {
	testCases := []struct {
		desc                                     string
		farm                                     types.Farm
		collectionsData                          types.CollectionData
		collections                              []types.Collection
		verifiedDenomIds                         []string
		CudosMarketsCollections                  []types.CudosMarketsCollection
		expectedResultCollections                []types.Collection
		expectedResultCudosMarketsCollectionsMap map[string]types.CudosMarketsCollection
		expectedError                            error
	}{
		{
			desc: "successful case",
			farm: types.Farm{
				Id: 1,
			},
			collectionsData: types.CollectionData{
				Data: types.CData{
					DenomsByDataProperty: []types.DenomsByDataProperty{
						{Id: "denom1"},
						{Id: "denom2"},
					},
				},
			},
			collections: []types.Collection{
				{Denom: types.Denom{Id: "denom1"}},
				{Denom: types.Denom{Id: "denom2"}},
			},
			verifiedDenomIds: []string{"denom1", "denom2"},
			CudosMarketsCollections: []types.CudosMarketsCollection{
				{Id: 1, DenomId: "denom1"},
				{Id: 2, DenomId: "denom2"},
			},
			expectedResultCollections: []types.Collection{
				{Denom: types.Denom{Id: "denom1"}},
				{Denom: types.Denom{Id: "denom2"}},
			},
			expectedResultCudosMarketsCollectionsMap: map[string]types.CudosMarketsCollection{
				"denom1": {Id: 1, DenomId: "denom1"},
				"denom2": {Id: 2, DenomId: "denom2"},
			},
			expectedError: nil,
		},
		{
			desc: "failed case: missing CUDOS Markets collection",
			farm: types.Farm{
				Id: 1,
			},
			collectionsData: types.CollectionData{
				Data: types.CData{
					DenomsByDataProperty: []types.DenomsByDataProperty{
						{Id: "denom1"},
					},
				},
			},
			collections: []types.Collection{
				{Denom: types.Denom{Id: "denom1"}},
			},
			expectedResultCollections:                []types.Collection{},
			expectedResultCudosMarketsCollectionsMap: map[string]types.CudosMarketsCollection{},
			verifiedDenomIds:                         nil,
			CudosMarketsCollections:                  []types.CudosMarketsCollection{},
			expectedError:                            fmt.Errorf("CUDOS Markets collection not found by denom id {denom1}"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {

			ctx := context.Background()
			mockAPIRequester := mockAPIRequester{}
			mockStorage := mockStorage{}
			// mockAPIRequester.On("GetFarmCollectionsFromHasura", ctx, tc.farm.Id).Return(tc.collectionsData, nil).Once()
			mockAPIRequester.On("GetFarmCollectionsWithNFTs", ctx, tc.verifiedDenomIds).Return(tc.collections, nil).Once()

			for _, denomId := range tc.verifiedDenomIds {
				mockAPIRequester.On("VerifyCollection", ctx, denomId).Return(true, nil).Once()
			}

			mockStorage.On("CudosMarketsCollections", ctx, tc.farm.Id).Return(tc.CudosMarketsCollections, nil).Once()

			payService := NewPayService(&infrastructure.Config{}, &mockAPIRequester, &mockHelper{}, &types.BtcNetworkParams{})

			resultCollections, resultMap, err := payService.getCollectionsWithNftsForFarm(ctx, &mockStorage, tc.farm)

			assert.Equal(t, tc.expectedResultCollections, resultCollections)
			assert.Equal(t, tc.expectedResultCudosMarketsCollectionsMap, resultMap)
			assert.Equal(t, tc.expectedError, err)

			mockAPIRequester.AssertExpectations(t)
			mockStorage.AssertExpectations(t)
		})
	}
}
