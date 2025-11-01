package observer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
	"github.com/xssnick/tonutils-go/address"
)

func getWalletAddressFromMessage(msg *redis.Message) (WalletAddress, error) {
	address, err := address.ParseRawAddr(msg.Payload)
	if err != nil {
		return WalletAddress{}, err
	}
	return WalletAddress(address.Data()), nil
}

func (o *observer) loadAddresses(ctx context.Context) error {
	wallets, err := o.rdb.Keys(ctx, "*").Result()
	if err != nil {
		return fmt.Errorf("failed to get redis db keys: %v", err)
	}

	o.addressesMutex.Lock()
	defer o.addressesMutex.Unlock()
	for _, wallet := range wallets {
		walletAddress, err := address.ParseRawAddr(wallet)
		if err != nil {
			slog.Error("failed to parse address from redis", "wallet", wallet, "error", err)
			continue
		}
		o.addresses[WalletAddress(walletAddress.Data())] = struct{}{}
	}
	return nil
}

func (o *observer) isAddressExists(key WalletAddress) bool {
	o.addressesMutex.RLock()
	defer o.addressesMutex.RUnlock()
	_, exists := o.addresses[key]
	return exists
}

func (o *observer) setAddress(key WalletAddress) {
	o.addressesMutex.Lock()
	defer o.addressesMutex.Unlock()
	o.addresses[key] = struct{}{}
}

func (o *observer) delAddress(key WalletAddress) {
	o.addressesMutex.Lock()
	defer o.addressesMutex.Unlock()
	delete(o.addresses, key)
}
