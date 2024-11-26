package observer

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/ton"
)

const (
	GetMasterShardsAttemptsLimit = 5
	ShardsHandlerLimit           = 10
)

type WalletAddress [32]byte

type Observer struct {
	ctx          context.Context
	api          ton.APIClientWrapped
	walletsSet   map[WalletAddress]time.Duration
	masterBlocks chan *ton.BlockIDExt
	shardBlocks  chan *ton.BlockIDExt
}

func NewBlockScanner(api ton.APIClientWrapped) *Observer {
	ctx := api.Client().StickyContext(context.Background())
	return &Observer{
		ctx:          ctx,
		api:          api,
		walletsSet:   map[WalletAddress]time.Duration{},
		masterBlocks: make(chan *ton.BlockIDExt),
		shardBlocks:  make(chan *ton.BlockIDExt),
	}
}

func (o *Observer) startMasterObserver(ctx context.Context) {
	var lastMasterSeqNo uint32
	for {
		currentMaster, err := o.api.GetMasterchainInfo(ctx)
		if err != nil {
			log.Printf("failed to get current master block: %v\n", err)
			continue
		}
		if currentMaster.SeqNo > lastMasterSeqNo {
			lastMasterSeqNo = currentMaster.SeqNo
			o.masterBlocks <- currentMaster
		}
	}
}

func (o *Observer) startMastersHandler(ctx context.Context) {
	for master := range o.masterBlocks {
		go o.handleNewMaster(ctx, master)
	}
}

func (o *Observer) handleNewMaster(ctx context.Context, master *ton.BlockIDExt) {
	attempt := 0
	for ; attempt < GetMasterShardsAttemptsLimit; attempt += 1 {
		shards, err := o.api.GetBlockShardsInfo(ctx, master)
		if err != nil {
			continue
		}
		for _, shard := range shards {
			o.shardBlocks <- shard
		}
		return
	}
	log.Printf("failed to get shards (%d, %d, %d)\n", master.Workchain, master.Shard, master.SeqNo)
}

func (o *Observer) startShardsHandler(ctx context.Context) {
	var wg sync.WaitGroup
	for idx := 0; idx < ShardsHandlerLimit; idx += 1 {
		go func(idx int) {
			defer wg.Done()
			o.shardHandleWorker(ctx, idx)
		}(idx)
	}

	wg.Wait()
}

func (o *Observer) shardHandleWorker(ctx context.Context, workerID int) {
	for shardBlock := range o.shardBlocks {
		blockTXs, err := o.GetTransactionIDsFromBlock(ctx, shardBlock)
		if err != nil {
			continue
		}
		for _, blockTX := range blockTXs {
			_, ok := o.walletsSet[WalletAddress(blockTX.Account)]
			if ok {
				log.Printf(
					"[WORKER %d] Address: %s; LT: %d; WC: %d, Shard: %d, SeqNo: %d\n",
					workerID,
					address.NewAddress(0, 0, blockTX.Account).String(),
					blockTX.LT,
					shardBlock.Workchain,
					shardBlock.Shard,
					shardBlock.SeqNo,
				)
			}
		}
	}
}

func (o *Observer) Start(ctx context.Context) error {
	a := address.MustParseRawAddr("0:1e73f30a0219c625af76bb2b30b3a90479827d641511ef17681c136ff1996d16")
	o.walletsSet[WalletAddress(a.Data())] = time.Hour * 100

	go o.startShardsHandler(ctx)
	go o.startMastersHandler(ctx)
	go o.startMasterObserver(ctx)

	<-o.ctx.Done()
	return nil
}
