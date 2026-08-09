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
	"log"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
	sharedschedule "github.com/ipchronicle/ipchronicle/internal/schedule"
)

const (
	PollInterval       = 30 * time.Second
	OnlineWindow       = 2 * time.Minute
	SyncSessionLease   = 10 * time.Minute
	SyncWakeCapability = "sync-wakeup-v1"
)

var (
	ErrEnrollmentKeyMissing   = errors.New("Agent enrollment key has not been initialized")
	ErrEnrollmentDisabled     = errors.New("Agent enrollment is disabled")
	ErrEnrollmentKeyInvalid   = errors.New("Agent enrollment key is invalid")
	ErrAgentUnauthenticated   = errors.New("Agent credential is invalid")
	ErrAgentRevoked           = errors.New("Agent credential is revoked")
	ErrInvalidMetadata        = errors.New("Agent metadata is invalid")
	ErrNodeNotFound           = errors.New("node does not exist")
	ErrNodeRevoked            = errors.New("node Agent credential is revoked")
	ErrNodeDeletionPending    = errors.New("node deletion is pending")
	ErrNodeSyncUnsupported    = errors.New("node Agent does not support temporary sync")
	ErrSyncSessionUnavailable = errors.New("Agent sync session is unavailable")
)

type SyncConnections interface {
	Connected(nodeID, sessionID string) bool
	Wake(nodeID string)
	Disconnect(nodeID string)
}

type Service struct {
	database            *sql.DB
	history             *sql.DB
	queries             *configdb.Queries
	historyQueries      *historydb.Queries
	masterKey           [32]byte
	now                 func() time.Time
	deletionWake        chan struct{}
	deleteHistory       func(context.Context, string) error
	deleteEgressHistory func(context.Context, string) error
	sync                SyncConnections
	historyMu           sync.Mutex
	retentionMu         sync.Mutex
}

type Enrollment struct {
	Enabled   bool
	HasKey    bool
	Key       string
	RotatedAt time.Time
}

type Metadata struct {
	Hostname            string
	AgentVersion        string
	OperatingSystem     string
	Architecture        string
	Capabilities        []string
	PhysicalMemoryBytes int64
}

type Registration struct {
	NodeID     uuid.UUID
	Credential string
}

type Poll struct {
	DesiredConfigurationRevision int64
	Enabled                      bool
	SyncSession                  *SyncSession
	AddressUploadReceipt         AddressUploadReceipt
	Task                         *Task
	AcceptedTerminalTaskID       *uuid.UUID
}

type SyncSession struct {
	ID        uuid.UUID
	ExpiresAt time.Time
}

type SyncAuthorization struct {
	NodeID    uuid.UUID
	SessionID uuid.UUID
	ExpiresAt time.Time
}

type Configuration struct {
	SchemaVersion          int
	Revision               int64
	Enabled                bool
	HistoryGeneration      string
	Egresses               []NetworkEgress
	Proxies                []AgentProxyConfiguration
	DiscoveryServices      DiscoveryServices
	ProbeSchedule          ProbeSchedule
	ProbeLowMemoryOverride bool
}

type ProbeSchedule struct {
	Enabled  bool
	Cron     string
	Timezone string
}

type Deletion struct {
	NodeID      uuid.UUID
	Status      string
	RequestedAt time.Time
	Error       *string
}

type EgressDeletion struct {
	EgressID    uuid.UUID
	NodeID      uuid.UUID
	Status      string
	RequestedAt time.Time
	Error       *string
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
	ConfigurationErrorRevision   *int64
	DeletionStatus               *string
	DeletionError                *string
	SyncStatus                   *string
	SyncExpiresAt                *time.Time
	RegisteredAt                 time.Time
	LastSeenAt                   *time.Time
}

