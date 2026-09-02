// Derived in part from IPQuality at commit 0ee5f192fed70c04615852efba0e4b8bd43546c7.
// Attribution and modification details are retained in THIRD_PARTY_NOTICES.md.

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const ipqsProbeAttempts = 3

type providerFinding struct {
	CountryCode string
	Proxy       *bool
	Tor         *bool
	VPN         *bool
	Server      *bool
	Abuser      *bool
	Robot       *bool
	Usage       string
	Company     string
	Score       string
}

func (finding providerFinding) factor(name string) any {
	switch name {
	case "Proxy":
		return finding.Proxy
	case "Tor":
		return finding.Tor
	case "VPN":
		return finding.VPN
	case "Server":
		return finding.Server
	case "Abuser":
		return finding.Abuser
	case "Robot":
		return finding.Robot
	default:
		return nil
	}
}

func (engine *nativeEngine) probeBasic(ctx context.Context) basicFinding {
	document := engine.http.json(ctx, http.MethodGet, engine.checkPlaceURL("lang=cn"), nil, nil)
	if document == nil {
		return engine.basicFromIPInfo(ctx)
	}
	english := engine.http.json(ctx, http.MethodGet, engine.checkPlaceURL("lang=en"), nil, nil)
	value := func(path ...string) string {
		result := documentString(document, path...)
		if result == "" && english != nil {
			result = documentString(english, path...)
		}
		return result
	}
	latitude := value("City", "Latitude")
	longitude := value("City", "Longitude")
	radius := value("City", "AccuracyRadius")
	regionCode := value("Country", "IsoCode")
	registeredCode := value("Country", "RegisteredCountry", "IsoCode")
	return basicFinding{
		ASN:          value("ASN", "AutonomousSystemNumber"),
		Organization: value("ASN", "AutonomousSystemOrganization"),
		Latitude:     latitude, Longitude: longitude,
		DMS: coordinateDMS(latitude, longitude), Map: coordinateMap(latitude, longitude, radius),
		TimeZone: value("City", "Location", "TimeZone"),
		City:     value("City", "Name"), PostalCode: value("City", "PostalCode"),
		SubCode:      firstArrayDocumentString(document, english, "City", "Subdivisions", "IsoCode"),
		Subdivisions: firstArrayDocumentString(document, english, "City", "Subdivisions", "Name"),
		RegionCode:   regionCode, RegionName: value("Country", "Name"),
		ContinentCode:        value("City", "Continent", "Code"),
		ContinentName:        value("City", "Continent", "Name"),
		RegisteredRegionCode: registeredCode,
		RegisteredRegionName: value("Country", "RegisteredCountry", "Name"),
		Type:                 geographicalType(regionCode, registeredCode),
	}
}

func (engine *nativeEngine) basicFromIPInfo(ctx context.Context) basicFinding {
	document := engine.explicitLookupHTTP.json(ctx, http.MethodGet,
		"https://ipinfo.io/widget/demo/"+engine.input.Target, nil, nil)
	location := strings.Split(documentString(document, "data", "loc"), ",")
	latitude, longitude := "", ""
	if len(location) == 2 {
		latitude, longitude = strings.TrimSpace(location[0]), strings.TrimSpace(location[1])
	}
	regionCode := documentString(document, "data", "country")
	registeredCode := documentString(document, "data", "abuse", "country")
	region := isoCountries[strings.ToUpper(regionCode)]
	registeredRegion := isoCountries[strings.ToUpper(registeredCode)]
	return basicFinding{
		ASN:          strings.TrimPrefix(documentString(document, "data", "asn", "asn"), "AS"),
		Organization: documentString(document, "data", "asn", "name"),
		Latitude:     latitude, Longitude: longitude,
		DMS: coordinateDMS(latitude, longitude), Map: coordinateMap(latitude, longitude, "1001"),
		TimeZone:   documentString(document, "data", "timezone"),
		City:       documentString(document, "data", "city"),
		PostalCode: documentString(document, "data", "postal"),
		RegionCode: regionCode, RegionName: region.Name, ContinentName: region.Region,
		RegisteredRegionCode: registeredCode, RegisteredRegionName: registeredRegion.Name,
		Type: geographicalType(regionCode, registeredCode),
	}
}

