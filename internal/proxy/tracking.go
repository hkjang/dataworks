package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"

	"dataworks/internal/store"
	"dataworks/internal/tracking"
)

// trackingReportPath is where browsers post the requests the page policy
// refused. It is unauthenticated because the browser sends the report without
// credentials, and it stores nothing but a bounded list of origins in memory.
const trackingReportPath = "/tracking/csp-report"

// maxTrackingReportBytes keeps the unauthenticated report endpoint from being
// used to push large bodies at the server.
const maxTrackingReportBytes = 8 * 1024

// apiPolicy is the policy for everything that is not a page. Nothing here is
// rendered, so nothing may load anything.
const apiPolicy = "default-src 'none'; frame-ancestors 'none'"

// trackingConf returns the effective visitor tracking configuration (admin
// settings overlay). A server without a snapshot, such as one built directly in
// tests, tracks nothing.
func (s *Server) trackingConf() tracking.Config {
	if s == nil {
		return tracking.Config{}
	}
	if p := s.trackRuntime.Load(); p != nil {
		return *p
	}
	return tracking.Config{}
}

// trackingConfigFrom builds the tracking configuration from the stored
// settings, applying the registry defaults for anything unset.
func (s *Server) trackingConfigFrom(stored map[string]store.AdminSetting) tracking.Config {
	values := map[string]string{}
	for _, d := range settingRegistry {
		if !strings.HasPrefix(d.Key, tracking.SettingPrefix) {
			continue
		}
		value, _ := s.effectiveSettingValue(stored, d)
		values[d.Key] = value
	}
	return tracking.ReadConfig(values)
}

// trackingAdminPage reports whether a page path is an administrative screen,
// which the snippet skips unless tracking.include_admin is on. The SPA is one
// document, so the check applies to the URL the browser loaded it from.
func trackingAdminPage(path string) bool {
	return path == "/admin" || path == "/admin/" ||
		strings.HasPrefix(path, dataWorksSPAPrefix+"settings") ||
		strings.HasPrefix(path, dataWorksSPAPrefix+"admin")
}

// pagePolicy keeps the strict page policy and adds only what the configured
// tracking snippet needs, including a nonce for its inline code. Styles allow
// inline declarations because the React components set them; scripts never do.
func pagePolicy(config tracking.Config, adminPage bool, nonce string) string {
	scripts := []string{"'self'"}
	connects := []string{"'self'"}
	images := []string{"'self'", "data:", "blob:"}
	active := config.Active(adminPage)
	if active {
		extraScripts, extraConnects, extraImages := config.PolicySources()
		scripts = append(scripts, "'nonce-"+nonce+"'")
		scripts = append(scripts, extraScripts...)
		connects = append(connects, extraConnects...)
		images = append(images, extraImages...)
	}
	policy := "default-src 'self'; script-src " + strings.Join(scripts, " ") +
		"; style-src 'self' 'unsafe-inline'; img-src " + strings.Join(images, " ") +
		"; font-src 'self' data:; connect-src " + strings.Join(connects, " ") +
		"; object-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'"
	if active {
		// While tracking is on, ask the browser to say what it refused. That
		// report is what turns a console error into a one-click fix.
		policy += "; report-uri " + trackingReportPath
	}
	return policy
}

// decorateSPAPage sets the page policy on the SPA shell and injects the
// tracking snippet when the administrator turned it on for this page.
func (s *Server) decorateSPAPage(w http.ResponseWriter, r *http.Request, page []byte) []byte {
	config := s.trackingConf()
	adminPage := trackingAdminPage(r.URL.Path)
	nonce := tracking.NewNonce()
	w.Header().Set("Content-Security-Policy", pagePolicy(config, adminPage, nonce))
	if !config.Active(adminPage) {
		return page
	}
	return tracking.Inject(page, config.Snippet(nonce), config.Placement)
}

// withAPIPolicy narrows the policy on everything that is not a page. Pages
// (the SPA shell, the legacy console, Swagger, the SSO logout frame) and the
// Momento proxy set or pass through their own headers.
func withAPIPolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasPrefix(path, dataWorksSPAPrefix), path == strings.TrimSuffix(dataWorksSPAPrefix, "/"),
			path == "/admin", path == "/admin/", path == "/swagger", path == "/favicon.ico",
			path == "/auth/keycloak/frontchannel-logout",
			strings.HasPrefix(path, tracking.MomentoProxyPath+"/"):
		default:
			w.Header().Set("Content-Security-Policy", apiPolicy)
		}
		next.ServeHTTP(w, r)
	})
}

