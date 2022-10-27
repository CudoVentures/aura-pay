package services

import (
	"context"
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

	// for all others - check if there their time of sending + config.RetryTime is <= time.Now().Unix()
	for _, tx := range txToRetry {
		if time.Now().Unix() >= tx.TimeSent+int64(s.config.RBFTransactionRetryDelayInSeconds) {
			retryCountExceeded, err := s.retryCountExceeded(tx, storage, ctx)
			if err != nil {
				return err
			}
			if retryCountExceeded {
				log.Error().Msgf("Transaction has reached max RBF retry count and manual intervention will be needed. TxHash: {%s}; FarmId: {%s}", tx.TxHash, tx.FarmId)
				continue
				//TODO: Alert via grafana/prometheus to someone that can manually handle the problem
			}
			// try to bump the fee
			RBFtxHash, err := s.apiRequester.BumpFee(ctx, "todo", tx.TxHash)
			if err != nil {
				return err
			}
			// mark old tx as replaced
			err = storage.UpdateTransactionsStatus(ctx, []string{tx.TxHash}, types.TransactionReplaced)
			if err != nil {
				return err
			}
			// connect old tx hash with the new tx hash that replaced it
			err = storage.SaveRBFTransactionHistory(ctx, tx.TxHash, RBFtxHash, tx.FarmId)
			if err != nil {
				return err
			}
			// save the new tx hash as a new pending transaction and increment the retries
			err = storage.SaveTxHashWithStatus(ctx, RBFtxHash, types.TransactionPending, tx.FarmId, tx.RetryCount+1)
			if err != nil {
				return err
			}
		}
		continue
	}

	// if it is not ignore them and continue
	// else send an rbf transaction ( or bumpfee ) to try and push them faster
	// mark that you have tried to send 1 time
	// connect the old tx with the newly one..think about what happens with the old so you wont go into a loop
	// if you try 3 times and nothing happens - raise alert and ask for manual intervention
	// repeat again everything

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
