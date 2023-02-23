package rpc

import (
	"context"
	"encoding/json"
	"log"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 处理程序处理 JSON-RPC 消息。每个连接有一个处理程序。注意
// 处理程序对于并发使用是不安全的。消息处理永远不会无限期地阻塞
// 因为 RPC 是在处理程序启动的后台 goroutines 上处理的。
//
// The entry points for incoming messages are:
//
//	h.handleMsg(message)
//	h.handleBatch(message)
//
// Outgoing calls use the requestOp struct. Register the request before sending it
// on the connection:
//
//	op := &requestOp{ids: ...}
//	h.addRequestOp(op)
//
// Now send the request, then wait for the reply to be delivered through handleMsg:
//
//	if err := op.wait(...); err != nil {
//		h.removeRequestOp(op) // timeout, etc.
//	}
type handler struct {
	reg            *serviceRegistry
	unsubscribeCb  *callback
	idgen          func() ID                      // subscription ID generator
	respWait       map[string]*requestOp          // 活跃的客户端请求
	clientSubs     map[string]*ClientSubscription // 活跃的客户端订阅
	CallWG         sync.WaitGroup                 // 挂起的调用 goroutines
	rootGtx        context.Context                // 被 close() 取消
	cancelRoot     func()                         // rootCtx 的取消函数
	conn           jsonWriter                     // 响应将发送到哪里
	log            log.Logger
	allowSubscribe bool

	subLock    sync.Mutex
	serverSubs map[ID]*Subscription
}

type callProc struct {
	ctx       context.Context
	notifiers []*Notifier
}

func NewHandler(connCtx context.Context, conn jsonWriter, idgen func() ID, reg *serviceRegistry) *handler {
	rootCtx, cancelRoot := context.WithCancel(connCtx)
	h := &handler{
		reg:            reg,
		idgen:          idgen,
		conn:           conn,
		respWait:       make(map[string]*requestOp),
		clientSubs:     make(map[string]*ClientSubscription),
		rootGtx:        rootCtx,
		cancelRoot:     cancelRoot,
		allowSubscribe: true,
		serverSubs:     make(map[ID]*Subscription),
		log:            log.Root(),
	}
	if conn.remoteAddr() != "" {
		h.log = h.log.New("conn", conn.remoteAddr())
	}
	h.unsubscribeCb = newCallback(reflect.Value{}, reflect.ValueOf(h.unsubscribe))
	return h
}

// batchCallBuffer 管理正在进行的调用消息及其在批处理期间的响应
// 称呼。处理和超时触发之间需要同步调用
// 协程。
type batchCallBuffer struct {
	mutex sync.Mutex
	calls []*jsonrpcMessage
	resp  []*jsonrpcMessage
	wrote bool
}

// nextCall 返回下一条未处理的消息。
func (b *batchCallBuffer) nextCall() *jsonrpcMessage {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.calls) == 0 {
		return nil
	}
	// 弹出发生在 `pushAnswer` 中。保留正在进行的通话
	// 所以我们可以在超时的情况下为它返回一个错误。
	msg := b.calls[0]
	return msg
}

// pushResponse 添加对 nextCall 返回的最后一次调用的响应。
func (b *batchCallBuffer) pushResponse(answer *jsonrpcMessage) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if answer != nil {
		b.resp = append(b.resp, answer)
	}

	b.calls = b.calls[1:]
}

// 超时发送到目前为止添加的响应。对于剩余的未接电话
// 消息，它发送超时错误响应。
func (b *batchCallBuffer) timeout(ctx context.Context, conn jsonWriter) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, msg := range b.calls {
		if !msg.isNotification() {
			resp := msg.errResponse(&internalServerError{errcodeTimeout, errMsgTimeout})
			b.resp = append(b.resp, resp)
		}
	}
	b.doWrite(ctx, conn, true)
}

// doWrite 实际上写响应。
// 这假设 b.mutex 被持有。
func (b *batchCallBuffer) doWrite(ctx context.Context, conn jsonWriter, isErrorResponse bool) {
	if b.wrote {
		return
	}
	b.wrote = true // can only write once
	if len(b.resp) > 0 {
		conn.writeJSON(ctx, b.resp, isErrorResponse)
	}
}

// handleBatch 批量执行所有消息并返回响应。
func (h *handler) handleBatch(msgs []*jsonrpcMessage) {
	// 为空批发出错误响应：
	if len(msgs) == 0 {

	}
}

// handleMsg 处理单个消息。
func (h *handler) handleMsg(msg *jsonrpcMessage) {
	//if ok := h.handleIm
}

// close 取消除 inflightReq 之外的所有请求并等待
// 调用 goroutines 关闭。
func (h *handler) close(err error, inflightReq *requestOp) {
	h.cancelAllRequests(err, inflightReq)
	h.CallWG.Wait()
	h.cancelRoot()
	h.cancelServerSubscriptions(err)
}

// addRequestOp 注册请求操作。
func (h *handler) addRequestOp(op *requestOp) {
	for _, id := range op.ids {
		h.respWait[string(id)] = op
	}
}

