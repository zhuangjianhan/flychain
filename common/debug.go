package common

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// 报告发出警告，要求用户向 github 跟踪器提交问题
func Report(extra ...interface{}) {
	fmt.Fprintln(os.Stderr, "You've encountered a sought after, hard to reproduce bug. Please report this to the developers")
	fmt.Fprintln(os.Stderr, extra...)

	_, file, line, _ := runtime.Caller(1)
	fmt.Fprintf(os.Stderr, "%v:%v\n", file, line)

	debug.PrintStack()

	fmt.Fprintln(os.Stderr, "#### BUG! PLEASE REPORT ####")
}

// PrintDeprecationWarning 使用 fmt.Println 在框中打印给定的字符串。
func PrintDeprecationWarning(str string) {
	line := strings.Repeat("#", len(str)+4)
	emptyLine := strings.Repeat(" ", len(str))
	fmt.Printf(`
%s
# %s #
# %s #
# %s #
%s

`, line, emptyLine, str, emptyLine, line)
}