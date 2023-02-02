package event

import (
	"reflect"
	"sync"
)

// FeedOf 实现了一对多的订阅，事件的载体是一个频道。
// 发送到 Feed 的值会同时传送到所有订阅的频道。
//
// 零值可以使用了。
type FeedOf[T any] struct {
	once      sync.Once     // 确保 init 只运行一次
	sendLock  chan struct{} // sendLock 有一个单元素缓冲区，持有时为空。它保护 sendCases。
	removeSub chan chan<- T // 中断发送
	sendCases caseList      // Send 使用的活动选择案例集

	// 收件箱包含新订阅的频道，直到它们被添加到 sendCases。
	mu    sync.Mutex
	inbox caseList
}

func (f *FeedOf[T]) init() {
	f.removeSub = make(chan chan<- T)
	f.sendLock = make(chan struct{}, 1)
	f.sendLock <- struct{}{}
	f.sendCases = caseList{{Chan: reflect.ValueOf(f.removeSub), Dir: reflect.SelectRecv}}
}

// Subscribe 向提要添加一个频道。未来的发送将在频道上传递
// 直到订阅被取消。
//
// 频道应该有足够的缓冲空间，以避免阻塞其他订阅者。慢的
// 订阅者不会被删除。
func (f *FeedOf[T]) Subscribe(channel chan<- T) Subscription {
	f.once.Do(f.init)

	chanval := reflect.ValueOf(channel)
	sub := &feedOfSub[T]{feed: f, channel: channel, err: make(chan error, 1)}

	// 将选择案例添加到收件箱。
	// 下一个 Send 会把它添加到 f.sendCases 中。
	f.mu.Lock()
	defer f.mu.Unlock()
	cas := reflect.SelectCase{Dir: reflect.SelectSend, Chan: chanval}
	f.inbox = append(f.inbox, cas)
	return sub
}

func (f *FeedOf[T]) remove(sub *feedOfSub[T]) {
	// 先从收件箱中删除，覆盖频道
	// 尚未添加到 f.sendCases 中。
	f.mu.Lock()
	index := f.inbox.find(sub.channel)
	if index != -1 {
		f.inbox = f.inbox.delete(index)
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()

	select {
	case f.removeSub <- sub.channel:
		// Send 将从 f.sendCases 中删除通道。
	case <-f.sendLock:
		// 没有发送正在进行中，现在我们有发送锁就​​删除通道。
		f.sendCases = f.sendCases.delete(f.sendCases.find(sub.channel))
		f.sendLock <- struct{}{}
	}
}

// Send 同时传送到所有订阅的频道。
// 它返回值被发送到的订阅者的数量。
func (f *FeedOf[T]) Send(value T) (nsent int) {
	rvalue := reflect.ValueOf(value)

	f.once.Do(f.init)
	<-f.sendLock

	// 获取发送锁后从收件箱添加新案例。
	f.mu.Lock()
	f.sendCases = append(f.sendCases, f.inbox...)
	f.inbox = nil 
	f.mu.Unlock()

	// 在所有通道上设置发送值。
	for i := firstSubSendCase; i < len(f.sendCases); i++ {
		f.sendCases[i].Send = rvalue
	}

	// 发送，直到选择了除 removeSub 之外的所有通道。 'cases' 跟踪前缀
	// 发送案例。当发送成功时，相应的案例移动到结尾
	// 'cases' 并且它缩小了一个元素。
	cases := f.sendCases
	for {
		// 快速路径：在添加到选择集之前尝试不阻塞地发送。
		// 如果订阅者足够快并且有空闲，这通常会成功
		//缓冲空间。
		for i := firstSubSendCase; i < len(cases); i++ {
			if cases[i].Chan.TrySend(rvalue) {
				nsent++
				cases = cases.deactivate(i)
				i--
			}
		}
		if len(cases) == firstSubSendCase {
			break
		}
		// 选择所有接收者，等待他们解锁。
		chosen, recv, _ := reflect.Select(cases)
		if chosen == 0 /* <-f.removeSub */ {
			index := f.sendCases.find(recv.Interface())
			f.sendCases = f.sendCases.delete(index)
			if index >= 0 && index < len(cases) {
				// 也收缩 'cases' 因为移除的 case 仍然有效。
				cases = f.sendCases[:len(cases)-1]
			}
		} else {
			cases = cases.deactivate(chosen)
			nsent++
		}
	}

	// 忘记发送的值并移交发送锁。
	for i := firstSubSendCase; i < len(f.sendCases); i++ {
		f.sendCases[i].Send = reflect.Value{}
	}
	f.sendLock <- struct{}{}
	return nsent
}

type feedOfSub[T any] struct {
	feed    *FeedOf[T]
	channel chan<- T
	errOnce sync.Once
	err     chan error
}

func (sub *feedOfSub[T]) Unsubscribe() {
	sub.errOnce.Do(func() {
		sub.feed.remove(sub)
		close(sub.err)
	})
}

func (sub *feedOfSub[T]) Err() <-chan error {
	return sub.err
}


