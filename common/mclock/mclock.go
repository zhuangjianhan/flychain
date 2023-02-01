package mclock

import (
	"time"
	_ "unsafe" //for go:linkname
)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// AbsTime 表示绝对单调时间。
type AbsTime int64

// 现在返回当前的绝对单调时间。
func Now() AbsTime {
	return AbsTime(nanotime())
}

// 添加返回 t + d 作为绝对时间。
func (t AbsTime) Add(d time.Duration) AbsTime {
	return t + AbsTime(d)
}

// Sub 返回 t - t2 作为持续时间。
func (t AbsTime) Sub(t2 AbsTime) time.Duration {
	return time.Duration(t - t2)
}

// Clock 接口可以用
// 模拟时钟。
type Clock interface {
	Now() AbsTime
	Sleep(time.Duration)
	NewTimer(time.Duration) ChanTimer
	After(time.Duration) <-chan AbsTime
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer 是由 AfterFunc 创建的可取消事件。
type Timer interface {
	// Stop 取消定时器。如果计时器已经完成，则返回 false
	// 过期或已停止。
	Stop() bool
}

// ChanTimer 是由 NewTimer 创建的可取消事件。
type ChanTimer interface {
	Timer

	// 当计时器到期时，C 返回的通道会收到一个值。
	C() <-chan AbsTime
	// Reset 使用新的超时重新安排计时器。
	// 它应该只在具有耗尽通道的停止或过期的计时器上调用。
	Reset(time.Duration)
}

// System 使用系统时钟实现 Clock。
type System struct{}

// 现在返回当前的单调时间。
func (c System) Now() AbsTime {
	return Now()
}

// 在给定的持续时间内休眠块。
func (c System) Sleep(d time.Duration) {
	time.Sleep(d)
}

// NewTimer 创建一个可以重新安排的计时器。
func (c System) NewTimer(d time.Duration) ChanTimer {
	ch := make(chan AbsTime, 1)
	t := time.AfterFunc(d, func() {
		// 这个发送是非阻塞的，因为 time.Timer 就是这样
		// 行为。在快乐的情况下无关紧要，但确实如此
		// 当 Reset 被误用时。
		select {
		case ch <- c.Now():
		default:
		}
	})
	return &systemTimer{t, ch}
}

// After 返回一个通道，它在 d 过去后接收当前时间。
func (c System) After(d time.Duration) <-chan AbsTime {
	ch := make(chan AbsTime, 1)
	time.AfterFunc(d, func() { ch <- c.Now() })
	return ch
}

// AfterFunc 在持续时间结束后在新的 goroutine 上运行 f。
func (c System) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)	
}

type systemTimer struct {
	*time.Timer
	ch <-chan AbsTime
}

func (st *systemTimer) Reset(d time.Duration) {
	st.Timer.Reset(d)
}

func (st *systemTimer) C() <-chan AbsTime {
	return st.ch
}
