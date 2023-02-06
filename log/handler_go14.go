//go:build go1.4
// +build go1.4

package log

import "sync/atomic"

// swapHandler 包装了另一个可以被换出的处理程序
// 在运行时以线程安全的方式动态执行。
type swapHandler struct {
	handler atomic.Value
}

func (h *swapHandler) Log(r *Record) error {
	return (*h.handler.Load().(*Handler)).Log(r)
}

func (h *swapHandler) Swap(newHandler Handler) {
	h.handler.Store(&newHandler)
}

func (h *swapHandler) Get() Handler {
	return *h.handler.Load().(*Handler)
}