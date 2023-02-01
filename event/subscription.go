package event

import (
	"context"
	"flychain/common/mclock"
	"sync"
	"time"
)

// 订阅表示事件流。事件的载体通常是
// 通道，但不是接口的一部分。
//
// 订阅在建立时可能会失败。通过错误报告失败
// 渠道。如果订阅有问题，它会收到一个值（例如
// 传送事件的网络连接已关闭）。只有一个值永远是
// 发送。
//
// 错误通道在订阅成功结束时关闭（即当
// 事件源已关闭）。当调用 Unsubscribe 时它也会关闭。
//
// Unsubscribe 方法取消事件的发送。您必须全部调用退订
// 确保与订阅相关的资源被释放的案例。有可能
// 调用任意次数。
type Subscription interface {
	Err() <-chan error // return the error channel
	Unsubscribe()      // 取消发送事件，关闭错误通道
}

// NewSubscription 在新的 goroutine 中运行一个生产者函数作为订阅。这
// 当取消订阅被调用时，提供给生产者的通道被关闭。如果 fn 返回一个
// 错误，它在订阅的错误通道上发送。
func NewSubscription(producer func(<-chan struct{}) error) Subscription {
	s := &funSub{unsub: make(chan struct{}), err: make(chan error, 1)}
	go func() {
		defer close(s.err)
		err := producer(s.unsub)
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.unsubscribed {
			if err != nil {
				s.err <- err
			}
			s.unsubscribed = true
		}
	}()
	return s
}

type funSub struct {
	unsub        chan struct{}
	err          chan error
	mu           sync.Mutex
	unsubscribed bool
}

func (s *funSub) Unsubscribe() {
	s.mu.Lock()
	if s.unsubscribed {
		s.mu.Unlock()
		return
	}
	s.unsubscribed = true
	close(s.unsub)
	s.mu.Unlock()
	// 等待生产者关闭。
	<-s.err
}

func (s *funSub) Err() <-chan error {
	return s.err
}

// Resubscribe 反复调用 fn 以保持订阅已建立。当。。。的时候
// 订阅建立，重新订阅等待失败，再次调用fn。这个
// 过程重复直到取消订阅被调用或活动订阅结束
// 成功地。
//
// 重新订阅在对 fn 的调用之间应用回退。调用之间的时间被调整
// 基于错误率，但永远不会超过 backoffMax。
func Resubscribe(backoffMax time.Duration, fn ResubscribeFunc) Subscription {
	return ResubscribeErr(backoffMax, func(ctx context.Context, _ error) (Subscription, error) {
		return fn(ctx)
	})
}

// ResubscribeFunc 尝试建立订阅。
type ResubscribeFunc func(context.Context) (Subscription, error)

// ResubscribeErr 重复调用 fn 以保持订阅已建立。当。。。的时候
// 订阅建立，ResubscribeErr 等待失败，再次调用 fn。这个
// 过程重复直到取消订阅被调用或活动订阅结束
// 成功地。
//
// Resubscribe 和 ResubscribeErr 的区别在于，有了 ResubscribeErr，
// 订阅失败的错误可用于回调进行记录
// 目的。
//
// ResubscribeErr 在对 fn 的调用之间应用回退。调用之间的时间被调整
// 基于错误率，但永远不会超过 backoffMax。
func ResubscribeErr(backoffMax time.Duration, fn ResubscribeErrFunc) Subscription {
	s := &resubscribeSub{
		waitTime:   backoffMax / 10,
		backoffMax: backoffMax,
		fn:         fn,
		err:        make(chan error),
		unsub:      make(chan struct{}),
	}
	go s.loop()
	return s
}

// ResubscribeErrFunc 尝试建立订阅。
// 对于除第一次调用之外的每次调用，此函数的第二个参数是
// 之前订阅发生的错误。
type ResubscribeErrFunc func(context.Context, error) (Subscription, error)

