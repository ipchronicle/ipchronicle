package center

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type apiServer struct {
	version                  string
	administrator            *admin.Service
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
