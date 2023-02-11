package rpc

import (
	"container/list"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
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

// NewID returns a new, random ID.
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
	h         *handler
	namespace string

	mu           sync.Mutex
	sub          *Subscription
	buffer       []json.RawMessage
	callReturned bool
	activated    bool
}

// CreateSubscription 返回耦合到
// RPC 连接。默认情况下，订阅处于非活动状态并且通知
// 被丢弃，直到订阅被标记为活动。这个做完了
// 在订阅 ID 发送给客户端后，由 RPC 服务器执行。
func (n *Notifier) CreateSubscription() *Subscription {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.sub != nil {
		panic("can't create multiple subscriptions with Notifier")
	} else if n.callReturned {
		panic("can't create subscription after subscribe call has returned")
	}
	n.sub = &Subscription{ID: n.h.idgen(), namespace: n.namespace, err: make(chan error, 1)}
	return n.sub
}

// Notify 将给定数据作为有效负载发送给客户端通知。
// 如果发生错误，RPC 连接将关闭并返回错误。
func (n *Notifier) Notify(id ID, data interface{}) error {
	enc, err := json.Marshal(data)
	if err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.sub == nil {
		panic("can't Notify before subscription is created")
	} else if n.sub.ID != id {
		panic("Notify with wrong ID")
	}
	if n.activated {
		return n.send(n.sub, enc)
	}
	n.buffer = append(n.buffer, enc)
	return nil
}

// Closed 返回一个在 RPC 连接关闭时关闭的通道。
// 弃用：使用订阅错误通道
func (n *Notifier) Closed() <-chan interface{} {
	return n.h.conn.closed
}

// takeSubscription 返回订阅（如果已经创建）。没有订阅可以
// 在此调用后创建。
func (n *Notifier) takeSubscription() *Subscription {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callReturned = true
	return n.sub
}

// 订阅 ID 发送到客户端后调用激活。通知是
// 激活前缓冲。这可以防止通知被发送到客户端之前
// 订阅 ID 被发送到客户端。
func (n *Notifier) activate() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, data := range n.buffer {
		if err := n.send(n.sub, data); err != nil {
			return err
		}
	}
	n.activated = true
	return nil
}

func (n *Notifier) send(sub *Subscription, data json.RawMessage) error {
	params, _ := json.Marshal(&subscriptionResult{ID: string(sub.ID), Result: data})
	ctx := context.Background()

	msg := &jsonrpcMessage{
		Version: vsn,
		Method: n.namespace + notificationMethodSuffix,
		Params: params,
	}
	return n.h.conn.writeJSON(ctx, msg, false)
}

// 订阅由通知程序创建并绑定到该通知程序。客户端可以使用
// 此订阅等待客户端的取消订阅请求，请参阅 Err()。
type Subscription struct {
	ID        ID
	namespace string
	err       chan error // 取消订阅时关闭
}

// Err 返回一个通道，该通道在客户端发送退订请求时关闭。
func (s *Subscription) Err() <-chan error {
	return s.err
}

// MarshalJSON 将订阅编组为其 ID。
func (s *Subscription) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.ID)
}

// ClientSubscription是通过Client的Subscribe建立的订阅或者
// EthSubscribe 方法。
type ClientSubscription struct {
	client    *Client
	etype     reflect.Type
	channel   reflect.Value
	namespace string
	subid     string

	// in 通道从客户端调度程序接收通知值。
	in chan json.RawMessage

	// 错误通道从转发循环接收错误。
	// 它被取消订阅关闭。
	err     chan error
	errOnce sync.Once

	// 通过发送 'quit' 请求关闭订阅。这是由处理
	//转发循环，当它停止发送到时关闭'forwardDone'
	// 子频道。最后，'unsubDone' 在服务器端取消订阅后关闭。
	quit        chan error
	forwardDone chan struct{}
	unsubDone   chan struct{}
}

var errUnsubscribed = errors.New("unsubscribed")

