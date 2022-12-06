package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSumMintedHashPowerForAllCollectionsShouldReturnZeroWithEmptyCollections(t *testing.T) {
	require.Equal(t, float64(0), sumMintedHashPowerForAllCollections([]types.Collection{}))
}

func TestSumMintedHashPowerForAllCollections(t *testing.T) {
	expirationDate := time.Now().Unix() + 100
	collections := []types.Collection{
		{
			Nfts: []types.NFT{
				{
					DataJson: types.NFTDataJson{
						ExpirationDate: expirationDate,
						HashRateOwned:  0.01,
					},
				},
				{
					DataJson: types.NFTDataJson{
						ExpirationDate: expirationDate,
						HashRateOwned:  1001,
					},
				},
				{
					DataJson: types.NFTDataJson{
						HashRateOwned: 5,
					},
				},
			},
		},
		{
			Nfts: []types.NFT{
				{
					DataJson: types.NFTDataJson{
						ExpirationDate: expirationDate,
						HashRateOwned:  2,
					},
				},
				{
					DataJson: types.NFTDataJson{
						ExpirationDate: expirationDate,
						HashRateOwned:  666,
					},
				},
			},
		},
	}
	require.Equal(t, float64(1669.01), sumMintedHashPowerForAllCollections(collections))
}

func TestCalculatePercentShouldReturnZeroIfInvalidHashingPowerProvided(t *testing.T) {
	require.Equal(t, btcutil.Amount(0), calculatePercent(-1, -1, btcutil.Amount(10000000)))
	require.Equal(t, btcutil.Amount(0), calculatePercent(10000, -1, btcutil.Amount(10000000)))
	require.Equal(t, btcutil.Amount(0), calculatePercent(-1, 10000, btcutil.Amount(10000000)))
	require.Equal(t, btcutil.Amount(0), calculatePercent(0, 0, btcutil.Amount(10000000)))
	require.Equal(t, btcutil.Amount(0), calculatePercent(10000, 0, btcutil.Amount(10000000)))
	require.Equal(t, btcutil.Amount(0), calculatePercent(0, 10000, btcutil.Amount(10000000)))
}

func TestCalculatePercentShouldReturnZeroIfRewardIsZero(t *testing.T) {
	require.Equal(t, btcutil.Amount(0), calculatePercent(10000, 10000, btcutil.Amount(0)))
}

