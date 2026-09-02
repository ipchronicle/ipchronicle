package probe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/center/history"
)

func TestNativeReportMatchesCenterFieldContract(t *testing.T) {
	trueValue := true
	engine := nativeEngine{input: nativeProbeInput{StartedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}
	providers := map[string]providerFinding{
		"IPinfo": {CountryCode: "US", Usage: "isp", Company: "business", Proxy: &trueValue},
	}
	media := map[string]mediaFinding{"TikTok": {Status: "解锁", Region: "US", Type: "原生"}}
	services := make(map[string]any, len(mailServices))
	for _, service := range mailServices {
		services[service.Name] = false
	}
	report := engine.buildReport(netip.MustParseAddr("203.0.113.10"), basicFinding{}, providers, media, mailFinding{
		Port25: false, Services: services,
		DNSBlacklist: dnsBlacklistFinding{Total: 1, Clean: 1, Marked: 0, Blacklisted: 0},
	})
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	interpreted, err := history.Interpret(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(interpreted.Issues) != 0 {
		t.Fatalf("native report contract issues = %#v", interpreted.Issues)
	}
	fields := make(map[string]history.FieldValue, len(interpreted.Fields))
	for _, field := range interpreted.Fields {
		fields[field.ID] = field
	}
	for _, path := range []string{"Info.ASN", "Type.Usage.ipregistry", "Score.IPQS", "Media.DisneyPlus.Status"} {
		if fields[path].Status != "unavailable" {
			t.Fatalf("missing provider field %s status = %q", path, fields[path].Status)
		}
	}
	if report.Head["IP"] != "203.0.*.*" || report.Head["Version"] != nativeProbeVersion {
		t.Fatalf("head = %#v", report.Head)
	}
	if masked := maskAddress(netip.MustParseAddr("2001:db8:1234::10")); masked != "2001:db8:1234:*:*:*:*:*" {
		t.Fatalf("masked IPv6 = %q", masked)
	}
	countries := report.Factor["CountryCode"].(map[string]any)
	if _, present := countries["IPWHOIS"]; present {
		t.Fatal("native report still contains the unused IPWHOIS provider")
	}
}

func TestNativeProviderParsersMapUpstreamFields(t *testing.T) {
	ipapiAPIKey := "test-ipapi-key"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		switch {
		case request.URL.Host == "ipinfo.io":
			body = `{"data":{"country":"US","asn":{"type":"hosting"},"company":{"type":"business"},"privacy":{"proxy":true,"tor":false,"vpn":true,"hosting":true}}}`
		case request.URL.Host == "api.ipapi.is":
			if request.Method != http.MethodPost || request.URL.RawQuery != "" ||
				request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("authenticated ipapi request = %s %s", request.Method, request.URL.String())
			}
			var input map[string]string
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil ||
				input["q"] != "203.0.113.10" || input["key"] != ipapiAPIKey {
				t.Fatalf("authenticated ipapi body = %#v, %v", input, err)
			}
			body = `{"location":{"country_code":"US"},"company":{"type":"hosting","abuser_score":"0.125 (Low)"},"asn":{"type":"isp"},"is_proxy":false,"is_tor":false,"is_vpn":true,"is_datacenter":true,"is_abuser":false,"is_crawler":false}`
		case request.URL.Host == "ipinfo.check.place" && request.URL.Query().Get("db") == "ip2location":
			body = `{"country_code":"US","usage_type":"DCH/COM","as_info":{"as_usage_type":"ISP"},"is_proxy":false,"proxy":{"is_public_proxy":false,"is_web_proxy":true,"is_tor":false,"is_vpn":true,"is_data_center":true,"is_spammer":false,"is_web_crawler":false,"is_scanner":false,"is_botnet":true},"fraud_score":42}`
		case request.URL.Host == "ipinfo.check.place" && request.URL.Query().Get("db") == "ipqualityscore":
			body = `{"country_code":"US","fraud_score":73,"proxy":false,"tor":false,"vpn":true,"recent_abuse":false,"bot_status":true}`
		}
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	engine := nativeEngine{
		input: nativeProbeInput{Target: "203.0.113.10", IPAPIAPIKey: ipapiAPIKey},
		http:  probeHTTP{client: client}, explicitLookupHTTP: probeHTTP{client: client},
	}
	ipinfo := engine.probeIPInfo(context.Background())
	if ipinfo.CountryCode != "US" || ipinfo.Usage != "hosting" || !boolValue(ipinfo.Proxy) || !boolValue(ipinfo.VPN) {
		t.Fatalf("IPinfo = %#v", ipinfo)
	}
	ipapi := engine.probeIPAPI(context.Background())
	if ipapi.Score != "12.50%" || ipapi.CountryCode != "US" || !boolValue(ipapi.Server) {
		t.Fatalf("ipapi = %#v", ipapi)
	}
	ip2location := engine.probeIP2Location(context.Background())
	if ip2location.Usage != "DCH" || ip2location.Company != "ISP" || !boolValue(ip2location.Proxy) ||
		!boolValue(ip2location.Robot) || ip2location.Score != "42" {
		t.Fatalf("IP2Location = %#v", ip2location)
	}
	ipqs := engine.probeIPQS(context.Background())
	if ipqs.Score != "73" || !boolValue(ipqs.Robot) {
		t.Fatalf("IPQS = %#v", ipqs)
	}
}

func TestNativeIPv6ProviderRequestsUseRawPathsAndExplicitLookupClient(t *testing.T) {
	var pathRequests, lookupRequests []*http.Request
	response := func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)),
			Header: make(http.Header), Request: request,
		}, nil
	}
	engine := nativeEngine{
		input: nativeProbeInput{Target: "2001:db8:1234::10"},
		http: probeHTTP{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			pathRequests = append(pathRequests, request)
			return response(request)
		})}},
		explicitLookupHTTP: probeHTTP{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			lookupRequests = append(lookupRequests, request)
			return response(request)
		})}},
	}

	_ = engine.probeBasic(context.Background())
	_ = engine.probeIPInfo(context.Background())
	_ = engine.probeIPAPI(context.Background())

	if len(pathRequests) != 2 || pathRequests[0].URL.Path != "/2001:db8:1234::10" ||
		pathRequests[0].URL.RawPath != "" || pathRequests[0].URL.Query().Get("lang") != "cn" {
		t.Fatalf("path requests = %#v", pathRequests)
	}
	if len(lookupRequests) != 2 || lookupRequests[0].URL.Host != "ipinfo.io" ||
		lookupRequests[0].URL.Path != "/widget/demo/2001:db8:1234::10" ||
		lookupRequests[1].URL.Query().Get("q") != "2001:db8:1234::10" {
		t.Fatalf("explicit lookup requests = %#v", lookupRequests)
	}
}

