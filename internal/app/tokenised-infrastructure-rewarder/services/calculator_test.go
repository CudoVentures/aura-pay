package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSumMintedHashPowerForAllCollectionsShouldReturnZeroWithEmptyCollections(t *testing.T) {
	result := sumMintedHashPowerForAllCollections([]types.Collection{})
	require.Equal(t, float64(0), result)
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
	result := sumMintedHashPowerForAllCollections(collections)
	require.Equal(t, 1674.01, result)
}

func TestCalculatePercentShouldReturnZeroIfInvalidHashingPowerProvided(t *testing.T) {
	require.Equal(t, decimal.Zero, calculatePercent(-1, -1, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculatePercent(10000, -1, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculatePercent(-1, 10000, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculatePercent(0, 0, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculatePercent(10000, 0, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculatePercent(0, 10000, decimal.NewFromInt(10000000)))
}

func TestCalculatePercentShouldReturnZeroIfRewardIsZero(t *testing.T) {
	require.Equal(t, decimal.Zero, calculatePercent(10000, 10000, decimal.Zero))
}

func TestCalculatePercent(t *testing.T) {
	require.Equal(t, decimal.NewFromInt(10).String(), calculatePercent(10000, 1000, decimal.NewFromInt(100)).String())
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldReturnErrorIfInvalidPeriod(t *testing.T) {
	s := NewPayService(nil, nil, nil, nil)
	_, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "", "", 1000, 100, nil, "", "", decimal.Zero)
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
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
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
				Reward:             decimal.Zero,
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
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_owner_1_payout_addr": float64(63.63636363636363),
		"nft_owner_2_payout_addr": float64(36.36363636363637),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      periodStart,
			TimeOwnedTo:        64,
			TotalTimeOwned:     63,
			PercentOfTimeOwned: 63.63636363636363,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      64,
			TimeOwnedTo:        periodEnd,
			TotalTimeOwned:     36,
			PercentOfTimeOwned: 36.36363636363637,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
			Reward:             decimal.Zero,
		},
	}

	// needed because values of decimal.Decimal are not exactly equal
	require.Equal(t, len(expectedNFTOwnersForPeriod), len(statistics.NFTOwnersForPeriod))
	for i := range expectedNFTOwnersForPeriod {
		require.Equal(t, expectedNFTOwnersForPeriod[i].Reward.String(), statistics.NFTOwnersForPeriod[i].Reward.String())
		expectedNFTOwnersForPeriod[i].Reward = statistics.NFTOwnersForPeriod[i].Reward
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
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_minter_payout_addr":  float64(9.090909090909092),
		"nft_owner_1_payout_addr": float64(3.0303030303030303),
		"nft_owner_2_payout_addr": float64(37.37373737373738),
		"nft_owner_3_payout_addr": float64(30.303030303030305),
		"nft_owner_4_payout_addr": float64(15.151515151515152),
		"nft_owner_5_payout_addr": float64(5.05050505050505),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.090909090909092,
			PayoutAddress:      "nft_minter_payout_addr",
			Owner:              "nft_minter",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.0303030303030303,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.37373737373738,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.303030303030305,
			PayoutAddress:      "nft_owner_3_payout_addr",
			Owner:              "nft_owner_3",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.151515151515152,
			PayoutAddress:      "nft_owner_4_payout_addr",
			Owner:              "nft_owner_4",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.05050505050505,
			PayoutAddress:      "nft_owner_5_payout_addr",
			Owner:              "nft_owner_5",
			Reward:             decimal.Zero,
		},
	}

	// needed because values of decimal.Decimal are not exactly equal
	require.Equal(t, len(expectedNFTOwnersForPeriod), len(statistics.NFTOwnersForPeriod))
	for i := range expectedNFTOwnersForPeriod {
		require.Equal(t, expectedNFTOwnersForPeriod[i].Reward.String(), statistics.NFTOwnersForPeriod[i].Reward.String())
		expectedNFTOwnersForPeriod[i].Reward = statistics.NFTOwnersForPeriod[i].Reward
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
	percents, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)

	expectedPercents := map[string]float64{
		"nft_minter_payout_addr":  float64(9.090909090909092),
		"nft_owner_1_payout_addr": float64(3.0303030303030303),
		"nft_owner_2_payout_addr": float64(37.37373737373738),
		"nft_owner_3_payout_addr": float64(30.303030303030305),
		"nft_owner_4_payout_addr": float64(15.151515151515152),
		"nft_owner_5_payout_addr": float64(5.05050505050505),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.090909090909092,
			PayoutAddress:      "nft_minter_payout_addr",
			Owner:              "nft_minter",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.0303030303030303,
			PayoutAddress:      "nft_owner_1_payout_addr",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.37373737373738,
			PayoutAddress:      "nft_owner_2_payout_addr",
			Owner:              "nft_owner_2",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.303030303030305,
			PayoutAddress:      "nft_owner_3_payout_addr",
			Owner:              "nft_owner_3",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.151515151515152,
			PayoutAddress:      "nft_owner_4_payout_addr",
			Owner:              "nft_owner_4",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.05050505050505,
			PayoutAddress:      "nft_owner_5_payout_addr",
			Owner:              "nft_owner_5",
			Reward:             decimal.Zero,
		},
	}
	// needed because values of decimal.Decimal are not exactly equal
	require.Equal(t, len(expectedNFTOwnersForPeriod), len(statistics.NFTOwnersForPeriod))
	for i := range expectedNFTOwnersForPeriod {
		require.Equal(t, expectedNFTOwnersForPeriod[i].Reward.String(), statistics.NFTOwnersForPeriod[i].Reward.String())
		expectedNFTOwnersForPeriod[i].Reward = statistics.NFTOwnersForPeriod[i].Reward
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
	_, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), types.NftTransferHistory{}, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
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

	_, err = s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, &statistics, currentNftOwner, "BTC", decimal.Zero)
	require.Equal(t, failErr, err)
}

type mockAPIRequester struct {
	mock.Mock
}

func (mar *mockAPIRequester) GetFarmStartTime(ctx context.Context, farmName string) (int64, error) {
	args := mar.Called(ctx, farmName)
	return args.Get(0).(int64), args.Error(1)
}

func (mar *mockAPIRequester) GetNftTransferHistory(ctx context.Context, collectionDenomId, nftId string, fromTimestamp int64) (types.NftTransferHistory, error) {
	args := mar.Called(ctx, collectionDenomId, nftId, fromTimestamp)
	return args.Get(0).(types.NftTransferHistory), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmTotalHashPowerFromPoolToday(ctx context.Context, farmName, sinceTimestamp string) (float64, error) {
	args := mar.Called(ctx, farmName, sinceTimestamp)
	return args.Get(0).(float64), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmCollectionsFromHasura(ctx context.Context, farmId int64) (types.CollectionData, error) {
	args := mar.Called(ctx, farmId)
	return args.Get(0).(types.CollectionData), args.Error(1)
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
