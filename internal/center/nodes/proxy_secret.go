package nodes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

var proxyPasswordAdditionalDataPrefix = []byte("ipchronicle:network-proxy-password:v1:")

func encryptProxyPassword(masterKey [32]byte, proxyID, value string) ([]byte, error) {
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
		return nil, fmt.Errorf("generate proxy-password nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(value)+gcm.Overhead())
	envelope[0] = secretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, []byte(value), proxyPasswordAdditionalData(proxyID))
	return envelope, nil
}

func decryptProxyPassword(masterKey [32]byte, proxyID string, envelope []byte) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != secretEnvelopeVersion {
		return "", errors.New("invalid proxy-password envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], proxyPasswordAdditionalData(proxyID))
	if err != nil {
		return "", errors.New("decrypt proxy password")
	}
	return string(plaintext), nil
}

func proxyPasswordAdditionalData(proxyID string) []byte {
	result := make([]byte, 0, len(proxyPasswordAdditionalDataPrefix)+len(proxyID))
	result = append(result, proxyPasswordAdditionalDataPrefix...)
	return append(result, proxyID...)
}
