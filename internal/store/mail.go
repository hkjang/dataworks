package store

import (
	"context"
	"strings"
	"time"

	"dataworks/internal/mail"
)

// MailDeliveryPage is what the admin screen lists: newest attempts first plus
// a status breakdown over the whole table.
type MailDeliveryPage struct {
	Items   []mail.Delivery `json:"items"`
	Total   int             `json:"total"`
	Summary map[string]int  `json:"summary"`
}

// InsertMailDelivery records an attempt before it is made. The body is never
// stored: subject and recipient answer "did it go out" without becoming a
// second copy of the content.
func (s *SQLStore) InsertMailDelivery(ctx context.Context, d mail.Delivery) error {
	_, err := s.db.ExecContext(ctx, s.bind(`INSERT INTO mail_deliveries
		(id, event, recipient, subject, ref, actor_id, status, attempts, error_message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'queued', 0, '', ?, ?)`),
		d.ID, d.Event, d.Recipient, d.Subject, d.Ref, d.ActorID, formatTime(d.CreatedAt), formatTime(d.CreatedAt))
	return err
}

// CompleteMailDelivery stores the outcome of an attempt.
func (s *SQLStore) CompleteMailDelivery(ctx context.Context, id, status string, attempts int, errorMessage string, at time.Time) error {
	if attempts < 1 {
		attempts = 1
	}
	_, err := s.db.ExecContext(ctx, s.bind(`UPDATE mail_deliveries SET status = ?, attempts = ?, error_message = ?, updated_at = ? WHERE id = ?`),
		status, attempts, errorMessage, formatTime(at), id)
	return err
}

// ListMailDeliveries lists attempts newest first, optionally filtered by
// status or event, with a status breakdown over every row.
func (s *SQLStore) ListMailDeliveries(ctx context.Context, status, event string, limit int) (MailDeliveryPage, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	page := MailDeliveryPage{Items: []mail.Delivery{}, Summary: map[string]int{}}
	query := `SELECT id, event, recipient, subject, ref, actor_id, status, attempts, error_message, created_at, updated_at FROM mail_deliveries`
	where, args := []string{}, []any{}
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		where = append(where, "status = ?")
		args = append(args, trimmed)
	}
	if trimmed := strings.TrimSpace(event); trimmed != "" {
		where = append(where, "event = ?")
		args = append(args, trimmed)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.bind(query+" ORDER BY created_at DESC, id DESC LIMIT ?"), args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item mail.Delivery
		var createdAt, updatedAt string
		if err := rows.Scan(&item.ID, &item.Event, &item.Recipient, &item.Subject, &item.Ref, &item.ActorID,
			&item.Status, &item.Attempts, &item.ErrorMessage, &createdAt, &updatedAt); err != nil {
			return page, err
		}
		item.CreatedAt = parseOptionalTime(createdAt)
		item.UpdatedAt = parseOptionalTime(updatedAt)
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	counts, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM mail_deliveries GROUP BY status`)
	if err != nil {
		return page, err
	}
	defer counts.Close()
	for counts.Next() {
		var key string
		var count int
		if err := counts.Scan(&key, &count); err != nil {
			return page, err
		}
		page.Summary[key] = count
		page.Total += count
	}
	return page, counts.Err()
}

// CountMailDeliveriesSince tells whether an event already went out after a
// point in time; the daily digest uses it so a restart or a second pod does
// not send the same summary twice.
func (s *SQLStore) CountMailDeliveriesSince(ctx context.Context, event string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, s.bind(`SELECT COUNT(*) FROM mail_deliveries WHERE event = ? AND created_at >= ?`),
		event, formatTime(since)).Scan(&count)
	return count, err
}

// LookupEmails maps account identifiers (user id or email, matched case
// insensitively) to the address on the users table. Only active accounts
// resolve, so a disabled user stops receiving mail the moment they are
// disabled. Keys in the result are lower-cased identifiers.
func (s *SQLStore) LookupEmails(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	wanted := map[string]struct{}{}
	for _, id := range ids {
		if trimmed := strings.ToLower(strings.TrimSpace(id)); trimmed != "" {
			wanted[trimmed] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, email FROM users WHERE status = 'active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		for _, key := range []string{strings.ToLower(strings.TrimSpace(id)), strings.ToLower(email)} {
			if _, ok := wanted[key]; ok {
				out[key] = email
			}
		}
	}
	return out, rows.Err()
}

// AdminUserIDs returns the active accounts whose role carries every scope —
// the people who decide approvals and review change sets.
func (s *SQLStore) AdminUserIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM users WHERE status = 'active' AND role IN ('admin', 'super_admin') ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
