package mail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Directory resolves account identifiers to email addresses. The users table
// already offers this, so mail keeps no user list of its own.
type Directory interface {
	LookupEmails(context.Context, []string) (map[string]string, error)
}

// Ledger persists one row per attempt. Bodies are never stored.
type Ledger interface {
	InsertMailDelivery(context.Context, Delivery) error
	CompleteMailDelivery(ctx context.Context, id, status string, attempts int, errorMessage string, at time.Time) error
}

// Delivery is one attempt to send one message to one person.
type Delivery struct {
	ID           string    `json:"id"`
	Event        string    `json:"event"`
	Recipient    string    `json:"recipient"`
	Subject      string    `json:"subject"`
	Ref          string    `json:"ref,omitempty"`
	ActorID      string    `json:"actor_id,omitempty"`
	Status       string    `json:"status"` // queued | sent | failed
	Attempts     int       `json:"attempts"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// MaxInFlight caps the background deliveries a service runs at once. A dead
// relay holds each one for two attempts plus the timeout, so without a cap a
// burst of events would pile up goroutines for minutes; beyond the cap a
// delivery is recorded as failed with ErrBusy instead of being started.
const MaxInFlight = 64

// Service sends notifications without ever blocking a request.
type Service struct {
	config    func() Config
	ledger    Ledger
	directory Directory
	logger    *slog.Logger
	now       func() time.Time
	send      func(context.Context, Config, Message) error
	wg        sync.WaitGroup
	inflight  chan struct{} // one slot per running delivery goroutine
}

// NewService wires the service to a config snapshot getter, the delivery
// ledger and the user directory. Any of the last two may be nil.
func NewService(config func() Config, ledger Ledger, directory Directory, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if config == nil {
		config = func() Config { return Config{} }
	}
	return &Service{config: config, ledger: ledger, directory: directory, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, send: Deliver,
		inflight: make(chan struct{}, MaxInFlight)}
}

// SetSender replaces the transport, which lets tests drive the service without
// a real relay.
func (s *Service) SetSender(sender func(context.Context, Config, Message) error) { s.send = sender }

// Config returns the current snapshot.
func (s *Service) Config() Config { return s.config() }

// Wait blocks until background deliveries finish; tests use it.
func (s *Service) Wait() { s.wg.Wait() }

// Notify resolves the recipients and sends in the background so no request
// waits on a mail server. The actor never receives mail about their own
// action, and recipients without an address are skipped quietly. Every
// recipient of one call gets exactly one message. When MaxInFlight deliveries
// are already running the message is recorded as failed rather than queued
// without bound.
func (s *Service) Notify(ctx context.Context, notification Notification, actorID string, recipients []string) {
	if s == nil {
		return
	}
	config := s.config()
	if !config.Enabled || !config.Allows(notification.Event) || config.Validate() != nil {
		return
	}
	addresses := s.resolve(ctx, recipients, actorID)
	if len(addresses) == 0 {
		return
	}
	body := notification.Render(config)
	for _, address := range addresses {
		delivery := s.record(ctx, notification, actorID, address)
		select {
		case s.inflight <- struct{}{}:
		default:
			s.complete(ctx, delivery, 0, ErrBusy)
			continue
		}
		s.wg.Add(1)
		go func(delivery Delivery, address string) {
			defer s.wg.Done()
			defer func() { <-s.inflight }()
			s.deliver(delivery, config, Message{To: address, Subject: notification.Subject, Body: body})
		}(delivery, address)
	}
}

// SendNow delivers immediately and reports the outcome, which is what the
// administrator's test button needs.
func (s *Service) SendNow(ctx context.Context, notification Notification, actorID, recipient string) error {
	config := s.config()
	if !config.Enabled {
		return ErrDisabled
	}
	if err := config.Validate(); err != nil {
		return err
	}
	delivery := s.record(ctx, notification, actorID, recipient)
	sendContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.Timeout+5*time.Second)
	defer cancel()
	err := s.send(sendContext, config, Message{To: recipient, Subject: notification.Subject, Body: notification.Render(config)})
	s.complete(sendContext, delivery, 1, err)
	return err
}

// deliver retries once, because a relay that briefly refuses a connection is
// common and losing the notification is worse than a short wait.
func (s *Service) deliver(delivery Delivery, config Config, message Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*config.Timeout+15*time.Second)
	defer cancel()
	var err error
	attempts := 0
	for attempts < 2 {
		attempts++
		if err = s.send(ctx, config, message); err == nil {
			break
		}
		if attempts == 1 {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	s.complete(ctx, delivery, attempts, err)
}

func (s *Service) record(ctx context.Context, notification Notification, actorID, address string) Delivery {
	delivery := Delivery{
		ID: newID(), Event: notification.Event, Recipient: address, Subject: trim(notification.Subject, 300),
		Ref: notification.Ref, ActorID: actorID, Status: "queued", CreatedAt: s.now(), UpdatedAt: s.now(),
	}
	if s.ledger != nil {
		if err := s.ledger.InsertMailDelivery(ctx, delivery); err != nil {
			s.logger.Warn("mail delivery was not recorded", "error", err)
		}
	}
	return delivery
}

func (s *Service) complete(ctx context.Context, delivery Delivery, attempts int, cause error) {
	status, message := "sent", ""
	if cause != nil {
		status, message = "failed", trim(cause.Error(), 1000)
		// The address and the reason are enough to answer "it never arrived"; the
		// body and credentials never reach the log.
		s.logger.Warn("notification mail failed", "event", delivery.Event, "recipient", delivery.Recipient, "error", cause)
	}
	if s.ledger == nil {
		return
	}
	if err := s.ledger.CompleteMailDelivery(ctx, delivery.ID, status, attempts, message, s.now()); err != nil {
		s.logger.Warn("mail delivery status was not recorded", "error", err)
	}
}

// resolve turns account identifiers into unique addresses, dropping the actor
// so nobody is told about their own action.
func (s *Service) resolve(ctx context.Context, recipients []string, actorID string) []string {
	actor := strings.TrimSpace(actorID)
	wanted := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		trimmed := strings.TrimSpace(recipient)
		if trimmed == "" || (actor != "" && strings.EqualFold(trimmed, actor)) {
			continue
		}
		wanted = append(wanted, trimmed)
	}
	if len(wanted) == 0 {
		return nil
	}
	emails := map[string]string{}
	if s.directory != nil {
		found, err := s.directory.LookupEmails(ctx, wanted)
		if err != nil {
			s.logger.Warn("mail recipients were not resolved", "error", err)
			return nil
		}
		emails = found
	}
	seen, addresses := map[string]struct{}{}, make([]string, 0, len(wanted))
	for _, recipient := range wanted {
		address := strings.TrimSpace(emails[strings.ToLower(recipient)])
		if address == "" && strings.Contains(recipient, "@") {
			// An identifier that is already an address needs no directory entry.
			address = recipient
		}
		if address == "" || (actor != "" && strings.EqualFold(address, actor)) {
			continue
		}
		key := strings.ToLower(address)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}

func newID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return "mail_" + hex.EncodeToString(raw[:])
}

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
