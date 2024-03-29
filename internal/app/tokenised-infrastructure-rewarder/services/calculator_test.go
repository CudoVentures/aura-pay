package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/infrastructure"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
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
	require.Equal(t, decimal.Zero, calculateRewardByPercent(-1, -1, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculateRewardByPercent(10000, -1, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculateRewardByPercent(-1, 10000, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculateRewardByPercent(0, 0, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculateRewardByPercent(10000, 0, decimal.NewFromInt(10000000)))
	require.Equal(t, decimal.Zero, calculateRewardByPercent(0, 10000, decimal.NewFromInt(10000000)))
}

func TestCalculatePercentShouldReturnZeroIfRewardIsZero(t *testing.T) {
	require.Equal(t, decimal.Zero, calculateRewardByPercent(10000, 10000, decimal.Zero))
}

func TestCalculatePercent(t *testing.T) {
	require.Equal(t, decimal.NewFromInt(10).String(), calculateRewardByPercent(10000, 1000, decimal.NewFromInt(100)).String())
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldReturnErrorIfInvalidPeriod(t *testing.T) {
	s := NewPayService(nil, nil, nil, nil)
	_, _, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), []types.NftTransferEvent{}, "", "", 1000, 100, "", "", decimal.Zero)
	require.Equal(t, errors.New("invalid period, start (1000) end (100)"), err)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldReturnHundredPercentRewardToCurrentOwnerIfNoTransferEvents(t *testing.T) {
	apiRequester := &mockAPIRequester{}
	payoutAddr := "payoutaddr"
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "addr1", "BTC").Return(payoutAddr, nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), []types.NftTransferEvent{}, "testdenom", "1", periodStart, periodEnd, currentNftOwner, "BTC", decimal.Zero)
	statistics.NFTOwnersForPeriod = nftOwnersForPeriod

	require.NoError(t, err)
	require.Equal(t, map[string]float64{"addr1": float64(100)}, percents)

	expectedStatistics := types.NFTStatistics{
		NFTOwnersForPeriod: []types.NFTOwnerInformation{
			{
				TimeOwnedFrom:      periodStart,
				TimeOwnedTo:        periodEnd,
				TotalTimeOwned:     periodEnd - periodStart,
				PayoutAddress:      "",
				PercentOfTimeOwned: 100,
				Owner:              "addr1",
				Reward:             decimal.Zero,
			},
		},
	}
	require.Equal(t, expectedStatistics, statistics)
}

func TestCalculateNftOwnersForTimePeriodWithRewardPercentShouldWorkWithSingleTransferEvent(t *testing.T) {
	history := `[
					{
						"to": "nft_owner_2",
						"from": "nft_owner_1",
						"timestamp": 64
					}
				]
	`

	var nftTransferHistory []types.NftTransferEvent
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC").Return("nft_owner_2_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)
	statistics.NFTOwnersForPeriod = nftOwnersForPeriod

	expectedPercents := map[string]float64{
		"nft_owner_1": float64(63.636363636363605),
		"nft_owner_2": float64(36.363636363636296),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      periodStart,
			TimeOwnedTo:        64,
			TotalTimeOwned:     63,
			PercentOfTimeOwned: 63.636363636363605,
			PayoutAddress:      "",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      64,
			TimeOwnedTo:        periodEnd,
			TotalTimeOwned:     36,
			PercentOfTimeOwned: 36.363636363636296,
			PayoutAddress:      "",
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
		[
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
	`

	var nftTransferHistory []types.NftTransferEvent
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_minter", "BTC").Return("nft_minter_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_3", "BTC").Return("nft_owner_3_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_4", "BTC").Return("nft_owner_4_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_5", "BTC").Return("nft_owner_5_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)
	statistics.NFTOwnersForPeriod = nftOwnersForPeriod

	expectedPercents := map[string]float64{
		"nft_minter":  float64(9.090909090909),
		"nft_owner_1": float64(3.030303030303),
		"nft_owner_2": float64(37.3737373737373),
		"nft_owner_3": float64(30.303030303030297),
		"nft_owner_4": float64(15.151515151515099),
		"nft_owner_5": float64(5.0505050505049995),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.090909090909,
			PayoutAddress:      "",
			Owner:              "nft_minter",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.030303030303,
			PayoutAddress:      "",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.3737373737373,
			PayoutAddress:      "",
			Owner:              "nft_owner_2",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.303030303030297,
			PayoutAddress:      "",
			Owner:              "nft_owner_3",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.151515151515099,
			PayoutAddress:      "",
			Owner:              "nft_owner_4",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.0505050505049995,
			PayoutAddress:      "",
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
		[
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
	`

	var nftTransferHistory []types.NftTransferEvent
	require.NoError(t, json.Unmarshal([]byte(history), &nftTransferHistory))

	apiRequester := &mockAPIRequester{}
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_minter", "BTC").Return("nft_minter_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_1", "BTC").Return("nft_owner_1_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_2", "BTC").Return("nft_owner_2_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_3", "BTC").Return("nft_owner_3_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_4", "BTC").Return("nft_owner_4_payout_addr", nil)
	apiRequester.On("GetPayoutAddressFromNode", mock.Anything, "nft_owner_5", "BTC").Return("nft_owner_5_payout_addr", nil)

	statistics := types.NFTStatistics{}
	currentNftOwner := "addr1"
	periodStart := int64(1)
	periodEnd := int64(100)
	s := NewPayService(nil, apiRequester, nil, nil)
	percents, nftOwnersForPeriod, err := s.calculateNftOwnersForTimePeriodWithRewardPercent(context.TODO(), nftTransferHistory, "testdenom", "1", periodStart, periodEnd, currentNftOwner, "BTC", decimal.Zero)
	require.NoError(t, err)
	statistics.NFTOwnersForPeriod = nftOwnersForPeriod

	expectedPercents := map[string]float64{
		"nft_minter":  float64(9.090909090909),
		"nft_owner_1": float64(3.030303030303),
		"nft_owner_2": float64(37.3737373737373),
		"nft_owner_3": float64(30.303030303030297),
		"nft_owner_4": float64(15.151515151515099),
		"nft_owner_5": float64(5.0505050505049995),
	}

	require.Equal(t, expectedPercents, percents)

	expectedNFTOwnersForPeriod := []types.NFTOwnerInformation{
		{
			TimeOwnedFrom:      1,
			TimeOwnedTo:        10,
			TotalTimeOwned:     9,
			PercentOfTimeOwned: 9.090909090909,
			PayoutAddress:      "",
			Owner:              "nft_minter",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      10,
			TimeOwnedTo:        13,
			TotalTimeOwned:     3,
			PercentOfTimeOwned: 3.030303030303,
			PayoutAddress:      "",
			Owner:              "nft_owner_1",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      13,
			TimeOwnedTo:        50,
			TotalTimeOwned:     37,
			PercentOfTimeOwned: 37.3737373737373,
			PayoutAddress:      "",
			Owner:              "nft_owner_2",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      50,
			TimeOwnedTo:        80,
			TotalTimeOwned:     30,
			PercentOfTimeOwned: 30.303030303030297,
			PayoutAddress:      "",
			Owner:              "nft_owner_3",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      80,
			TimeOwnedTo:        95,
			TotalTimeOwned:     15,
			PercentOfTimeOwned: 15.151515151515099,
			PayoutAddress:      "",
			Owner:              "nft_owner_4",
			Reward:             decimal.Zero,
		},
		{
			TimeOwnedFrom:      95,
			TimeOwnedTo:        100,
			TotalTimeOwned:     5,
			PercentOfTimeOwned: 5.0505050505049995,
			PayoutAddress:      "",
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

func TestCalculateHourlyMaintenanceFee(t *testing.T) {
	result, _ := decimal.NewFromString("0.0000000134408602")
	testCases := []struct {
		desc                    string
		farm                    types.Farm
		currentHashPowerForFarm float64
		helper                  InfrastructureHelper
		expectedResult          decimal.Decimal
	}{
		{
			desc: "successful case",
			farm: types.Farm{
				MaintenanceFeeInBtc: 0.01,
			},
			currentHashPowerForFarm: 1000,
			expectedResult:          result,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := NewPayService(nil, &mockAPIRequester{}, &mockHelper{}, nil)

			result := s.calculateHourlyMaintenanceFee(tc.farm, tc.currentHashPowerForFarm)
			assert.Equal(t, tc.expectedResult.String(), result.String(), "unexpected result for %s", tc.desc)
		})
	}
}

func TestCalculateMaintenanceFeeForNFT(t *testing.T) {
	testCases := []struct {
		desc                       string
		periodStart                int64
		periodEnd                  int64
		nftHashPower               float64
		hourlyFeePerThInBtcDecimal decimal.Decimal
		rewardForNftBtcDecimal     decimal.Decimal
		config                     infrastructure.Config
		expectedNftMaintenanceFee  decimal.Decimal
		expectedCudoMaintenance    decimal.Decimal
		expectedRewardForNft       decimal.Decimal
	}{
		{
			desc:                       "successful case",
			periodStart:                0,
			periodEnd:                  3600,
			nftHashPower:               1,
			hourlyFeePerThInBtcDecimal: decimal.NewFromFloat(0.0001),
			rewardForNftBtcDecimal:     decimal.NewFromFloat(0.001),
			config: infrastructure.Config{
				CUDOMaintenanceFeePercent: 10,
			},
			expectedNftMaintenanceFee: decimal.NewFromFloat(0.00009),
			expectedCudoMaintenance:   decimal.NewFromFloat(0.00001),
			expectedRewardForNft:      decimal.NewFromFloat(0.0009),
		}, {
			desc:                       "zero reward",
			periodStart:                0,
			periodEnd:                  3600,
			nftHashPower:               1,
			hourlyFeePerThInBtcDecimal: decimal.NewFromFloat(0.0001),
			rewardForNftBtcDecimal:     decimal.Zero,
			config: infrastructure.Config{
				CUDOMaintenanceFeePercent: 10,
			},
			expectedNftMaintenanceFee: decimal.Zero,
			expectedCudoMaintenance:   decimal.Zero,
			expectedRewardForNft:      decimal.Zero,
		},
		{
			desc:                       "zero maintenance fee",
			periodStart:                0,
			periodEnd:                  3600,
			nftHashPower:               1,
			hourlyFeePerThInBtcDecimal: decimal.Zero,
			rewardForNftBtcDecimal:     decimal.NewFromFloat(0.001),
			config: infrastructure.Config{
				CUDOMaintenanceFeePercent: 10,
			},
			expectedNftMaintenanceFee: decimal.Zero,
			expectedCudoMaintenance:   decimal.Zero,
			expectedRewardForNft:      decimal.NewFromFloat(0.001),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			s := NewPayService(&tc.config, &mockAPIRequester{}, &mockHelper{}, nil)

			nftMaintenanceFee, cudoMaintenance, rewardForNft, err := s.calculateMaintenanceFeeForNFT(tc.periodStart, tc.periodEnd, tc.hourlyFeePerThInBtcDecimal, tc.nftHashPower, tc.rewardForNftBtcDecimal)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedNftMaintenanceFee.String(), nftMaintenanceFee.String(), "unexpected NFT maintenance fee for %s", tc.desc)
			assert.Equal(t, tc.expectedCudoMaintenance.String(), cudoMaintenance.String(), "unexpected Cudo maintenance fee for %s", tc.desc)
			assert.Equal(t, tc.expectedRewardForNft.String(), rewardForNft.String(), "unexpected reward for NFT for %s", tc.desc)
		})
	}
}

func TestCalculateCudosFeeOfTotalFarmIncome(t *testing.T) {
	testCases := []struct {
		desc                         string
		config                       infrastructure.Config
		totalFarmIncomeBtcDecimal    decimal.Decimal
		expectedFarmIncomeBtcDecimal decimal.Decimal
		expectedCudosFeeBtcDecimal   decimal.Decimal
	}{
		{
			desc: "Test with a 10% CUDO fee",
			config: infrastructure.Config{
				CUDOFeeOnAllBTC: 10,
			},
			totalFarmIncomeBtcDecimal:    decimal.NewFromFloat(1),
			expectedFarmIncomeBtcDecimal: decimal.NewFromFloat(0.9),
			expectedCudosFeeBtcDecimal:   decimal.NewFromFloat(0.1),
		},
		{
			desc: "Test with a 20% CUDO fee",
			config: infrastructure.Config{
				CUDOFeeOnAllBTC: 20,
			},
			totalFarmIncomeBtcDecimal:    decimal.NewFromFloat(1),
			expectedFarmIncomeBtcDecimal: decimal.NewFromFloat(0.8),
			expectedCudosFeeBtcDecimal:   decimal.NewFromFloat(0.2),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			payService := NewPayService(&tc.config, &mockAPIRequester{}, &mockHelper{}, nil)

			farmIncomeBtcDecimal, cudosFeeBtcDecimal := payService.calculateCudosFeeOfTotalFarmIncome(tc.totalFarmIncomeBtcDecimal)

			if !farmIncomeBtcDecimal.Equal(tc.expectedFarmIncomeBtcDecimal) {
				t.Errorf("Expected farm income: %s, got: %s", tc.expectedFarmIncomeBtcDecimal, farmIncomeBtcDecimal)
			}

			if !cudosFeeBtcDecimal.Equal(tc.expectedCudosFeeBtcDecimal) {
				t.Errorf("Expected CUDO fee: %s, got: %s", tc.expectedCudosFeeBtcDecimal, cudosFeeBtcDecimal)
			}
		})
	}
}

func TestSumMintedHashPowerForCollection(t *testing.T) {
	testCases := []struct {
		desc                   string
		collection             types.Collection
		expectedTotalHashPower float64
	}{
		{
			desc: "Test with multiple NFTs",
			collection: types.Collection{
				Nfts: []types.NFT{
					{
						DataJson: types.NFTDataJson{
							HashRateOwned: 10,
						},
					},
					{
						DataJson: types.NFTDataJson{
							HashRateOwned: 20,
						},
					},
					{
						DataJson: types.NFTDataJson{
							HashRateOwned: 30,
						},
					},
				},
			},
			expectedTotalHashPower: 60,
		},
		{
			desc: "Test with a single NFT",
			collection: types.Collection{
				Nfts: []types.NFT{
					{
						DataJson: types.NFTDataJson{
							HashRateOwned: 5,
						},
					},
				},
			},
			expectedTotalHashPower: 5,
		},
		{
			desc: "Test with an empty collection",
			collection: types.Collection{
				Nfts: []types.NFT{},
			},
			expectedTotalHashPower: 0,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			totalHashPower := sumMintedHashPowerForCollection(tC.collection)

			if totalHashPower != tC.expectedTotalHashPower {
				t.Errorf("Expected total hash power: %f, got: %f", tC.expectedTotalHashPower, totalHashPower)
			}
		})
	}
}

