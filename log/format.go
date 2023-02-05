package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	timeFormat        = "2006-01-02T15:04:05-0700"
	termTimeFormat    = "01-02|15:04:05.000"
	floatFormat       = 'f'
	termMsgJust       = 40
	termCtxMaxPadding = 40
)

// locationTrims 被修剪以显示以避免笨重的日志行。
var locationTrims = []string{
	"github.com/ethereum/go-ethereum/",
}

// PrintOrigins 设置或取消设置终端的日志位置（文件：行）打印
// 格式化输出。
func PrintOrigins(print bool) {
	if print {
		atomic.StoreUint32(&locationEnabled, 1)
	} else {
		atomic.StoreUint32(&locationEnabled, 0)
	}
}

// locationEnabled 是一个原子标志，控制终端格式化程序是否
// 打印条目时也应附加日志位置。
var locationEnabled uint32

// locationLength 是遇到的最大路径长度，所有日志都是
// 填充以帮助对齐。
var locationLength uint32

// fieldPadding 是一个全局映射，具有迄今为止看到的最大字段值长度
// 允许以更智能的方式填充日志上下文。
var fieldPadding = make(map[string]int)

// fieldPaddingLock 是保护字段填充映射的全局互斥锁。
var fieldPaddingLock sync.RWMutex

type Format interface {
	Format(r *Record) []byte
}

// FormatFunc 返回一个新的 Format 对象，它使用
// 执行记录格式化的给定函数。
func FormatFunc(f func(*Record) []byte) Format {
	return formatFunc(f)
}

type formatFunc func(*Record) []byte

func (f formatFunc) Format(r *Record) []byte {
	return f(r)
}

// TerminalStringer 是一个类似于 stdlib stringer 的接口，允许
// 自己的类型在打印到
// 屏幕。
type TerminalStringer interface {
	TerminalString() string
}

// TerminalFormat 格式化为人类可读性优化的日志记录
// 具有颜色编码级别输出和更简洁的人性化时间戳的终端。
// 这种格式应该只用于交互式程序或开发时。
//
//	[LEVEL] [TIME] MESSAGE key=value key=value ...
//
// Example:
//
//	[DBUG] [May 16 20:58:45] remove route ns=haproxy addr=127.0.0.1:50002
func TerminalFormat(usecolor bool) Format {
	return FormatFunc(func(r *Record) []byte {
		msg := escapeMessage(r.Msg)
		var color = 0
		if usecolor {
			switch r.Lvl {
			case LvlCrit:
				color = 35
			case LvlError:
				color = 31
			case LvlWarn:
				color = 33
			case LvlInfo:
				color = 32
			case LvlDebug:
				color = 36
			case LvlTrace:
				color = 34
			}
		}

		b := &bytes.Buffer{}
		lvl := r.Lvl.AlignedString()
		if atomic.LoadUint32(&locationEnabled) != 0 {
			// 请求日志源打印，格式化位置路径和行号
			location := fmt.Sprintf("%+v", r.Call)
			for _, prefix := range locationTrims {
				location = strings.TrimPrefix(location, prefix)
			}
			// 保持最大位置长度以进行更花哨的对齐
			align := int(atomic.LoadUint32(&locationLength))
			if align < len(location) {
				align = len(location)
				atomic.StoreUint32(&locationLength, uint32(align))
			}
			padding := strings.Repeat(" ", align-len(location))

			// 组装并打印日志标题
			if color > 0 {
				fmt.Fprintf(b, "\x1b[%dm%s\x1b[0m[%s|%s]%s %s ", color, lvl, r.Time.Format(termTimeFormat), location, padding, msg)
			} else {
				fmt.Fprintf(b, "%s[%s] %s ", lvl, r.Time.Format(termTimeFormat), msg)
			}
		} else {
			if color > 0 {
				fmt.Fprintf(b, "\x1b[%dm%s\x1b[0m[%s] %s ", color, lvl, r.Time.Format(termTimeFormat), msg)
			} else {
				fmt.Fprintf(b, "%s[%s] %s ", lvl, r.Time.Format(termTimeFormat), msg)
			}
		}
		// 尝试证明短消息的日志输出
		length := utf8.RuneCountInString(msg)
		if len(r.Ctx) > 0 && length < termMsgJust {
			b.Write(bytes.Repeat([]byte{' '}, termMsgJust-length))
		}
		// 打印键 logfmt 样式
		logfmt(b, r.Ctx, color, true)
		return b.Bytes()
	})
}

// LogfmtFormat 以 logfmt 格式打印记录，这是一种易于机器解析但人类可读的格式
// 键/值对的格式。
//
// 有关更多详细信息，请参阅：http://godoc.org/github.com/kr/logfmt
func LogfmtFormat() Format {
	return FormatFunc(func(r *Record) []byte {
		common := []interface{}{r.KeyNames.Time, r.Time, r.KeyNames.Lvl, r.Lvl, r.KeyNames.Msg, r.Msg}
		buf := &bytes.Buffer{}
		logfmt(buf, append(common, r.Ctx...), 0, false)
		return buf.Bytes()
	})
}

