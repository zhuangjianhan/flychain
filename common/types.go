package common

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flychain/common/hexutil"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"
	"strings"

	"golang.org/x/crypto/sha3"
)

// 哈希和地址的长度（以字节为单位）。
const (
	// HashLength 是哈希的预期长度
	HashLength = 32
	// AddressLength 是地址的预期长度
	AddressLength = 20
)

var (
	hashT    = reflect.TypeOf(Hash{})
	addressT = reflect.TypeOf(Address{})
)

// Hash 表示任意数据的 32 字节 Keccak256 哈希。
type Hash [HashLength]byte

// BytesToHash 将 b 设置为散列。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

// BigToHash 将 b 的字节表示设置为散列。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func BigToHash(b *big.Int) Hash { return BytesToHash(b.Bytes()) }

// HexToHash 将 s 的字节表示设置为散列。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func HexToHash(s string) Hash { return BytesToHash(FromHex(s)) }

// Bytes 获取底层哈希的字节表示。
func (h Hash) Bytes() []byte { return h[:] }

// Big 将散列转换为大整数。
func (h Hash) Big() *big.Int { return new(big.Int).SetBytes(h[:]) }

// Hex 将哈希值转换为十六进制字符串。
func (h Hash) Hex() string { return hexutil.Encode(h[:]) }

// TerminalString 实现 log.TerminalStringer，为控制台格式化一个字符串
// 记录期间的输出。
func (h Hash) TerminalString() string {
	return fmt.Sprintf("%x..%x", h[:3], h[29:])
}

// String 实现了 stringer 接口，并且在以下情况下也被记录器使用
// 将完整日志记录到一个文件中。
func (h Hash) String() string {
	return h.Hex()
}

// 格式实现 fmt.Formatter。
// Hash 支持 %v、%s、%q、%x、%X 和 %d 格式动词。
func (h Hash) Format(s fmt.State, c rune) {
	hexb := make([]byte, 2+len(h)*2)
	copy(hexb, "0x")
	hex.Encode(hexb[2:], h[:])

	switch c {
	case 'x', 'X':
		if !s.Flag('#') {
			hexb = hexb[2:]
		}
		if c == 'X' {
			hexb = bytes.ToUpper(hexb)
		}
		fallthrough
	case 'v', 's':
		s.Write(hexb)
	case 'q':
		q := []byte{'"'}
		s.Write(q)
		s.Write(hexb)
		s.Write(q)
	case 'd':
		fmt.Fprint(s, ([len(h)]byte(h)))
	default:
		fmt.Fprintf(s, "%%!%c(hash=%x)", c, h)
	}
}

// UnmarshalText 解析十六进制语法中的散列。
func (h *Hash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("Hash", input, h[:])
}

// UnmarshalJSON 解析十六进制语法中的散列。
func (h *Hash) UnmarshalJSON(input []byte) error {
	return hexutil.UnmarshalFixedJSON(hashT, input, h[:])
}

// MarshalText 返回 h 的十六进制表示。
func (h Hash) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// SetBytes 将散列设置为 b 的值。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}

// 生成工具 testing/quick.Generator。
func (h Hash) Generate(rand *rand.Rand, size int) reflect.Value {
	m := rand.Intn(len(h))
	for i := len(h) - 1; i > m; i-- {
		h[i] = byte(rand.Uint32())
	}
	return reflect.ValueOf(h)
}

// Scan 为数据库/sql 实现 Scanner。
func (h *Hash) Scan(src interface{}) error {
	srcB, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("can't scan %T into Hash", src)
	}
	if len(srcB) != HashLength {
		return fmt.Errorf("can't scan []byte of len %d into Hash, want %d", len(srcB), HashLength)
	}
	copy(h[:], srcB)
	return nil
}

// Value 为数据库/sql 实现估值器。
func (h Hash) Value() (driver.Value, error) {
	return h[:], nil
}

// 如果 Hash 实现指定的 GraphQL 类型，则 ImplementsGraphQLType 返回 true。
func (Hash) ImplementsGraphQLType(name string) bool { return name == "Bytes32" }

