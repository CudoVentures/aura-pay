package services

import (
	"fmt"
	"log"
	"time"

	"github.com/CudoVentures/tokenised-infrastructure-rewarder/internal/app/tokenised-infrastructure-rewarder/requesters"
)

func GetNonExpiredNFTs() ([]requesters.NFT, error) {
	nonExpiredNFTsFromHasura, err := getAllNonExpiredNFTsFromHasura()
	if err != nil {
		return nil, err
	}

	nftCollectionsFromNode, err := getNFTCollectionsFromNode(nonExpiredNFTsFromHasura)
	var nonExpiredNFTs []requesters.NFT

	for _, collection := range nftCollectionsFromNode {
		for _, nft := range collection.Result.Collection.Nfts {
			if nft.DataJson.ExpirationDate >= time.Now().UTC().Unix() {
				nonExpiredNFTs = append(nonExpiredNFTs, nft)
			} else {
				err := fmt.Errorf("NFT with denomId [%s], tokenId [%s] and expirationTime [%d] is expired and reward will not payed",
					collection.Result.Collection.Denom, nft.Id, nft.DataJson.ExpirationDate)
				log.Fatal(err)
				return nil, err
			}
		}
	}

	return nonExpiredNFTs, nil
}

func getNFTCollectionsFromNode(nonExpiredNFTsFromHasura map[string][]int) ([]requesters.NFTCollectionResponse, error) {
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

func getAllNonExpiredNFTsFromHasura() (map[string][]int, error) {
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
		result[elem.DenomId] = append(result[elem.DenomId], elem.Id)
	}

	return result
}