func TestCalculateRewardByPercent(t *testing.T) {
	testCases := []struct {
		desc               string
		availableHashPower float64
		actualHashPower    float64
		reward             decimal.Decimal
		expectedReward     decimal.Decimal
	}{
		{
			desc:               "Test with valid input",
			availableHashPower: 100,
			actualHashPower:    25,
			reward:             decimal.NewFromFloat(1),
			expectedReward:     decimal.NewFromFloat(0.25),
		},
		{
			desc:               "Test with zero available hash power",
			availableHashPower: 0,
			actualHashPower:    25,
			reward:             decimal.NewFromFloat(1),
			expectedReward:     decimal.Zero,
		},
		{
			desc:               "Test with zero actual hash power",
			availableHashPower: 100,
			actualHashPower:    0,
			reward:             decimal.NewFromFloat(1),
			expectedReward:     decimal.Zero,
		},
		{
			desc:               "Test with zero reward",
			availableHashPower: 100,
			actualHashPower:    25,
			reward:             decimal.Zero,
			expectedReward:     decimal.Zero,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			calculatedReward := calculateRewardByPercent(tC.availableHashPower, tC.actualHashPower, tC.reward)

			if !calculatedReward.Equal(tC.expectedReward) {
				t.Errorf("Expected reward: %s, got: %s", tC.expectedReward, calculatedReward)
			}
		})
	}
}

