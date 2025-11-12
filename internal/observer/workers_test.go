package observer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

var (
	errExpectedResponseListEmpty = errors.New("expected response list is empty")
)

type MockLiteclient struct {
	responseList []*cell.Slice
}

func (m *MockLiteclient) ResponseListLength() int {
	return len(m.responseList)
}

func (m *MockLiteclient) GetTransactionIDsFromBlock(ctx context.Context, blockID *ton.BlockIDExt) ([]ton.TransactionShortInfo, error) {
	return nil, nil
}
func (m *MockLiteclient) GetBlockTransactionsV2(ctx context.Context, block *ton.BlockIDExt, count uint32, after ...*ton.TransactionID3) ([]ton.TransactionShortInfo, bool, error) {
	return nil, false, nil
}
func (m *MockLiteclient) GetMasterchainInfo(ctx context.Context, timeout time.Duration) (*ton.BlockIDExt, error) {
	return nil, nil
}
func (m *MockLiteclient) GetBlockShardsInfo(ctx context.Context, master *ton.BlockIDExt) ([]*ton.BlockIDExt, error) {
	return nil, nil
}
func (m *MockLiteclient) GetTransaction(ctx context.Context, block *ton.BlockIDExt, addr *address.Address, lt uint64) (*tlb.Transaction, error) {
	return nil, nil
}
func (m *MockLiteclient) GetBlockData(ctx context.Context, block *ton.BlockIDExt) (*tlb.Block, error) {
	if len(m.responseList) == 0 {
		return nil, errExpectedResponseListEmpty
	}

	if m.responseList[0] == nil {
		m.responseList = m.responseList[1:]
		return nil, fmt.Errorf("failed to get block data: block is not applied")
	}
	header := &tlb.BlockHeader{}
	err := header.LoadFromCell(m.responseList[0])
	if err != nil {
		return nil, err
	}
	if header.SeqNo != block.SeqNo && header.Shard.WorkchainID != block.Workchain {
		return nil, fmt.Errorf("block header seqno %d does not match block id seqno %d", header.SeqNo, block.SeqNo)
	}

	m.responseList = m.responseList[1:]
	return &tlb.Block{
		BlockInfo: *header,
	}, nil
}
func (m *MockLiteclient) LookupBlock(ctx context.Context, timeout time.Duration, workchain int32, shard int64, seqno uint32) (*ton.BlockIDExt, error) {
	return nil, nil
}