type resubscribeSub struct {
	fn                   ResubscribeErrFunc
	err                  chan error
	unsub                chan struct{}
	unsubOnce            sync.Once
	lastTry              mclock.AbsTime
	lastSubErr           error
	waitTime, backoffMax time.Duration
}

func (s *resubscribeSub) Unsubscribe() {
	s.unsubOnce.Do(func() {
		s.unsub <- struct{}{}
		<-s.err
	})
}

func (s *resubscribeSub) Err() <-chan error {
	return s.err
}

func (s *resubscribeSub) loop() {
	defer close(s.err)
	var done bool
	for !done {
		sub := s.subscribe()
		if sub == nil {
			break
		}
		done = s.waitForError(sub)
		sub.Unsubscribe()
	}
}
 
func (s *resubscribeSub) subscribe() Subscription {
	subscribed := make(chan error)
	var sub Subscription
	for {
		s.lastTry = mclock.Now()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			rsub, err := s.fn(ctx, s.lastSubErr)
			sub = rsub
			subscribed <- err
		}()
		select {
		case err := <-subscribed:
			cancel()
			if err == nil {
				if sub == nil {
					panic("event: ResubscribeFunc returned nil subscription and no error")
				}
				return sub
			}
			// 订阅失败，等待下一次尝试。
			if s.backoffWait() {
				return nil// 等待期间取消订阅
			}
		case <-s.unsub:
			cancel()
			<-subscribed// 避免泄露 s.fn goroutine。
			return nil
		}
	}
}

func (s *resubscribeSub) waitForError(sub Subscription) bool {
	defer sub.Unsubscribe()
	select {
	case err := <-sub.Err():
		s.lastSubErr = err
		return err == nil
	case <-s.unsub:
		return true
	}
}

func (s *resubscribeSub) backoffWait() bool {
	if time.Duration(mclock.Now()-s.lastTry) > s.backoffMax {
		s.waitTime = s.backoffMax / 10
	} else {
		s.waitTime *= 2
		if s.waitTime > s.backoffMax {
			s.waitTime = s.backoffMax
		}
	}

	t := time.NewTimer(s.waitTime)
	defer t.Stop()
	select {
	case <-t.C:
		return false
	case <-s.unsub:
		return true
	}
}

// SubscriptionScope 提供了一次取消订阅多个订阅的工具。
//
// 对于处理多个订阅的代码，可以方便地使用作用域
// 通过一次调用取消订阅所有这些。该示例演示了在
// 更大的程序。
//
// 零值可以使用了。
type SubscriptionScope struct {
	mu sync.Mutex
	subs map[*scopeSub]struct{}
	closed bool 
}

type scopeSub struct {
	sc *SubscriptionScope
	s Subscription
}

// Track 开始跟踪一个订阅。如果作用域关闭，Track 返回 nil。这
// 返回的订阅是一个包装器。取消订阅包装器会将其从
// 范围。
func (sc *SubscriptionScope) Track(s Subscription) Subscription {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.closed {
		return nil
	}
	if sc.subs == nil {
		sc.subs = make(map[*scopeSub]struct{})
	}
	ss := &scopeSub{sc, s}
	sc.subs[ss] = struct{}{}
	return ss
}

// 关闭调用取消订阅所有跟踪的订阅并阻止进一步添加
// 跟踪集。 Close 后调用 Track 返回 nil。
func (sc *SubscriptionScope) Close() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.closed {
		return
	}
	sc.closed = true
	for s := range sc.subs {
		s.s.Unsubscribe()
	}
	sc.subs = nil
}

// Count 返回跟踪订阅的数量。
// 它是用来调试的。
func (sc *SubscriptionScope) Count() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.subs)
}

func (s *scopeSub) Unsubscribe() {
	s.s.Unsubscribe()
	s.sc.mu.Lock()
	defer s.sc.mu.Unlock()
	delete(s.sc.subs, s)
}

func (s *scopeSub) Err() <-chan error {
	return s.s.Err()
}
