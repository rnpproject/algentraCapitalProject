// Package security keeps encrypted broker credentials compatible with the
// Python service's Fernet format during the migration.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"time"
	"unicode/utf8"
)

const managedCopyPasswordPurpose = "algentra:managed-copy-password:v1"

var ErrTokenInvalid = errors.New("kredensial akun tidak dapat dipulihkan")

func EncryptManagedCopyPassword(appSecret, password string) (string, error) {
	key := deriveKey(appSecret, managedCopyPasswordPurpose)
	block, err := aes.NewCipher(key[16:])
	if err != nil {
		return "", ErrTokenInvalid
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	plain := []byte(password)
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	for range padding {
		plain = append(plain, byte(padding))
	}
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plain)
	message := make([]byte, 0, 1+8+len(iv)+len(ciphertext)+sha256.Size)
	message = append(message, 0x80)
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(time.Now().Unix()))
	message = append(message, timestamp[:]...)
	message = append(message, iv...)
	message = append(message, ciphertext...)
	mac := hmac.New(sha256.New, key[:16])
	_, _ = mac.Write(message)
	message = append(message, mac.Sum(nil)...)
	return base64.URLEncoding.EncodeToString(message), nil
}

func DecryptManagedCopyPassword(appSecret, token string) (string, error) {
	key := deriveKey(appSecret, managedCopyPasswordPurpose)
	message, err := base64.URLEncoding.DecodeString(token)
	if err != nil || len(message) < 1+8+aes.BlockSize+aes.BlockSize+sha256.Size || message[0] != 0x80 {
		return "", ErrTokenInvalid
	}
	macStart := len(message) - sha256.Size
	mac := hmac.New(sha256.New, key[:16])
	_, _ = mac.Write(message[:macStart])
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(expected, message[macStart:]) != 1 {
		return "", ErrTokenInvalid
	}
	ivStart, ciphertextStart := 1+8, 1+8+aes.BlockSize
	ciphertext := message[ciphertextStart:macStart]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", ErrTokenInvalid
	}
	block, err := aes.NewCipher(key[16:])
	if err != nil {
		return "", ErrTokenInvalid
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, message[ivStart:ciphertextStart]).CryptBlocks(plain, ciphertext)
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return "", ErrTokenInvalid
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return "", ErrTokenInvalid
		}
	}
	plain = plain[:len(plain)-padding]
	if !utf8.Valid(plain) {
		return "", ErrTokenInvalid
	}
	return string(plain), nil
}

func deriveKey(appSecret, purpose string) []byte {
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write([]byte(purpose))
	return mac.Sum(nil)
}
