package nodes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
	"github.com/ipchronicle/ipchronicle/internal/center/database/historydb"
)

const maxNetworkProxies = 64

var (
	ErrInvalidNetworkProxy         = errors.New("network proxy is invalid")
	ErrNetworkProxyNotFound        = errors.New("network proxy does not exist")
	ErrNetworkProxyAlreadyExists   = errors.New("network proxy name already exists")
	ErrNetworkProxyLimitReached    = errors.New("network proxy limit reached")
	ErrNetworkProxyDeletionPending = errors.New("network proxy deletion is pending")
)

type NetworkProxy struct {
	ID                 uuid.UUID
	NodeID             uuid.UUID
	Name               string
	Scheme             string
	Host               string
	Port               int64
	Username           *string
	PasswordConfigured bool
	Enabled            bool
	Status             string
	IPv4               NetworkProxyFamilyResult
	IPv6               NetworkProxyFamilyResult
	DeletionStatus     *string
	DeletionError      *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type NetworkProxyFamilyResult struct {
	Status        string
	PublicAddress *string
	FailureReason *string
	LastCheckedAt *time.Time
}

type NetworkProxyCreate struct {
	Name     string
	Scheme   string
	Host     string
	Port     int64
	Username *string
	Password *string
}

type NetworkProxyUpdate struct {
	Name           string
	Scheme         string
	Host           string
	Port           int64
	Username       *string
	Enabled        bool
	PasswordAction string
	Password       *string
}

type AgentProxyConfiguration struct {
	ID       uuid.UUID
	Scheme   string
	Host     string
	Port     int64
	Username *string
	Password *string
}

func (s *Service) ListNetworkProxies(ctx context.Context, nodeID uuid.UUID) ([]NetworkProxy, error) {
	if _, err := s.queries.GetNodeByID(ctx, nodeID.String()); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNodeNotFound
	} else if err != nil {
		return nil, err
	}
	records, err := s.queries.ListNodeNetworkProxies(ctx, nodeID.String())
	if err != nil {
		return nil, err
	}
	return s.networkProxiesFromRecords(ctx, nodeID, records)
}

func (s *Service) CreateNetworkProxy(ctx context.Context, nodeID uuid.UUID, input NetworkProxyCreate) (NetworkProxy, error) {
	normalized, err := normalizeNetworkProxy(input.Name, input.Scheme, input.Host, input.Port, input.Username)
	if err != nil || !validProxyPassword(input.Password) {
		return NetworkProxy{}, ErrInvalidNetworkProxy
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkProxy{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return NetworkProxy{}, err
	}
	if _, err := queries.GetNodeNetworkProxyByName(ctx, configdb.GetNodeNetworkProxyByNameParams{
		NodeID: nodeID.String(), Name: normalized.Name,
	}); err == nil {
		return NetworkProxy{}, ErrNetworkProxyAlreadyExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return NetworkProxy{}, err
	}
	count, err := queries.CountNodeNetworkProxies(ctx, nodeID.String())
	if err != nil {
		return NetworkProxy{}, err
	}
	if count >= maxNetworkProxies {
		return NetworkProxy{}, ErrNetworkProxyLimitReached
	}
	egresses, err := queries.ListNodeEgresses(ctx, nodeID.String())
	if err != nil {
		return NetworkProxy{}, err
	}
	if len(egresses)+2 > maxNodeEgresses {
		return NetworkProxy{}, ErrEgressLimitReached
	}
	id := uuid.New()
	var encrypted []byte
	if input.Password != nil {
		encrypted, err = encryptProxyPassword(s.masterKey, id.String(), *input.Password)
		if err != nil {
			return NetworkProxy{}, err
		}
	}
	now := s.now().UTC().Truncate(time.Second).Unix()
	if err := queries.CreateNetworkProxy(ctx, configdb.CreateNetworkProxyParams{
		ID: id.String(), NodeID: nodeID.String(), Name: normalized.Name, Scheme: normalized.Scheme,
		Host: normalized.Host, Port: normalized.Port, Username: normalized.Username,
		PasswordEncrypted: encrypted, Enabled: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return NetworkProxy{}, err
	}
	proxyID := id.String()
	for _, family := range []string{"ipv4", "ipv6"} {
		pathID := uuid.NewString()
		if err := queries.CreateNodeEgress(ctx, configdb.CreateNodeEgressParams{
			ID: pathID, NodeID: nodeID.String(), Name: "proxy:" + family + ":" + id.String(),
			Kind: "proxy", Family: family, ProxyID: &proxyID,
			Enabled: 1, Available: 1, Automatic: 0,
			LightweightIntervalSeconds: int64(defaultLightweightInterval / time.Second),
			CreatedAt:                  now, UpdatedAt: now,
		}); err != nil {
			return NetworkProxy{}, err
		}
	}
	if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
		return NetworkProxy{}, err
	}
	record, err := queries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: nodeID.String(), ID: id.String(),
	})
	if err != nil {
		return NetworkProxy{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NetworkProxy{}, err
	}
	s.sync.Wake(nodeID.String())
	return networkProxyFromRecord(record)
}

