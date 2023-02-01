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

func(e feedTypeError) Error() string {
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

