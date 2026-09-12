package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// OIDCFlowState is the short-lived, single-use record created when a login is started and
// consumed by the Authorization Code callback.
type OIDCFlowState struct {
	Nonce    string
	Verifier string
	// Silent marks a prompt=none attempt. The callback uses it to treat a login_required
	// refusal as an ordinary "no session" answer instead of a login failure.
	Silent bool
	// ReturnTo is the in-app path to land on after login ("" = role default landing).
	ReturnTo string
}

// SaveOIDCFlowState persists a short-lived OIDC login-flow state (state → nonce + PKCE verifier
// + silent/return_to) so the Authorization Code callback can validate it even if it lands on a
// different instance or after a restart. Opportunistically prunes entries older than 10 minutes.
func (s *SQLStore) SaveOIDCFlowState(ctx context.Context, state string, fs OIDCFlowState, createdAt time.Time) error {
	cutoff := createdAt.Add(-10 * time.Minute).UTC().Format(time.RFC3339Nano)
	_, _ = s.db.ExecContext(ctx, s.bind(`DELETE FROM oidc_flow_states WHERE created_at < ?`), cutoff)
	silent := 0
	if fs.Silent {
		silent = 1
	}
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO oidc_flow_states (state, nonce, verifier, silent, return_to, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(state) DO UPDATE SET nonce=excluded.nonce, verifier=excluded.verifier,
			silent=excluded.silent, return_to=excluded.return_to, created_at=excluded.created_at`),
		state, fs.Nonce, fs.Verifier, silent, fs.ReturnTo, createdAt.UTC().Format(time.RFC3339Nano))
	return err
}

// TakeOIDCFlowState atomically consumes a flow state: it returns the record and deletes the row.
// found is false if the state is unknown or older than the 10-minute TTL.
func (s *SQLStore) TakeOIDCFlowState(ctx context.Context, state string) (fs OIDCFlowState, found bool, err error) {
	var (
		createdAt string
		silent    int
	)
	row := s.db.QueryRowContext(ctx, s.bind(`SELECT nonce, verifier, COALESCE(silent,0), COALESCE(return_to,''), created_at
		FROM oidc_flow_states WHERE state = ?`), state)
	if err = row.Scan(&fs.Nonce, &fs.Verifier, &silent, &fs.ReturnTo, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OIDCFlowState{}, false, nil
		}
		return OIDCFlowState{}, false, err
	}
	fs.Silent = silent != 0
	// Consume regardless of age (single-use).
	_, _ = s.db.ExecContext(ctx, s.bind(`DELETE FROM oidc_flow_states WHERE state = ?`), state)
	if ts, perr := time.Parse(time.RFC3339Nano, createdAt); perr == nil && time.Since(ts) > 10*time.Minute {
		return OIDCFlowState{}, false, nil
	}
	return fs, true, nil
}
