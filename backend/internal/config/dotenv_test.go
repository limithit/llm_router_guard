package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenvLine(t *testing.T) {
	cases := []struct {
		line string
		k, v string
		ok   bool
	}{
		{"JWT_SECRET=abc123", "JWT_SECRET", "abc123", true},
		{"  export PORT=9090 ", "PORT", "9090", true},
		{"# comment", "", "", false},
		{"", "", "", false},
		{"KEY =  spaced  ", "KEY", "spaced", true},
		{`Q="hello # not-a-comment"`, "Q", "hello # not-a-comment", true},
		{"S='sq # kept'", "S", "sq # kept", true},
		{"T=value # trailing comment", "T", "value", true},
		{"P=p@ss#noSpace", "P", "p@ss#noSpace", true}, // 无前置空格的 # 不算注释
		{"EMPTY=", "EMPTY", "", true},
		{"bad key=X", "", "", false},
		{"1BAD=X", "", "", false},
		{"noequals", "", "", false},
		{"CRLF=value\r", "CRLF", "value", true},
	}
	for _, c := range cases {
		k, v, ok := parseDotenvLine(c.line)
		if ok != c.ok || k != c.k || v != c.v {
			t.Errorf("parseDotenvLine(%q) = (%q,%q,%v), want (%q,%q,%v)", c.line, k, v, ok, c.k, c.v, c.ok)
		}
	}
}

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// snapshotEnv 注册若干键的当前值供 t 结束恢复（applyDotenv 用裸 os.Setenv，
// 不注册会泄漏污染同包其他测试）。
func snapshotEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, os.Getenv(k))
	}
}

func TestApplyDotenvPrecedenceAndQuirks(t *testing.T) {
	snapshotEnv(t, "JWT_SECRET", "ADMIN_PASSWORD", "DB_DSN", "DB_TYPE", "TRUSTED_PROXIES", "EMPTY_ONE")
	t.Setenv("JWT_SECRET", "from-real-env") // 非空真实环境：文件不得覆盖
	t.Setenv("ADMIN_PASSWORD", "")          // 空串=未设置：文件值可填充
	p := writeEnv(t, strings.Join([]string{
		"JWT_SECRET=from-file",
		"ADMIN_PASSWORD=file-pwd",
		"DB_DSN=/app/data/db/gateway.db", // sqlite 本地 → 防护忽略
		"# comment line",
		"TRUSTED_PROXIES=10.0.0.0/8, 172.16.0.0/12 # trailing",
		"EMPTY_ONE=",
		"DB_TYPE=sqlite",
		"",
	}, "\n"))
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(os.Stderr) // 警告进测试日志即可
	n := applyDotenv(f, p)

	if got := os.Getenv("JWT_SECRET"); got != "from-real-env" {
		t.Errorf("real env must win, got %q", got)
	}
	if got := os.Getenv("ADMIN_PASSWORD"); got != "file-pwd" {
		t.Errorf("empty real env should be filled from file, got %q", got)
	}
	if got := os.Getenv("DB_DSN"); got != "" {
		t.Errorf("container-path DB_DSN must be skipped locally, got %q", got)
	}
	if got := os.Getenv("TRUSTED_PROXIES"); got != "10.0.0.0/8, 172.16.0.0/12" {
		t.Errorf("trailing comment strip wrong: %q", got)
	}
	if got := os.Getenv("EMPTY_ONE"); got != "" {
		t.Errorf("empty file value must not be set, got %q", got)
	}
	if got := os.Getenv("DB_TYPE"); got != "sqlite" {
		t.Errorf("DB_TYPE should load, got %q", got)
	}
	if n != 3 { // ADMIN_PASSWORD + TRUSTED_PROXIES + DB_TYPE
		t.Errorf("applied count = %d, want 3", n)
	}
}

func TestApplyDotenvNativeDSNNotSkipped(t *testing.T) {
	snapshotEnv(t, "DB_TYPE", "DB_DSN")
	t.Setenv("DB_TYPE", "postgres")
	p := writeEnv(t, "DB_DSN=/app/data/db/gateway.db\n")
	f, _ := os.Open(p)
	defer f.Close()
	applyDotenv(f, p)
	if got := os.Getenv("DB_DSN"); got != "/app/data/db/gateway.db" {
		t.Errorf("non-sqlite must keep DB_DSN as-is, got %q", got)
	}
}

func TestLoadDotenvStopsAtFirstExisting(t *testing.T) {
	snapshotEnv(t, "DOT_A", "DOTENV_FILE")
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOT_A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "..does-not-matter"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadDotenv()
	if got != ".env" {
		t.Fatalf("expected ./env hit, got %q", got)
	}
	if os.Getenv("DOT_A") != "1" {
		t.Error("DOT_A not applied")
	}
}
