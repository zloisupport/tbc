package terabox

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"tbc/internal/util"

	"resty.dev/v3"
)

// ErrCookieExpired is returned when TeraBox accepts the request but does not
// recognize the session: the cookie is expired, or it was issued by another
// TeraBox frontend than the API host being used.
var ErrCookieExpired = errors.New("server did not recognize the session: the cookie is expired or belongs to another TeraBox frontend (match --base-url to the domain in your browser's address bar)")

const (
	userAgent = "okhttp/7.4"
	appId     = "250528"

	// DefaultBaseURL is the TeraBox API host used when none is configured.
	DefaultBaseURL = "https://www.terabox.com"
	// BaseURLEnvName overrides DefaultBaseURL, e.g. for accounts that use
	// another frontend (https://dm.terabox.com, https://dm.nephobox.com).
	BaseURLEnvName = "TERABOX_BASE_URL"
)

type Client struct {
	client   *resty.Client
	cookies  []*http.Cookie
	jsToken  string
	bdsToken string
	baseURL  string
	host     string
	domain   string
	region   string
	cwd      string
}

func NewClient(cookieStr string, httpClient *http.Client) (*Client, error) {
	return NewClientWithBaseURL(cookieStr, "", httpClient)
}

// NewClientWithBaseURL builds a client for the given TeraBox-compatible API
// host. An empty baseURL falls back to TERABOX_BASE_URL and then
// DefaultBaseURL. Accounts created on another frontend may only work with
// that frontend's host, e.g. https://dm.terabox.com.
func NewClientWithBaseURL(cookieStr, baseURL string, httpClient *http.Client) (*Client, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = strings.TrimSpace(os.Getenv(BaseURLEnvName))
	}
	if base == "" {
		base = DefaultBaseURL
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	base = strings.TrimRight(base, "/")

	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid TeraBox base URL: %q", base)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating cookie jar: %w", err)
	}

	var restyClient *resty.Client
	if httpClient != nil {
		restyClient = resty.NewWithClient(httpClient)
	} else {
		restyClient = resty.New()
	}
	restyClient.SetCookieJar(jar)
	restyClient.SetHeader("User-Agent", userAgent)
	restyClient.SetHeader("Referer", base+"/")
	restyClient.SetHeader("Origin", base)
	restyClient.SetBaseURL(base)

	cookies := strings.Split(cookieStr, ";")

	var cookieSlice []*http.Cookie
	var cookieNames []string
	hasNuds := false
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		if cookie == "" {
			continue
		}
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		if name == "ndus" {
			hasNuds = true
		}
		httpCookie := &http.Cookie{
			Name:     name,
			Value:    value,
			Domain:   cookieDomain(u.Hostname()),
			Secure:   u.Scheme == "https",
			HttpOnly: true,
		}
		restyClient.SetCookie(httpCookie)
		cookieSlice = append(cookieSlice, httpCookie)
		cookieNames = append(cookieNames, name)
	}
	if !hasNuds {
		return nil, fmt.Errorf("cookie does not contain ndus: copy the cookie value from %s in your browser", base)
	}

	c := &Client{
		client:  restyClient,
		cookies: cookieSlice,
		baseURL: base,
		host:    u.Hostname(),
		domain:  cookieDomain(u.Hostname()),
		cwd:     "/",
	}

	c.jsToken, c.bdsToken, err = c.getToken()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain authentication tokens: %w (base URL: %s, sent cookies: %s)",
			err, base, strings.Join(cookieNames, ", "))
	}

	return c, nil
}

// cookieDomain returns the registrable parent domain to share the session
// cookies with API subdomains (www.terabox.com -> terabox.com).
func cookieDomain(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

func (c *Client) getToken() (jsToken, bdsToken string, err error) {
	body, err := util.GetResponse(c.client.R().Get("/main"))
	if err != nil {
		return "", "", fmt.Errorf("error getting initial response: %w", err)
	}

	bodyStr := string(body)

	reBdsToken := regexp.MustCompile(`"bdstoken":"([^"]*)"`)
	matchBdsToken := reBdsToken.FindStringSubmatch(bodyStr)
	if len(matchBdsToken) < 2 {
		err = fmt.Errorf("bdstoken not found in /main response (cookie may be expired or the page layout changed)")
		return
	}
	// An empty bdstoken means the server did not start a session for this
	// cookie: /main is served as a logged-out page.
	if matchBdsToken[1] == "" {
		err = ErrCookieExpired
		return
	}
	bdsToken = matchBdsToken[1]

	reRegionPrefix := regexp.MustCompile(`"regionDomainPrefix":"([^"]*)"`)
	if match := reRegionPrefix.FindStringSubmatch(bodyStr); len(match) > 1 {
		c.region = match[1]
	}

	reJsToken := regexp.MustCompile(`window\.jsToken%20%3D%20a%7D%3Bfn%28%22([^%]+)`)
	matchJsToken := reJsToken.FindSubmatch(body)
	if len(matchJsToken) > 1 {
		jsToken = string(matchJsToken[1])
	} else {
		err = fmt.Errorf("jsToken not found in /main response (unexpected page layout)")
		return
	}

	return jsToken, bdsToken, nil
}
