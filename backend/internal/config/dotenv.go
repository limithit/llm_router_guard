// dotenv.go 裸进程启动时读取 .env（docker compose 场景行为不变：compose 自行解析
// .env 注入真实环境变量，本包读不到也不算错——真实环境变量永远优先于文件值）。
package config

import (
	"bufio"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv 按顺序查找并加载第一个存在的文件：
//
//	$DOTENV_FILE 指定的路径 → ./.env → ../.env
//
// （在 backend/ 下启动 server.exe/go run 时，`../.env` 即仓库根目录那份，与
// compose 共用同一文件。）
//
// 优先级与语义（对齐 12-factor / godotenv 主流约定）：
//   - 已在环境中设置的**非空**变量不被文件值覆盖（部署注入 > 文件）；
//   - 值为空的行（KEY=）视为未设置——与全代码库"空=未设"的读取语义一致；
//   - 支持 # 注释行、行尾 " #" 注释、export 前缀、单/双引号包裹的值；
//   - 不做变量插值、不展开转义符（保持可预测）。
//
// 本地运行防护：值形如 /app/... 的 DB_DSN 是 compose 容器卷内路径，sqlite 本地
// 执行若照抄会在当前盘根悄悄建出一个空新库——此时打印警告并忽略该行。
//
// 返回实际加载的文件路径；未找到任何文件返回空串（不算错误）。
func LoadDotenv() string {
	candidates := []string{".env", filepath.Join("..", ".env")}
	if p := os.Getenv("DOTENV_FILE"); p != "" {
		candidates = append([]string{p}, candidates...)
	}
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		n := applyDotenv(f, p)
		f.Close()
		if n > 0 {
			log.Printf("[config] dotenv: loaded %d var(s) from %s (real env vars take precedence)", n, p)
		}
		return p // 命中第一个存在的文件即止，不做多文件合并
	}
	return ""
}

func applyDotenv(r io.Reader, path string) int {
	type entry struct {
		val   string
		index int
	}
	vars := map[string]entry{}
	order := []string{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 16*1024), 1<<20) // 允许大值（如长 TRUSTED_PROXIES 列表）
	for sc.Scan() {
		k, v, ok := parseDotenvLine(sc.Text())
		if !ok {
			continue
		}
		if v == "" { // 空值 = 未设置
			continue
		}
		if cur := os.Getenv(k); cur != "" { // 真实环境优先（非空才算设置过）
			continue
		}
		if _, seen := vars[k]; !seen {
			order = append(order, k)
		}
		vars[k] = entry{val: v, index: len(order)} // 同名后者覆盖前者
	}

	// 本地运行防护：容器卷内路径的 DB_DSN
	if e, ok := vars["DB_DSN"]; ok && strings.HasPrefix(e.val, "/app/") {
		dbType := "sqlite"
		if t := os.Getenv("DB_TYPE"); t != "" {
			dbType = t
		} else if t, ok := vars["DB_TYPE"]; ok {
			dbType = t.val
		}
		if dbType == "sqlite" {
			log.Printf("[config] dotenv: WARNING %s sets DB_DSN=%q which is a container-internal path; "+
				"ignored for local run (set an absolute native path, or leave DB_DSN unset for ./gateway.db)", path, e.val)
			delete(vars, "DB_DSN")
		}
	}

	n := 0
	for _, k := range order {
		if e, ok := vars[k]; ok {
			_ = os.Setenv(k, e.val)
			n++
		}
	}
	return n
}

// parseDotenvLine 解析一行 "KEY=VALUE"；返回 ok=false 表示应跳过。
func parseDotenvLine(raw string) (string, string, bool) {
	line := strings.TrimSpace(strings.TrimRight(raw, "\r\n"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if !validEnvKey(k) {
		return "", "", false
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		v = v[1 : len(v)-1] // 引号内原样（允许含 # 与空格）
	} else {
		// 未加引号： " #" 起为行尾注释；整值以 # 开头视为注释掉的赋值
		if strings.HasPrefix(v, "#") {
			v = ""
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
	}
	return k, v, true
}

func validEnvKey(k string) bool {
	if k == "" || (k[0] >= '0' && k[0] <= '9') {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}