// UnmarshalGraphQL 解组提供的 GraphQL 查询数据。
func (h *Hash) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		err = h.UnmarshalText([]byte(input))
	default:
		err = fmt.Errorf("unexpected type %T for Hash", input)
	}
	return err
}

// UnprefixedHash 允许编组没有 0x 前缀的哈希。
type UnprefixedHash Hash

// UnmarshalText 从十六进制解码散列。 0x 前缀是可选的。
func (h *UnprefixedHash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedHash", input, h[:])
}

// MarshalText 将哈希编码为十六进制。
func (h UnprefixedHash) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(h[:])), nil
}

/////////// Address

// address 表示以太坊账户的 20 字节地址
type Address [AddressLength]byte

// BytesToAddress 返回值为 b 的地址。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func BytesToAddress(b []byte) Address {
	var a Address
	a.SetBytes(b)
	return a
}

// BigToAddress 返回字节值为 b 的地址。
// 如果 b 大于 len(h)，b 将从左边裁剪。
func BigToAddress(b *big.Int) Address { return BytesToAddress(b.Bytes()) }

// HexToAddress 返回字节值为 s 的地址。
// 如果 s 大于 len(h)，s 将从左边裁剪。
func HexToAddress(s string) Address { return BytesToAddress(FromHex(s)) }

// IsHexAddress 验证字符串是否可以表示有效的十六进制编码
// 以太坊地址与否。
func IsHexAddress(s string) bool {
	if has0xPrefix(s) {
		s = s[2:]
	}
	return len(s) == 2*AddressLength && isHex(s)
}

// Bytes 获取底层地址的字符串表示形式。
func (a Address) Bytes() []byte { return a[:] }

// Hash 通过用零填充它来将地址转换为哈希。
func (a Address) Hash() Hash { return BytesToHash(a[:]) }

// Big 将地址转换为大整数。
func (a Address) Big() *big.Int { return new(big.Int).SetBytes(a[:]) }

// Hex 返回地址的符合 EIP55 的十六进制字符串表示形式。
func (a Address) Hex() string {
	return string(a.checksumHex())
}

// 字符串实现 fmt.Stringer。
func (a Address) String() string {
	return a.Hex()
}

func (a *Address) checksumHex() []byte {
	buf := a.hex()

	//计算校验和
	sha := sha3.NewLegacyKeccak256()
	sha.Write(buf[2:])
	hash := sha.Sum(nil)
	for i := 2; i < len(buf); i++ {
		hashByte := hash[(i-2)/2]
		if i%2 == 0 {
			hashByte = hashByte >> 4
		} else {
			hashByte &= 0xf
		}
		if buf[i] > '9' && hashByte > 7 {
			buf[i] -= 32
		}
	}
	return buf[:]
}

func (a Address) hex() []byte {
	var buf [len(a)*2 + 2]byte
	copy(buf[:2], "0x")
	hex.Encode(buf[2:], a[:])
	return buf[:]
}

// 格式实现 fmt.Formatter。
// 地址支持 %v、%s、%q、%x、%X 和 %d 格式动词。
func (a Address) Format(s fmt.State, c rune) {
	switch c {
	case 'v', 's':
		s.Write(a.checksumHex())
	case 'q':
		q := []byte{'"'}
		s.Write(q)
		s.Write(a.checksumHex())
		s.Write(q)
	case 'x', 'X':
		// %x 禁用校验和。
		hex := a.hex()
		if !s.Flag('#') {
			hex = hex[2:]
		}
		if c == 'X' {
			hex = bytes.ToUpper(hex)
		}
		s.Write(hex)
	case 'd':
		fmt.Fprint(s, ([len(a)]byte)(a))
	default:
		fmt.Fprintf(s, "%%!%c(address=%x)", c, a)
	}
}

// SetBytes 将地址设置为 b 的值。
// 如果 b 大于 len(a)，b 将从左边裁剪。
func (a *Address) SetBytes(b []byte) {
	if len(b) > len(a) {
		b = b[len(b)-AddressLength:]
	}
	copy(a[AddressLength-len(b):], b)
}

