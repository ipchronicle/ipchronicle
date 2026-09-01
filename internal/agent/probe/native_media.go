// Derived in part from IPQuality at commit 0ee5f192fed70c04615852efba0e4b8bd43546c7.
// Attribution and modification details are retained in THIRD_PARTY_NOTICES.md.

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

const disneyAuthorization = "ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"

const disneyTokenForm = "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange" +
	"&latitude=0&longitude=0&platform=browser&subject_token=DISNEYASSERTION" +
	"&subject_token_type=urn%3Abamtech%3Aparams%3Aoauth%3Atoken-type%3Adevice"

const disneyGraphQL = `{"query":"mutation refreshToken($input: RefreshTokenInput!) {\n` +
	`            refreshToken(refreshToken: $input) {\n` +
	`                activeSession {\n` +
	`                    sessionId\n` +
	`                }\n` +
	`            }\n` +
	`        }","variables":{"input":{"refreshToken":"ILOVEDISNEY"}}}`

type mediaFinding struct {
	Status string
	Region string
	Type   string
}

func (engine *nativeEngine) probeMedia(ctx context.Context) map[string]mediaFinding {
	return map[string]mediaFinding{
		"TikTok":           engine.probeTikTok(ctx),
		"DisneyPlus":       engine.probeDisneyPlus(ctx),
		"Netflix":          engine.probeNetflix(ctx),
		"Youtube":          engine.probeYouTube(ctx),
		"AmazonPrimeVideo": engine.probePrimeVideo(ctx),
		"Reddit":           engine.probeReddit(ctx),
		"ChatGPT":          engine.probeChatGPT(ctx),
	}
}

func (engine *nativeEngine) probeTikTok(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "tiktok.com")
	response, err := engine.http.get(ctx, "https://www.tiktok.com/", nil)
	if err != nil {
		return mediaFinding{Status: "屏蔽"}
	}
	if strings.Contains(string(response.Body), "Please wait...") {
		response, err = engine.http.get(ctx, "https://www.tiktok.com/explore", nil)
		if err != nil {
			return mediaFinding{Status: "屏蔽"}
		}
	}
	if region := firstPattern(response.Body, `"region"\s*:\s*"([^"]+)"`); region != "" {
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	}
	headers := make(http.Header)
	headers.Set("Accept-Language", "en")
	response, err = engine.http.get(ctx, "https://www.tiktok.com/", headers)
	if err != nil {
		return mediaFinding{Status: "失败"}
	}
	if region := firstPattern(response.Body, `"region"\s*:\s*"([^"]+)"`); region != "" {
		return mediaFinding{Status: "机房", Region: region, Type: typeValue}
	}
	return mediaFinding{Status: "失败"}
}

func (engine *nativeEngine) probeDisneyPlus(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "disneyplus.com")
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+disneyAuthorization)
	headers.Set("Content-Type", "application/json; charset=UTF-8")
	device := engine.http.json(ctx, http.MethodPost, "https://disney.api.edge.bamgrid.com/devices", headers,
		[]byte(`{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`))
	assertion := documentString(device, "assertion")
	if assertion == "" {
		return mediaFinding{Status: "失败"}
	}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	token := engine.http.json(ctx, http.MethodPost, "https://disney.api.edge.bamgrid.com/token", headers,
		[]byte(strings.Replace(disneyTokenForm, "DISNEYASSERTION", assertion, 1)))
	if documentString(token, "error_description") == "forbidden-location" {
		return mediaFinding{Status: "屏蔽"}
	}
	refreshToken := documentString(token, "refresh_token")
	if refreshToken == "" {
		return mediaFinding{Status: "失败"}
	}
	headers.Set("Authorization", disneyAuthorization)
	headers.Set("Content-Type", "application/json")
	session := engine.http.json(ctx, http.MethodPost,
		"https://disney.api.edge.bamgrid.com/graph/v1/device/graphql", headers,
		[]byte(strings.Replace(disneyGraphQL, "ILOVEDISNEY", refreshToken, 1)))
	region := documentString(session, "extensions", "sdk", "session", "location", "countryCode")
	supported := documentBool(session, "extensions", "sdk", "session", "inSupportedLocation")
	if region == "" {
		return mediaFinding{Status: "屏蔽"}
	}
	preview, _ := engine.http.get(ctx, "https://disneyplus.com", nil)
	unavailable := strings.Contains(preview.FinalURL, "unavailable")
	if region == "JP" || supported != nil && *supported {
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	}
	if supported != nil && !*supported && !unavailable {
		return mediaFinding{Status: "待支持", Region: region, Type: typeValue}
	}
	return mediaFinding{Status: "屏蔽"}
}

func (engine *nativeEngine) probeNetflix(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "netflix.com")
	first, firstErr := engine.http.get(ctx, "https://www.netflix.com/title/81280792", nil)
	second, secondErr := engine.http.get(ctx, "https://www.netflix.com/title/70143836", nil)
	if firstErr != nil || secondErr != nil || len(first.Body) == 0 || len(second.Body) == 0 {
		return mediaFinding{Status: "失败"}
	}
	region := mediaRegion(first.Body)
	if region == "" {
		region = mediaRegion(second.Body)
	}
	firstOriginal := strings.Contains(string(first.Body), "Oh no!")
	secondOriginal := strings.Contains(string(second.Body), "Oh no!")
	if firstOriginal && secondOriginal {
		return mediaFinding{Status: "仅自制", Region: region, Type: typeValue}
	}
	if !firstOriginal || !secondOriginal {
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	}
	return mediaFinding{Status: "屏蔽"}
}