func (engine *nativeEngine) probeProviders(ctx context.Context) map[string]providerFinding {
	return map[string]providerFinding{
		"IPinfo":      engine.probeIPInfo(ctx),
		"SCAMALYTICS": engine.probeScamalytics(ctx),
		"ipregistry":  engine.probeIPRegistry(ctx),
		"ipapi":       engine.probeIPAPI(ctx),
		"AbuseIPDB":   engine.probeAbuseIPDB(ctx),
		"IP2LOCATION": engine.probeIP2Location(ctx),
		"DBIP":        engine.probeDBIP(ctx),
		"ipdata":      engine.probeIPData(ctx),
		"IPQS":        engine.probeIPQS(ctx),
	}
}

func (engine *nativeEngine) probeIPInfo(ctx context.Context) providerFinding {
	document := engine.explicitLookupHTTP.json(ctx, http.MethodGet,
		"https://ipinfo.io/widget/demo/"+engine.input.Target, nil, nil)
	return providerFinding{
		CountryCode: documentString(document, "data", "country"),
		Proxy:       documentBool(document, "data", "privacy", "proxy"),
		Tor:         documentBool(document, "data", "privacy", "tor"),
		VPN:         documentBool(document, "data", "privacy", "vpn"),
		Server:      documentBool(document, "data", "privacy", "hosting"),
		Usage:       documentString(document, "data", "asn", "type"),
		Company:     documentString(document, "data", "company", "type"),
	}
}

func (engine *nativeEngine) probeScamalytics(ctx context.Context) providerFinding {
	document := engine.checkPlaceDocument(ctx, "scamalytics")
	return providerFinding{
		CountryCode: documentString(document, "external_datasources", "maxmind_geolite2", "ip_country_code"),
		Proxy:       documentBool(document, "external_datasources", "firehol", "is_proxy"),
		Tor:         documentBool(document, "external_datasources", "x4bnet", "is_tor"),
		VPN:         documentBool(document, "scamalytics", "scamalytics_proxy", "is_vpn"),
		Server:      documentBool(document, "scamalytics", "scamalytics_proxy", "is_datacenter"),
		Abuser:      documentBool(document, "scamalytics", "is_blacklisted_external"),
		Robot: combinedBool(
			documentBool(document, "external_datasources", "x4bnet", "is_blacklisted_spambot"),
			documentBool(document, "external_datasources", "x4bnet", "is_bot_operamini"),
			documentBool(document, "external_datasources", "x4bnet", "is_bot_semrush"),
		),
		Score: documentString(document, "scamalytics", "scamalytics_score"),
	}
}

func (engine *nativeEngine) probeIPRegistry(ctx context.Context) providerFinding {
	key := "sb69ksjcajfs4c"
	headers := headersWithUserAgent(browserUserAgent)
	response, err := engine.http.get(ctx, "https://ipregistry.co", headers)
	if err == nil {
		match := regexp.MustCompile(`apiKey="([a-zA-Z0-9]+)"`).FindSubmatch(response.Body)
		if len(match) == 2 {
			key = string(match[1])
		}
	}
	headers.Set("Origin", "https://ipregistry.co")
	headers.Set("Referer", "https://ipregistry.co/")
	document := engine.http.json(ctx, http.MethodGet, "https://api.ipregistry.co/"+
		engine.input.Target+"?hostname=true&key="+key, headers, nil)
	return providerFinding{
		CountryCode: documentString(document, "location", "country", "code"),
		Proxy:       documentBool(document, "security", "is_proxy"),
		Tor:         combinedBool(documentBool(document, "security", "is_tor"), documentBool(document, "security", "is_tor_exit")),
		VPN:         documentBool(document, "security", "is_vpn"),
		Server:      documentBool(document, "security", "is_cloud_provider"),
		Abuser:      documentBool(document, "security", "is_abuser"),
		Usage:       documentString(document, "connection", "type"),
		Company:     documentString(document, "company", "type"),
	}
}