func (s *Service) UpdateNetworkProxy(ctx context.Context, nodeID, id uuid.UUID, input NetworkProxyUpdate) (NetworkProxy, error) {
	normalized, err := normalizeNetworkProxy(input.Name, input.Scheme, input.Host, input.Port, input.Username)
	if err != nil || (input.PasswordAction != "keep" && input.PasswordAction != "replace" && input.PasswordAction != "clear") ||
		(input.PasswordAction == "replace") != (input.Password != nil) || !validProxyPassword(input.Password) {
		return NetworkProxy{}, ErrInvalidNetworkProxy
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkProxy{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return NetworkProxy{}, err
	}
	record, err := queries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: nodeID.String(), ID: id.String(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkProxy{}, ErrNetworkProxyNotFound
	}
	if err != nil {
		return NetworkProxy{}, err
	}
	if record.DeletionRequestedAt != nil {
		return NetworkProxy{}, ErrNetworkProxyDeletionPending
	}
	if sameName, lookupErr := queries.GetNodeNetworkProxyByName(ctx, configdb.GetNodeNetworkProxyByNameParams{
		NodeID: nodeID.String(), Name: normalized.Name,
	}); lookupErr == nil && sameName.ID != id.String() {
		return NetworkProxy{}, ErrNetworkProxyAlreadyExists
	} else if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return NetworkProxy{}, lookupErr
	}
	encrypted := record.PasswordEncrypted
	switch input.PasswordAction {
	case "replace":
		encrypted, err = encryptProxyPassword(s.masterKey, id.String(), *input.Password)
		if err != nil {
			return NetworkProxy{}, err
		}
	case "clear":
		encrypted = nil
	}
	passwordChanged := input.PasswordAction == "replace" || (input.PasswordAction == "clear" && len(record.PasswordEncrypted) != 0)
	changed := record.Name != normalized.Name || record.Scheme != normalized.Scheme ||
		record.Host != normalized.Host || record.Port != normalized.Port ||
		!equalOptionalString(record.Username, normalized.Username) || record.Enabled != boolInt(input.Enabled) || passwordChanged
	if changed {
		now := s.now().UTC().Truncate(time.Second).Unix()
		updatedRows, err := queries.UpdateNetworkProxy(ctx, configdb.UpdateNetworkProxyParams{
			Name: normalized.Name, Scheme: normalized.Scheme, Host: normalized.Host,
			Port: normalized.Port, Username: normalized.Username, PasswordEncrypted: encrypted,
			Enabled: boolInt(input.Enabled), UpdatedAt: now, NodeID: nodeID.String(), ID: id.String(),
		})
		if err != nil {
			return NetworkProxy{}, err
		}
		if updatedRows != 1 {
			return NetworkProxy{}, ErrNetworkProxyNotFound
		}
		if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
			return NetworkProxy{}, err
		}
	}
	updated, err := queries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: nodeID.String(), ID: id.String(),
	})
	if err != nil {
		return NetworkProxy{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NetworkProxy{}, err
	}
	if changed {
		s.sync.Wake(nodeID.String())
	}
	if changed {
		return networkProxyFromRecord(updated)
	}
	proxies, err := s.networkProxiesFromRecords(ctx, nodeID, []configdb.NetworkProxy{updated})
	if err != nil {
		return NetworkProxy{}, err
	}
	return proxies[0], nil
}

