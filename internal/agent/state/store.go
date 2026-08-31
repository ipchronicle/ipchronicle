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
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	sharedschedule "github.com/ipchronicle/ipchronicle/internal/schedule"
	bolt "go.etcd.io/bbolt"
)

const (
	masterKeySize         = 32
	localSchemaVersion    = 8
	secretEnvelopeVersion = 1
)

func SchemaVersion() int {
	return localSchemaVersion
}

var (
	ErrNotEnrolled            = errors.New("Agent is not enrolled")
	ErrNoConfiguration        = errors.New("Agent has no applied configuration")
	metaBucket                = []byte("meta")
	identityBucket            = []byte("identity")
	configurationBucket       = []byte("configuration")
	addressCurrentBucket      = []byte("address-current")
	addressEventsBucket       = []byte("address-events")
	addressGapsBucket         = []byte("address-gaps")
	probeRunsBucket           = []byte("probe-runs")
	probeExecutionsBucket     = []byte("probe-executions")
	probeArtifactsBucket      = []byte("probe-artifacts")
	probeSequencesBucket      = []byte("probe-sequences")
	probeGapsBucket           = []byte("probe-gaps")
	probeTasksBucket          = []byte("probe-tasks")
	probeControlBucket        = []byte("probe-control")
	probeProcessBucket        = []byte("probe-process")
	agentUpdatesBucket        = []byte("agent-updates")
	schemaVersionKey          = []byte("schema-version")
	centerURLKey              = []byte("center-url")
	nodeIDKey                 = []byte("node-id")
	credentialKey             = []byte("credential")
	appliedRevisionKey        = []byte("applied-configuration-revision")
	configurationKey          = []byte("current")
	configurationErrorKey     = []byte("error")
	configurationErrorRevKey  = []byte("error-revision")
	revokedKey                = []byte("revoked")
	credentialAdditionalData  = []byte("ipchronicle:agent-credential:v1")
	proxyAdditionalDataPrefix = []byte("ipchronicle:agent-proxy-password:v1:")
)

type Identity struct {
	CenterURL                    string
	NodeID                       string
	Credential                   string
	AppliedConfigurationRevision int64
}

type Configuration struct {
	SchemaVersion          int               `json:"schemaVersion"`
	Revision               int64             `json:"revision"`
	Enabled                bool              `json:"enabled"`
	HistoryGeneration      string            `json:"historyGeneration"`
	DiscoveryPaths         []Egress          `json:"discoveryPaths,omitempty"`
	ProbeTargets           []Egress          `json:"probeTargets,omitempty"`
	Proxies                []Proxy           `json:"proxies,omitempty"`
	DiscoveryServices      DiscoveryServices `json:"discoveryServices"`
	ProbeSchedule          ProbeSchedule     `json:"probeSchedule"`
	ProbeLowMemoryOverride bool              `json:"probeLowMemoryOverride"`
}

type ProbeSchedule struct {
	Enabled  bool   `json:"enabled"`
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
}

type DiscoveryServices struct {
	IPv4 []string `json:"ipv4Services"`
	IPv6 []string `json:"ipv6Services"`
}

type Egress struct {
	ID                         string  `json:"id"`
	PathID                     *string `json:"pathId,omitempty"`
	PublicAddress              *string `json:"publicAddress,omitempty"`
	Kind                       string  `json:"kind"`
	Family                     string  `json:"family"`
	InterfaceName              *string `json:"interfaceName,omitempty"`
	SourceAddress              *string `json:"sourceAddress,omitempty"`
	ProxyID                    *string `json:"proxyId,omitempty"`
	Enabled                    bool    `json:"enabled"`
	LightweightIntervalSeconds int64   `json:"lightweightIntervalSeconds"`
}

