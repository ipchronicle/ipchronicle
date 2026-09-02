// The native probe is a Go port of the IPQuality probe behavior from
// https://github.com/xykt/IPQuality at commit
// 0ee5f192fed70c04615852efba0e4b8bd43546c7. Both projects are licensed under
// AGPL-3.0; attribution and modification details are retained in
// THIRD_PARTY_NOTICES.md.

package probe

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const nativeProbeVersion = "native-0ee5f19"

type nativeEngine struct {
	input              nativeProbeInput
	http               probeHTTP
	explicitLookupHTTP probeHTTP
}

type nativeReport struct {
	Head   map[string]any `json:"Head"`
	Info   map[string]any `json:"Info"`
	Type   map[string]any `json:"Type"`
	Score  map[string]any `json:"Score"`
	Factor map[string]any `json:"Factor"`
	Media  map[string]any `json:"Media"`
	Mail   map[string]any `json:"Mail"`
}

type basicFinding struct {
	ASN, Organization, Latitude, Longitude, DMS, Map, TimeZone string
	City, PostalCode, SubCode, Subdivisions                    string
	RegionCode, RegionName, ContinentCode, ContinentName       string
	RegisteredRegionCode, RegisteredRegionName, Type           string
}

type isoCountry struct {
	Name   string `json:"name"`
	Code   string `json:"alpha-2"`
	Region string `json:"region"`
}

//go:embed data/iso3166.json
var iso3166Data []byte

var isoCountries = parseISOCountries()

func parseISOCountries() map[string]isoCountry {
	var countries []isoCountry
	if err := json.Unmarshal(iso3166Data, &countries); err != nil {
		panic("parse embedded ISO 3166 data: " + err.Error())
	}
	result := make(map[string]isoCountry, len(countries))
	for _, country := range countries {
		result[strings.ToUpper(country.Code)] = country
	}
	return result
}

