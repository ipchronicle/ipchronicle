package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	masterKeySize         = 32
	localSchemaVersion    = 1
	secretEnvelopeVersion = 1
)

var (
	ErrNotEnrolled           = errors.New("Agent is not enrolled")
	metaBucket               = []byte("meta")
	identityBucket           = []byte("identity")
	schemaVersionKey         = []byte("schema-version")
	centerURLKey             = []byte("center-url")
	nodeIDKey                = []byte("node-id")
	credentialKey            = []byte("credential")
	appliedRevisionKey       = []byte("applied-configuration-revision")
	credentialAdditionalData = []byte("ipchronicle:agent-credential:v1")
)

type Identity struct {
	CenterURL                    string
	NodeID                       string
	Credential                   string
	AppliedConfigurationRevision int64
}

type Store struct {
	database  *bolt.DB
	masterKey [masterKeySize]byte
}

func Open(directory string) (*Store, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("Agent state directory must be absolute")
	}
	if err := ensurePrivateDirectory(directory); err != nil {
		return nil, err
	}
	databasePath := filepath.Join(directory, "state.db")
	masterKeyPath := filepath.Join(directory, "master.key")
	databaseExists, err := regularFileExists(databasePath)
	if err != nil {
		return nil, fmt.Errorf("inspect Agent state database: %w", err)
	}
	masterKey, err := loadOrCreateMasterKey(masterKeyPath, databaseExists)
	if err != nil {
		return nil, err
	}
	database, err := bolt.Open(databasePath, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("open Agent state database: %w", err)
	}
	store := &Store{database: database, masterKey: masterKey}
	if err := store.initialize(); err != nil {
		_ = database.Close()
		return nil, err
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("inspect Agent state database permissions: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		_ = database.Close()
		return nil, fmt.Errorf("Agent state database permissions %o allow group or other access", info.Mode().Perm())
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.database.Close()
}

func (s *Store) Identity() (Identity, error) {
	var identity Identity
	err := s.database.View(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(identityBucket)
		if bucket == nil || bucket.Get(nodeIDKey) == nil {
			return ErrNotEnrolled
		}
		identity.CenterURL = string(bucket.Get(centerURLKey))
		identity.NodeID = string(bucket.Get(nodeIDKey))
		envelope := append([]byte(nil), bucket.Get(credentialKey)...)
		credential, err := decryptCredential(s.masterKey, envelope)
		if err != nil {
			return err
		}
		identity.Credential = credential
		encodedRevision := bucket.Get(appliedRevisionKey)
		if len(encodedRevision) != 8 {
			return errors.New("invalid applied configuration revision in Agent state")
		}
		identity.AppliedConfigurationRevision = int64(binary.BigEndian.Uint64(encodedRevision))
		return nil
	})
	return identity, err
}

func (s *Store) SaveIdentity(identity Identity) error {
	if identity.CenterURL == "" || identity.NodeID == "" || identity.Credential == "" || identity.AppliedConfigurationRevision < 0 {
		return errors.New("cannot store incomplete Agent identity")
	}
	encrypted, err := encryptCredential(s.masterKey, identity.Credential)
	if err != nil {
		return err
	}
	return s.database.Update(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(identityBucket)
		if bucket.Get(nodeIDKey) != nil {
			existingCenter := string(bucket.Get(centerURLKey))
			existingNode := string(bucket.Get(nodeIDKey))
			if existingCenter == identity.CenterURL && existingNode == identity.NodeID {
				return nil
			}
			return errors.New("Agent identity already exists")
		}
		revision := make([]byte, 8)
		binary.BigEndian.PutUint64(revision, uint64(identity.AppliedConfigurationRevision))
		for key, value := range map[string][]byte{
			string(centerURLKey):       []byte(identity.CenterURL),
			string(nodeIDKey):          []byte(identity.NodeID),
			string(credentialKey):      encrypted,
			string(appliedRevisionKey): revision,
		} {
			if err := bucket.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) initialize() error {
	return s.database.Update(func(transaction *bolt.Tx) error {
		meta, err := transaction.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		version := meta.Get(schemaVersionKey)
		if version == nil {
			encoded := make([]byte, 8)
			binary.BigEndian.PutUint64(encoded, localSchemaVersion)
			if err := meta.Put(schemaVersionKey, encoded); err != nil {
				return err
			}
		} else if len(version) != 8 || binary.BigEndian.Uint64(version) != localSchemaVersion {
			return fmt.Errorf("unsupported Agent state schema version")
		}
		_, err = transaction.CreateBucketIfNotExists(identityBucket)
		return err
	})
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create Agent state directory: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect Agent state directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("Agent state path is not a directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Agent state directory permissions %o allow group or other access", info.Mode().Perm())
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	return true, nil
}

func loadOrCreateMasterKey(path string, databaseExists bool) ([masterKeySize]byte, error) {
	var key [masterKeySize]byte
	file, err := os.Open(path)
	if err == nil {
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return key, fmt.Errorf("inspect Agent master key: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return key, errors.New("Agent master key must be a root-only regular file")
		}
		if _, err := io.ReadFull(file, key[:]); err != nil {
			return key, fmt.Errorf("read Agent master key: %w", err)
		}
		var extra [1]byte
		if count, err := file.Read(extra[:]); err != io.EOF || count != 0 {
			return key, errors.New("Agent master key must contain exactly 32 bytes")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return key, fmt.Errorf("open Agent master key: %w", err)
	}
	if databaseExists {
		return key, errors.New("Agent master key is missing while state database exists")
	}
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("generate Agent master key: %w", err)
	}
	file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return key, fmt.Errorf("create Agent master key: %w", err)
	}
	if _, err := file.Write(key[:]); err != nil {
		_ = file.Close()
		return key, fmt.Errorf("write Agent master key: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return key, fmt.Errorf("sync Agent master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return key, fmt.Errorf("close Agent master key: %w", err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return key, fmt.Errorf("open Agent state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return key, fmt.Errorf("sync Agent state directory: %w", err)
	}
	return key, nil
}

func encryptCredential(masterKey [masterKeySize]byte, value string) ([]byte, error) {
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
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(value)+gcm.Overhead())
	envelope[0] = secretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, []byte(value), credentialAdditionalData)
	return envelope, nil
}

func decryptCredential(masterKey [masterKeySize]byte, envelope []byte) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != secretEnvelopeVersion {
		return "", errors.New("invalid Agent credential envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], credentialAdditionalData)
	if err != nil {
		return "", errors.New("decrypt Agent credential")
	}
	return string(plaintext), nil
}
