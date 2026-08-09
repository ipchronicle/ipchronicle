package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	masterKeySize         = 32
	localSchemaVersion    = 2
	secretEnvelopeVersion = 1
)

var (
	ErrNotEnrolled           = errors.New("Agent is not enrolled")
	ErrNoConfiguration       = errors.New("Agent has no applied configuration")
	metaBucket               = []byte("meta")
	identityBucket           = []byte("identity")
	configurationBucket      = []byte("configuration")
	schemaVersionKey         = []byte("schema-version")
	centerURLKey             = []byte("center-url")
	nodeIDKey                = []byte("node-id")
	credentialKey            = []byte("credential")
	appliedRevisionKey       = []byte("applied-configuration-revision")
	configurationKey         = []byte("current")
	configurationErrorKey    = []byte("error")
	configurationErrorRevKey = []byte("error-revision")
	revokedKey               = []byte("revoked")
	credentialAdditionalData = []byte("ipchronicle:agent-credential:v1")
)

type Identity struct {
	CenterURL                    string
	NodeID                       string
	Credential                   string
	AppliedConfigurationRevision int64
}

type Configuration struct {
	SchemaVersion     int      `json:"schemaVersion"`
	Revision          int64    `json:"revision"`
	Enabled           bool     `json:"enabled"`
	HistoryGeneration string   `json:"historyGeneration"`
	Egresses          []Egress `json:"egresses,omitempty"`
}

type Egress struct {
	ID                         string  `json:"id"`
	Kind                       string  `json:"kind"`
	Family                     string  `json:"family"`
	InterfaceName              *string `json:"interfaceName,omitempty"`
	SourceAddress              *string `json:"sourceAddress,omitempty"`
	Enabled                    bool    `json:"enabled"`
	LightweightIntervalSeconds int64   `json:"lightweightIntervalSeconds"`
	ProbeOnAddressChange       bool    `json:"probeOnAddressChange"`
}

