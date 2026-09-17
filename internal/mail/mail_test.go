package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/big"
	"net"
	"net/smtp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRelay is a minimal port-25 style relay: by default no TLS and no AUTH,
// which is what most company relays look like from the app's side. Tests can
// turn on AUTH and TLS to check what the client sends in each case.
type fakeRelay struct {
	listener net.Listener
	auth     string // mechanisms advertised after EHLO, "" for none
	// silentOnMail makes the relay accept MAIL FROM and never answer it, like a
	// relay that went quiet after the greeting.
	silentOnMail bool
	// tlsConfig, when set, makes the relay offer STARTTLS (or, with implicitTLS,
	// speak TLS from the first byte like port 465).
	tlsConfig   *tls.Config
	implicitTLS bool
	mu          sync.Mutex
	from        string
	to          []string
	data        string
	ehlo        string
	authed      []string // every AUTH line the client sent, verbatim
}

func startFakeRelay(t *testing.T) *fakeRelay {
	return startFakeRelayWithAuth(t, "")
}

// startFakeRelayWithAuth advertises AUTH with the given mechanisms and accepts
// any credentials, still without STARTTLS.
func startFakeRelayWithAuth(t *testing.T, mechanisms string) *fakeRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay := &fakeRelay{listener: listener, auth: mechanisms}
	go relay.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return relay
}

func (f *fakeRelay) addr() (string, int) {
	addr := f.listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func (f *fakeRelay) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.session(conn)
	}
}

func (f *fakeRelay) session(conn net.Conn) {
	defer conn.Close()
	if f.tlsConfig != nil && f.implicitTLS {
		conn = tls.Server(conn, f.tlsConfig)
	}
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 relay.test ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			f.mu.Lock()
			f.ehlo = strings.TrimSpace(line[4:])
			f.mu.Unlock()
			write("250-relay.test")
			if _, already := conn.(*tls.Conn); f.tlsConfig != nil && !already {
				write("250-STARTTLS")
			}
			if f.auth != "" {
				write("250-AUTH " + f.auth)
			}
			write("250 SIZE 10240000")
		case upper == "STARTTLS":
			write("220 go ahead")
			conn = tls.Server(conn, f.tlsConfig)
			reader = bufio.NewReader(conn)
		case strings.HasPrefix(upper, "HELO"):
			write("250 relay.test")
		case strings.HasPrefix(upper, "AUTH "):
			f.mu.Lock()
			f.authed = append(f.authed, line)
			f.mu.Unlock()
			if strings.HasPrefix(upper, "AUTH LOGIN") {
				write("334 VXNlcm5hbWU6") // Username:
				if _, err := reader.ReadString('\n'); err != nil {
					return
				}
				write("334 UGFzc3dvcmQ6") // Password:
				if _, err := reader.ReadString('\n'); err != nil {
					return
				}
			}
			write("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			if f.silentOnMail {
				_, _ = reader.ReadString('\n') // blocks until the client hangs up
				return
			}
			f.mu.Lock()
			f.from = strings.Trim(line[10:], "<> ")
			f.mu.Unlock()
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			f.mu.Lock()
			f.to = append(f.to, strings.Trim(line[8:], "<> "))
			f.mu.Unlock()
			write("250 OK")
		case upper == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				part, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if part == ".\r\n" {
					break
				}
				body.WriteString(part)
			}
			f.mu.Lock()
			f.data = body.String()
			f.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("502 not implemented")
		}
	}
}

func (f *fakeRelay) received() (string, []string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.from, append([]string(nil), f.to...), f.data
}

func (f *fakeRelay) authLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.authed...)
}

func relayConfig(relay *fakeRelay) Config {
	host, port := relay.addr()
	return Config{Enabled: true, Host: host, Port: port, Security: "auto", FromAddress: "dataworks@example.test", FromName: "Data Works", Timeout: 3 * time.Second, Events: map[string]bool{}}
}

func TestDeliverSpeaksPlainSMTPToAnUnauthenticatedRelay(t *testing.T) {
	relay := startFakeRelay(t)
	config := relayConfig(relay)
	err := Deliver(context.Background(), config, Message{To: "ops@example.test", Subject: "승인 요청", Body: "첫 줄\n.둘째 줄"})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	from, to, data := relay.received()
	if from != "dataworks@example.test" || len(to) != 1 || to[0] != "ops@example.test" {
		t.Fatalf("envelope = %q -> %v", from, to)
	}
	if !strings.Contains(data, "Subject: =?utf-8?q?") || !strings.Contains(data, "Content-Type: text/plain; charset=utf-8") {
		t.Fatalf("headers missing UTF-8 subject/content type:\n%s", data)
	}
	if !strings.Contains(data, "\r\n첫 줄\r\n..둘째 줄") {
		t.Fatalf("body not dot-stuffed/CRLF normalised:\n%s", data)
	}
	if relay.ehlo != "example.test" {
		t.Fatalf("EHLO = %q, want sender domain", relay.ehlo)
	}
}

