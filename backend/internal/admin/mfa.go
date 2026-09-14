// mfa.go TOTP MFA 辅助（REQ-020/021/022）：密钥生成、二维码、验证、恢复码。
// 加固（M-07/M-27）：TOTP 单调时间步（90s 容忍窗内防重放）；
// 恢复码核销用「旧值条件更新」实现原子消费（并发双花防护）；恢复码数量钳制。
package admin

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"

	"llmrouter/internal/crypto"
	"llmrouter/internal/model"
)

const issuer = "LLM-Router-Guard"

func itoa(n int) string { return strconv.Itoa(n) }

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
// 仅用于首次绑定流程（对新密钥无重放史可查）；登录路径请用 Server.verifyTOTP。
func validateTOTP(code, secret string) bool {
	return totp.Validate(code, secret)
}

// verifyTOTP 登录路径校验（M-07）：±1 窗内容忍时钟偏差，但成功核销的时间步必须
// 严格大于该用户上次已用步（last_totp_step），杜绝同一窗口内抓包重放。
// 返回 (本次步, 是否通过)。
func (s *Server) verifyTOTP(u *model.AdminUser, code string) (int64, bool) {
	opts := totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	now := time.Now().UTC()
	for w := int64(-1); w <= 1; w++ {
		t := now.Add(time.Duration(w) * 30 * time.Second)
		if ok, _ := totp.ValidateCustom(code, u.MFASecret, t, opts); ok {
			step := t.Unix() / 30
			if step <= u.LastTOTPStep {
				return 0, false
			}
			return step, true
		}
	}
	return 0, false
}

// persistTOTPStep 记录已核销时间步（写失败不阻断本次登录——最坏退化为无重放防护一次）。
func (s *Server) persistTOTPStep(uid uint, step int64) {
	s.db.Model(&model.AdminUser{}).Where("id = ? AND last_totp_step < ?", uid, step).
		UpdateColumn("last_totp_step", step)
}

// consumeRecoveryCAS 原子消费一个恢复码（M-07：并发下同一码只能成功一次）。
// 做法：内存中删除命中的哈希后，以「原值未变」为条件写回；RowsAffected==1 才算消费成功。
func (s *Server) consumeRecoveryCAS(u *model.AdminUser, code string) bool {
	used, next := consumeRecoveryCode(code, u.RecoveryCodesJSON)
	if !used {
		return false
	}
	res := s.db.Model(&model.AdminUser{}).
		Where("id = ? AND recovery_codes_json = ?", u.ID, u.RecoveryCodesJSON).
		Update("recovery_codes_json", next)
	return res.Error == nil && res.RowsAffected == 1
}

// generateRecoveryCodes 生成 n 个恢复码：明文(仅一次返回给用户) + sha256 哈希列表(入库)。
// M-27：数量钳制 [1,100]（防止被配置成天文数字拖死请求线程）。
func generateRecoveryCodes(n int) ([]string, []string) {
	if n < 1 {
		n = 10
	}
	if n > 100 {
		n = 100
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
// 未命中返回 false（保持原列表）。注意：调用方需通过 Server.consumeRecoveryCAS 落库以保证原子性。
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
