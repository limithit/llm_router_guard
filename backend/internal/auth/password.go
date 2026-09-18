// password.go 口令强度策略。
//
// 所有设密入口（首启引导 ADMIN_PASSWORD / 自助改密 / 管理员建户 / 管理员代改密）
// 统一走 ValidatePassword，杜绝弱口令与出厂占位口令。
package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 口令长度边界：下限按字符数（对中文友好），上限按字节（bcrypt 输入界 72 字节，
// 超出会被静默截断，从而导致"超长口令等于前缀"）。
const (
	PasswordMinRunes = 10
	PasswordMaxBytes = 72
)

// minClassKinds 至少需要满足的字符类别数（小写/大写/数字/符号 四选几）。
const minClassKinds = 3

// weakPasswords 常见弱口令与出厂占位值（小写比较）。
var weakPasswords = map[string]bool{
	// 通用弱口令
	"password":      true,
	"password1":     true,
	"password123":   true,
	"passw0rd":      true,
	"p@ssw0rd":      true,
	"12345678":      true,
	"123456789":     true,
	"1234567890":    true,
	"qwerty":        true,
	"qwerty123":     true,
	"qwertyuiop":    true,
	"abc123":        true,
	"abc123456":     true,
	"iloveyou":      true,
	"letmein":       true,
	"welcome":       true,
	"monkey":        true,
	"dragon":        true,
	"sunshine":      true,
	"princess":      true,
	"football":      true,
	"baseball":      true,
	"admin":         true,
	"admin123":      true,
	"admin1234":     true,
	"administrator": true,
	"root":          true,
	"root123":       true,
	"toor":          true,
	"test":          true,
	"test123":       true,
	"test1234":      true,
	"guest":         true,
	"changeme":      true,
	"change-me":     true,
	"secret":        true,
	"default":       true,
	"system":        true,
	"manager":       true,
	"superman":      true,
	"master":        true,
	"shadow":        true,
	"trustno1":      true,
	"whatever":      true,
	// 本项目相关占位值（.env.template / compose 示例中出现过）
	"llm-router-guard-master-key": true,
	"llmrouter":                   true,
	"llm-router-guard":            true,
	"gateway123":                  true,
}

// ValidatePassword 校验口令强度，返回的第一个错误即可直接回显给用户。
//
// 规则：
//  1. 字符数 >= PasswordMinRunes，字节数 <= PasswordMaxBytes；
//  2. 不得为纯空白；
//  3. 至少满足 3 类字符（小写/大写/数字/符号）；
//  4. 不在弱口令黑名单内（忽略大小写）；
//  5. 不得整体等于或包含用户名（用户名长度 >= 3 时检查）；
//  6. 不得为单一字符重复或纯连续序列（如 aaaaaaaa / 1234567890 / abcdefghij）。
//
// username 传空字符串表示不做第 5 条检查。
func ValidatePassword(pw, username string) error {
	if pw == "" {
		return errors.New("密码不能为空")
	}
	if utf8.RuneCountInString(pw) < PasswordMinRunes {
		return fmt.Errorf("密码至少 %d 个字符", PasswordMinRunes)
	}
	if len(pw) > PasswordMaxBytes {
		return fmt.Errorf("密码最长 %d 字节（超出部分会被静默截断）", PasswordMaxBytes)
	}
	if strings.TrimSpace(pw) == "" {
		return errors.New("密码不能为纯空白字符")
	}

	lower := strings.ToLower(pw)
	if weakPasswords[lower] {
		return errors.New("该密码属于常见弱口令，请更换")
	}
	// 弱口令 + 简单后缀（password1234 / admin2024）也算命中
	for w := range weakPasswords {
		if len(w) >= 6 && strings.HasPrefix(lower, w) && len(lower) <= len(w)+4 {
			return errors.New("该密码属于常见弱口令的简单变形，请更换")
		}
	}

	if u := strings.ToLower(strings.TrimSpace(username)); len(u) >= 3 {
		if lower == u {
			return errors.New("密码不能与用户名相同")
		}
		if strings.Contains(lower, u) {
			return errors.New("密码不能包含用户名")
		}
	}

	if isSingleRuneRepeat(pw) {
		return errors.New("密码不能是单一字符的重复")
	}
	if isSequential(pw) {
		return errors.New("密码不能是连续递增/递减序列")
	}

	if kinds := charKinds(pw); kinds < minClassKinds {
		return fmt.Errorf("密码需至少包含大写字母、小写字母、数字、符号中的 %d 类（当前 %d 类）", minClassKinds, kinds)
	}
	return nil
}

// charKinds 统计口令命中的字符类别数（小写/大写/数字/符号）。
func charKinds(pw string) int {
	var lower, upper, digit, symbol bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsLetter(r):
			// 无大小写之分的字母（CJK 等）计入「小写」一类，
			// 否则中文口令会因「既非大写也非小写」白丢一类。
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsSymbol(r), unicode.IsPunct(r):
			symbol = true
		}
	}
	kinds := 0
	for _, ok := range []bool{lower, upper, digit, symbol} {
		if ok {
			kinds++
		}
	}
	return kinds
}

// isSingleRuneRepeat 判断是否全部为同一字符（aaaaaa / 111111）。
func isSingleRuneRepeat(pw string) bool {
	runes := []rune(pw)
	if len(runes) < 2 {
		return false
	}
	for _, r := range runes[1:] {
		if r != runes[0] {
			return false
		}
	}
	return true
}

// isSequential 判断是否为纯连续序列（步长 ±1，如 1234567890 / abcdefghij）。
// 仅含大小写字母与数字时才判定，避免误伤含符号的高强度口令。
func isSequential(pw string) bool {
	runes := []rune(pw)
	if len(runes) < 4 {
		return false
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	// 统一按小写比较（A-Z 与 a-z 视为同一序列）
	prev := unicode.ToLower(runes[0])
	step := rune(0)
	for _, r := range runes[1:] {
		cur := unicode.ToLower(r)
		d := cur - prev
		if d != 1 && d != -1 {
			return false
		}
		if step == 0 {
			step = d
		} else if d != step {
			return false
		}
		prev = cur
	}
	return true
}
