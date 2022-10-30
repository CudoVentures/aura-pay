package services

import (
	"context"
	"fmt"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/rs/zerolog/log"
)

func NewRetryService(config *infrastructure.Config, apiRequester ApiRequester, helper Helper, btcNetworkParams *types.BtcNetworkParams) *retryService {
	return &retryService{
		config:           config,
		helper:           helper,
		btcNetworkParams: btcNetworkParams,
		apiRequester:     apiRequester,
	}
}

func (s *retryService) Execute(ctx context.Context, btcClient BtcClient, storage Storage) error {
	unconfirmedTransactionHashes, err := storage.GetTxHashesByStatus(ctx, types.TransactionPending)
	if err != nil {
		return err
	}

	txToConfirm := []string{}
	txToRetry := []types.TransactionHashWithStatus{}

	for _, tx := range unconfirmedTransactionHashes {
		txHash, err := chainhash.NewHashFromStr(tx.TxHash)
		if err != nil {
			return err
		}

		decodedTx, err := btcClient.GetRawTransactionVerbose(txHash)
		if err != nil {
			return err
		}

		if decodedTx.Confirmations > 0 {
			txToConfirm = append(txToConfirm, tx.TxHash)
		} else {
			txToRetry = append(txToRetry, tx)
		}
	}

	// all the ones that were included in atleast 1 block - mark them as completed
	err = storage.UpdateTransactionsStatus(ctx, txToConfirm, types.TransactionCompleted)
	if err != nil {
		return err
	}

	// for all others - check if enough time has passed
	for _, tx := range txToRetry {
		if time.Now().Unix() >= tx.TimeSent+int64(s.config.RBFTransactionRetryDelayInSeconds) {
			err := s.retryTransaction(tx, storage, ctx, btcClient)
			if err != nil {

				return err
			}
		}
		continue
	}
	return nil
}

func (s *retryService) retryTransaction(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context, btcClient BtcClient) error {
	retryCountExceeded, err := s.retryCountExceeded(tx, storage, ctx)
	if err != nil {
		return err
	}
	if retryCountExceeded {
		//TODO: Alert via grafana/prometheus to someone that can manually handle the problem
		return fmt.Errorf("transaction has reached max RBF retry count and manual intervention will be needed. TxHash: {%s}; FarmId: {%s}", tx.TxHash, tx.FarmId)
	}
	_, err = btcClient.LoadWallet(tx.FarmId)
	if err != nil {
		return err
	}
	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return err
	}

	RBFtxHash, err := s.apiRequester.BumpFee(ctx, tx.FarmId, tx.TxHash)
	if err != nil {
		return err
	}

	defer func() {
		if err := btcClient.WalletLock(); err != nil {
			log.Error().Msgf("Failed to lock wallet %s: %s", tx.FarmId, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} locked", tx.FarmId)

		err = btcClient.UnloadWallet(&tx.FarmId)
		if err != nil {
			log.Error().Msgf("Failed to unload wallet %s: %s", tx.FarmId, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} unloaded", tx.FarmId)
	}()

	// update old tx status to replaced
	err = storage.UpdateTransactionsStatus(ctx, []string{tx.TxHash}, types.TransactionReplaced)
	if err != nil {
		return err
	}

	// link replaced transaction with the tx that replaced it
	err = storage.SaveRBFTransactionHistory(ctx, tx.TxHash, RBFtxHash, tx.FarmId)
	if err != nil {
		return err
	}

	// save the new tx with status pending, new timestamp, and retryCount of old one + 1
	err = storage.SaveTxHashWithStatus(ctx, RBFtxHash, types.TransactionPending, tx.FarmId, tx.RetryCount+1)
	if err != nil {
		return err
	}
	return nil
}

func (s *retryService) retryCountExceeded(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context) (bool, error) {
	if tx.RetryCount >= s.config.RBFTransactionRetryDelayInSeconds {
		err := storage.UpdateTransactionsStatus(ctx, []string{tx.TxHash}, types.TransactionFailed)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

type retryService struct {
	config           *infrastructure.Config
	helper           Helper
	btcNetworkParams *types.BtcNetworkParams
	apiRequester     ApiRequester
}