func TestCalculatePercentByTime(t *testing.T) {
	testCases := []struct {
		desc                    string
		timestampPrevPayment    int64
		timestampCurrentPayment int64
		nftStartTime            int64
		nftEndTime              int64
		totalRewardForPeriod    decimal.Decimal
		expectedReward          decimal.Decimal
	}{
		{
			desc:                    "Test with valid input",
			timestampPrevPayment:    1000,
			timestampCurrentPayment: 2000,
			nftStartTime:            1000,
			nftEndTime:              2000,
			totalRewardForPeriod:    decimal.NewFromFloat(1),
			expectedReward:          decimal.NewFromFloat(1),
		},
		{
			desc:                    "Test with NFT only active for half the period",
			timestampPrevPayment:    1000,
			timestampCurrentPayment: 2000,
			nftStartTime:            1000,
			nftEndTime:              1500,
			totalRewardForPeriod:    decimal.NewFromFloat(1),
			expectedReward:          decimal.NewFromFloat(0.5),
		},
		{
			desc:                    "Test with NFT not active during the period",
			timestampPrevPayment:    1000,
			timestampCurrentPayment: 2000,
			nftStartTime:            3000,
			nftEndTime:              4000,
			totalRewardForPeriod:    decimal.NewFromFloat(1),
			expectedReward:          decimal.Zero,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			calculatedReward := calculatePercentByTime(tC.timestampPrevPayment, tC.timestampCurrentPayment, tC.nftStartTime, tC.nftEndTime, tC.totalRewardForPeriod)

			if !calculatedReward.Equal(tC.expectedReward) {
				t.Errorf("Expected reward: %s, got: %s", tC.expectedReward, calculatedReward)
			}
		})
	}
}

