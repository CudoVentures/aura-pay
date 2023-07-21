// package main

// import (
// 	"context"
// 	"encoding/json"
// 	"fmt"

// 	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
// 	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
// 	"github.com/joho/godotenv"
// 	"github.com/rs/zerolog/log"
// )

// var farmName = "valtest4"
// var sendAddresses = map[string]float64{
// 	"n2Wgrv3LAaaWzF44B7DdumkcL2GnvaCmjP": 0.00001,
// }

// func main() {
// 	if err := godotenv.Load(".env"); err != nil {
// 		log.Error().Msgf("No .env file found: %s", err)
// 		return
// 	}

// 	config := infrastructure.NewConfig()
// 	provider := infrastructure.NewProvider(config)
// 	requestClient := requesters.NewRequester(config)
// 	ctx := context.Background()

// 	btcClient, err := provider.InitBtcRpcClient()
// 	if err != nil {
// 		log.Error().Msgf("Error initiating btc rpc client: %s", err)
// 		return
// 	}

// 	rawMessage, err := btcClient.RawRequest("listwallets", []json.RawMessage{})
// 	if err != nil {
// 		log.Error().Msgf("Error listing wallets: %s", err)
// 		return
// 	}

// 	loadedWalletsNames := []string{}
// 	err = json.Unmarshal(rawMessage, &loadedWalletsNames)
// 	if err != nil {
// 		log.Error().Msgf("Error unmarshalling wallets: %s", err)
// 		return
// 	}

// 	if len(loadedWalletsNames) > 0 {
// 		log.Debug().Msgf("Loaded wallets found. Unloading...")
// 		for _, loadedWalletName := range loadedWalletsNames {
// 			if err := btcClient.UnloadWallet(&loadedWalletName); err != nil {
// 				log.Error().Msgf("Failed to unload wallet %s: %s", loadedWalletName, err)
// 				return
// 			}
// 		}
// 	}

// 	_, err = btcClient.LoadWallet(farmName)
// 	if err != nil {
// 		log.Error().Msgf("Error loading wallet: %s", err)
// 		result, err := btcClient.CreateWallet(farmName)
// 		if err != nil {
// 			log.Error().Msgf("Error creating wallet: %s", err)
// 			return
// 		}
// 		fmt.Println("Wallet created: ", result)
// 		address, err := btcClient.GetNewAddress(farmName)
// 		if err != nil {
// 			log.Error().Msgf("Error receiving btc: %s", err)
// 			return
// 		}
// 		fmt.Println("Address: ", address)
// 	}

// 	defer func() {
// 		if err := btcClient.UnloadWallet(&farmName); err != nil {
// 			log.Error().Msgf("Failed to unload wallet %s: %s", farmName, err)
// 			return
// 		}

// 		log.Debug().Msgf("Farm Wallet: {%s} unloaded", farmName)
// 	}()

// 	txHash, err := requestClient.SendMany(ctx, sendAddresses)
// 	if err != nil {
// 		log.Error().Msgf("Error sending btc: %s", err)
// 		return
// 	}
// 	fmt.Println(txHash)

// 	// err = btcClient.WalletPassphrase(config.AuraPoolTestFarmWalletPassword, 60)
// 	// if err != nil {
// 	// 	log.Error().Msgf("Error unlocking wallet: %s", err)
// 	// 	return
// 	// }

// 	bumpTxhash, err := requestClient.BumpFee(ctx, txHash)
// 	if err != nil {
// 		log.Error().Msgf("Error bumping fee: %s", err)
// 		return
// 	}

// 	fmt.Println(bumpTxhash)
// }
