package observer

import (
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/xssnick/tonutils-go/ton"
)

type virtualWorkchain struct {
	ID     int32
	Shards map[int64]uint32 // shard -> it's last seqno
}

func (w *virtualWorkchain) ShardsLogString() string {
	b := strings.Builder{}
	b.WriteRune('{')
	idx := 0
	for shard, seqno := range w.Shards {
		idx++
		b.WriteString(shardFriendlyName(shard))
		b.WriteRune(':')
		b.WriteString(strconv.FormatUint(uint64(seqno), 10))
		if idx != len(w.Shards) {
			b.WriteRune(',')
		}
	}
	b.WriteRune('}')
	return b.String()
}

type shardStack []*ton.BlockIDExt

func (s *shardStack) Push(shard *ton.BlockIDExt) {
	*s = append(*s, shard)
}

func (s *shardStack) Pop() *ton.BlockIDExt {
	if len(*s) == 0 {
		return nil
	}
	top := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return top
}

func (s *shardStack) LogString() string {
	b := strings.Builder{}
	b.WriteRune('[')
	for idx, shard := range *s {
		b.WriteString(shardFriendlyName(shard.Shard))
		b.WriteRune(':')
		b.WriteString(strconv.FormatUint(uint64(shard.SeqNo), 10))
		if idx != len(*s)-1 {
			b.WriteRune(',')
		}
	}
	b.WriteRune(']')

	return b.String()
}

func shardFriendlyName(shard int64) string {
	return hex.EncodeToString(
		binary.BigEndian.AppendUint64(
			make([]byte, 0, 8),
			uint64(shard),
		),
	)
}