// Credentials never travel over a connection without TLS, whichever mechanism
// the relay advertises: PLAIN would fail anyway and LOGIN would leak the
// password in base64. The relay must not see an AUTH command at all.
func TestDeliverRefusesToSendCredentialsWithoutTLS(t *testing.T) {
	for _, mechanisms := range []string{"LOGIN", "PLAIN", "PLAIN LOGIN", "DIGEST-MD5"} {
		for _, security := range []string{"none", "auto"} {
			relay := startFakeRelayWithAuth(t, mechanisms)
			config := relayConfig(relay)
			config.Security, config.Username, config.Password = security, "svc", "s3cret-pw"
			err := Deliver(context.Background(), config, Message{To: "ops@example.test", Subject: "x", Body: "y"})
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "TLS 없이는 인증하지 않습니다") {
				t.Fatalf("AUTH %s / security=%s: Deliver = %v, want ErrInvalid explaining the TLS policy", mechanisms, security, err)
			}
			if lines := relay.authLines(); len(lines) != 0 {
				t.Fatalf("AUTH %s / security=%s: relay received %q over plaintext", mechanisms, security, lines)
			}
			if from, _, _ := relay.received(); from != "" {
				t.Fatalf("AUTH %s / security=%s: mail went out unauthenticated: from=%q", mechanisms, security, from)
			}
		}
	}
	// A challenge-response mechanism never puts the password on the wire, so it
	// remains usable on a plaintext relay.
	relay := startFakeRelayWithAuth(t, "CRAM-MD5")
	config := relayConfig(relay)
	config.Security, config.Username, config.Password = "none", "svc", "s3cret-pw"
	if err := Deliver(context.Background(), config, Message{To: "ops@example.test", Subject: "x", Body: "y"}); err != nil {
		t.Fatalf("CRAM-MD5 over plaintext: %v", err)
	}
	if lines := relay.authLines(); len(lines) != 1 || !strings.HasPrefix(strings.ToUpper(lines[0]), "AUTH CRAM-MD5") {
		t.Fatalf("AUTH lines = %q, want one CRAM-MD5", lines)
	}
	// An empty username keeps the old behaviour: no AUTH, mail goes out.
	relay = startFakeRelayWithAuth(t, "LOGIN")
	config = relayConfig(relay)
	config.Security = "none"
	if err := Deliver(context.Background(), config, Message{To: "ops@example.test", Subject: "x", Body: "y"}); err != nil {
		t.Fatalf("no username over plaintext: %v", err)
	}
	if lines := relay.authLines(); len(lines) != 0 {
		t.Fatalf("AUTH lines without username = %q", lines)
	}
}

// selfSignedTLS is a throwaway certificate for the fake relay; the client
// skips verification, which is the documented option for private relays.
func selfSignedTLS(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
}

// Over TLS, whether negotiated with STARTTLS or implicit on the port, the same
// credentials do authenticate with whatever the relay advertises.
func TestDeliverAuthenticatesOverTLS(t *testing.T) {
	for _, tc := range []struct{ security, mechanisms, want string }{
		{"starttls", "LOGIN", "AUTH LOGIN"},
		{"auto", "PLAIN LOGIN", "AUTH PLAIN"},
		{"tls", "LOGIN PLAIN", "AUTH PLAIN"},
		{"tls", "LOGIN", "AUTH LOGIN"},
	} {
		relay := startFakeRelayWithAuth(t, tc.mechanisms)
		relay.tlsConfig, relay.implicitTLS = selfSignedTLS(t), tc.security == "tls"
		config := relayConfig(relay)
		config.Security, config.SkipVerify, config.Username, config.Password = tc.security, true, "svc", "s3cret-pw"
		if err := Deliver(context.Background(), config, Message{To: "ops@example.test", Subject: "x", Body: "y"}); err != nil {
			t.Fatalf("security=%s AUTH %s: Deliver = %v", tc.security, tc.mechanisms, err)
		}
		lines := relay.authLines()
		if len(lines) != 1 || !strings.HasPrefix(strings.ToUpper(lines[0]), tc.want) {
			t.Fatalf("security=%s AUTH %s: relay saw %q, want %s", tc.security, tc.mechanisms, lines, tc.want)
		}
		if from, to, _ := relay.received(); from != "dataworks@example.test" || len(to) != 1 {
			t.Fatalf("security=%s: envelope = %q -> %v", tc.security, from, to)
		}
	}
}