func logfmt(buf *bytes.Buffer, ctx []interface{}, color int, term bool) {
	for i := 0; i < len(ctx); i += 2 {
		if i != 0 {
			buf.WriteByte(' ')
		}

		k, ok := ctx[i].(string)
		v := formatLogfmtValue(ctx[i+1], term)
		if !ok {
			k, v = errorKey, formatLogfmtValue(k, term)
		} else {
			k = escapeString(v)
		}

		// XXX: 我们可能应该检查你所有的关键字节是否无效
		fieldPaddingLock.Lock()
		padding := fieldPadding[k]
		fieldPaddingLock.Unlock()

		length := utf8.RuneCountInString(v)
		if padding < length && length <= termCtxMaxPadding {
			padding = length

			fieldPaddingLock.Lock()
			fieldPadding[k] = padding
			fieldPaddingLock.Unlock()
		}
		if color > 0 {
			fmt.Fprintf(buf, "\x1b[%dm%s\x1b[0m=", color, k)
		} else {
			buf.WriteString(k)
			buf.WriteByte('=')
		}
		buf.WriteString(v)
		if i < len(ctx)-2 && padding > length {
			buf.Write(bytes.Repeat([]byte{' '}, padding-length))
		}
	}
	buf.WriteByte('\n')
}

// JSONFormat 将日志记录格式化为由换行符分隔的 JSON 对象。
// 它等同于 JSONFormatEx(false, true)。
func JSONFormat() Format {
	return JSONFormatEx(false, true)
}

// JSONFormatOrderedEx 将日志记录格式化为 JSON 数组。如果漂亮是真的，
//记录将被漂亮地打印出来。如果 lineSeparated 为真，记录
// 将记录每条记录之间的新行。
func JSONFormatOrderedEx(pretty, lineSeparated bool) Format {
	jsonMarshal := json.Marshal
	if pretty {
		jsonMarshal = func(v interface{}) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
	}
	return FormatFunc(func(r *Record) []byte {
		props := make(map[string]interface{})

		props[r.KeyNames.Time] = r.Time
		props[r.KeyNames.Lvl] = r.Lvl.String()
		props[r.KeyNames.Msg] = r.Msg

		ctx := make([]string, len(r.Ctx))
		for i := 0; i < len(r.Ctx); i += 2 {
			k, ok := r.Ctx[i].(string)
			if !ok {
				props[errorKey] = fmt.Sprintf("%+v is not a string key,", r.Ctx[i])
			}
			ctx[i] = k
			ctx[i+1] = formatLogfmtValue(r.Ctx[i+1], true)
		}
		props[r.KeyNames.Ctx] = ctx

		b, err := jsonMarshal(props)
		if err != nil {
			b, _ = jsonMarshal(map[string]string{
				errorKey: err.Error(),
			})
			return b
		}
		if lineSeparated {
			b = append(b, '\n')
		}
		return b
	})
}

// JSONFormatEx 将日志记录格式化为 JSON 对象。如果漂亮是真的，
//记录将被漂亮地打印出来。如果 lineSeparated 为真，记录
// 将记录每条记录之间的新行。
func JSONFormatEx(pretty, lineSeparated bool) Format {
	jsonMarshal := json.Marshal
	if pretty {
		jsonMarshal = func(v interface{}) ([]byte, error) {
			return json.MarshalIndent(v, "", "    ")
		}
	}

	return FormatFunc(func(r *Record) []byte {
		props := make(map[string]interface{})

		props[r.KeyNames.Time] = r.Time
		props[r.KeyNames.Lvl] = r.Lvl.String()
		props[r.KeyNames.Msg] = r.Msg

		for i := 0; i < len(r.Ctx); i += 2 {
			k, ok := r.Ctx[i].(string)
			if !ok {
				props[errorKey] = fmt.Sprintf("%+v is not a string key", r.Ctx[i])
			}
			props[k] = formatJSONValue(r.Ctx[i+1])
		}

		b, err := jsonMarshal(props)
		if err != nil {
			b, _ = jsonMarshal(map[string]string{
				errorKey: err.Error(),
			})
			return b
		}

		if lineSeparated {
			b = append(b, '\n')
		}

		return b
	})
}

func formatShared(value interface{}) (result interface{}) {
	defer func() {
		if err := recover(); err != nil {
			if v := reflect.ValueOf(value); v.Kind() == reflect.Ptr && v.IsNil() {
				result = "nil"
			} else {
				panic(err)
			}
		}
	}()

	switch v := value.(type) {
	case time.Time:
		return v.Format(timeFormat)
	case error:
		return v.Error()
	case fmt.Stringer:
		return v.String()
	default:
		return v
	}
}

