package cmdtest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"text/template"
	"time"

	"github.com/docker/docker/pkg/reexec"
)

func NewTestCmd(t *testing.T, data interface{})

type TestCmd struct {
	// 为方便起见，所有测试方法均可用。
	*testing.T

	Func    template.FuncMap
	Data    interface{}
	Cleanup func()

	cmd    *exec.Cmd
	stdout *bufio.Reader
	stdin  io.WriteCloser
	stderr *testlogger
	// Err 会包含进程退出错误或中断信号错误
	Err error
}

var id int32

// 使用名称作为 argv[0] 运行 exec 的当前二进制文件，这将触发
// reexec init function for that name (e.g. "geth-test" in cmd/geth/run_test.go)
func (tt *TestCmd) Run(name string, args ...string) {
	id := atomic.AddInt32(&id, 1)
	tt.stderr = &testlogger{t: tt.T, name: fmt.Sprintf("%d", id)}
	tt.cmd = &exec.Cmd{
		Path:   reexec.Self(),
		Args:   append([]string{name}, args...),
		Stderr: tt.stderr,
	}
	stdout, err := tt.cmd.StdoutPipe()
	if err != nil {
		tt.Fatal(err)
	}
	tt.stdout = bufio.NewReader(stdout)
	if tt.stdin, err = tt.cmd.StdinPipe(); err != nil {
		tt.Fatal(err)
	}
	if err := tt.cmd.Start(); err != nil {
		tt.Fatal(err)
	}
}

// InputLine 将给定的文本写入孩子的标准输入。
// 这个方法也可以从 expect 模板调用，例如：
//
// geth.expect(`Passphrase: {{.InputLine "password"}}`)
func (tt *TestCmd) InputLine(s string) string {
	io.WriteString(tt.stdin, s+"\n")
	return ""
}

func (tt *TestCmd) SetTemplateFunc(name string, fn interface{}) {
	if tt.Func == nil {
		tt.Func = make(map[string]interface{})
	}
	tt.Func[name] = fn
}

// Expect 将其参数作为模板运行，然后期望
// 子进程在 5s 内输出模板的结果。
//
// 如果模板以换行符开头，则删除换行符
// 在匹配之前。
func (tt *TestCmd) Expect(tplsource string) {
	// 通过运行模板生成预期的输出。
	tpl := template.Must(template.New("").Funcs(tt.Func).Parse(tplsource))
	wantbuf := new(bytes.Buffer)
	if err := tpl.Execute(wantbuf, tt.Data); err != nil {
		panic(err)
	}
	// 在开头修剪一个换行符。这使得测试看起来
	// 更好，因为所有预期的字符串都在第 0 列。
	want := bytes.TrimPrefix(wantbuf.Bytes(), []byte("\n"))
	if err := tt.matchExactOutput(want); err != nil {
		tt.Fatal(err)
	}
	tt.Logf("Matched stdout text:\n%s", want)
}

// Output 从 stdout 读取所有输出，并返回数据。
func (tt *TestCmd) Output() []byte {
	var buf []byte
	tt.withKillTimeOut(func() { buf, _ = io.ReadAll(tt.stdout) })
	return buf 
}

func (tt *TestCmd) matchExactOutput(want []byte) error {
	buf := make([]byte, len(want))
	n := 0
	tt.withKillTimeOut(func() { n, _ = io.ReadFull(tt.stdout, buf) })
	buf = buf[:n]
	if n < len(want) || !bytes.Equal(buf, want) {
		// 在不匹配的情况下获取任何额外的缓冲输出
		// 因为它可能有助于调试。
		buf = append(buf, make([]byte, tt.stdout.Buffered())...)
		tt.stdout.Read(buf[n:])
		// 找到不匹配的位置。
		for i := 0; i < n; i++ {
			if want[i] != buf[i] {
				return fmt.Errorf("output mismatch at ◊:\n---------------- (stdout text)\n%s◊%s\n---------------- (expected text)\n%s",
					buf[:i], buf[i:n], want)
			}
		}
		if n < len(want) {
			return fmt.Errorf("not enough output, got until ◊:\n---------------- (stdout text)\n%s\n---------------- (expected text)\n%s◊%s",
				buf, want[:n], want[n:])
		}
	}
	return nil
}

