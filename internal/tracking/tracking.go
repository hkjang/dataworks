// Package tracking injects an administrator-configured visitor tracking snippet
// into the pages Data Works serves.
//
// Pasting a <script> tag is the easy half. The served pages carry a content
// security policy that allows scripts from the application origin only, so a
// snippet added without help is blocked silently and the administrator has no
// way to tell why the dashboard stays empty. This package therefore produces
// both halves: the markup to inject, with a per-request nonce on every script
// tag, and the policy sources that markup needs, read out of the snippet itself.
//
// Momento comes first because it is the self-hosted collector: with the
// same-origin proxy no visitor data and no policy source ever leaves the
// installation.
package tracking

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

const (
	ProviderNone    = "none"
	ProviderMomento = "momento"
	ProviderGA4     = "ga4"
	ProviderGTM     = "gtm"
	ProviderMatomo  = "matomo"
	ProviderCustom  = "custom"

	PlacementHead = "head"
	PlacementBody = "body"

	// MaxSnippetBytes bounds a pasted snippet. Real loaders are a few hundred
	// bytes; anything larger is a mistake or an attempt to store something else.
	MaxSnippetBytes = 8 * 1024

	// MomentoProxyPath is the same-origin prefix the server forwards to the
	// Momento collector, so the snippet never names an external origin.
	MomentoProxyPath = "/momento"

	// SettingPrefix is the admin-settings namespace every key below lives in.
	SettingPrefix = "tracking."
)

// Providers lists the supported providers in the order administrators see
// them. Momento is first on purpose: it is the only choice that keeps data
// inside the network.
var Providers = []string{ProviderNone, ProviderMomento, ProviderGA4, ProviderGTM, ProviderMatomo, ProviderCustom}

// Config is the administrator's tracking configuration. Everything is off
// unless an administrator turns it on, so a fresh installation serves exactly
// the pages it served before this package existed.
type Config struct {
	Enabled            bool   `json:"enabled"`
	Provider           string `json:"provider"`
	MomentoURL         string `json:"momento_url,omitempty"`
	MomentoSiteID      string `json:"momento_site_id,omitempty"`
	MomentoProxy       bool   `json:"momento_proxy"`
	MomentoEnvironment string `json:"momento_environment,omitempty"`
	MeasurementID      string `json:"measurement_id,omitempty"`
	MatomoURL          string `json:"matomo_url,omitempty"`
	MatomoSiteID       string `json:"matomo_site_id,omitempty"`
	CustomSnippet      string `json:"custom_snippet,omitempty"`
	AllowedHosts       string `json:"allowed_hosts,omitempty"`
	IncludeAdmin       bool   `json:"include_admin"`
	Placement          string `json:"placement"`
}

// ReadConfig maps stored settings (string values keyed by the tracking.* keys)
// onto the configuration. Missing or malformed values fall back to "off".
func ReadConfig(values map[string]string) Config {
	get := func(key, fallback string) string {
		if value, ok := values[SettingPrefix+key]; ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	getBool := func(key string, fallback bool) bool {
		parsed, err := strconv.ParseBool(get(key, strconv.FormatBool(fallback)))
		if err != nil {
			return fallback
		}
		return parsed
	}
	config := Config{
		Enabled:            getBool("enabled", false),
		Provider:           strings.ToLower(get("provider", ProviderNone)),
		MomentoURL:         get("momento_url", ""),
		MomentoSiteID:      get("momento_site_id", ""),
		MomentoProxy:       getBool("momento_proxy", true),
		MomentoEnvironment: get("momento_environment", "prd"),
		MeasurementID:      get("measurement_id", ""),
		MatomoURL:          get("matomo_url", ""),
		MatomoSiteID:       get("matomo_site_id", ""),
		CustomSnippet:      get("custom_snippet", ""),
		AllowedHosts:       get("allowed_hosts", ""),
		IncludeAdmin:       getBool("include_admin", false),
		Placement:          strings.ToLower(get("placement", PlacementHead)),
	}
	if config.Placement != PlacementBody {
		config.Placement = PlacementHead
	}
	return config
}

// Active reports whether a page should carry the snippet. Administrative
// pages are excluded unless an administrator asks for them, because console
// traffic is rarely the visitor data anybody wants to measure.
func (c Config) Active(adminPage bool) bool {
	if !c.Enabled || c.Provider == ProviderNone || c.Provider == "" {
		return false
	}
	if adminPage && !c.IncludeAdmin {
		return false
	}
	return strings.TrimSpace(c.Snippet("")) != ""
}

// Validate reports what is missing for the chosen provider. It only complains
// when tracking is on, so a half-filled configuration can be saved key by key.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch c.Provider {
	case ProviderNone, "":
		return nil
	case ProviderMomento:
		if strings.TrimSpace(c.MomentoURL) == "" || strings.TrimSpace(c.MomentoSiteID) == "" {
			return fmt.Errorf("tracking.momento_url과 tracking.momento_site_id가 필요합니다")
		}
		if originOf(c.MomentoURL) == "" {
			return fmt.Errorf("tracking.momento_url이 올바른 주소가 아닙니다")
		}
	case ProviderGA4, ProviderGTM:
		if strings.TrimSpace(c.MeasurementID) == "" {
			return fmt.Errorf("tracking.measurement_id가 필요합니다")
		}
	case ProviderMatomo:
		if strings.TrimSpace(c.MatomoURL) == "" || strings.TrimSpace(c.MatomoSiteID) == "" {
			return fmt.Errorf("tracking.matomo_url과 tracking.matomo_site_id가 필요합니다")
		}
		if originOf(c.MatomoURL) == "" {
			return fmt.Errorf("tracking.matomo_url이 올바른 주소가 아닙니다")
		}
	case ProviderCustom:
		if strings.TrimSpace(c.CustomSnippet) == "" {
			return fmt.Errorf("tracking.custom_snippet이 비어 있습니다")
		}
		if len(c.CustomSnippet) > MaxSnippetBytes {
			return fmt.Errorf("추적 코드는 %d바이트를 넘을 수 없습니다", MaxSnippetBytes)
		}
	default:
		return fmt.Errorf("tracking.provider는 %s 중 하나여야 합니다", strings.Join(Providers, ", "))
	}
	return nil
}