func TestHandleShardBlock(t *testing.T) {
	tests := []struct {
		name               string
		lastSeenShardBlock []*ton.BlockIDExt
		currentShardBlock  *ton.BlockIDExt
		ltResponseList     []*cell.Slice
		want               shardStack
		wantError          error
	}{
		{
			name:               "one new shard block",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 1}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			ltResponseList: []*cell.Slice{
				makeBlockWithOneParentCellSlice(0, 0x2000000000000000, 2, 1, false),
			},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			},
			wantError: nil,
		},
		{
			name:               "more than one new shard block",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 1}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 3},
			ltResponseList: []*cell.Slice{
				makeBlockWithOneParentCellSlice(0, 0x2000000000000000, 3, 2, false),
				makeBlockWithOneParentCellSlice(0, 0x2000000000000000, 2, 1, false),
			},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 3},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			},
			wantError: nil,
		},
		{
			name:               "shard block after merge and 1 block diff for each shardchain",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x1000000000000000, SeqNo: 4}, {Workchain: 0, Shard: 0x3000000000000000, SeqNo: 8}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 10},
			//     10
			//     /\
			//    5  9
			//    4  8
			ltResponseList: []*cell.Slice{
				makeBlockWithTwoParentsCellSlice(0, 0x2000000000000000, 10, 5, 9),
				makeBlockWithOneParentCellSlice(0, 0x1000000000000000, 5, 4, false),
				makeBlockWithOneParentCellSlice(0, 0x3000000000000000, 9, 8, false),
			},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 10},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x1000000000000000, SeqNo: 5},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x3000000000000000, SeqNo: 9},
			},
			wantError: nil,
		},
		{
			name:               "shard block after merge and split",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 7}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 10},
			//     10
			//     /\
			//    9  |
			//    8  8
			//     \/
			//     7
			ltResponseList: []*cell.Slice{
				makeBlockWithTwoParentsCellSlice(0, 0x2000000000000000, 10, 9, 8),
				makeBlockWithOneParentCellSlice(0, 0x1000000000000000, 9, 8, false),
				makeBlockWithOneParentCellSlice(0, 0x1000000000000000, 8, 7, true),
				makeBlockWithOneParentCellSlice(0, 0x3000000000000000, 8, 7, true),
			},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 10},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x1000000000000000, SeqNo: 9},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x1000000000000000, SeqNo: 8},
				&ton.BlockIDExt{Workchain: 0, Shard: 0x3000000000000000, SeqNo: 8},
			},
			wantError: nil,
		},
		{
			name:               "block is not applied errors more than attempts limit",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 1}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			ltResponseList: []*cell.Slice{
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				makeBlockWithOneParentCellSlice(0, 0x2000000000000000, 2, 1, false),
			},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			},
			wantError: nil,
		},
		{
			name:               "unexpected error from liteclient returned after retries",
			lastSeenShardBlock: []*ton.BlockIDExt{{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 1}},
			currentShardBlock:  &ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			ltResponseList:     []*cell.Slice{},
			want: shardStack{
				&ton.BlockIDExt{Workchain: 0, Shard: 0x2000000000000000, SeqNo: 2},
			},
			wantError: errExpectedResponseListEmpty,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			mockLiteclient := &MockLiteclient{
				responseList: test.ltResponseList,
			}

			obs := New(mockLiteclient, nil)

			for _, shard := range test.lastSeenShardBlock {
				obs.workchainSetShard(shard)
			}

			stack := make(shardStack, 0)
			err := obs.handleShardBlock(ctx, test.currentShardBlock, &stack)

			assert.ErrorIs(t, err, test.wantError)
			assert.Equal(t, 0, mockLiteclient.ResponseListLength())
			assert.Equal(t, len(test.want), len(stack))
			for wantShard := test.want.Pop(); wantShard != nil; wantShard = test.want.Pop() {
				gotShard := stack.Pop()

				assert.Equal(t, wantShard.Workchain, gotShard.Workchain)
				assert.Equal(t, wantShard.Shard, gotShard.Shard)
				assert.Equal(t, wantShard.SeqNo, gotShard.SeqNo)
			}
		})
	}
}

func (o *observer) workchainSetShard(shard *ton.BlockIDExt) {
	o.workchain.Shards[shard.Shard] = shard.SeqNo
}

func makeBlockWithOneParentCellSlice(workchain int64, shard uint64, seqno uint64, parentSeqno uint64, afterSplit bool) *cell.Slice {
	return cell.BeginCell(). // tlb.BlockHeader
		// tlb.blockInfoPart
		MustStoreUInt(0x9bc7a987, 32).
		MustStoreUInt(1, 32).
		MustStoreBoolBit(true).       // NotMaster
		MustStoreBoolBit(false).      // AfterMerge
		MustStoreBoolBit(false).      // BeforeSplit
		MustStoreBoolBit(afterSplit). // AfterSplit
		MustStoreBoolBit(false).      // WantSplit
		MustStoreBoolBit(false).      // WantMerge
		MustStoreBoolBit(false).      // KeyBlock
		MustStoreBoolBit(false).      // VertSeqnoIncr
		MustStoreUInt(0, 8).          // Flags
		MustStoreUInt(seqno, 32).     // SeqNo
		MustStoreUInt(0, 32).         // VertSeqNo

		MustStoreInt(0b00, 2).       // Shard.Magic
		MustStoreInt(60, 6).         // Shard.PrefixBits
		MustStoreInt(workchain, 32). // Shard.WorkchainID
		MustStoreUInt(shard, 64).    // Shard.ShardPrefix

		MustStoreUInt(1, 32). // Shard
		MustStoreUInt(0, 32). // GenUtime
		MustStoreUInt(0, 64). // StartLt
		MustStoreUInt(0, 64). // EndLt
		MustStoreUInt(0, 32). // GenValidatorListHashShort
		MustStoreUInt(0, 32). // GenCatchainSeqno
		MustStoreUInt(0, 32). // MinRefMcSeqno
		MustStoreUInt(0, 32). // PrevKeyBlockSeqno

		// tlb.ExtBlkRef because NotMaster is true
		MustStoreRef(
			cell.BeginCell().
				MustStoreUInt(0, 64).                  // ExtBlkRef.EndLt
				MustStoreUInt(0, 32).                  // ExtBlkRef.SeqNo
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.RootHash
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.FileHash
				EndCell(),
		).
		// tlb.BlkPrevInfo; Only single block parent
		MustStoreRef(
			cell.BeginCell().
				MustStoreUInt(0, 64).                  // ExtBlkRef.EndLt
				MustStoreUInt(parentSeqno, 32).        // ExtBlkRef.SeqNo
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.RootHash
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.FileHash
				EndCell(),
		).
		EndCell().
		BeginParse()
}

