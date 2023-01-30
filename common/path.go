package common

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// MakeName 创建一个遵循以太坊约定的节点名称
// 对于这样的名字。它添加了操作系统名称和 Go 运行时版本
// 名字。
func MakeName(name, version string) string {
	return fmt.Sprintf("%s/v%s/%s/%s", name, version, runtime.GOOS, runtime.Version())
}

// FileExist 检查文件路径是否存在文件。
func FileExist(filePath string) bool {
	_, err := os.Stat(filePath)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	
	return true
}

// AbsolutePath 返回 datadir + 文件名，如果是绝对路径则返回文件名。
func AbsolutePath(datadir string, filename string) string {
	if filepath.IsAbs(filename) {
		return filename
	}
	return filepath.Join(datadir, filename)
}
