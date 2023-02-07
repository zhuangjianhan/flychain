package rpc

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var (
	// 当连接不支持通知时返回 ErrNotificationsUnsupported
	ErrNotificationsUnsupported = errors.New("notifications not supported")
	// 当找不到给定 id 的通知时，返回 ErrSubscriptionNotFound
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

var globalGen = randomIDGenerator()

// ID 定义了一个伪随机数，用于识别 RPC 订阅。
type ID string

//NewID returns a new, random ID.
func NewID() ID {
	return globalGen()
}

// randomIDGenerator 返回一个生成随机 ID 的函数。
func randomIDGenerator() func() ID {
	var buf = make([]byte, 8)
	var seed int64
	if _, err := crand.Read(buf); err == nil {
		seed = int64(binary.BigEndian.Uint16(buf))
	} else {
		seed = int64(time.Now().Nanosecond())
	}

	var (
		mu  sync.Mutex
		rng = rand.New(rand.NewSource(seed))
	)
	return func() ID {
		mu.Lock()
		defer mu.Unlock()
		id := make([]byte, 16)
		rng.Read(id)
		return encodeID(id)
	}
}

func encodeID(b []byte) ID {
	id := hex.EncodeToString(b)
	id = strings.TrimLeft(id, "0")
	if id == "" {
		id = "0" // ID 是 RPC 数量，没有前导零，0 是 0x0。
	}
	return ID("0x" + id)
}

type notifierKey struct{}

// NotifierFromContext 返回存储在 ctx 中的 Notifier 值（如果有）。
func NotifierFromContext(ctx context.Context)

// 通知程序绑定到支持订阅的 RPC 连接。
// 服务器回调使用通知程序发送通知。
type Notifier struct {
	h *handler
	namespace string
	
	
}
