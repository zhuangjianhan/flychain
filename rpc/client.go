package rpc

import (
	"context"
	"encoding/json"
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
	idgen    func() ID // for subscriptions
	isHTTP   bool      // connection type: http, ws or ipc
	services *serviceRegistry

	idCounter uint32

	// 此函数，如果非零，则在连接丢失时调用。
	reconnectFunc reconnectFunc

	// writeConn 用于写入调用者 goroutine 上的连接。它应该
	// 只能在调度之外访问，并持有写锁。写锁是
	// 通过在 reqInit 上发送获取并通过在 reqSent 上发送释放。
	writeConn jsonWriter

	// 用于调度
	close       chan struct{}
	closing     chan struct{}    // 客户端退出时关闭
	didClose    chan struct{}    // 客户端退出时关闭
	reconnected chan ServerCodec // write/reconnect 发送新连接的地方
	readOp      chan readOp      // read messages
	readErr     chan error       // errors from read
	reqInit     chan *requestOp  // 注册响应 ID，获取写锁
	reqSent     chan error       // 写完成信号，释放写锁
	reqTimeout  chan *requestOp  // 当调用超时到期时删除响应 ID
}

type reconnectFunc func(context.Context) (ServerCodec, error)

type clientContextKey struct{}

type clientConn struct {
	codec   ServerCodec
	handler *handler
}

func (cc *clientConn) close(err error, inflightReq *requestOp) {
	cc.handler.close(err, inflightReq)
	cc.codec.close()
}

type readOp struct {
	msgs  []*jsonrpcMessage
	batch bool
}

type requestOp struct {
	ids []json.RawMessage
	err error
	resp chan *jsonrpcMessage // 最多接收 len(ids) 个响应
	sub *ClientSub
}
