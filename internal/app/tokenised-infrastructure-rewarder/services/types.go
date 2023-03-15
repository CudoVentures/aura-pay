package services

import (
	"context"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/jmoiron/sqlx"
	"github.com/shopspring/decimal"
)

type CollectionProcessResult struct {
	CUDOMaintenanceFeeBtcDecimal  decimal.Decimal
	FarmMaintenanceFeeBtcDecimal  decimal.Decimal
	NftRewardsAfterFeesBtcDecimal decimal.Decimal
	CollectionPaymentAllocation   types.CollectionPaymentAllocation
	NftStatistics                 []types.NFTStatistics
}

type NftProcessResult struct {
	CudoPartOfMaintenanceFeeBtcDecimal         decimal.Decimal
	MaintenanceFeeBtcDecimal                   decimal.Decimal
	RewardForNftAfterFeeBtcDecimal             decimal.Decimal
	AllNftOwnersForTimePeriodWithRewardPercent map[string]float64
	NftOwnersForPeriod                         []types.NFTOwnerInformation
	NftPeriodStart                             int64
	NftPeriodEnd                               int64
}

type PayService struct {
	config                    *infrastructure.Config
	helper                    Helper
	btcNetworkParams          *types.BtcNetworkParams
	apiRequester              ApiRequester
	lastEmailTimestamp        int64
	btcWalletOpenFailsPerFarm map[string]int
}

type ApiRequester interface {
	GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error)

	GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error)

	GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error)

	GetFarmStartTime(ctx context.Context, farmName string) (int64, error)

	GetFarmCollectionsFromHasura(ctx context.Context, farmId int64) (types.CollectionData, error)

	VerifyCollection(ctx context.Context, denomId string) (bool, error)

	GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error)

	SendMany(ctx context.Context, destinationAddressesWithAmount map[string]float64) (string, error)

	BumpFee(ctx context.Context, txId string) (string, error)
}

type Provider interface {
	InitBtcRpcClient() (*rpcclient.Client, error)
	InitDBConnection() (*sqlx.DB, error)
}

type BtcClient interface {
	LoadWallet(walletName string) (*btcjson.LoadWalletResult, error)

	UnloadWallet(walletName *string) error

	WalletPassphrase(passphrase string, timeoutSecs int64) error

	WalletLock() error

	GetRawTransactionVerbose(txHash *chainhash.Hash) (*btcjson.TxRawResult, error)

	ListUnspent() ([]btcjson.ListUnspentResult, error)
}

type Storage interface {
	GetApprovedFarms(ctx context.Context) ([]types.Farm, error)

	GetPayoutTimesForNFT(ctx context.Context, collectionDenomId, nftId string) ([]types.NFTStatistics, error)

	SaveStatistics(ctx context.Context, receivedRewardForFarmBtcDecimal decimal.Decimal, collectionPaymentAllocationsStatistics []types.CollectionPaymentAllocation, destinationAddressesWithAmount map[string]types.AmountInfo, statistics []types.NFTStatistics, txHash string, farmId int64, farmSubAccountName string) error

	GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error)

	UpdateTransactionsStatus(ctx context.Context, txHashesToMarkCompleted []string, status string) error

	SaveTxHashWithStatus(ctx context.Context, txHash, status, farmSubAccountName string, retryCount int) error

	SaveRBFTransactionInformation(ctx context.Context, oldTxHash, oldTxStatus, newRBFTxHash, newRBFTXStatus, farmSubAccountName string, retryCount int) error

	GetUTXOTransaction(ctx context.Context, txId string) (types.UTXOTransaction, error)

	GetLastUTXOTransactionByFarmId(ctx context.Context, farmId int64) (types.UTXOTransaction, error)

	GetCurrentAcummulatedAmountForAddress(ctx context.Context, key string, farmId int64) (decimal.Decimal, error)

	UpdateThresholdStatus(ctx context.Context, processedTransactions string, paymentTimestamp int64, addressesWithThresholdToUpdateBtcDecimal map[string]decimal.Decimal, farmId int64) error

	SetInitialAccumulatedAmountForAddress(ctx context.Context, address string, farmId int64, amount int) error

	GetFarmAuraPoolCollections(ctx context.Context, farmId int64) ([]types.AuraPoolCollection, error)
}

type Helper interface {
	DaysIn(m time.Month, year int) int
	Unix() int64
	Date() (year int, month time.Month, day int)
	SendMail(message string) error
}
