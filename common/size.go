package common

import (
	"fmt"
)

// StorageSize 是支持用户友好的浮点值的包装器
// 格式化。
type StorageSize float64

// String 实现了 stringer 接口。
func (s StorageSize) String() string {
	if s > 1099511627776 {
		return fmt.Sprintf("%.2f TiB", s/1099511627776)
	} else if s > 1073741824 {
		return fmt.Sprintf("%.2f GiB", s/1073741824)
	} else if s > 1048576 {
		return fmt.Sprintf("%.2f MiB", s/1048576)
	} else if s > 1024 {
		return fmt.Sprintf("%.2f KiB", s/1024)
	} else {
		return fmt.Sprintf("%.2f B", s)
	}
}

// TerminalString 实现 log.TerminalStringer，为控制台格式化一个字符串
// 记录期间的输出
func (s StorageSize) TerminalString() string {
	if s > 1099511627776 {
		return fmt.Sprintf("%.2fTiB", s/1099511627776)
	} else if s > 1073741824 {
		return fmt.Sprintf("%.2fGiB", s/1073741824)
	} else if s > 1048576 {
		return fmt.Sprintf("%.2fMiB", s/1048576)
	} else if s > 1024 {
		return fmt.Sprintf("%.2fKiB", s/1024)
	} else {
		return fmt.Sprintf("%.2fB", s)
	}
}