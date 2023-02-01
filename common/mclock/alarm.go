package mclock

import "time"

// Alarm 在通道上发送定时通知。这与常规计时器非常相似，
// 但是在需要一遍又一遍地重新安排同一个计时器的代码中更容易使用。
//
// 当调度一个Alarm时，C()返回的channel稍后会收到一个值
// 比预定的时间。警报在触发后可以重复使用，也可以
// 通过调用 Stop 取消。
type Alarm struct {
	ch       chan struct{}
	clock    Clock
	timer     Timer
	deadline AbsTime
}

// NewAlarm 创建一个警报。
func NewAlarm(clock Clock) *Alarm {
	if clock == nil {
		panic("nil clock")
	}
	return &Alarm{
		ch:    make(chan struct{}, 1),
		clock: clock,
	}
}

// C 返回告警通知通道。此频道保持不变
// 警报的整个生命周期，永远不会关闭。
func (e *Alarm) C() <-chan struct{} {
	return e.ch
}

// Stop 取消警报并排空通道。
// 这种方法对于并发使用是不安全的。
func (e *Alarm) Stop() {
	//Clear timer
	if e.timer != nil {
		e.timer.Stop()
	}
	e.deadline = 0

	// 排空通道
	select {
	case <-e.ch:
	default:
	}
}

// Schedule 将警报设置为不晚于给定时间触发。如果警报已经
// 已安排但尚未触发，它可能会比新安排的时间更早触发。
func (e *Alarm) Schedule(time AbsTime) {
	now := e.clock.Now()
	e.schedule(now, time)
}

func (e *Alarm) schedule(now, newDeadline AbsTime) {
	if e.timer != nil {
		if e.deadline > now && e.deadline <= newDeadline {
			// 在这里，可以重用当前定时器，因为它已经被调度到
			// 早于新的截止日期。
			//
			// e.deadline > now 部分条件很重要。如果老
			// deadline 已经过去了，我们假设定时器已经触发并且需要
			// 重新安排。
			return
		}
		e.timer.Stop()
	}

	// 设置定时器。
	d := time.Duration(0)
	if newDeadline < now {
		newDeadline = now
	} else {
		d = newDeadline.Sub(now)
	}
	e.timer = e.clock.AfterFunc(d, e.send)
	e.deadline = newDeadline
}	

func (e *Alarm) send() {
	select {
	case e.ch <- struct{}{}:
	default:
	}
}