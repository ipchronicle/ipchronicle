package nodes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const (
	PollInterval = 30 * time.Second
	OnlineWindow = 2 * time.Minute
)

var (
	ErrEnrollmentKeyMissing = errors.New("Agent enrollment key has not been initialized")
	ErrEnrollmentDisabled   = errors.New("Agent enrollment is disabled")
	ErrEnrollmentKeyInvalid = errors.New("Agent enrollment key is invalid")
	ErrAgentUnauthenticated = errors.New("Agent credential is invalid")
	ErrAgentRevoked         = errors.New("Agent credential is revoked")
	ErrInvalidMetadata      = errors.New("Agent metadata is invalid")
)

type Service struct {
	database  *sql.DB
	queries   *configdb.Queries
	masterKey [32]byte
	now       func() time.Time
}

type Enrollment struct {
	Enabled   bool
	HasKey    bool
	Key       string
	RotatedAt time.Time
}

type Metadata struct {
	Hostname        string
	AgentVersion    string
	OperatingSystem string
	Architecture    string
	Capabilities    []string
}

type Registration struct {
	NodeID     uuid.UUID
	Credential string
}

type Poll struct {
	DesiredConfigurationRevision int64
	Enabled                      bool
}

type Node struct {
	ID                           uuid.UUID
	Name                         string
	Hostname                     string
	Status                       string
	Enabled                      bool
	AgentVersion                 string
	OperatingSystem              string
	Architecture                 string
	Capabilities                 []string
	DesiredConfigurationRevision int64
	AppliedConfigurationRevision int64
	ConfigurationStatus          string
	ConfigurationError           *string
	RegisteredAt                 time.Time
	LastSeenAt                   *time.Time
}

func NewService(database *sql.DB, queries *configdb.Queries, masterKey [32]byte) *Service {
	if database == nil || queries == nil {
		panic("node service database dependencies must not be nil")
	}
	return &Service{
		database: database, queries: queries, masterKey: masterKey,
		now: time.Now,
	}
}

func (s *Service) Enrollment(ctx context.Context) (Enrollment, error) {
	record, err := s.queries.GetAgentEnrollment(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Enrollment{}, nil
	}
	if err != nil {
		return Enrollment{}, err
	}
	key, err := decryptEnrollmentKey(s.masterKey, record.KeyEncrypted)
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{
		Enabled: record.Enabled == 1, HasKey: true, Key: key,
		RotatedAt: time.Unix(record.RotatedAt, 0).UTC(),
	}, nil
}

