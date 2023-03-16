package services

import (
	"context"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
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

/*
	Checks the confirmation status of pending transactions,
	updates the status for those with confirmations,
	and retries transactions that have not been confirmed within the specified time frame.

	1. Retrieve unconfirmed transaction hashes from the storage with the TransactionPending status.

	2. Iterate through the unconfirmed transaction hashes and do the following:
		a. Create a chainhash.Hash object from the transaction hash string.
		b. Retrieve the verbose transaction information from the btcClient.
		c. If the transaction has confirmations countgreater than 0,
		   append the transaction hash to the txToConfirm slice.
		   Otherwise, append the transaction object to the txToRetry slice.

	3. Update the status of transactions that have confirmations to TransactionCompleted in the storage.
	4. Iterate through the transactions that need to be retried and do the following:
		a. Check if enough time has passed since the transaction was sent
		   based on the RBFTransactionRetryDelayInSeconds configuration value.
		b. If the delay requirement is met, call the retryTransaction() function
		   to attempt to resend the transaction with a higher fee.
*/
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
	err = storage.UpdateTransactionsStatus(ctx, txToConfirm, types.TransactionCompleted)
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

/*
	Retry a transaction by increasing its fee using the RBF (Replace-By-Fee) mechanism.
	It checks if the retry count has exceeded the maximum allowed number, and if not,
	it creates a new transaction with a higher fee and saves it in the storage.

	1. Check if the retry count has exceeded the maximum allowed number of retries for the transaction.
	   If it has, log an error message, send an email notification, and return nil.
	2. Load the wallet associated with the transaction.
	3. Use a defer statement to lock and unload the wallet when the function execution completes.
	   This ensures that the wallet is locked and unloaded even if an error occurs during the execution.
	4. Unlock the wallet for a duration of 60 seconds.
	5. Call BumpFee() to increase the transaction fee and create a new RBF (Replace-By-Fee) transaction.
	   Store the new transaction hash in newRBFtxHash.
	6. Save the new RBF transaction information in the storage,
	   marking the old transaction as TransactionReplaced
	   and the new transaction as TransactionPending.
	7. Increment the retry count by 1.
*/
func (s *RetryService) retryTransaction(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context, btcClient BtcClient) error {
	retryCountExceeded, err := s.retryCountExceeded(tx, storage, ctx)
	if err != nil {
		return err
	}
	if retryCountExceeded {
		message := fmt.Sprintf("transaction has reached max RBF retry count and manual intervention will be needed. TxHash: {%s}; Farm Name: {%s}", tx.TxHash, tx.FarmBtcWalletName)
		log.Error().Msg(message)
		err = s.helper.SendMail(message)
		if err != nil {
			return err
		}
		return nil
	}
	_, err = btcClient.LoadWallet(tx.FarmBtcWalletName)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Farm Wallet: {%s} loaded", tx.FarmBtcWalletName)

	defer func() {
		if err := btcClient.WalletLock(); err != nil {
			log.Error().Msgf("Failed to lock wallet %s: %s", tx.FarmBtcWalletName, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} locked", tx.FarmBtcWalletName)

		err = btcClient.UnloadWallet(&tx.FarmBtcWalletName)
		if err != nil {
			log.Error().Msgf("Failed to unload wallet %s: %s", tx.FarmBtcWalletName, err)
		}
		log.Debug().Msgf("Farm Wallet: {%s} unloaded", tx.FarmBtcWalletName)
	}()

	err = btcClient.WalletPassphrase(s.config.AuraPoolTestFarmWalletPassword, 60)
	if err != nil {
		return err
	}

	newRBFtxHash, err := s.apiRequester.BumpFee(ctx, tx.TxHash)
	if err != nil {
		return err
	}

	err = storage.SaveRBFTransactionInformation(ctx, tx.TxHash, types.TransactionReplaced, newRBFtxHash, types.TransactionPending, tx.FarmBtcWalletName, tx.RetryCount+1)

	return nil
}

/*
	Used to determine if a transaction has reached the maximum number of allowed retries
	for the RBF (Replace-By-Fee) mechanism. If the retry count has exceeded the limit,
	the transaction status is updated to TransactionFailed.

	1. Compare the RetryCount of the transaction (tx.RetryCount)
	   with the maximum allowed number of retries (s.config.RBFTransactionRetryMaxCount).
	2. If the retry count is greater than or equal to the maximum allowed number of retries:
		a. Update the transaction status to TransactionFailed in the storage.
		b. If an error occurs while updating the transaction status,
		   return true (indicating the retry count has exceeded) and the error.
		c. If the transaction status update is successful,
		   return true (indicating the retry count has exceeded) and nil for the error.
	3. If the retry count is less than the maximum allowed number of retries,
	   return false (indicating the retry count has not exceeded) and nil for the error.
*/
func (s *RetryService) retryCountExceeded(tx types.TransactionHashWithStatus, storage Storage, ctx context.Context) (bool, error) {
	if tx.RetryCount >= s.config.RBFTransactionRetryMaxCount {
		err := storage.UpdateTransactionsStatus(ctx, []string{tx.TxHash}, types.TransactionFailed)
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