func makeBlockWithTwoParentsCellSlice(workchain int64, shard uint64, seqno uint64, firstParentSeqno uint64, secondParentSeqno uint64) *cell.Slice {
	return cell.BeginCell(). // tlb.BlockHeader
		// tlb.blockInfoPart
		MustStoreUInt(0x9bc7a987, 32).
		MustStoreUInt(1, 32).
		MustStoreBoolBit(true).   // NotMaster
		MustStoreBoolBit(true).   // AfterMerge
		MustStoreBoolBit(false).  // BeforeSplit
		MustStoreBoolBit(false).  // AfterSplit
		MustStoreBoolBit(false).  // WantSplit
		MustStoreBoolBit(false).  // WantMerge
		MustStoreBoolBit(false).  // KeyBlock
		MustStoreBoolBit(false).  // VertSeqnoIncr
		MustStoreUInt(0, 8).      // Flags
		MustStoreUInt(seqno, 32). // SeqNo
		MustStoreUInt(0, 32).     // VertSeqNo

		MustStoreInt(0b00, 2).       // Shard.Magic
		MustStoreInt(60, 6).         // Shard.PrefixBits
		MustStoreInt(workchain, 32). // Shard.WorkchainID
		MustStoreUInt(shard, 64).    // Shard.ShardPrefix

		MustStoreUInt(1, 32). // Shard
		MustStoreUInt(0, 32). // GenUtime
		MustStoreUInt(0, 64). // StartLt
		MustStoreUInt(0, 64). // EndLt
		MustStoreUInt(0, 32). // GenValidatorListHashShort
		MustStoreUInt(0, 32). // GenCatchainSeqno
		MustStoreUInt(0, 32). // MinRefMcSeqno
		MustStoreUInt(0, 32). // PrevKeyBlockSeqno

		// tlb.ExtBlkRef because NotMaster is true
		MustStoreRef(
			cell.BeginCell().
				MustStoreUInt(0, 64).                  // ExtBlkRef.EndLt
				MustStoreUInt(0, 32).                  // ExtBlkRef.SeqNo
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.RootHash
				MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.FileHash
				EndCell(),
		).
		// tlb.BlkPrevInfo; Only single block parent
		MustStoreRef(
			cell.BeginCell().
				MustStoreRef(
					cell.BeginCell().
						MustStoreUInt(0, 64).                  // ExtBlkRef.EndLt
						MustStoreUInt(firstParentSeqno, 32).   // ExtBlkRef.SeqNo
						MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.RootHash
						MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.FileHash
						EndCell(),
				).
				MustStoreRef(
					cell.BeginCell().
						MustStoreUInt(0, 64).                  // ExtBlkRef.EndLt
						MustStoreUInt(secondParentSeqno, 32).  // ExtBlkRef.SeqNo
						MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.RootHash
						MustStoreSlice(make([]byte, 32), 256). // ExtBlkRef.FileHash
						EndCell(),
				).
				EndCell(),
		).
		EndCell().
		BeginParse()
}
