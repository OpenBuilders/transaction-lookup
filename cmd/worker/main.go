package main

import (
	"context"
	"log"
	"transaction-lookup/internal/observer"
)

func main() {
	ctx, _ := context.WithCancel(context.Background())

	log.Println("initializing new api client...")
	api, err := observer.NewAPIClient()
	if err != nil {
		log.Println("liteserver api client init failed")
		return
	}

	log.Println("initializing new block scanner...")
	scanner := observer.NewBlockScanner(api)

	log.Println("monitoring new blocks...")
	scanner.Start(ctx)
}