// removeRequestOp 停止等待给定的请求 ID。
func (h *handler) removeRequestOp(op *requestOp) {
	for _, id := range op.ids {
		delete(h.respWait, string(id))
	}
}

// cancelAllRequests 取消阻塞并删除挂起的请求和活动订阅。
func (h *handler) cancelAllRequests(err error, infligntReq *requestOp) {
	didclose := make(map[*requestOp]bool)
	if infligntReq != nil {
		didclose[infligntReq] = true
	}

	for id, op := range h.respWait {
		// 删除 op 以便以后的调用不会再次关闭 op.resp。
		delete(h.respWait, id)

		if !didclose[op] {
			op.err = err
			close(op.resp)
			didclose[op] = true
		}
	}
	for id, sub := range h.clientSubs {
		delete(h.clientSubs, id)
		sub.close(err)
	}
}

func (h *handler) addSubscriptions(nn []*Notifier) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	for _, n := range nn {
		if sub := n.takeSubscription(); sub != nil {
			h.serverSubs[sub.ID] = sub
		}
	}
}

// cancelServerSubscriptions 删除所有订阅并关闭它们的错误通道。
func (h *handler) cancelServerSubscriptions(err error) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	for id, s := range h.serverSubs {
		s.err <- err
		close(s.err)
		delete(h.serverSubs, id)
	}
}

// startCallProc 在一个新的 goroutine 中运行 fn 并开始在 h.calls 等待组中跟踪它。
func (h *handler) startCallProc(fn func(*callProc)) {
	h.CallWG.Add(1)
	go func() {
		ctx, cancel := context.WithCancel(h.rootGtx)
		defer h.CallWG.Done()
		defer cancel()
		fn(&callProc{ctx: ctx})
	}()
}

// handleImmediate 执行非调用消息。如果消息是一个调用或需要回复，它返回 false
func (h *handler) handlerImmediate(msg *jsonrpcMessage) bool {
	start := time.Now()
	switch {
	case msg.isNotification():
		if strings.HasSuffix(msg.Method, notificationMethodSuffix) {
			h.handleSubscriptionResult(msg)
			return true
		}
	case msg.isResponse():
		h.handleResponse(msg)
		h.log.Trace("Handled RPC response", "reqid", idForLog{msg.ID}, "duration", time.Since(start))
		return true
	default:
		return false
	}
}

// handleSubscriptionResult 处理订阅通知。
func (h *handler) handleSubscriptionResult(msg *jsonrpcMessage) {
	var result subscriptionResult
	if err := json.Unmarshal(msg.Params, &result); err != nil {
		h.log.Debug("Dropping invalid subscription message")
		return
	}
	if h.clientSubs[result.ID] != nil {
		h.clientSubs[result.ID].deliver(result.Result)
	}
}

// handleResponse 处理方法调用响应。
func (h *handler) handleResponse(msg *jsonrpcMessage) {
	op := h.respWait[string(msg.ID)]
	if op == nil {
		h.log.Debug("Unsolicited RPC response", "reqid", idForLog{msg.ID})
		return
	}
	delete(h.respWait, string(msg.ID))
	// 对于正常响应，只需将响应转发给 Call/BatchCall。
	if op.sub == nil {
		op.resp <- msg
		return 
	}
	// 对于订阅响应，如果服务器启动订阅
	//表示成功。 EthSubscribe 在任何一种情况下都可以通过
	// op.resp 通道。
	defer close(op.resp)
	if msg.Error != nil {
		op.err = msg.Error
	}
	if op.err = json.Unmarshal(msg.Result, &op.sub.subid); op.err != nil {
		go op.sub.run()
		h.clientSubs[op.sub.subid] = op.sub
	}
}

// handleSubscribe 处理 *_subscribe 方法调用。
func (h *handler) handleSubscribe(cp *callProc, msg *jsonrpcMessage) *jsonrpcMessage {
	if !h.allowSubscribe {
		return msg.errResponse(&internalServerError{
			code: errcodeNotificationsUnsupported,
			message: ErrNotificationsUnsupported.Error(),
		})
	}

	// 订阅方法名称是第一个参数。
	//name, err := parseSub
}

// runMethod 运行 RPC 方法的 Go 回调。
func (h *handler) runMethod(ctx context.Context, msg *jsonrpcMessage, callb *callback, args []reflect.Value) *jsonrpcMessage {
	result, err := callb.call(ctx, msg.Method, args) 
	if err != nil {
		return msg.errResponse(err)
	}
	return msg.response(result)
}

// unsubscribe 是所有 *_unsubscribe 调用的回调函数。
func (h *handler) unsubscribe(ctx context.Context, id ID) (bool, error) {
	h.subLock.Lock()
	defer h.subLock.Unlock()

	s := h.serverSubs[id]
	if s == nil {
		return false, ErrSubscriptionNotFound
	}
	close(s.err)
	delete(h.serverSubs, id)
	return true, nil
}

type idForLog struct{ json.RawMessage }

func (id idForLog) String() string {
	if s, err := strconv.Unquote(string(id.RawMessage)); err == nil {
		return s
	}
	return string(id.RawMessage)
}
