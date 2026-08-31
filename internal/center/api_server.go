package center

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	centerhistory "github.com/ipchronicle/ipchronicle/internal/center/history"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/center/notifications"
	"github.com/ipchronicle/ipchronicle/internal/center/systemsettings"
	centerupdates "github.com/ipchronicle/ipchronicle/internal/center/updates"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type apiServer struct {
	version                string
	revision               string
	administrator          *admin.Service
	nodes                  *nodes.Service
	notifications          *notifications.Service
	updates                *centerupdates.Service
	systemSettings         *systemsettings.Service
	configSchemaVersion    int64
	historySchemaVersion   int64
	trustedProxyConfigured bool
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
	transport := api.SystemStatusTransportSecurityHttp
	if security.CookieSecure {
		transport = api.SystemStatusTransportSecurityHttps
	}
	settings, err := s.systemSettings.Get(ctx)
	if err != nil {
		return nil, err
	}
	externalOriginMode := api.Automatic
	if settings.ExternalOrigin != "" {
		externalOriginMode = api.Custom
	}
	return api.GetSystemStatus200JSONResponse{
		Service:                api.IpchronicleCenter,
		Status:                 api.Ok,
		Version:                s.version,
		SourceRevision:         s.revision,
		ConfigSchemaVersion:    s.configSchemaVersion,
		HistorySchemaVersion:   s.historySchemaVersion,
		TransportSecurity:      transport,
		TransportWarning:       transport == api.SystemStatusTransportSecurityHttp,
		ExternalOriginMode:     externalOriginMode,
		TrustedProxyConfigured: s.trustedProxyConfigured,
	}, nil
}

func (s apiServer) GetSystemSettings(ctx context.Context, _ api.GetSystemSettingsRequestObject) (api.GetSystemSettingsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetSystemSettings401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	settings, err := s.systemSettings.Get(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetSystemSettings200JSONResponse(systemSettingsResponse(settings, requestSecurityFromContext(ctx).ExpectedOrigin)), nil
}

func (s apiServer) UpdateSystemSettings(ctx context.Context, request api.UpdateSystemSettingsRequestObject) (api.UpdateSystemSettingsResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateSystemSettings401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateSystemSettings403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateSystemSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	settings, err := s.systemSettings.Update(ctx, request.Body.ExternalOrigin)
	if errors.Is(err, systemsettings.ErrInvalidExternalOrigin) {
		return api.UpdateSystemSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidSystemSettings)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateSystemSettings200JSONResponse(systemSettingsResponse(settings, requestSecurityFromContext(ctx).ExpectedOrigin)), nil
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
	node, err := s.nodes.Update(ctx, request.NodeId, request.Body.Name, request.Body.Enabled)
	switch {
	case errors.Is(err, nodes.ErrInvalidNodeName):
		return api.UpdateNode400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
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

func (s apiServer) GetNodeProbe(ctx context.Context, request api.GetNodeProbeRequestObject) (api.GetNodeProbeResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetNodeProbe401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	state, err := s.nodes.Probe(ctx, request.NodeId)
	if errors.Is(err, nodes.ErrNodeNotFound) {
		return api.GetNodeProbe404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetNodeProbe200JSONResponse(probeStateResponse(state)), nil
}

func (s apiServer) UpdateNodeProbeSettings(ctx context.Context, request api.UpdateNodeProbeSettingsRequestObject) (api.UpdateNodeProbeSettingsResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNodeProbeSettings401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNodeProbeSettings403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNodeProbeSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	state, err := s.nodes.UpdateProbeSettings(ctx, request.NodeId, nodes.ProbeSettingsUpdate{
		Schedule: nodes.ProbeSchedule{
			Enabled: request.Body.Schedule.Enabled,
			Cron:    request.Body.Schedule.Cron, Timezone: request.Body.Schedule.Timezone,
		},
		LowMemoryOverride: request.Body.LowMemoryOverride, ProbeOnNewAddress: request.Body.ProbeOnNewAddress,
	})
	switch {
	case errors.Is(err, nodes.ErrInvalidProbeSettings):
		return api.UpdateNodeProbeSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidProbeSettings)}, nil
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.UpdateNodeProbeSettings404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.UpdateNodeProbeSettings409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.UpdateNodeProbeSettings409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNodeProbeSettings200JSONResponse(probeStateResponse(state)), nil
}

func (s apiServer) CreateCompleteProbeTask(ctx context.Context, request api.CreateCompleteProbeTaskRequestObject) (api.CreateCompleteProbeTaskResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateCompleteProbeTask401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateCompleteProbeTask403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateCompleteProbeTask400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	task, err := s.nodes.CreateCompleteProbeTask(ctx, request.NodeId, request.Body.PublicAddressIds)
	switch {
	case errors.Is(err, nodes.ErrInvalidProbeTargets):
		return api.CreateCompleteProbeTask400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.CreateCompleteProbeTask404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDisabled):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.NodeDisabled)}, nil
	case errors.Is(err, nodes.ErrNodeOffline):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.NodeOffline)}, nil
	case errors.Is(err, nodes.ErrProbeTaskSlotOccupied):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.ProbeTaskSlotOccupied)}, nil
	case errors.Is(err, nodes.ErrProbeAlreadyRunning):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.ProbeAlreadyRunning)}, nil
	case errors.Is(err, nodes.ErrProbePausedLowMemory):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.ProbePausedLowMemory)}, nil
	case errors.Is(err, nodes.ErrProbeTargetUnavailable):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.ProbeTargetUnavailable)}, nil
	case errors.Is(err, nodes.ErrNoEnabledEgress):
		return api.CreateCompleteProbeTask409JSONResponse{ConflictJSONResponse: conflict(api.NoEnabledEgress)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateCompleteProbeTask202JSONResponse(probeTaskResponse(task)), nil
}

