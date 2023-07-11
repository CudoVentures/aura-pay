package services

import (
	"context"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/rs/zerolog/log"
)

func (s *RetryService) getWalletTransaction(ctx context.Context, btcClient BtcClient, tx types.TransactionHashWithStatus) (*types.BtcWalletTransaction, error) {
	loaded, err := s.loadWallet(btcClient, tx.FarmBtcWalletName)
	if err != nil || !loaded {
		return nil, err
	}
	defer unloadWallet(btcClient, tx.FarmBtcWalletName)

	decodedTx, err := s.apiRequester.GetWalletTransaction(ctx, tx.TxHash)
	if err != nil {
		return nil, err
	}

	return decodedTx, nil
}

// loadWallet attempts to load the specified Bitcoin wallet using the given BTC client.
// If the wallet fails to load for 15 consecutive attempts, the function returns an error.
// The function returns a boolean to indicate whether the wallet was successfully loaded or not.
// If the wallet is loaded successfully - nullate the fail counter for the wallet.
// Returns:
// - bool: True if the wallet was successfully loaded, false otherwise.
// - error: An error indicating the reason for the wallet load failure, if any.
func (s *RetryService) loadWallet(btcClient BtcClient, farmName string) (bool, error) {
	_, err := btcClient.LoadWallet(farmName)
	if err != nil {
		s.btcWalletOpenFailsPerFarm[farmName]++
		if s.btcWalletOpenFailsPerFarm[farmName] >= 15 {
			s.btcWalletOpenFailsPerFarm[farmName] = 0
			return false, fmt.Errorf("failed to load wallet %s for 15 times", farmName)
		}

		log.Warn().Msgf("Failed to load wallet %s for %d consecutive times: %s", farmName, s.btcWalletOpenFailsPerFarm[farmName], err)
		return false, nil
	}

	s.btcWalletOpenFailsPerFarm[farmName] = 0
	log.Debug().Msgf("Farm Wallet: {%s} loaded", farmName)

	return true, nil
}
