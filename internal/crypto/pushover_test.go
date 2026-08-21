package crypto

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestFieldRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, keySize)
	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "empty"},
		{name: "ASCII", plaintext: "deployment complete"},
		{name: "multi-byte UTF-8", plaintext: "привет, 世界"},
		{name: "message rune limit", plaintext: strings.Repeat("界", 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncryptField(tt.plaintext, key)
			if err != nil {
				t.Fatalf("EncryptField() error = %v", err)
			}
			if encoded == tt.plaintext {
				t.Fatal("EncryptField() returned plaintext unchanged")
			}
			got, err := DecryptField(encoded, key)
			if err != nil {
				t.Fatalf("DecryptField() error = %v", err)
			}
			if got != tt.plaintext {
				t.Errorf("DecryptField() = %q, want %q", got, tt.plaintext)
			}
		})
	}
}

func TestDecryptFieldRejectsWrongKeyLength(t *testing.T) {
	if _, err := DecryptField("", []byte("short")); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("DecryptField() error = %v, want 32-byte key error", err)
	}
}

func TestDecryptFieldRejectsTamperedCiphertext(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, keySize)
	encoded, err := EncryptField("authenticated message", key)
	if err != nil {
		t.Fatalf("EncryptField() error = %v", err)
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	payload[aes.BlockSize] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(payload)

	if _, err := DecryptField(tampered, key); err == nil || !strings.Contains(err.Error(), "HMAC mismatch") {
		t.Errorf("DecryptField() error = %v, want HMAC mismatch", err)
	}
}

func TestDecryptFieldRejectsTruncatedPayload(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, keySize)
	truncated := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, aes.BlockSize))
	if _, err := DecryptField(truncated, key); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Errorf("DecryptField() error = %v, want truncated payload error", err)
	}
}

func TestParseKeyRejectsInvalidEncoding(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{name: "wrong length", encoded: "abcd", want: "64 hexadecimal characters"},
		{name: "non-hexadecimal", encoded: strings.Repeat("z", 64), want: "hexadecimal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseKey(tt.encoded); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ParseKey() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}
