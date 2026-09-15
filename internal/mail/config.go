package mail

import (
	"strconv"
	"strings"
	"time"
)

// SettingPrefix groups every mail key in the admin settings registry.
const SettingPrefix = "mail."

// Defaults aim at the common case: an internal relay on port 25 that accepts
// mail from the network without credentials.
const (
	DefaultPort     = 25
	DefaultSecurity = "auto"
	DefaultFromName = "Data Works"
	DefaultTimeout  = 10
	defaultTimeout  = DefaultTimeout * time.Second
)

// EventSettings maps each event to the switch that turns it off. Two events
// share a switch when they belong to one conversation (a request and its
// decision), so an administrator never silences half of it.
var EventSettings = map[string]string{
	EventApprovalRequested: "mail.notify_approval",
	EventApprovalDecided:   "mail.notify_approval",
	EventChangeSetSubmit:   "mail.notify_change_set",
	EventAlertFired:        "mail.notify_alert",
	EventAccessExpiring:    "mail.notify_expiring",
}

// ReadConfig builds the configuration from effective setting values, keyed by
// the full setting name. Missing or blank keys fall back to the defaults.
func ReadConfig(values map[string]string) Config {
	config := Config{Port: DefaultPort, Security: DefaultSecurity, Timeout: defaultTimeout, Events: map[string]bool{}}
	config.Enabled = boolValue(values, "mail.enabled")
	config.Host = stringValue(values, "mail.smtp_host", "")
	config.Username = stringValue(values, "mail.username", "")
	config.Password = values["mail.password"]
	config.FromAddress = stringValue(values, "mail.from_address", "")
	config.FromName = stringValue(values, "mail.from_name", DefaultFromName)
	config.Security = strings.ToLower(stringValue(values, "mail.security", DefaultSecurity))
	config.BaseURL = strings.TrimRight(stringValue(values, "mail.base_url", ""), "/")
	config.SkipVerify = boolValue(values, "mail.skip_tls_verify")
	if port, ok := numberValue(values, "mail.smtp_port"); ok && port > 0 {
		config.Port = port
	}
	if seconds, ok := numberValue(values, "mail.timeout_seconds"); ok && seconds > 0 {
		config.Timeout = time.Duration(seconds) * time.Second
	}
	// A relay on the implicit TLS port needs no extra configuration.
	if config.Security == DefaultSecurity && config.Port == 465 {
		config.Security = "tls"
	}
	for event, key := range EventSettings {
		if raw, ok := values[key]; ok && strings.TrimSpace(raw) != "" {
			config.Events[event] = boolValue(values, key)
		}
	}
	if strings.TrimSpace(config.FromAddress) == "" && strings.TrimSpace(config.Host) != "" {
		config.FromAddress = "dataworks@" + config.Host
	}
	return config
}

// Problem explains why mail cannot go out, or returns "" when it can. It is what
// the settings screen shows next to the on/off state.
func (c Config) Problem() string {
	if !c.Enabled {
		return ""
	}
	if err := c.Validate(); err != nil {
		return strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
	}
	return ""
}

func stringValue(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return fallback
}

func boolValue(values map[string]string, key string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(values[key]))
	return err == nil && parsed
}

func numberValue(values map[string]string, key string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(values[key]))
	if err != nil {
		return 0, false
	}
	return parsed, true
}