// ValidProvider reports whether name is one of Providers.
func ValidProvider(name string) bool {
	for _, provider := range Providers {
		if provider == strings.ToLower(strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// UsesMomentoProxy reports whether the same-origin proxy is the way the
// snippet reaches the collector.
func (c Config) UsesMomentoProxy() bool {
	return c.Provider == ProviderMomento && c.MomentoProxy && originOf(c.MomentoURL) != ""
}

// MomentoTarget returns the collector the same-origin proxy forwards to, or
// nil when the proxy must stay closed. The proxy is only open while tracking
// is on so a disabled configuration leaves no open door to an internal host.
func (c Config) MomentoTarget() *url.URL {
	if !c.Enabled || !c.UsesMomentoProxy() {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(c.MomentoURL))
	if err != nil || parsed.Host == "" {
		return nil
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed
}

// Snippet renders the markup to inject. The nonce is applied to every script
// tag so the policy can stay strict.
func (c Config) Snippet(nonce string) string {
	switch c.Provider {
	case ProviderMomento:
		site := html.EscapeString(strings.TrimSpace(c.MomentoSiteID))
		base := strings.TrimRight(strings.TrimSpace(c.MomentoURL), "/")
		if site == "" || originOf(base) == "" {
			return ""
		}
		environment := html.EscapeString(strings.TrimSpace(c.MomentoEnvironment))
		if environment == "" {
			environment = "prd"
		}
		if c.MomentoProxy {
			// The tracker is fetched and reports through this origin, so the
			// policy never has to name the collector.
			return WithNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-endpoint="%s" data-environment="%s" data-contract-version="1"></script>`,
				MomentoProxyPath, site, MomentoProxyPath, environment), nonce)
		}
		return WithNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="%s" data-contract-version="1"></script>`,
			html.EscapeString(base), site, environment), nonce)
	case ProviderGA4:
		id := html.EscapeString(strings.TrimSpace(c.MeasurementID))
		if id == "" {
			return ""
		}
		return WithNonce(fmt.Sprintf(`<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','%s');</script>`, id, id), nonce)
	case ProviderGTM:
		id := html.EscapeString(strings.TrimSpace(c.MeasurementID))
		if id == "" {
			return ""
		}
		return WithNonce(fmt.Sprintf(`<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','%s');</script>`, id), nonce)
	case ProviderMatomo:
		base := strings.TrimRight(strings.TrimSpace(c.MatomoURL), "/")
		site := html.EscapeString(strings.TrimSpace(c.MatomoSiteID))
		if originOf(base) == "" || site == "" {
			return ""
		}
		return WithNonce(fmt.Sprintf(`<script>var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);(function(){var u="%s/";_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId','%s']);var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();</script>`, html.EscapeString(base), site), nonce)
	case ProviderCustom:
		snippet := strings.TrimSpace(c.CustomSnippet)
		if len(snippet) > MaxSnippetBytes {
			return ""
		}
		return WithNonce(snippet, nonce)
	}
	return ""
}

// WithNonce adds the nonce to every script tag that does not already carry
// one, which is what lets a pasted snippet run under a strict policy unchanged.
func WithNonce(snippet, nonce string) string {
	if nonce == "" || snippet == "" {
		return snippet
	}
	var builder strings.Builder
	remaining := snippet
	for {
		index := strings.Index(strings.ToLower(remaining), "<script")
		if index < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		end := index + len("<script")
		builder.WriteString(remaining[:end])
		tag := remaining[end:]
		if closing := strings.Index(tag, ">"); closing >= 0 {
			tag = tag[:closing]
		}
		if !strings.Contains(strings.ToLower(tag), "nonce=") {
			builder.WriteString(` nonce="` + html.EscapeString(nonce) + `"`)
		}
		remaining = remaining[end:]
	}
}

// NewNonce returns a fresh base64 nonce for one response.
func NewNonce() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic("tracking: crypto/rand unavailable: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(buffer)
}

// Inject places the snippet just before the closing tag its placement names,
// falling back to the end of the document when the tag is missing.
func Inject(page []byte, snippet, placement string) []byte {
	if strings.TrimSpace(snippet) == "" {
		return page
	}
	marker := "</head>"
	if placement == PlacementBody {
		marker = "</body>"
	}
	text := string(page)
	index := strings.LastIndex(strings.ToLower(text), marker)
	if index < 0 {
		return []byte(text + "\n" + snippet + "\n")
	}
	return []byte(text[:index] + snippet + "\n" + text[index:])
}

// PolicySources lists the extra origins the snippet needs, derived from the
// provider so a common setup needs no policy knowledge at all. A Momento
// snippet going through the same-origin proxy needs nothing.
func (c Config) PolicySources() (scripts []string, connects []string, images []string) {
	add := func(origin string) {
		scripts = append(scripts, origin)
		connects = append(connects, origin)
		images = append(images, origin)
	}
	switch c.Provider {
	case ProviderMomento:
		if !c.MomentoProxy {
			if origin := originOf(c.MomentoURL); origin != "" {
				add(origin)
			}
		}
	case ProviderGA4, ProviderGTM:
		scripts = append(scripts, "https://www.googletagmanager.com")
		connects = append(connects, "https://www.google-analytics.com", "https://analytics.google.com", "https://*.google-analytics.com")
		images = append(images, "https://www.google-analytics.com", "https://www.googletagmanager.com")
	case ProviderMatomo:
		if origin := originOf(c.MatomoURL); origin != "" {
			add(origin)
		}
	case ProviderCustom:
		// A pasted snippet names the addresses it loads and reports to, so
		// those origins are allowed without anybody reading a policy error.
		for _, origin := range SnippetOrigins(c.CustomSnippet) {
			add(origin)
		}
	}
	for _, host := range SplitHosts(c.AllowedHosts) {
		add(host)
	}
	return scripts, connects, images
}

// SplitHosts breaks the comma, space or newline separated allow list into its
// entries.
func SplitHosts(list string) []string {
	var hosts []string
	for _, host := range strings.FieldsFunc(list, func(letter rune) bool {
		return letter == ',' || letter == ' ' || letter == '\n' || letter == '\r' || letter == '\t'
	}) {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}

// AddAllowedHost appends an origin to the allow list, leaving the existing
// entries and their order alone.
func AddAllowedHost(existing, origin string) string {
	origin = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
	if origin == "" {
		return existing
	}
	for _, host := range SplitHosts(existing) {
		if strings.EqualFold(host, origin) {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return origin
	}
	return strings.TrimSpace(existing) + ", " + origin
}

// SnippetOrigins lists every http(s) origin written into a tracking snippet:
// the script it loads, the endpoint it posts to, the pixel it requests. A
// tracker almost always writes its own address somewhere in its loader, so
// reading them here is what keeps a pasted snippet working without the
// administrator translating a policy error into a host name.
func SnippetOrigins(snippet string) []string {
	origins := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	lower := strings.ToLower(snippet)
	for index := 0; index < len(snippet); {
		start := strings.Index(lower[index:], "http")
		if start < 0 {
			break
		}
		start += index
		end := start
		for end < len(snippet) && !isURLBoundary(snippet[end]) {
			end++
		}
		index = end
		origin := originOf(snippet[start:end])
		if origin == "" || !strings.HasPrefix(strings.ToLower(origin), "http") {
			continue
		}
		if _, duplicate := seen[origin]; duplicate {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

// isURLBoundary reports the characters that cannot appear in a URL written
// inside HTML or JavaScript, which is where each address ends.
func isURLBoundary(letter byte) bool {
	switch letter {
	case '"', '\'', '`', '<', '>', ' ', '\t', '\n', '\r', ')', ',', ';', '\\', '+':
		return true
	}
	return false
}

// originOf reduces a URL to scheme://host, which is the unit a policy allows.
// Only http(s) addresses have an origin worth allowing.
func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}