func TestCalculateLeftoverNftRewardDistribution(t *testing.T) {
	testCases := []struct {
		desc               string
		rewardForNftOwners decimal.Decimal
		statistics         []types.NFTStatistics
		expectedLeftover   decimal.Decimal
		expectedError      error
	}{
		{
			desc:               "Test with valid input",
			rewardForNftOwners: decimal.NewFromFloat(1),
			statistics: []types.NFTStatistics{
				{
					Reward:                   decimal.NewFromFloat(0.2),
					MaintenanceFee:           decimal.NewFromFloat(0.1),
					CUDOPartOfMaintenanceFee: decimal.NewFromFloat(0.05),
				},
				{
					Reward:                   decimal.NewFromFloat(0.3),
					MaintenanceFee:           decimal.NewFromFloat(0.2),
					CUDOPartOfMaintenanceFee: decimal.NewFromFloat(0.1),
				},
			},
			expectedLeftover: decimal.NewFromFloat(0.05),
			expectedError:    nil,
		},
		{
			desc:               "Test with distributed rewards exceeding farm reward",
			rewardForNftOwners: decimal.NewFromFloat(1),
			statistics: []types.NFTStatistics{
				{
					Reward:                   decimal.NewFromFloat(0.5),
					MaintenanceFee:           decimal.NewFromFloat(0.3),
					CUDOPartOfMaintenanceFee: decimal.NewFromFloat(0.1),
				},
				{
					Reward:                   decimal.NewFromFloat(0.6),
					MaintenanceFee:           decimal.NewFromFloat(0.4),
					CUDOPartOfMaintenanceFee: decimal.NewFromFloat(0.2),
				},
			},
			expectedLeftover: decimal.Decimal{},
			expectedError:    fmt.Errorf("distributed NFT awards bigger than farm nft reward. NftRewardDistribution: %s, TotalFarmRewardAfterCudosFee: %s", decimal.NewFromFloat(2.1), decimal.NewFromFloat(1)),
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			leftover, err := calculateLeftoverNftRewardDistribution(tC.rewardForNftOwners, tC.statistics)
			if err != nil && tC.expectedError.Error() != err.Error() {
				t.Errorf("Expected error: %s, got: %s", tC.expectedError, err)
			}

			if !leftover.Equal(tC.expectedLeftover) {
				t.Errorf("Expected leftover: %s, got: %s", tC.expectedLeftover, leftover)
			}
		})
	}
}

