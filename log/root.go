package log

import (
	"os"
)

var (
	root = &logger{[]interface{}{}, new(swapHandler)}
	stdoutHandler = StreamHandler(os.Stdout, LogfmtFormat())
	stderrHandler = StreamHandler(os.Stderr, LogfmtFormat())
)

// New 返回具有给定上下文的新记录器。
// New 是 Root().New 的一个方便的别名
func New(ctx ...interface{}) Logger {
	return root.New(ctx...)
}

//root 返回根记录器
func Root() Logger {
	return root
}

// 以下函数绕过导出的记录器方法 (logger.Debug,
// 等）以保持所有路径的调用深度相同 logger.write 所以
// runtime.Caller(2) 始终引用客户端代码中的调用站点。

// Trace 是 Root().Trace 的一个方便的别名
func Trace(msg string, ctx ...interface{}) {
	root.write(msg, LvlTrace, ctx, skipLevel)
}

// Debug is a convenient alias for Root().Debug
func Debug(msg string, ctx ...interface{}) {
	root.write(msg, LvlDebug, ctx, skipLevel)
}

// Info is a convenient alias for Root().Info
func Info(msg string, ctx ...interface{}) {
	root.write(msg, LvlInfo, ctx, skipLevel)
}

// Warn is a convenient alias for Root().Warn
func Warn(msg string, ctx ...interface{}) {
	root.write(msg, LvlWarn, ctx, skipLevel)
}

// Error is a convenient alias for Root().Error
func Error(msg string, ctx ...interface{}) {
	root.write(msg, LvlError, ctx, skipLevel)
}

// Crit is a convenient alias for Root().Crit
func Crit(msg string, ctx ...interface{}) {
	root.write(msg, LvlCrit, ctx, skipLevel)
	os.Exit(1)
}

// Output 是 write 的一个方便的别名，允许修改
//调用深度（要跳过的堆栈帧数）。
// 调用深度影响日志消息的报告行号。
// 零调用深度报告 Output 的直接调用者。
// 非零调用深度跳过尽可能多的堆栈帧。
func Output(msg string, lvl Lvl, calldepth int, ctx ...interface{}) {
	root.write(msg, lvl, ctx, calldepth+skipLevel)
}