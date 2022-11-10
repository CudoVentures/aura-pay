package infrastructure

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HasuraURL                         string
	NodeRestUrl                       string
	BitcoinNodeUrl                    string
	BitcoinNodePort                   string
	BitcoinNodeUserName               string
	BitcoinNodePassword               string
	FoundryPoolAPIBaseURL             string
	FoundryPoolAPIKey                 string
	DbDriverName                      string
	DbHost                            string
	DbPort                            string
	DbUser                            string
	DbPassword                        string
	DbName                            string
	HasuraActionsURL                  string
	IsTesting                         bool
	AuraPoolBackEndUrl                string
	Network                           string
	CUDOMaintenanceFeePercent         float64
	CUDOFeePayoutAddress              string
	CUDOFeeOnAllBTC                   float64
	AuraPoolTestFarmWalletPassword    string
	WorkerProcessIntervalPayment      time.Duration
	WorkerProcessIntervalRetry        time.Duration
	WorkerFailureRetryDelay           time.Duration
	RBFTransactionRetryDelayInSeconds int
	RBFTransactionRetryMaxCount       int
}

// NewConfig New returns a new Config struct
func NewConfig() *Config {
	return &Config{
		HasuraURL:                         getEnv("HASURA_URL", ""),
		NodeRestUrl:                       getEnv("NODE_REST_URL", ""),
		BitcoinNodeUrl:                    getEnv("BITCOIN_NODE_URL", ""),
		BitcoinNodePort:                   getEnv("BITCOIN_NODE_PORT", ""),
		BitcoinNodeUserName:               getEnv("BITCOIN_NODE_USER_NAME", ""),
		BitcoinNodePassword:               getEnv("BITCOIN_NODE_PASSWORD", ""),
		FoundryPoolAPIBaseURL:             getEnv("FOUNDRY_POOL_API_BASE_URL", ""),
		FoundryPoolAPIKey:                 getEnv("FOUNDRY_POOL_API_KEY", ""),
		DbDriverName:                      getEnv("DB_DRIVER_NAME", ""),
		DbHost:                            getEnv("DB_HOST", ""),
		DbPort:                            getEnv("DB_PORT", ""),
		DbUser:                            getEnv("DB_USER", ""),
		DbPassword:                        getEnv("DB_PASSWORD", ""),
		DbName:                            getEnv("DB_NAME", ""),
		HasuraActionsURL:                  getEnv("HASURA_ACTIONS_URL", ""),
		IsTesting:                         getEnvAsBool("IS_TESTING", true),
		AuraPoolBackEndUrl:                getEnv("AURA_POOL_BACKEND_URL", ""),
		Network:                           getEnv("NETWORK", ""),
		CUDOMaintenanceFeePercent:         getEnvAsFloat64("CUDO_MAINTENANCE_FEE_PERCENT", 10.0),
		CUDOFeeOnAllBTC:                   getEnvAsFloat64("CUDO_FEE_ON_ALL_BTC", 2.0),
		CUDOFeePayoutAddress:              getEnv("CUDO_FEE_PAYOUT_ADDRESS", ""),
		AuraPoolTestFarmWalletPassword:    getEnv("AURA_POOL_TEST_FARM_WALLET_PASSWORD", ""),
		WorkerProcessIntervalPayment:      getEnvAsDuration("WORKER_PROCESS_INTERVAL_PAYMENT", time.Second*5),
		WorkerProcessIntervalRetry:        getEnvAsDuration("WORKER_PROCESS_INTERVAL_RETRY", time.Second*13),
		WorkerFailureRetryDelay:           getEnvAsDuration("WORKER_FAILURE_RETRY_DELAY", time.Second*5),
		RBFTransactionRetryDelayInSeconds: getEnvAsInt("RBF_TRANSACTION_RETRY_DELAY_IN_SECONDS", 18000),
		RBFTransactionRetryMaxCount:       getEnvAsInt("RBF_TRANSACTION_RETRY_MAX_COUNT", 2),
	}
}

// TODO: NewTestConfig() *Config {...}

// Simple helper function to read an environment or return a default value
func getEnv(key string, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultVal
}

// Simple helper function to read an environment variable into integer or return a default value
func getEnvAsInt(name string, defaultVal int) int {
	valueStr := getEnv(name, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}

	return defaultVal
}

// Simple helper function to read an environment variable into integer or return a default value
func getEnvAsFloat64(name string, defaultVal float64) float64 {
	valueStr := getEnv(name, "")
	if value, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64); err == nil {
		return value
	}

	return defaultVal
}

// Helper to read an environment variable into a bool or return default value
func getEnvAsBool(name string, defaultVal bool) bool {
	valStr := getEnv(name, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}

	return defaultVal
}

// Helper to read an environment variable into a string slice or return default value
func getEnvAsSlice(name string, defaultVal []string, sep string) []string {
	valStr := getEnv(name, "")

	if valStr == "" {
		return defaultVal
	}

	val := strings.Split(valStr, sep)

	return val
}

func getEnvAsDuration(name string, defaultVal time.Duration) time.Duration {
	valStr := getEnv(name, "")
	if valStr == "" {
		return defaultVal
	}
	if duration, err := time.ParseDuration(valStr); err == nil {
		return duration
	}
	return defaultVal
}