// MarshalText 返回 a 的十六进制表示。
func (a Address) MarshalText() ([]byte, error) {
	return hexutil.Bytes(a[:]).MarshalText()
}

// UnmarshalText 解析十六进制语法中的散列。
func (a *Address) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("Address", input, a[:])
}

// UnmarshalJSON 解析十六进制语法中的散列。
func (a *Address) UnmarshalJSON(input []byte) error {
	return hexutil.UnmarshalFixedJSON(addressT, input, a[:])
}

// Scan 为数据库/sql 实现 Scanner。
func (a *Address) Scan(src interface{}) error {
	srcB, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("can't scan %T into Address", src)
	}
	if len(srcB) != AddressLength {
		return fmt.Errorf("can't scan []byte of len %d into Address, want %d", len(srcB), AddressLength)
	}
	copy(a[:], srcB)
	return nil
}

// Value 为数据库/sql 实现 valuer
func (a Address) Value() (driver.Value, error) {
	return a[:], nil
}

// 如果 Hash 实现指定的 GraphQL 类型，则 ImplementsGraphQLType 返回 true。
func (a Address) ImplementsGraphQLType(name string) bool { return name == "Address" }

// UnmarshalGraphQL 解组提供的 GraphQL 查询数据。
func (a *Address) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		err = a.UnmarshalText([]byte(input))
	default:
		err = fmt.Errorf("unexpected type %T for Address", input)
	}
	return err
}

// UnprefixedAddress 允许封送没有 0x 前缀的地址。
type UnprefixedAddress Address

// UnmarshalText 从十六进制解码地址。 0x 前缀是可选的。
func (a *UnprefixedAddress) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedAddress", input, a[:])
}

// MarshalText 将地址编码为十六进制。
func (a UnprefixedAddress) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(a[:])), nil
}

// MixedcaseAddress 保留原始字符串，可能是也可能不是
//正确校验和
type MixedcaseAddress struct {
	addr     Address
	original string
}

// NewMixedcaseAddress 构造函数（主要用于测试）
func NewMixedcaseAddress(addr Address) MixedcaseAddress {
	return MixedcaseAddress{addr: addr, original: addr.Hex()}
}

// NewMixedcaseAddressFromString 主要用于单元测试
func NewMixedcaseAddressFromString(hexaddr string) (*MixedcaseAddress, error) {
	if !IsHexAddress(hexaddr) {
		return nil, errors.New("invalid address")
	}
	a := FromHex(hexaddr)
	return &MixedcaseAddress{addr: BytesToAddress(a), original: hexaddr}, nil
}

// UnmarshalJSON 解析 MixedcaseAddress
func (ma *MixedcaseAddress) UnmarshalJSON(input []byte) error {
	if err := hexutil.UnmarshalFixedJSON(addressT, input, ma.addr[:]); err != nil {
		return err
	}
	return json.Unmarshal(input, &ma.original)
}

// MarshalJSON 编组原始值
func (ma *MixedcaseAddress) MarshalJSON() ([]byte, error) {
	if strings.HasPrefix(ma.original, "0x") || strings.HasPrefix(ma.original, "0x") {
		return json.Marshal(fmt.Sprintf("0x%s", ma.original[2:]))
	}
	return json.Marshal(fmt.Sprintf("0x%s", ma.original))
}

// 地址返回地址
func (ma *MixedcaseAddress) Address() Address {
	return ma.addr
}

// 字符串实现 fmt.Stringer
func (ma *MixedcaseAddress) String() string {
	if ma.ValidChecksum() {
		return fmt.Sprintf("%s [chksum ok]", ma.original)
	}
	return fmt.Sprintf("%s [chksum INVALID", ma.original)
}

// 如果地址具有有效校验和，则 ValidChecksum 返回 true
func (ma *MixedcaseAddress) ValidChecksum() bool {
	return ma.original == ma.addr.Hex()
}

// 原始返回混合大小写的输入字符串
func (ma *MixedcaseAddress) Original() string {
	return ma.original
}