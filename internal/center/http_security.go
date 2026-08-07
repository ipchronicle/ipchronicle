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

type proxyPolicy struct {
	externalOrigin *url.URL
	trustedProxies []netip.Prefix
}

func newProxyPolicy(externalOrigin *url.URL, trustedProxies []netip.Prefix) proxyPolicy {
	return proxyPolicy{externalOrigin: externalOrigin, trustedProxies: trustedProxies}
}

func (p proxyPolicy) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := remoteAddress(r.RemoteAddr)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		client := peer
		if p.trusted(peer) {
			if forwardedScheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); forwardedScheme == "http" || forwardedScheme == "https" {
				scheme = forwardedScheme
			}
			if forwardedClient, ok := p.forwardedClient(r.Header.Get("X-Forwarded-For"), peer); ok {
				client = forwardedClient
			}
		}

		expectedOrigin := scheme + "://" + r.Host
		cookieSecure := scheme == "https"
		if p.externalOrigin != nil {
			expectedOrigin = originString(p.externalOrigin)
			cookieSecure = p.externalOrigin.Scheme == "https"
		}
		sessionToken := ""
		if cookie, err := r.Cookie(administratorSessionCookie); err == nil {
			sessionToken = cookie.Value
		}
		security := requestSecurity{
			Authorization:  r.Header.Get("Authorization"),
			ClientAddress:  client.String(),
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

func (p proxyPolicy) trusted(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	for _, prefix := range p.trustedProxies {
		if prefix.Contains(address.Unmap()) {
			return true
		}
	}
	return false
}

func (p proxyPolicy) forwardedClient(header string, peer netip.Addr) (netip.Addr, bool) {
	if header == "" {
		return netip.Addr{}, false
	}
	parts := strings.Split(header, ",")
	addresses := make([]netip.Addr, 0, len(parts)+1)
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return netip.Addr{}, false
		}
		addresses = append(addresses, address.Unmap())
	}
	addresses = append(addresses, peer.Unmap())
	for index := len(addresses) - 1; index >= 0; index-- {
		if !p.trusted(addresses[index]) {
			return addresses[index], true
		}
	}
	return addresses[0], true
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
