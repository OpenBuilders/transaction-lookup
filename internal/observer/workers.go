package observer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	lt "transaction-lookup/internal/liteclient"

	"github.com/redis/go-redis/v9"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

const (
	redisChannelExpired   = "__keyevent@0__:expired"
	redisChannelNewExpire = "__keyevent@0__:expire"

	lookupBlockTimeoutPerNode = time.Millisecond * 400
	lookupBlockDelay          = time.Millisecond * 400

	newBlockGenerationAverageTime = time.Second * 3

	handleMasterShardsAttemptsLimit    = 3
	serializeParentBlocksAttemptsLimit = 3
	shardsHandlerLimit                 = 10
)

// ––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––
// Redis workers
// ––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––

func (o *observer) startRedisEventsHandler(ctx context.Context) {
	pubsub := o.rdb.PSubscribe(ctx, "__keyevent@0__:*")
	defer pubsub.Close()

	for {
		select {
		case <-ctx.Done():
			slog.Info("redis events handler stopped")
			return
		case message := <-pubsub.Channel():
			switch message.Channel {
			case redisChannelExpired:
				slog.Debug("wallet expired", "wallet", message.Payload)
				walletAddress, err := getWalletAddressFromMessage(message)
				if err != nil {
					slog.Error("received bad message", "message", message.Payload, "channel", redisChannelExpired, "error", err)
				}
				o.delAddress(walletAddress)
			case redisChannelNewExpire:
				slog.Debug("new wallet set", "wallet", message.Payload)
				walletAddress, err := getWalletAddressFromMessage(message)
				if err != nil {
					slog.Error("received bad message", "message", message.Payload, "channel", redisChannelNewExpire, "error", err)
				}
				o.setAddress(walletAddress)
			default:
				slog.Warn("new unknown event", "channel", message.Channel, "message", message.Payload)
			}
		}
	}
}

func (o *observer) startRedisNotifier(ctx context.Context) {
	for noticedWallet := range o.noticedWallets {
		_, err := o.rdb.XAdd(
			ctx,
			&redis.XAddArgs{
				Stream: "noticed_wallets",
				Values: map[string]interface{}{
					"wallet": fmt.Sprintf("%d:%x", 0, noticedWallet),
				},
			},
		).Result()
		if err != nil {
			slog.Error("failed to write to stream", "wallet", fmt.Sprintf("%d:%x", 0, noticedWallet), "error", err)
			continue
		}
	}
	slog.Info("redis notifier stopped")
}

// ––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––
// Blockchain workers
// ––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––

func (o *observer) startMasterObserver(ctx context.Context) {
	currentMaster, err := o.lt.GetMasterchainInfo(ctx, lookupBlockTimeoutPerNode)
	if err != nil {
		slog.Error("failed to get current master block", "error", err)
		return
	}

	lastMasterSeqNo := currentMaster.SeqNo
	lastMasterLookupTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			close(o.masterBlocks)
			slog.Info("master-blocks channel closed")
			slog.Info("master-blocks observer stopped")
			return
		default:
			time.Sleep(lookupBlockDelay)

			// That lookup request will try dirrefent nodes in case of timeout
			currentMaster, err := o.lt.GetMasterchainInfo(ctx, lookupBlockTimeoutPerNode)
			if err != nil {
				if !errors.Is(err, context.DeadlineExceeded) {
					slog.Error("failed to get current master block", "error", err)
				}
				continue
			}

			if currentMaster.SeqNo > lastMasterSeqNo {
				slog.Debug("current seqno", "v", currentMaster.SeqNo, "time", time.Since(lastMasterLookupTime))
				if time.Since(lastMasterLookupTime) > newBlockGenerationAverageTime {
					slog.Warn("new block lookup took too long", "time", time.Since(lastMasterLookupTime), "seqno", currentMaster.SeqNo)
				}

				lastMasterLookupTime = time.Now()
				lastMasterSeqNo = currentMaster.SeqNo
				o.masterBlocks <- currentMaster
			}
		}
	}
}
func (o *observer) startMastersHandler(ctx context.Context) {
	for masterBlock := range o.masterBlocks {
		o.handleNewMaster(ctx, masterBlock)
	}

	close(o.shardBlocks)
	slog.Info("shard-blocks channel closed")
	slog.Info("master-blocks handler stopped")
}

