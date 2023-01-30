/*
包 hexutil 实现了带有 0x 前缀的十六进制编码。
以太坊 RPC API 使用此编码来传输 JSON 有效负载中的二进制数据。
# 编码规则
所有十六进制数据必须有前缀“0x”。
对于字节片，十六进制数据的长度必须是偶数。空字节片
编码为“0x”。
整数使用最少的数字编码（无前导零数字）。他们的
编码长度可能不均匀。数字零编码为“0x0”。
*/
package hexutil

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
)

const uintBits = 32 << (uint(^uint(0)) >> 63)

//Errors

var (
	ErrEmptyString   = &decError{"empty hex string"}
	ErrSyntax        = &decError{"invalid hex string"}
	ErrMissingPrefix = &decError{"hex string without 0x prefix"}
	ErrOddLength     = &decError{"hex string of odd length"}
	ErrEmptyNumber   = &decError{"hex string \"0x\""}
	ErrLeadingZero   = &decError{"hex number with leading zero digits"}
	ErrUint64Range   = &decError{"hex number > 64 bits"}
	ErrUintRange     = &decError{fmt.Sprintf("hex number > %d bits", uintBits)}
	ErrBig256Range   = &decError{"hex number > 256 bits"}
)

type decError struct{ msg string }

func (err decError) Error() string { return err.msg }

// Decode 解码带有 0x 前缀的十六进制字符串。
func Decode(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrEmptyString
	}
	if !has0xPrefix(input) {
		return nil, ErrMissingPrefix
	}
	b, err := hex.DecodeString(input[2:])
	if err != nil {
		err = mapError(err)
	}
	return b, err
}

// MustDecode 解码带有 0x 前缀的十六进制字符串。它因无效输入而恐慌。
func MustDecode(input string) []byte {
	dec, err := Decode(input)
	if err != nil {
		panic(err)
	}
	return dec
}

// Encode 将 b 编码为带有 0x 前缀的十六进制字符串
func Encode(b []byte) string {
	enc := make([]byte, len(b)*2+2)
	copy(enc, "0x")
	hex.Encode(enc[2:], b)
	return string(enc)
}

// DecodeUint64 解码带有 0x 前缀的十六进制字符串作为数量。
func DecodeUint64(input string) (uint64, error) {
	raw, err := checkNumber(input)
	if err != nil {
		return 0, err
	}
	dec, err := strconv.ParseUint(raw, 16, 64)
	if err != nil {
		err = mapError(err)
	}
	return dec, err
}


// MustDecodeUint64 解码带有 0x 前缀的十六进制字符串作为数量。
// 它因无效输入而恐慌。
func MustDecodeUint64(input string) uint64 {
	dec, err := DecodeUint64(input)
	if err != nil {
		panic(err)
	}
	return dec
}


// EncodeUint64 将 i 编码为带有 0x 前缀的十六进制字符串。
func EncodeUint64(i uint64) string {
	enc := make([]byte, 2, 10)
	copy(enc, "0x")
	return string(strconv.AppendUint(enc, i, 16))
}

var bigWordNibbles int

func init() {
	// 这是一种计算 big.Word 所需的半字节数的奇怪方法。
	// 通常的方法是使用常量运算，但 go vet 无法处理。
	b, _ := new(big.Int).SetString("FFFFFFFFFF", 16)
	switch len(b.Bits()) {
	case 1:
		bigWordNibbles = 16
	case 2:
		bigWordNibbles = 8
	default:
		panic("weird big.Word size")
	}
}


// DecodeBig 解码带有 0x 前缀的十六进制字符串作为数量。
// 不接受大于 256 位的数字
func DecodeBig(input string) (*big.Int, error) {
	raw, err := checkNumber(input)
	if err != nil {
		return nil, err
	}
	if len(raw) > 64 {
		return nil, ErrBig256Range
	}
	words := make([]big.Word, len(raw)/bigWordNibbles+1)
	end := len(raw)
	for i := range words {
		start := end - bigWordNibbles
		if start < 0 {
			start = 0
		}
		for ri := start; ri < end; ri++ {
			nib := decodeNibble(raw[ri])
			if nib == badNibble {
				return nil, ErrSyntax
			}
			words[i] *= 16
			words[i] += big.Word(nib)
		}
		end = start
	} 
	dec := new(big.Int).SetBits(words)
	return dec, nil
} 


// MustDecodeBig 解码带有 0x 前缀的十六进制字符串作为数量。
// 它因无效输入而恐慌。
func MustDecodeBig(input string) *big.Int {
	dec, err := DecodeBig(input)
	if err != nil {
		panic(err)
	}
	return dec
}


// EncodeBig 将 bigint 编码为带有 0x 前缀的十六进制字符串。
func EncodeBig(bigint *big.Int) string {
	if sign := bigint.Sign(); sign == 0 {
		return "0x0"
	} else if sign > 0 {
		return "0x" + bigint.Text(16)
	} else {
		return "-0x" + bigint.Text(16)[1:]
	}
}
 
func checkNumber(input string) (raw string, err error) {
	if len(input) == 0 {
		return "", ErrEmptyNumber
	}
	if !has0xPrefix(input) {
		return "", ErrMissingPrefix
	}
	input = input[2:]
	if len(input) == 0 {
		return "", ErrEmptyNumber
	}
	if len(input) > 1 && input[0] == '0' {
		return "", ErrLeadingZero
	}
	return input, nil
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

const badNibble = ^uint64(0)

func decodeNibble(in byte) uint64 {
	switch {
	case in >= '0' && in <= '9' :
		return uint64(in - '0')
	case in >= 'A' && in <= 'F':
		return uint64(in - 'A' + 10)
	case in >= 'a' && in <= 'f':
		return uint64(in - 'a' + 10)
	default:
		return badNibble
	}
}

func mapError(err error) error {
	if err, ok := err.(*strconv.NumError); ok {
		switch err.Err {
		case strconv.ErrRange:
			return ErrUintRange
		case strconv.ErrSyntax:
			return ErrSyntax
		}
	}
	if _, ok := err.(hex.InvalidByteError); ok {
		return ErrSyntax
	}
	if err == hex.ErrLength {
		return ErrOddLength
	}
	return err
}
