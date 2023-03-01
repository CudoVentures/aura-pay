package services

import (
	"testing"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestConvertAmountToBTC(t *testing.T) {
	// Arrange
	destinationAddressesWithAmount := map[string]types.AmountInfo{
		"address1": {
			Amount:           decimal.NewFromFloat(124.124124126),
			ThresholdReached: true,
		},
		"address2": {
			Amount:           decimal.NewFromFloat(0.123),
			ThresholdReached: true,
		},
		"address3": {
			Amount:           decimal.NewFromFloat(12412323.1),
			ThresholdReached: true,
		},
	}

	// Act
	result, err := convertAmountToBTC(destinationAddressesWithAmount)

	//Assert
	require.NoError(t, err)
	require.Equal(t, map[string]float64{
		"address1": 124.12412412,
		"address2": 0.123,
		"address3": 12412323.1,
	}, result, "Amounts are not equal to the given up to 8th digit")
}

func TestFilterByPaymentThreshold(t *testing.T) {
	// Arrange
	destinationAddressesWithAmount := map[string]types.AmountInfo{
		"address1": {
			Amount:           decimal.NewFromFloat(124.124124126),
			ThresholdReached: true,
		},
		"address2": {
			Amount:           decimal.NewFromFloat(0.123),
			ThresholdReached: true,
		},
		"address3": {
			Amount:           decimal.NewFromFloat(12412323.1),
			ThresholdReached: true,
		},
	}

	// Act
	result, err := convertAmountToBTC(destinationAddressesWithAmount)

	//Assert
	require.NoError(t, err)
	require.Equal(t, map[string]float64{
		"address1": 124.12412412,
		"address2": 0.123,
		"address3": 12412323.1,
	}, result, "Amounts are not equal to the given up to 8th digit")
}
