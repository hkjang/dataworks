// Package mail delivers event notifications through a company SMTP relay.
//
// Internal relays commonly accept mail on port 25 with no credentials and no
// TLS, so authentication and encryption are optional and the transport adapts
// to whatever the server advertises. Credentials, however, never travel over a
// connection without TLS: PLAIN and LOGIN are used only after STARTTLS or on
// an implicit-TLS port, and a plaintext relay that demands them is reported as
// a configuration error rather than fed the password. Nothing here blocks a
// request: sending happens in the background and every attempt is recorded so
// an administrator can see what left the building.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

var (
	ErrDisabled = errors.New("mail is disabled")
	ErrInvalid  = errors.New("invalid mail configuration")
	ErrBusy     = errors.New("too many mail deliveries in flight")
)

// Event names. Each one maps to a mail.notify_* switch in eventSettings; the
// events people wait for in this app are approvals, reviews, alerts and expiry.
const (
	EventApprovalRequested = "approval.requested"   // a proxied request is parked until an admin decides
	EventApprovalDecided   = "approval.decided"     // the requester hears the decision
	EventChangeSetSubmit   = "change_set.submitted" // a settings change set waits for review
	EventAlertFired        = "alert.fired"          // an alert rule crossed its threshold
	EventAccessExpiring    = "access.expiring"      // daily digest of contracts and entitlements about to lapse
	EventTest              = "test"
)

// Config is the effective relay configuration. Username and Password never
// serialise; the settings API reports the password only as "set".
type Config struct {
	Enabled     bool          `json:"enabled"`
	Host        string        `json:"host"`
	Port        int           `json:"port"`
	Username    string        `json:"-"`
	Password    string        `json:"-"`
	FromAddress string        `json:"from_address"`
	FromName    string        `json:"from_name"`
	Security    string        `json:"security"`
	SkipVerify  bool          `json:"skip_verify"`
	BaseURL     string        `json:"base_url"`
	Timeout     time.Duration `json:"-"`
	Events      map[string]bool
}

// Address is the RFC 5322 From header value.
func (c Config) Address() string {
	from := strings.TrimSpace(c.FromAddress)
	if name := strings.TrimSpace(c.FromName); name != "" {
		return mime.QEncoding.Encode("utf-8", name) + " <" + from + ">"
	}
	return from
}

func (c Config) endpoint() string { return net.JoinHostPort(c.Host, fmt.Sprint(c.Port)) }

// timeout is the per-step relay limit actually applied: connect and, after
// that, the whole session each get this long.
func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultTimeout
	}
	return c.Timeout
}

// AttemptBudget is the longest one Deliver call can take: a connect that
// takes the full timeout followed by a session that takes the full timeout.
func (c Config) AttemptBudget() time.Duration { return 2 * c.timeout() }

// Allows reports whether an event should be delivered. Unknown events are sent,
// so adding a notification never requires a settings change first.
func (c Config) Allows(event string) bool {
	if enabled, known := c.Events[event]; known {
		return enabled
	}
	return true
}

// Validate reports why the configuration cannot send, or nil when it can.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("%w: mail.smtp_host is required", ErrInvalid)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("%w: mail.smtp_port must be between 1 and 65535", ErrInvalid)
	}
	if !strings.Contains(c.FromAddress, "@") {
		return fmt.Errorf("%w: mail.from_address must be an email address", ErrInvalid)
	}
	if !ValidSecurity(c.Security) {
		return fmt.Errorf("%w: mail.security must be auto, none, starttls, or tls", ErrInvalid)
	}
	return nil
}

// ValidSecurity reports whether v is one of the supported transport modes.
func ValidSecurity(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "auto", "none", "starttls", "tls":
		return true
	}
	return false
}

type Message struct {
	To      string
	Subject string
	Body    string
}

// Deliver opens a connection and sends one message. It is exported so the
// settings screen can prove the relay works before anything depends on it.
func Deliver(ctx context.Context, config Config, message Message) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("%w: recipient is required", ErrInvalid)
	}
	client, err := dial(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	// The connection deadline only bounds the relay's silence; the caller's
	// context must be able to end the session too, or a send outlives the budget
	// the caller planned around.
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	if err := send(client, config, message); err != nil {
		if cause := ctx.Err(); cause != nil {
			return fmt.Errorf("SMTP 발송 제한 시간 초과: %w", cause)
		}
		return err
	}
	return nil
}