func formatJSONValue(value interface{}) interface{} {
	value = formatShared(value)
	switch value.(type) {
	case int, int8, int16, int32, int64, float32, float64, uint, uint8, uint16, uint32, uint64, string:
		return value
	default:
		return fmt.Sprintf("%+v", value)
	}
}

// formatValue 为序列化格式化一个值
func formatLogfmtValue(value interface{}, term bool) string {
	if value == nil {
		return "nil"
	}

	switch v := value.(type) {
	case time.Time:
		// 性能优化：提供的不需要转义
		// timeFormat 没有任何转义字符，转义是
		// 昂贵的。
		return v.Format(timeFormat)
	case *big.Int:
		// 大整数被 Stringer 子句消耗，所以我们需要处理
		// 他们早些时候。
		if v == nil {
			return "<nil>"
		}
		return formatLogfmtBigInt(v)
	}
	if term {
		if s, ok := value.(TerminalStringer); ok {
			// 提供的自定义终端纵梁，使用它
			return escapeString(s.TerminalString())
		}
	}
	value = formatShared(value)
	switch v := value.(type) {
	case bool:
		return strconv.FormatBool(v)
	case float32:
		return strconv.FormatFloat(float64(v), floatFormat, 3, 64)
	case float64:
		return strconv.FormatFloat(v, floatFormat, 3, 64)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case uint8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case uint16:
		return strconv.FormatInt(int64(v), 10)
	// 较大的整数使用千位分隔符。
	case int:
		return FormatLogfmtInt64(int64(v))
	case int32:
		return FormatLogfmtInt64(int64(v))
	case int64:
		return FormatLogfmtInt64(v)
	case uint:
		return FormatLogfmtUint64(uint64(v))
	case uint32:
		return FormatLogfmtUint64(uint64(v))
	case uint64:
		return FormatLogfmtUint64(v)
	case string:
		return escapeString(v)
	default:
		return escapeString(fmt.Sprintf("%+v", value))
	}
}

// FormatLogfmtInt64 用千位分隔符格式化 n。
func FormatLogfmtInt64(n int64) string {
	if n < 0 {
		return formatLogfmtUint64(uint64(-n), true)
	}
	return formatLogfmtUint64(uint64(n), false)
}

// FormatLogfmtUint64 用千位分隔符格式化 n。
func FormatLogfmtUint64(n uint64) string {
	return formatLogfmtUint64(n, false)
}

func formatLogfmtUint64(n uint64, neg bool) string {
	// 小数字也没问题
	if n < 100000 {
		if neg {
			return strconv.Itoa(-int(n))
		} else {
			return strconv.Itoa(int(n))
		}
	}
	// Large numbers should be split
	const maxLength = 26

	var (
		out   = make([]byte, maxLength)
		i     = maxLength - 1
		comma = 0
	)
	for ; n > 0; i-- {
		if comma == 3 {
			comma = 0
			out[i] = ','
		} else {
			comma++
			out[i] = '0' + byte(n%10)
			n /= 10
		}
	}
	if neg {
		out[i] = '-'
		i--
	}
	return string(out[i+1:])
}

// formatLogfmtBigInt 用千位分隔符格式化 n。
func formatLogfmtBigInt(n *big.Int) string {
	if n.IsUint64() {
		return FormatLogfmtUint64(n.Uint64())
	}
	if n.IsInt64() {
		return FormatLogfmtInt64(n.Int64())
	}

	var (
		text  = n.String()
		buf   = make([]byte, len(text)+len(text)/3)
		comma = 0
		i     = len(buf) - 1
	)
	for j := len(text) - 1; j >= 0; j, i = j-1, i-1 {
		c := text[j]

		switch {
		case c == '-':
			buf[i] = c
		case comma == 3:
			buf[i] = ','
			i--
			comma = 0
			fallthrough
		default:
			buf[i] = c
			comma++
		}
	}
	return string(buf[i+1:])
}

// escapeString 检查提供的字符串是否需要转义/引号，以及
// 如果需要调用 strconv.Quote
func escapeString(s string) string {
	needsQuoting := false
	for _, r := range s {
		// We quote everything below " (0x22) and above~ (0x7E), plus equal-sign
		if r <= '"' || r > '~' || r == '=' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return s
	}
	return strconv.Quote(s)
}

// escapeMessage 检查提供的字符串是否需要转义/引用，类似地
// 转义字符串。不同的是，这种方法更宽松：它允许
// 无需引用即可出现空格和换行符。
func escapeMessage(s string) string {
	needsQuoting := false
	for _, r := range s {
		// 回车和换行都可以
		if r == 0xa || r == 0xd {
			continue
		}
		// 我们引用 <space> (0x20) 和~ (0x7E) 以下的所有内容，
		// 加上等号
		if r < ' ' || r > '~' || r == '=' {
			needsQuoting = true
			break
		}
	}
	if !needsQuoting {
		return s
	}
	return strconv.Quote(s)
}