func newClientSubscription(c *Client, namespace string, channel reflect.Value) *ClientSubscription {
	sub := &ClientSubscription{
		client:      c,
		namespace:   namespace,
		etype:       channel.Type().Elem(),
		channel:     channel,
		in:          make(chan json.RawMessage),
		quit:        make(chan error),
		forwardDone: make(chan struct{}),
		unsubDone:   make(chan struct{}),
		err:         make(chan error),
	}
	return sub
}

// Err返回订阅错误通道。Err的预期用途是计划
// 当客户端连接意外关闭时重新订阅。
//
// 当订阅由于错误而结束时，错误通道会收到一个值。这个
// 如果在基础客户端上调用了Close，则收到的错误为零
// 发生错误。
//
// 当对订阅调用Unsubscribe时，错误通道关闭。
func (sub *ClientSubscription) Err() <-chan error {
	return sub.err
}

// 取消订阅取消订阅通知并关闭错误通道。
// 它可以安全地调用多次。
func (sub *ClientSubscription) Unsubscribe() {
	sub.errOnce.Do(func() {
		select {
		case sub.quit <- errUnsubscribed:
			<-sub.unsubDone
		case <-sub.unsubDone:
		}
		close(sub.err)
	})
}

// 客户端的消息分派器调用delivery来发送通知值。
func (sub *ClientSubscription) deliver(result json.RawMessage) (ok bool) {
	select {
	case sub.in <- result:
		return true
	case <-sub.forwardDone:
		return false
	}
}

// close is called by the client's message dispatcher when the connection is closed.
func (sub *ClientSubscription) close(err error) {
	select {
	case sub.quit <- err:
	case <-sub.forwardDone:
	}
}

// run是订阅的转发循环。它运行在自己的goroutine中
// 在创建订阅后由客户端的处理程序启动。
func (sub *ClientSubscription) run() {
	defer close(sub.unsubDone)

	unsubscribe, err := sub.forward()

	// 如果是，客户端的调度循环将无法执行取消订阅调用
	// 在 sub.deliver() 或 sub.close() 中阻塞。关闭 forwardDone 解除阻塞。
	close(sub.forwardDone)

	// 调用服务端的取消订阅方法。
	if unsubscribe {
		sub.requestUnsubscribe()
	}

	// Send the error
	if err != nil {
		if err == ErrClientQuit {
			// 调用 Client.Close 时，ErrClientQuit 到达此处。据报道这是一个
			// nil error 因为它不是错误，但是我们不能在这里关闭 sub.err。
			err = nil
		}
		sub.err <- err
	}
}

// forward是转发循环。它接收RPC通知并发送它们
// 在订阅频道上。
func (sub *ClientSubscription) forward() (unsubscribeServer bool, err error) {
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.quit)},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(sub.in)},
		{Dir: reflect.SelectSend, Chan: sub.channel},
	}
	buffer := list.New()

	for {
		var chosen int
		var recv reflect.Value
		if buffer.Len() == 0 {
			// 空闲，省略发送案例。
			chosen, recv, _ = reflect.Select(cases[:2])
		} else {
			// 非空缓冲区，发送第一个排队的项目。
			cases[2].Send = reflect.ValueOf(buffer.Front().Value)
			chosen, recv, _ = reflect.Select(cases)
		}

		switch chosen {
		case 0: // <-sub.quit
			if !recv.IsNil() {
				err = recv.Interface().(error)
			}
			if err == errUnsubscribed {
				// 退出因为取消订阅被调用，在服务器上取消订阅。
				return true, nil
			}
			return false, err
		case 1: // <-sub.in
			val, err := sub.unmarshal(recv.Interface().(json.RawMessage))
			if err != nil {
				return true, err
			}
			if buffer.Len() == maxClientSubscriptionBuffer {
				return true, ErrSubscriptionQueueOverflow
			}
			buffer.PushBack(val)

		case 2: // sub.channel<-
			cases[2].Send = reflect.Value{}
			buffer.Remove(buffer.Front())
		}
	}
}

func (sub *ClientSubscription) unmarshal(result json.RawMessage) (interface{}, error) {
	val := reflect.New(sub.etype)
	err := json.Unmarshal(result, val.Interface())
	return val.Elem().Interface(), err
}

func (sub *ClientSubscription) requestUnsubscribe() error {
	var result interface{}
	return sub.client.Call()
}
