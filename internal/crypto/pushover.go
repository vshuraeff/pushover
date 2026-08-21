// Package crypto implements pushover's documented end-to-end encryption scheme
// for the encrypted message parameter.
package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

const keySize = 32

// EncryptField encrypts one plaintext message field with a 32-byte key.
func EncryptField(plaintext string, key []byte) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("create gzip writer: %w", err)
	}
	if _, err := writer.Write([]byte(plaintext)); err != nil {
		return "", fmt.Errorf("compress plaintext: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finish compressing plaintext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("generate encryption IV: %w", err)
	}
	padded := pkcs7Pad(compressed.Bytes(), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	payload := make([]byte, 0, len(iv)+len(ciphertext)+sha256.Size)
	payload = append(payload, iv...)
	payload = append(payload, ciphertext...)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	payload = append(payload, mac.Sum(nil)...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

// DecryptField decrypts one base64-encoded message field with a 32-byte key.
func DecryptField(encoded string, key []byte) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode encrypted field: %w", err)
	}
	minimumSize := aes.BlockSize + aes.BlockSize + sha256.Size
	if len(payload) < minimumSize {
		return "", fmt.Errorf("encrypted field payload is truncated")
	}

	macStart := len(payload) - sha256.Size
	authenticated := payload[:macStart]
	wantMAC := payload[macStart:]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(authenticated)
	if !hmac.Equal(mac.Sum(nil), wantMAC) {
		return "", fmt.Errorf("encrypted field HMAC mismatch")
	}

	iv := authenticated[:aes.BlockSize]
	ciphertext := authenticated[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("encrypted field ciphertext length is not a multiple of the AES block size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create AES cipher: %w", err)
	}
	padded := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, ciphertext)
	compressed, err := pkcs7Unpad(padded, aes.BlockSize)
	if err != nil {
		return "", err
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return "", fmt.Errorf("open compressed plaintext: %w", err)
	}
	plaintext, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return "", fmt.Errorf("decompress plaintext: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close compressed plaintext: %w", closeErr)
	}
	return string(plaintext), nil
}

// ParseKey parses a 64-character hexadecimal encryption key.
func ParseKey(encoded string) ([]byte, error) {
	if len(encoded) != hex.EncodedLen(keySize) {
		return nil, fmt.Errorf("encryption key must be 64 hexadecimal characters")
	}
	key, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key as hexadecimal: %w", err)
	}
	return key, nil
}

func validateKey(key []byte) error {
	if len(key) != keySize {
		return fmt.Errorf("encryption key must be 32 bytes (AES-256)")
	}
	return nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	paddingSize := blockSize - len(data)%blockSize
	padded := make([]byte, len(data)+paddingSize)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(paddingSize)
	}
	return padded
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid PKCS7-padded plaintext length")
	}
	paddingSize := int(data[len(data)-1])
	if paddingSize == 0 || paddingSize > blockSize || paddingSize > len(data) {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	for _, value := range data[len(data)-paddingSize:] {
		if int(value) != paddingSize {
			return nil, fmt.Errorf("invalid PKCS7 padding")
		}
	}
	return data[:len(data)-paddingSize], nil
}
