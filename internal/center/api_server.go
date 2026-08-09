package center

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type apiServer struct {
	version                  string
	administrator            *admin.Service
	nodes                    *nodes.Service
	configSchemaVersion      int64
	historySchemaVersion     int64
	externalOriginConfigured bool
	trustedProxyConfigured   bool
}

func (s apiServer) Login(ctx context.Context, request api.LoginRequestObject) (api.LoginResponseObject, error) {
	security := requestSecurityFromContext(ctx)
	if !originMatches(security.Origin, security.ExpectedOrigin) {
		return api.Login403JSONResponse{ForbiddenJSONResponse: forbidden(api.OriginNotAllowed)}, nil
	}
	if request.Body == nil {
		return api.Login400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	totpCode := ""
	if request.Body.TotpCode != nil {
		totpCode = *request.Body.TotpCode
	}
	session, retryAfter, err := s.administrator.Login(
		ctx,
		request.Body.Username,
		request.Body.Password,
		totpCode,
		security.ClientAddress,
		security.UserAgent,
	)
	switch {
	case errors.Is(err, admin.ErrInvalidCredentials):
		return api.Login401JSONResponse{UnauthorizedJSONResponse: unauthorized(api.InvalidCredentials)}, nil
	case errors.Is(err, admin.ErrTOTPRequired):
		return api.Login401JSONResponse{UnauthorizedJSONResponse: unauthorized(api.TotpRequired)}, nil
	case errors.Is(err, admin.ErrRateLimited):
		parameters := map[string]string{"retryAfterSeconds": fmt.Sprint(max(1, int(math.Ceil(retryAfter.Seconds()))))}
		return api.Login429JSONResponse(errorResponse(api.RateLimited, parameters)), nil
	case err != nil:
		return nil, err
	}
	cookie := sessionCookie(session.Token, session.ExpiresAt, security.CookieSecure)
	return api.Login200JSONResponse{
		Body: api.AuthenticatedSession{
			Account:   accountResponse(session.Account),
			CsrfToken: session.CSRFToken,
			ExpiresAt: session.ExpiresAt,
		},
		Headers: api.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (s apiServer) GetAuthenticatedSession(ctx context.Context, _ api.GetAuthenticatedSessionRequestObject) (api.GetAuthenticatedSessionResponseObject, error) {
	principal, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetAuthenticatedSession401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	return api.GetAuthenticatedSession200JSONResponse{
		Account:   accountResponse(principal.Account),
		CsrfToken: s.administrator.CSRFToken(principal.Token),
		ExpiresAt: principal.ExpiresAt,
	}, nil
}

func (s apiServer) Logout(ctx context.Context, request api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	principal, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.Logout401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.Logout403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if err := s.administrator.RevokeSession(ctx, principal.TokenDigest); err != nil {
		return nil, err
	}
	cookie := expiredSessionCookie(requestSecurityFromContext(ctx).CookieSecure)
	return api.Logout204Response{Headers: api.Logout204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s apiServer) GetAccount(ctx context.Context, _ api.GetAccountRequestObject) (api.GetAccountResponseObject, error) {
	principal, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetAccount401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	return api.GetAccount200JSONResponse(accountResponse(principal.Account)), nil
}

func (s apiServer) UpdateAccount(ctx context.Context, request api.UpdateAccountRequestObject) (api.UpdateAccountResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateAccount401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateAccount403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateAccount400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	account, revoked, err := s.administrator.UpdateAccount(ctx, admin.AccountUpdate{
		CurrentPassword: request.Body.CurrentPassword,
		Username:        request.Body.Username,
		NewPassword:     request.Body.NewPassword,
	})
	if err != nil {
		code := api.InvalidRequest
		if errors.Is(err, admin.ErrCurrentPassword) {
			code = api.CurrentPasswordInvalid
		} else if errors.Is(err, admin.ErrNoAccountChange) {
			code = api.NoAccountChange
		}
		return api.UpdateAccount400JSONResponse{BadRequestJSONResponse: badRequest(code)}, nil
	}
	response := api.UpdateAccount200JSONResponse{Body: api.AccountUpdateResult{
		Account: accountResponse(account), SessionRevoked: revoked,
	}}
	if revoked {
		cookie := expiredSessionCookie(requestSecurityFromContext(ctx).CookieSecure)
		response.Headers.SetCookie = &cookie
	}
	return response, nil
}

func (s apiServer) UpdateAccountLocale(ctx context.Context, request api.UpdateAccountLocaleRequestObject) (api.UpdateAccountLocaleResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateAccountLocale401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateAccountLocale403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil || !request.Body.Locale.Valid() {
		return api.UpdateAccountLocale400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	account, err := s.administrator.UpdateLocale(ctx, string(request.Body.Locale))
	if err != nil {
		return nil, err
	}
	return api.UpdateAccountLocale200JSONResponse(accountResponse(account)), nil
}

func (s apiServer) StartTOTPEnrollment(ctx context.Context, request api.StartTOTPEnrollmentRequestObject) (api.StartTOTPEnrollmentResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.StartTOTPEnrollment401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.StartTOTPEnrollment403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.StartTOTPEnrollment400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	enrollment, err := s.administrator.StartTOTPEnrollment(ctx, request.Body.CurrentPassword)
	if errors.Is(err, admin.ErrCurrentPassword) {
		return api.StartTOTPEnrollment400JSONResponse{BadRequestJSONResponse: badRequest(api.CurrentPasswordInvalid)}, nil
	}
	if errors.Is(err, admin.ErrTOTPAlreadyEnabled) {
		return api.StartTOTPEnrollment409JSONResponse{ConflictJSONResponse: conflict(api.TotpAlreadyEnabled)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.StartTOTPEnrollment200JSONResponse{
		Secret: enrollment.Secret, ProvisioningUri: enrollment.ProvisioningURI,
	}, nil
}

func (s apiServer) ConfirmTOTPEnrollment(ctx context.Context, request api.ConfirmTOTPEnrollmentRequestObject) (api.ConfirmTOTPEnrollmentResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.ConfirmTOTPEnrollment401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.ConfirmTOTPEnrollment403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.ConfirmTOTPEnrollment400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	account, err := s.administrator.ConfirmTOTPEnrollment(ctx, request.Body.Code)
	if errors.Is(err, admin.ErrInvalidTOTP) {
		return api.ConfirmTOTPEnrollment400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidTotp)}, nil
	}
	if errors.Is(err, admin.ErrTOTPAlreadyEnabled) {
		return api.ConfirmTOTPEnrollment409JSONResponse{ConflictJSONResponse: conflict(api.TotpAlreadyEnabled)}, nil
	}
	if errors.Is(err, admin.ErrTOTPEnrollment) {
		return api.ConfirmTOTPEnrollment409JSONResponse{ConflictJSONResponse: conflict(api.TotpEnrollmentNotStarted)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.ConfirmTOTPEnrollment200JSONResponse(accountResponse(account)), nil
}

func (s apiServer) DisableTOTP(ctx context.Context, request api.DisableTOTPRequestObject) (api.DisableTOTPResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DisableTOTP401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DisableTOTP403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.DisableTOTP400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	err = s.administrator.DisableTOTP(ctx, request.Body.CurrentPassword, request.Body.Code)
	if errors.Is(err, admin.ErrCurrentPassword) {
		return api.DisableTOTP400JSONResponse{BadRequestJSONResponse: badRequest(api.CurrentPasswordInvalid)}, nil
	}
	if errors.Is(err, admin.ErrInvalidTOTP) {
		return api.DisableTOTP400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidTotp)}, nil
	}
	if errors.Is(err, admin.ErrTOTPNotEnabled) {
		return api.DisableTOTP409JSONResponse{ConflictJSONResponse: conflict(api.TotpNotEnabled)}, nil
	}
	if err != nil {
		return nil, err
	}
	cookie := expiredSessionCookie(requestSecurityFromContext(ctx).CookieSecure)
	return api.DisableTOTP204Response{Headers: api.DisableTOTP204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s apiServer) RevokeAllSessions(ctx context.Context, request api.RevokeAllSessionsRequestObject) (api.RevokeAllSessionsResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.RevokeAllSessions401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.RevokeAllSessions403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if err := s.administrator.RevokeAllSessions(ctx); err != nil {
		return nil, err
	}
	cookie := expiredSessionCookie(requestSecurityFromContext(ctx).CookieSecure)
	return api.RevokeAllSessions204Response{Headers: api.RevokeAllSessions204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s apiServer) GetSystemStatus(ctx context.Context, _ api.GetSystemStatusRequestObject) (api.GetSystemStatusResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetSystemStatus401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	security := requestSecurityFromContext(ctx)
	transport := api.Http
	if security.CookieSecure {
		transport = api.Https
	}
	return api.GetSystemStatus200JSONResponse{
		Service:                  api.IpchronicleCenter,
		Status:                   api.Ok,
		Version:                  s.version,
		ConfigSchemaVersion:      s.configSchemaVersion,
		HistorySchemaVersion:     s.historySchemaVersion,
		TransportSecurity:        transport,
		TransportWarning:         transport == api.Http,
		ExternalOriginConfigured: s.externalOriginConfigured,
		TrustedProxyConfigured:   s.trustedProxyConfigured,
	}, nil
}

func (s apiServer) ListNodes(ctx context.Context, _ api.ListNodesRequestObject) (api.ListNodesResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNodes401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	records, err := s.nodes.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]api.Node, 0, len(records))
	for _, record := range records {
		items = append(items, nodeResponse(record))
	}
	return api.ListNodes200JSONResponse{Items: items}, nil
}

func (s apiServer) UpdateNode(ctx context.Context, request api.UpdateNodeRequestObject) (api.UpdateNodeResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNode401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNode403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNode400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	node, err := s.nodes.SetEnabled(ctx, request.NodeId, request.Body.Enabled)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.UpdateNode404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.UpdateNode409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.UpdateNode409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNode200JSONResponse(nodeResponse(node)), nil
}

func (s apiServer) RevokeNode(ctx context.Context, request api.RevokeNodeRequestObject) (api.RevokeNodeResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.RevokeNode401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.RevokeNode403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	node, err := s.nodes.Revoke(ctx, request.NodeId)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.RevokeNode404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.RevokeNode409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.RevokeNode200JSONResponse(nodeResponse(node)), nil
}

func (s apiServer) StartNodeSyncSession(ctx context.Context, request api.StartNodeSyncSessionRequestObject) (api.StartNodeSyncSessionResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.StartNodeSyncSession401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.StartNodeSyncSession403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	node, err := s.nodes.StartSyncSession(ctx, request.NodeId)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.StartNodeSyncSession404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.StartNodeSyncSession409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.StartNodeSyncSession409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case errors.Is(err, nodes.ErrNodeSyncUnsupported):
		return api.StartNodeSyncSession409JSONResponse{ConflictJSONResponse: conflict(api.NodeSyncUnsupported)}, nil
	case err != nil:
		return nil, err
	}
	return api.StartNodeSyncSession200JSONResponse(nodeResponse(node)), nil
}

func (s apiServer) StopNodeSyncSession(ctx context.Context, request api.StopNodeSyncSessionRequestObject) (api.StopNodeSyncSessionResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.StopNodeSyncSession401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.StopNodeSyncSession403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	node, err := s.nodes.StopSyncSession(ctx, request.NodeId)
	if errors.Is(err, nodes.ErrNodeNotFound) {
		return api.StopNodeSyncSession404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.StopNodeSyncSession200JSONResponse(nodeResponse(node)), nil
}

func (s apiServer) DeleteNode(ctx context.Context, request api.DeleteNodeRequestObject) (api.DeleteNodeResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DeleteNode401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DeleteNode403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	deletion, err := s.nodes.Delete(ctx, request.NodeId)
	if errors.Is(err, nodes.ErrNodeNotFound) {
		return api.DeleteNode404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteNode202JSONResponse(deletionResponse(deletion)), nil
}

func (s apiServer) GetNodeNetwork(ctx context.Context, request api.GetNodeNetworkRequestObject) (api.GetNodeNetworkResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetNodeNetwork401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	state, err := s.nodes.Network(ctx, request.NodeId)
	if errors.Is(err, nodes.ErrNodeNotFound) {
		return api.GetNodeNetwork404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetNodeNetwork200JSONResponse(networkStateResponse(state)), nil
}

func (s apiServer) CreateNodeEgress(ctx context.Context, request api.CreateNodeEgressRequestObject) (api.CreateNodeEgressResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateNodeEgress401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateNodeEgress403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateNodeEgress400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	egress, err := s.nodes.CreateEgress(ctx, request.NodeId, nodes.NetworkEgressSelector{
		Kind: string(request.Body.Kind), Family: string(request.Body.Family),
		InterfaceName: request.Body.InterfaceName, SourceAddress: request.Body.SourceAddress,
	})
	switch {
	case errors.Is(err, nodes.ErrInvalidEgressCandidate):
		return api.CreateNodeEgress400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidEgressCandidate)}, nil
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.CreateNodeEgress404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNetworkInventoryUnavailable):
		return api.CreateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NetworkInventoryUnavailable)}, nil
	case errors.Is(err, nodes.ErrEgressAlreadyExists):
		return api.CreateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.EgressAlreadyExists)}, nil
	case errors.Is(err, nodes.ErrEgressLimitReached):
		return api.CreateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.EgressLimitReached)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.CreateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.CreateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateNodeEgress201JSONResponse(egressResponse(egress)), nil
}

