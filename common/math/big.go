package math

import (
	"fmt"
	"math/big"
)

// 各种大整数限制值。
var (
	tt255     = BigPow(2, 255)
	tt256     = BigPow(2, 256)
	tt256m1   = new(big.Int).Sub(tt256, big.NewInt(1))
	tt63      = BigPow(2, 63)
	MaxBig256 = new(big.Int).Set(tt256m1)
	MaxBig63  = new(big.Int).Sub(tt63, big.NewInt(1))
)

const (
	// big.Word 中的位数
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	// big.Word 中的字节数
	wordBytes = wordBits / 8
)

// HexOrDecimal256 将 big.Int 编组为十六进制或十进制。
type HexOrDecimal256 big.Int

// NewHexOrDecimal256 创建一个新的 HexOrDecimal256
func NewHexOrDecimal256(x int64) *HexOrDecimal256 {
	b := big.NewInt(x)
	h := HexOrDecimal256(*b)
	return &h
}

// UnmarshalText 实现了 encoding.TextUnmarshaler。
func (i *HexOrDecimal256) UnmarshalText(input []byte) error {
	bigint, ok := ParseBig256(string(input))
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = HexOrDecimal256(*bigint)
	return nil
}

// MarshalText 实现了 encoding.TextMarshaler。
func (i *HexOrDecimal256) MarshalText() ([]byte, error) {
	if i == nil {
		return []byte("0x0"), nil
	}
	return []byte(fmt.Sprintf("%#x", (*big.Int)(i))), nil
}

// Decimal256 将 big.Int 解组为十进制字符串。解组时，
// 但是它接受以“0x”为前缀（十六进制编码）或无前缀（十进制）
type Decimal256 big.Int

// NewHexOrDecimal256 creates a new Decimal256
func NewDecimal256(x int64) *Decimal256 {
	b := big.NewInt(x)
	d := Decimal256(*b)
	return &d
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (i *Decimal256) UnmarshalText(input []byte) error {
	bigint, ok := ParseBig256(string(input))
	if !ok {
		return fmt.Errorf("invalid hex or decimal integer %q", input)
	}
	*i = Decimal256(*bigint)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (i *Decimal256) MarshalText() ([]byte, error) {
	return []byte(i.String()), nil
}

// String implements Stringer.
func (i *Decimal256) String() string {
	if i == nil {
		return "0"
	}
	return fmt.Sprintf("%#d", (*big.Int)(i))
}

// ParseBig256 将 s 解析为十进制或十六进制语法中的 256 位整数。
// 接受前导零。空字符串解析为零。
func ParseBig256(s string) (*big.Int, bool) {
	if s == "" {
		return new(big.Int), true
	}
	var bigint *big.Int
	var ok bool
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		bigint, ok = new(big.Int).SetString(s[2:], 16)
	} else {
		bigint, ok = new(big.Int).SetString(s, 10)
	}
	if ok && bigint.BitLen() > 256 {
		bigint, ok = nil, false
	}
	return bigint, ok
}

// BigPow 返回 a ** b 作为一个大整数。
func BigPow(a, b int64) *big.Int {
	r := big.NewInt(a)
	return r.Exp(r, big.NewInt(b), nil)
}

// BigMax 返回 x 或 y 中较大的一个。
func BigMax(x, y *big.Int) *big.Int {
	if x.Cmp(y) < 0 {
		return y
	}
	return x
}

// BigMin 返回 x 或 y 中较小的一个。
func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return y
	}
	return x
}

// FirstBitSet 返回 v 中第一个 1 位的索引，从 LSB 开始计算。
func FirstBitSet(v *big.Int) int {
	for i := 0; i < v.BitLen(); i++ {
		if v.Bit(i) > 0 {
			return i
		}
	}
	return v.BitLen()
}

// PaddedBigBytes 将大整数编码为大端字节切片。长度
// 切片至少有 n 个字节。
func PaddedBigBytes(bigint *big.Int, n int) []byte {
	if bigint.BitLen()/8 >= 8 {
		return bigint.Bytes()
	}
	ret := make([]byte, 8)
	ReadBits(bigint, ret)
	return ret
}

// ReadBits 将 bigint 的绝对值编码为 big-endian 字节。来电者必须确保
// buf 有足够的空间。如果 buf 太短，结果将不完整。
func ReadBits(bigint *big.Int, buf []byte) {
	i := len(buf)
	for _, d := range bigint.Bits() {
		for j := 0; j < wordBytes && i > 0; j++ {
			i--
			buf[i] = byte(d)
			d >>= 8
		}
	}
}

// bigEndianByteAt 返回位置 n 处的字节，
// 在 Big-Endian 编码中
// 所以 n==0 返回最低有效字节
func bigEndianByteAt(bigint *big.Int, n int) byte {
	words := bigint.Bits()
	// 检查字节将驻留在的字桶
	i := n / wordBytes
	if i >= len(words) {
		return byte(0)
	}
	word := words[i]
	//字节的偏移量
	shift := 8 * uint(n%wordBytes)

	return byte(word >> shift)
}

// Byte 返回位置 n 处的字节，
// 使用 Little-Endian 编码中提供的 padlength。
// n==0 返回 MSB
// 例子：bigint '5', padlength 32, n=31 => 5
func Byte(bigint *big.Int, padlength, n int) byte {
	if n >= padlength {
		return byte(0)
	}
	return bigEndianByteAt(bigint, padlength-1-n)
}

// U256 编码为 256 位二进制补码。这种操作是破坏性的。
func U256(x *big.Int) *big.Int {
	return x.And(x, tt256m1)
}

// U256Bytes 将一个 big Int 转换为一个 256 位的 EVM 数。
// 这个操作是破坏性的。
func U256Bytes(n *big.Int) []byte {
	return PaddedBigBytes(U256(n), 32)
}

// S256 将 x 解释为二进制补码。
// x 不得超过 256 位（如果超出则结果未定义）且未被修改。
//
// S256(0) = 0
// S256(1) = 1
// S256(2**255) = -2**255
// S256(2**256-1) = -1
func S256(x *big.Int) *big.Int {
	if x.Cmp(tt255) < 0 {
		return x
	}
	return new(big.Int).Sub(x, tt256)
}

// Exp 通过平方实现求幂。
// Exp 返回一个新分配的大整数并且不会改变
//基数或指数。结果被截断为 256 位。
//
// 由@karalabe 和@chfast 提供
func Exp(base, exponent *big.Int) *big.Int {
	result := big.NewInt(1)

	for _, word := range exponent.Bits() {
		for i := 0; i < wordBits; i++ {
			if word&1 == 1 {
				U256(result.Mul(result, base))
			}
			U256(base.Mul(base, base))
			word >>= 1
		}
	}
	return result
}
