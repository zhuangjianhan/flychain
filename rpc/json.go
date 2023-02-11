package rpc

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	vsn                      = "2.0"
	serviceMethodSeparator   = "_"
	subscribeMethodSuffix    = "_subscribe"
	unsubscribeMethodSuffix  = "_unsubscribe"
	notificationMethodSuffix = "_subscription"

	defaultWriteTimeout = 10 * time.Second // used if context has no deadline
)

var null = json.RawMessage("null")

type subscriptionResult struct {
	ID     string          `json:"subscription"`
	Result json.RawMessage `json:"result,omitempty"`
}

// 这种类型的值可以是 JSON-RPC 请求、通知、成功响应或
// 错误响应。它是哪一个取决于字段。
type jsonrpcMessage struct {
	Version string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method, omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *jsonError      `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func (msg *jsonrpcMessage) isNotification() bool {
	return msg.hasValidVersion() && msg.ID == nil && msg.Method != ""
}

func (msg *jsonrpcMessage) isCall() bool {
	return msg.hasValidVersion() && msg.hasValidID() && msg.Method != ""
}

func (msg *jsonrpcMessage) isResponse() bool {
	return msg.hasValidVersion() && msg.hasValidID() && msg.Method == "" && msg.Params == nil && (msg.Result != nil || msg.Error != nil)
}

func (msg *jsonrpcMessage) hasValidID() bool {
	return len(msg.ID) > 0 && msg.ID[0] != '{' && msg.ID[0] != '['
}

func (msg *jsonrpcMessage) hasValidVersion() bool {
	return msg.Version == vsn
}

func (msg *jsonrpcMessage) isSubscribe() bool {
	return strings.HasSuffix(msg.Method, subscribeMethodSuffix)
}

func (msg *jsonrpcMessage) isUnsubscribe() bool {
	return strings.HasSuffix(msg.Method, unsubscribeMethodSuffix)
}

func (msg *jsonrpcMessage) namespace() string {
	elem := strings.SplitN(msg.Method, serviceMethodSeparator, 2)
	return elem[0]
}

func (msg *jsonrpcMessage) String() string {
	b, _ := json.Marshal(msg)
	return string(b)
}

func (msg *jsonrpcMessage) errResponse(err error) *jsonrpcMessage {
	resp := errorMessage(err)
	resp.ID = msg.ID
	return resp
}

func (msg *jsonrpcMessage) response(result interface{}) *jsonrpcMessage {
	enc, err := json.Marshal(result)
	if err != nil {
		return msg.errResponse(&internalServerError{errcodeMarshalError, err.Error()})
	}
	return &jsonrpcMessage{Version: vsn, ID: msg.ID, Result: enc}
}

func errorMessage(err error) *jsonrpcMessage {
	msg := &jsonrpcMessage{Version: vsn, ID: null, Error: &jsonError{
		Code:    errcodeDefault,
		Message: err.Error(),
	}}
	ec, ok := err.(Error)
	if ok {
		msg.Error.Code = ec.ErrorCode()
	}
	de, ok := err.(DataError)
	if ok {
		msg.Error.Data = de.ErrorData()
	}
	return msg
}

type jsonError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (err *jsonError) Error() string {
	if err.Message == "" {
		return fmt.Sprintf("json-rpc error %d", err.Code)
	}
	return err.Message
}

func (err *jsonError) ErrorCode() int {
	return err.Code
}

func (err *jsonError) ErrorData() interface{} {
	return err.Data
}

// Conn 是 net.Conn 方法的子集，这些方法足以用于 ServerCodec。
type Conn interface {
	io.ReadWriteCloser
	SetWriteDeadline(time.Time) error
}

type deadlineCloser interface {
	io.Closer
	SetWriteDeadline(time.Time) error
}

// ConnRemoteAddr 包装 RemoteAddr 操作，它返回一个描述
// 连接的对等地址。如果一个 Conn 也实现了 ConnRemoteAddr，这个
// 描述用于日志消息。
type ConnRemoteAddr interface {
	RemoteAddr() string
}

// jsonCodec 读取 JSON-RPC 消息并将其写入底层连接。它也有
// 支持解析参数和序列化（结果）对象。
type jsonCodec struct {
	remote  string
	closer  sync.Once        // close closed channel once
	closeCh chan interface{} // closed on Close
	decode  decodeFunc       //解码器允许多重传输
	encMu   sync.Mutex       //保护编码器
	encode  encodeFunc       // 允许多重传输的编码器
	conn    deadlineCloser
}

type encodeFunc = func(v interface{}, isErrorResponse bool) error

type decodeFunc = func(v interface{}) error

// NewFuncCodec 创建一个使用给定函数进行读写的编解码器。如果连接
// 实现 ConnRemoteAddr，日志消息将使用它来包含远程地址
// 连接。
func NewFuncCodec(conn deadlineCloser, encode encodeFunc, decode decodeFunc) ServerCodec {
	codec := &jsonCodec{
		closeCh: make(chan interface{}),
		encode: encode,
		decode: decode,
		conn: conn,
	}
	if ra, ok := conn.(ConnRemoteAddr); ok {
		codec.remote = ra.RemoteAddr()
	}
	return codec
}


func (c *jsonCodec) peerInfo() 

func 