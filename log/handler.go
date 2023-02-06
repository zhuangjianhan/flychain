package log

import (
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"sync"

	"github.com/go-stack/stack"
)

// Handler 定义日志记录的写入位置和方式。
// Logger 通过写入 Handler 来打印其日志记录。
// 处理程序是可组合的，为您提供了极大的组合灵活性
// 它们来实现适合您的应用程序的日志记录结构。
type Handler interface {
	Log(r *Record) error
}

// FuncHandler 返回一个记录给定记录的处理程序
// 功能。
func FuncHandler(fn func(r *Record) error) Handler {
	return funcHandler(fn)
}

type funcHandler func(r *Record) error

func (h funcHandler) Log(r *Record) error {
	return h(r)
}

// StreamHandler 将日志记录写入一个 io.Writer
// 使用给定的格式。可以使用 StreamHandler
// 轻松开始将日志记录写入其他
// 输出。
//
// StreamHandler 用 LazyHandler 和 SyncHandler 包装自己
// 评估惰性对象并执行安全的并发写入。
func StreamHandler(wr io.Writer, fmtr Format) Handler {
	h := FuncHandler(func(r *Record) error {
		_, err := wr.Write(fmtr.Format(r))
		return err
	})
	return LazyHandler(SyncHandler(h))
}

// SyncHandler 可以包裹在处理程序周围以保证
// 一次只能进行一个日志操作。有必要
// 用于线程安全的并发写入。
func SyncHandler(h Handler) Handler {
	var mu sync.Mutex
	return FuncHandler(func(r *Record) error {
		mu.Lock()
		defer mu.Unlock()

		return h.Log(r)
	})
}

// FileHandler 返回一个将日志记录写入给定文件的处理程序
// 使用给定的格式。如果路径
// 已经存在，FileHandler 将追加到给定的文件。如果没有，
// FileHandler 将创建模式为 0644 的文件。
func FileHandler(path string, fmtr Format) (Handler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return closingHandler{f, StreamHandler(f, fmtr)}, nil
}

// NetHandler 打开给定地址的套接字并写入记录
// 通过连接。
func NetHandler(network, addr string, fmtr Format) (Handler, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return closingHandler{conn, StreamHandler(conn, fmtr)}, nil
}

// XXX: closingHandler 目前基本未使用
// 这意味着将来 Handler 接口支持时
// 一个可能的 Close() 操作
type closingHandler struct {
	io.WriteCloser
	Handler
}

func (h *closingHandler) Close() error {
	return h.WriteCloser.Close()
}

// CallerFileHandler 返回一个添加行号和文件的Handler
// 使用键“caller”调用上下文的函数。
func CallerFileHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		r.Ctx = append(r.Ctx, "caller", fmt.Sprint(r.Call))
		return h.Log(r)
	})
}

// CallerFuncHandler 返回一个将调用函数名添加到的 Handler
// 键为“fn”的上下文。
func CallerFuncHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		r.Ctx = append(r.Ctx, "fn", formatCall("%+n", r.Call))
		return h.Log(r)
	})
}

// 这个函数是为了在 Go < 1.8 上进行审查。
func formatCall(format string, c stack.Call) string {
	return fmt.Sprintf(format, c)
}

// CallerStackHandler 返回一个将堆栈跟踪添加到上下文的处理程序
// 使用键“stack”。堆栈跟踪被格式化为以空格分隔的列表
// 在匹配的 [] 中调用站点。最近的呼叫站点首先列出。
// 每个调用站点都根据格式进行格式化。请参阅文档
// 打包 github.com/go-stack/stack 以获得支持的格式列表。
func CallerStackHandler(format string, h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		s := stack.Trace().TrimBelow(r.Call).TrimRuntime()
		if len(s) > 0 {
			r.Ctx = append(r.Ctx, "stack", fmt.Sprintf(format, s))
		}
		return h.Log(r)
	})
}

// FilterHandler 返回一个仅将记录写入的 Handler
// 如果给定函数的计算结果为真，则包装处理程序。例如，
// 只记录 'err' 键不为 nil 的记录：
func FilterHandler(fn func(r *Record) bool, h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		if fn(r) {
			return h.Log(r)
		}
		return nil
	})
}

// MatchFilterHandler 返回一个只写记录的Handler
// 如果记录中的给定键，则到包装的处理程序
// 上下文匹配值。例如，只记录记录
// 来自你的 ui 包：
func MatchFilterHandler(key string, value interface{}, h Handler) Handler {
	return FilterHandler(func(r *Record) bool {
		switch key {
		case r.KeyNames.Lvl:
			return r.Lvl == value
		case r.KeyNames.Time:
			return r.Time == value
		case r.KeyNames.Msg:
			return r.Msg == value
		}

		for i := 0; i < len(r.Ctx); i += 2 {
			if r.Ctx[i] == key {
				return r.Ctx[i+1] == value
			}
		}
		return false
	}, h)
}

// LvlFilterHandler 返回一个只写的 Handler
// 小于给定详细程度的记录
// 级别到包装的处理程序。例如，只
// 记录错误/暴击记录：
func LvlFilterHandler(maxLvl Lvl, h Handler) Handler {
	return FilterHandler(func(r *Record) bool {
		return r.Lvl <= maxLvl
	}, h)
}