func TestCheckTotalAmountToDistribute(t *testing.T) {
	testCases := []struct {
		desc                              string
		receivedRewardForFarmBtcDecimal   decimal.Decimal
		destinationAddressesWithAmountBtc map[string]decimal.Decimal
		expectedError                     error
	}{
		{
			desc:                            "Equal amounts",
			receivedRewardForFarmBtcDecimal: decimal.NewFromInt(100),
			destinationAddressesWithAmountBtc: map[string]decimal.Decimal{
				"address1": decimal.NewFromInt(50),
				"address2": decimal.NewFromInt(50),
			},
			expectedError: nil,
		},
		{
			desc:                            "Unequal amounts",
			receivedRewardForFarmBtcDecimal: decimal.NewFromInt(100),
			destinationAddressesWithAmountBtc: map[string]decimal.Decimal{
				"address1": decimal.NewFromInt(40),
				"address2": decimal.NewFromInt(50),
			},
			expectedError: fmt.Errorf("distributed amount doesn't equal total farm rewards. Distributed amount: {90}, TotalFarmReward: {100}"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := checkTotalAmountToDistribute(tc.receivedRewardForFarmBtcDecimal, tc.destinationAddressesWithAmountBtc)

			if tc.expectedError == nil && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			} else if tc.expectedError != nil && (err == nil || err.Error() != tc.expectedError.Error()) {
				t.Errorf("Expected error: %v, got: %v", tc.expectedError, err)
			}
		})
	}
}