func (s apiServer) UpdateNodeEgress(ctx context.Context, request api.UpdateNodeEgressRequestObject) (api.UpdateNodeEgressResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNodeEgress401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNodeEgress403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNodeEgress400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	egress, err := s.nodes.SetEgressEnabled(ctx, request.NodeId, request.EgressId, request.Body.Enabled)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound), errors.Is(err, nodes.ErrEgressNotFound):
		return api.UpdateNodeEgress404JSONResponse{NotFoundJSONResponse: notFound(api.EgressNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.UpdateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.UpdateNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNodeEgress200JSONResponse(egressResponse(egress)), nil
}

func (s apiServer) DeleteNodeEgress(ctx context.Context, request api.DeleteNodeEgressRequestObject) (api.DeleteNodeEgressResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DeleteNodeEgress401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DeleteNodeEgress403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	err = s.nodes.DeleteEgress(ctx, request.NodeId, request.EgressId)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound), errors.Is(err, nodes.ErrEgressNotFound):
		return api.DeleteNodeEgress404JSONResponse{NotFoundJSONResponse: notFound(api.EgressNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.DeleteNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.DeleteNodeEgress409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.DeleteNodeEgress204Response{}, nil
}

func (s apiServer) GetAgentEnrollment(ctx context.Context, _ api.GetAgentEnrollmentRequestObject) (api.GetAgentEnrollmentResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetAgentEnrollment401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	enrollment, err := s.nodes.Enrollment(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetAgentEnrollment200JSONResponse(enrollmentResponse(enrollment, requestSecurityFromContext(ctx).ExpectedOrigin, s.version)), nil
}

func (s apiServer) UpdateAgentEnrollment(ctx context.Context, request api.UpdateAgentEnrollmentRequestObject) (api.UpdateAgentEnrollmentResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateAgentEnrollment401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateAgentEnrollment403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateAgentEnrollment400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	enrollment, err := s.nodes.SetEnrollmentEnabled(ctx, request.Body.Enabled)
	if errors.Is(err, nodes.ErrEnrollmentKeyMissing) {
		return api.UpdateAgentEnrollment409JSONResponse{ConflictJSONResponse: conflict(api.RegistrationKeyNotInitialized)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateAgentEnrollment200JSONResponse(enrollmentResponse(enrollment, requestSecurityFromContext(ctx).ExpectedOrigin, s.version)), nil
}

func (s apiServer) RotateAgentEnrollmentKey(ctx context.Context, request api.RotateAgentEnrollmentKeyRequestObject) (api.RotateAgentEnrollmentKeyResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.RotateAgentEnrollmentKey401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.RotateAgentEnrollmentKey403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	enrollment, err := s.nodes.RotateEnrollmentKey(ctx)
	if err != nil {
		return nil, err
	}
	return api.RotateAgentEnrollmentKey200JSONResponse(enrollmentResponse(enrollment, requestSecurityFromContext(ctx).ExpectedOrigin, s.version)), nil
}

func (s apiServer) RegisterAgent(ctx context.Context, request api.RegisterAgentRequestObject) (api.RegisterAgentResponseObject, error) {
	if request.Body == nil {
		return api.RegisterAgent400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	registration, err := s.nodes.Register(ctx, request.Body.RegistrationKey, metadataFromAPI(request.Body.Metadata))
	switch {
	case errors.Is(err, nodes.ErrInvalidMetadata):
		return api.RegisterAgent400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	case errors.Is(err, nodes.ErrEnrollmentKeyInvalid):
		return api.RegisterAgent401JSONResponse{AgentUnauthorizedJSONResponse: agentUnauthorized(api.RegistrationKeyInvalid)}, nil
	case errors.Is(err, nodes.ErrEnrollmentDisabled):
		return api.RegisterAgent403JSONResponse{AgentForbiddenJSONResponse: agentForbidden(api.RegistrationDisabled)}, nil
	case err != nil:
		return nil, err
	}
	return api.RegisterAgent201JSONResponse{
		NodeId: registration.NodeID, Credential: registration.Credential,
		PollIntervalSeconds: int(nodes.PollInterval / time.Second),
	}, nil
}

func (s apiServer) PollAgent(ctx context.Context, request api.PollAgentRequestObject) (api.PollAgentResponseObject, error) {
	if request.Body == nil {
		return api.PollAgent400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	credential := bearerToken(requestSecurityFromContext(ctx).Authorization)
	poll, err := s.nodes.Poll(
		ctx, credential, metadataFromAPI(request.Body.Metadata),
		request.Body.AppliedConfigurationRevision, request.Body.ConfigurationError,
		request.Body.ConfigurationErrorRevision, networkInventoryFromAPI(request.Body.NetworkInventory),
		request.Body.NetworkInventoryError,
	)
	switch {
	case errors.Is(err, nodes.ErrInvalidMetadata):
		return api.PollAgent400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	case errors.Is(err, nodes.ErrAgentUnauthenticated):
		return api.PollAgent401JSONResponse{AgentUnauthorizedJSONResponse: agentUnauthorized(api.AgentUnauthenticated)}, nil
	case errors.Is(err, nodes.ErrAgentRevoked):
		return api.PollAgent403JSONResponse{AgentForbiddenJSONResponse: agentForbidden(api.AgentRevoked)}, nil
	case err != nil:
		return nil, err
	}
	result := api.PollAgent200JSONResponse{
		CenterVersion: s.version, DesiredConfigurationRevision: poll.DesiredConfigurationRevision,
		Enabled: poll.Enabled, PollIntervalSeconds: int(nodes.PollInterval / time.Second),
	}
	if poll.SyncSession != nil {
		result.SyncSession = &api.AgentSyncSession{
			Id: poll.SyncSession.ID, ExpiresAt: poll.SyncSession.ExpiresAt,
			WebsocketPath: "/api/v1/agent/sync/" + poll.SyncSession.ID.String(),
		}
	}
	return result, nil
}

func (s apiServer) GetAgentConfiguration(ctx context.Context, _ api.GetAgentConfigurationRequestObject) (api.GetAgentConfigurationResponseObject, error) {
	credential := bearerToken(requestSecurityFromContext(ctx).Authorization)
	configuration, err := s.nodes.Configuration(ctx, credential)
	switch {
	case errors.Is(err, nodes.ErrAgentUnauthenticated):
		return api.GetAgentConfiguration401JSONResponse{AgentUnauthorizedJSONResponse: agentUnauthorized(api.AgentUnauthenticated)}, nil
	case errors.Is(err, nodes.ErrAgentRevoked):
		return api.GetAgentConfiguration403JSONResponse{AgentForbiddenJSONResponse: agentForbidden(api.AgentRevoked)}, nil
	case err != nil:
		return nil, err
	}
	egresses := make([]api.AgentEgressConfiguration, 0, len(configuration.Egresses))
	for _, egress := range configuration.Egresses {
		egresses = append(egresses, api.AgentEgressConfiguration{
			Id: egress.ID, Kind: api.NetworkEgressKind(egress.Kind), Family: api.AddressFamily(egress.Family),
			InterfaceName: egress.InterfaceName, SourceAddress: egress.SourceAddress, Enabled: egress.Enabled,
			LightweightIntervalSeconds: egress.LightweightIntervalSeconds,
			ProbeOnAddressChange:       egress.ProbeOnAddressChange,
		})
	}
	return api.GetAgentConfiguration200JSONResponse{
		SchemaVersion: api.AgentConfigurationSnapshotSchemaVersion(configuration.SchemaVersion),
		Revision:      configuration.Revision, Enabled: configuration.Enabled,
		HistoryGeneration: configuration.HistoryGeneration, Egresses: egresses,
	}, nil
}

func (s apiServer) authorize(ctx context.Context, mutation bool, csrf string) (admin.Principal, api.ErrorCode, error) {
	security := requestSecurityFromContext(ctx)
	principal, err := s.administrator.Authenticate(ctx, security.SessionToken)
	if errors.Is(err, admin.ErrUnauthenticated) {
		return admin.Principal{}, api.Unauthenticated, nil
	}
	if err != nil {
		return admin.Principal{}, "", err
	}
	if !mutation {
		return principal, "", nil
	}
	if !originMatches(security.Origin, security.ExpectedOrigin) {
		return admin.Principal{}, api.OriginNotAllowed, nil
	}
	if !s.administrator.ValidateCSRF(principal.Token, csrf) {
		return admin.Principal{}, api.CsrfFailed, nil
	}
	return principal, "", nil
}

func accountResponse(account admin.Account) api.Account {
	locale := api.En
	if account.Locale == string(api.ZhCN) {
		locale = api.ZhCN
	}
	return api.Account{
		Username:               account.Username,
		Locale:                 locale,
		UsesDefaultCredentials: account.UsesDefaultCredentials,
		TotpEnabled:            account.TOTPEnabled,
	}
}

func errorResponse(code api.ErrorCode, parameters map[string]string) api.ErrorResponse {
	response := api.ErrorResponse{Code: code}
	if len(parameters) > 0 {
		response.Parameters = &parameters
	}
	return response
}

func unauthorized(code api.ErrorCode) api.UnauthorizedJSONResponse {
	return api.UnauthorizedJSONResponse(errorResponse(code, nil))
}

func forbidden(code api.ErrorCode) api.ForbiddenJSONResponse {
	return api.ForbiddenJSONResponse(errorResponse(code, nil))
}

func badRequest(code api.ErrorCode) api.BadRequestJSONResponse {
	return api.BadRequestJSONResponse(errorResponse(code, nil))
}

func conflict(code api.ErrorCode) api.ConflictJSONResponse {
	return api.ConflictJSONResponse(errorResponse(code, nil))
}

func notFound(code api.ErrorCode) api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse(errorResponse(code, nil))
}

func agentUnauthorized(code api.ErrorCode) api.AgentUnauthorizedJSONResponse {
	return api.AgentUnauthorizedJSONResponse(errorResponse(code, nil))
}

func agentForbidden(code api.ErrorCode) api.AgentForbiddenJSONResponse {
	return api.AgentForbiddenJSONResponse(errorResponse(code, nil))
}

func metadataFromAPI(metadata api.AgentMetadata) nodes.Metadata {
	return nodes.Metadata{
		Hostname: metadata.Hostname, AgentVersion: metadata.AgentVersion,
		OperatingSystem: string(metadata.OperatingSystem), Architecture: string(metadata.Architecture),
		Capabilities: metadata.Capabilities,
	}
}

func networkInventoryFromAPI(inventory *api.NetworkInventory) *nodes.NetworkInventory {
	if inventory == nil {
		return nil
	}
	result := &nodes.NetworkInventory{CapturedAt: inventory.CapturedAt}
	result.Interfaces = make([]nodes.NetworkInterface, 0, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		result.Interfaces = append(result.Interfaces, nodes.NetworkInterface{
			Name: item.Name, Index: item.Index, Up: item.Up, Loopback: item.Loopback,
		})
	}
	result.Addresses = make([]nodes.NetworkAddress, 0, len(inventory.Addresses))
	for _, item := range inventory.Addresses {
		result.Addresses = append(result.Addresses, nodes.NetworkAddress{
			InterfaceName: item.InterfaceName, Address: item.Address, PrefixLength: item.PrefixLength,
			Family: string(item.Family), Scope: string(item.Scope), Temporary: item.Temporary,
			Tentative: item.Tentative, Deprecated: item.Deprecated, Duplicate: item.Duplicate,
		})
	}
	result.Routes = make([]nodes.NetworkRoute, 0, len(inventory.Routes))
	for _, item := range inventory.Routes {
		result.Routes = append(result.Routes, nodes.NetworkRoute{
			InterfaceName: item.InterfaceName, Family: string(item.Family), Destination: item.Destination,
			Gateway: item.Gateway, Metric: item.Metric, Default: item.Default,
		})
	}
	return result
}

func enrollmentResponse(enrollment nodes.Enrollment, centerURL, centerVersion string) api.AgentEnrollmentSettings {
	response := api.AgentEnrollmentSettings{Enabled: enrollment.Enabled, HasKey: enrollment.HasKey}
	if !enrollment.HasKey {
		return response
	}
	installerURL := "https://github.com/ipchronicle/ipchronicle/releases/latest/download/install-agent.sh"
	versionArgument := ""
	if centerVersion != "dev" {
		version := strings.TrimPrefix(centerVersion, "v")
		installerURL = "https://github.com/ipchronicle/ipchronicle/releases/download/v" + version + "/install-agent.sh"
		versionArgument = " --version " + shellQuote(version)
	}
	command := "curl --proto '=https' --tlsv1.2 -fsSL " + shellQuote(installerURL) + " | " +
		"sh -s -- --center-url " + shellQuote(centerURL) + " --registration-key " +
		shellQuote(enrollment.Key) + versionArgument
	response.InstallationCommand = &command
	rotatedAt := enrollment.RotatedAt
	response.RotatedAt = &rotatedAt
	return response
}

func nodeResponse(node nodes.Node) api.Node {
	return api.Node{
		Id: node.ID, Name: node.Name, Hostname: node.Hostname, Status: api.NodeStatus(node.Status),
		Enabled: node.Enabled, AgentVersion: node.AgentVersion,
		OperatingSystem: api.AgentPlatform(node.OperatingSystem), Architecture: api.AgentArchitecture(node.Architecture),
		Capabilities:                 node.Capabilities,
		DesiredConfigurationRevision: node.DesiredConfigurationRevision,
		AppliedConfigurationRevision: node.AppliedConfigurationRevision,
		ConfigurationStatus:          api.NodeConfigurationStatus(node.ConfigurationStatus),
		ConfigurationError:           node.ConfigurationError,
		DeletionStatus:               deletionStatus(node.DeletionStatus),
		DeletionError:                node.DeletionError,
		SyncStatus:                   syncStatus(node.SyncStatus),
		SyncExpiresAt:                node.SyncExpiresAt,
		RegisteredAt:                 node.RegisteredAt, LastSeenAt: node.LastSeenAt,
	}
}

func networkStateResponse(state nodes.NodeNetworkState) api.NodeNetworkState {
	response := api.NodeNetworkState{
		InventoryError: state.InventoryError, InventoryReceivedAt: state.InventoryReceivedAt,
		Egresses:   make([]api.NetworkEgress, 0, len(state.Egresses)),
		Candidates: make([]api.NetworkEgressCandidate, 0, len(state.Candidates)),
	}
	if state.Inventory != nil {
		response.Inventory = networkInventoryResponse(*state.Inventory)
	}
	for _, item := range state.Egresses {
		response.Egresses = append(response.Egresses, egressResponse(item))
	}
	for _, item := range state.Candidates {
		candidate := api.NetworkEgressCandidate{
			Kind: api.NetworkEgressCandidateKind(item.Kind), Family: api.AddressFamily(item.Family),
			InterfaceName: item.InterfaceName, SourceAddress: item.SourceAddress,
			Temporary: item.Temporary, Eligible: item.Eligible, ConfiguredEgressId: item.ConfiguredEgressID,
		}
		if item.Scope != nil {
			value := api.NetworkAddressScope(*item.Scope)
			candidate.Scope = &value
		}
		if item.UnavailableReason != nil {
			value := api.NetworkEgressCandidateUnavailableReason(*item.UnavailableReason)
			candidate.UnavailableReason = &value
		}
		response.Candidates = append(response.Candidates, candidate)
	}
	return response
}

func networkInventoryResponse(inventory nodes.NetworkInventory) *api.NetworkInventory {
	response := &api.NetworkInventory{CapturedAt: inventory.CapturedAt}
	response.Interfaces = make([]api.NetworkInterface, 0, len(inventory.Interfaces))
	for _, item := range inventory.Interfaces {
		response.Interfaces = append(response.Interfaces, api.NetworkInterface{
			Name: item.Name, Index: item.Index, Up: item.Up, Loopback: item.Loopback,
		})
	}
	response.Addresses = make([]api.NetworkAddress, 0, len(inventory.Addresses))
	for _, item := range inventory.Addresses {
		response.Addresses = append(response.Addresses, api.NetworkAddress{
			InterfaceName: item.InterfaceName, Address: item.Address, PrefixLength: item.PrefixLength,
			Family: api.AddressFamily(item.Family), Scope: api.NetworkAddressScope(item.Scope),
			Temporary: item.Temporary, Tentative: item.Tentative, Deprecated: item.Deprecated, Duplicate: item.Duplicate,
		})
	}
	response.Routes = make([]api.NetworkRoute, 0, len(inventory.Routes))
	for _, item := range inventory.Routes {
		response.Routes = append(response.Routes, api.NetworkRoute{
			InterfaceName: item.InterfaceName, Family: api.AddressFamily(item.Family),
			Destination: item.Destination, Gateway: item.Gateway, Metric: item.Metric, Default: item.Default,
		})
	}
	return response
}

func egressResponse(egress nodes.NetworkEgress) api.NetworkEgress {
	return api.NetworkEgress{
		Id: egress.ID, NodeId: egress.NodeID, Name: egress.Name,
		Kind: api.NetworkEgressKind(egress.Kind), Family: api.AddressFamily(egress.Family),
		InterfaceName: egress.InterfaceName, SourceAddress: egress.SourceAddress,
		Enabled: egress.Enabled, Available: egress.Available, Automatic: egress.Automatic,
		LightweightIntervalSeconds: egress.LightweightIntervalSeconds,
		ProbeOnAddressChange:       egress.ProbeOnAddressChange,
	}
}

func deletionResponse(deletion nodes.Deletion) api.NodeDeletion {
	return api.NodeDeletion{
		NodeId: deletion.NodeID, Status: api.NodeDeletionStatus(deletion.Status),
		RequestedAt: deletion.RequestedAt, Error: deletion.Error,
	}
}

func deletionStatus(status *string) *api.NodeDeletionStatus {
	if status == nil {
		return nil
	}
	value := api.NodeDeletionStatus(*status)
	return &value
}

func syncStatus(status *string) *api.NodeSyncStatus {
	if status == nil {
		return nil
	}
	value := api.NodeSyncStatus(*status)
	return &value
}

func bearerToken(header string) string {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n") {
		return ""
	}
	return token
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func csrfValue(value *api.CSRFToken) string {
	if value == nil {
		return ""
	}
	return *value
}

func sessionCookie(token string, expiresAt time.Time, secure bool) string {
	return (&http.Cookie{
		Name:     administratorSessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   sessionLifetimeSeconds,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

func expiredSessionCookie(secure bool) string {
	return (&http.Cookie{
		Name:     administratorSessionCookie,
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

const sessionLifetimeSeconds = 30 * 24 * 60 * 60
