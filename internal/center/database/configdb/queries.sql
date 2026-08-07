-- name: GetAdministrator :one
SELECT id, username, password_hash, locale, uses_default_credentials,
       totp_secret_encrypted, totp_enabled, totp_last_used_step,
       created_at, credentials_updated_at
FROM administrators
WHERE id = 1;

-- name: CreateAdministrator :exec
INSERT INTO administrators (
    id, username, password_hash, locale, uses_default_credentials,
    created_at, credentials_updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?);

-- name: GetAdministratorByUsername :one
SELECT id, username, password_hash, locale, uses_default_credentials,
       totp_secret_encrypted, totp_enabled, totp_last_used_step,
       created_at, credentials_updated_at
FROM administrators
WHERE username = ? COLLATE NOCASE;

-- name: UpdateAdministratorUsername :exec
UPDATE administrators
SET username = ?, uses_default_credentials = ?,
    credentials_updated_at = ?
WHERE id = 1;

-- name: UpdateAdministratorPassword :exec
UPDATE administrators
SET password_hash = ?, uses_default_credentials = ?,
    credentials_updated_at = ?
WHERE id = 1;

-- name: RehashAdministratorPassword :exec
UPDATE administrators
SET password_hash = ?
WHERE id = 1;

-- name: UpdateAdministratorLocale :exec
UPDATE administrators
SET locale = ?
WHERE id = 1;

-- name: SetTOTPEnrollment :exec
UPDATE administrators
SET totp_secret_encrypted = ?, totp_enabled = 0,
    totp_last_used_step = -1
WHERE id = 1;

-- name: EnableTOTP :exec
UPDATE administrators
SET totp_enabled = 1, totp_last_used_step = ?
WHERE id = 1 AND totp_secret_encrypted IS NOT NULL;

-- name: UseTOTPStep :execrows
UPDATE administrators
SET totp_last_used_step = ?
WHERE id = 1 AND totp_enabled = 1 AND totp_last_used_step < ?;

-- name: DisableTOTP :exec
UPDATE administrators
SET totp_secret_encrypted = NULL, totp_enabled = 0,
    totp_last_used_step = -1
WHERE id = 1;

-- name: CreateAdministratorSession :exec
INSERT INTO administrator_sessions (
    token_digest, created_at, expires_at, client_address, user_agent
) VALUES (?, ?, ?, ?, ?);

-- name: GetAdministratorSession :one
SELECT token_digest, created_at, expires_at, client_address, user_agent
FROM administrator_sessions
WHERE token_digest = ? AND expires_at > ?;

-- name: DeleteAdministratorSession :exec
DELETE FROM administrator_sessions
WHERE token_digest = ?;

-- name: DeleteAllAdministratorSessions :exec
DELETE FROM administrator_sessions;

-- name: DeleteExpiredAdministratorSessions :exec
DELETE FROM administrator_sessions
WHERE expires_at <= ?;

-- name: GetSystemState :one
SELECT id, history_generation, pending_history_generation, history_reset_at
FROM system_state
WHERE id = 1;

-- name: CreateSystemState :exec
INSERT INTO system_state (id, history_generation)
VALUES (1, ?);

-- name: SetPendingHistoryGeneration :exec
UPDATE system_state
SET pending_history_generation = ?
WHERE id = 1 AND pending_history_generation IS NULL;

-- name: PromotePendingHistoryGeneration :execrows
UPDATE system_state
SET history_generation = pending_history_generation,
    pending_history_generation = NULL,
    history_reset_at = ?
WHERE id = 1 AND pending_history_generation = ?;