func TestLoginAuthStartsOnlyOverTLS(t *testing.T) {
	auth := loginAuth{username: "svc", password: "s3cret-pw"}
	if _, _, err := auth.Start(&smtp.ServerInfo{Name: "relay.internal", TLS: false, Auth: []string{"LOGIN"}}); err == nil {
		t.Fatal("LOGIN must refuse a plaintext connection even when the server name matches")
	}
	if mechanism, _, err := auth.Start(&smtp.ServerInfo{Name: "relay.internal", TLS: true, Auth: []string{"LOGIN"}}); err != nil || mechanism != "LOGIN" {
		t.Fatalf("Start over TLS = %q, %v", mechanism, err)
	}
}

func TestDeliverReportsDeadRelayAndBadConfig(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := listener.Addr().(*net.TCPAddr)
	_ = listener.Close() // nothing listens here any more
	config := Config{Enabled: true, Host: addr.IP.String(), Port: addr.Port, Security: "auto", FromAddress: "a@b.test", Timeout: time.Second}
	if err := Deliver(context.Background(), config, Message{To: "x@y.test"}); err == nil {
		t.Fatal("expected a connection error")
	}
	for _, broken := range []Config{
		{Port: 25, Security: "auto", FromAddress: "a@b.test"},
		{Host: "h", Port: 0, Security: "auto", FromAddress: "a@b.test"},
		{Host: "h", Port: 25, Security: "auto", FromAddress: "nope"},
		{Host: "h", Port: 25, Security: "ssl", FromAddress: "a@b.test"},
	} {
		if err := broken.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Validate(%+v) = %v, want ErrInvalid", broken, err)
		}
	}
}

func TestReadConfigDefaultsMatchAnInternalRelay(t *testing.T) {
	config := ReadConfig(map[string]string{})
	if config.Enabled || config.Port != 25 || config.Security != "auto" || config.Timeout != 10*time.Second || config.FromName != DefaultFromName {
		t.Fatalf("defaults = %+v", config)
	}
	if !config.Allows(EventAlertFired) {
		t.Fatal("unknown/unspecified events must be allowed")
	}
	config = ReadConfig(map[string]string{
		"mail.enabled": "true", "mail.smtp_host": "smtp.internal", "mail.smtp_port": "465", "mail.timeout_seconds": "3",
		"mail.notify_alert": "false", "mail.base_url": "https://dw.internal/", "mail.password": "s3cret",
	})
	if !config.Enabled || config.Security != "tls" || config.Timeout != 3*time.Second || config.FromAddress != "dataworks@smtp.internal" {
		t.Fatalf("config = %+v", config)
	}
	if config.Allows(EventAlertFired) || !config.Allows(EventApprovalRequested) {
		t.Fatal("event switch must silence only its own event")
	}
	if config.Link("/admin") != "https://dw.internal/admin" || (Config{}).Link("/admin") != "" {
		t.Fatalf("Link = %q", config.Link("/admin"))
	}
	if config.Problem() != "" {
		t.Fatalf("Problem = %q", config.Problem())
	}
	if p := ReadConfig(map[string]string{"mail.enabled": "true"}).Problem(); !strings.Contains(p, "mail.smtp_host") {
		t.Fatalf("Problem without host = %q", p)
	}
}

// memLedger behaves like the SQL store: a context that is already done makes
// the statement fail instead of being ignored.
type memLedger struct {
	mu   sync.Mutex
	rows map[string]Delivery
}

func (m *memLedger) InsertMailDelivery(ctx context.Context, d Delivery) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]Delivery{}
	}
	m.rows[d.ID] = d
	return nil
}

func (m *memLedger) CompleteMailDelivery(ctx context.Context, id, status string, attempts int, errorMessage string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.rows[id]
	d.Status, d.Attempts, d.ErrorMessage, d.UpdatedAt = status, attempts, errorMessage, at
	m.rows[id] = d
	return nil
}

func (m *memLedger) byStatus() map[string][]Delivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string][]Delivery{}
	for _, d := range m.rows {
		out[d.Status] = append(out[d.Status], d)
	}
	return out
}

type memDirectory map[string]string

func (m memDirectory) LookupEmails(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if email, ok := m[strings.ToLower(id)]; ok {
			out[strings.ToLower(id)] = email
		}
	}
	return out, nil
}

