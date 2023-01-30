// 包 common 包含各种辅助函数。
package common

import (
	"encoding/hex"
	"errors"
	"flychain/common/hexutil"
)

// FromHex 返回由十六进制字符串 s 表示的字节。
// s 可以以“0x”为前缀。
func FromHex(s string) []byte {
	if has0xPrefix(s) {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return Hex2Bytes(s)
}

// CopyBytes 返回所提供字节的精确副本。
func CopyBytes(b []byte) (copiedBytes []byte) {
	if b == nil {
		return nil
	}
	copiedBytes = make([]byte, len(b))
	copy(copiedBytes, b)

	return
}

// has0xPrefix 验证 str 以 '0x' 或 '0X' 开头。
func has0xPrefix(str string) bool {
	return len(str) >= 2 && str[0] == '0' && (str[1] == 'x' || str[1] == 'X')
}

// isHexCharacter 返回 bool of c 是一个有效的十六进制数。
func isHexCharacter(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

// isHex 验证每个字节是否为有效的十六进制字符串。
func isHex(str string) bool {
	if len(str)%2 != 0 {
		return false
	}
	for _, c := range []byte(str) {
		if !isHexCharacter(c) {
			return false
		}
	}
	return true
}

// Bytes2Hex 返回 d 的十六进制编码。
func Bytes2Hex(d []byte) string {
	return hex.EncodeToString(d)
}

// Hex2Bytes 返回由十六进制字符串 str 表示的字节。
func Hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)
	return h
}

// Hex2BytesFixed 返回指定固定长度的字节 flen。
func Hex2BytesFixed(str string, flen int) []byte {
	h, _ := hex.DecodeString(str)
	if len(h) == flen {
		return h
	}
	if len(h) > flen {
		return h[len(h)-flen:]
	}
	hh := make([]byte, flen)
	copy(hh[flen-len(h):flen], h) 
	return hh
}


// ParseHexOrString 尝试对 b 进行十六进制解码，但如果缺少前缀，它只会返回原始字节
func ParseHexOrString(str string) ([]byte, error) {
	b, err := hexutil.Decode(str) 
	if errors.Is(err, hexutil.ErrMissingPrefix) {
		return []byte(str), nil
	}
	return b, err
}

// RightPadBytes 零填充切片到右边直到长度 l。
func RightPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l) 
	copy(padded, slice)

	return padded
}

// LeftPadBytes 零填充切片到左边直到长度 l。
func LeftPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}
	padded := make([]byte, l) 
	copy(padded[l-len(slice):], slice) 
	
	return padded
}

// TrimLeftZeroes 返回不带前导零的 s 的子切片
func TrimLeftZeroes(s []byte) []byte {
	idx := 0
	for ; idx < len(s); idx++ {
		if s[idx] != 0 {
			break
		}
	}
	return s[idx:]
}


// TrimRightZeroes 返回不带尾随零的 s 的子切片
func TrimRightZeroes(s []byte) []byte {
	idx := len(s)
	for ; idx > 0 ; idx-- {
		if s[idx-1] != 0 {
			break
		}
	}
	return s[:idx]
}