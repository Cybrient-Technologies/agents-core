// Package crypto implements the runtime's local API-key encryption
// (AES-256-GCM, keyed by WORKSPACE_KEY).
//
// Wire format (base64): iv[12] || tag[16] || ciphertext, with a 16-byte GCM tag.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

const (
	nonceSize = 12
	tagSize   = 16
)

// Cipher holds the 32-byte AES-256 key derived from the hex WORKSPACE_KEY.
type Cipher struct {
	gcm cipher.AEAD
}

// New builds a Cipher from the hex-encoded 32-byte WORKSPACE_KEY.
func New(workspaceKeyHex string) (*Cipher, error) {
	key, err := hex.DecodeString(workspaceKeyHex)
	if err != nil {
		return nil, errors.New("WORKSPACE_KEY is not valid hex")
	}
	if len(key) != 32 {
		return nil, errors.New("WORKSPACE_KEY must decode to 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, nonceSize)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// Decrypt reverses Encrypt: base64(iv||tag||cipher) -> plaintext.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("ciphertext is not valid base64")
	}
	if len(data) < nonceSize+tagSize {
		return "", errors.New("ciphertext too short")
	}
	iv := data[:nonceSize]
	tag := data[nonceSize : nonceSize+tagSize]
	ct := data[nonceSize+tagSize:]
	// Go expects ciphertext with the tag appended.
	plaintext, err := c.gcm.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Encrypt produces base64(iv||tag||cipher).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	iv := make([]byte, nonceSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := c.gcm.Seal(nil, iv, []byte(plaintext), nil) // cipher || tag
	ct := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]
	out := make([]byte, 0, nonceSize+tagSize+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}
