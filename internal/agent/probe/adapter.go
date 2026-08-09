package probe

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/ipchronicle/ipchronicle/internal/agent/state"
	xproxy "golang.org/x/net/proxy"
)

const proxyDialTimeout = 15 * time.Second

type localProxyAdapter struct {
	listener net.Listener
	server   *http.Server
	done     chan error
}

func startLocalProxyAdapter(proxyConfiguration state.Proxy) (*localProxyAdapter, error) {
	handler := goproxy.NewProxyHttpServer()
	handler.Verbose = false
	handler.Logger = log.New(io.Discard, "", 0)
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
	}
	baseDialer := &net.Dialer{Timeout: proxyDialTimeout, KeepAlive: -1}
	proxyAddress := net.JoinHostPort(proxyConfiguration.Host, strconv.FormatInt(proxyConfiguration.Port, 10))
	switch proxyConfiguration.Scheme {
	case "http", "https":
		upstream := &url.URL{Scheme: proxyConfiguration.Scheme, Host: proxyAddress}
		if proxyConfiguration.Username != nil || proxyConfiguration.Password != nil {
			username, password := proxyCredentials(proxyConfiguration)
			upstream.User = url.UserPassword(username, password)
		}
		transport.Proxy = http.ProxyURL(upstream)
		transport.DialContext = baseDialer.DialContext
		handler.Tr = transport
		upstreamWithoutCredentials := *upstream
		upstreamWithoutCredentials.User = nil
		handler.ConnectDial = handler.NewConnectDialToProxyWithHandler(upstreamWithoutCredentials.String(), func(request *http.Request) {
			if proxyConfiguration.Username == nil && proxyConfiguration.Password == nil {
				return
			}
			username, password := proxyCredentials(proxyConfiguration)
			token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
			request.Header.Set("Proxy-Authorization", "Basic "+token)
		})
	case "socks5":
		var authentication *xproxy.Auth
		if proxyConfiguration.Username != nil || proxyConfiguration.Password != nil {
			username, password := proxyCredentials(proxyConfiguration)
			authentication = &xproxy.Auth{User: username, Password: password}
		}
		dialer, err := xproxy.SOCKS5("tcp", proxyAddress, authentication, baseDialer)
		if err != nil {
			return nil, fmt.Errorf("prepare SOCKS5 adapter: %w", err)
		}
		contextDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS5 adapter does not support cancellation")
		}
		transport.DialContext = contextDialer.DialContext
		handler.Tr = transport
		handler.ConnectDial = dialer.Dial
	default:
		return nil, errors.New("unsupported proxy scheme")
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("listen for execution-scoped proxy adapter: %w", err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
	adapter := &localProxyAdapter{listener: listener, server: server, done: make(chan error, 1)}
	go func() {
		adapter.done <- server.Serve(listener)
	}()
	return adapter, nil
}

func (adapter *localProxyAdapter) URL() string {
	return "http://" + adapter.listener.Addr().String()
}

func (adapter *localProxyAdapter) Close() error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownErr := adapter.server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, adapter.server.Close())
	}
	serveErr := <-adapter.done
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}

func proxyCredentials(configuration state.Proxy) (string, string) {
	var username, password string
	if configuration.Username != nil {
		username = *configuration.Username
	}
	if configuration.Password != nil {
		password = *configuration.Password
	}
	return username, password
}