func (engine *nativeEngine) probeIPAPI(ctx context.Context) providerFinding {
	method := http.MethodGet
	target := "https://api.ipapi.is/?q=" + queryEscapeAddress(engine.input.Target)
	var headers http.Header
	var body []byte
	if engine.input.IPAPIAPIKey != "" {
		method = http.MethodPost
		target = "https://api.ipapi.is"
		headers = make(http.Header)
		headers.Set("Content-Type", "application/json")
		body, _ = json.Marshal(map[string]string{"q": engine.input.Target, "key": engine.input.IPAPIAPIKey})
	}
	document := engine.explicitLookupHTTP.json(ctx, method, target, headers, body)
	score := documentString(document, "company", "abuser_score")
	if score == "" {
		score = documentString(document, "abuser_score")
	}
	fields := strings.Fields(score)
	if len(fields) > 0 {
		if numeric, err := strconv.ParseFloat(fields[0], 64); err == nil {
			score = fmt.Sprintf("%.2f%%", numeric*100)
		} else {
			score = ""
		}
	} else {
		score = ""
	}
	country := documentString(document, "location", "country_code")
	if country == "" {
		country = documentString(document, "cc")
	}
	return providerFinding{
		CountryCode: country,
		Proxy:       documentBool(document, "is_proxy"), Tor: documentBool(document, "is_tor"),
		VPN: documentBool(document, "is_vpn"), Server: documentBool(document, "is_datacenter"),
		Abuser: documentBool(document, "is_abuser"), Robot: documentBool(document, "is_crawler"),
		Usage: documentString(document, "asn", "type"), Company: documentString(document, "company", "type"),
		Score: score,
	}
}

func (engine *nativeEngine) probeAbuseIPDB(ctx context.Context) providerFinding {
	document := engine.checkPlaceDocument(ctx, "abuseipdb")
	return providerFinding{
		Usage: documentString(document, "data", "usageType"),
		Score: documentString(document, "data", "abuseConfidenceScore"),
	}
}

func (engine *nativeEngine) probeIP2Location(ctx context.Context) providerFinding {
	document := engine.checkPlaceDocument(ctx, "ip2location")
	return providerFinding{
		CountryCode: documentString(document, "country_code"),
		Proxy: combinedBool(documentBool(document, "is_proxy"),
			documentBool(document, "proxy", "is_public_proxy"), documentBool(document, "proxy", "is_web_proxy")),
		Tor: documentBool(document, "proxy", "is_tor"), VPN: documentBool(document, "proxy", "is_vpn"),
		Server: documentBool(document, "proxy", "is_data_center"),
		Abuser: documentBool(document, "proxy", "is_spammer"),
		Robot: combinedBool(documentBool(document, "proxy", "is_web_crawler"),
			documentBool(document, "proxy", "is_scanner"), documentBool(document, "proxy", "is_botnet")),
		Usage:   firstSlashPart(documentString(document, "usage_type")),
		Company: firstSlashPart(documentString(document, "as_info", "as_usage_type")),
		Score:   documentString(document, "fraud_score"),
	}
}

func (engine *nativeEngine) probeDBIP(ctx context.Context) providerFinding {
	response, err := engine.explicitLookupHTTP.get(ctx, "https://db-ip.com/api/core/", nil)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return providerFinding{}
	}
	match := regexp.MustCompile(`data-api-key="([^"]+)"`).FindSubmatch(response.Body)
	if len(match) != 2 {
		return providerFinding{}
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "text/plain;charset=UTF-8")
	headers.Set("Origin", "https://db-ip.com")
	headers.Set("Referer", "https://db-ip.com/")
	document := engine.explicitLookupHTTP.json(ctx, http.MethodPost, "https://api.db-ip.com/v2/"+
		string(match[1])+"/self?convertCurrencies", headers,
		[]byte(`[["11.49","EUR"],["139.90","EUR"],["699.90","EUR"]]`))
	level := strings.ToLower(documentString(document, "threatLevel"))
	score := ""
	switch level {
	case "low":
		score = "0"
	case "medium":
		score = "50"
	case "high":
		score = "100"
	}
	return providerFinding{
		CountryCode: documentString(document, "countryCode"),
		Proxy:       documentBool(document, "isProxy"), Robot: documentBool(document, "isCrawler"),
		Score: score,
	}
}

