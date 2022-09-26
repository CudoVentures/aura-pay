package infrastructure

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HasuraURL              string
	NodeRestUrl            string
	BitcoinNodeUrl         string
	BitcoinNodePort        string
	BitcoinNodeUserName    string
	BitcoinNodePassword    string
	FoundryPoolAPIBaseURL  string
	FoundryPoolAPIKey      string
	DbDriverName           string
	DbUserNameWithPassword string
	DbName                 string
	HasuraActionsURL       string
	IsTesting              bool
	AuraPoolBackEndUrl     string
}

// NewConfig New returns a new Config struct
func NewConfig() *Config {
	return &Config{
		HasuraURL:              getEnv("HASURA_URL", ""),
		NodeRestUrl:            getEnv("NODE_REST_URL", ""),
		BitcoinNodeUrl:         getEnv("BITCOIN_NODE_URL", ""),
		BitcoinNodePort:        getEnv("BITCOIN_NODE_PORT", ""),
		BitcoinNodeUserName:    getEnv("BITCOIN_NODE_USER_NAME", ""),
		BitcoinNodePassword:    getEnv("BITCOIN_NODE_PASSWORD", ""),
		FoundryPoolAPIBaseURL:  getEnv("FOUNDRY_POOL_API_BASE_URL", ""),
		FoundryPoolAPIKey:      getEnv("FOUNDRY_POOL_API_KEY", ""),
		DbDriverName:           getEnv("DB_DRIVER_NAME", ""),
		DbUserNameWithPassword: getEnv("DB_USER_NAME_WITH_PASSWORD", ""),
		DbName:                 getEnv("DB_NAME", ""),
		HasuraActionsURL:       getEnv("HASURA_ACTIONS_URL", ""),
		IsTesting:              getEnvAsBool("IS_TESTING", true),
		AuraPoolBackEndUrl:     getEnv("AURA_POOL_BACKEND_URL", ""),
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