type ControlState struct {
	AppliedConfigurationRevision int64
	ConfigurationError           *string
	ConfigurationErrorRevision   *int64
	Revoked                      bool
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
	if err := store.validateCurrentConfiguration(); err != nil {
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

func (s *Store) ControlState() (ControlState, error) {
	var state ControlState
	err := s.database.View(func(transaction *bolt.Tx) error {
		identity := transaction.Bucket(identityBucket)
		if identity == nil || identity.Get(nodeIDKey) == nil {
			return ErrNotEnrolled
		}
		encodedRevision := identity.Get(appliedRevisionKey)
		if len(encodedRevision) != 8 {
			return errors.New("invalid applied configuration revision in Agent state")
		}
		state.AppliedConfigurationRevision = int64(binary.BigEndian.Uint64(encodedRevision))
		configuration := transaction.Bucket(configurationBucket)
		if configuration == nil {
			return errors.New("Agent configuration bucket is missing")
		}
		if message := configuration.Get(configurationErrorKey); message != nil {
			value := string(message)
			state.ConfigurationError = &value
			encodedErrorRevision := configuration.Get(configurationErrorRevKey)
			if len(encodedErrorRevision) != 8 {
				return errors.New("invalid Agent configuration error revision")
			}
			valueRevision := int64(binary.BigEndian.Uint64(encodedErrorRevision))
			state.ConfigurationErrorRevision = &valueRevision
		} else if configuration.Get(configurationErrorRevKey) != nil {
			return errors.New("Agent configuration error revision exists without an error")
		}
		state.Revoked = len(configuration.Get(revokedKey)) == 1 && configuration.Get(revokedKey)[0] == 1
		return nil
	})
	return state, err
}

func (s *Store) Configuration() (Configuration, error) {
	var configuration Configuration
	err := s.database.View(func(transaction *bolt.Tx) error {
		bucket := transaction.Bucket(configurationBucket)
		if bucket == nil || bucket.Get(configurationKey) == nil {
			return ErrNoConfiguration
		}
		if err := json.Unmarshal(bucket.Get(configurationKey), &configuration); err != nil {
			return fmt.Errorf("decode Agent configuration: %w", err)
		}
		return validateConfiguration(configuration)
	})
	return configuration, err
}

func (s *Store) ApplyConfiguration(configuration Configuration) error {
	if err := validateConfiguration(configuration); err != nil {
		return err
	}
	if configuration.SchemaVersion != 2 {
		return errors.New("new Agent configuration must use schema version 2")
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("encode Agent configuration: %w", err)
	}
	return s.database.Update(func(transaction *bolt.Tx) error {
		identity := transaction.Bucket(identityBucket)
		if identity == nil || identity.Get(nodeIDKey) == nil {
			return ErrNotEnrolled
		}
		currentRevision := identity.Get(appliedRevisionKey)
		if len(currentRevision) != 8 {
			return errors.New("invalid applied configuration revision in Agent state")
		}
		if configuration.Revision <= int64(binary.BigEndian.Uint64(currentRevision)) {
			return errors.New("Agent configuration revision must advance")
		}
		bucket := transaction.Bucket(configurationBucket)
		if err := bucket.Put(configurationKey, encoded); err != nil {
			return err
		}
		revision := make([]byte, 8)
		binary.BigEndian.PutUint64(revision, uint64(configuration.Revision))
		if err := identity.Put(appliedRevisionKey, revision); err != nil {
			return err
		}
		if err := bucket.Delete(configurationErrorKey); err != nil {
			return err
		}
		return bucket.Delete(configurationErrorRevKey)
	})
}

func (s *Store) RecordConfigurationFailure(revision int64, cause error) error {
	if revision < 1 || cause == nil {
		return errors.New("configuration failure requires a revision and cause")
	}
	runes := []rune(cause.Error())
	if len(runes) > 1024 {
		runes = runes[:1024]
	}
	message := string(runes)
	return s.database.Update(func(transaction *bolt.Tx) error {
		identity := transaction.Bucket(identityBucket)
		if identity == nil || identity.Get(nodeIDKey) == nil {
			return ErrNotEnrolled
		}
		applied := identity.Get(appliedRevisionKey)
		if len(applied) != 8 || revision <= int64(binary.BigEndian.Uint64(applied)) {
			return errors.New("configuration failure revision must be newer than the applied revision")
		}
		bucket := transaction.Bucket(configurationBucket)
		if err := bucket.Put(configurationErrorKey, []byte(message)); err != nil {
			return err
		}
		encodedRevision := make([]byte, 8)
		binary.BigEndian.PutUint64(encodedRevision, uint64(revision))
		return bucket.Put(configurationErrorRevKey, encodedRevision)
	})
}

func (s *Store) MarkRevoked() error {
	return s.database.Update(func(transaction *bolt.Tx) error {
		if transaction.Bucket(identityBucket).Get(nodeIDKey) == nil {
			return ErrNotEnrolled
		}
		return transaction.Bucket(configurationBucket).Put(revokedKey, []byte{1})
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
		} else if len(version) != 8 || binary.BigEndian.Uint64(version) > localSchemaVersion || binary.BigEndian.Uint64(version) < 1 {
			return fmt.Errorf("unsupported Agent state schema version")
		} else if binary.BigEndian.Uint64(version) < localSchemaVersion {
			encoded := make([]byte, 8)
			binary.BigEndian.PutUint64(encoded, localSchemaVersion)
			if err := meta.Put(schemaVersionKey, encoded); err != nil {
				return err
			}
		}
		if _, err = transaction.CreateBucketIfNotExists(identityBucket); err != nil {
			return err
		}
		_, err = transaction.CreateBucketIfNotExists(configurationBucket)
		return err
	})
}

func (s *Store) validateCurrentConfiguration() error {
	control, err := s.ControlState()
	if errors.Is(err, ErrNotEnrolled) {
		return nil
	}
	if err != nil {
		return err
	}
	configuration, err := s.Configuration()
	if control.AppliedConfigurationRevision == 0 && errors.Is(err, ErrNoConfiguration) {
		return nil
	}
	if err != nil {
		return err
	}
	if configuration.Revision != control.AppliedConfigurationRevision {
		return errors.New("Agent configuration revision does not match applied revision")
	}
	return nil
}

func validateConfiguration(configuration Configuration) error {
	if (configuration.SchemaVersion != 1 && configuration.SchemaVersion != 2) || configuration.Revision < 1 {
		return errors.New("unsupported Agent configuration snapshot")
	}
	generation, err := hex.DecodeString(configuration.HistoryGeneration)
	if err != nil || len(generation) != 32 || configuration.HistoryGeneration != strings.ToLower(configuration.HistoryGeneration) {
		return errors.New("invalid Agent history generation")
	}
	if configuration.SchemaVersion == 1 {
		if len(configuration.Egresses) != 0 {
			return errors.New("schema version 1 configuration contains network egresses")
		}
		return nil
	}
	if len(configuration.Egresses) > 64 {
		return errors.New("Agent configuration contains too many network egresses")
	}
	seen := make(map[string]struct{}, len(configuration.Egresses))
	for _, egress := range configuration.Egresses {
		if _, err := uuid.Parse(egress.ID); err != nil ||
			(egress.Kind != "default" && egress.Kind != "interface" && egress.Kind != "source") ||
			(egress.Family != "ipv4" && egress.Family != "ipv6") || egress.LightweightIntervalSeconds < 1 {
			return errors.New("Agent configuration contains an invalid network egress")
		}
		if _, exists := seen[egress.ID]; exists {
			return errors.New("Agent configuration contains a duplicate network egress")
		}
		seen[egress.ID] = struct{}{}
		switch egress.Kind {
		case "default":
			if egress.InterfaceName != nil || egress.SourceAddress != nil {
				return errors.New("default network egress contains a selector")
			}
		case "interface":
			if !validStoredInterface(egress.InterfaceName) || egress.SourceAddress != nil {
				return errors.New("interface network egress contains an invalid selector")
			}
		case "source":
			if !validStoredInterface(egress.InterfaceName) || egress.SourceAddress == nil {
				return errors.New("source network egress contains an invalid selector")
			}
			address, err := netip.ParseAddr(*egress.SourceAddress)
			if err != nil || address != address.Unmap() || (address.Is4() && egress.Family != "ipv4") || (address.Is6() && egress.Family != "ipv6") {
				return errors.New("source network egress contains an invalid address")
			}
		}
	}
	return nil
}

func validStoredInterface(value *string) bool {
	return value != nil && *value == strings.TrimSpace(*value) && len(*value) >= 1 && len(*value) <= 64 &&
		!strings.ContainsAny(*value, "\x00\r\n\t")
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
