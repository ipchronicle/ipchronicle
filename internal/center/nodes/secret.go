package nodes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

const secretEnvelopeVersion byte = 1

var enrollmentKeyAdditionalData = []byte("ipchronicle:agent-enrollment-key:v1")

func encryptEnrollmentKey(masterKey [32]byte, value string) ([]byte, error) {
	return encryptSecret(masterKey, value, enrollmentKeyAdditionalData, "enrollment-key")
}

func decryptEnrollmentKey(masterKey [32]byte, envelope []byte) (string, error) {
	return decryptSecret(masterKey, envelope, enrollmentKeyAdditionalData, "enrollment key")
}

func recoveryKeyAdditionalData(nodeID string) []byte {
	return []byte("ipchronicle:node-recovery-key:v1:" + nodeID)
}

func encryptRecoveryKey(masterKey [32]byte, nodeID, value string) ([]byte, error) {
	return encryptSecret(masterKey, value, recoveryKeyAdditionalData(nodeID), "node-recovery-key")
}

func decryptRecoveryKey(masterKey [32]byte, nodeID string, envelope []byte) (string, error) {
	return decryptSecret(masterKey, envelope, recoveryKeyAdditionalData(nodeID), "node recovery key")
}

func encryptSecret(masterKey [32]byte, value string, additionalData []byte, name string) ([]byte, error) {
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
		return nil, fmt.Errorf("generate %s nonce: %w", name, err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(value)+gcm.Overhead())
	envelope[0] = secretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, []byte(value), additionalData)
	return envelope, nil
}

func decryptSecret(masterKey [32]byte, envelope, additionalData []byte, name string) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != secretEnvelopeVersion {
		return "", fmt.Errorf("invalid %s envelope", name)
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], additionalData)
	if err != nil {
		return "", fmt.Errorf("decrypt %s", name)
	}
	return string(plaintext), nil
}