func NewService(database, history *sql.DB, queries *configdb.Queries, masterKey [32]byte, syncConnections SyncConnections) *Service {
	if database == nil || history == nil || queries == nil || syncConnections == nil {
		panic("node service database dependencies must not be nil")
	}
	service := &Service{
		database: database, history: history, queries: queries, masterKey: masterKey,
		historyQueries: historydb.New(history), now: time.Now,
		deletionWake: make(chan struct{}, 1), sync: syncConnections,
	}
	service.deleteHistory = service.deleteNodeHistory
	service.deleteEgressHistory = service.deleteNetworkEgressHistory
	return service
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

func (s *Service) Poll(
	ctx context.Context,
	credential string,
	metadata Metadata,
	appliedRevision int64,
	configurationError *string,
	configurationErrorRevision *int64,
	inventory *NetworkInventory,
	inventoryError *string,
	addressUploads ...AddressUpload,
) (Poll, error) {
	if len(addressUploads) > 1 {
		return Poll{}, ErrInvalidMetadata
	}
	addressUpload := AddressUpload{}
	if len(addressUploads) == 1 {
		addressUpload = addressUploads[0]
	}
	metadata, err := validateMetadata(metadata)
	if err != nil || appliedRevision < 0 || !validConfigurationError(configurationError) ||
		(configurationError == nil) != (configurationErrorRevision == nil) ||
		validateNetworkReport(inventory, inventoryError) != nil {
		return Poll{}, ErrInvalidMetadata
	}
	node, err := s.authenticateAgent(ctx, credential)
	if err != nil {
		return Poll{}, err
	}
	if appliedRevision < node.AppliedConfigurationRevision || appliedRevision > node.DesiredConfigurationRevision {
		return Poll{}, ErrInvalidMetadata
	}
	if configurationErrorRevision != nil &&
		(*configurationErrorRevision <= appliedRevision || *configurationErrorRevision > node.DesiredConfigurationRevision) {
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
		ConfigurationErrorRevision: configurationErrorRevision, LastSeenAt: &now, ID: node.ID,
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
	if changed, err := queries.UpdateNodePhysicalMemory(ctx, configdb.UpdateNodePhysicalMemoryParams{
		PhysicalMemoryBytes: &metadata.PhysicalMemoryBytes, ID: node.ID,
	}); err != nil {
		return Poll{}, err
	} else if changed != 1 {
		return Poll{}, ErrAgentRevoked
	}
	acceptedTerminalTaskID, err := s.applyProbeControlReport(
		ctx, queries, node.ID, addressUpload.ProbeStatus, addressUpload.TaskReport, now,
	)
	if err != nil {
		return Poll{}, err
	}
	networkChanged, err := s.applyNetworkReport(ctx, queries, node.ID, inventory, inventoryError, now)
	if err != nil {
		return Poll{}, err
	}
	if networkChanged {
		if err := incrementNodeConfiguration(ctx, queries, node.ID); err != nil {
			return Poll{}, err
		}
	}
	var syncSession *SyncSession
	session, err := queries.GetActiveNodeSyncSession(ctx, configdb.GetActiveNodeSyncSessionParams{
		NodeID: node.ID, ExpiresAt: now,
	})
	if err == nil {
		deliveredAt := now
		changed, err := queries.MarkNodeSyncSessionDelivered(ctx, configdb.MarkNodeSyncSessionDeliveredParams{
			DeliveredAt: &deliveredAt, NodeID: node.ID, SessionID: session.SessionID, ExpiresAt: now,
		})
		if err != nil {
			return Poll{}, err
		}
		if changed != 1 {
			return Poll{}, ErrSyncSessionUnavailable
		}
		sessionID, err := uuid.Parse(session.SessionID)
		if err != nil {
			return Poll{}, fmt.Errorf("parse stored sync session ID %q: %w", session.SessionID, err)
		}
		syncSession = &SyncSession{ID: sessionID, ExpiresAt: time.Unix(session.ExpiresAt, 0).UTC()}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Poll{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Poll{}, err
	}
	receipt, err := s.ingestAddressUpload(ctx, node.ID, addressUpload, now)
	if err != nil {
		return Poll{}, err
	}
	current, err := s.queries.GetNodeByID(ctx, node.ID)
	if err != nil {
		return Poll{}, err
	}
	task, err := s.deliverProbeTask(ctx, node.ID, now)
	if err != nil {
		return Poll{}, err
	}
	return Poll{
		DesiredConfigurationRevision: current.DesiredConfigurationRevision,
		Enabled:                      current.Enabled == 1, SyncSession: syncSession, AddressUploadReceipt: receipt,
		Task: task, AcceptedTerminalTaskID: acceptedTerminalTaskID,
	}, nil
}

func (s *Service) AuthorizeSync(ctx context.Context, credential string, sessionID uuid.UUID) (SyncAuthorization, error) {
	node, err := s.authenticateAgent(ctx, credential)
	if err != nil {
		return SyncAuthorization{}, err
	}
	record, err := s.queries.GetActiveNodeSyncSessionByID(ctx, configdb.GetActiveNodeSyncSessionByIDParams{
		NodeID: node.ID, SessionID: sessionID.String(), ExpiresAt: s.now().UTC().Unix(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthorization{}, ErrSyncSessionUnavailable
	}
	if err != nil {
		return SyncAuthorization{}, err
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return SyncAuthorization{}, fmt.Errorf("parse stored node ID %q: %w", record.NodeID, err)
	}
	return SyncAuthorization{
		NodeID: nodeID, SessionID: sessionID, ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC(),
	}, nil
}

func (s *Service) Configuration(ctx context.Context, credential string) (Configuration, error) {
	node, err := s.authenticateAgent(ctx, credential)
	if err != nil {
		return Configuration{}, err
	}
	state, err := s.queries.GetSystemState(ctx)
	if err != nil {
		return Configuration{}, err
	}
	egressRecords, err := s.queries.ListActiveNodeEgresses(ctx, node.ID)
	if err != nil {
		return Configuration{}, err
	}
	egresses := make([]NetworkEgress, 0, len(egressRecords))
	for _, record := range egressRecords {
		egress, err := egressFromRecord(record)
		if err != nil {
			return Configuration{}, err
		}
		egresses = append(egresses, egress)
	}
	proxyRecords, err := s.queries.ListNodeNetworkProxies(ctx, node.ID)
	if err != nil {
		return Configuration{}, err
	}
	proxies := make([]AgentProxyConfiguration, 0, len(proxyRecords))
	for _, record := range proxyRecords {
		proxy, err := agentProxyFromRecord(s.masterKey, record)
		if err != nil {
			return Configuration{}, err
		}
		proxies = append(proxies, proxy)
	}
	discoveryServices, err := s.ObservationSettings(ctx)
	if err != nil {
		return Configuration{}, err
	}
	settings, err := s.queries.GetNodeProbeSettings(ctx, node.ID)
	if err != nil {
		return Configuration{}, err
	}
	probeSchedule := ProbeSchedule{
		Enabled: settings.ProbeScheduleEnabled == 1,
		Cron:    settings.ProbeScheduleCron, Timezone: settings.ProbeScheduleTimezone,
	}
	if err := sharedschedule.ValidateProbe(probeSchedule.Cron, probeSchedule.Timezone); err != nil {
		return Configuration{}, fmt.Errorf("read stored probe schedule: %w", err)
	}
	return Configuration{
		SchemaVersion: 5, Revision: node.DesiredConfigurationRevision,
		Enabled: node.Enabled == 1, HistoryGeneration: state.HistoryGeneration,
		Egresses: egresses, Proxies: proxies, DiscoveryServices: discoveryServices,
		ProbeSchedule: probeSchedule, ProbeLowMemoryOverride: settings.ProbeLowMemoryOverride == 1,
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
	deletionRecords, err := s.queries.ListActiveNodeDeletions(ctx, 1000)
	if err != nil {
		return nil, err
	}
	deletions := make(map[string]configdb.NodeDeletionOperation, len(deletionRecords))
	for _, deletion := range deletionRecords {
		deletions[deletion.NodeID] = deletion
	}
	now := s.now().UTC()
	syncRecords, err := s.queries.ListNodeSyncSessions(ctx, now.Unix())
	if err != nil {
		return nil, err
	}
	syncSessions := make(map[string]configdb.NodeSyncSession, len(syncRecords))
	for _, session := range syncRecords {
		syncSessions[session.NodeID] = session
	}
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
		if record.ConfigurationError != nil && record.ConfigurationErrorRevision != nil &&
			*record.ConfigurationErrorRevision == record.DesiredConfigurationRevision {
			configurationStatus = "failed"
		} else if record.AppliedConfigurationRevision != record.DesiredConfigurationRevision {
			configurationStatus = "pending"
		}
		var lastSeenAt *time.Time
		if record.LastSeenAt != nil {
			value := time.Unix(*record.LastSeenAt, 0).UTC()
			lastSeenAt = &value
		}
		node := Node{
			ID: id, Name: record.Name, Hostname: record.Hostname, Status: status,
			Enabled: record.Enabled == 1, AgentVersion: record.AgentVersion,
			OperatingSystem: record.OperatingSystem, Architecture: record.Architecture,
			Capabilities:                 append([]string{}, capabilities[record.ID]...),
			DesiredConfigurationRevision: record.DesiredConfigurationRevision,
			AppliedConfigurationRevision: record.AppliedConfigurationRevision,
			ConfigurationStatus:          configurationStatus, ConfigurationError: record.ConfigurationError,
			ConfigurationErrorRevision: record.ConfigurationErrorRevision,
			RegisteredAt:               time.Unix(record.RegisteredAt, 0).UTC(), LastSeenAt: lastSeenAt,
		}
		if deletion, found := deletions[record.ID]; found {
			status := deletion.Status
			node.DeletionStatus = &status
			node.DeletionError = deletion.LastError
		}
		if session, found := syncSessions[record.ID]; found {
			syncStatus := "pending"
			if session.DeliveredAt != nil {
				if s.sync.Connected(record.ID, session.SessionID) {
					syncStatus = "connected"
				} else {
					syncStatus = "degraded"
				}
			}
			expiresAt := time.Unix(session.ExpiresAt, 0).UTC()
			node.SyncStatus = &syncStatus
			node.SyncExpiresAt = &expiresAt
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Node, error) {
	nodes, err := s.List(ctx)
	if err != nil {
		return Node{}, err
	}
	for _, node := range nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return Node{}, ErrNodeNotFound
}

func (s *Service) SetEnabled(ctx context.Context, id uuid.UUID, enabled bool) (Node, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetNodeByID(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeNotFound
	}
	if err != nil {
		return Node{}, err
	}
	if deletion, deletionErr := queries.GetNodeDeletion(ctx, id.String()); deletionErr == nil && deletion.Status != "completed" {
		return Node{}, ErrNodeDeletionPending
	} else if deletionErr != nil && !errors.Is(deletionErr, sql.ErrNoRows) {
		return Node{}, deletionErr
	}
	if record.RevokedAt != nil {
		return Node{}, ErrNodeRevoked
	}
	changedConfiguration := (record.Enabled == 1) != enabled
	if changedConfiguration {
		value := boolInteger(enabled)
		changed, err := queries.SetNodeEnabled(ctx, configdb.SetNodeEnabledParams{
			Enabled: value, ID: id.String(), Enabled_2: value,
		})
		if err != nil {
			return Node{}, err
		}
		if changed != 1 {
			return Node{}, ErrNodeDeletionPending
		}
	}
	if err := transaction.Commit(); err != nil {
		return Node{}, err
	}
	if changedConfiguration {
		s.sync.Wake(id.String())
	}
	return s.Get(ctx, id)
}

func (s *Service) StartSyncSession(ctx context.Context, id uuid.UUID) (Node, error) {
	now := s.now().UTC().Truncate(time.Second)
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetNodeByID(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeNotFound
	}
	if err != nil {
		return Node{}, err
	}
	if record.RevokedAt != nil {
		return Node{}, ErrNodeRevoked
	}
	if deletion, deletionErr := queries.GetNodeDeletion(ctx, id.String()); deletionErr == nil && deletion.Status != "completed" {
		return Node{}, ErrNodeDeletionPending
	} else if deletionErr != nil && !errors.Is(deletionErr, sql.ErrNoRows) {
		return Node{}, deletionErr
	}
	if _, err := queries.GetNodeCapability(ctx, configdb.GetNodeCapabilityParams{
		NodeID: id.String(), Capability: SyncWakeCapability,
	}); errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeSyncUnsupported
	} else if err != nil {
		return Node{}, err
	}
	sessionID := uuid.New()
	if err := queries.UpsertNodeSyncSession(ctx, configdb.UpsertNodeSyncSessionParams{
		NodeID: id.String(), SessionID: sessionID.String(), RequestedAt: now.Unix(),
		ExpiresAt: now.Add(SyncSessionLease).Unix(),
	}); err != nil {
		return Node{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Node{}, err
	}
	s.sync.Disconnect(id.String())
	return s.Get(ctx, id)
}

func (s *Service) StopSyncSession(ctx context.Context, id uuid.UUID) (Node, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if _, err := queries.GetNodeByID(ctx, id.String()); errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeNotFound
	} else if err != nil {
		return Node{}, err
	}
	if err := queries.DeleteNodeSyncSession(ctx, id.String()); err != nil {
		return Node{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Node{}, err
	}
	s.sync.Disconnect(id.String())
	return s.Get(ctx, id)
}

func (s *Service) Revoke(ctx context.Context, id uuid.UUID) (Node, error) {
	now := s.now().UTC().Truncate(time.Second).Unix()
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetNodeByID(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeNotFound
	}
	if err != nil {
		return Node{}, err
	}
	if deletion, deletionErr := queries.GetNodeDeletion(ctx, id.String()); deletionErr == nil && deletion.Status != "completed" {
		return Node{}, ErrNodeDeletionPending
	} else if deletionErr != nil && !errors.Is(deletionErr, sql.ErrNoRows) {
		return Node{}, deletionErr
	}
	if record.RevokedAt == nil {
		changed, err := queries.RevokeNode(ctx, configdb.RevokeNodeParams{RevokedAt: &now, ID: id.String()})
		if err != nil {
			return Node{}, err
		}
		if changed != 1 {
			return Node{}, ErrNodeNotFound
		}
	}
	if err := queries.UpsertRevokedAgentCredential(ctx, configdb.UpsertRevokedAgentCredentialParams{
		CredentialDigest: record.CredentialDigest, RevokedAt: now, Reason: "revoked",
	}); err != nil {
		return Node{}, err
	}
	if err := queries.DeleteNodeSyncSession(ctx, id.String()); err != nil {
		return Node{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Node{}, err
	}
	s.sync.Disconnect(id.String())
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) (Deletion, error) {
	now := s.now().UTC().Truncate(time.Second).Unix()
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Deletion{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	record, err := queries.GetNodeByID(ctx, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return Deletion{}, ErrNodeNotFound
	}
	if err != nil {
		return Deletion{}, err
	}
	if record.RevokedAt == nil {
		if _, err := queries.RevokeNode(ctx, configdb.RevokeNodeParams{RevokedAt: &now, ID: id.String()}); err != nil {
			return Deletion{}, err
		}
	}
	if err := queries.UpsertRevokedAgentCredential(ctx, configdb.UpsertRevokedAgentCredentialParams{
		CredentialDigest: record.CredentialDigest, RevokedAt: now, Reason: "deleted",
	}); err != nil {
		return Deletion{}, err
	}
	if err := queries.DeleteNodeSyncSession(ctx, id.String()); err != nil {
		return Deletion{}, err
	}
	if err := queries.CreateNodeDeletion(ctx, configdb.CreateNodeDeletionParams{
		NodeID: id.String(), CredentialDigest: record.CredentialDigest,
		RequestedAt: now, UpdatedAt: now,
	}); err != nil {
		return Deletion{}, err
	}
	operation, err := queries.GetNodeDeletion(ctx, id.String())
	if err != nil {
		return Deletion{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Deletion{}, err
	}
	s.sync.Disconnect(id.String())
	select {
	case s.deletionWake <- struct{}{}:
	default:
	}
	return deletionFromRecord(operation)
}

func (s *Service) RunDeletionWorker(ctx context.Context, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.processDeletions(ctx, 16); err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("process node deletions: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.deletionWake:
		case <-ticker.C:
		}
	}
}

func (s *Service) processDeletions(ctx context.Context, limit int64) error {
	cutoff := s.now().UTC().Add(-30 * 24 * time.Hour).Unix()
	if err := s.queries.DeleteTerminalProbeTasksBefore(ctx, &cutoff); err != nil {
		return err
	}
	if err := s.processNodeDeletions(ctx, limit); err != nil {
		return err
	}
	return s.processEgressDeletions(ctx, limit)
}

func (s *Service) processNodeDeletions(ctx context.Context, limit int64) error {
	operations, err := s.queries.ListActiveNodeDeletions(ctx, limit)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		now := s.now().UTC().Truncate(time.Second).Unix()
		if operation.Status == "failed" {
			if err := s.queries.RetryNodeDeletion(ctx, configdb.RetryNodeDeletionParams{UpdatedAt: now, NodeID: operation.NodeID}); err != nil {
				return err
			}
		}
		if err := s.deleteHistory(ctx, operation.NodeID); err != nil {
			message := boundedError(err)
			if recordErr := s.queries.FailNodeDeletion(ctx, configdb.FailNodeDeletionParams{
				UpdatedAt: now, LastError: &message, NodeID: operation.NodeID,
			}); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			continue
		}
		transaction, err := s.database.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		queries := s.queries.WithTx(transaction)
		if err := queries.DeleteNodeCapabilities(ctx, operation.NodeID); err == nil {
			err = queries.DeleteNodeEgressDeletionOperations(ctx, operation.NodeID)
		}
		if err == nil {
			err = queries.DeleteNode(ctx, operation.NodeID)
		}
		if err == nil {
			err = queries.CompleteNodeDeletion(ctx, configdb.CompleteNodeDeletionParams{UpdatedAt: now, NodeID: operation.NodeID})
		}
		if err == nil {
			err = transaction.Commit()
		} else {
			_ = transaction.Rollback()
		}
		if err != nil {
			message := boundedError(err)
			if recordErr := s.queries.FailNodeDeletion(ctx, configdb.FailNodeDeletionParams{
				UpdatedAt: now, LastError: &message, NodeID: operation.NodeID,
			}); recordErr != nil {
				return errors.Join(err, recordErr)
			}
		}
	}
	return nil
}

func (s *Service) processEgressDeletions(ctx context.Context, limit int64) error {
	operations, err := s.queries.ListActiveEgressDeletions(ctx, limit)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		now := s.now().UTC().Truncate(time.Second).Unix()
		if operation.Status == "failed" {
			if err := s.queries.RetryEgressDeletion(ctx, configdb.RetryEgressDeletionParams{
				UpdatedAt: now, EgressID: operation.EgressID, NodeID: operation.NodeID,
			}); err != nil {
				return err
			}
		}
		if err := s.deleteEgressHistory(ctx, operation.EgressID); err != nil {
			message := boundedError(err)
			if recordErr := s.queries.FailEgressDeletion(ctx, configdb.FailEgressDeletionParams{
				UpdatedAt: now, LastError: &message, EgressID: operation.EgressID, NodeID: operation.NodeID,
			}); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			continue
		}
		transaction, err := s.database.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		queries := s.queries.WithTx(transaction)
		_, err = queries.DeleteNodeEgress(ctx, configdb.DeleteNodeEgressParams{
			ID: operation.EgressID, NodeID: operation.NodeID,
		})
		if err == nil {
			err = queries.CompleteEgressDeletion(ctx, configdb.CompleteEgressDeletionParams{
				UpdatedAt: now, EgressID: operation.EgressID, NodeID: operation.NodeID,
			})
		}
		if err == nil {
			err = transaction.Commit()
		} else {
			_ = transaction.Rollback()
		}
		if err != nil {
			message := boundedError(err)
			if recordErr := s.queries.FailEgressDeletion(ctx, configdb.FailEgressDeletionParams{
				UpdatedAt: now, LastError: &message, EgressID: operation.EgressID, NodeID: operation.NodeID,
			}); recordErr != nil {
				return errors.Join(err, recordErr)
			}
		}
	}
	return nil
}

func (s *Service) deleteNodeHistory(ctx context.Context, nodeID string) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	if err := queries.DeleteNodeProbeHistory(ctx, nodeID); err != nil {
		return err
	}
	if err := queries.DeleteNodeProbeGaps(ctx, nodeID); err != nil {
		return err
	}
	if err := queries.DeleteNodeProbeComparisonProgress(ctx, nodeID); err != nil {
		return err
	}
	if err := queries.DeleteNodeAddressStates(ctx, nodeID); err != nil {
		return err
	}
	if err := queries.DeleteNodeAddressEvents(ctx, nodeID); err != nil {
		return err
	}
	if err := queries.DeleteNodeAddressGaps(ctx, nodeID); err != nil {
		return err
	}
	return transaction.Commit()
}

func (s *Service) deleteNetworkEgressHistory(ctx context.Context, egressID string) error {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	transaction, err := s.history.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	queries := s.historyQueries.WithTx(transaction)
	if err := queries.DeleteEgressProbeSnapshots(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEgressProbeExecutions(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEgressProbeGaps(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEgressProbeComparisonProgress(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEmptyProbeRuns(ctx); err != nil {
		return err
	}
	if err := queries.DeleteEgressAddressStates(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEgressAddressEvents(ctx, egressID); err != nil {
		return err
	}
	if err := queries.DeleteEgressAddressGaps(ctx, egressID); err != nil {
		return err
	}
	return transaction.Commit()
}

func (s *Service) authenticateAgent(ctx context.Context, credential string) (configdb.Node, error) {
	digest := sha256.Sum256([]byte(credential))
	node, err := s.queries.GetNodeByCredentialDigest(ctx, digest[:])
	if err == nil {
		if node.RevokedAt != nil {
			return configdb.Node{}, ErrAgentRevoked
		}
		return node, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return configdb.Node{}, err
	}
	if _, revokedErr := s.queries.GetRevokedAgentCredential(ctx, digest[:]); revokedErr == nil {
		return configdb.Node{}, ErrAgentRevoked
	} else if !errors.Is(revokedErr, sql.ErrNoRows) {
		return configdb.Node{}, revokedErr
	}
	return configdb.Node{}, ErrAgentUnauthenticated
}

func deletionFromRecord(record configdb.NodeDeletionOperation) (Deletion, error) {
	id, err := uuid.Parse(record.NodeID)
	if err != nil {
		return Deletion{}, fmt.Errorf("parse stored node deletion ID %q: %w", record.NodeID, err)
	}
	return Deletion{
		NodeID: id, Status: record.Status,
		RequestedAt: time.Unix(record.RequestedAt, 0).UTC(), Error: record.LastError,
	}, nil
}

func egressDeletionFromRecord(record configdb.EgressDeletionOperation) (EgressDeletion, error) {
	egressID, err := uuid.Parse(record.EgressID)
	if err != nil {
		return EgressDeletion{}, fmt.Errorf("parse stored egress deletion ID %q: %w", record.EgressID, err)
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return EgressDeletion{}, fmt.Errorf("parse stored egress deletion node ID %q: %w", record.NodeID, err)
	}
	return EgressDeletion{
		EgressID: egressID, NodeID: nodeID, Status: record.Status,
		RequestedAt: time.Unix(record.RequestedAt, 0).UTC(), Error: record.LastError,
	}, nil
}

func boundedError(err error) string {
	runes := []rune(err.Error())
	if len(runes) > 1024 {
		runes = runes[:1024]
	}
	return string(runes)
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
	if metadata.OperatingSystem != "linux" || (metadata.Architecture != "amd64" && metadata.Architecture != "arm64") ||
		len(metadata.Capabilities) > 64 || metadata.PhysicalMemoryBytes < 1 {
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