type sentLog struct {
	mu   sync.Mutex
	sent []Message
	fail func(Message) error
}

func (l *sentLog) send(_ context.Context, _ Config, m Message) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fail != nil {
		if err := l.fail(m); err != nil {
			return err
		}
	}
	l.sent = append(l.sent, m)
	return nil
}

func (l *sentLog) recipients() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []string{}
	for _, m := range l.sent {
		out = append(out, m.To)
	}
	return out
}

func newTestService(config Config, ledger *memLedger, directory memDirectory) (*Service, *sentLog) {
	log := &sentLog{}
	service := NewService(func() Config { return config }, ledger, directory, nil)
	service.SetSender(log.send)
	return service, log
}

func TestNotifySkipsActorDuplicatesAndUnknownAccounts(t *testing.T) {
	directory := memDirectory{"u_alice": "alice@example.test", "u_bob": "bob@example.test", "u_bob2": "BOB@example.test"}
	config := Config{Enabled: true, Host: "h", Port: 25, Security: "none", FromAddress: "a@b.test", Events: map[string]bool{}}
	ledger := &memLedger{}
	service, log := newTestService(config, ledger, directory)
	notification := Notification{Event: EventChangeSetSubmit, Subject: "검토 요청"}
	service.Notify(context.Background(), notification, "u_alice", []string{"u_alice", "u_bob", "u_bob2", "u_ghost", "", "carol@example.test"})
	service.Wait()
	got := log.recipients()
	sort.Strings(got)
	if len(got) != 2 || got[0] != "bob@example.test" || got[1] != "carol@example.test" {
		t.Fatalf("recipients = %v (actor excluded, duplicates collapsed, unknown ids dropped, bare addresses kept)", got)
	}
	if rows := ledger.byStatus(); len(rows["sent"]) != 2 || len(rows["queued"]) != 0 {
		t.Fatalf("ledger = %+v", rows)
	}
}

