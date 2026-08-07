package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/pquerna/otp/totp"
)

func TestBootstrapIsAppliedOnlyOnce(t *testing.T) {
	service, store := newTestService(t)
	if err := service.Bootstrap(context.Background(), "first", "first-password"); err != nil {
		t.Fatal(err)
	}
	if err := service.Bootstrap(context.Background(), "second", "second-password"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(context.Background(), "first", "first-password", "", "192.0.2.1", "test"); err != nil {
		t.Fatalf("original bootstrap credentials no longer work: %v", err)
	}
	if _, _, err := service.Login(context.Background(), "second", "second-password", "", "192.0.2.2", "test"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("second bootstrap changed account: %v", err)
	}
	_ = store
}

func TestSessionPersistsAndCanBeRevoked(t *testing.T) {
	service, store := newBootstrappedTestService(t)
	session, _, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(store.Config, store.ConfigQueries, store.MasterKey)
	principal, err := restarted.Authenticate(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("session did not survive service restart: %v", err)
	}
	if err := restarted.RevokeSession(context.Background(), principal.TokenDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate(context.Background(), session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked session still authenticated: %v", err)
	}
}

func TestSessionExpiresAtAbsoluteLifetime(t *testing.T) {
	service, _ := newBootstrappedTestService(t)
	now := time.Unix(2_000_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	session, _, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(sessionLifetime)
	if _, err := service.Authenticate(context.Background(), session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session remained valid at its absolute expiry: %v", err)
	}
}

func TestLoginThrottleUsesUsernameAndClientAddress(t *testing.T) {
	t.Run("username", func(t *testing.T) {
		service, _ := newBootstrappedTestService(t)
		now := time.Unix(2_000_000_000, 0).UTC()
		service.now = func() time.Time { return now }
		for range 2 {
			if _, _, err := service.Login(context.Background(), "admin", "wrong", "", "192.0.2.1", "test"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("failed login = %v", err)
			}
		}
		if _, wait, err := service.Login(context.Background(), "ADMIN", "admin", "", "192.0.2.2", "test"); !errors.Is(err, ErrRateLimited) || wait != time.Second {
			t.Fatalf("username throttle = %v, %v", wait, err)
		}
		now = now.Add(time.Second)
		if _, _, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.2", "test"); err != nil {
			t.Fatalf("successful login did not recover after delay: %v", err)
		}
	})

	t.Run("client address", func(t *testing.T) {
		service, _ := newBootstrappedTestService(t)
		now := time.Unix(2_000_000_000, 0).UTC()
		service.now = func() time.Time { return now }
		for range 2 {
			if _, _, err := service.Login(context.Background(), "someone", "wrong", "", "192.0.2.1", "test"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("failed login = %v", err)
			}
		}
		if _, wait, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.1", "test"); !errors.Is(err, ErrRateLimited) || wait != time.Second {
			t.Fatalf("address throttle = %v, %v", wait, err)
		}
	})
}

func TestDefaultCredentialWarningTracksActualCredentials(t *testing.T) {
	service, _ := newTestService(t)
	if err := service.Bootstrap(context.Background(), "Admin", "admin"); err != nil {
		t.Fatal(err)
	}
	account, err := service.Account(context.Background())
	if err != nil || !account.UsesDefaultCredentials {
		t.Fatalf("case-insensitive default account was not detected: %#v, %v", account, err)
	}
	owner := "owner"
	account, _, err = service.UpdateAccount(context.Background(), AccountUpdate{
		CurrentPassword: "admin", Username: &owner,
	})
	if err != nil || account.UsesDefaultCredentials {
		t.Fatalf("renamed account still reports defaults: %#v, %v", account, err)
	}
	adminUsername := "admin"
	account, _, err = service.UpdateAccount(context.Background(), AccountUpdate{
		CurrentPassword: "admin", Username: &adminUsername,
	})
	if err != nil || !account.UsesDefaultCredentials {
		t.Fatalf("restored default credentials were not detected: %#v, %v", account, err)
	}
}

func TestAccountPasswordChangeAndRecoveryRevokeSessions(t *testing.T) {
	service, _ := newBootstrappedTestService(t)
	first, _, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	newPassword := "updated-password"
	account, revoked, err := service.UpdateAccount(context.Background(), AccountUpdate{
		CurrentPassword: "admin",
		NewPassword:     &newPassword,
	})
	if err != nil || !revoked || account.UsesDefaultCredentials {
		t.Fatalf("password update result = %#v, %v, %v", account, revoked, err)
	}
	if _, err := service.Authenticate(context.Background(), first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("password change did not revoke session: %v", err)
	}
	second, _, err := service.Login(context.Background(), "admin", newPassword, "", "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverPassword(context.Background(), "recovered-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), second.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("recovery did not revoke session: %v", err)
	}
	if _, _, err := service.Login(context.Background(), "admin", "recovered-password", "", "192.0.2.1", "test"); err != nil {
		t.Fatalf("recovered password does not work: %v", err)
	}
}

func TestTOTPEnrollmentLoginAndRecovery(t *testing.T) {
	service, _ := newBootstrappedTestService(t)
	now := time.Unix(2_000_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	enrollment, err := service.StartTOTPEnrollment(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(enrollment.Secret, now)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.ConfirmTOTPEnrollment(context.Background(), code)
	if err != nil || !account.TOTPEnabled {
		t.Fatalf("confirm enrollment = %#v, %v", account, err)
	}
	if _, _, err := service.Login(context.Background(), "admin", "admin", code, "192.0.2.1", "test"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("enrollment confirmation code was accepted again: %v", err)
	}
	if _, _, err := service.Login(context.Background(), "admin", "admin", "", "192.0.2.1", "test"); !errors.Is(err, ErrTOTPRequired) {
		t.Fatalf("login without TOTP = %v", err)
	}
	now = now.Add(31 * time.Second)
	code, err = totp.GenerateCode(enrollment.Secret, now)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := service.Login(context.Background(), "admin", "admin", code, "192.0.2.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Login(context.Background(), "admin", "admin", code, "192.0.2.2", "test"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("reused TOTP code was accepted: %v", err)
	}
	if err := service.RecoverDisableTOTP(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("TOTP recovery did not revoke session: %v", err)
	}
	account, err = service.Account(context.Background())
	if err != nil || account.TOTPEnabled {
		t.Fatalf("TOTP remains enabled after recovery: %#v, %v", account, err)
	}
}

func newBootstrappedTestService(t *testing.T) (*Service, *database.Store) {
	t.Helper()
	service, store := newTestService(t)
	if err := service.Bootstrap(context.Background(), "admin", "admin"); err != nil {
		t.Fatal(err)
	}
	return service, store
}

func newTestService(t *testing.T) (*Service, *database.Store) {
	t.Helper()
	store, err := database.Open(context.Background(), database.PathsFromDataDirectory(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewService(store.Config, store.ConfigQueries, store.MasterKey), store
}
