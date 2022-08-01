package services

import (
	"fmt"
	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
	"log"
	"time"
)

func GetAllNonExpiredNFTsFromHasura() (map[string][]int, error) {
	nonExpiredNFTsFromHasuraData, err := requesters.GetAllNonExpiredNFTsFromHasura()
	if err != nil {
		return nil, err
	}
	groupedIdsByDenom := groupNFTsIdByDenomId(nonExpiredNFTsFromHasuraData)
	return groupedIdsByDenom, nil
}

func groupNFTsIdByDenomId(data requesters.NFTData) map[string][]int {
	result := make(map[string][]int)

	for _, elem := range data.Data.NftsByExpirationDate {
		if val, ok := result[elem.DenomId]; ok {
			val = append(val, elem.Id)
		} else {
			result[elem.DenomId] = append(result[elem.DenomId], elem.Id)
		}
	}

	return result
}

func GetNonExpiredNFTs() ([]requesters.NFT, error) {
	nonExpiredNFTsFromHasura, err := GetAllNonExpiredNFTsFromHasura()
	if err != nil {
		return nil, err
	}
	nftCollectionsFromNode, err := GetNFTCollectionsFromNode(nonExpiredNFTsFromHasura)
	var nonExpiredNFTs []requesters.NFT

	for _, collection := range nftCollectionsFromNode {
		for _, nft := range collection.Result.Collection.Nfts {
			if nft.Data.ExpirationDate >= time.Now().UTC().Unix() {
				nonExpiredNFTs = append(nonExpiredNFTs, nft)
			} else {
				err := fmt.Errorf("NFT with denomId [%s], tokenId [%s] and expirationTime [%d] is expired and reward will not payed",
					collection.Result.Collection.Denom, nft.Id, nft.Data.ExpirationDate)
				log.Fatal(err)
				return nil, err
			}
		}
	}

	return nonExpiredNFTs, nil
}

func GetNFTCollectionsFromNode(nonExpiredNFTsFromHasura map[string][]int) ([]requesters.NFTCollectionResponse, error) {
	var result []requesters.NFTCollectionResponse
	for k, v := range nonExpiredNFTsFromHasura {
		NFTCollectionResponse, err := requesters.GetNFTsByIds(k, v)
		if err != nil {
			return nil, err
		}
		result = append(result, NFTCollectionResponse)
	}

	return result, nil
}
