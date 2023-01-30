package hexutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
)

var (
	bytesT  = reflect.TypeOf(Bytes(nil))
	bigT    = reflect.TypeOf((*Big)(nil))
	uintT   = reflect.TypeOf(Uint(0))
	uint64T = reflect.TypeOf(Uint64(0))
)

// 字节编组/解组为带有 0x 前缀的 JSON 字符串。
// 空切片编组为“0x”
type Bytes []byte

// MarshalText 实现 encoding.TextMarshaler
func (b Bytes) MarshalText() ([]byte, error) {
	result := make([]byte, len(b)*2+2)
	copy(result, `0x`)
	hex.Encode(result[2:], b)
	return result, nil
}

// UnmarshalJSON 实现了 json.Unmarshaler。
func (b *Bytes) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(bytesT)
	}
	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), bytesT)
}

// UnmarshalText 实现了 encoding.TextUnmarshaler
func (b *Bytes) UnmarshalText(input []byte) error {
	raw, err := checkText(input, true)
	if err != nil {
		return err
	}
	dec := make([]byte, len(raw)/2)
	if _, err = hex.Decode(dec, raw); err != nil {
		err = mapError(err)
	} else {
		*b = dec
	}
	return err
}

// String 返回 b 的十六进制编码。
func (b Bytes) String() string {
	return Encode(b)
}

// 如果 Bytes 实现指定的 GraphQL 类型，则 ImplementsGraphQLType 返回 true。
func (b Bytes) ImplementsGraphQLType(name string) bool { return name == "Bytes" }

// UnmarshalGraphQL 解组提供的 GraphQL 查询数据。
func (b *Bytes) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		data, err := Decode(input)
		if err != nil {
			return err
		}
		*b = data
	default:
		err = fmt.Errorf("unexpected type %T for Bytes", input)
	}
	return err
}

// UnmarshalFixedJSON 将输入解码为带有 0x 前缀的字符串。出线长度
// 确定所需的输入长度。该函数通常用于实现
// 固定大小类型的 UnmarshalJSON 方法。
func UnmarshalFixedJSON(typ reflect.Type, input, out []byte) error {
	if !isString(input) {
		return errNonString(typ)
	}
	return wrapTypeError(UnmarshalFixedText(typ.String(), input[1:len(input)-1], out), typ)
}

// UnmarshalFixedText 将输入解码为带有 0x 前缀的字符串。出线长度
// 确定所需的输入长度。该函数通常用于实现
// 固定大小类型的 UnmarshalText 方法。
func UnmarshalFixedText(typname string, input, out []byte) error {
	raw, err := checkText(input, true)
	if err != nil {
		return err
	}
	if len(raw)/2 != len(out) {
		return fmt.Errorf("hex string has length %d, want %d for %s", len(raw), len(out)*2, typname)
	}
	// 在修改之前预先验证语法。
	for _, b := range raw {
		if decodeNibble(b) == badNibble {
			return ErrSyntax
		}
	}
	hex.Decode(out, raw)
	return nil
}

// UnmarshalFixedUnprefixedText 将输入解码为带有可选 0x 前缀的字符串。这
// out 的长度决定了所需的输入长度。该函数常用于
// 为固定大小的类型实现 UnmarshalText 方法。
func UnmarshalFixedUnprefixedText(typname string, input, out []byte) error {
	raw, err := checkText(input, false)
	if err != nil {
		return err
	}
	if len(raw)/2 != len(out) {
		return fmt.Errorf("hex string has length %d, want %d for %s", len(raw), len(out)*2, typname)
	}
	// 在修改之前预先验证语法。
	for _, b := range raw {
		if decodeNibble(b) == badNibble {
			return ErrSyntax
		}
	}
	hex.Decode(out, raw)
	return nil
}

// Big marshals/unmarshals 作为带有 0x 前缀的 JSON 字符串。
// 零值封送为“0x0”。
//
// 目前不支持负整数。试图指挥他们将
// 返回一个错误。大于 256 位的值会被 Unmarshal 拒绝，但会被
// 编组没有错误。
type Big big.Int

// MarshalText 实现 encoding.TextMarshaler
func (b Big) MarshalText() ([]byte, error) {
	return []byte(EncodeBig((*big.Int)(&b))), nil
}

// UnmarshalJSON 实现了 json.Unmarshaler。
func (b *Big) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(bigT)
	}
	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), bigT)
}

// UnmarshalText 实现 encoding.TextUnmarshaler
func (b *Big) UnmarshalText(input []byte) error {
	raw, err := checkNumberText(input)
	if err != nil {
		return err
	}
	if len(raw) > 64 {
		return ErrBig256Range
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
				return ErrSyntax
			}
			words[i] *= 16
			words[i] += big.Word(nib)
		}
		end = start
	}
	var dec big.Int
	dec.SetBits(words)
	*b = (Big)(dec)
	return nil
}

