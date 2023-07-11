package services

import "github.com/rs/zerolog/log"

// unloadWallet attempts to unload the specified Bitcoin wallet (farmName) using the given BTC client.
// If the wallet fails to unload, an error message is logged. If the wallet is successfully unloaded, a debug
// message is logged.
func unloadWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.UnloadWallet(&farmName); err != nil {
		log.Error().Msgf("Failed to unload wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} unloaded", farmName)
}

// lockWallet attempts to lock the specified Bitcoin wallet (farmName) using the given BTC client.
// If the wallet fails to lock, an error message is logged. If the wallet is successfully locked, a debug
// message is logged.
func lockWallet(btcClient BtcClient, farmName string) {
	if err := btcClient.WalletLock(); err != nil {
		log.Error().Msgf("Failed to lock wallet %s: %s", farmName, err)
		return
	}

	log.Debug().Msgf("Farm Wallet: {%s} locked", farmName)
}
