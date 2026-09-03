package center

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

const administratorSessionCookie = "ipchronicle_session"

type requestSecurity struct {
	Authorization  string
	ClientAddress  string
	Scheme         string
	ExpectedOrigin string
	Origin         string
	SessionToken   string
	UserAgent      string
	CookieSecure   bool
}

type requestSecurityKey struct{}

func requestSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := remoteAddress(r.RemoteAddr)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if forwardedScheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); forwardedScheme == "http" || forwardedScheme == "https" {
			scheme = forwardedScheme
		}
		expectedOrigin := scheme + "://" + r.Host
		cookieSecure := scheme == "https"
		sessionToken := ""
		if cookie, err := r.Cookie(administratorSessionCookie); err == nil {
			sessionToken = cookie.Value
		}
		security := requestSecurity{
			Authorization:  r.Header.Get("Authorization"),
			ClientAddress:  peer.String(),
			Scheme:         scheme,
			ExpectedOrigin: expectedOrigin,
			Origin:         r.Header.Get("Origin"),
			SessionToken:   sessionToken,
			UserAgent:      r.UserAgent(),
			CookieSecure:   cookieSecure,
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestSecurityKey{}, security)))
	})
}

func requestSecurityFromContext(ctx context.Context) requestSecurity {
	security, _ := ctx.Value(requestSecurityKey{}).(requestSecurity)
	return security
}

func remoteAddress(value string) netip.Addr {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	address, _ := netip.ParseAddr(strings.Trim(host, "[]"))
	return address.Unmap()
}

func firstForwardedValue(value string) string {
	first, _, _ := strings.Cut(value, ",")
	return strings.ToLower(strings.TrimSpace(first))
}

func originString(value *url.URL) string {
	return strings.ToLower(value.Scheme) + "://" + strings.ToLower(value.Host)
}

func originMatches(actual, expected string) bool {
	if actual == "" || actual == "null" {
		return false
	}
	actualURL, err := url.Parse(actual)
	if err != nil || actualURL.User != nil || actualURL.RawQuery != "" || actualURL.Fragment != "" || (actualURL.Path != "" && actualURL.Path != "/") {
		return false
	}
	return originString(actualURL) == strings.ToLower(expected)
}
