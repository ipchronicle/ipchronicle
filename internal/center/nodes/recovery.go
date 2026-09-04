package nodes

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

var ErrRecoveryKeyInvalid = errors.New("node recovery key is invalid")

type RecoveryCredential struct {
	NodeID    uuid.UUID
	Key       string
	RotatedAt time.Time
}

func (s *Service) EnsureRecoveryKeys(ctx context.Context) error {
	nodeIDs, err := s.queries.ListNodesWithoutRecoveryCredential(ctx)
	if err != nil {
		return err
	}
	for _, nodeID := range nodeIDs {
		_, digest, encrypted, err := s.newRecoveryCredential(nodeID)
		if err != nil {
			return err
		}
		if _, err := s.queries.CreateNodeRecoveryCredential(ctx, configdb.CreateNodeRecoveryCredentialParams{
			NodeID: nodeID, KeyDigest: digest, KeyEncrypted: encrypted,
			RotatedAt: s.now().UTC().Truncate(time.Second).Unix(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RecoveryCredential(ctx context.Context, nodeID uuid.UUID) (RecoveryCredential, error) {
	if err := requireMutableNode(ctx, s.queries, nodeID); err != nil {
		return RecoveryCredential{}, err
	}
	record, err := s.queries.GetNodeRecoveryCredential(ctx, nodeID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return s.createRecoveryCredential(ctx, nodeID)
	}
	if err != nil {
		return RecoveryCredential{}, err
	}
	key, err := decryptRecoveryKey(s.masterKey, record.NodeID, record.KeyEncrypted)
	if err != nil {
		return RecoveryCredential{}, err
	}
	return RecoveryCredential{
		NodeID: nodeID, Key: key,
		RotatedAt: time.Unix(record.RotatedAt, 0).UTC(),
	}, nil
}

func (s *Service) RotateRecoveryKey(ctx context.Context, nodeID uuid.UUID) (RecoveryCredential, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return RecoveryCredential{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return RecoveryCredential{}, err
	}
	key, digest, encrypted, err := s.newRecoveryCredential(nodeID.String())
	if err != nil {
		return RecoveryCredential{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	changed, err := queries.UpdateNodeRecoveryCredential(ctx, configdb.UpdateNodeRecoveryCredentialParams{
		KeyDigest: digest, KeyEncrypted: encrypted, RotatedAt: now.Unix(), NodeID: nodeID.String(),
	})
	if err != nil {
		return RecoveryCredential{}, err
	}
	if changed != 1 {
		return RecoveryCredential{}, ErrRecoveryKeyInvalid
	}
	if err := transaction.Commit(); err != nil {
		return RecoveryCredential{}, err
	}
	return RecoveryCredential{NodeID: nodeID, Key: key, RotatedAt: now}, nil
}

func (s *Service) Recover(ctx context.Context, recoveryKey string, metadata Metadata) (Registration, error) {
	metadata, err := validateMetadata(metadata)
	if err != nil {
		return Registration{}, err
	}
	digest := sha256.Sum256([]byte(recoveryKey))
	credential, err := randomToken("ipc_agent_")
	if err != nil {
		return Registration{}, err
	}
	credentialDigest := sha256.Sum256([]byte(credential))
	now := s.now().UTC().Truncate(time.Second)

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Registration{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	target, err := queries.GetNodeRecoveryTargetByDigest(ctx, digest[:])
	if errors.Is(err, sql.ErrNoRows) {
		return Registration{}, ErrRecoveryKeyInvalid
	}
	if err != nil {
		return Registration{}, err
	}
	if subtle.ConstantTimeCompare(digest[:], target.KeyDigest) != 1 || target.RevokedAt != nil || target.DeletionPending {
		return Registration{}, ErrRecoveryKeyInvalid
	}
	nodeID, err := uuid.Parse(target.NodeID)
	if err != nil {
		return Registration{}, ErrRecoveryKeyInvalid
	}
	if err := queries.UpsertRevokedAgentCredential(ctx, configdb.UpsertRevokedAgentCredentialParams{
		CredentialDigest: target.CredentialDigest, RevokedAt: now.Unix(), Reason: "revoked",
	}); err != nil {
		return Registration{}, err
	}
	changed, err := queries.ReplaceRecoveredNodeIdentity(ctx, configdb.ReplaceRecoveredNodeIdentityParams{
		Hostname: metadata.Hostname, CredentialDigest: credentialDigest[:],
		AgentVersion: metadata.AgentVersion, AgentRevision: metadata.AgentRevision,
		OperatingSystem: metadata.OperatingSystem, Architecture: metadata.Architecture,
		ID: target.NodeID,
	})
	if err != nil {
		return Registration{}, err
	}
	if changed != 1 {
		return Registration{}, ErrRecoveryKeyInvalid
	}
	if err := replaceCapabilities(ctx, queries, target.NodeID, metadata.Capabilities); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteNodeSyncSession(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteNodeNetworkInventory(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteRecoveredNodeCurrentPaths(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteRecoveredNodeHostEgressDeletionOperations(ctx, configdb.DeleteRecoveredNodeHostEgressDeletionOperationsParams{
		NodeID: target.NodeID, NodeID_2: target.NodeID,
	}); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteRecoveredNodeHostEgresses(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	if err := queries.DeletePendingPublicAddressProbes(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	if err := queries.DeleteNodeProbeStatus(ctx, target.NodeID); err != nil {
		return Registration{}, err
	}
	completedAt := now.Unix()
	if _, err := queries.ExpireActiveNodeTasks(ctx, configdb.ExpireActiveNodeTasksParams{
		CompletedAt: &completedAt, NodeID: target.NodeID,
	}); err != nil {
		return Registration{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Registration{}, err
	}
	s.sync.Disconnect(target.NodeID)
	return Registration{NodeID: nodeID, Credential: credential}, nil
}

func (s *Service) createRecoveryCredential(ctx context.Context, nodeID uuid.UUID) (RecoveryCredential, error) {
	key, digest, encrypted, err := s.newRecoveryCredential(nodeID.String())
	if err != nil {
		return RecoveryCredential{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	created, err := s.queries.CreateNodeRecoveryCredential(ctx, configdb.CreateNodeRecoveryCredentialParams{
		NodeID: nodeID.String(), KeyDigest: digest, KeyEncrypted: encrypted, RotatedAt: now.Unix(),
	})
	if err != nil {
		return RecoveryCredential{}, err
	}
	if created == 0 {
		return s.RecoveryCredential(ctx, nodeID)
	}
	return RecoveryCredential{NodeID: nodeID, Key: key, RotatedAt: now}, nil
}

func (s *Service) newRecoveryCredential(nodeID string) (string, []byte, []byte, error) {
	key, err := randomToken("ipc_recover_")
	if err != nil {
		return "", nil, nil, err
	}
	digest := sha256.Sum256([]byte(key))
	encrypted, err := encryptRecoveryKey(s.masterKey, nodeID, key)
	if err != nil {
		return "", nil, nil, err
	}
	return key, digest[:], encrypted, nil
}
