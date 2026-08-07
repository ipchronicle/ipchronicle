package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database/configdb"
)

const (
	sessionTokenLength = 32
	sessionLifetime    = 30 * 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTOTPRequired       = errors.New("TOTP code is required")
	ErrRateLimited        = errors.New("login is rate limited")
	ErrUnauthenticated    = errors.New("administrator session is not authenticated")
	ErrCurrentPassword    = errors.New("current password is invalid")
	ErrInvalidTOTP        = errors.New("TOTP code is invalid or has already been used")
	ErrTOTPAlreadyEnabled = errors.New("TOTP is already enabled")
	ErrTOTPNotEnabled     = errors.New("TOTP is not enabled")
	ErrTOTPEnrollment     = errors.New("TOTP enrollment has not been started")
	ErrNoAccountChange    = errors.New("no account change was requested")
)

type Service struct {
	database  *sql.DB
	queries   *configdb.Queries
	masterKey [32]byte
	hasher    *PasswordHasher
	throttle  *loginThrottle
	now       func() time.Time
}

func NewService(database *sql.DB, queries *configdb.Queries, masterKey [32]byte) *Service {
	if database == nil || queries == nil {
		panic("administrator service requires a configuration database")
	}
	return &Service{
		database:  database,
		queries:   queries,
		masterKey: masterKey,
		hasher:    NewPasswordHasher(),
		throttle:  newLoginThrottle(),
		now:       time.Now,
	}
}

func (s *Service) Bootstrap(ctx context.Context, username, password string) error {
	if err := ValidateUsername(username); err != nil {
		return fmt.Errorf("invalid bootstrap username: %w", err)
	}
	if err := ValidateBootstrapPassword(password); err != nil {
		return fmt.Errorf("invalid bootstrap password: %w", err)
	}
	if _, err := s.queries.GetAdministrator(ctx); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read administrator: %w", err)
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	now := s.now().UTC().Unix()
	if err := s.queries.CreateAdministrator(ctx, configdb.CreateAdministratorParams{
		Username:               username,
		PasswordHash:           hash,
		Locale:                 "en",
		UsesDefaultCredentials: defaultCredentialsValue(username, password),
		CreatedAt:              now,
		CredentialsUpdatedAt:   now,
	}); err != nil {
		return fmt.Errorf("create administrator: %w", err)
	}
	return nil
}

type Account struct {
	Username               string
	Locale                 string
	UsesDefaultCredentials bool
	TOTPEnabled            bool
}

func accountFromRecord(record configdb.Administrator) Account {
	return Account{
		Username:               record.Username,
		Locale:                 record.Locale,
		UsesDefaultCredentials: record.UsesDefaultCredentials == 1,
		TOTPEnabled:            record.TotpEnabled == 1,
	}
}

func (s *Service) Account(ctx context.Context) (Account, error) {
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return Account{}, err
	}
	return accountFromRecord(record), nil
}

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	Account   Account
}

type Principal struct {
	TokenDigest [sha256.Size]byte
	Token       string
	ExpiresAt   time.Time
	Account     Account
}

