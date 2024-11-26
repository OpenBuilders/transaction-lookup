package observer

import (
	"context"
	"log"

	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/ton"
)

func NewAPIClient() (ton.APIClientWrapped, error) {
	client := liteclient.NewConnectionPool()

	cfg, err := liteclient.GetConfigFromUrl(context.Background(), "https://ton.org/global.config.json")
	if err != nil {
		return nil, err
	}
	err = client.AddConnectionsFromConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	api := ton.NewAPIClient(client, ton.ProofCheckPolicyFast).WithRetry()
	api.SetTrustedBlockFromConfig(cfg)

	log.Println("fetching and checking proofs since config init block ...")
	_, err = api.CurrentMasterchainInfo(context.Background())
	if err != nil {
		return nil, err
	}
	return api, nil
}