func (engine *nativeEngine) probeYouTube(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "www.youtube.com")
	headers := make(http.Header)
	headers.Set("Accept-Language", "en")
	headers.Set("Cookie", "CONSENT=YES+cb.20220301-11-p0.en+FX+700; PREF=tz=Asia.Shanghai")
	response, err := engine.http.get(ctx, "https://www.youtube.com/premium", headers)
	if err != nil {
		return mediaFinding{Status: "失败"}
	}
	body := string(response.Body)
	if strings.Contains(body, "www.google.cn") {
		return mediaFinding{Status: "中国", Region: "CN"}
	}
	if strings.Contains(body, "Premium is not available in your country") {
		return mediaFinding{Status: "禁会员"}
	}
	region := firstPattern(response.Body, `"contentRegion"\s*:\s*"([^"]+)"`)
	if strings.Contains(body, "ad-free") {
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	}
	return mediaFinding{Status: "失败"}
}

func (engine *nativeEngine) probePrimeVideo(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "www.primevideo.com")
	response, err := engine.http.get(ctx, "https://www.primevideo.com", nil)
	if err != nil {
		return mediaFinding{Status: "失败"}
	}
	region := firstPattern(response.Body, `"currentTerritory"\s*:\s*"([^"]+)"`)
	if region == "" {
		return mediaFinding{Status: "屏蔽"}
	}
	return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
}

func (engine *nativeEngine) probeReddit(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "reddit.com")
	response, err := engine.http.get(ctx, "https://www.reddit.com/svc/shreddit/reddit-chat", nil)
	if err != nil {
		return mediaFinding{Status: "失败"}
	}
	switch response.StatusCode {
	case http.StatusOK:
		return mediaFinding{Status: "解锁", Region: firstPattern(response.Body, `country="([^"]+)"`), Type: typeValue}
	case http.StatusForbidden:
		return mediaFinding{Status: "屏蔽"}
	default:
		return mediaFinding{Status: "失败"}
	}
}

func (engine *nativeEngine) probeChatGPT(ctx context.Context) mediaFinding {
	typeValue := mediaUnlockType(ctx, "chat.openai.com", "ios.chat.openai.com", "api.openai.com")
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer null")
	headers.Set("Origin", "https://platform.openai.com")
	apiResponse, apiErr := engine.http.get(ctx, "https://api.openai.com/compliance/cookie_requirements", headers)
	appResponse, appErr := engine.http.get(ctx, "https://ios.chat.openai.com/", nil)
	apiBlocked := apiErr == nil && strings.Contains(string(apiResponse.Body), "unsupported_country")
	appBlocked := appErr == nil && strings.Contains(string(appResponse.Body), "VPN")
	if apiBlocked {
		favicon, err := engine.http.get(ctx, "https://chatgpt.com/favicon.ico", nil)
		if err == nil && favicon.StatusCode != http.StatusForbidden {
			apiBlocked = false
		}
	}
	region := engine.chatGPTRegion(ctx)
	switch {
	case apiErr == nil && appErr == nil && !apiBlocked && !appBlocked:
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	case apiBlocked && appBlocked:
		return mediaFinding{Status: "屏蔽"}
	case apiErr == nil && !apiBlocked && appBlocked:
		return mediaFinding{Status: "仅网页", Region: region, Type: typeValue}
	case apiBlocked && appErr == nil && !appBlocked:
		return mediaFinding{Status: "仅APP", Region: region, Type: typeValue}
	case apiErr != nil && appBlocked:
		return mediaFinding{Status: "屏蔽"}
	case engine.input.Family == "ipv6" && appErr == nil && !appBlocked:
		return mediaFinding{Status: "解锁", Region: region, Type: typeValue}
	default:
		return mediaFinding{Status: "失败"}
	}
}

func (engine *nativeEngine) chatGPTRegion(ctx context.Context) string {
	response, err := engine.http.get(ctx, "https://chat.openai.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(response.Body), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "loc="); ok {
			return value
		}
	}
	return ""
}

func mediaUnlockType(ctx context.Context, domains ...string) string {
	resolver := net.DefaultResolver
	for _, domain := range domains {
		addresses, err := resolver.LookupIPAddr(ctx, domain)
		if err != nil || !containsPublicAddress(addresses) {
			return "DNS"
		}
		wildcard := fmt.Sprintf("ipchronicle-%d.%s", time.Now().UnixNano(), domain)
		if wildcardAddresses, err := resolver.LookupIPAddr(ctx, wildcard); err == nil && len(wildcardAddresses) > 0 {
			return "DNS"
		}
	}
	return "原生"
}

func containsPublicAddress(addresses []net.IPAddr) bool {
	for _, address := range addresses {
		parsed, ok := netip.AddrFromSlice(address.IP)
		if ok && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
			return true
		}
	}
	return false
}

func firstPattern(body []byte, expression string) string {
	match := regexp.MustCompile(expression).FindSubmatch(body)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}

func mediaRegion(body []byte) string {
	for _, expression := range []string{
		`"countryOfSignup"\s*:\s*"([A-Za-z]{2})"`,
		`"contentRegion"\s*:\s*"([A-Za-z]{2})"`,
		`"countryCode"\s*:\s*"([A-Za-z]{2})"`,
	} {
		if value := firstPattern(body, expression); value != "" {
			return strings.ToUpper(value)
		}
	}
	var document map[string]any
	if json.Unmarshal(body, &document) == nil {
		return strings.ToUpper(documentString(document, "country"))
	}
	return ""
}
