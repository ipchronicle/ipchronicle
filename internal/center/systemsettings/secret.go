package systemsettings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const ipapiSecretEnvelopeVersion byte = 1

var ipapiAPIKeyAdditionalData = []byte("ipchronicle:ipapi-api-key:v1")

func encryptIPAPIAPIKey(masterKey [32]byte, value string) ([]byte, error) {
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
		return nil, fmt.Errorf("generate ipapi API-key nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(value)+gcm.Overhead())
	envelope[0] = ipapiSecretEnvelopeVersion
	envelope = append(envelope, nonce...)
	return gcm.Seal(envelope, nonce, []byte(value), ipapiAPIKeyAdditionalData), nil
}

func decryptIPAPIAPIKey(masterKey [32]byte, envelope []byte) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != ipapiSecretEnvelopeVersion {
		return "", errors.New("invalid ipapi API-key envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], ipapiAPIKeyAdditionalData)
	if err != nil {
		return "", errors.New("decrypt ipapi API key")
	}
	return string(plaintext), nil
}
