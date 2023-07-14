package tokenised_infrastructure_rewarder

import (
	"context"
	"errors"

	"os"
	"sync"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	services "github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/services"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}
func TestWorkerShouldReturnIfContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	Start(ctx, cancel, &infrastructure.Config{}, nil, nil, &sync.Mutex{}, time.Second*1)

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

	Start(ctx, cancel, &infrastructure.Config{
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

	go Start(ctx, cancel, &infrastructure.Config{
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

	go Start(ctx, cancel, &infrastructure.Config{
		WorkerFailureRetryDelay: 200 * time.Millisecond,
	}, nil, mp, &sync.Mutex{}, 200*time.Millisecond)

	time.Sleep(1 * time.Second)

	cancel()

	require.Greater(t, mp.initDbConnectionCallsCount, 1)
}

func TestMaxErrorCountReached(t *testing.T) {
	config := &infrastructure.Config{
		ServiceMaxErrorCount: 5,
	}
	err := errors.New("test error")

	var receivedConfig *infrastructure.Config
	var receivedMessage string

	// Temporarily replace mSendMail function with a mock function
	tempSendMail := mSendMail
	mSendMail = func(config *infrastructure.Config, message string) {
		receivedConfig = config
		receivedMessage = message
	}
	defer func() { mSendMail = tempSendMail }() // Restore original function after the test

	maxErrorCountReached(config, err)

	assert.Equal(t, config, receivedConfig, "Expected config to be passed to sendMail")
	assert.Contains(t, receivedMessage, "Application has exceeded the ServiceMaxErrorCount", "Expected message to contain error")
}

// MockHelper is a mock of the infrastructure.Helper
type MockHelper struct {
	mock.Mock
}

func (m *MockHelper) SendMail(message string) error {
	args := m.Called(message)
	return args.Error(0)
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