func TestIPQSProviderRetriesOnlyTransientFailures(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		status := http.StatusServiceUnavailable
		body := `{}`
		if requests == 2 {
			status = http.StatusOK
			body = `{"success":true,"country_code":"US","fraud_score":73,"proxy":false}`
		}
		return &http.Response{
			StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	engine := nativeEngine{
		input: nativeProbeInput{Target: "203.0.113.10"},
		http:  probeHTTP{client: client},
	}

	finding := engine.probeIPQS(context.Background())
	if requests != 2 || finding.Score != "73" || finding.CountryCode != "US" || finding.Proxy == nil || *finding.Proxy {
		t.Fatalf("IPQS requests = %d, finding = %#v", requests, finding)
	}
}

func TestIPQSProviderDoesNotRetryQuotaResponse(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"success":false,"message":"You have exceeded your request quota of 200 per day."}`,
			)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	engine := nativeEngine{
		input: nativeProbeInput{Target: "203.0.113.10"},
		http:  probeHTTP{client: client},
	}

	finding := engine.probeIPQS(context.Background())
	if requests != 1 || finding != (providerFinding{}) {
		t.Fatalf("IPQS quota requests = %d, finding = %#v", requests, finding)
	}
}

func TestNativeRequestUserAgentsMatchUpstreamRequestClasses(t *testing.T) {
	var requests []*http.Request
	client := probeHTTP{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request)
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)),
			Header: make(http.Header), Request: request,
		}, nil
	})}}

	if _, err := client.get(context.Background(), "https://ipinfo.check.place/203.0.113.10?db=ip2location", nil); err != nil {
		t.Fatal(err)
	}
	browserHeaders := headersWithUserAgent(browserUserAgent)
	if _, err := client.get(context.Background(), "https://www.netflix.com/title/81280792", browserHeaders); err != nil {
		t.Fatal(err)
	}

	if got := requests[0].UserAgent(); got != curlUserAgent {
		t.Fatalf("ordinary provider User-Agent = %q", got)
	}
	if got := requests[1].UserAgent(); got != browserUserAgent {
		t.Fatalf("browser provider User-Agent = %q", got)
	}
}

func TestNativeMediaAndDNSBlacklistHelpers(t *testing.T) {
	if region := mediaRegion([]byte(`{"countryOfSignup":"jp"}`)); region != "JP" {
		t.Fatalf("media region = %q", region)
	}
	if region := mediaRegion([]byte(
		`{"requestCountry":{"id":"US","countryName":"United\\x20States"}}`,
	)); region != "US" {
		t.Fatalf("Netflix request-country region = %q", region)
	}
	if region := firstPattern(normalizeEmbeddedMediaBody([]byte(
		`{&#34;currentTerritory&#34;:&#34;US&#34;}`,
	)), `"currentTerritory"\s*:\s*"([A-Za-z]{2})"`); region != "US" {
		t.Fatalf("Prime Video HTML-encoded region = %q", region)
	}
	publicAddresses := []net.IPAddr{{IP: net.ParseIP("151.101.1.140")}}
	if result := redditUnlockTypeFromDNS(publicAddresses, 4); result != "原生" {
		t.Fatalf("Reddit multi-answer unlock type = %q", result)
	}
	if result := redditUnlockTypeFromDNS(publicAddresses, 2); result != "DNS" {
		t.Fatalf("Reddit short-answer unlock type = %q", result)
	}
	zones := uniqueDNSBlacklistZones("b.example\na.example\nb.example\n\n")
	if strings.Join(zones, ",") != "a.example,b.example" {
		t.Fatalf("DNSBL zones = %q", zones)
	}
	if decimalByte(0) != "0" || decimalByte(42) != "42" || decimalByte(255) != "255" {
		t.Fatal("decimal byte conversion failed")
	}
}

