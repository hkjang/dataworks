package mail

import (
	"fmt"
	"mime"
	"strings"
	"time"
)

// Notification is one event rendered for people: a subject, a few lines that
// say what happened, and where to go. Bodies are plain text so any client
// shows them the same way.
type Notification struct {
	Event   string
	Subject string
	Lines   []string
	Path    string // app path the mail links to; joined with mail.base_url
	Ref     string // what the mail is about (approval id, change set id, rule id) — kept in the record
}

// Render produces the body. The link is omitted when no base URL is configured
// rather than pointing at a host that does not exist.
func (n Notification) Render(config Config) string {
	var b strings.Builder
	for _, line := range n.Lines {
		b.WriteString(line)
		b.WriteString("\r\n")
	}
	if link := config.Link(n.Path); link != "" {
		b.WriteString("\r\n")
		b.WriteString(link)
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n-- \r\n이 메일은 ")
	b.WriteString(strings.TrimSpace(config.FromName))
	b.WriteString(" 알림입니다. 받지 않으려면 관리자에게 알려 주세요.\r\n")
	return b.String()
}

// Link joins an app path to the configured base URL.
func (c Config) Link(path string) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

// TestMessage is what the settings screen sends to prove the relay works.
func TestMessage() Notification {
	return Notification{
		Event:   EventTest,
		Subject: "[Data Works] 메일 설정 시험",
		Lines:   []string{"이 메일이 도착했다면 SMTP 릴레이 설정이 올바릅니다.", "보낸 시각: " + time.Now().Format(time.RFC3339)},
		Path:    "/dataworks/settings",
	}
}

// compose writes the RFC 5322 message with a UTF-8 safe subject.
func compose(config Config, message Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", config.Address())
	fmt.Fprintf(&b, "To: %s\r\n", strings.TrimSpace(message.To))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", strings.TrimSpace(message.Subject)))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%d.%s>\r\n", time.Now().UnixNano(), helloName(config))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\nAuto-Submitted: auto-generated\r\n\r\n")
	// net/smtp's DATA writer dot-stuffs, so only the line endings are normalised here.
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(message.Body, "\r\n", "\n"), "\n", "\r\n"))
	return b.String()
}