type mockAPIRequester struct {
	mock.Mock
}

func (mar *mockAPIRequester) GetHasuraCollectionNftMintEvents(ctx context.Context, collectionDenomId string) (types.NftMintHistory, error) {
	args := mar.Called(ctx, collectionDenomId)
	return args.Get(0).(types.NftMintHistory), args.Error(1)
}

func (mar *mockAPIRequester) GetDenomNftTransferHistory(ctx context.Context, collectionDenomId string, lastPaymentTimestamp, periodEnd int64) ([]types.NftTransferEvent, error) {
	args := mar.Called(ctx, collectionDenomId, lastPaymentTimestamp, periodEnd)
	return args.Get(0).([]types.NftTransferEvent), args.Error(1)
}

func (mar *mockAPIRequester) GetFarmStartTime(ctx context.Context, farmName string) (int64, error) {
	args := mar.Called(ctx, farmName)
	return args.Get(0).(int64), args.Error(1)
}

func (mar *mockAPIRequester) GetChainNftMintTimestamp(ctx context.Context, denomId, tokenId string) (int64, error) {
	args := mar.Called(ctx, denomId, tokenId)
	return args.Get(0).(int64), args.Error(1)
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

func (mar *mockAPIRequester) GetPayoutAddressFromNode(ctx context.Context, cudosAddress, network string) (string, error) {
	args := mar.Called(ctx, cudosAddress, network)
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

func (mar *mockAPIRequester) GetWalletTransaction(ctx context.Context, txId string) (*types.BtcWalletTransaction, error) {
	args := mar.Called(ctx, txId)
	return args.Get(0).(*types.BtcWalletTransaction), args.Error(1)
}
