package event

import (
	"errors"
	"reflect"
	"sync"
)

var errBadChannel = errors.New("event: Subscribe argument does not have sendable channel type")

// Feed 实现了一对多的订阅，事件的载体是一个频道。
// 发送到 Feed 的值会同时传送到所有订阅的频道。
//
// Feeds 只能用于单一类型。类型由第一个 Send 或
// 订阅操作。如果类型不正确，则对这些方法的后续调用会出现恐慌
// 匹配。
//
// 零值可以使用了。
type Feed struct {
	once      sync.Once        // 确保 init 只运行一次
	sendLock  chan struct{}    // sendLock 有一个单元素缓冲区，持有时为空。它保护 sendCases。
	removeSub chan interface{} // 中断发送
	sendCases caseList         // Send 使用的活动选择案例集

	// 收件箱包含新订阅的频道，直到它们被添加到 sendCases。
	mu    sync.Mutex
	inbox caseList
	etype reflect.Type
}

// 这是 sendCases 中第一个实际订阅频道的索引。
// sendCases[0] 是 removeSub 通道的 SelectRecv case。
const firstSubSendCase = 1

type feedTypeError struct {
	got, want reflect.Type
	op        string
}

func (e feedTypeError) Error() string {
	return "event: wrong type in " + e.op + " got " + e.got.String() + ", want " + e.want.String()
}

func (f *Feed) init() {
	f.removeSub = make(chan interface{})
	f.sendLock = make(chan struct{}, 1)
	f.sendLock <- struct{}{}
	f.sendCases = caseList{{Chan: reflect.ValueOf(f.removeSub), Dir: reflect.SelectRecv}}
}

// Subscribe 向提要添加一个频道。未来的发送将在频道上传递
// 直到订阅被取消。添加的所有通道必须具有相同的元素类型。
//
// 频道应该有足够的缓冲空间，以避免阻塞其他订阅者。
// 慢速订阅者不会被丢弃。
func (f *Feed) Subscribe(channel interface{}) Subscription {
	f.once.Do(f.init)

	chanval := reflect.ValueOf(channel)
	chantyp := chanval.Type()
	if chantyp.Kind() != reflect.Chan || chantyp.ChanDir()&reflect.SendDir == 0 {
		panic(errBadChannel)
	}
	sub := &feedSub{feed: f, channel: chanval, err: make(chan error, 1)}

	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.typecheck(chantyp.Elem()) {
		panic(feedTypeError{op: "Subscribe", got: chantyp, want: reflect.ChanOf(reflect.SendDir, f.etype)})
	}
	// 将选择案例添加到收件箱。
	// 下一个 Send 会把它添加到 f.sendCases 中。
	cas := reflect.SelectCase{Dir: reflect.SelectSend, Chan: chanval}
	f.inbox = append(f.inbox, cas)
	return sub
}

// 注意：调用者必须持有 f.mu
func (f *Feed) typecheck(typ reflect.Type) bool {
	if f.etype == nil {
		f.etype = typ
		return true
	}
	return f.etype == typ
}

func (f *Feed) remove(sub *feedSub) {
	// 先从收件箱中删除，覆盖频道
	// 尚未添加到 f.sendCases 中。
	ch := sub.channel.Interface()
	f.mu.Lock()
	index := f.inbox.find(ch)
	if index != -1 {
		f.inbox = f.inbox.delete(index)
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()

	select {
	case f.removeSub <- ch:
		// Send 将从 f.sendCases 中删除通道。
	case <-f.sendLock:
		// 没有发送正在进行中，现在我们有发送锁就​​删除通道。
		f.sendCases = f.sendCases.delete(f.sendCases.find(ch))
		f.sendLock <- struct{}{}
	}
}

// Send 同时传送到所有订阅的频道。
// 它返回值被发送到的订阅者的数量。
func (f *Feed) Send(value interface{}) (nsent int) {
	rvalue := reflect.ValueOf(value)

	f.once.Do(f.init)
	<-f.sendLock

	// 获取发送锁后从收件箱添加新案例。
	f.mu.Lock()
	f.sendCases = append(f.sendCases, f.inbox...)
	f.inbox = nil

	if !f.typecheck(rvalue.Type()) {
		f.sendLock <- struct{}{}
		f.mu.Unlock()
		panic(feedTypeError{op: "Send", got: rvalue.Type(), want: f.etype})
	}
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

type feedSub struct {
	feed    *Feed
	channel reflect.Value
	errOnce sync.Once
	err     chan error
}

func (sub *feedSub) Unsubscribe() {
	sub.errOnce.Do(func() {
		sub.feed.remove(sub)
		close(sub.err)
	})
}

func (sub *feedSub) Err() <-chan error {
	return sub.err
}

type caseList []reflect.SelectCase

// find 返回包含给定通道的案例的索引。
func (cs caseList) find(channel interface{}) int {
	for i, cas := range cs {
		if cas.Chan.Interface() == channel {
			return i
		}
	}
	return -1
}

// delete 从 cs 中移除给定的 case。
func (cs caseList) delete(index int) caseList {
	return append(cs[:index], cs[:index+1]...)
}

// deactivate 将索引处的案例移动到 cs 切片的不可访问部分。
func (cs caseList) deactivate(index int) caseList {
	last := len(cs) - 1
	cs[index], cs[last] = cs[last], cs[index]
	return cs[:last]
}
