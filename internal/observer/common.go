package observer

import (
	"context"
	"sort"

	"github.com/xssnick/tonutils-go/ton"
)

func (o *Observer) GetTransactionIDsFromBlock(ctx context.Context, blockID *ton.BlockIDExt) ([]ton.TransactionShortInfo, error) {
	var (
		txIDList []ton.TransactionShortInfo
		after    *ton.TransactionID3
		next     = true
	)
	for next {
		fetchedIDs, more, err := o.api.GetBlockTransactionsV2(ctx, blockID, 256, after)
		if err != nil {
			return nil, err
		}
		txIDList = append(txIDList, fetchedIDs...)
		next = more
		if more {
			after = fetchedIDs[len(fetchedIDs)-1].ID3()
		}
	}
	sort.Slice(txIDList, func(i, j int) bool {
		return txIDList[i].LT < txIDList[j].LT
	})
	return txIDList, nil
}