// ExpectRegexp 期望子进程输出与
// 在 5s 内给出正则表达式。
//
// 请注意，任意数量的输出可能会被消耗
// 正则表达式。这通常意味着不能使用 expect
// 在 ExpectRegexp 之后。
func (tt *TestCmd) ExpectRegexp(regex string) (*regexp.Regexp, []string) {
	regex = strings.TrimPrefix(regex, "\n")
	var (
		re      = regexp.MustCompile(regex)
		rtee    = &runeTee{in: tt.stdout}
		matches []int
	)
	tt.withKillTimeOut(func() { matches = re.FindReaderSubmatchIndex(rtee) })
	output := rtee.buf.Bytes()
	if matches == nil {
		tt.Fatalf("Output did not match:\n---------------- (stdout text)\n%s\n---------------- (regular expression)\n%s",
			output, regex)
		return re, nil
	}
	tt.Logf("Match stdout text:\n%s", output)
	var submatches []string
	for i := 0; i < len(matches); i++ {
		submatch := string(output[matches[i]:matches[i+1]])
		submatches = append(submatches, submatch)
	}
	return re, submatches
}

// ExpectExit 期望子进程在 5s 内退出而不
// 在标准输出上打印任何附加文本。
func (tt *TestCmd) ExpectExit() {
	var output []byte
	tt.withKillTimeOut(func() {
		output, _ = io.ReadAll(tt.stdout)
	})
	tt.WaitExit()
	if tt.Cleanup != nil {
		tt.Cleanup()
	}
	if len(output) > 0 {
		tt.Errorf("Unmatched stdout text:\n%s", output)
	}
}

func (tt *TestCmd) WaitExit() {
	tt.Err = tt.cmd.Wait()
}

func (tt *TestCmd) Interrupt() {
	tt.Err = tt.cmd.Process.Signal(os.Interrupt)
}

// ExitStatus 公开进程的操作系统退出代码
// 它只会在进程完成后返回一个有效值。
func (tt *TestCmd) ExitStatus() int {
	if tt.Err != nil {
		exitErr := tt.Err.(*exec.ExitError)
		if exitErr != nil {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
	}
	return 0
}

// StderrText 返回到目前为止写入的任何 stderr 输出。
// 返回的文本包含 ExpectExit 之后的所有日志行
// 回。
func (tt *TestCmd) StderrText() string {
	tt.stderr.mu.Lock()
	defer tt.stderr.mu.Unlock()
	return tt.stderr.buf.String()
}

func (tt *TestCmd) CloseStdin() {
	tt.stdin.Close()
}

func (tt *TestCmd) Kill() {
	tt.cmd.Process.Kill()
	if tt.Cleanup != nil {
		tt.Cleanup()
	}
}

func (tt *TestCmd) withKillTimeOut(fn func()) {
	timeout := time.AfterFunc(5*time.Second, func() {
		tt.Log("Killing the child process (timeout)")
		tt.Kill()
	})
	defer timeout.Stop()
	fn()
}

// testlogger 通过 t.Log 记录所有写入的行，并且
// 收集它们供以后检查。
type testlogger struct {
	t    *testing.T
	mu   sync.Mutex
	buf  bytes.Buffer
	name string
}

func (tl *testlogger) Write(b []byte) (n int, err error) {
	lines := bytes.Split(b, []byte("\n"))
	for _, line := range lines {
		if len(line) > 0 {
			tl.t.Logf("(stderr:%v) %s", tl.name, line)
		}
	}
	tl.mu.Lock()
	tl.buf.Write(b)
	tl.mu.Unlock()
	return len(b), err
}

// runeTee 将读取的文本收集到 buf 中。
type runeTee struct {
	in interface {
		io.Reader
		io.ByteReader
		io.RuneReader
	}
	buf bytes.Buffer
}

func (rtee *runeTee) Read(b []byte) (n int, err error) {
	n, err = rtee.in.Read(b)
	rtee.buf.Write(b[:n])
	return n, err
}

func (rtee *runeTee) ReadRune() (r rune, size int, err error) {
	r, size, err = rtee.in.ReadRune()
	if err == nil {
		rtee.buf.WriteRune(r)
	}
	return r, size, err
}

func (rtee *runeTee) ReadByte() (b byte, err error) {
	b, err = rtee.in.ReadByte()
	if err == nil {
		rtee.buf.WriteByte(b)
	}
	return b, err
}
