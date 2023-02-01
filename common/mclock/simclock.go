package mclock

import (
	"container/heap"
	"sync"
	"time"
)

//模拟实现了一个虚拟时钟，用于可重现的时间敏感测试。它
// 在实际处理时间为零的虚拟时间尺度上模拟调度程序。
//
// 虚拟时钟不会自己前进，调用Run让它前进并执行定时器。
// 由于无法影响 Go 调度程序，因此测试涉及的超时行为
// goroutines 需要特别注意。测试此类超时的一个好方法如下：首先
// 执行应该超时的操作。确保您要测试的定时器
// 被建造。然后运行时钟直到超时。最后观察效果
type Simulated struct {
	now       AbsTime
	scheduled simTimerHeap
	mu        sync.RWMutex
	cond      *sync.Cond
}

// simTimer 在虚拟时钟上实现 ChanTimer。
type simTimer struct {
	at    AbsTime
	index int // position in s.scheduled
	s     *Simulated
	do    func()
	ch    <-chan AbsTime
}

func (s *Simulated) init() {
	if s.cond == nil {
		s.cond = sync.NewCond(&s.mu)
	}
}

// Run 将时钟移动给定的持续时间，在该持续时间之前执行所有计时器。
func (s *Simulated) Run(d time.Duration) {
	s.mu.Lock()
	s.init()

	end := s.now.Add(d)
	var do []func()
	for len(s.scheduled) > 0 && s.scheduled[0].at <= end {
		ev := heap.Pop(&s.scheduled).(*simTimer)
		do = append(do, ev.do)
	}
	s.now = end
	s.mu.Unlock()

	for _, fn := range do {
		fn()
	}
}

// ActiveTimers 返回未触发的计时器数。
func (s *Simulated) ActiveTimers() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.scheduled)
}

// WaitForTimers 等到时钟至少有 n 个预定的定时器。
func (s *Simulated) WaitForTimers(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()

	for len(s.scheduled) < n {
		s.cond.Wait()
	}
}

// 现在返回当前虚拟时间。
func (s *Simulated) Now() AbsTime {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.now
}

// 现在返回当前虚拟时间。
func (s *Simulated) Sleep(d time.Duration) {
	<-s.After(d)
}

// NewTimer 创建一个计时器，当时钟提前 d 时触发。
func (s *Simulated) NewTimer(d time.Duration) ChanTimer {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan AbsTime, 1)
	var timer *simTimer
	timer = s.schedule(d, func() { ch <- timer.at })
	timer.ch = ch
	return timer
}

// After 返回一个在时钟之后接收当前时间的通道
// 已经前进了 d
func (s *Simulated) After(d time.Duration) <-chan AbsTime {
	return s.NewTimer(d).C()
}

// AfterFunc 在时钟提前 d 后运行 fn。与系统不同
// clock, fn 运行在调用 Run 的 goroutine 上。
func (s *Simulated) AfterFunc(d time.Duration, fn func()) Timer {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.schedule(d, fn)
}

func (s *Simulated) schedule(d time.Duration, fn func()) *simTimer {
	s.init()

	at := s.now.Add(d)
	ev := &simTimer{do: fn, at: at, s: s}
	heap.Push(&s.scheduled, ev)
	s.cond.Broadcast()
	return ev
}

func (ev *simTimer) Stop() bool {
	ev.s.mu.Lock()
	defer ev.s.mu.Unlock()

	if ev.index < 0 {
		return false
	}
	heap.Remove(&ev.s.scheduled, ev.index)
	ev.s.cond.Broadcast()
	ev.index = -1
	return true
}

func (ev *simTimer) Reset(d time.Duration) {
	if ev.ch == nil {
		panic("mclock: Reset() on timer created by AfterFunc")
	}

	ev.s.mu.Lock()
	defer ev.s.mu.Unlock()
	ev.at = ev.s.now.Add(d)
	if ev.index < 0 {
		heap.Push(&ev.s.scheduled, ev)// already expired
	} else {
		heap.Fix(&ev.s.scheduled, ev.index)// hasn't fired yet, reschedule
	}
	ev.s.cond.Broadcast()
}

func (ev *simTimer) C() <-chan AbsTime {
	if ev.ch == nil {
		panic("mclock: C() on timer created by AfterFunc")
	}
	return ev.ch
}

type simTimerHeap []*simTimer

func (h *simTimerHeap) Len() int {
	return len(*h)
}

func (h *simTimerHeap) Less(i, j int) bool {
	return (*h)[i].at < (*h)[j].at
}

func (h *simTimerHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
	(*h)[i].index = i
	(*h)[j].index = j
}

func (h *simTimerHeap) Push(x interface{}) {
	t := x.(*simTimer)
	t.index = len(*h)
	*h = append(*h, t)
}

func (h *simTimerHeap) Pop() interface{} {
	end := len(*h) - 1
	t := (*h)[end]
	t.index = -1
	(*h)[end] = nil
	*h = (*h)[:end]
	return t
}
