package rpc

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
	reg *serviceRe
}