package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

// legacyEncrypt 复刻 v1 方案（单轮 SHA-256 派生 + 无前缀 base64），验证 M-06
// 升级后历史密文仍可解密。
func legacyEncrypt(t *testing.T, masterKey, plain string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(masterKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	io.ReadFull(rand.Reader, nonce)
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(plain), nil))
}

func TestCipherV2RoundTripAndLegacyCompat(t *testing.T) {
	c, err := NewCipher("test-master")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := c.Encrypt("sk-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct, ctPrefixV2) {
		t.Errorf("new ciphertext must carry k2: prefix, got %q", ct)
	}
	if pt, err := c.Decrypt(ct); err != nil || pt != "sk-secret-123" {
		t.Fatalf("v2 round trip failed: %q %v", pt, err)
	}
	// 旧密文（无 k2: 前缀）必须仍可解
	if pt, err := c.Decrypt(legacyEncrypt(t, "test-master", "old-value")); err != nil || pt != "old-value" {
		t.Errorf("legacy decrypt failed: %q %v", pt, err)
	}
	// 错误密钥两代都解不开
	c2, _ := NewCipher("other-master")
	if _, err := c2.Decrypt(ct); err == nil {
		t.Error("wrong key must fail to decrypt v2")
	}
	if _, err := c2.Decrypt(legacyEncrypt(t, "test-master", "x")); err == nil {
		t.Error("wrong key must fail to decrypt legacy")
	}
}
