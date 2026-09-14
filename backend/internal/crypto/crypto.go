// Package crypto 提供：AES-GCM 加密（供应商 API Key 加密存储 REQ-005④）、
// 脱敏展示、SHA-256 摘要（API Key 校验）、TOTP 恢复码哈希。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"
)

// Cipher 持有两代数据加密密钥：
//   - v2（"k2:" 前缀密文）：MASTER_KEY 经 HKDF-SHA256 拉伸（M-06：旧版单轮无盐
//     SHA-256 派生，弱口令可离线爆破后解密全库凭据）；
//   - legacy（无前缀密文）：v1 单轮 SHA-256 派生，仅用于读取历史密文——新写入
//     一律 v2。密钥轮换（换 MASTER_KEY）后 legacy 解密同样失败，行为不变。
type Cipher struct {
	aead   cipher.AEAD
	legacy cipher.AEAD
}

const ctPrefixV2 = "k2:"

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// NewCipher 由任意主密钥字符串派生 AES-256-GCM。
func NewCipher(masterKey string) (*Cipher, error) {
	v2, err := hkdf.Key(sha256.New, []byte(masterKey),
		[]byte("llm-router-guard/master-kdf-v2"), "api-provider-key:aes-256-gcm", 32)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(v2)
	if err != nil {
		return nil, err
	}
	lsum := sha256.Sum256([]byte(masterKey))
	legacy, err := newAEAD(lsum[:])
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead, legacy: legacy}, nil
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return ctPrefixV2 + base64.StdEncoding.EncodeToString(ct), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	a := c.legacy
	body := enc
	if strings.HasPrefix(enc, ctPrefixV2) {
		a, body = c.aead, strings.TrimPrefix(enc, ctPrefixV2)
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", err
	}
	ns := a.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := a.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// MaskKey 生成脱敏展示串：前4 + **** + 后2。
func MaskKey(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:4]) + "****" + string(r[len(r)-2:])
}

func Sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RandomHex 生成 n 字节随机 hex 串（API Key / 恢复码用）。
func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}