func (s *Service) DeleteNetworkProxy(ctx context.Context, nodeID, id uuid.UUID) (NetworkProxy, error) {
	now := s.now().UTC().Truncate(time.Second).Unix()
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return NetworkProxy{}, err
	}
	defer transaction.Rollback()
	queries := s.queries.WithTx(transaction)
	if err := requireMutableNode(ctx, queries, nodeID); err != nil {
		return NetworkProxy{}, err
	}
	record, err := queries.GetNodeNetworkProxy(ctx, configdb.GetNodeNetworkProxyParams{
		NodeID: nodeID.String(), ID: id.String(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkProxy{}, ErrNetworkProxyNotFound
	} else if err != nil {
		return NetworkProxy{}, err
	}
	proxyID := id.String()
	paths, err := queries.ListNodeEgressesByProxy(ctx, configdb.ListNodeEgressesByProxyParams{
		NodeID: nodeID.String(), ProxyID: &proxyID,
	})
	if err != nil {
		return NetworkProxy{}, err
	}
	newOperation := record.DeletionRequestedAt == nil
	if _, err := queries.MarkNetworkProxyDeletion(ctx, configdb.MarkNetworkProxyDeletionParams{
		DeletionRequestedAt: &now, NodeID: nodeID.String(), ID: id.String(),
	}); err != nil {
		return NetworkProxy{}, err
	}
	for _, path := range paths {
		if err := queries.CreateEgressDeletion(ctx, configdb.CreateEgressDeletionParams{
			EgressID: path.ID, NodeID: nodeID.String(), RequestedAt: now, UpdatedAt: now,
		}); err != nil {
			return NetworkProxy{}, err
		}
	}
	if newOperation {
		if err := incrementNodeConfiguration(ctx, queries, nodeID.String()); err != nil {
			return NetworkProxy{}, err
		}
	}
	if len(paths) == 0 {
		if _, err := queries.DeleteMarkedNetworkProxyIfUnreferenced(ctx, configdb.DeleteMarkedNetworkProxyIfUnreferencedParams{
			NodeID: nodeID.String(), ID: id.String(),
		}); err != nil {
			return NetworkProxy{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return NetworkProxy{}, err
	}
	if newOperation {
		s.sync.Wake(nodeID.String())
	}
	select {
	case s.deletionWake <- struct{}{}:
	default:
	}
	proxy, err := networkProxyFromRecord(record)
	if err != nil {
		return NetworkProxy{}, err
	}
	status := "pending"
	proxy.DeletionStatus = &status
	return proxy, nil
}

func (s *Service) networkProxiesFromRecords(ctx context.Context, nodeID uuid.UUID, records []configdb.NetworkProxy) ([]NetworkProxy, error) {
	egresses, err := s.queries.ListNodeEgresses(ctx, nodeID.String())
	if err != nil {
		return nil, err
	}
	states, err := s.historyQueries.ListNodeAddressStates(ctx, nodeID.String())
	if err != nil {
		return nil, err
	}
	deletions, err := s.queries.ListActiveNodeEgressDeletions(ctx, nodeID.String())
	if err != nil {
		return nil, err
	}
	stateByEgress := make(map[string]historydb.AddressState, len(states))
	for _, state := range states {
		stateByEgress[state.EgressID] = state
	}
	deletionByEgress := make(map[string]configdb.EgressDeletionOperation, len(deletions))
	for _, deletion := range deletions {
		deletionByEgress[deletion.EgressID] = deletion
	}
	pathsByProxy := make(map[string]map[string]configdb.NetworkEgress)
	for _, egress := range egresses {
		if egress.Kind == "proxy" && egress.ProxyID != nil {
			if pathsByProxy[*egress.ProxyID] == nil {
				pathsByProxy[*egress.ProxyID] = make(map[string]configdb.NetworkEgress)
			}
			pathsByProxy[*egress.ProxyID][egress.Family] = egress
		}
	}
	result := make([]NetworkProxy, 0, len(records))
	for _, record := range records {
		proxy, err := networkProxyFromRecord(record)
		if err != nil {
			return nil, err
		}
		paths := pathsByProxy[record.ID]
		if proxy.Enabled {
			proxy.IPv4 = networkProxyFamilyResult(paths["ipv4"], stateByEgress, record.UpdatedAt)
			proxy.IPv6 = networkProxyFamilyResult(paths["ipv6"], stateByEgress, record.UpdatedAt)
			proxy.Status = networkProxyStatus(proxy.IPv4.Status, proxy.IPv6.Status)
		}
		if record.DeletionRequestedAt != nil {
			status := "pending"
			for _, path := range paths {
				if deletion, ok := deletionByEgress[path.ID]; ok && deletion.Status == "failed" {
					status = "failed"
					proxy.DeletionError = deletion.LastError
					break
				}
			}
			proxy.DeletionStatus = &status
		}
		result = append(result, proxy)
	}
	return result, nil
}

func networkProxyFamilyResult(path configdb.NetworkEgress, states map[string]historydb.AddressState, proxyUpdatedAt int64) NetworkProxyFamilyResult {
	result := NetworkProxyFamilyResult{Status: "checking"}
	if path.ID == "" {
		return result
	}
	state, exists := states[path.ID]
	if !exists || state.ReceivedAt <= proxyUpdatedAt {
		return result
	}
	checkedAt := time.Unix(state.LastCheckedAt, 0).UTC()
	result.LastCheckedAt = &checkedAt
	if state.Status == "confirmed" && state.PublicAddress != nil {
		result.Status = "available"
		result.PublicAddress = state.PublicAddress
		return result
	}
	result.Status = "unavailable"
	result.FailureReason = state.FailureReason
	return result
}

func networkProxyStatus(ipv4, ipv6 string) string {
	if ipv4 == "checking" || ipv6 == "checking" {
		return "checking"
	}
	if ipv4 == "available" && ipv6 == "available" {
		return "dual-stack"
	}
	if ipv4 == "available" {
		return "ipv4-only"
	}
	if ipv6 == "available" {
		return "ipv6-only"
	}
	return "unavailable"
}

func agentProxyFromRecord(masterKey [32]byte, record configdb.NetworkProxy) (AgentProxyConfiguration, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return AgentProxyConfiguration{}, fmt.Errorf("parse stored network proxy ID %q: %w", record.ID, err)
	}
	result := AgentProxyConfiguration{
		ID: id, Scheme: record.Scheme, Host: record.Host, Port: record.Port, Username: record.Username,
	}
	if len(record.PasswordEncrypted) != 0 {
		password, err := decryptProxyPassword(masterKey, record.ID, record.PasswordEncrypted)
		if err != nil {
			return AgentProxyConfiguration{}, fmt.Errorf("read stored proxy credential for %s: %w", record.ID, err)
		}
		result.Password = &password
	}
	return result, nil
}

func networkProxyFromRecord(record configdb.NetworkProxy) (NetworkProxy, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil {
		return NetworkProxy{}, fmt.Errorf("parse stored network proxy ID %q: %w", record.ID, err)
	}
	nodeID, err := uuid.Parse(record.NodeID)
	if err != nil {
		return NetworkProxy{}, fmt.Errorf("parse stored network proxy node ID %q: %w", record.NodeID, err)
	}
	enabled := record.Enabled == 1
	status := "disabled"
	if enabled {
		status = "checking"
	}
	return NetworkProxy{
		ID: id, NodeID: nodeID, Name: record.Name, Scheme: record.Scheme, Host: record.Host, Port: record.Port,
		Username: record.Username, PasswordConfigured: len(record.PasswordEncrypted) != 0, Enabled: enabled,
		Status: status, IPv4: NetworkProxyFamilyResult{Status: status}, IPv6: NetworkProxyFamilyResult{Status: status},
		CreatedAt: time.Unix(record.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(record.UpdatedAt, 0).UTC(),
	}, nil
}

func normalizeNetworkProxy(name, scheme, host string, port int64, username *string) (NetworkProxyCreate, error) {
	name = strings.TrimSpace(name)
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.TrimSpace(host)
	if !validBoundedText(name, 128) || (scheme != "http" && scheme != "https" && scheme != "socks5") ||
		!validProxyHost(host) || port < 1 || port > 65535 {
		return NetworkProxyCreate{}, ErrInvalidNetworkProxy
	}
	if username != nil {
		value := *username
		if value == "" {
			username = nil
		} else if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 || strings.ContainsRune(value, '\x00') {
			return NetworkProxyCreate{}, ErrInvalidNetworkProxy
		} else {
			username = &value
		}
	}
	return NetworkProxyCreate{Name: name, Scheme: scheme, Host: host, Port: port, Username: username}, nil
}

func validProxyHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address == address.Unmap() && !address.IsUnspecified() && !address.IsMulticast()
	}
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return false
	}
	for _, character := range host {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return host != ""
}

func validProxyPassword(password *string) bool {
	return password == nil || (utf8.ValidString(*password) && len(*password) >= 1 && len(*password) <= 4096 && !strings.ContainsRune(*password, '\x00'))
}

func equalOptionalString(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