func (engine *nativeEngine) probeIPData(ctx context.Context) providerFinding {
	document := engine.checkPlaceDocument(ctx, "ipdata")
	return providerFinding{
		CountryCode: documentString(document, "country_code"),
		Proxy:       documentBool(document, "threat", "is_proxy"), Tor: documentBool(document, "threat", "is_tor"),
		Server: documentBool(document, "threat", "is_datacenter"),
		Abuser: combinedBool(documentBool(document, "threat", "is_threat"),
			documentBool(document, "threat", "is_known_abuser"), documentBool(document, "threat", "is_known_attacker")),
	}
}

func (engine *nativeEngine) probeIPQS(ctx context.Context) providerFinding {
	document := engine.checkPlaceDocumentWithRetry(ctx, "ipqualityscore", ipqsProbeAttempts)
	return providerFinding{
		CountryCode: documentString(document, "country_code"),
		Proxy:       documentBool(document, "proxy"), Tor: documentBool(document, "tor"),
		VPN: documentBool(document, "vpn"), Abuser: documentBool(document, "recent_abuse"),
		Robot: documentBool(document, "bot_status"), Score: documentString(document, "fraud_score"),
	}
}

func (engine *nativeEngine) checkPlaceDocumentWithRetry(
	ctx context.Context,
	database string,
	attempts int,
) map[string]any {
	for attempt := 0; attempt < attempts; attempt++ {
		response, err := engine.http.do(ctx, http.MethodGet, engine.checkPlaceURL("db="+database), nil, nil)
		retry := false
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			if document := decodeJSONDocument(response.Body); document != nil {
				return document
			}
			retry = true
		} else {
			retry = transientProviderFailure(ctx, response.StatusCode, err)
		}
		if attempt+1 >= attempts || !retry {
			return nil
		}
		delay := 250 * time.Millisecond * time.Duration(1<<attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

func transientProviderFailure(ctx context.Context, statusCode int, err error) bool {
	if err != nil {
		return ctx.Err() == nil
	}
	return statusCode == http.StatusRequestTimeout || statusCode == http.StatusTooEarly || statusCode >= 500
}

func (engine *nativeEngine) checkPlaceDocument(ctx context.Context, database string) map[string]any {
	return engine.http.json(ctx, http.MethodGet, engine.checkPlaceURL("db="+database), nil, nil)
}

func (engine *nativeEngine) checkPlaceURL(query string) string {
	return "https://ipinfo.check.place/" + engine.input.Target + "?" + query
}

func firstArrayDocumentString(primary, fallback map[string]any, object, array, field string) string {
	read := func(document map[string]any) string {
		container, ok := documentValue(document, object, array).([]any)
		if !ok || len(container) == 0 {
			return ""
		}
		item, ok := container[0].(map[string]any)
		if !ok {
			return ""
		}
		return documentString(item, field)
	}
	if value := read(primary); value != "" {
		return value
	}
	return read(fallback)
}

func geographicalType(region, registered string) string {
	if region == "" {
		return ""
	}
	if strings.EqualFold(region, registered) {
		return "原生IP"
	}
	return "广播IP"
}

func coordinateMap(latitude, longitude, radius string) string {
	if latitude == "" || longitude == "" || radius == "" {
		return ""
	}
	zoom := 15
	if value, err := strconv.ParseFloat(radius, 64); err == nil {
		switch {
		case value > 1000:
			zoom = 12
		case value > 500:
			zoom = 13
		case value > 250:
			zoom = 14
		}
	}
	return fmt.Sprintf("https://check.place/%s,%s,%d,cn", latitude, longitude, zoom)
}

func firstSlashPart(value string) string {
	value, _, _ = strings.Cut(value, "/")
	return strings.TrimSpace(value)
}