func TestCombinedBoolRequiresCompleteNegativeEvidence(t *testing.T) {
	trueValue, falseValue := true, false
	if result := combinedBool(&falseValue, nil, &falseValue); result != nil {
		t.Fatalf("partial negative evidence = %v", *result)
	}
	if result := combinedBool(&falseValue, &trueValue, nil); result == nil || !*result {
		t.Fatalf("positive evidence = %v", result)
	}
	if result := combinedBool(nil, &trueValue); result == nil || !*result {
		t.Fatalf("positive evidence after missing value = %v", result)
	}
	if result := combinedBool(&falseValue, &falseValue); result == nil || *result {
		t.Fatalf("complete negative evidence = %v", result)
	}
}

func TestBasicIPInfoFallbackIncludesEmbeddedCountryNames(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"data":{"country":"US","abuse":{"country":"CA"},"loc":"40.0,-75.0"}}`
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	engine := nativeEngine{
		input: nativeProbeInput{Target: "203.0.113.10"},
		http:  probeHTTP{client: client}, explicitLookupHTTP: probeHTTP{client: client},
	}
	finding := engine.basicFromIPInfo(context.Background())
	if finding.RegionName != "United States of America" || finding.ContinentName != "Americas" ||
		finding.RegisteredRegionName != "Canada" {
		t.Fatalf("IPinfo fallback = %#v", finding)
	}
}

func TestDisneyForbiddenLocationResponseIsBlocked(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		status := http.StatusCreated
		body := `{"assertion":"test-assertion"}`
		if request.URL.Path == "/token" {
			status = http.StatusBadRequest
			body = `{"error":"unauthorized_client","error_description":"forbidden-location"}`
		}
		return &http.Response{
			StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
			Header: make(http.Header), Request: request,
		}, nil
	})}
	engine := nativeEngine{http: probeHTTP{client: client}}

	finding := engine.probeDisneyPlus(context.Background())
	if finding.Status != "屏蔽" || finding.Region != "" || finding.Type != "" {
		t.Fatalf("Disney+ finding = %#v", finding)
	}
}

func TestOverrideDialHostKeepsTLSHostnameOutOfTheDialAddress(t *testing.T) {
	var receivedNetwork, receivedAddress string
	wantError := errors.New("dial stopped")
	dialContext := overrideDialHost(
		func(_ context.Context, network, address string) (net.Conn, error) {
			receivedNetwork, receivedAddress = network, address
			return nil, wantError
		},
		"www.reddit.com",
		netip.MustParseAddr("2001:db8::10"),
	)

	_, err := dialContext(context.Background(), "tcp6", "www.reddit.com:443")
	if !errors.Is(err, wantError) || receivedNetwork != "tcp6" || receivedAddress != "[2001:db8::10]:443" {
		t.Fatalf("dial = (%q, %q, %v)", receivedNetwork, receivedAddress, err)
	}
}

func TestNativeProbeReturnsGlobalCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dialer := &net.Dialer{}
	_, err := runNativeProbe(ctx, nativeProbeInput{
		Target: "203.0.113.10", Family: "ipv4",
		HTTPClient: &http.Client{}, DialContext: dialer.DialContext,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled native probe error = %v", err)
	}
}

func TestDialEndpointKeepsSuccessfulConnectTunnelOpen(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodConnect || request.Host != "smtp.example:25" {
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		connection, buffered, err := response.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffered.Flush()
		time.Sleep(20 * time.Millisecond)
		_, _ = connection.Write([]byte("220 smtp.example ready\r\n"))
		_ = connection.SetReadDeadline(time.Now().Add(time.Second))
		_, _ = bufio.NewReader(connection).ReadString('\n')
	}))
	defer proxy.Close()

	engine := nativeEngine{input: nativeProbeInput{ProxyAdapterURL: proxy.URL}}
	if !engine.smtpAvailable(context.Background(), "smtp.example:25") {
		t.Fatal("SMTP banner was not readable through the CONNECT tunnel")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
