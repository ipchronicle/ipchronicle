package admin

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const secretEnvelopeVersion byte = 1

var totpAdditionalData = []byte("ipchronicle:administrator-totp:v1")

func encryptTOTPSecret(masterKey [32]byte, secret string) ([]byte, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(secret)+gcm.Overhead())
	envelope[0] = secretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, []byte(secret), totpAdditionalData)
	return envelope, nil
}

func decryptTOTPSecret(masterKey [32]byte, envelope []byte) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != secretEnvelopeVersion {
		return "", errors.New("invalid TOTP secret envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	ciphertext := envelope[1+gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, totpAdditionalData)
	if err != nil {
		return "", errors.New("decrypt TOTP secret")
	}
	return string(plaintext), nil
}
