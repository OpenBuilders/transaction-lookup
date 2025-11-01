package observer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type WalletAddress [32]byte

type liteclient interface {
	GetTransactionIDsFromBlock(ctx context.Context, blockID *ton.BlockIDExt) ([]ton.TransactionShortInfo, error)
	GetBlockTransactionsV2(ctx context.Context, block *ton.BlockIDExt, count uint32, after ...*ton.TransactionID3) ([]ton.TransactionShortInfo, bool, error)
	GetMasterchainInfo(ctx context.Context, timeout time.Duration) (*ton.BlockIDExt, error)
	GetBlockShardsInfo(ctx context.Context, master *ton.BlockIDExt) ([]*ton.BlockIDExt, error)
	GetTransaction(ctx context.Context, block *ton.BlockIDExt, addr *address.Address, lt uint64) (*tlb.Transaction, error)
	GetBlockData(ctx context.Context, block *ton.BlockIDExt) (*tlb.Block, error)
	LookupBlock(ctx context.Context, timeout time.Duration, workchain int32, shard int64, seqno uint32) (*ton.BlockIDExt, error)
}

type rdb interface {
	PSubscribe(ctx context.Context, channels ...string) *redis.PubSub
	XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd
	Keys(ctx context.Context, pattern string) *redis.StringSliceCmd
}

type observer struct {
	lt  liteclient
	rdb rdb

	addresses      map[WalletAddress]struct{}
	addressesMutex sync.RWMutex

	workchain      *virtualWorkchain
	masterBlocks   chan *ton.BlockIDExt
	shardBlocks    chan *ton.BlockIDExt
	noticedWallets chan WalletAddress
}

func New(lt liteclient, rdb rdb) *observer {
	return &observer{
		lt:  lt,
		rdb: rdb,

		addresses:      make(map[WalletAddress]struct{}),
		addressesMutex: sync.RWMutex{},

		workchain: &virtualWorkchain{
			ID:     0,
			Shards: make(map[int64]uint32),
		},
		masterBlocks:   make(chan *ton.BlockIDExt),
		shardBlocks:    make(chan *ton.BlockIDExt),
		noticedWallets: make(chan WalletAddress),
	}
}

func (o *observer) Start(ctx context.Context, wg *sync.WaitGroup) error {
	slog.Info("loading redis wallets in memory...")
	if err := o.loadAddresses(ctx); err != nil {
		return err
	}
	slog.Info("loading done")

	wg.Add(1)
	go func() {
		defer wg.Done()
		o.startRedisEventsHandler(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.startRedisNotifier(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.startShardsHandler(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.startMastersHandler(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.startMasterObserver(ctx)
	}()
	return nil
}
