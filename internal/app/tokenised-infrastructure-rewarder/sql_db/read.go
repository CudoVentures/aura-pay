package sql_db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
)

func (sdb *SqlDB) GetPayoutTimesForNFT(ctx context.Context, collectionDenomId string, nftId string) ([]types.NFTStatistics, error) {
	var payoutTimes []types.NFTStatisticsRepo
	if err := sdb.SelectContext(ctx, &payoutTimes, selectNFTPayoutHistory, collectionDenomId, nftId); err != nil {
		return nil, err
	}

	var payoutTimesParsed []types.NFTStatistics

	for _, payoutTimeRepo := range payoutTimes {
		var ownerInfos []types.NFTOwnerInformation

		for _, ownerInfoRepo := range payoutTimeRepo.NFTOwnersForPeriod {
			parsedInfo := types.NFTOwnerInformation{
				TimeOwnedFrom:      ownerInfoRepo.TimeOwnedFrom,
				TimeOwnedTo:        ownerInfoRepo.TimeOwnedTo,
				TotalTimeOwned:     ownerInfoRepo.TotalTimeOwned,
				PercentOfTimeOwned: ownerInfoRepo.PercentOfTimeOwned,
				Owner:              ownerInfoRepo.Owner,
				PayoutAddress:      ownerInfoRepo.PayoutAddress,
				Reward:             btcutil.Amount(ownerInfoRepo.Reward),
				CreatedAt:          ownerInfoRepo.CreatedAt,
				UpdatedAt:          ownerInfoRepo.UpdatedAt,
			}

			ownerInfos = append(ownerInfos, parsedInfo)
		}

		payoutTimeParsed := types.NFTStatistics{
			Id:                       payoutTimeRepo.Id,
			TokenId:                  payoutTimeRepo.TokenId,
			DenomId:                  payoutTimeRepo.DenomId,
			PayoutPeriodStart:        payoutTimeRepo.PayoutPeriodStart,
			PayoutPeriodEnd:          payoutTimeRepo.PayoutPeriodEnd,
			Reward:                   btcutil.Amount(payoutTimeRepo.Reward),
			MaintenanceFee:           btcutil.Amount(payoutTimeRepo.MaintenanceFee),
			CUDOPartOfMaintenanceFee: btcutil.Amount(payoutTimeRepo.CUDOPartOfMaintenanceFee),
			NFTOwnersForPeriod:       ownerInfos,
			TxHash:                   payoutTimeRepo.TxHash,
			CreatedAt:                payoutTimeRepo.CreatedAt,
			UpdatedAt:                payoutTimeRepo.UpdatedAt,
		}

		payoutTimesParsed = append(payoutTimesParsed, payoutTimeParsed)
	}

	return payoutTimesParsed, nil
}

func (sdb *SqlDB) GetApprovedFarms(ctx context.Context) ([]types.Farm, error) {
	farms := []types.Farm{}
	if err := sdb.SelectContext(ctx, &farms, selectApprovedFarms); err != nil {
		return nil, err
	}
	return farms, nil
}

func (sdb *SqlDB) GetTxHashesByStatus(ctx context.Context, status string) ([]types.TransactionHashWithStatus, error) {
	txHashesWithStatus := []types.TransactionHashWithStatus{}
	if err := sdb.SelectContext(ctx, &txHashesWithStatus, selectTxHashStatus, status); err != nil {
		return nil, err
	}
	return txHashesWithStatus, nil
}

func (sdb *SqlDB) GetCurrentAcummulatedAmountForAddress(ctx context.Context, address string, farmId int64) (float64, error) {
	var result []types.AddressThresholdAmountByFarm
	if err := sdb.SelectContext(ctx, &result, selectThresholdByAddress, address, farmId); err != nil {
		return 0, err
	}

	if len(result) > 1 {
		return 0, fmt.Errorf("more then one threshold address for farm! Address: %s, FarmId: %d", address, farmId)
	} else if len(result) == 0 {
		return 0, sql.ErrNoRows
	}

	return result[0].AmountBTC, nil
}

func (sdb *SqlDB) GetUTXOTransaction(ctx context.Context, txHash string) (types.UTXOTransaction, error) {
	var result []types.UTXOTransaction
	if err := sdb.SelectContext(ctx, &result, selectUTXOById, txHash); err != nil {
		return types.UTXOTransaction{}, err
	}

	if len(result) > 1 {
		return types.UTXOTransaction{}, fmt.Errorf("tx_hash with %s is duplicated in table utxo_transactions", txHash)
	} else if len(result) == 0 {
		return types.UTXOTransaction{}, sql.ErrNoRows
	}

	return result[0], nil
}

func (sdb *SqlDB) GetFarmAuraPoolCollections(ctx context.Context, farmId int64) ([]types.AuraPoolCollection, error) {
	collections := []types.AuraPoolCollection{}
	if err := sdb.SelectContext(ctx, &collections, selectFarmCollections, farmId); err != nil {
		return nil, err
	}
	return collections, nil
}

const selectNFTPayoutHistory = `SELECT * FROM statistics_nft_payout_history WHERE denom_id=$1 and token_id=$2 ORDER BY payout_period_end ASC`
const selectTxHashStatus = `SELECT * FROM statistics_tx_hash_status WHERE status=$1 ORDER BY time_sent ASC`
const selectApprovedFarms = `SELECT id, name, description, sub_account_name, total_farm_hashrate, address_for_receiving_rewards_from_pool, leftover_reward_payout_address, maintenance_fee_payout_address, maintenance_fee_in_btc FROM farms WHERE status='approved'`
const selectThresholdByAddress = `SELECT * FROM threshold_amounts WHERE btc_address=$1 AND farm_id=$2`
const selectUTXOById = `SELECT * FROM utxo_transactions WHERE tx_hash=$1`
const selectFarmCollections = `SELECT id, denom_id, hashing_power FROM collections WHERE farm_id=$1`