type Proxy struct {
	ID       string  `json:"id"`
	Scheme   string  `json:"scheme"`
	Host     string  `json:"host"`
	Port     int64   `json:"port"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
}

type storedConfiguration struct {
	SchemaVersion          int               `json:"schemaVersion"`
	Revision               int64             `json:"revision"`
	Enabled                bool              `json:"enabled"`
	HistoryGeneration      string            `json:"historyGeneration"`
	DiscoveryPaths         []Egress          `json:"discoveryPaths,omitempty"`
	ProbeTargets           []Egress          `json:"probeTargets,omitempty"`
	Proxies                []storedProxy     `json:"proxies,omitempty"`
	DiscoveryServices      DiscoveryServices `json:"discoveryServices"`
	ProbeSchedule          ProbeSchedule     `json:"probeSchedule"`
	ProbeLowMemoryOverride bool              `json:"probeLowMemoryOverride"`
}

type storedProxy struct {
	ID                string  `json:"id"`
	Scheme            string  `json:"scheme"`
	Host              string  `json:"host"`
	Port              int64   `json:"port"`
	Username          *string `json:"username,omitempty"`
	PasswordEncrypted []byte  `json:"passwordEncrypted,omitempty"`
}

type ControlState struct {
	AppliedConfigurationRevision int64
	ConfigurationError           *string
	ConfigurationErrorRevision   *int64
	Revoked                      bool
}

type Store struct {
	database        *bolt.DB
	masterKey       [masterKeySize]byte
	directory       string
	resultDirectory string
	probeMu         sync.Mutex
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
	resultDirectory := filepath.Join(directory, "results")
	if err := ensurePrivateDirectory(resultDirectory); err != nil {
		return nil, fmt.Errorf("prepare Agent result directory: %w", err)
	}
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
	store := &Store{database: database, masterKey: masterKey, directory: directory, resultDirectory: resultDirectory}
	if err := store.initialize(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.validateCurrentConfiguration(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.validateProbeControlState(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.validateAgentUpdateState(); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := store.reconcileProbeArtifacts(); err != nil {
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

func (s *Store) Directory() string {
	return s.directory
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
		decoded, err := decodeStoredConfiguration(s.masterKey, bucket.Get(configurationKey))
		if err != nil {
			return err
		}
		configuration = decoded
		return validateConfiguration(configuration)
	})
	return configuration, err
}

func (s *Store) ApplyConfiguration(configuration Configuration) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if err := validateConfiguration(configuration); err != nil {
		return err
	}
	encoded, err := encodeStoredConfiguration(s.masterKey, configuration)
	if err != nil {
		return fmt.Errorf("encode Agent configuration: %w", err)
	}
	resetAt := time.Now().UTC().Truncate(time.Second)
	err = s.database.Update(func(transaction *bolt.Tx) error {
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
		var previousGeneration string
		if current := bucket.Get(configurationKey); current != nil {
			previous, err := decodeStoredConfiguration(s.masterKey, current)
			if err != nil {
				return err
			}
			previousGeneration = previous.HistoryGeneration
		}
		if err := bucket.Put(configurationKey, encoded); err != nil {
			return err
		}
		revision := make([]byte, 8)
		binary.BigEndian.PutUint64(revision, uint64(configuration.Revision))
		if err := identity.Put(appliedRevisionKey, revision); err != nil {
			return err
		}
		discardedAddressItems, err := reconcileAddressBuckets(transaction, configuration, previousGeneration)
		if err != nil {
			return err
		}
		discardedProbeItems, err := reconcileProbeGeneration(transaction, configuration, previousGeneration)
		if err != nil {
			return err
		}
		if previousGeneration != "" && previousGeneration != configuration.HistoryGeneration {
			status, err := probeStatusFromTransaction(transaction)
			if err != nil {
				return err
			}
			status.HistoryResetGeneration = cloneString(&configuration.HistoryGeneration)
			status.HistoryResetAt = cloneTime(&resetAt)
			status.HistoryResetDiscardedAddressItems = discardedAddressItems
			status.HistoryResetDiscardedProbeItems = discardedProbeItems
			if err := putJSON(transaction.Bucket(probeControlBucket), probeStatusKey, status); err != nil {
				return err
			}
		}
		if err := bucket.Delete(configurationErrorKey); err != nil {
			return err
		}
		return bucket.Delete(configurationErrorRevKey)
	})
	if err != nil {
		return err
	}
	return s.removeUnreferencedProbeResults()
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
		} else if len(version) != 8 || binary.BigEndian.Uint64(version) != localSchemaVersion {
			return errors.New("unsupported Agent state schema version; purge the Agent state and enroll again")
		}
		if _, err = transaction.CreateBucketIfNotExists(identityBucket); err != nil {
			return err
		}
		for _, name := range [][]byte{
			configurationBucket, addressCurrentBucket, addressEventsBucket, addressGapsBucket,
			probeRunsBucket, probeExecutionsBucket, probeArtifactsBucket, probeSequencesBucket,
			probeGapsBucket, probeTasksBucket, probeControlBucket, probeProcessBucket, agentUpdatesBucket,
		} {
			if _, err = transaction.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
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
	if configuration.SchemaVersion != localSchemaVersion || configuration.Revision < 1 {
		return errors.New("unsupported Agent configuration snapshot")
	}
	generation, err := hex.DecodeString(configuration.HistoryGeneration)
	if err != nil || len(generation) != 32 || configuration.HistoryGeneration != strings.ToLower(configuration.HistoryGeneration) {
		return errors.New("invalid Agent history generation")
	}
	if !validDiscoveryServiceList(configuration.DiscoveryServices.IPv4) || !validDiscoveryServiceList(configuration.DiscoveryServices.IPv6) {
		return errors.New("Agent configuration contains invalid discovery services")
	}
	if err := sharedschedule.ValidateProbe(configuration.ProbeSchedule.Cron, configuration.ProbeSchedule.Timezone); err != nil {
		return fmt.Errorf("Agent configuration contains an invalid complete-probe schedule: %w", err)
	}
	if len(configuration.DiscoveryPaths) > 64 || len(configuration.ProbeTargets) > 64 {
		return errors.New("Agent configuration contains too many network egresses")
	}
	if len(configuration.Proxies) > 64 {
		return errors.New("Agent configuration contains too many network proxies")
	}
	proxies := make(map[string]struct{}, len(configuration.Proxies))
	for _, proxy := range configuration.Proxies {
		if _, err := uuid.Parse(proxy.ID); err != nil ||
			(proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5") ||
			!validStoredProxyHost(proxy.Host) || proxy.Port < 1 || proxy.Port > 65535 ||
			!validStoredProxyUsername(proxy.Username) || !validStoredProxyPassword(proxy.Password) {
			return errors.New("Agent configuration contains an invalid network proxy")
		}
		if _, exists := proxies[proxy.ID]; exists {
			return errors.New("Agent configuration contains a duplicate network proxy")
		}
		proxies[proxy.ID] = struct{}{}
	}
	discoveryPaths := configuration.DiscoveryPaths
	probeTargets := configuration.ProbeTargets
	seen := make(map[string]struct{}, len(discoveryPaths)+len(probeTargets))
	referencedProxies := make(map[string]struct{}, len(configuration.Proxies))
	allEgresses := append(slices.Clone(discoveryPaths), probeTargets...)
	for index, egress := range allEgresses {
		probeTarget := index >= len(discoveryPaths)
		if _, err := uuid.Parse(egress.ID); err != nil ||
			(egress.Kind != "default" && egress.Kind != "interface" && egress.Kind != "source" && egress.Kind != "proxy") ||
			(egress.Family != "ipv4" && egress.Family != "ipv6") ||
			(!probeTarget && egress.LightweightIntervalSeconds < 1) ||
			(probeTarget && egress.LightweightIntervalSeconds != 0) {
			return errors.New("Agent configuration contains an invalid network egress")
		}
		identity := egress.ID
		if egress.PathID != nil {
			identity += "\x00" + *egress.PathID
		}
		if _, exists := seen[identity]; exists {
			return errors.New("Agent configuration contains a duplicate network egress")
		}
		seen[identity] = struct{}{}
		if probeTarget {
			if egress.PathID == nil || egress.PublicAddress == nil {
				return errors.New("Agent probe target contains an invalid path identity")
			}
			if _, err := uuid.Parse(*egress.PathID); err != nil {
				return errors.New("Agent probe target contains an invalid path identity")
			}
			address, err := netip.ParseAddr(*egress.PublicAddress)
			if err != nil || address != address.Unmap() || address.String() != *egress.PublicAddress ||
				(address.Is4() && egress.Family != "ipv4") || (address.Is6() && egress.Family != "ipv6") {
				return errors.New("Agent probe target contains an invalid public address")
			}
		} else if egress.PathID != nil || egress.PublicAddress != nil {
			return errors.New("Agent discovery path contains a public-address identity")
		}
		switch egress.Kind {
		case "default":
			if egress.InterfaceName != nil || egress.SourceAddress != nil || egress.ProxyID != nil {
				return errors.New("default network egress contains a selector")
			}
		case "interface":
			if !validStoredInterface(egress.InterfaceName) || egress.SourceAddress != nil || egress.ProxyID != nil {
				return errors.New("interface network egress contains an invalid selector")
			}
		case "source":
			if !validStoredInterface(egress.InterfaceName) || egress.SourceAddress == nil || egress.ProxyID != nil {
				return errors.New("source network egress contains an invalid selector")
			}
			address, err := netip.ParseAddr(*egress.SourceAddress)
			if err != nil || address != address.Unmap() || (address.Is4() && egress.Family != "ipv4") || (address.Is6() && egress.Family != "ipv6") {
				return errors.New("source network egress contains an invalid address")
			}
		case "proxy":
			if egress.InterfaceName != nil || egress.SourceAddress != nil || egress.ProxyID == nil {
				return errors.New("proxy network egress contains an invalid selector")
			}
			if _, exists := proxies[*egress.ProxyID]; !exists {
				return errors.New("proxy network egress references an unavailable proxy")
			}
			referencedProxies[*egress.ProxyID] = struct{}{}
		}
	}
	if len(referencedProxies) != len(proxies) {
		return errors.New("Agent configuration contains an unreferenced network proxy")
	}
	return nil
}

func validDiscoveryServiceList(values []string) bool {
	if len(values) < 2 || len(values) > 8 {
		return false
	}
	hosts := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != strings.TrimSpace(value) || len(value) < 8 || len(value) > 2048 {
			return false
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
			parsed.User != nil || parsed.Fragment != "" || parsed.Hostname() == "" {
			return false
		}
		host := strings.ToLower(parsed.Hostname())
		if _, exists := hosts[host]; exists {
			return false
		}
		hosts[host] = struct{}{}
	}
	return true
}

func validStoredInterface(value *string) bool {
	return value != nil && *value == strings.TrimSpace(*value) && len(*value) >= 1 && len(*value) <= 64 &&
		!strings.ContainsAny(*value, "\x00\r\n\t")
}

func validStoredProxyHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address == address.Unmap() && !address.IsUnspecified() && !address.IsMulticast()
	}
	if len(host) < 1 || len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	for _, character := range host {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}

func validStoredProxyUsername(value *string) bool {
	return value == nil || (utf8.ValidString(*value) && utf8.RuneCountInString(*value) >= 1 &&
		utf8.RuneCountInString(*value) <= 512 && !strings.ContainsRune(*value, '\x00'))
}

func validStoredProxyPassword(value *string) bool {
	return value == nil || (utf8.ValidString(*value) && len(*value) >= 1 && len(*value) <= 4096 &&
		!strings.ContainsRune(*value, '\x00'))
}

func encodeStoredConfiguration(masterKey [masterKeySize]byte, configuration Configuration) ([]byte, error) {
	stored := storedConfiguration{
		SchemaVersion: configuration.SchemaVersion, Revision: configuration.Revision,
		Enabled: configuration.Enabled, HistoryGeneration: configuration.HistoryGeneration,
		DiscoveryPaths: configuration.DiscoveryPaths, ProbeTargets: configuration.ProbeTargets,
		Proxies:           make([]storedProxy, 0, len(configuration.Proxies)),
		DiscoveryServices: configuration.DiscoveryServices, ProbeSchedule: configuration.ProbeSchedule,
		ProbeLowMemoryOverride: configuration.ProbeLowMemoryOverride,
	}
	for _, proxy := range configuration.Proxies {
		item := storedProxy{
			ID: proxy.ID, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port, Username: proxy.Username,
		}
		if proxy.Password != nil {
			encrypted, err := encryptProxyPassword(masterKey, proxy.ID, *proxy.Password)
			if err != nil {
				return nil, err
			}
			item.PasswordEncrypted = encrypted
		}
		stored.Proxies = append(stored.Proxies, item)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode Agent configuration: %w", err)
	}
	return encoded, nil
}

func decodeStoredConfiguration(masterKey [masterKeySize]byte, encoded []byte) (Configuration, error) {
	var stored storedConfiguration
	if err := decodeJSON(encoded, &stored, "Agent configuration"); err != nil {
		return Configuration{}, err
	}
	configuration := Configuration{
		SchemaVersion: stored.SchemaVersion, Revision: stored.Revision, Enabled: stored.Enabled,
		HistoryGeneration: stored.HistoryGeneration, DiscoveryPaths: stored.DiscoveryPaths,
		ProbeTargets:      stored.ProbeTargets,
		DiscoveryServices: stored.DiscoveryServices, ProbeSchedule: stored.ProbeSchedule,
		ProbeLowMemoryOverride: stored.ProbeLowMemoryOverride,
	}
	if len(stored.Proxies) != 0 {
		configuration.Proxies = make([]Proxy, 0, len(stored.Proxies))
	}
	for _, proxy := range stored.Proxies {
		item := Proxy{ID: proxy.ID, Scheme: proxy.Scheme, Host: proxy.Host, Port: proxy.Port, Username: proxy.Username}
		if len(proxy.PasswordEncrypted) != 0 {
			password, err := decryptProxyPassword(masterKey, proxy.ID, proxy.PasswordEncrypted)
			if err != nil {
				return Configuration{}, fmt.Errorf("read retained proxy credential for %s: %w", proxy.ID, err)
			}
			item.Password = &password
		}
		configuration.Proxies = append(configuration.Proxies, item)
	}
	return configuration, nil
}

func (configuration Configuration) DiscoveryPathList() []Egress {
	return configuration.DiscoveryPaths
}

func (configuration Configuration) ProbeTargetList() []Egress {
	return configuration.ProbeTargets
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

func encryptProxyPassword(masterKey [masterKeySize]byte, proxyID, value string) ([]byte, error) {
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
		return nil, fmt.Errorf("generate retained proxy-password nonce: %w", err)
	}
	envelope := make([]byte, 1, 1+len(nonce)+len(value)+gcm.Overhead())
	envelope[0] = secretEnvelopeVersion
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, []byte(value), agentProxyAdditionalData(proxyID))
	return envelope, nil
}

func decryptProxyPassword(masterKey [masterKeySize]byte, proxyID string, envelope []byte) (string, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(envelope) < 1+gcm.NonceSize()+gcm.Overhead() || envelope[0] != secretEnvelopeVersion {
		return "", errors.New("invalid retained proxy-password envelope")
	}
	nonce := envelope[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, envelope[1+gcm.NonceSize():], agentProxyAdditionalData(proxyID))
	if err != nil {
		return "", errors.New("decrypt retained proxy password")
	}
	return string(plaintext), nil
}

func agentProxyAdditionalData(proxyID string) []byte {
	result := make([]byte, 0, len(proxyAdditionalDataPrefix)+len(proxyID))
	result = append(result, proxyAdditionalDataPrefix...)
	return append(result, proxyID...)
}