func (s *Service) Login(ctx context.Context, username, password, totpCode, clientAddress, userAgent string) (Session, time.Duration, error) {
	now := s.now().UTC()
	if wait := s.throttle.RetryAfter(username, clientAddress, now); wait > 0 {
		return Session{}, wait, ErrRateLimited
	}
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return Session{}, 0, fmt.Errorf("read administrator: %w", err)
	}
	passwordValid, needsRehash, err := s.hasher.Verify(ctx, password, record.PasswordHash)
	if err != nil {
		return Session{}, 0, fmt.Errorf("verify password: %w", err)
	}
	usernameValid := subtle.ConstantTimeCompare(
		[]byte(strings.ToLower(strings.TrimSpace(username))),
		[]byte(strings.ToLower(record.Username)),
	) == 1
	if !usernameValid || !passwordValid {
		s.throttle.Failed(username, clientAddress, now)
		return Session{}, 0, ErrInvalidCredentials
	}

	var usedTOTPStep *int64
	if record.TotpEnabled == 1 {
		if totpCode == "" {
			return Session{}, 0, ErrTOTPRequired
		}
		secret, err := decryptTOTPSecret(s.masterKey, record.TotpSecretEncrypted)
		if err != nil {
			return Session{}, 0, err
		}
		step, err := validateTOTPCode(secret, totpCode, now)
		if err != nil {
			s.throttle.Failed(username, clientAddress, now)
			return Session{}, 0, ErrInvalidCredentials
		}
		usedTOTPStep = &step
	}

	rawToken := make([]byte, sessionTokenLength)
	if _, err := rand.Read(rawToken); err != nil {
		return Session{}, 0, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	digest := sha256.Sum256(rawToken)
	expiresAt := now.Add(sessionLifetime)
	var replacementHash string
	if needsRehash {
		replacementHash, err = s.hasher.Hash(ctx, password)
		if err != nil {
			return Session{}, 0, err
		}
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, 0, err
	}
	queries := s.queries.WithTx(transaction)
	rollback := func(cause error) (Session, time.Duration, error) {
		return Session{}, 0, errors.Join(cause, transaction.Rollback())
	}
	if usedTOTPStep != nil {
		rows, err := queries.UseTOTPStep(ctx, configdb.UseTOTPStepParams{
			TotpLastUsedStep:   *usedTOTPStep,
			TotpLastUsedStep_2: *usedTOTPStep,
		})
		if err != nil {
			return rollback(err)
		}
		if rows != 1 {
			return rollback(ErrInvalidCredentials)
		}
	}
	if replacementHash != "" {
		if err := queries.RehashAdministratorPassword(ctx, replacementHash); err != nil {
			return rollback(err)
		}
	}
	if err := queries.CreateAdministratorSession(ctx, configdb.CreateAdministratorSessionParams{
		TokenDigest:   digest[:],
		CreatedAt:     now.Unix(),
		ExpiresAt:     expiresAt.Unix(),
		ClientAddress: clientAddress,
		UserAgent:     truncate(userAgent, 512),
	}); err != nil {
		return rollback(err)
	}
	if err := transaction.Commit(); err != nil {
		return Session{}, 0, err
	}
	s.throttle.Succeeded(username, clientAddress)
	_ = s.queries.DeleteExpiredAdministratorSessions(ctx, now.Unix())
	return Session{
		Token:     token,
		CSRFToken: s.CSRFToken(token),
		ExpiresAt: expiresAt,
		Account:   accountFromRecord(record),
	}, 0, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) != sessionTokenLength {
		return Principal{}, ErrUnauthenticated
	}
	digest := sha256.Sum256(rawToken)
	now := s.now().UTC()
	record, err := s.queries.GetAdministratorSession(ctx, configdb.GetAdministratorSessionParams{
		TokenDigest: digest[:],
		ExpiresAt:   now.Unix(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	account, err := s.Account(ctx)
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		TokenDigest: digest,
		Token:       token,
		ExpiresAt:   time.Unix(record.ExpiresAt, 0).UTC(),
		Account:     account,
	}, nil
}

func (s *Service) CSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, s.masterKey[:])
	_, _ = mac.Write([]byte("ipchronicle:csrf:v1:"))
	_, _ = mac.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) ValidateCSRF(sessionToken, supplied string) bool {
	expected := s.CSRFToken(sessionToken)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

func (s *Service) RevokeSession(ctx context.Context, digest [sha256.Size]byte) error {
	return s.queries.DeleteAdministratorSession(ctx, digest[:])
}

func (s *Service) RevokeAllSessions(ctx context.Context) error {
	return s.queries.DeleteAllAdministratorSessions(ctx)
}

type AccountUpdate struct {
	CurrentPassword string
	Username        *string
	NewPassword     *string
}

func (s *Service) UpdateAccount(ctx context.Context, update AccountUpdate) (Account, bool, error) {
	if update.Username == nil && update.NewPassword == nil {
		return Account{}, false, ErrNoAccountChange
	}
	if update.Username != nil {
		if err := ValidateUsername(*update.Username); err != nil {
			return Account{}, false, err
		}
	}
	if update.NewPassword != nil {
		if err := ValidateNewPassword(*update.NewPassword); err != nil {
			return Account{}, false, err
		}
	}
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return Account{}, false, err
	}
	valid, _, err := s.hasher.Verify(ctx, update.CurrentPassword, record.PasswordHash)
	if err != nil {
		return Account{}, false, err
	}
	if !valid {
		return Account{}, false, ErrCurrentPassword
	}
	updatedUsername := record.Username
	if update.Username != nil {
		updatedUsername = *update.Username
	}
	updatedPassword := update.CurrentPassword
	if update.NewPassword != nil {
		updatedPassword = *update.NewPassword
	}
	usesDefault := defaultCredentialsValue(updatedUsername, updatedPassword)
	var passwordHash string
	if update.NewPassword != nil {
		passwordHash, err = s.hasher.Hash(ctx, *update.NewPassword)
		if err != nil {
			return Account{}, false, err
		}
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, false, err
	}
	queries := s.queries.WithTx(transaction)
	rollback := func(cause error) (Account, bool, error) {
		return Account{}, false, errors.Join(cause, transaction.Rollback())
	}
	now := s.now().UTC().Unix()
	if update.Username != nil {
		if err := queries.UpdateAdministratorUsername(ctx, configdb.UpdateAdministratorUsernameParams{
			Username:               *update.Username,
			UsesDefaultCredentials: usesDefault,
			CredentialsUpdatedAt:   now,
		}); err != nil {
			return rollback(err)
		}
		record.Username = *update.Username
		record.UsesDefaultCredentials = usesDefault
	}
	revoked := update.NewPassword != nil
	if update.NewPassword != nil {
		if err := queries.UpdateAdministratorPassword(ctx, configdb.UpdateAdministratorPasswordParams{
			PasswordHash:           passwordHash,
			UsesDefaultCredentials: usesDefault,
			CredentialsUpdatedAt:   now,
		}); err != nil {
			return rollback(err)
		}
		if err := queries.DeleteAllAdministratorSessions(ctx); err != nil {
			return rollback(err)
		}
		record.UsesDefaultCredentials = usesDefault
	}
	if err := transaction.Commit(); err != nil {
		return Account{}, false, err
	}
	return accountFromRecord(record), revoked, nil
}

