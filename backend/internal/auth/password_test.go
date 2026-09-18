package auth

import "testing"

func TestValidatePasswordAcceptsStrong(t *testing.T) {
	ok := []struct {
		name string
		pw   string
		user string
	}{
		{"mixed classes", "Str0ng-Passw0rd!", "admin"},
		{"three classes no symbol", "CorrectHorse42", "admin"},
		{"long random hex-ish", "a7F2k9Qz3Lm", "ops"},
		{"chinese with digit+symbol", "网关管理口令2024!", "admin"},
	}
	for _, c := range ok {
		if err := ValidatePassword(c.pw, c.user); err != nil {
			t.Errorf("%s: expected accept, got %v", c.name, err)
		}
	}
}

func TestValidatePasswordRejectsWeak(t *testing.T) {
	bad := []struct {
		name string
		pw   string
		user string
	}{
		{"too short", "Ab1!xy", "admin"},
		{"empty", "", "admin"},
		{"blank only", "          ", "admin"},
		{"denylist exact", "password", "admin"},
		{"denylist mixed case", "PassWord", "admin"},
		{"denylist with suffix", "password2024", "admin"},
		{"project placeholder", "llm-router-guard-master-key", "admin"},
		{"equals username", "administrator", "administrator"},
		{"contains username", "BobAdminX9!z", "bobadmin"},
		{"single rune repeat", "aaaaaaaaaaaa", "admin"},
		{"digit repeat", "111111111111", "admin"},
		{"ascending digits", "1234567890", "admin"},
		{"ascending letters", "abcdefghij", "admin"},
		{"descending", "9876543210", "admin"},
		{"only two classes", "abcdefghijklmnop", "admin"},
		{"lowercase+digit only", "abcdefgh1234", "admin"},
		{"over bcrypt byte limit", "Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!Ab1!", "admin"},
	}
	for _, c := range bad {
		if err := ValidatePassword(c.pw, c.user); err == nil {
			t.Errorf("%s: expected reject for %q, got nil", c.name, c.pw)
		}
	}
}

func TestValidatePasswordSkipsUsernameCheckWhenShort(t *testing.T) {
	// 用户名短于 3 字符时不做包含检查（否则 "ab" 会误伤大量口令）
	if err := ValidatePassword("ab9X!k2Lm7Q", "ab"); err != nil {
		t.Errorf("expected accept with short username, got %v", err)
	}
}

func TestCharKinds(t *testing.T) {
	cases := map[string]int{
		"abc":     1,
		"aB":      2,
		"aB1":     3,
		"aB1!":    4,
		"!@#$":    1,
		"":        0,
		"网关abc1!": 3,
	}
	for pw, want := range cases {
		if got := charKinds(pw); got != want {
			t.Errorf("charKinds(%q) = %d, want %d", pw, got, want)
		}
	}
}

func TestIsSequential(t *testing.T) {
	yes := []string{"1234", "abcdef", "9876543210", "ABCDEFG"}
	no := []string{"1357", "abcabc", "a1b2", "1234!", "ab12cd"}
	for _, s := range yes {
		if !isSequential(s) {
			t.Errorf("isSequential(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isSequential(s) {
			t.Errorf("isSequential(%q) = true, want false", s)
		}
	}
}
