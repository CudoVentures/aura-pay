package tokenised_infrastructure_rewarder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWorkerShouldReturnIfContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	Start(ctx, &infrastructure.Config{}, nil, nil, &sync.Mutex{}, time.Second*1)

	require.Error(t, ctx.Err())
}

func TestWorkerShouldReturnIfContextIsCanceledDuringProcessPayment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mps := &mockPayService{}
	mps.On("Execute", mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		cancel()
	})

	mp := &mockProvider{}

	connCfg := &rpcclient.ConnConfig{
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default,
	}

	client, err := rpcclient.New(connCfg, nil)
	require.NoError(t, err)

	mp.On("InitBtcRpcClient").Return(client, nil)

	db, err := sqlx.Connect("sqlite3", ":memory:")
	require.NoError(t, err)

	mp.On("InitDBConnection").Return(db, nil)

	Start(ctx, &infrastructure.Config{
		WorkerFailureRetryDelay: 1 * time.Second,
	}, mps, mp, &sync.Mutex{}, 1*time.Second)

	require.Error(t, ctx.Err())
}

func TestWorkerShouldRetryIfRpcConnectionFails(t *testing.T) {
	mp := &mockProvider{}

	connCfg := &rpcclient.ConnConfig{
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default,
	}

	client, err := rpcclient.New(connCfg, nil)
	require.NoError(t, err)

	mp.On("InitBtcRpcClient").Return(client, errors.New("should fail"))

	ctx, cancel := context.WithCancel(context.Background())

	go Start(ctx, &infrastructure.Config{
		WorkerFailureRetryDelay: 200 * time.Millisecond,
	}, nil, mp, &sync.Mutex{}, 200*time.Millisecond)

	time.Sleep(1 * time.Second)

	cancel()

	require.Greater(t, mp.initBtcRpcClientCallsCount, 1)
}

func TestWorkerShouldRetryIfDbConnectionFails(t *testing.T) {
	mp := &mockProvider{}

	connCfg := &rpcclient.ConnConfig{
		HTTPPostMode: true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:   true, // Bitcoin core does not provide TLS by default,
	}

	client, err := rpcclient.New(connCfg, nil)
	require.NoError(t, err)

	mp.On("InitBtcRpcClient").Return(client, nil)

	db, err := sqlx.Connect("sqlite3", ":memory:")
	require.NoError(t, err)

	mp.On("InitDBConnection").Return(db, errors.New("should fail"))

	ctx, cancel := context.WithCancel(context.Background())

	go Start(ctx, &infrastructure.Config{
		WorkerFailureRetryDelay: 200 * time.Millisecond,
	}, nil, mp, &sync.Mutex{}, 200*time.Millisecond)

	time.Sleep(1 * time.Second)

	cancel()

	require.Greater(t, mp.initDbConnectionCallsCount, 1)
}

type mockPayService struct {
	mock.Mock
}

func (mps *mockPayService) Execute(ctx context.Context, btcClient services.BtcClient, storage services.Storage) error {
	args := mps.Called(ctx, btcClient, storage)
	return args.Error(0)
}

type mockProvider struct {
	mock.Mock
	initBtcRpcClientCallsCount int
	initDbConnectionCallsCount int
}

func (mp *mockProvider) InitBtcRpcClient() (*rpcclient.Client, error) {
	mp.initBtcRpcClientCallsCount += 1
	args := mp.Called()
	return args.Get(0).(*rpcclient.Client), args.Error(1)
}

func (mp *mockProvider) InitDBConnection() (*sqlx.DB, error) {
	mp.initDbConnectionCallsCount += 1
	args := mp.Called()
	return args.Get(0).(*sqlx.DB), args.Error(1)
}

type mockAPIRequester struct {
	mock.Mock
}

func (mar *mockAPIRequester) GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error) {
	args := mar.Called(ctx, collectionDenomId, nftId, fromTimestamp)
	return args.Get(0).(types.NftTransferHistory), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error) {
	args := mar.Called(ctx, farmName, sinceTimestamp)
	return args.Get(0).(float64), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmCollectionsFromHasura(ctx context.Context, farmId int64) (types.CollectionData, error) {
	args := mar.Called(ctx, farmId)
	return args.Get(0).(types.CollectionData), args.Error(1)
}

func (mar *mockAPIRequester) VerifyCollection(ctx context.Context, denomId string) (bool, error) {
	args := mar.Called(ctx, denomId)
	return args.Bool(0), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error) {
	args := mar.Called(ctx, denomIds)
	return args.Get(0).([]types.Collection), args.Error(1)
}

func (mar *mockAPIRequester) GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error) {
	args := mar.Called(ctx, cudosAddress, network, tokenId, denomId)
	return args.String(0), args.Error(1)
}
