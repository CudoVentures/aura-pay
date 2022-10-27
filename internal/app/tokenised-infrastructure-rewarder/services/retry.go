package services

import (
	"context"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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
	// 1. How do we know if a tx is stuck and needs to be replaced?
	//- By checking if it has been confirmed in a block after certain amount of time (180 mins for example)

	// get all txs from DB with status unconfirmed
	unconfirmedTransactionHashes, err := storage.GetTxHashesByStatus(ctx, types.TransactionCompleted)
	if err != nil {
		return err
	}

	txToConfirm := []string{}
	txToPossiblyRetry := []types.TransactionHashWithStatus{}
	// for each one of them go to the BTC node over RPC and check if they have been included in a block
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
			txToPossiblyRetry = append(txToPossiblyRetry, tx)
		}
	}

	// all the ones that were included in atleast 1 block - mark them as completed
	err = storage.UpdateTransactionsStatus(ctx, txToConfirm, types.TransactionCompleted)

	if err != nil {
		return err
	}

	// for all others - check if there their time of sending + config.RetryTime is <= time.Now().Unix()

	// if it is not ignore them and continue
	// else send an rbf transaction ( or bumpfee ) to try and push them faster
	// mark that you have tried to send 1 time
	// connect the old tx with the newly one..think about what happens with the old so you wont go into a loop
	// if you try 3 times and nothing happens - raise alert and ask for manual intervention
	// repeat again everything

	return nil
}

type retryService struct {
	config           *infrastructure.Config
	helper           Helper
	btcNetworkParams *types.BtcNetworkParams
	apiRequester     ApiRequester
}
