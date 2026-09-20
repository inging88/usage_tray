package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func TestDesktopSafeStorage(t *testing.T) {
	password := "fixture-password"
	payload := []byte(`{"token":"fixture-only"}`)
	padding := aes.BlockSize - len(payload)%aes.BlockSize
	padded := append(append([]byte{}, payload...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	key, _ := pbkdf2.Key(sha1.New, password, []byte("saltysalt"), 1003, 16)
	block, _ := aes.NewCipher(key)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, bytes.Repeat([]byte(" "), aes.BlockSize)).CryptBlocks(ciphertext, padded)
	encoded := base64.StdEncoding.EncodeToString(append([]byte("v10"), ciphertext...))
	got, err := decryptClaudeDesktop(encoded, password)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("desktop decode", err)
	}
	for _, bad := range []string{"", "not-base64", base64.StdEncoding.EncodeToString([]byte("v10short")), base64.StdEncoding.EncodeToString(append([]byte("v20"), ciphertext...))} {
		if _, err := decryptClaudeDesktop(bad, password); err == nil {
			t.Fatal("accepted invalid encryption")
		}
	}
	if _, err := decryptClaudeDesktop(encoded, "wrong-password"); err == nil {
		t.Fatal("accepted wrong key")
	}
}
func TestDesktopTokenScopeAndExpiry(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour).UnixMilli()
	data := fmt.Sprintf(`{
 "acct:a|client:org:https://api.anthropic.com:user:profile":{"token":"profile-token","expiresAt":%d},
 "acct:a|client:org:https://api.anthropic.com:user:inference":{"token":"not-profile","expiresAt":%d},
 "acct:a|client:org:https://other.example:user:profile":{"token":"wrong-host","expiresAt":%d},
 "acct:a|old:org:https://api.anthropic.com:user:profile":{"token":"expired","expiresAt":1}
 }`, future, future+100, future+200)
	token, _ := selectDesktopToken([]byte(data), now)
	if token != "profile-token" {
		t.Fatal("wrong token selected")
	}
	token, _ = selectDesktopToken([]byte(data), now.Add(2*time.Hour))
	if token != "" {
		t.Fatal("used expired token")
	}
}

func TestDesktopHistoryHonestFallback(t *testing.T) {
	now := time.Now()
	fixture := fmt.Sprintf(`{"version":2,"samples":[{"t":%d,"u":{"fh":4,"sd":90}}]}`, now.Add(-3*time.Minute).UnixMilli())
	u, ok := parseClaudeDesktopHistory([]byte(fixture), now)
	if !ok || u.Short.Left != 96 || u.Week.Left != 10 || u.Short.ResetAt != 0 || u.Week.ResetAt != 0 || u.Src != "앱 기록" || u.AgeMin != 3 {
		t.Fatal(u, ok)
	}
	if _, ok := parseClaudeDesktopHistory([]byte(fixture), now.Add(25*time.Hour)); ok {
		t.Fatal("accepted old history")
	}
	if _, ok := parseClaudeDesktopHistory([]byte(`{"version":2,"samples":[{"t":1,"u":{}}]}`), now); ok {
		t.Fatal("missing quota became zero")
	}
}
