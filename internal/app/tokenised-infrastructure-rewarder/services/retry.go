package services

import (
	"context"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/rs/zerolog/log"
)

func NewRetryService(config *infrastructure.Config, apiRequester ApiRequester, helper Helper, btcNetworkParams *types.BtcNetworkParams) *RetryService {
	return &RetryService{
		config:           config,
		helper:           helper,
		btcNetworkParams: btcNetworkParams,
		apiRequester:     apiRequester,
	}
}

func (s *RetryService) Execute(ctx context.Context, btcClient BtcClient, storage Storage) error {
	unconfirmedTransactionHashes, err := storage.GetTxHashesByStatus(ctx, types.TransactionPending)
	if err != nil {
		return err
	}

	var txToConfirm []string
	var txToRetry []types.TransactionHashWithStatus

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

	// all the ones that were included in at least 1 block - mark them as completed
	err = storage.UpdateTransactionsStatus(ctx, nil, txToConfirm, types.TransactionCompleted)
	if err != nil {
		return err
	}

	// for all others - check if enough time has passed; if so - send bump fee tx
	for _, tx := range txToRetry {
		if s.helper.Unix() >= tx.TimeSent+int64(s.config.RBFTransactionRetryDelayInSeconds) {
			err := s.retryTransaction(tx, storage, ctx, btcClient)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *RetryService) retryTransaction(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context, btcClient BtcClient) error {
	retryCountExceeded, err := s.retryCountExceeded(tx, storage, ctx)
	if err != nil {
		return err
	}
	if retryCountExceeded {
		//TODO: Alert via grafana/prometheus to someone that can manually handle the problem
		log.Error().Msgf("transaction has reached max RBF retry count and manual intervention will be needed. TxHash: {%s}; FarmId: {%s}", tx.TxHash, tx.FarmSubAccountName)
		return nil
	}
	_, err = btcClient.LoadWallet(tx.FarmSubAccountName)
	if err != nil {
		log.Debug().Msgf("Farm Wallet was not loaded due to error: walletName {%s}, error: {%s}", tx.FarmSubAccountName, err)
		serr, ok := err.(*btcjson.RPCError)
		if ok {
			walletSuccessfullyReloaded, newErr := retryWalletLoad(serr, btcClient, tx.FarmSubAccountName, s.helper)
			if !walletSuccessfullyReloaded {
				return newErr
			}
		} else {
			return err
		}
	}
	log.Debug().Msgf("Farm Wallet: {%s} loaded", tx.FarmSubAccountName)

	defer func() {
		if err := btcClient.WalletLock(); err != nil {
			log.Error().Msgf("Failed to lock wallet %s: %s", tx.FarmSubAccountName, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} locked", tx.FarmSubAccountName)

		err = btcClient.UnloadWallet(&tx.FarmSubAccountName)
		if err != nil {
			log.Error().Msgf("Failed to unload wallet %s: %s", tx.FarmSubAccountName, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} unloaded", tx.FarmSubAccountName)
	}()

	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return err
	}

	newRBFtxHash, err := s.apiRequester.BumpFee(ctx, tx.TxHash)
	if err != nil {
		return err
	}

	err = storage.SaveRBFTransactionInformation(ctx, tx.TxHash, types.TransactionReplaced, newRBFtxHash, types.TransactionPending, tx.FarmSubAccountName, tx.RetryCount+1)

	return nil
}

func (s *RetryService) retryCountExceeded(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context) (bool, error) {
	if tx.RetryCount >= s.config.RBFTransactionRetryMaxCount {
		err := storage.UpdateTransactionsStatus(ctx, nil, []string{tx.TxHash}, types.TransactionFailed)
		if err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

type RetryService struct {
	config           *infrastructure.Config
	helper           Helper
	btcNetworkParams *types.BtcNetworkParams
	apiRequester     ApiRequester
}
