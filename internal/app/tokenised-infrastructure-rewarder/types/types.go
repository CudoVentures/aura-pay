package types

type Farm struct {
	Id                                 string       `json:"id"`
	SubAccountName                     string       `json:"sub_account_name"`
	AddressForReceivingRewardsFromPool string       `json:"address_for_receiving_rewards_from_pool"` // Address set in the mining pool that receives the farm rewards
	LeftoverRewardPayoutAddress        string       `json:"leftover_reward_payout_address"`
	MaintenanceFeePayoutdAddress       string       `json:"maintenance_fee_payout_address"`
	Collections                        []Collection `json:"collections"`
	MonthlyMaintenanceFeeInBTC         float64      `json:"maintenance_fee_in_btc"`
}

type Collection struct {
	Denom Denom `json:"denom"`
	Nfts  []NFT `json:"nfts"`
}

type Denom struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Schema      string `json:"schema"`
	Creator     string `json:"creator"`
	Symbol      string `json:"symbol"`
	Traits      string `json:"traits"`
	Minter      string `json:"minter"`
	Description string `json:"description"`
	Data        string `json:"data"`
}

type NFT struct {
	Id       string `json:"id"`
	Name     string `json:"name"`
	Uri      string `json:"uri"`
	Data     string `json:"data"`
	DataJson NFTDataJson
	Owner    string `json:"owner"`
}

type NFTDataJson struct {
	ExpirationDate int64   `json:"expiration_date"`
	HashRateOwned  float64 `json:"hash_rate_owned"`
}