func (s apiServer) GetProbeRun(ctx context.Context, request api.GetProbeRunRequestObject) (api.GetProbeRunResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetProbeRun401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	run, err := s.nodes.ProbeRun(ctx, request.RunId)
	if errors.Is(err, nodes.ErrProbeRunNotFound) {
		return api.GetProbeRun404JSONResponse{NotFoundJSONResponse: notFound(api.ProbeRunNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProbeRun200JSONResponse(probeRunResponse(run)), nil
}

func (s apiServer) GetProbeSnapshot(ctx context.Context, request api.GetProbeSnapshotRequestObject) (api.GetProbeSnapshotResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetProbeSnapshot401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	snapshot, err := s.nodes.ProbeSnapshot(ctx, request.SnapshotId)
	if errors.Is(err, nodes.ErrProbeSnapshotNotFound) {
		return api.GetProbeSnapshot404JSONResponse{NotFoundJSONResponse: notFound(api.ProbeSnapshotNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProbeSnapshot200JSONResponse(probeSnapshotResponse(snapshot)), nil
}

func (s apiServer) StarProbeSnapshot(ctx context.Context, request api.StarProbeSnapshotRequestObject) (api.StarProbeSnapshotResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.StarProbeSnapshot401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.StarProbeSnapshot403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	snapshot, err := s.nodes.SetProbeSnapshotStarred(ctx, request.SnapshotId, true)
	if errors.Is(err, nodes.ErrProbeSnapshotNotFound) {
		return api.StarProbeSnapshot404JSONResponse{NotFoundJSONResponse: notFound(api.ProbeSnapshotNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.StarProbeSnapshot200JSONResponse(probeSnapshotResponse(snapshot)), nil
}

func (s apiServer) UnstarProbeSnapshot(ctx context.Context, request api.UnstarProbeSnapshotRequestObject) (api.UnstarProbeSnapshotResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UnstarProbeSnapshot401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UnstarProbeSnapshot403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	snapshot, err := s.nodes.SetProbeSnapshotStarred(ctx, request.SnapshotId, false)
	if errors.Is(err, nodes.ErrProbeSnapshotNotFound) {
		return api.UnstarProbeSnapshot404JSONResponse{NotFoundJSONResponse: notFound(api.ProbeSnapshotNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UnstarProbeSnapshot200JSONResponse(probeSnapshotResponse(snapshot)), nil
}

func (s apiServer) GetHistoryState(ctx context.Context, _ api.GetHistoryStateRequestObject) (api.GetHistoryStateResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetHistoryState401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	state, err := s.nodes.History(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetHistoryState200JSONResponse(historyStateResponse(state)), nil
}

func (s apiServer) ResetHistory(ctx context.Context, request api.ResetHistoryRequestObject) (api.ResetHistoryResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.ResetHistory401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.ResetHistory403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	state, err := s.nodes.ResetHistory(ctx)
	if err != nil {
		return nil, err
	}
	return api.ResetHistory200JSONResponse(historyStateResponse(state)), nil
}

func (s apiServer) ListHistoryProbeSnapshots(ctx context.Context, request api.ListHistoryProbeSnapshotsRequestObject) (api.ListHistoryProbeSnapshotsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListHistoryProbeSnapshots401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	filter := historyFilter(
		request.Params.NodeId, request.Params.EgressId, request.Params.From, request.Params.To,
		request.Params.Page, request.Params.PageSize,
	)
	if request.Params.RunStatus != nil {
		filter.RunStatus = string(*request.Params.RunStatus)
	}
	if request.Params.Trigger != nil {
		filter.Trigger = string(*request.Params.Trigger)
	}
	filter.Changed = request.Params.Changed
	if request.Params.FormatStatus != nil {
		filter.FormatStatus = string(*request.Params.FormatStatus)
	}
	page, err := s.nodes.ListHistoryProbeSnapshots(ctx, filter)
	if err != nil {
		return nil, err
	}
	return api.ListHistoryProbeSnapshots200JSONResponse(probeSnapshotHistoryPageResponse(page)), nil
}

func (s apiServer) ListHistoryAddressEvents(ctx context.Context, request api.ListHistoryAddressEventsRequestObject) (api.ListHistoryAddressEventsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListHistoryAddressEvents401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	filter := historyFilter(
		request.Params.NodeId, request.Params.EgressId, request.Params.From, request.Params.To,
		request.Params.Page, request.Params.PageSize,
	)
	if request.Params.EventKind != nil {
		filter.EventKind = string(*request.Params.EventKind)
	}
	if request.Params.Family != nil {
		filter.Family = string(*request.Params.Family)
	}
	if request.Params.GapPage != nil {
		filter.GapPage = *request.Params.GapPage
	}
	page, err := s.nodes.ListHistoryAddressEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	return api.ListHistoryAddressEvents200JSONResponse(addressHistoryPageResponse(page)), nil
}

func (s apiServer) ListHistoryProbeGaps(ctx context.Context, request api.ListHistoryProbeGapsRequestObject) (api.ListHistoryProbeGapsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListHistoryProbeGaps401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	page, err := s.nodes.ListHistoryProbeGaps(ctx, historyFilter(
		request.Params.NodeId, request.Params.EgressId, request.Params.From, request.Params.To,
		request.Params.Page, request.Params.PageSize,
	))
	if err != nil {
		return nil, err
	}
	return api.ListHistoryProbeGaps200JSONResponse(probeHistoryGapPageResponse(page)), nil
}

func (s apiServer) ListHistoryFormatEvents(ctx context.Context, request api.ListHistoryFormatEventsRequestObject) (api.ListHistoryFormatEventsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListHistoryFormatEvents401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	page, err := s.nodes.ListHistoryFormatEvents(ctx, historyFilter(
		request.Params.NodeId, request.Params.EgressId, request.Params.From, request.Params.To,
		request.Params.Page, request.Params.PageSize,
	))
	if err != nil {
		return nil, err
	}
	return api.ListHistoryFormatEvents200JSONResponse(probeFormatEventPageResponse(page)), nil
}

func (s apiServer) CompareProbeSnapshots(ctx context.Context, request api.CompareProbeSnapshotsRequestObject) (api.CompareProbeSnapshotsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.CompareProbeSnapshots401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	comparison, err := s.nodes.CompareProbeSnapshots(ctx, request.Params.BeforeSnapshotId, request.Params.AfterSnapshotId)
	switch {
	case errors.Is(err, nodes.ErrProbeSnapshotNotFound):
		return api.CompareProbeSnapshots404JSONResponse{NotFoundJSONResponse: notFound(api.ProbeSnapshotNotFound)}, nil
	case errors.Is(err, nodes.ErrSnapshotEgressMismatch):
		return api.CompareProbeSnapshots409JSONResponse{ConflictJSONResponse: conflict(api.SnapshotEgressMismatch)}, nil
	case err != nil:
		return nil, err
	}
	return api.CompareProbeSnapshots200JSONResponse(probeSnapshotComparisonResponse(comparison)), nil
}

func (s apiServer) UpdateHistoryRetention(ctx context.Context, request api.UpdateHistoryRetentionRequestObject) (api.UpdateHistoryRetentionResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateHistoryRetention401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateHistoryRetention403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateHistoryRetention400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	state, err := s.nodes.UpdateHistoryRetention(ctx, nodes.HistoryRetentionUpdate{
		Mode: string(request.Body.Mode), MaxAgeDays: request.Body.MaxAgeDays,
		MaxLogicalBytes: request.Body.MaxLogicalBytes,
	})
	if errors.Is(err, nodes.ErrInvalidHistoryRetention) {
		return api.UpdateHistoryRetention400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateHistoryRetention200JSONResponse(historyStateResponse(state)), nil
}

func (s apiServer) CleanupHistory(ctx context.Context, request api.CleanupHistoryRequestObject) (api.CleanupHistoryResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CleanupHistory401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CleanupHistory403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	result, err := s.nodes.CleanupHistory(ctx)
	if err != nil {
		return nil, err
	}
	return api.CleanupHistory200JSONResponse(historyCleanupResponse(result)), nil
}

func (s apiServer) GetNetworkObservationSettings(ctx context.Context, _ api.GetNetworkObservationSettingsRequestObject) (api.GetNetworkObservationSettingsResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.GetNetworkObservationSettings401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	settings, err := s.nodes.ObservationSettings(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetNetworkObservationSettings200JSONResponse(networkObservationSettingsResponse(settings)), nil
}

func (s apiServer) UpdateNetworkObservationSettings(ctx context.Context, request api.UpdateNetworkObservationSettingsRequestObject) (api.UpdateNetworkObservationSettingsResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNetworkObservationSettings401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNetworkObservationSettings403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNetworkObservationSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	settings, err := s.nodes.UpdateObservationSettings(ctx, nodes.DiscoveryServices{
		IPv4: request.Body.Ipv4Services, IPv6: request.Body.Ipv6Services,
	})
	if errors.Is(err, nodes.ErrInvalidObservationSettings) {
		return api.UpdateNetworkObservationSettings400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidObservationSettings)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateNetworkObservationSettings200JSONResponse(networkObservationSettingsResponse(settings)), nil
}

func (s apiServer) UpdatePublicAddress(ctx context.Context, request api.UpdatePublicAddressRequestObject) (api.UpdatePublicAddressResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdatePublicAddress401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdatePublicAddress403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdatePublicAddress400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	address, err := s.nodes.UpdatePublicAddress(ctx, request.NodeId, request.PublicAddressId, nodes.PublicAddressUpdate{
		ProbeEnabled: request.Body.ProbeEnabled,
	})
	if errors.Is(err, nodes.ErrNodeNotFound) || errors.Is(err, nodes.ErrPublicAddressNotFound) {
		return api.UpdatePublicAddress404JSONResponse{NotFoundJSONResponse: notFound(api.EgressNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdatePublicAddress200JSONResponse(publicAddressResponse(address)), nil
}

func (s apiServer) ListNodeNetworkProxies(ctx context.Context, request api.ListNodeNetworkProxiesRequestObject) (api.ListNodeNetworkProxiesResponseObject, error) {
	_, failure, err := s.authorize(ctx, false, "")
	if err != nil {
		return nil, err
	}
	if failure != "" {
		return api.ListNodeNetworkProxies401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	proxies, err := s.nodes.ListNetworkProxies(ctx, request.NodeId)
	if errors.Is(err, nodes.ErrNodeNotFound) {
		return api.ListNodeNetworkProxies404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]api.NetworkProxy, 0, len(proxies))
	for _, proxy := range proxies {
		items = append(items, networkProxyResponse(proxy))
	}
	return api.ListNodeNetworkProxies200JSONResponse{Items: items}, nil
}

func (s apiServer) CreateNodeNetworkProxy(ctx context.Context, request api.CreateNodeNetworkProxyRequestObject) (api.CreateNodeNetworkProxyResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.CreateNodeNetworkProxy401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.CreateNodeNetworkProxy403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.CreateNodeNetworkProxy400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	proxy, err := s.nodes.CreateNetworkProxy(ctx, request.NodeId, nodes.NetworkProxyCreate{
		Name: request.Body.Name, Scheme: string(request.Body.Scheme), Host: request.Body.Host,
		Port: request.Body.Port, Username: request.Body.Username, Password: request.Body.Password,
	})
	switch {
	case errors.Is(err, nodes.ErrInvalidNetworkProxy):
		return api.CreateNodeNetworkProxy400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNetworkProxy)}, nil
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.CreateNodeNetworkProxy404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyAlreadyExists):
		return api.CreateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NetworkProxyAlreadyExists)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyLimitReached):
		return api.CreateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NetworkProxyLimitReached)}, nil
	case errors.Is(err, nodes.ErrEgressLimitReached):
		return api.CreateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.EgressLimitReached)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.CreateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.CreateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.CreateNodeNetworkProxy201JSONResponse(networkProxyResponse(proxy)), nil
}

func (s apiServer) UpdateNodeNetworkProxy(ctx context.Context, request api.UpdateNodeNetworkProxyRequestObject) (api.UpdateNodeNetworkProxyResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.UpdateNodeNetworkProxy401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.UpdateNodeNetworkProxy403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	if request.Body == nil {
		return api.UpdateNodeNetworkProxy400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	proxy, err := s.nodes.UpdateNetworkProxy(ctx, request.NodeId, request.ProxyId, nodes.NetworkProxyUpdate{
		Name: request.Body.Name, Scheme: string(request.Body.Scheme), Host: request.Body.Host,
		Port: request.Body.Port, Username: request.Body.Username,
		PasswordAction: string(request.Body.PasswordAction), Password: request.Body.Password,
	})
	switch {
	case errors.Is(err, nodes.ErrInvalidNetworkProxy):
		return api.UpdateNodeNetworkProxy400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidNetworkProxy)}, nil
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.UpdateNodeNetworkProxy404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyNotFound):
		return api.UpdateNodeNetworkProxy404JSONResponse{NotFoundJSONResponse: notFound(api.NetworkProxyNotFound)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyAlreadyExists):
		return api.UpdateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NetworkProxyAlreadyExists)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyDeletionPending):
		return api.UpdateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NetworkProxyDeletionPending)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.UpdateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.UpdateNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateNodeNetworkProxy200JSONResponse(networkProxyResponse(proxy)), nil
}

func (s apiServer) DeleteNodeNetworkProxy(ctx context.Context, request api.DeleteNodeNetworkProxyRequestObject) (api.DeleteNodeNetworkProxyResponseObject, error) {
	_, failure, err := s.authorize(ctx, true, csrfValue(request.Params.XCSRFToken))
	if err != nil {
		return nil, err
	}
	if failure == api.Unauthenticated {
		return api.DeleteNodeNetworkProxy401JSONResponse{UnauthorizedJSONResponse: unauthorized(failure)}, nil
	}
	if failure != "" {
		return api.DeleteNodeNetworkProxy403JSONResponse{ForbiddenJSONResponse: forbidden(failure)}, nil
	}
	proxy, err := s.nodes.DeleteNetworkProxy(ctx, request.NodeId, request.ProxyId)
	switch {
	case errors.Is(err, nodes.ErrNodeNotFound):
		return api.DeleteNodeNetworkProxy404JSONResponse{NotFoundJSONResponse: notFound(api.NodeNotFound)}, nil
	case errors.Is(err, nodes.ErrNetworkProxyNotFound):
		return api.DeleteNodeNetworkProxy404JSONResponse{NotFoundJSONResponse: notFound(api.NetworkProxyNotFound)}, nil
	case errors.Is(err, nodes.ErrNodeRevoked):
		return api.DeleteNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeRevoked)}, nil
	case errors.Is(err, nodes.ErrNodeDeletionPending):
		return api.DeleteNodeNetworkProxy409JSONResponse{ConflictJSONResponse: conflict(api.NodeDeletionPending)}, nil
	case err != nil:
		return nil, err
	}
	return api.DeleteNodeNetworkProxy202JSONResponse(networkProxyResponse(proxy)), nil
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
	return api.GetAgentEnrollment200JSONResponse(enrollmentResponse(enrollment)), nil
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
	return api.UpdateAgentEnrollment200JSONResponse(enrollmentResponse(enrollment)), nil
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
	if request.Body == nil {
		return api.RotateAgentEnrollmentKey400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	enrollment, err := s.nodes.RotateEnrollmentKey(ctx, request.Body.DefaultProbeTimezone)
	if errors.Is(err, nodes.ErrEnrollmentTimezone) {
		return api.RotateAgentEnrollmentKey400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.RotateAgentEnrollmentKey200JSONResponse(enrollmentResponse(enrollment)), nil
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
	upload := addressUploadFromAPI(request.Body.AddressStates, request.Body.AddressEvents, request.Body.AddressGaps)
	upload.ProbeStatus = probeStatusFromAPI(request.Body.ProbeStatus)
	upload.TaskReport = taskReportFromAPI(request.Body.TaskReport)
	poll, err := s.nodes.Poll(
		ctx, credential, metadataFromAPI(request.Body.Metadata),
		request.Body.AppliedConfigurationRevision, request.Body.ConfigurationError,
		request.Body.ConfigurationErrorRevision, networkInventoryFromAPI(request.Body.NetworkInventory),
		request.Body.NetworkInventoryError, upload,
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
		AddressUploadReceipt: addressUploadReceiptResponse(poll.AddressUploadReceipt),
	}
	if poll.SyncSession != nil {
		result.SyncSession = &api.AgentSyncSession{
			Id: poll.SyncSession.ID, ExpiresAt: poll.SyncSession.ExpiresAt,
			WebsocketPath: "/api/v1/agent/sync/" + poll.SyncSession.ID.String(),
		}
	}
	if poll.Task != nil {
		result.Task = &api.AgentTask{
			Id: poll.Task.ID, Kind: api.AgentTaskKind(poll.Task.Kind),
			Trigger:   api.AgentTaskTrigger(poll.Task.Trigger),
			CreatedAt: poll.Task.CreatedAt, ExpiresAt: poll.Task.ExpiresAt,
			TargetVersion: poll.Task.TargetVersion,
		}
		if poll.Task.Kind == "complete-probe" {
			publicAddressIDs := append([]uuid.UUID(nil), poll.Task.PublicAddressIDs...)
			result.Task.PublicAddressIds = &publicAddressIDs
		}
	}
	result.AcceptedTerminalTaskId = poll.AcceptedTerminalTaskID
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
	discoveryPaths := make([]api.AgentDiscoveryPath, 0, len(configuration.DiscoveryPaths))
	for _, egress := range configuration.DiscoveryPaths {
		discoveryPaths = append(discoveryPaths, api.AgentDiscoveryPath{
			Id: egress.ID, Kind: api.NetworkEgressKind(egress.Kind), Family: api.AddressFamily(egress.Family),
			InterfaceName: egress.InterfaceName, SourceAddress: egress.SourceAddress, ProxyId: egress.ProxyID,
			LightweightIntervalSeconds: egress.LightweightIntervalSeconds,
		})
	}
	probeTargets := make([]api.AgentProbeTarget, 0, len(configuration.ProbeTargets))
	for _, target := range configuration.ProbeTargets {
		if target.PathID == nil || target.PublicAddress == nil {
			return nil, errors.New("public-address probe target is incomplete")
		}
		probeTargets = append(probeTargets, api.AgentProbeTarget{
			Id: target.ID, PathId: *target.PathID, PublicAddress: *target.PublicAddress,
			Kind: api.NetworkEgressKind(target.Kind), Family: api.AddressFamily(target.Family),
			InterfaceName: target.InterfaceName, SourceAddress: target.SourceAddress, ProxyId: target.ProxyID,
		})
	}
	proxies := make([]api.AgentProxyConfiguration, 0, len(configuration.Proxies))
	for _, proxy := range configuration.Proxies {
		proxies = append(proxies, api.AgentProxyConfiguration{
			Id: proxy.ID, Scheme: api.NetworkProxyScheme(proxy.Scheme), Host: proxy.Host,
			Port: proxy.Port, Username: proxy.Username, Password: proxy.Password,
		})
	}
	return api.GetAgentConfiguration200JSONResponse{
		SchemaVersion: api.AgentConfigurationSnapshotSchemaVersion(configuration.SchemaVersion),
		Revision:      configuration.Revision, Enabled: configuration.Enabled,
		HistoryGeneration: configuration.HistoryGeneration,
		DiscoveryPaths:    discoveryPaths, ProbeTargets: probeTargets, Proxies: proxies,
		ProbeSchedule: api.ProbeSchedule{
			Enabled: configuration.ProbeSchedule.Enabled,
			Cron:    configuration.ProbeSchedule.Cron, Timezone: configuration.ProbeSchedule.Timezone,
		},
		ProbeLowMemoryOverride: configuration.ProbeLowMemoryOverride,
		DiscoveryServices: api.NetworkObservationSettingsUpdate{
			Ipv4Services: configuration.DiscoveryServices.IPv4,
			Ipv6Services: configuration.DiscoveryServices.IPv6,
		},
	}, nil
}

func (s apiServer) UploadProbeArtifact(ctx context.Context, request api.UploadProbeArtifactRequestObject) (api.UploadProbeArtifactResponseObject, error) {
	if request.Body == nil {
		return api.UploadProbeArtifact400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	}
	credential := bearerToken(requestSecurityFromContext(ctx).Authorization)
	receipt, err := s.nodes.UploadProbeArtifact(ctx, credential, probeArtifactFromAPI(*request.Body))
	switch {
	case errors.Is(err, nodes.ErrInvalidProbeArtifact):
		return api.UploadProbeArtifact400JSONResponse{BadRequestJSONResponse: badRequest(api.InvalidRequest)}, nil
	case errors.Is(err, nodes.ErrAgentUnauthenticated):
		return api.UploadProbeArtifact401JSONResponse{AgentUnauthorizedJSONResponse: agentUnauthorized(api.AgentUnauthenticated)}, nil
	case errors.Is(err, nodes.ErrAgentRevoked):
		return api.UploadProbeArtifact403JSONResponse{AgentForbiddenJSONResponse: agentForbidden(api.AgentRevoked)}, nil
	case err != nil:
		return nil, err
	}
	return api.UploadProbeArtifact200JSONResponse{
		ArtifactId: receipt.ID, Revision: receipt.Revision,
		Disposition: api.AgentProbeArtifactDisposition(receipt.Disposition),
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
		Hostname: metadata.Hostname, AgentVersion: metadata.AgentVersion, AgentRevision: metadata.SourceRevision,
		OperatingSystem: string(metadata.OperatingSystem), Architecture: string(metadata.Architecture),
		Capabilities: metadata.Capabilities, PhysicalMemoryBytes: metadata.PhysicalMemoryBytes,
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

func enrollmentResponse(enrollment nodes.Enrollment) api.AgentEnrollmentSettings {
	response := api.AgentEnrollmentSettings{Enabled: enrollment.Enabled, HasKey: enrollment.HasKey}
	if !enrollment.HasKey {
		return response
	}
	response.RegistrationKey = &enrollment.Key
	response.DefaultProbeTimezone = &enrollment.DefaultProbeTimezone
	rotatedAt := enrollment.RotatedAt
	response.RotatedAt = &rotatedAt
	return response
}

func systemSettingsResponse(settings systemsettings.Settings, requestOrigin string) api.SystemSettings {
	effectiveOrigin := settings.ExternalOrigin
	if effectiveOrigin == "" {
		effectiveOrigin = requestOrigin
	}
	return api.SystemSettings{
		Automatic: settings.ExternalOrigin == "", ExternalOrigin: settings.ExternalOrigin,
		EffectiveOrigin: effectiveOrigin,
	}
}

func nodeResponse(node nodes.Node) api.Node {
	response := api.Node{
		Id: node.ID, Name: node.Name, Hostname: node.Hostname, Status: api.NodeStatus(node.Status),
		Enabled: node.Enabled, AgentVersion: node.AgentVersion, SourceRevision: node.AgentRevision,
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
		PublicAddresses: make([]api.NodePublicAddressSummary, 0, len(node.PublicAddresses)),
	}
	for _, address := range node.PublicAddresses {
		response.PublicAddresses = append(response.PublicAddresses, api.NodePublicAddressSummary{
			Id: address.ID, Address: address.Address, Family: api.AddressFamily(address.Family),
			Available: address.Available, ProbeEnabled: address.ProbeEnabled,
		})
	}
	return response
}

func probeStateResponse(state nodes.ProbeState) api.NodeProbeState {
	response := api.NodeProbeState{
		NodeId: state.NodeID,
		Schedule: api.ProbeSchedule{
			Enabled: state.Schedule.Enabled, Cron: state.Schedule.Cron, Timezone: state.Schedule.Timezone,
		},
		LowMemoryOverride: state.LowMemoryOverride, ProbeOnNewAddress: state.ProbeOnNewAddress,
		PhysicalMemoryBytes: state.PhysicalMemoryBytes,
		PausedLowMemory:     state.PausedLowMemory,
		RecentRuns:          make([]api.ProbeRunSummary, 0, len(state.RecentRuns)),
	}
	if state.AgentStatus != nil {
		response.AgentStatus = probeStatusResponse(*state.AgentStatus)
	}
	if state.Task != nil {
		task := probeTaskResponse(*state.Task)
		response.Task = &task
	}
	for _, run := range state.RecentRuns {
		response.RecentRuns = append(response.RecentRuns, api.ProbeRunSummary{
			Id: run.ID, NodeId: run.NodeID, Trigger: api.ProbeTrigger(run.Trigger),
			StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Status: api.ProbeRunStatus(run.Status),
			ExpectedExecutions: int(run.ExpectedExecutions), CompletedExecutions: int(run.CompletedExecutions),
		})
	}
	return response
}

func probeStatusResponse(status nodes.ProbeStatus) *api.AgentProbeStatus {
	response := &api.AgentProbeStatus{
		ActiveRunId: status.ActiveRunID, NextScheduledAt: status.NextScheduledAt,
		LastOccurrenceAt:       status.LastOccurrenceAt,
		HistoryResetGeneration: status.HistoryResetGeneration, HistoryResetAt: status.HistoryResetAt,
		HistoryResetDiscardedAddressItems: &status.HistoryResetDiscardedAddressItems,
		HistoryResetDiscardedProbeItems:   &status.HistoryResetDiscardedProbeItems,
	}
	if status.LastOccurrenceTrigger != nil {
		value := api.ProbeTrigger(*status.LastOccurrenceTrigger)
		response.LastOccurrenceTrigger = &value
	}
	if status.LastOccurrenceStatus != nil {
		value := api.AgentProbeOccurrenceStatus(*status.LastOccurrenceStatus)
		response.LastOccurrenceStatus = &value
	}
	if status.LastSkipReason != nil {
		value := api.AgentProbeSkipReason(*status.LastSkipReason)
		response.LastSkipReason = &value
	}
	return response
}

func probeTaskResponse(task nodes.Task) api.ProbeTask {
	response := api.ProbeTask{
		Id: task.ID, NodeId: task.NodeID, Status: api.ProbeTaskStatus(task.Status),
		CreatedAt: task.CreatedAt, ExpiresAt: task.ExpiresAt,
		AcknowledgedAt: task.AcknowledgedAt, StartedAt: task.StartedAt, CompletedAt: task.CompletedAt,
		RunId: task.RunID, Offline: task.Offline,
	}
	if task.RejectionReason != nil {
		value := api.AgentProbeSkipReason(*task.RejectionReason)
		response.RejectionReason = &value
	}
	return response
}

func probeRunResponse(run nodes.ProbeRun) api.ProbeRun {
	response := api.ProbeRun{
		Id: run.ID, NodeId: run.NodeID, ConfigurationRevision: run.ConfigurationRevision,
		HistoryGeneration: run.HistoryGeneration, Trigger: api.ProbeTrigger(run.Trigger),
		TaskId:    run.TaskID,
		StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Status: api.ProbeRunStatus(run.Status),
		ExpectedExecutions: int(run.ExpectedExecutions), Executions: make([]api.ProbeExecution, 0, len(run.Executions)),
	}
	for _, execution := range run.Executions {
		item := api.ProbeExecution{
			Id: execution.ID, RunId: execution.RunID, EgressId: execution.EgressID,
			Ordinal: int(execution.Ordinal), Sequence: execution.Sequence,
			Status: api.ProbeExecutionStatus(execution.Status), StartedAt: execution.StartedAt,
			CompletedAt: execution.CompletedAt, Diagnostic: execution.Diagnostic, SnapshotId: execution.SnapshotID,
		}
		if execution.FailureStage != nil {
			value := api.ProbeFailureStage(*execution.FailureStage)
			item.FailureStage = &value
		}
		response.Executions = append(response.Executions, item)
	}
	return response
}

func probeSnapshotResponse(snapshot nodes.ProbeSnapshot) api.ProbeSnapshot {
	response := api.ProbeSnapshot{
		Id: snapshot.ID, ExecutionId: snapshot.ExecutionID, EgressId: snapshot.EgressID,
		Sequence: snapshot.Sequence, ObservedAt: snapshot.ObservedAt, RawResult: snapshot.RawResult,
		Starred: snapshot.Starred, Baseline: snapshot.Baseline,
		Fields:             make([]api.KnownProbeField, 0, len(snapshot.Fields)),
		FormatIssues:       make([]api.ProbeFormatIssue, 0, len(snapshot.FormatIssues)),
		Changes:            make([]api.ProbeFieldChange, 0, len(snapshot.Changes)),
		PreviousSnapshotId: snapshot.PreviousSnapshotID,
	}
	for _, field := range snapshot.Fields {
		response.Fields = append(response.Fields, knownProbeFieldResponse(field))
	}
	for _, issue := range snapshot.FormatIssues {
		response.FormatIssues = append(response.FormatIssues, probeFormatIssueResponse(issue))
	}
	for _, change := range snapshot.Changes {
		response.Changes = append(response.Changes, probeFieldChangeResponse(change))
	}
	return response
}

func historyStateResponse(state nodes.HistoryState) api.HistoryState {
	return api.HistoryState{
		Generation: state.Generation, ResetAt: state.ResetAt,
		Retention: historyRetentionResponse(state.Retention), Usage: historyUsageResponse(state.Usage),
	}
}

func knownProbeFieldResponse(field centerhistory.FieldValue) api.KnownProbeField {
	response := api.KnownProbeField{
		Id: field.ID, Group: field.Group, Path: field.Path,
		ExpectedTypes: probeJSONTypes(field.ExpectedTypes),
		Status:        api.KnownProbeFieldStatus(field.Status), Value: field.Value,
	}
	if field.ActualType != nil {
		value := api.ProbeJSONType(*field.ActualType)
		response.ActualType = &value
	}
	return response
}

func probeFormatIssueResponse(issue centerhistory.FormatIssue) api.ProbeFormatIssue {
	response := api.ProbeFormatIssue{
		Path: issue.Path, Kind: api.ProbeFormatIssueKind(issue.Kind),
		ExpectedTypes: probeJSONTypes(issue.ExpectedTypes),
	}
	if issue.ActualType != nil {
		value := api.ProbeJSONType(*issue.ActualType)
		response.ActualType = &value
	}
	return response
}

func probeFieldChangeResponse(change centerhistory.FieldChange) api.ProbeFieldChange {
	return api.ProbeFieldChange{
		FieldId: change.FieldID, Group: change.Group, Path: change.Path,
		ValueType: api.ProbeJSONType(change.ValueType), Before: change.Before, After: change.After,
	}
}

func probeJSONTypes(values []centerhistory.JSONType) []api.ProbeJSONType {
	result := make([]api.ProbeJSONType, 0, len(values))
	for _, value := range values {
		result = append(result, api.ProbeJSONType(value))
	}
	return result
}

func historyRetentionResponse(settings nodes.HistoryRetentionSettings) api.HistoryRetentionSettings {
	return api.HistoryRetentionSettings{
		Mode: api.HistoryRetentionMode(settings.Mode), MaxAgeDays: settings.MaxAgeDays,
		MaxLogicalBytes: settings.MaxLogicalBytes, UpdatedAt: settings.UpdatedAt,
		LastCleanupAt: settings.LastCleanupAt, LastCleanupDeletedItems: settings.LastCleanupDeletedItems,
		LastCleanupError: settings.LastCleanupError,
	}
}

func historyUsageResponse(usage nodes.HistoryUsage) api.HistoryUsage {
	return api.HistoryUsage{
		LogicalBytes: usage.LogicalBytes, ProtectedLogicalBytes: usage.ProtectedLogicalBytes,
		RecordCount: usage.RecordCount, DatabaseBytes: usage.DatabaseBytes,
		WalBytes: usage.WALBytes, SharedMemoryBytes: usage.SharedMemoryBytes,
		OverBudget: usage.OverBudget, OverageBytes: usage.OverageBytes,
	}
}

func historyCleanupResponse(result nodes.HistoryCleanupResult) api.HistoryCleanupResult {
	return api.HistoryCleanupResult{
		DeletedItems: result.DeletedItems, CompletedAt: result.CompletedAt,
		Usage: historyUsageResponse(result.Usage),
	}
}

func historyFilter(
	nodeID *api.HistoryNodeFilter,
	egressID *api.HistoryEgressFilter,
	from *api.HistoryFrom,
	to *api.HistoryTo,
	page *api.HistoryPage,
	pageSize *api.HistoryPageSize,
) nodes.HistoryFilter {
	result := nodes.HistoryFilter{
		NodeID: nodeID, EgressID: egressID, From: from, To: to, Page: 1, PageSize: 50,
	}
	if page != nil {
		result.Page = *page
	}
	if pageSize != nil {
		result.PageSize = *pageSize
	}
	return result
}

func historyOwnerResponse(owner nodes.HistoryOwner) api.HistoryOwner {
	return api.HistoryOwner{NodeName: owner.NodeName, EgressName: owner.EgressName}
}

func probeSnapshotHistoryPageResponse(page nodes.ProbeSnapshotPage) api.ProbeSnapshotHistoryPage {
	response := api.ProbeSnapshotHistoryPage{
		Items: make([]api.ProbeSnapshotSummary, 0, len(page.Items)), Total: page.Total,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, api.ProbeSnapshotSummary{
			Id: item.ID, ExecutionId: item.ExecutionID, RunId: item.RunID,
			NodeId: item.NodeID, EgressId: item.EgressID, Owner: historyOwnerResponse(item.Owner),
			Sequence: item.Sequence, Trigger: api.ProbeTrigger(item.Trigger), RunStatus: api.ProbeRunStatus(item.RunStatus),
			ObservedAt: item.ObservedAt, ReceivedAt: item.ReceivedAt, EncodedSize: item.EncodedSize,
			Starred: item.Starred, Current: item.Current, Processed: item.Processed,
			Baseline: item.Baseline, ChangeCount: item.ChangeCount,
			FormatStatus:       api.ProbeFormatStatus(item.FormatStatus),
			FormatIssueCount:   item.FormatIssueCount,
			PreviousSnapshotId: item.PreviousSnapshotID,
		})
	}
	return response
}

func addressHistoryPageResponse(page nodes.AddressHistoryPage) api.AddressHistoryPage {
	response := api.AddressHistoryPage{
		Events: make([]api.HistoryAddressEvent, 0, len(page.Events)),
		Gaps:   make([]api.HistoryAddressGap, 0, len(page.Gaps)), Total: page.Total, GapTotal: page.GapTotal,
	}
	for _, item := range page.Events {
		response.Events = append(response.Events, api.HistoryAddressEvent{
			NodeId: item.NodeID, Owner: historyOwnerResponse(item.Owner), Event: publicAddressEventResponse(item.Event),
		})
	}
	for _, item := range page.Gaps {
		response.Gaps = append(response.Gaps, api.HistoryAddressGap{
			NodeId: item.NodeID, Owner: historyOwnerResponse(item.Owner), Gap: publicAddressGapResponse(item.Gap),
		})
	}
	return response
}

func probeHistoryGapPageResponse(page nodes.ProbeHistoryGapPage) api.ProbeHistoryGapPage {
	response := api.ProbeHistoryGapPage{
		Items: make([]api.ProbeHistoryGap, 0, len(page.Items)), Total: page.Total,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, probeHistoryGapResponse(item))
	}
	return response
}

func probeHistoryGapResponse(item nodes.ProbeHistoryGap) api.ProbeHistoryGap {
	return api.ProbeHistoryGap{
		Id: item.ID, NodeId: item.NodeID, EgressId: item.EgressID, Owner: historyOwnerResponse(item.Owner),
		DroppedCount: item.DroppedCount, FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
		FirstObservedAt: item.FirstObservedAt, LastObservedAt: item.LastObservedAt,
	}
}

func probeFormatEventResponse(item nodes.FormatEvent) api.ProbeFormatEvent {
	response := api.ProbeFormatEvent{
		Id: item.ID, NodeId: item.NodeID, EgressId: item.EgressID,
		ExecutionId: item.ExecutionID, SnapshotId: item.SnapshotID,
		Owner: historyOwnerResponse(item.Owner), Sequence: item.Sequence,
		Kind:       api.ProbeFormatEventKind(item.Kind),
		Issues:     make([]api.ProbeFormatIssue, 0, len(item.Issues)),
		ObservedAt: item.ObservedAt, RecordedAt: item.RecordedAt,
	}
	for _, issue := range item.Issues {
		response.Issues = append(response.Issues, probeFormatIssueResponse(issue))
	}
	return response
}

func probeFormatEventPageResponse(page nodes.FormatEventPage) api.ProbeFormatEventPage {
	response := api.ProbeFormatEventPage{
		Items: make([]api.ProbeFormatEvent, 0, len(page.Items)), Total: page.Total,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, probeFormatEventResponse(item))
	}
	return response
}

func probeSnapshotComparisonResponse(comparison nodes.ProbeSnapshotComparison) api.ProbeSnapshotComparison {
	response := api.ProbeSnapshotComparison{
		BeforeId: comparison.BeforeID, AfterId: comparison.AfterID, EgressId: comparison.EgressID,
		Fields: make([]api.ComparedProbeField, 0, len(comparison.Fields)),
	}
	for _, field := range comparison.Fields {
		response.Fields = append(response.Fields, api.ComparedProbeField{
			Id: field.ID, Group: field.Group, Path: field.Path,
			ExpectedTypes: probeJSONTypes(field.ExpectedTypes),
			Before:        knownProbeFieldResponse(field.Before), After: knownProbeFieldResponse(field.After),
			Changed: field.Changed,
		})
	}
	return response
}

func probeStatusFromAPI(status *api.AgentProbeStatus) *nodes.ProbeStatus {
	if status == nil {
		return nil
	}
	result := &nodes.ProbeStatus{
		ActiveRunID: status.ActiveRunId, NextScheduledAt: status.NextScheduledAt,
		LastOccurrenceAt:       status.LastOccurrenceAt,
		HistoryResetGeneration: status.HistoryResetGeneration, HistoryResetAt: status.HistoryResetAt,
	}
	if status.HistoryResetDiscardedAddressItems != nil {
		result.HistoryResetDiscardedAddressItems = *status.HistoryResetDiscardedAddressItems
	}
	if status.HistoryResetDiscardedProbeItems != nil {
		result.HistoryResetDiscardedProbeItems = *status.HistoryResetDiscardedProbeItems
	}
	if status.LastOccurrenceTrigger != nil {
		value := string(*status.LastOccurrenceTrigger)
		result.LastOccurrenceTrigger = &value
	}
	if status.LastOccurrenceStatus != nil {
		value := string(*status.LastOccurrenceStatus)
		result.LastOccurrenceStatus = &value
	}
	if status.LastSkipReason != nil {
		value := string(*status.LastSkipReason)
		result.LastSkipReason = &value
	}
	return result
}

func taskReportFromAPI(report *api.AgentTaskReport) *nodes.TaskReport {
	if report == nil {
		return nil
	}
	result := &nodes.TaskReport{
		ID: report.Id, Status: string(report.Status), AcknowledgedAt: report.AcknowledgedAt,
		StartedAt: report.StartedAt, CompletedAt: report.CompletedAt, RunID: report.RunId,
		PreviousVersion: report.PreviousVersion, ResultVersion: report.ResultVersion,
		FailureCode: report.FailureCode, Diagnostic: report.Diagnostic,
	}
	if report.RejectionReason != nil {
		value := string(*report.RejectionReason)
		result.RejectionReason = &value
	}
	return result
}

func probeArtifactFromAPI(artifact api.AgentProbeArtifact) nodes.ProbeArtifact {
	result := nodes.ProbeArtifact{ID: artifact.ArtifactId, Revision: artifact.Revision}
	if artifact.Run != nil {
		run := artifact.Run
		result.Run = &nodes.ProbeRunArtifact{
			ID: run.Id, ConfigurationRevision: run.NodeConfigurationRevision,
			HistoryGeneration: run.HistoryGeneration, Trigger: string(run.Trigger), TaskID: run.TaskId,
			StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Status: string(run.Status),
			Executions: make([]nodes.ProbeExecutionManifest, 0, len(run.Executions)),
		}
		for _, execution := range run.Executions {
			result.Run.Executions = append(result.Run.Executions, nodes.ProbeExecutionManifest{
				ID: execution.Id, EgressID: execution.EgressId,
				Ordinal: int64(execution.Ordinal), Sequence: execution.Sequence,
			})
		}
	}
	if artifact.Execution != nil {
		execution := artifact.Execution
		result.Execution = &nodes.ProbeExecutionArtifact{
			ID: execution.Id, EgressID: execution.EgressId, Ordinal: int64(execution.Ordinal),
			Sequence: execution.Sequence, Status: string(execution.Status),
			StartedAt: execution.StartedAt, CompletedAt: execution.CompletedAt, Diagnostic: execution.Diagnostic,
		}
		if execution.FailureStage != nil {
			value := string(*execution.FailureStage)
			result.Execution.FailureStage = &value
		}
		if execution.RawResult != nil {
			result.Execution.RawResult = append([]byte(nil), (*execution.RawResult)...)
		}
	}
	if artifact.Gap != nil {
		gap := artifact.Gap
		result.Gap = &nodes.ProbeGapArtifact{
			ID: gap.Id, EgressID: gap.EgressId, HistoryGeneration: gap.HistoryGeneration,
			DroppedCount: gap.DroppedCount, FirstSequence: gap.FirstSequence, LastSequence: gap.LastSequence,
			FirstObservedAt: gap.FirstObservedAt, LastObservedAt: gap.LastObservedAt,
		}
	}
	return result
}

func networkStateResponse(state nodes.NodeNetworkState) api.NodeNetworkState {
	response := api.NodeNetworkState{
		NetworkProxies:  make([]api.NetworkProxy, 0, len(state.NetworkProxies)),
		AddressEvents:   make([]api.PublicAddressEvent, 0, len(state.AddressEvents)),
		AddressGaps:     make([]api.PublicAddressGap, 0, len(state.AddressGaps)),
		PublicAddresses: make([]api.PublicAddress, 0, len(state.PublicAddresses)),
	}
	for _, item := range state.NetworkProxies {
		response.NetworkProxies = append(response.NetworkProxies, networkProxyResponse(item))
	}
	for _, item := range state.PublicAddresses {
		response.PublicAddresses = append(response.PublicAddresses, publicAddressResponse(item))
	}
	for _, item := range state.AddressEvents {
		response.AddressEvents = append(response.AddressEvents, publicAddressEventResponse(item))
	}
	for _, item := range state.AddressGaps {
		response.AddressGaps = append(response.AddressGaps, publicAddressGapResponse(item))
	}
	return response
}

func publicAddressResponse(address nodes.PublicAddress) api.PublicAddress {
	return api.PublicAddress{
		Id: address.ID, Address: address.Address, Family: api.AddressFamily(address.Family),
		ProbeEnabled: address.ProbeEnabled,
		Available:    address.Available, SelectedNodeId: address.SelectedNodeID,
		SelectedNodeName: address.SelectedNodeName, PathCount: address.PathCount,
		LikelyNat: address.LikelyNAT, ProxyPath: address.ProxyPath,
		FirstSeenAt: address.FirstSeenAt, LastSeenAt: address.LastSeenAt,
		LastCheckedAt: address.SelectedLastChecked, LastSucceededAt: address.SelectedLastSucceeded,
		LatestSnapshotId: address.LatestSnapshotID, LatestSnapshotAt: address.LatestSnapshotAt,
	}
}

func addressUploadFromAPI(states *[]api.AgentAddressState, events *[]api.AgentAddressEvent, gaps *[]api.AgentAddressGap) nodes.AddressUpload {
	result := nodes.AddressUpload{}
	if states != nil {
		result.States = make([]nodes.AddressState, 0, len(*states))
		for _, item := range *states {
			var failureReason *string
			if item.FailureReason != nil {
				value := string(*item.FailureReason)
				failureReason = &value
			}
			result.States = append(result.States, nodes.AddressState{
				EgressID: item.EgressId, HistoryGeneration: item.HistoryGeneration,
				Family: string(item.Family), Status: string(item.Status), Sequence: item.Sequence,
				PublicAddress: item.PublicAddress, LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
				ProxyPath: item.ProxyPath, LikelyNAT: item.LikelyNat, Temporary: item.Temporary,
				FailureReason: failureReason, LastCheckedAt: item.LastCheckedAt,
				LastSucceededAt: item.LastSucceededAt, LastChangedAt: item.LastChangedAt,
			})
		}
	}
	if events != nil {
		result.Events = make([]nodes.AddressEvent, 0, len(*events))
		for _, item := range *events {
			var failureReason *string
			if item.FailureReason != nil {
				value := string(*item.FailureReason)
				failureReason = &value
			}
			result.Events = append(result.Events, nodes.AddressEvent{
				ID: item.Id, EgressID: item.EgressId, HistoryGeneration: item.HistoryGeneration,
				Sequence: item.Sequence, Kind: string(item.Kind), Family: string(item.Family),
				PublicAddress:  item.PublicAddress,
				LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
				ProxyPath: item.ProxyPath, LikelyNAT: item.LikelyNat, Temporary: item.Temporary,
				FailureReason: failureReason, ObservedAt: item.ObservedAt,
			})
		}
	}
	if gaps != nil {
		result.Gaps = make([]nodes.AddressGap, 0, len(*gaps))
		for _, item := range *gaps {
			result.Gaps = append(result.Gaps, nodes.AddressGap{
				ID: item.Id, EgressID: item.EgressId, HistoryGeneration: item.HistoryGeneration,
				DroppedCount: item.DroppedCount, FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
				FirstObservedAt: item.FirstObservedAt, LastObservedAt: item.LastObservedAt,
			})
		}
	}
	return result
}

func addressUploadReceiptResponse(receipt nodes.AddressUploadReceipt) api.AgentAddressUploadReceipt {
	result := api.AgentAddressUploadReceipt{
		AcceptedEventIds:  make([]uuid.UUID, 0, len(receipt.AcceptedEventIDs)),
		DiscardedEventIds: make([]uuid.UUID, 0, len(receipt.DiscardedEventIDs)),
		AcceptedGaps:      make([]api.AgentAddressGapReceipt, 0, len(receipt.AcceptedGaps)),
		DiscardedGaps:     make([]api.AgentAddressGapReceipt, 0, len(receipt.DiscardedGaps)),
	}
	result.AcceptedEventIds = append(result.AcceptedEventIds, receipt.AcceptedEventIDs...)
	result.DiscardedEventIds = append(result.DiscardedEventIds, receipt.DiscardedEventIDs...)
	for _, item := range receipt.AcceptedGaps {
		result.AcceptedGaps = append(result.AcceptedGaps, api.AgentAddressGapReceipt{Id: item.ID, LastSequence: item.LastSequence})
	}
	for _, item := range receipt.DiscardedGaps {
		result.DiscardedGaps = append(result.DiscardedGaps, api.AgentAddressGapReceipt{Id: item.ID, LastSequence: item.LastSequence})
	}
	return result
}

func addressStateResponse(item nodes.AddressState) api.AgentAddressState {
	var failureReason *api.AddressFailureReason
	if item.FailureReason != nil {
		value := api.AddressFailureReason(*item.FailureReason)
		failureReason = &value
	}
	return api.AgentAddressState{
		EgressId: item.EgressID, HistoryGeneration: item.HistoryGeneration,
		Family: api.AddressFamily(item.Family), Status: api.AddressObservationStatus(item.Status),
		Sequence: item.Sequence, PublicAddress: item.PublicAddress,
		LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
		ProxyPath: item.ProxyPath, LikelyNat: item.LikelyNAT, Temporary: item.Temporary,
		FailureReason: failureReason, LastCheckedAt: item.LastCheckedAt,
		LastSucceededAt: item.LastSucceededAt, LastChangedAt: item.LastChangedAt,
	}
}

func addressEventResponse(item nodes.AddressEvent) api.AgentAddressEvent {
	var failureReason *api.AddressFailureReason
	if item.FailureReason != nil {
		value := api.AddressFailureReason(*item.FailureReason)
		failureReason = &value
	}
	return api.AgentAddressEvent{
		Id: item.ID, EgressId: item.EgressID, HistoryGeneration: item.HistoryGeneration,
		Sequence: item.Sequence, Kind: api.AddressEventKind(item.Kind), Family: api.AddressFamily(item.Family),
		PublicAddress:  item.PublicAddress,
		LocalInterface: item.LocalInterface, LocalAddress: item.LocalAddress,
		ProxyPath: item.ProxyPath, LikelyNat: item.LikelyNAT, Temporary: item.Temporary,
		FailureReason: failureReason, ObservedAt: item.ObservedAt,
	}
}

func addressGapResponse(item nodes.AddressGap) api.AgentAddressGap {
	return api.AgentAddressGap{
		Id: item.ID, EgressId: item.EgressID, HistoryGeneration: item.HistoryGeneration,
		DroppedCount: item.DroppedCount, FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
		FirstObservedAt: item.FirstObservedAt, LastObservedAt: item.LastObservedAt,
	}
}

func networkObservationSettingsResponse(settings nodes.DiscoveryServices) api.NetworkObservationSettings {
	return api.NetworkObservationSettings{
		Ipv4Services: settings.IPv4, Ipv6Services: settings.IPv6, UpdatedAt: settings.UpdatedAt,
	}
}

func publicAddressEventResponse(item nodes.AddressEvent) api.PublicAddressEvent {
	var failureReason *api.AddressFailureReason
	if item.FailureReason != nil {
		value := api.AddressFailureReason(*item.FailureReason)
		failureReason = &value
	}
	return api.PublicAddressEvent{
		Id: item.ID, Sequence: item.Sequence, Kind: api.AddressEventKind(item.Kind), Family: api.AddressFamily(item.Family),
		PublicAddress: item.PublicAddress,
		FailureReason: failureReason, ObservedAt: item.ObservedAt,
	}
}

func publicAddressGapResponse(item nodes.AddressGap) api.PublicAddressGap {
	return api.PublicAddressGap{
		Id: item.ID, DroppedCount: item.DroppedCount,
		FirstSequence: item.FirstSequence, LastSequence: item.LastSequence,
		FirstObservedAt: item.FirstObservedAt, LastObservedAt: item.LastObservedAt,
	}
}

func networkProxyResponse(proxy nodes.NetworkProxy) api.NetworkProxy {
	var deletionStatus *api.NetworkProxyDeletionStatus
	if proxy.DeletionStatus != nil {
		value := api.NetworkProxyDeletionStatus(*proxy.DeletionStatus)
		deletionStatus = &value
	}
	return api.NetworkProxy{
		Id: proxy.ID, NodeId: proxy.NodeID, Name: proxy.Name, Scheme: api.NetworkProxyScheme(proxy.Scheme),
		Host: proxy.Host, Port: proxy.Port, Username: proxy.Username,
		PasswordConfigured: proxy.PasswordConfigured, Status: api.NetworkProxyStatus(proxy.Status),
		Ipv4: networkProxyFamilyResponse(proxy.IPv4), Ipv6: networkProxyFamilyResponse(proxy.IPv6),
		DeletionStatus: deletionStatus, DeletionError: proxy.DeletionError,
		CreatedAt: proxy.CreatedAt, UpdatedAt: proxy.UpdatedAt,
	}
}

func networkProxyFamilyResponse(result nodes.NetworkProxyFamilyResult) api.NetworkProxyFamilyResult {
	var failureReason *api.AddressFailureReason
	if result.FailureReason != nil {
		value := api.AddressFailureReason(*result.FailureReason)
		failureReason = &value
	}
	return api.NetworkProxyFamilyResult{
		Status: api.NetworkProxyFamilyStatus(result.Status), PublicAddress: result.PublicAddress,
		FailureReason: failureReason, LastCheckedAt: result.LastCheckedAt,
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
