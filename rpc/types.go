package rpc

import (
	"context"
	"encoding/json"
	"flychain/common"
	"flychain/common/hexutil"
	"fmt"
	"math"
	"strings"
)

// API 描述了通过 RPC 接口提供的一组方法
type API struct {
	Namespace     string      // 暴露 Service 的 rpc 方法的命名空间
	Version       string      // 已弃用 - 此字段不再使用，但为了兼容性而保留
	Service       interface{} // 持有方法的接收者实例
	Public        bool        // 已弃用 - 此字段不再使用，但为了兼容性而保留
	Authenticated bool        // api 是否只能在身份验证后可用。
}

// ServerCodec实现服务端读取、解析、写入RPC消息
// 一个 RPC 会话。实现必须是安全的，因为可以调用编解码器
// 同时执行多个 go-routines。
type ServerCodec interface {
	peerInfo() PeerInfo
	readBatch() (msg []*jsonrpcMessage, isBatch bool, err error)
	close()

	jsonWriter
}

// jsonWriter 可以将 JSON 消息写入其底层连接。
// 实现对于并发使用必须是安全的。
type jsonWriter interface {
	// writeJSON 将消息写入连接。
	writeJSON(ctx context.Context, msg interface{}, isError bool) error

	// Closed 返回一个在连接关闭时关闭的通道。
	closed() <-chan interface{}
	// RemoteAddr 返回连接的对端地址。
	remoteAddr() string
}

type BlockNumber int64

const (
	SafeBlockNumber      = BlockNumber(-4)
	FinalizedBlockNumber = BlockNumber(-3)
	PendingBlockNumber   = BlockNumber(-2)
	LatestBlockNumber    = BlockNumber(-1)
	EarliestBlockNumber  = BlockNumber(0)
)

// UnmarshalJSON 将给定的 JSON 片段解析为 BlockNumber。它支持：
// - “safe”、“finalized”、“latest”、“earliest”或“pending”作为字符串参数
// - 块号
// 返回的错误：
// - 当给定参数不是已知字符串时出现无效块号错误
// - 当给定的块号太小或太大时出现超出范围的错误
func (bn *BlockNumber) UnmarshalJSON(data []byte) error {
	input := strings.TrimSpace(string(data))
	if len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"' {
		input = input[1 : len(input)-1]
	}

	switch input {
	case "earliest":
		*bn = EarliestBlockNumber
		return nil
	case "latest":
		*bn = LatestBlockNumber
		return nil
	case "pending":
		*bn = PendingBlockNumber
		return nil
	case "finalized":
		*bn = FinalizedBlockNumber
		return nil
	case "safe":
		*bn = SafeBlockNumber
		return nil
	}

	blckNum, err := hexutil.DecodeUint64(input)
	if err != nil {
		return err
	}
	if blckNum > math.MaxInt64 {
		return fmt.Errorf("block number larger than int64")
	}
	*bn = BlockNumber(blckNum)
	return nil
}

// MarshalText 实现了 encoding.TextMarshaler。它编组：
// - “safe”、“finalized”、“latest”、“earliest”或“pending”作为字符串
// - 其他数字为十六进制
func (bn BlockNumber) MarshalText() ([]byte, error) {
	switch bn {
	case EarliestBlockNumber:
		return []byte("earliest"), nil
	case LatestBlockNumber:
		return []byte("latest"), nil
	case PendingBlockNumber:
		return []byte("pending"), nil
	case FinalizedBlockNumber:
		return []byte("finalized"), nil
	case SafeBlockNumber:
		return []byte("safe"), nil
	default:
		return hexutil.Uint64(bn).MarshalText()
	}
}

func (bn BlockNumber) Int64() int64 {
	return (int64)(bn)
}

type BlockNumberOrHash struct {
	BlockNumber      *BlockNumber `json:"blockNumber,omitempty"`
	BlockHash        *common.Hash `json:"blockHash,omitempty"`
	RequireCanonical bool         `json:"requireCanonical,omitempty"`
}

func (bnh *BlockNumberOrHash) UnmarshalJSON(data []byte) error {
	type erased BlockNumberOrHash
	e := erased{}
	err := json.Unmarshal(data, &e)
	if err == nil {
		if e.BlockNumber != nil && e.BlockHash != nil {
			return fmt.Errorf("cannot specify both BlockHash and BlockNumber, choose one or the other")
		}
		bnh.BlockNumber = e.BlockNumber
		bnh.BlockHash = e.BlockHash
		bnh.RequireCanonical = e.RequireCanonical
		return nil
	}
	var input string
	err = json.Unmarshal(data, &input)
	if err != nil {
		return err
	}
	switch input {
	case "earliest":
		bn := EarliestBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "latest":
		bn := LatestBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "pending":
		bn := PendingBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "finalized":
		bn := FinalizedBlockNumber
		bnh.BlockNumber = &bn
		return nil
	case "safe":
		bn := SafeBlockNumber
		bnh.BlockNumber = &bn
		return nil
	default:
		if len(input) == 66 {
			hash := common.Hash{}
			err := hash.UnmarshalText([]byte(input))
			if err != nil {
				return err
			}
			bnh.BlockHash = &hash
			return nil
		} else {
			blckNum, err := hexutil.DecodeUint64(input)
			if err != nil {
				return err
			}
			if blckNum > math.MaxInt64 {
				return fmt.Errorf("blocknumber too high")
			}
			bn := BlockNumber(blckNum)
			bnh.BlockNumber = &bn
			return nil
		}
	}
}

func (bnh *BlockNumberOrHash) Number() (BlockNumber, bool) {
	if bnh.BlockNumber != nil {
		return *bnh.BlockNumber, true
	}
	return BlockNumber(0), false
}

func (bnh *BlockNumberOrHash) Hash() (common.Hash, bool) {
	if bnh.BlockHash != nil {
		return *bnh.BlockHash, true
	}
	return common.Hash{}, false
}

func BlockNumberOrHashWithNumber(blockNr BlockNumber) BlockNumberOrHash {
	return BlockNumberOrHash{
		BlockNumber: &blockNr,
		BlockHash: nil,
		RequireCanonical: false,
	}
}

func BlockNumberOrHashWithHash(hash common.Hash, canonical bool) BlockNumberOrHash {
	return BlockNumberOrHash{
		BlockNumber:      nil,
		BlockHash:        &hash,
		RequireCanonical: canonical,
	}
}