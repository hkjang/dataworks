package proxy

import (
	"net/url"
	"strings"
)

// Silent SSO (OIDC prompt=none).
//
// A visitor who already holds a Keycloak session should land on the workbench without seeing
// the login screen. prompt=none asks the provider to answer from an existing session only: it
// never renders UI, so the answer is either an authorization code (→ ordinary login) or
// error=login_required (→ "no session", which is a normal outcome, not a failure).
//
// The whole point of this file is preventing a redirect loop. The browser owns the first two
// guards (a once-per-tab-session flag and a signed-out flag, see web/src/features/auth/
// silent-sso.ts); the server owns the rest: prompt=none is honoured only while the
// administrator has switched auto_login on, and a refusal lands on the login screen with the
// ?sso=none marker in the address so the browser will not retry even if its storage was wiped.

// ssoRefusalMarker is the query parameter the callback appends to the login screen address
// after a silent attempt was refused. The SPA reads it and does not try again.
const ssoRefusalMarker = "sso=none"

// dataWorksSSODefaultReturn is where a silent refusal lands when no usable return_to was
// carried. The SPA has no dedicated /login route: the login screen is rendered in place at
// any workbench path, so the marker is attached to the deep link itself.
const dataWorksSSODefaultReturn = "/dataworks/"

// safeReturnTo accepts only same-origin in-app paths for return_to: it must start with a
// single "/" (so "//evil" and "https://evil" are out), parse as a relative reference without a
// host, and carry no CR/LF. Anything else is dropped so this login flow cannot be used as an
// open redirect.
func safeReturnTo(value string) bool {
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		strings.HasPrefix(value, "/\\") || strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil {
		return false
	}
	// The callback places the session tokens in the URL fragment, which only the browser
	// pages consume. Landing on an API path would leave tokens sitting in the address bar of
	// a JSON response, so return_to is limited to the two browser entry points.
	p := parsed.Path
	return p == "/dataworks" || strings.HasPrefix(p, "/dataworks/") || p == "/admin" || strings.HasPrefix(p, "/admin/")
}

// normalizeReturnTo returns value when it is a safe in-app path, otherwise "".
func normalizeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if safeReturnTo(value) {
		return value
	}
	return ""
}

// silentRefusalLocation builds the address the browser is sent to when a prompt=none attempt
// came back with login_required: the deep link it started from (or the workbench root) with
// the ?sso=none marker merged into the query string.
func silentRefusalLocation(returnTo string) string {
	target := normalizeReturnTo(returnTo)
	if target == "" {
		target = dataWorksSSODefaultReturn
	}
	parsed, err := url.Parse(target)
	if err != nil {
		parsed = &url.URL{Path: dataWorksSSODefaultReturn}
	}
	q := parsed.Query()
	q.Set("sso", "none")
	parsed.RawQuery = q.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

// silentRefusalError reports whether an OIDC error code is one of the "no interactive
// session" answers a prompt=none request can legitimately produce.
func silentRefusalError(code string) bool {
	switch strings.TrimSpace(code) {
	case "login_required", "interaction_required", "consent_required", "account_selection_required":
		return true
	}
	return false
}