type trackingReport struct {
	Report struct {
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		DocumentURI        string `json:"document-uri"`
	} `json:"csp-report"`
}

// handleTrackingReport records what a browser refused to load. Reports are
// always answered with 204 so a misbehaving page never sees an error from us,
// and they are only kept while tracking is on — the policy only names this
// endpoint then, so anything else is noise.
func (s *Server) handleTrackingReport(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if r.Method != http.MethodPost || s.trackReports == nil || !s.trackingConf().Enabled {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxTrackingReportBytes))
	if err != nil || len(body) == 0 {
		return
	}
	var report trackingReport
	if json.Unmarshal(body, &report) != nil {
		return
	}
	directive := report.Report.EffectiveDirective
	if directive == "" {
		directive = report.Report.ViolatedDirective
	}
	s.trackReports.Record(report.Report.BlockedURI, directive, report.Report.DocumentURI)
}

// handleTrackingStatus tells the administrator what the current configuration
// amounts to: whether pages carry the snippet, what is missing if not, and
// which origins the policy allows for it.
func (s *Server) handleTrackingStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	config := s.trackingConf()
	problem := ""
	if err := config.Validate(); err != nil {
		problem = err.Error()
	}
	scripts, connects, images := config.PolicySources()
	proxyTarget := ""
	if target := config.MomentoTarget(); target != nil {
		proxyTarget = target.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         config.Enabled,
		"provider":        config.Provider,
		"providers":       tracking.Providers,
		"placement":       config.Placement,
		"include_admin":   config.IncludeAdmin,
		"active":          config.Active(false),
		"active_admin":    config.Active(true),
		"problem":         problem,
		"momento_proxy":   config.UsesMomentoProxy(),
		"proxy_path":      tracking.MomentoProxyPath,
		"proxy_target":    proxyTarget,
		"script_sources":  nonNilStrings(scripts),
		"connect_sources": nonNilStrings(connects),
		"image_sources":   nonNilStrings(images),
		"report_path":     trackingReportPath,
		"snippet":         config.Snippet(""),
	})
}

func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

// handleTrackingViolations lists (GET) or forgets (DELETE) the origins the
// page policy blocked, so a snippet can be fixed without reading the browser
// console.
func (s *Server) handleTrackingViolations(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"items": s.trackReports.List(s.trackingConf())})
	case http.MethodDelete:
		s.trackReports.Forget()
		s.auditAdmin(r, "tracking.violations.clear", "", "")
		w.WriteHeader(http.StatusNoContent)
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

// handleTrackingAllow adds one blocked origin to tracking.allowed_hosts. It is
// the one-click fix for the reports listed above.
func (s *Server) handleTrackingAllow(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var payload struct {
		Origin string `json:"origin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	origin := strings.TrimSpace(payload.Origin)
	lower := strings.ToLower(origin)
	if origin == "" || !(strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) || strings.ContainsAny(origin, " ;,'\"") {
		writeOpenAIError(w, http.StatusBadRequest, "origin must be an http(s) origin such as https://collector.internal", "invalid_request_error", "invalid_origin")
		return
	}
	def, ok := settingDefByKey(tracking.SettingPrefix + "allowed_hosts")
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "tracking.allowed_hosts is not registered", "server_error", "setting_missing")
		return
	}
	if !s.canWriteSetting(r, def) {
		writeOpenAIError(w, http.StatusForbidden, "your role cannot modify this setting category", "permission_error", "settings_role_denied")
		return
	}
	updated := tracking.AddAllowedHost(s.trackingConf().AllowedHosts, origin)
	if err := s.applySettingWrite(r, def, updated, "tracking: allow blocked origin"); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "setting_invalid")
		return
	}
	s.auditAdmin(r, "setting.update", def.Key, auditJSON(map[string]any{"key": def.Key, "origin": origin}))
	writeJSON(w, http.StatusOK, map[string]any{"key": def.Key, "value": updated, "items": s.trackReports.List(s.trackingConf())})
}

// handleMomentoProxy forwards /momento/* to the configured Momento collector
// so the tracker script and its beacons stay on this origin and the page
// policy never has to name an external host. It is closed unless tracking is
// on with the Momento provider in proxy mode.
func (s *Server) handleMomentoProxy(w http.ResponseWriter, r *http.Request) {
	target := s.trackingConf().MomentoTarget()
	if target == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodOptions {
		w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = target.Path + strings.TrimPrefix(r.URL.Path, tracking.MomentoProxyPath)
			pr.Out.URL.RawPath = ""
			pr.Out.Host = target.Host
			// The collector only needs the request; the visitor's session
			// cookie for this application is none of its business.
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "momento collector unavailable", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}