func runNativeProbe(ctx context.Context, input nativeProbeInput) ([]byte, error) {
	target, err := netip.ParseAddr(input.Target)
	if err != nil || (input.Family == "ipv4") != target.Is4() {
		return nil, errors.New("native probe target does not match its address family")
	}
	lookupClient := input.ExplicitLookupHTTPClient
	if lookupClient == nil {
		lookupClient = input.HTTPClient
	}
	engine := &nativeEngine{
		input: input, http: probeHTTP{client: input.HTTPClient},
		explicitLookupHTTP: probeHTTP{client: lookupClient},
	}
	basic := engine.probeBasic(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	providers := engine.probeProviders(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	media := engine.probeMedia(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mail := engine.probeMail(ctx, target)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report := engine.buildReport(target, basic, providers, media, mail)
	raw, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("encode native probe report: %w", err)
	}
	return raw, nil
}

func (engine *nativeEngine) buildReport(
	target netip.Addr,
	basic basicFinding,
	providers map[string]providerFinding,
	media map[string]mediaFinding,
	mail mailFinding,
) nativeReport {
	started := engine.input.StartedAt
	if started.IsZero() {
		started = time.Now()
	}
	chinaStandardTime := time.FixedZone("CST", 8*60*60)
	report := nativeReport{
		Head: map[string]any{
			"IP": maskAddress(target), "Command": "ipchronicle-agent native probe",
			"GitHub":  "https://github.com/ipchronicle/ipchronicle",
			"Time":    started.In(chinaStandardTime).Format("2006-01-02 15:04:05 MST"),
			"Version": nativeProbeVersion,
		},
		Info: map[string]any{
			"ASN": optionalText(basic.ASN), "Organization": optionalText(basic.Organization),
			"Latitude": optionalText(basic.Latitude), "Longitude": optionalText(basic.Longitude),
			"DMS": optionalText(basic.DMS), "Map": optionalText(basic.Map),
			"TimeZone": optionalText(basic.TimeZone),
			"City": map[string]any{
				"Name": optionalText(basic.City), "PostalCode": optionalText(basic.PostalCode),
				"SubCode": optionalText(basic.SubCode), "Subdivisions": optionalText(basic.Subdivisions),
			},
			"Region": map[string]any{
				"Code": optionalText(basic.RegionCode), "Name": optionalText(basic.RegionName),
			},
			"Continent": map[string]any{
				"Code": optionalText(basic.ContinentCode), "Name": optionalText(basic.ContinentName),
			},
			"RegisteredRegion": map[string]any{
				"Code": optionalText(basic.RegisteredRegionCode),
				"Name": optionalText(basic.RegisteredRegionName),
			},
			"Type": optionalText(basic.Type),
		},
		Type: map[string]any{
			"Usage": map[string]any{
				"IPinfo":      providerType(providers["IPinfo"].Usage),
				"ipregistry":  providerType(providers["ipregistry"].Usage),
				"ipapi":       providerType(providers["ipapi"].Usage),
				"AbuseIPDB":   providerType(providers["AbuseIPDB"].Usage),
				"IP2LOCATION": providerType(providers["IP2LOCATION"].Usage),
			},
			"Company": map[string]any{
				"IPinfo":     providerType(providers["IPinfo"].Company),
				"ipregistry": providerType(providers["ipregistry"].Company),
				"ipapi":      providerType(providers["ipapi"].Company),
			},
		},
		Score: map[string]any{
			"IP2LOCATION": scoreText(providers["IP2LOCATION"]),
			"SCAMALYTICS": scoreText(providers["SCAMALYTICS"]),
			"ipapi":       scoreText(providers["ipapi"]),
			"AbuseIPDB":   scoreText(providers["AbuseIPDB"]),
			"IPQS":        scoreText(providers["IPQS"]),
			"DBIP":        scoreText(providers["DBIP"]),
		},
		Factor: buildFactors(providers),
		Media:  buildMedia(media),
		Mail:   buildMail(mail),
	}
	return report
}

func buildFactors(providers map[string]providerFinding) map[string]any {
	providerNames := []string{
		"IP2LOCATION", "ipapi", "ipregistry", "IPQS", "SCAMALYTICS",
		"ipdata", "IPinfo", "DBIP",
	}
	result := make(map[string]any, 7)
	countries := make(map[string]any, len(providerNames))
	for _, name := range providerNames {
		country := strings.ToUpper(strings.TrimSpace(providers[name].CountryCode))
		if len(country) == 2 {
			countries[name] = country
		} else {
			countries[name] = nil
		}
	}
	result["CountryCode"] = countries
	for _, factor := range []string{"Proxy", "Tor", "VPN", "Server", "Abuser", "Robot"} {
		values := make(map[string]any, len(providerNames))
		for _, name := range providerNames {
			values[name] = providers[name].factor(factor)
		}
		result[factor] = values
	}
	return result
}

func buildMedia(findings map[string]mediaFinding) map[string]any {
	result := make(map[string]any, len(findings))
	for _, name := range []string{
		"TikTok", "DisneyPlus", "Netflix", "Youtube", "AmazonPrimeVideo", "Reddit", "ChatGPT",
	} {
		finding := findings[name]
		result[name] = map[string]any{
			"Status": optionalText(finding.Status),
			"Region": optionalText(finding.Region),
			"Type":   optionalText(finding.Type),
		}
	}
	return result
}

func buildMail(finding mailFinding) map[string]any {
	result := make(map[string]any, len(finding.Services)+2)
	result["Port25"] = finding.Port25
	for _, service := range mailServices {
		result[service.Name] = finding.Services[service.Name]
	}
	result["DNSBlacklist"] = map[string]any{
		"Total": finding.DNSBlacklist.Total, "Clean": finding.DNSBlacklist.Clean,
		"Marked": finding.DNSBlacklist.Marked, "Blacklisted": finding.DNSBlacklist.Blacklisted,
	}
	return result
}

func maskAddress(address netip.Addr) string {
	if address.Is4() {
		parts := strings.Split(address.String(), ".")
		return parts[0] + "." + parts[1] + ".*.*"
	}
	value := address.As16()
	first := uint16(value[0])<<8 | uint16(value[1])
	second := uint16(value[2])<<8 | uint16(value[3])
	third := uint16(value[4])<<8 | uint16(value[5])
	return fmt.Sprintf("%x:%x:%x:*:*:*:*:*", first, second, third)
}

func optionalText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func providerType(value string) any {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	switch value {
	case "business", "commercial", "com":
		return "商业"
	case "isp", "fixed line isp", "line isp":
		return "家宽"
	case "hosting", "data center/web hosting/transit", "dch":
		return "机房"
	case "education", "university/college/school", "edu":
		return "教育"
	case "government", "gov":
		return "政府"
	case "banking":
		return "银行"
	case "organization", "org":
		return "组织"
	case "military", "mil":
		return "军队"
	case "library", "lib":
		return "图书馆"
	case "content delivery network", "cdn":
		return "CDN"
	case "mobile isp", "mob":
		return "手机"
	case "search engine spider", "ses":
		return "蜘蛛"
	case "reserved", "rsv":
		return "保留"
	default:
		return "其他"
	}
}

func scoreText(finding providerFinding) any {
	if strings.TrimSpace(finding.Score) == "" {
		return nil
	}
	return finding.Score
}

func coordinateDMS(latitude, longitude string) string {
	lat, latErr := strconv.ParseFloat(latitude, 64)
	lon, lonErr := strconv.ParseFloat(longitude, 64)
	if latErr != nil || lonErr != nil {
		return ""
	}
	format := func(value float64, positive, negative string) string {
		direction := positive
		if value < 0 {
			direction = negative
			value = math.Abs(value)
		}
		degrees := math.Floor(value)
		minutesValue := (value - degrees) * 60
		minutes := math.Floor(minutesValue)
		seconds := math.Round((minutesValue - minutes) * 60)
		return fmt.Sprintf("%.0f°%.0f′%.0f″%s", degrees, minutes, seconds, direction)
	}
	return format(lon, "E", "W") + ", " + format(lat, "N", "S")
}
