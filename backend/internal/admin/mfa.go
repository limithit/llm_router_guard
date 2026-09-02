// mfa.go TOTP MFA 辅助（REQ-020/021/022）：密钥生成、二维码、验证、恢复码。
package admin

import (
	"encoding/base64"
	"encoding/json"

	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"

	"llmrouter/internal/crypto"
)

const issuer = "LLM-Router-Guard"

// newTOTPSecret 生成 (secret, otpauth_url)；绑定前暂存 secret，enable 时校验。
func newTOTPSecret(account string) (secret, otpauthURL string, err error) {
	k, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: account})
	if err != nil {
		return "", "", err
	}
	return k.Secret(), k.URL(), nil
}

// qrPngBase64 生成二维码 PNG 的 base64 字符串（前端用 data:image/png;base64,.. 展示）。
func qrPngBase64(text string) (string, error) {
	png, err := qrcode.Encode(text, qrcode.Medium, 240)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

// validateTOTP 校验动态验证码（pquerna/otp 默认容忍 ±1 个 30 秒时间窗）。
func validateTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}

// generateRecoveryCodes 生成 n 个恢复码：明文(仅一次返回给用户) + sha256 哈希列表(入库)。
func generateRecoveryCodes(n int) ([]string, []string) {
	if n < 1 {
		n = 10
	}
	plain := make([]string, n)
	hashed := make([]string, n)
	for i := 0; i < n; i++ {
		p := crypto.RandomHex(8) // 16 hex 字符 → xxxx-xxxx-xxxx-xxxx
		p = p[:4] + "-" + p[4:8] + "-" + p[8:12] + "-" + p[12:16]
		plain[i] = p
		hashed[i] = crypto.Sha256Hex(p)
	}
	return plain, hashed
}

// marshalRecoveryHashed 序列化哈希列表为 JSON 文本入库。
func marshalRecoveryHashed(hashed []string) string {
	b, _ := json.Marshal(hashed)
	return string(b)
}

// consumeRecoveryCode 校验并消费一个恢复码：命中返回 true 并从列表中删除该哈希，返回新 JSON。
// 未命中返回 false（保持原列表）。
func consumeRecoveryCode(code, hashedJSON string) (bool, string) {
	var list []string
	if err := json.Unmarshal([]byte(hashedJSON), &list); err != nil || len(list) == 0 {
		return false, hashedJSON
	}
	target := crypto.Sha256Hex(code)
	for i, h := range list {
		if h == target {
			next := append(list[:i:i], list[i+1:]...)
			return true, marshalRecoveryHashed(next)
		}
	}
	return false, hashedJSON
}

// countRecoveryLeft 返回剩余恢复码数量。
func countRecoveryLeft(hashedJSON string) int {
	var list []string
	if err := json.Unmarshal([]byte(hashedJSON), &list); err != nil {
		return 0
	}
	return len(list)
}