// ToInt 将 b 转换为 big.Int
func (b *Big) ToInt() *big.Int {
	return (*big.Int)(b)
}

// String 返回 b 的十六进制编码。
func (b *Big) String() string {
	return EncodeBig(b.ToInt())
}

// 如果 Big 实现了提供的 GraphQL 类型，则 ImplementsGraphQLType 返回 true。
func (b Big) ImplementsGraphQLType(name string) bool { return name == "BigInt" }

// UnmarshalGraphQL 解组提供的 GraphQL 查询数据。
func (b *Big) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		return b.UnmarshalText([]byte(input))
	case int32:
		var num big.Int
		num.SetInt64(int64(input))
		*b = Big(num)
	default:
		err = fmt.Errorf("unexpected type %T for BigInt", input)
	}
	return err
}

// Uint64 编组/解组为带有 0x 前缀的 JSON 字符串。
// 零值封送为“0x0”
type Uint64 uint64

// MarshalText 实现了 encoding.TextMarshaler。
func (b Uint64) MarshalText() ([]byte, error) {
	buf := make([]byte, 2, 10)
	copy(buf, `0x`)
	buf = strconv.AppendUint(buf, uint64(b), 16)
	return buf, nil
}

// UnmarshalJSON 实现了 json.Unmarshaler。
func (b *Uint64) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(uint64T)
	}
	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), uint64T)
}

// UnmarshalText 实现 encoding.TextUnmarshaler
func (b *Uint64) UnmarshalText(input []byte) error {
	raw, err := checkNumberText(input)
	if err != nil {
		return err
	}
	if len(raw) > 16 {
		return ErrUint64Range
	}
	var dec uint64
	for _, byte := range raw {
		nib := decodeNibble(byte)
		if nib == badNibble {
			return ErrSyntax
		}
		dec *= 16
		dec += nib
	}
	*b = Uint64(dec)
	return nil
}

// String 返回 b 的十六进制编码。
func (b Uint64) String() string {
	return EncodeUint64(uint64(b))
}

// 如果 Uint64 实现了提供的 GraphQL 类型，则 ImplementsGraphQLType 返回 true。
func (b Uint64) ImplementsGraphQLType(name string) bool { return name == "Long" }

// UnmarshalGraphQL 解组提供的 GraphQL 查询数据。
func (b *Uint64) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		return b.UnmarshalText([]byte(input))
	case int32:
		*b = Uint64(input)
	default:
		err = fmt.Errorf("unexpected type %T for Long", input)
	}
	return err
}

// Uint marshals/unmarshals 作为带有 0x 前缀的 JSON 字符串。
// 零值封送为“0x0”
type Uint uint

// MarshalText 实现了 encoding.TextMarshaler。
func (b Uint) MarshalText() ([]byte, error) {
	return Uint64(b).MarshalText()
}

// UnmarshalJSON 实现了 json.Unmarshaler。
func (b *Uint) UnmarshalJSON(input []byte) error {
	if !isString(input) {
		return errNonString(uintT)
	}
	return wrapTypeError(b.UnmarshalText(input[1:len(input)-1]), uintT)
} 

// UnmarshalText 实现了 encoding.TextUnmarshaler。
func (b *Uint) UnmarshalText(input []byte) error {
	var u64 Uint64
	err := u64.UnmarshalText(input)
	if u64 > Uint64(^uint(0)) || err == ErrUint64Range {
		return ErrUintRange
	} else if err != nil {
		return err
	}
	*b = Uint(u64)
	return nil
}

// String 返回 b 的十六进制编码。
func (b Uint) String() string {
	return EncodeUint64(uint64(b))
}

func isString(input []byte) bool {
	return len(input) >= 2 && input[0] == '"' && (input[len(input)-1] == '"')
}

func bytesHave0xPrefix(input []byte) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}

func checkText(input []byte, wantPrefix bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, nil // 允许空字符串
	}
	if bytesHave0xPrefix(input) {
		input = input[2:]
	} else if wantPrefix {
		return nil, ErrMissingPrefix
	}
	if len(input)%2 != 0 {
		return nil, ErrOddLength
	}
	return input, nil
}

func checkNumberText(input []byte) (raw []byte, err error) {
	if len(input) == 0 {
		return nil, nil
	}
	if !bytesHave0xPrefix(input) {
		return nil, ErrMissingPrefix
	}
	input = input[2:]
	if len(input) == 0 {
		return nil, ErrEmptyNumber
	}
	if len(input) > 1 && input[0] == '0' {
		return nil, ErrLeadingZero
	}
	return input, nil
}

func wrapTypeError(err error, typ reflect.Type) error {
	if _, ok := err.(*decError); ok {
		return &json.UnmarshalTypeError{Value: err.Error(), Type: typ}
	}
	return err
}

func errNonString(typ reflect.Type) error {
	return &json.UnmarshalTypeError{Value: "non-string", Type: typ}
}