func send(client *smtp.Client, config Config, message Message) error {
	if err := startSession(client, config); err != nil {
		return err
	}
	if err := client.Mail(strings.TrimSpace(config.FromAddress)); err != nil {
		return fmt.Errorf("MAIL FROM 실패: %w", err)
	}
	if err := client.Rcpt(strings.TrimSpace(message.To)); err != nil {
		return fmt.Errorf("RCPT TO 실패: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA 실패: %w", err)
	}
	if _, err := writer.Write([]byte(compose(config, message))); err != nil {
		return fmt.Errorf("본문 전송 실패: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("본문 종료 실패: %w", err)
	}
	return client.Quit()
}

func dial(ctx context.Context, config Config) (*smtp.Client, error) {
	timeout := config.timeout()
	dialer := &net.Dialer{Timeout: timeout}
	var (
		connection net.Conn
		err        error
	)
	if config.Security == "tls" {
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: config.tlsConfig()}).DialContext(ctx, "tcp", config.endpoint())
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", config.endpoint())
	}
	if err != nil {
		return nil, fmt.Errorf("SMTP 연결 실패: %w", err)
	}
	// The relay must answer within the timeout at every step, not only on connect.
	_ = connection.SetDeadline(time.Now().Add(timeout))
	client, err := smtp.NewClient(connection, config.Host)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("SMTP 세션 시작 실패: %w", err)
	}
	return client, nil
}

// ErrPlaintextAuth is returned when credentials are configured but the
// connection ended up without TLS and the relay offers only mechanisms that
// would put the password on the wire (PLAIN, LOGIN). The policy is documented
// under mail.security in the admin guide: use starttls/tls, or clear the
// username for a relay that needs no credentials.
var ErrPlaintextAuth = fmt.Errorf("%w: TLS 없이는 인증하지 않습니다. mail.security 를 starttls 또는 tls 로 바꾸거나 사용자 이름을 비우세요", ErrInvalid)

// startSession upgrades and authenticates only as far as the relay allows, so
// an unauthenticated internal relay works with the same settings as a hosted
// provider that demands both.
func startSession(client *smtp.Client, config Config) error {
	if err := client.Hello(helloName(config)); err != nil {
		return fmt.Errorf("EHLO 실패: %w", err)
	}
	if config.Security == "starttls" || config.Security == "auto" {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(config.tlsConfig()); err != nil {
				return fmt.Errorf("STARTTLS 실패: %w", err)
			}
		} else if config.Security == "starttls" {
			return fmt.Errorf("%w: 서버가 STARTTLS를 지원하지 않습니다", ErrInvalid)
		}
	}
	if strings.TrimSpace(config.Username) == "" {
		return nil
	}
	supported, mechanisms := client.Extension("AUTH")
	if !supported {
		return fmt.Errorf("%w: 서버가 인증을 지원하지 않습니다. 사용자 이름을 비우고 사용하세요", ErrInvalid)
	}
	upper := strings.ToUpper(mechanisms)
	// One judgment for every mechanism that sends the password itself: only
	// over TLS, whether that came from STARTTLS or an implicit-TLS port. It does
	// not matter what the relay is called or what it advertises.
	_, secured := client.TLSConnectionState()
	switch {
	case secured && strings.Contains(upper, "PLAIN"):
		return client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.Host))
	case secured && strings.Contains(upper, "LOGIN"):
		return client.Auth(loginAuth{username: config.Username, password: config.Password})
	case strings.Contains(upper, "CRAM-MD5"), secured:
		// CRAM-MD5 answers a challenge without revealing the password, so it is
		// fine on a plaintext relay; over TLS it is also the fallback when the
		// relay advertises nothing we recognise.
		return client.Auth(smtp.CRAMMD5Auth(config.Username, config.Password))
	}
	return ErrPlaintextAuth
}

func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.SkipVerify} //nolint:gosec // opt-in for internal relays with private certificates
}

// helloName keeps the EHLO name to the sender domain, which relays that check
// the greeting are happier with than a container hostname.
func helloName(config Config) string {
	if index := strings.LastIndex(config.FromAddress, "@"); index >= 0 && index+1 < len(config.FromAddress) {
		return config.FromAddress[index+1:]
	}
	return "localhost"
}

// loginAuth implements the LOGIN mechanism that several corporate relays use
// instead of PLAIN. The standard library only ships PLAIN and CRAM-MD5.
type loginAuth struct{ username, password string }

// Start applies the same rule as startSession: the password only ever goes
// over TLS. The check is repeated here so the mechanism stays safe even if it
// is picked by some other path.
func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, ErrPlaintextAuth
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimRight(string(fromServer), ": ")) {
	case "username":
		return []byte(a.username), nil
	case "password":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("알 수 없는 LOGIN 요청: %s", fromServer)
}
