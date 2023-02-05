package log

import (
	"fmt"
	"time"

	"github.com/go-stack/stack"
)

const timeKey = "t"
const lvlKey = "lvl"
const msgKey = "msg"
const ctxKey = "ctx"
const errorKey = "LOG15_ERROR"
const skipLevel = 2

type Lvl int

const (
	LvlCrit Lvl = iota
	LvlError
	LvlWarn
	LvlInfo
	LvlDebug
	LvlTrace
)

// AlignedString 返回包含 Lvl 名称的 5 个字符的字符串。
func (l Lvl) AlignedString() string {
	switch l {
	case LvlTrace:
		return "TRACE "
	case LvlDebug:
		return "DEBUG"
	case LvlInfo:
		return "INFO "
	case LvlWarn:
		return "WARN "
	case LvlError:
		return "ERROR"
	case LvlCrit:
		return "CRIT "
	default:
		panic("bad level")
	}
}

// String returns the name of a Lvl.
func (l Lvl) String() string {
	switch l {
	case LvlTrace:
		return "trce"
	case LvlDebug:
		return "dbug"
	case LvlInfo:
		return "info"
	case LvlWarn:
		return "warn"
	case LvlError:
		return "eror"
	case LvlCrit:
		return "crit"
	default:
		panic("bad level")
	}
}

// LvlFromString returns the appropriate Lvl from a string name.
// Useful for parsing command line args and configuration files.
func LvlFromString(lvlString string) (Lvl, error) {
	switch lvlString {
	case "trace", "trce":
		return LvlTrace, nil
	case "debug", "dbug":
		return LvlDebug, nil
	case "info":
		return LvlInfo, nil
	case "warn":
		return LvlWarn, nil
	case "error", "eror":
		return LvlError, nil
	case "crit":
		return LvlCrit, nil
	default:
		return LvlDebug, fmt.Errorf("unknown level: %v", lvlString)
	}
}

// 记录是记录器要求其处理程序写入的内容
type Record struct {
	Time     time.Time
	Lvl      Lvl
	Msg      string
	Ctx      []interface{}
	Call     stack.Call
	KeyNames RecordKeyNames
}

// 当执行写函数时，RecordKeyNames 被存储在一个 Record 中。
type RecordKeyNames struct {
	Time string
	Msg  string
	Lvl  string
	Ctx  string
}

// 记录器将键/值对写入处理程序
type Logger interface {
	// New 返回一个新的 Logger，它有这个 logger 的上下文加上给定的上下文
	New(ctx ...interface{}) Logger

	// GetHandler 获取与记录器关联的处理程序。
	
}