func (s *Service) RotateEnrollmentKey(ctx context.Context) (Enrollment, error) {
	key, err := randomToken("ipc_reg_")
	if err != nil {
		return Enrollment{}, err
	}
	encrypted, err := encryptEnrollmentKey(s.masterKey, key)
	if err != nil {
		return Enrollment{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	digest := sha256.Sum256([]byte(key))
	if err := s.queries.UpsertAgentEnrollmentKey(ctx, configdb.UpsertAgentEnrollmentKeyParams{
		KeyDigest: digest[:], KeyEncrypted: encrypted,
		CreatedAt: now.Unix(), RotatedAt: now.Unix(),
	}); err != nil {
		return Enrollment{}, err
	}
	return Enrollment{Enabled: true, HasKey: true, Key: key, RotatedAt: now}, nil
}

func (s *Service) SetEnrollmentEnabled(ctx context.Context, enabled bool) (Enrollment, error) {
	changed, err := s.queries.SetAgentEnrollmentEnabled(ctx, boolInteger(enabled))
	if err != nil {
		return Enrollment{}, err
	}
	if changed == 0 {
		return Enrollment{}, ErrEnrollmentKeyMissing
	}
	return s.Enrollment(ctx)
}

func (s *Service) Register(ctx context.Context, registrationKey string, metadata Metadata) (Registration, error) {
	metadata, err := validateMetadata(metadata)
	if err != nil {
		return Registration{}, err
	}
	enrollment, err := s.queries.GetAgentEnrollment(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Registration{}, ErrEnrollmentDisabled
	}
	if err != nil {
		return Registration{}, err
	}
	if enrollment.Enabled != 1 {
		return Registration{}, ErrEnrollmentDisabled
	}
	digest := sha256.Sum256([]byte(registrationKey))
	if subtle.ConstantTimeCompare(digest[:], enrollment.KeyDigest) != 1 {
		return Registration{}, ErrEnrollmentKeyInvalid
	}

	credential, err := randomToken("ipc_agent_")
	if err != nil {
		return Registration{}, err
	}
	credentialDigest := sha256.Sum256([]byte(credential))
	nodeID := uuid.New()
	now := s.now().UTC().Truncate(time.Second)
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Registration{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := queries.CreateNode(ctx, configdb.CreateNodeParams{
		ID: nodeID.String(), Name: initialNodeName(metadata.Hostname),
		Hostname: metadata.Hostname, CredentialDigest: credentialDigest[:],
		AgentVersion: metadata.AgentVersion, OperatingSystem: metadata.OperatingSystem,
		Architecture: metadata.Architecture, RegisteredAt: now.Unix(),
	}); err != nil {
		return Registration{}, err
	}
	if err := replaceCapabilities(ctx, queries, nodeID.String(), metadata.Capabilities); err != nil {
		return Registration{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Registration{}, err
	}
	return Registration{NodeID: nodeID, Credential: credential}, nil
}

func (s *Service) Poll(ctx context.Context, credential string, metadata Metadata, appliedRevision int64, configurationError *string) (Poll, error) {
	metadata, err := validateMetadata(metadata)
	if err != nil || appliedRevision < 0 || !validConfigurationError(configurationError) {
		return Poll{}, ErrInvalidMetadata
	}
	digest := sha256.Sum256([]byte(credential))
	node, err := s.queries.GetNodeByCredentialDigest(ctx, digest[:])
	if errors.Is(err, sql.ErrNoRows) {
		return Poll{}, ErrAgentUnauthenticated
	}
	if err != nil {
		return Poll{}, err
	}
	if node.RevokedAt != nil {
		return Poll{}, ErrAgentRevoked
	}
	if appliedRevision > node.DesiredConfigurationRevision {
		return Poll{}, ErrInvalidMetadata
	}
	now := s.now().UTC().Truncate(time.Second).Unix()
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Poll{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	changed, err := queries.UpdateNodeHeartbeat(ctx, configdb.UpdateNodeHeartbeatParams{
		Hostname: metadata.Hostname, AgentVersion: metadata.AgentVersion,
		OperatingSystem: metadata.OperatingSystem, Architecture: metadata.Architecture,
		AppliedConfigurationRevision: appliedRevision, ConfigurationError: configurationError,
		LastSeenAt: &now, ID: node.ID,
	})
	if err != nil {
		return Poll{}, err
	}
	if changed != 1 {
		return Poll{}, ErrAgentRevoked
	}
	if err := replaceCapabilities(ctx, queries, node.ID, metadata.Capabilities); err != nil {
		return Poll{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Poll{}, err
	}
	return Poll{
		DesiredConfigurationRevision: node.DesiredConfigurationRevision,
		Enabled:                      node.Enabled == 1,
	}, nil
}

func (s *Service) List(ctx context.Context) ([]Node, error) {
	records, err := s.queries.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	capabilityRecords, err := s.queries.ListNodeCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	capabilities := make(map[string][]string, len(records))
	for _, capability := range capabilityRecords {
		capabilities[capability.NodeID] = append(capabilities[capability.NodeID], capability.Capability)
	}
	now := s.now().UTC()
	nodes := make([]Node, 0, len(records))
	for _, record := range records {
		id, err := uuid.Parse(record.ID)
		if err != nil {
			return nil, fmt.Errorf("parse stored node ID %q: %w", record.ID, err)
		}
		status := "offline"
		if record.RevokedAt != nil {
			status = "revoked"
		} else if record.Enabled != 1 {
			status = "disabled"
		} else if record.LastSeenAt != nil && now.Sub(time.Unix(*record.LastSeenAt, 0)) <= OnlineWindow {
			status = "online"
		}
		configurationStatus := "current"
		if record.ConfigurationError != nil {
			configurationStatus = "failed"
		} else if record.AppliedConfigurationRevision != record.DesiredConfigurationRevision {
			configurationStatus = "pending"
		}
		var lastSeenAt *time.Time
		if record.LastSeenAt != nil {
			value := time.Unix(*record.LastSeenAt, 0).UTC()
			lastSeenAt = &value
		}
		nodes = append(nodes, Node{
			ID: id, Name: record.Name, Hostname: record.Hostname, Status: status,
			Enabled: record.Enabled == 1, AgentVersion: record.AgentVersion,
			OperatingSystem: record.OperatingSystem, Architecture: record.Architecture,
			Capabilities:                 append([]string{}, capabilities[record.ID]...),
			DesiredConfigurationRevision: record.DesiredConfigurationRevision,
			AppliedConfigurationRevision: record.AppliedConfigurationRevision,
			ConfigurationStatus:          configurationStatus, ConfigurationError: record.ConfigurationError,
			RegisteredAt: time.Unix(record.RegisteredAt, 0).UTC(), LastSeenAt: lastSeenAt,
		})
	}
	return nodes, nil
}

func validateMetadata(metadata Metadata) (Metadata, error) {
	if metadata.Hostname != strings.TrimSpace(metadata.Hostname) ||
		utf8.RuneCountInString(metadata.Hostname) < 1 || utf8.RuneCountInString(metadata.Hostname) > 253 ||
		strings.ContainsAny(metadata.Hostname, "\x00\r\n\t") {
		return Metadata{}, ErrInvalidMetadata
	}
	if metadata.AgentVersion != strings.TrimSpace(metadata.AgentVersion) ||
		utf8.RuneCountInString(metadata.AgentVersion) < 1 || utf8.RuneCountInString(metadata.AgentVersion) > 64 ||
		strings.ContainsAny(metadata.AgentVersion, "\x00\r\n\t") {
		return Metadata{}, ErrInvalidMetadata
	}
	if metadata.OperatingSystem != "linux" || (metadata.Architecture != "amd64" && metadata.Architecture != "arm64") || len(metadata.Capabilities) > 64 {
		return Metadata{}, ErrInvalidMetadata
	}
	seen := make(map[string]struct{}, len(metadata.Capabilities))
	capabilities := slices.Clone(metadata.Capabilities)
	for _, capability := range capabilities {
		if capability != strings.TrimSpace(capability) || utf8.RuneCountInString(capability) < 1 ||
			utf8.RuneCountInString(capability) > 64 || strings.ContainsAny(capability, "\x00\r\n\t") {
			return Metadata{}, ErrInvalidMetadata
		}
		if _, exists := seen[capability]; exists {
			return Metadata{}, ErrInvalidMetadata
		}
		seen[capability] = struct{}{}
	}
	slices.Sort(capabilities)
	metadata.Capabilities = capabilities
	return metadata, nil
}

func validConfigurationError(value *string) bool {
	if value == nil {
		return true
	}
	count := utf8.RuneCountInString(*value)
	return count >= 1 && count <= 1024 && !strings.ContainsRune(*value, '\x00')
}

func replaceCapabilities(ctx context.Context, queries *configdb.Queries, nodeID string, capabilities []string) error {
	if err := queries.DeleteNodeCapabilities(ctx, nodeID); err != nil {
		return err
	}
	for _, capability := range capabilities {
		if err := queries.CreateNodeCapability(ctx, configdb.CreateNodeCapabilityParams{
			NodeID: nodeID, Capability: capability,
		}); err != nil {
			return err
		}
	}
	return nil
}

func initialNodeName(hostname string) string {
	runes := []rune(hostname)
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return string(runes)
}

func randomToken(prefix string) (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func boolInteger(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
