package rpc

import (
	"errors"
	"time"
)

var (
	ErrBadResult                 = errors.New("bad result in JSON-RPC response")
	ErrClientQuit                = errors.New("client is closed")
	ErrNoResult                  = errors.New("no result in JSON-RPC response")
	ErrSubscriptionQueueOverflow = errors.New("subscription queue overflow")
	errClientReconnected         = errors.New("client reconnected")
	errDead                      = errors.New("connection lost")
)

const (
	//Timeouts
	defaultDialTimeout = 10 * time.Second // used if context has no deadline
	subscribeTimeout   = 5 * time.Second  // overall timeout eth_subscribe, rpc_modules calls
)

const (
	// 当订阅者跟不上时订阅被删除。
	//
	// 这可以通过提供一个具有足够大小缓冲区的通道来解决，
	// 但这可能很不方便并且很难在文档中解释。另一个问题
	// 缓冲通道是缓冲区是静态的，即使它可能不需要
	// 大多数时候。
	//
	// 这里采用的方法是维护一个每个订阅的链表缓冲区
	// 按需收缩。如果缓冲区达到以下大小，则订阅
	// 掉了。
	maxClientSubscriptionBuffer = 20000
)

// BatchElem 是批处理请求中的一个元素。
type BatchElem struct {
	Method string
	Args   []interface{}
	// 结果被解组到这个字段中。结果必须设置为
	// 所需类型的非 nil 指针值，否则响应将是
	// 丢弃。
	Result interface{}
	// 如果服务器为此请求返回错误，或者如果
	// 解组到 Result 失败。它不是为 I/O 错误设置的。
	Error error
}

// Client 表示与 RPC 服务器的连接。
type Client struct {
	idgen func() ID // for subscriptions
	isHTTP bool 
}
