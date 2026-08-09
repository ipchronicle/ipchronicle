package notifications

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const senderSecretEnvelopeVersion byte = 1

var senderConfigurationAdditionalDataPrefix = []byte("ipchronicle:notification-sender:v1:")

func encryptSenderConfiguration(masterKey [32]byte, senderID string, plaintext []byte) ([]byte, error) {
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
		return nil, fmt.Errorf("generate sender-configuration nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(plaintext)+gcm.Overhead())
	envelope[0] = senderSecretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, plaintext, senderConfigurationAdditionalData(senderID))
	return envelope, nil
}

func decryptSenderConfiguration(masterKey [32]byte, senderID string, envelope []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != senderSecretEnvelopeVersion {
		return nil, errors.New("invalid notification sender envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], senderConfigurationAdditionalData(senderID))
	if err != nil {
		return nil, errors.New("decrypt notification sender configuration")
	}
	return plaintext, nil
}

func senderConfigurationAdditionalData(senderID string) []byte {
	result := make([]byte, 0, len(senderConfigurationAdditionalDataPrefix)+len(senderID))
	result = append(result, senderConfigurationAdditionalDataPrefix...)
	return append(result, senderID...)
}