func (o *observer) handleNewMaster(ctx context.Context, master *ton.BlockIDExt) {
	slog.Debug("current workchain state", "workchain", o.workchain.ID, "shards", o.workchain.ShardsLogString())
	var (
		attempts = 0
		err      error
	)
	for attempts < handleMasterShardsAttemptsLimit {
		// Getting from current master block all shards
		var shards []*ton.BlockIDExt
		if shards, err = o.lt.GetBlockShardsInfo(ctx, master); err != nil {
			if lt.IsNotReadyError(err) || lt.IsTimeoutError(err) {
				slog.Warn("failed to get shards, retrying...", "error", err, "workchain", master.Workchain, "shard", shardFriendlyName(master.Shard), "seqno", master.SeqNo)
				time.Sleep(time.Millisecond * 100)
				continue
			}

			attempts += 1
			slog.Error("failed to get shards", "error", err, "workchain", master.Workchain, "shard", shardFriendlyName(master.Shard), "seqno", master.SeqNo)
			continue
		}

		if len(o.workchain.Shards) == 0 {
			// First time we see this workchain, so we need to handle all shards
			for _, shard := range shards {
				o.workchain.Shards[shard.Shard] = shard.SeqNo
				o.shardBlocks <- shard
			}
			return
		}

		// Fill new virtual workchain shards map to save it
		newVirtualWorkchainShards := make(map[int64]uint32, len(shards))
		for _, shard := range shards {
			//  Handle recursively each shard down to one of known shards from previous state
			stack := make(shardStack, 0)
			if err := o.handleShardBlock(ctx, shard, &stack); err != nil {
				slog.Error("failed to handle shard block", "error", err, "workchain", shard.Workchain, "shard", shardFriendlyName(shard.Shard), "seqno", shard.SeqNo)
				continue
			}

			slog.Debug("handled shard stack", "workchain", shard.Workchain, "shard", shardFriendlyName(shard.Shard), "seqno", shard.SeqNo, "stack", stack.LogString())

			// Empty stack to start handling old shards first
			for top := stack.Pop(); top != nil; top = stack.Pop() {
				o.shardBlocks <- top
			}

			newVirtualWorkchainShards[shard.Shard] = shard.SeqNo
		}
		// Update virtual workchain shards map
		o.workchain.Shards = newVirtualWorkchainShards
		return
	}
	slog.Error("failed to get shards", "workchain", master.Workchain, "shard", shardFriendlyName(master.Shard), "seqno", master.SeqNo)
}

func (o *observer) handleShardBlock(ctx context.Context, shard *ton.BlockIDExt, stack *shardStack) error {
	// If we met one of known shards from previous state
	oldShardSeqNo, ok := o.workchain.Shards[shard.Shard]
	if ok && oldShardSeqNo >= shard.SeqNo {
		return nil
	}

	stack.Push(shard)

	var (
		attempts = 0
		err      error
	)
	for attempts < serializeParentBlocksAttemptsLimit {
		// Get shard block data to serialize his parent blocks (1+)
		var shardData *tlb.Block
		if shardData, err = o.lt.GetBlockData(ctx, shard); err != nil {
			if lt.IsNotReadyError(err) || lt.IsTimeoutError(err) {
				slog.Warn("failed to get shard block data, retrying...", "error", err, "workchain", shard.Workchain, "shard", shardFriendlyName(shard.Shard), "seqno", shard.SeqNo)
				time.Sleep(time.Millisecond * 100)
				continue
			}

			attempts += 1
			slog.Error("failed to get shard block data", "error", err, "workchain", shard.Workchain, "shard", shardFriendlyName(shard.Shard), "seqno", shard.SeqNo)
			continue
		}

		var parents []*ton.BlockIDExt
		if parents, err = shardData.BlockInfo.GetParentBlocks(); err != nil {
			attempts += 1
			continue
		}

		// Parse parent blocks
		for _, parent := range parents {
			if parentErr := o.handleShardBlock(ctx, parent, stack); parentErr != nil {
				err = errors.Join(err, parentErr)
			}
		}
		return err
	}
	return fmt.Errorf("failed to serialize parent blocks: %w", err)
}

func (o *observer) startShardsHandler(ctx context.Context) {
	var wg sync.WaitGroup
	for idx := 0; idx < shardsHandlerLimit; idx += 1 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			o.shardHandleWorker(ctx, idx)
		}(idx)
	}
	wg.Wait()
	close(o.noticedWallets)
	slog.Info("noticed wallets channel closed")
	slog.Info("shards-handler observer stopped")
}

func (o *observer) shardHandleWorker(ctx context.Context, idx int) {
	for shardBlock := range o.shardBlocks {
		slog.Debug("handling new shard", "workchain", shardBlock.Workchain, "shard", shardFriendlyName(shardBlock.Shard), "seqno", shardBlock.SeqNo)
		blockTXs, err := o.lt.GetTransactionIDsFromBlock(ctx, shardBlock)
		if err != nil {
			slog.Error("failed to get block transactions", "workchain", shardBlock.Workchain, "shard", shardFriendlyName(shardBlock.Shard), "seqno", shardBlock.SeqNo, "error", err)
			continue
		}
		for _, blockTX := range blockTXs {
			if !o.isAddressExists(WalletAddress(blockTX.Account)) {
				continue
			}
			o.noticedWallets <- WalletAddress(blockTX.Account)
			slog.Debug("noticed wallet",
				"wid", idx,
				"address", address.NewAddress(0, 0, blockTX.Account).String(),
				"lt", blockTX.LT,
				"workchain", shardBlock.Workchain,
				"shard", shardFriendlyName(shardBlock.Shard),
				"seqno", shardBlock.SeqNo,
			)
		}
	}
	slog.Info("shards-handler worker stopped", "wid", idx)
}
