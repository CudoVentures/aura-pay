package gql

import (
	"context"
	"fmt"
	"github.com/hasura/go-graphql-client"
)

func GetAllNFTs() {
	client := graphql.NewClient("https://example.com/graphql", nil)
	query := GetAllNFTsQuery{Name: "test"}
	err := client.Query(context.Background(), query, nil)
	if err != nil {
		// Handle error.
	}
	fmt.Println(query.Name)

}