func (s *Service) UpdateLocale(ctx context.Context, locale string) (Account, error) {
	if locale != "zh-CN" && locale != "en" {
		return Account{}, errors.New("unsupported locale")
	}
	if err := s.queries.UpdateAdministratorLocale(ctx, locale); err != nil {
		return Account{}, err
	}
	return s.Account(ctx)
}

func (s *Service) StartTOTPEnrollment(ctx context.Context, currentPassword string) (TOTPEnrollment, error) {
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if record.TotpEnabled == 1 {
		return TOTPEnrollment{}, ErrTOTPAlreadyEnabled
	}
	valid, _, err := s.hasher.Verify(ctx, currentPassword, record.PasswordHash)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if !valid {
		return TOTPEnrollment{}, ErrCurrentPassword
	}
	enrollment, err := generateTOTPEnrollment(record.Username)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	encrypted, err := encryptTOTPSecret(s.masterKey, enrollment.Secret)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if err := s.queries.SetTOTPEnrollment(ctx, encrypted); err != nil {
		return TOTPEnrollment{}, err
	}
	return enrollment, nil
}

func (s *Service) ConfirmTOTPEnrollment(ctx context.Context, code string) (Account, error) {
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return Account{}, err
	}
	if record.TotpEnabled == 1 {
		return Account{}, ErrTOTPAlreadyEnabled
	}
	if len(record.TotpSecretEncrypted) == 0 {
		return Account{}, ErrTOTPEnrollment
	}
	secret, err := decryptTOTPSecret(s.masterKey, record.TotpSecretEncrypted)
	if err != nil {
		return Account{}, err
	}
	step, err := validateTOTPCode(secret, code, s.now().UTC())
	if err != nil {
		return Account{}, ErrInvalidTOTP
	}
	if err := s.queries.EnableTOTP(ctx, step); err != nil {
		return Account{}, err
	}
	return s.Account(ctx)
}

func (s *Service) DisableTOTP(ctx context.Context, currentPassword, code string) error {
	record, err := s.queries.GetAdministrator(ctx)
	if err != nil {
		return err
	}
	if record.TotpEnabled != 1 {
		return ErrTOTPNotEnabled
	}
	valid, _, err := s.hasher.Verify(ctx, currentPassword, record.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return ErrCurrentPassword
	}
	secret, err := decryptTOTPSecret(s.masterKey, record.TotpSecretEncrypted)
	if err != nil {
		return err
	}
	step, err := validateTOTPCode(secret, code, s.now().UTC())
	if err != nil {
		return ErrInvalidTOTP
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := s.queries.WithTx(transaction)
	rows, err := queries.UseTOTPStep(ctx, configdb.UseTOTPStepParams{
		TotpLastUsedStep: step, TotpLastUsedStep_2: step,
	})
	if err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if rows != 1 {
		return errors.Join(ErrInvalidTOTP, transaction.Rollback())
	}
	if err := queries.DisableTOTP(ctx); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := queries.DeleteAllAdministratorSessions(ctx); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	return transaction.Commit()
}

func (s *Service) RecoverPassword(ctx context.Context, password string) error {
	if _, err := s.queries.GetAdministrator(ctx); err != nil {
		return fmt.Errorf("read administrator: %w", err)
	}
	if err := ValidateNewPassword(password); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return err
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := s.queries.WithTx(transaction)
	if err := queries.UpdateAdministratorPassword(ctx, configdb.UpdateAdministratorPasswordParams{
		PasswordHash: hash, CredentialsUpdatedAt: s.now().UTC().Unix(),
	}); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := queries.DeleteAllAdministratorSessions(ctx); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	return transaction.Commit()
}

func (s *Service) RecoverDisableTOTP(ctx context.Context) error {
	if _, err := s.queries.GetAdministrator(ctx); err != nil {
		return fmt.Errorf("read administrator: %w", err)
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := s.queries.WithTx(transaction)
	if err := queries.DisableTOTP(ctx); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	if err := queries.DeleteAllAdministratorSessions(ctx); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	return transaction.Commit()
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func defaultCredentialsValue(username, password string) int64 {
	if strings.EqualFold(username, "admin") && password == "admin" {
		return 1
	}
	return 0
}