func TestCalculatePercent(t *testing.T) {
	require.Equal(t, btcutil.Amount(10), calculatePercent(10000, 1000, 100))
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldReturnErrorIfInvalidPeriod(t *testing.T) {
	s := NewPayService(nil, nil, nil, nil)
	_, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "", "", 1000, 100, nil, "", "", 0)
	require.Equal(t, errors.New("invalid period, start (1000) end (100)"), err)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldReturnHundredPercentRewardToCurrentOwnerIfNoTransferEvents(t *testing.T) {
	apiRequester := &mockAPIRequester{}
	payoutAddr := "payoutaddr"
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "addr1", "BTC", "1", "testdenom").Return(payoutAddr, nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.NoError(t, err)
	require.Equal(t, map[string]float64{payoutAddr: float64(100)}, percents)

	expectedStatistics := types.NFTStatistics{
		NFTOwnersForPeriod: []types.NFTOwnerInformation{
			{
				TimeOwnedFrom:      periodStart,
				TimeOwnedTo:        periodEnd,
				TotalTimeOwned:     periodEnd - periodStart,
				PayoutAddress:      payoutAddr,
				PercentOfTimeOwned: 100,
				Owner:              "addr1",
			},
		},
	}
	require.Equal(t, expectedStatistics, statistics)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldWorkWithSingleTransferEvent(t *testing.T) {
	history := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_owner_2",
						"from": "nft_owner_1",
						"timestamp": 64
					}
				]
			}
		}
	}
	`

	var nftTransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC", "1", "testdenom").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "testdenom").Return("nft_owner_2_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_owner_1_payout_addr": float64(63.64),
		"nft_owner_2_payout_addr": float64(36.36),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      periodStart,
			TimeOwnedTo:        64,
			TotalTimeOwned:     63,
			PercentOfTimeOwned: 63.64,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
		},
		{
			TimeOwnedFrom:      64,
			TimeOwnedTo:        periodEnd,
			TotalTimeOwned:     36,
			PercentOfTimeOwned: 36.36,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
		},
	}

	require.Equal(t, expectedNFTOwnersForPeriod, statistics.NFTOwnersForPeriod)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldWorkWithMultipleTransferEventsStartingFromMint(t *testing.T) {
	history := `
		{
			"data": {
				"action_nft_transfer_events": {
					"events": [
						{
							"to": "nft_minter",
							"from": "0x0",
							"timestamp": 1
						},
						{
							"to": "nft_owner_1",
							"from": "nft_minter",
							"timestamp": 10
						},
						{
							"to": "nft_owner_2",
							"from": "nft_owner_1",
							"timestamp": 13
						},
						{
							"to": "nft_owner_3",
							"from": "nft_owner_2",
							"timestamp": 50
						},
						{
							"to": "nft_owner_4",
							"from": "nft_owner_3",
							"timestamp": 80
						},
						{
							"to": "nft_owner_5",
							"from": "nft_owner_4",
							"timestamp": 95
						}
					]
				}
			}
		}
	`

	var nftTransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_minter", "BTC", "1", "testdenom").Return("nft_minter_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC", "1", "testdenom").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "testdenom").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_3", "BTC", "1", "testdenom").Return("nft_owner_3_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_4", "BTC", "1", "testdenom").Return("nft_owner_4_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_5", "BTC", "1", "testdenom").Return("nft_owner_5_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_minter_payout_addr":  float64(9.09),
		"nft_owner_1_payout_addr": float64(3.03),
		"nft_owner_2_payout_addr": float64(37.37),
		"nft_owner_3_payout_addr": float64(30.30),
		"nft_owner_4_payout_addr": float64(15.15),
		"nft_owner_5_payout_addr": float64(5.05),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.09,
			PayoutAddress:      "nft_minter_payout_addr",
			Owner:              "nft_minter",
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.03,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.37,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.30,
			PayoutAddress:      "nft_owner_3_payout_addr",
			Owner:              "nft_owner_3",
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.15,
			PayoutAddress:      "nft_owner_4_payout_addr",
			Owner:              "nft_owner_4",
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.05,
			PayoutAddress:      "nft_owner_5_payout_addr",
			Owner:              "nft_owner_5",
		},
	}

	require.Equal(t, expectedNFTOwnersForPeriod, statistics.NFTOwnersForPeriod)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldWorkWithMultipleTransferEventsWithoutMintEvent(t *testing.T) {
	history := `
		{
			"data": {
				"action_nft_transfer_events": {
					"events": [
						{
							"to": "nft_owner_1",
							"from": "nft_minter",
							"timestamp": 10
						},
						{
							"to": "nft_owner_2",
							"from": "nft_owner_1",
							"timestamp": 13
						},
						{
							"to": "nft_owner_3",
							"from": "nft_owner_2",
							"timestamp": 50
						},
						{
							"to": "nft_owner_4",
							"from": "nft_owner_3",
							"timestamp": 80
						},
						{
							"to": "nft_owner_5",
							"from": "nft_owner_4",
							"timestamp": 95
						}
					]
				}
			}
		}
	`

	var nftTransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_minter", "BTC", "1", "testdenom").Return("nft_minter_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC", "1", "testdenom").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC", "1", "testdenom").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_3", "BTC", "1", "testdenom").Return("nft_owner_3_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_4", "BTC", "1", "testdenom").Return("nft_owner_4_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_5", "BTC", "1", "testdenom").Return("nft_owner_5_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_minter_payout_addr":  float64(9.09),
		"nft_owner_1_payout_addr": float64(3.03),
		"nft_owner_2_payout_addr": float64(37.37),
		"nft_owner_3_payout_addr": float64(30.30),
		"nft_owner_4_payout_addr": float64(15.15),
		"nft_owner_5_payout_addr": float64(5.05),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.09,
			PayoutAddress:      "nft_minter_payout_addr",
			Owner:              "nft_minter",
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.03,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.37,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.30,
			PayoutAddress:      "nft_owner_3_payout_addr",
			Owner:              "nft_owner_3",
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.15,
			PayoutAddress:      "nft_owner_4_payout_addr",
			Owner:              "nft_owner_4",
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.05,
			PayoutAddress:      "nft_owner_5_payout_addr",
			Owner:              "nft_owner_5",
		},
	}

	require.Equal(t, expectedNFTOwnersForPeriod, statistics.NFTOwnersForPeriod)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldFailIfGetPayoutAddressFromNodeFails(t *testing.T) {
	apiRequester := &mockAPIRequester{}
	failErr := errors.New("failed to get payout address from node")
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", failErr)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	_, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.Equal(t, failErr, err)

	history := `
	{
		"data": {
			"action_nft_transfer_events": {
				"events": [
					{
						"to": "nft_owner_2",
						"from": "nft_owner_1",
						"timestamp": 64
					}
				]
			}
		}
	}
	`

	var nftTransferHistory types.NftTransferHistory
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	_, err = s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", 0)
	require.Equal(t, failErr, err)
}

type mockAPIRequester struct {
	mock.Mock
}

func (mar *mockAPIRequester) GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error) {
	args := mar.Called(ctx, collectionDenomId, nftId, fromTimestamp)
	return args.Get(0).(types.NftTransferHistory), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error) {
	args := mar.Called(ctx, farmName, sinceTimestamp)
	return args.Get(0).(float64), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmCollectionsFromHasura(ctx context.Context, farmId string) (types.CollectionData, error) {
	args := mar.Called(ctx, farmId)
	return args.Get(0).(types.CollectionData), args.Error(1)
}

func (mar *mockAPIRequester) GetFarms(ctx context.Context) ([]types.Farm, error) {
	args := mar.Called(ctx)
	return args.Get(0).([]types.Farm), args.Error(1)
}

func (mar *mockAPIRequester) VerifyCollection(ctx context.Context, denomId string) (bool, error) {
	args := mar.Called(ctx, denomId)
	return args.Bool(0), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmCollectionsWithNFTs(ctx context.Context, denomIds []string) ([]types.Collection, error) {
	args := mar.Called(ctx, denomIds)
	return args.Get(0).([]types.Collection), args.Error(1)
}

func (mar *mockAPIRequester) GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network, tokenId, denomId string) (string, error) {
	args := mar.Called(ctx, cudosAddress, network, tokenId, denomId)
	return args.String(0), args.Error(1)
}

func (mar *mockAPIRequester) SendMany(ctx context.Context, destinationAddressesWithAmount map[string]float64) (string, error) {
	args := mar.Called(ctx, destinationAddressesWithAmount)
	return args.String(0), args.Error(1)
}

func (mar *mockAPIRequester) BumpFee(ctx context.Context, txId string) (string, error) {
	args := mar.Called(ctx, txId)
	return args.String(0), args.Error(1)
}
