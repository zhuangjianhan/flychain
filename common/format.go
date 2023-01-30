package common

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// PrettyDuration 是 time.Duration 值的漂亮打印版本
// 格式化文本表示中不必要的精度
type PrettyDuration time.Duration

var prettyDurationRe = regexp.MustCompile(`\.[0-9]{4,}`)

// String 实现了 Stringer 接口，允许漂亮地打印持续时间
// 值四舍五入到三位小数。
func (d PrettyDuration) String() string {
	label := time.Duration(d).String()
	if match := prettyDurationRe.FindString(label); len(match) > 4 {
		label = strings.Replace(label, match, match[:4], 1)
	}
	return label
}

// PrettyAge 是 time.Duration 值的漂亮打印版本，四舍五入
// 值最多为一个最重要的单位，包括天/周/年
type PrettyAge time.Time

// ageUnits 是年龄漂亮打印使用的单位列表。
var ageUnits = []struct {
	Size time.Duration
	Symbol string
}{
	{12 * 30 * 24 * time.Hour, "y"},
	{30 * 24 * time.Hour, "mo"},
	{7 * 24 * time.Hour, "w"},
	{24 * time.Hour, "d"},
	{time.Hour, "h"},
	{time.Minute, "m"},
	{time.Second, "s"},
}

// String 实现了 Stringer 接口，允许漂亮地打印持续时间
// 值四舍五入到最重要的时间单位。
func (t PrettyAge) String() string {
	// 计算时间差并处理 0 cornercase
	diff := time.Since(time.Time(t))
	if diff < time.Second {
		return "0"
	}
	// 返回前累加 3 个分量的精度
	result, prec := "", 0
	for _, unit := range ageUnits {
		if diff > unit.Size{
			result = fmt.Sprintf("%s%d%s", result, diff/unit.Size, unit.Symbol)
			diff %= unit.Size

			if prec += 1; prec >= 3 {
				break
			}
		}
	}
	return result
}