func TestNotifyDoesNothingWhenOffIncompleteOrSwitchedOff(t *testing.T) {
	directory := memDirectory{"u_bob": "bob@example.test"}
	for name, config := range map[string]Config{
		"disabled":     {Enabled: false, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test"},
		"no host":      {Enabled: true, Port: 25, Security: "auto", FromAddress: "a@b.test"},
		"event is off": {Enabled: true, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test", Events: map[string]bool{EventAlertFired: false}},
	} {
		ledger := &memLedger{}
		service, log := newTestService(config, ledger, directory)
		service.Notify(context.Background(), Notification{Event: EventAlertFired, Subject: "x"}, "", []string{"u_bob"})
		service.Wait()
		if len(log.recipients()) != 0 || len(ledger.byStatus()) != 0 {
			t.Fatalf("%s: mail went out or was recorded: %v %+v", name, log.recipients(), ledger.byStatus())
		}
	}
	// A switched-off event must not silence the others.
	config := Config{Enabled: true, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test", Events: map[string]bool{EventAlertFired: false}}
	service, log := newTestService(config, &memLedger{}, directory)
	service.Notify(context.Background(), Notification{Event: EventApprovalRequested, Subject: "x"}, "", []string{"u_bob"})
	service.Wait()
	if len(log.recipients()) != 1 {
		t.Fatalf("other events must still go out: %v", log.recipients())
	}
	if err := service.SendNow(context.Background(), TestMessage(), "", "bob@example.test"); err != nil {
		t.Fatalf("SendNow: %v", err)
	}
	off, _ := newTestService(Config{}, &memLedger{}, directory)
	if err := off.SendNow(context.Background(), TestMessage(), "", "bob@example.test"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("SendNow while disabled = %v", err)
	}
}

func TestNotifyCapsDeliveriesInFlightAndRecordsTheOverflow(t *testing.T) {
	directory := memDirectory{}
	recipients := make([]string, 0, MaxInFlight+3)
	for i := 0; i < MaxInFlight+3; i++ {
		recipients = append(recipients, "user"+strconv.Itoa(i)+"@example.test")
	}
	config := Config{Enabled: true, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test", Timeout: time.Second}
	ledger := &memLedger{}
	service, log := newTestService(config, ledger, directory)
	// Every send hangs like a relay that accepted the connection and went quiet,
	// until the test lets go.
	release := make(chan struct{})
	log.fail = func(Message) error { <-release; return nil }
	started := time.Now()
	service.Notify(context.Background(), Notification{Event: EventAlertFired, Subject: "alert"}, "", recipients)
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("Notify blocked the caller")
	}
	rows := ledger.byStatus()
	if len(rows["queued"]) != MaxInFlight || len(rows["failed"]) != 3 {
		t.Fatalf("ledger = queued %d, failed %d (want %d in flight and the rest refused)", len(rows["queued"]), len(rows["failed"]), MaxInFlight)
	}
	for _, row := range rows["failed"] {
		if row.Attempts != 0 || !strings.Contains(row.ErrorMessage, ErrBusy.Error()) {
			t.Fatalf("overflow row = %+v", row)
		}
	}
	close(release)
	service.Wait()
	if rows := ledger.byStatus(); len(rows["sent"]) != MaxInFlight || len(rows["queued"]) != 0 {
		t.Fatalf("after release ledger = %+v", rows)
	}
	// Slots are handed back: the next message goes out normally.
	service.Notify(context.Background(), Notification{Event: EventAlertFired, Subject: "again"}, "", []string{"late@example.test"})
	service.Wait()
	if got := log.recipients(); got[len(got)-1] != "late@example.test" {
		t.Fatalf("recipients = %v", got)
	}
}

// A send that outlives its budget (the relay accepted the connection and went
// quiet, and only the connection deadline ends the session) must still be
// recorded: the ledger write cannot share the expired sending context, or the
// row stays "queued" forever.
func TestDeliveryOutcomeIsRecordedAfterTheSendBudgetExpires(t *testing.T) {
	directory := memDirectory{}
	config := Config{Enabled: true, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test", Timeout: time.Millisecond}
	ledger := &memLedger{}
	service, _ := newTestService(config, ledger, directory)
	service.SetSender(func(ctx context.Context, _ Config, _ Message) error {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		return errors.New("relay went quiet")
	})
	service.Notify(context.Background(), Notification{Event: EventAlertFired, Subject: "alert"}, "", []string{"slow@example.test"})
	service.Wait()
	rows := ledger.byStatus()
	if len(rows["failed"]) != 1 || len(rows["queued"]) != 0 {
		t.Fatalf("ledger after background delivery = %+v (want the row marked failed, not left queued)", rows)
	}
	if row := rows["failed"][0]; row.Attempts != 2 || !strings.Contains(row.ErrorMessage, "relay went quiet") {
		t.Fatalf("failed row = %+v", row)
	}
	ledger = &memLedger{}
	service, _ = newTestService(config, ledger, directory)
	service.SetSender(func(ctx context.Context, _ Config, _ Message) error {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		return errors.New("relay went quiet")
	})
	if err := service.SendNow(context.Background(), TestMessage(), "", "slow@example.test"); err == nil {
		t.Fatal("SendNow must report the failure")
	}
	rows = ledger.byStatus()
	if len(rows["failed"]) != 1 || len(rows["queued"]) != 0 {
		t.Fatalf("ledger after test send = %+v (want the row marked failed, not left queued)", rows)
	}
}

// The sending context bounds the whole session, not only the dial: a relay
// that answers EHLO and then falls silent must not hold Deliver past it.
func TestDeliverStopsWhenTheContextExpiresMidSession(t *testing.T) {
	relay := startFakeRelay(t)
	relay.silentOnMail = true
	config := relayConfig(relay) // connection deadline 3s, far beyond the context
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := Deliver(ctx, config, Message{To: "ops@example.test", Subject: "x", Body: "y"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Deliver = %v, want the context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("Deliver held the session for %v after the context expired", elapsed)
	}
}

func TestNotifyRecordsFailuresWithoutFailingTheCaller(t *testing.T) {
	directory := memDirectory{"u_bob": "bob@example.test", "u_eve": "eve@example.test"}
	config := Config{Enabled: true, Host: "h", Port: 25, Security: "auto", FromAddress: "a@b.test", Timeout: time.Millisecond}
	ledger := &memLedger{}
	service, log := newTestService(config, ledger, directory)
	log.fail = func(m Message) error {
		if m.To == "eve@example.test" {
			return errors.New("relay refused")
		}
		return nil
	}
	started := time.Now()
	service.Notify(context.Background(), Notification{Event: EventAlertFired, Subject: "alert"}, "", []string{"u_bob", "u_eve"})
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("Notify blocked the caller")
	}
	service.Wait()
	rows := ledger.byStatus()
	if len(rows["sent"]) != 1 || len(rows["failed"]) != 1 {
		t.Fatalf("ledger = %+v", rows)
	}
	failed := rows["failed"][0]
	if failed.Recipient != "eve@example.test" || failed.Attempts != 2 || !strings.Contains(failed.ErrorMessage, "relay refused") {
		t.Fatalf("failed row = %+v (want two attempts and the reason)", failed)
	}
}
