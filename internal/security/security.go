package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandomToken(byteLen int) (string, error) {
	if byteLen <= 0 {
		byteLen = 32
	}
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func RandomLetters(length int) (string, error) {
	if length <= 0 {
		length = 48
	}
	out := make([]byte, length)
	buf := make([]byte, length*2)
	i := 0
	for i < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= len(letters)*(256/len(letters)) {
				continue
			}
			out[i] = letters[int(b)%len(letters)]
			i++
			if i == length {
				break
			}
		}
	}
	return string(out), nil
}

func SHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func EncryptString(value, secret string) (string, error) {
	value = strings.TrimSpace(value)
	secret = strings.TrimSpace(secret)
	if value == "" {
		return "", nil
	}
	if secret == "" {
		return "", errors.New("encryption secret is empty")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	out := append(nonce, ciphertext...)
	return base64.RawURLEncoding.EncodeToString(out), nil
}

func DecryptString(value, secret string) (string, error) {
	value = strings.TrimSpace(value)
	secret = strings.TrimSpace(secret)
	if value == "" {
		return "", nil
	}
	if secret == "" {
		return "", errors.New("encryption secret is empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func ConstantTimeEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func MaskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + "..." + secret[len(secret)-4:]
}