// MultiHandler 将任何写入分派给它的每个处理程序。
// 这对于写入不同类型的日志信息很有用
//到不同的位置。例如，记录到一个文件和
// 标准错误：
func MultiHandler(hs ...Handler) Handler {
	return FuncHandler(func(r *Record) error {
		for _, h := range hs {
			h.Log(r)
		}
		return nil
	})
}

// FailoverHandler 将所有日志记录写入第一个处理程序
// 已指定，但将故障转移并写入第二个处理程序，如果
// 第一个处理程序失败，对于所有指定的处理程序依此类推。
// 例如，您可能想登录到网络套接字，但故障转移
// 如果网络出现故障，写入文件，然后
// 如果文件写入失败则标准输出：
//
//	log.FailoverHandler(
//	    log.Must.NetHandler("tcp", ":9090", log.JSONFormat()),
//	    log.Must.FileHandler("/var/log/app.log", log.LogfmtFormat()),
//	    log.StdoutHandler)
//
// 所有不进入第一个处理程序的写入都将添加具有键的上下文
// 形式“failover_err_{idx}”解释遇到的错误
// 尝试写入列表中它们之前的处理程序。
func FailoverHandler(hs ...Handler) Handler {
	return FuncHandler(func(r *Record) error {
		var err error
		for i, h := range hs {
			err = h.Log(r)
			if err == nil {
				return nil
			}
			r.Ctx = append(r.Ctx, fmt.Sprintf("failover_err_%d", i), err)
		}

		return err
	})
}

// ChannelHandler 将所有记录写入给定的通道。
// 如果通道已满，它会阻塞。对异步处理很有用
// 日志消息，由 BufferedHandler 使用。
func ChannelHandler(recs chan<- *Record) Handler {
	return FuncHandler(func(r *Record) error {
		recs <- r
		return nil
	})
}

// BufferedHandler 将所有记录写入缓冲
// 冲入包装的给定大小的通道
// 可用于写入的处理程序。由于这些
// 写入是异步发生的，所有写入到 BufferedHandler
// 从不返回错误，并且忽略来自包装处理程序的任何错误。
func BufferedHandler(bufSize int, h Handler) Handler {
	recs := make(chan *Record, bufSize)
	go func() {
		for m := range recs {
			_ = h.Log(m)
		}
	}()
	return ChannelHandler(recs)
}

// LazyHandler 在评估后将所有值写入包装的处理程序
// 记录上下文中的任何惰性函数。已经包好了
// 围绕这个库中的 StreamHandler 和 SyslogHandler，你只需要
// 如果您编写自己的处理程序，则它。
func LazyHandler(h Handler) Handler {
	return FuncHandler(func(r *Record) error {
		// 遍历值（奇数索引）并重新分配
		// 任何惰性 fn 的值与其执行结果
		hadErr := false
		for i := 1; i < len(r.Ctx); i += 2 {
			lz, ok := r.Ctx[i].(Lazy)
			if ok {
				v, err := evaluateLazy(lz)
				if err != nil {
					hadErr = true
					r.Ctx[i] = err
				} else {
					if cs, ok := v.(stack.CallStack); ok {
						v = cs.TrimBelow(r.Call).TrimRuntime()
					}
					r.Ctx[i] = v
				}
			}
		}

		if hadErr {
			r.Ctx = append(r.Ctx, errorKey, "bad lazy")
		}

		return h.Log(r)
	})
}

func evaluateLazy(lz Lazy) (interface{}, error) {
	t := reflect.TypeOf(lz.Fn)

	if t.Kind() != reflect.Func {
		return nil, fmt.Errorf("INVALID_LAZY, not func: %+v", lz.Fn)
	}

	if t.NumIn() > 0 {
		return nil, fmt.Errorf("INVALID_LAZY, func takes args: %+v", lz.Fn)
	}

	if t.NumOut() == 0 {
		return nil, fmt.Errorf("INVALID_LAZY, no func return val: %+v", lz.Fn)
	}

	value := reflect.ValueOf(lz.Fn)
	results := value.Call([]reflect.Value{})
	if len(results) == 1 {
		return results[0].Interface(), nil
	}
	values := make([]interface{}, len(results))
	for i, v := range results {
		values[i] = v.Interface()
	}
	return values, nil
}

// DiscardHandler 报告所有写入成功但不执行任何操作。
// 这对于在运行时通过动态禁用日志记录很有用
// 记录器的 SetHandler 方法。
func DiscardHandler() Handler {
	return FuncHandler(func(r *Record) error {
		return nil
	})
}

// 必须提供以下Handler创建函数
// 它不返回错误参数，只返回一个处理程序
// 失败时恐慌：FileHandler、NetHandler、SyslogHandler、SyslogNetHandler
var Must muster

func must(h Handler, err error) Handler {
	if err != nil {
		panic(err)
	}
	return h
}

type muster struct{}

func (m muster) FileHandler(path string, fmtr Format) Handler {
	return must(FileHandler(path, fmtr))
}

func (m muster) NetHandler(network, addr string, fmtr Format) Handler {
	return must(NetHandler(network, addr, fmtr))
}
