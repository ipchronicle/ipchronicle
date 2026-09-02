package probefields

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type localizedName struct {
	English string
	Chinese string
}

var directNames = map[string]localizedName{
	"Head.IP":                       {English: "IP address", Chinese: "IP 地址"},
	"Info.ASN":                      {English: "Autonomous system number", Chinese: "自治系统编号"},
	"Info.Organization":             {English: "Organization", Chinese: "组织"},
	"Info.Latitude":                 {English: "Latitude", Chinese: "纬度"},
	"Info.Longitude":                {English: "Longitude", Chinese: "经度"},
	"Info.DMS":                      {English: "DMS coordinates", Chinese: "度分秒坐标"},
	"Info.Map":                      {English: "Map reference", Chinese: "地图引用"},
	"Info.TimeZone":                 {English: "Time zone", Chinese: "时区"},
	"Info.City.Name":                {English: "City", Chinese: "城市"},
	"Info.City.PostalCode":          {English: "Postal code", Chinese: "邮政编码"},
	"Info.City.SubCode":             {English: "City subdivision code", Chinese: "城市分区代码"},
	"Info.City.Subdivisions":        {English: "City subdivisions", Chinese: "城市分区"},
	"Info.Region.Code":              {English: "Region code", Chinese: "地区代码"},
	"Info.Region.Name":              {English: "Region", Chinese: "地区"},
	"Info.Continent.Code":           {English: "Continent code", Chinese: "洲代码"},
	"Info.Continent.Name":           {English: "Continent", Chinese: "洲"},
	"Info.RegisteredRegion.Code":    {English: "Registered region code", Chinese: "注册地区代码"},
	"Info.RegisteredRegion.Name":    {English: "Registered region", Chinese: "注册地区"},
	"Info.Type":                     {English: "Address type", Chinese: "地址类型"},
	"Mail.DNSBlacklist.Total":       {English: "DNS blocklists checked", Chinese: "已检查 DNS 黑名单"},
	"Mail.DNSBlacklist.Clean":       {English: "Clean DNS blocklists", Chinese: "正常 DNS 黑名单"},
	"Mail.DNSBlacklist.Marked":      {English: "Marked DNS blocklists", Chinese: "已标记 DNS 黑名单"},
	"Mail.DNSBlacklist.Blacklisted": {English: "Blacklisted DNS blocklists", Chinese: "已拉黑 DNS 黑名单"},
}

var factorNames = map[string]localizedName{
	"CountryCode": {English: "Country code", Chinese: "国家代码"},
	"Proxy":       {English: "Proxy indicator", Chinese: "代理指标"},
	"Tor":         {English: "Tor indicator", Chinese: "Tor 指标"},
	"VPN":         {English: "VPN indicator", Chinese: "VPN 指标"},
	"Server":      {English: "Server indicator", Chinese: "服务器指标"},
	"Abuser":      {English: "Abuse indicator", Chinese: "滥用指标"},
	"Robot":       {English: "Automation indicator", Chinese: "自动化指标"},
}

var classificationValues = map[string][2]string{
	"business":                        {"Business", "商业"},
	"commercial":                      {"Business", "商业"},
	"com":                             {"Business", "商业"},
	"商业":                              {"Business", "商业"},
	"isp":                             {"Residential ISP", "家宽"},
	"fixed line isp":                  {"Residential ISP", "家宽"},
	"line isp":                        {"Residential ISP", "家宽"},
	"residential":                     {"Residential ISP", "家宽"},
	"residential isp":                 {"Residential ISP", "家宽"},
	"家宽":                              {"Residential ISP", "家宽"},
	"hosting":                         {"Hosting", "机房"},
	"data center":                     {"Hosting", "机房"},
	"data center/web hosting/transit": {"Hosting", "机房"},
	"dch":                             {"Hosting", "机房"},
	"dch/com":                         {"Hosting", "机房"},
	"机房":                              {"Hosting", "机房"},
	"education":                       {"Education", "教育"},
	"university":                      {"Education", "教育"},
	"university/college/school":       {"Education", "教育"},
	"edu":                             {"Education", "教育"},
	"教育":                              {"Education", "教育"},
	"government":                      {"Government", "政府"},
	"gov":                             {"Government", "政府"},
	"政府":                              {"Government", "政府"},
	"banking":                         {"Banking", "银行"},
	"bank":                            {"Banking", "银行"},
	"银行":                              {"Banking", "银行"},
	"organization":                    {"Organization", "组织"},
	"org":                             {"Organization", "组织"},
	"组织":                              {"Organization", "组织"},
	"military":                        {"Military", "军队"},
	"mil":                             {"Military", "军队"},
	"军队":                              {"Military", "军队"},
	"library":                         {"Library", "图书馆"},
	"lib":                             {"Library", "图书馆"},
	"图书馆":                             {"Library", "图书馆"},
	"content delivery network":        {"CDN", "CDN"},
	"cdn":                             {"CDN", "CDN"},
	"mobile isp":                      {"Mobile network", "移动网络"},
	"mobile":                          {"Mobile network", "移动网络"},
	"mobile network":                  {"Mobile network", "移动网络"},
	"mob":                             {"Mobile network", "移动网络"},
	"手机":                              {"Mobile network", "移动网络"},
	"移动网络":                            {"Mobile network", "移动网络"},
	"search engine spider":            {"Search crawler", "搜索爬虫"},
	"search crawler":                  {"Search crawler", "搜索爬虫"},
	"web spider":                      {"Search crawler", "搜索爬虫"},
	"spider":                          {"Search crawler", "搜索爬虫"},
	"ses":                             {"Search crawler", "搜索爬虫"},
	"蜘蛛":                              {"Search crawler", "搜索爬虫"},
	"搜索爬虫":                            {"Search crawler", "搜索爬虫"},
	"reserved":                        {"Reserved", "保留"},
	"rsv":                             {"Reserved", "保留"},
	"保留":                              {"Reserved", "保留"},
	"other":                           {"Other", "其他"},
	"其他":                              {"Other", "其他"},
}

var mediaStatusValues = map[string][2]string{
	"yes":                 {"Unlocked", "解锁"},
	"true":                {"Unlocked", "解锁"},
	"available":           {"Unlocked", "解锁"},
	"unlock":              {"Unlocked", "解锁"},
	"unlocked":            {"Unlocked", "解锁"},
	"解锁":                  {"Unlocked", "解锁"},
	"no":                  {"Blocked", "屏蔽"},
	"false":               {"Blocked", "屏蔽"},
	"unavailable":         {"Blocked", "屏蔽"},
	"block":               {"Blocked", "屏蔽"},
	"blocked":             {"Blocked", "屏蔽"},
	"屏蔽":                  {"Blocked", "屏蔽"},
	"failed":              {"Check failed", "检测失败"},
	"failure":             {"Check failed", "检测失败"},
	"check failed":        {"Check failed", "检测失败"},
	"失败":                  {"Check failed", "检测失败"},
	"检测失败":                {"Check failed", "检测失败"},
	"pending":             {"Not yet supported", "待支持"},
	"not supported":       {"Not yet supported", "待支持"},
	"not yet supported":   {"Not yet supported", "待支持"},
	"待支持":                 {"Not yet supported", "待支持"},
	"nf.only":             {"Originals only", "仅自制内容"},
	"nf only":             {"Originals only", "仅自制内容"},
	"originals only":      {"Originals only", "仅自制内容"},
	"only originals":      {"Originals only", "仅自制内容"},
	"仅自制":                 {"Originals only", "仅自制内容"},
	"仅自制内容":               {"Originals only", "仅自制内容"},
	"china":               {"Mainland China", "中国大陆"},
	"mainland china":      {"Mainland China", "中国大陆"},
	"中国":                  {"Mainland China", "中国大陆"},
	"中国大陆":                {"Mainland China", "中国大陆"},
	"noprem.":             {"Premium unavailable", "Premium 不可用"},
	"no premium":          {"Premium unavailable", "Premium 不可用"},
	"premium unavailable": {"Premium unavailable", "Premium 不可用"},
	"禁会员":                 {"Premium unavailable", "Premium 不可用"},
	"premium 不可用":         {"Premium unavailable", "Premium 不可用"},
	"webonly":             {"Web only", "仅网页可用"},
	"web only":            {"Web only", "仅网页可用"},
	"仅网页":                 {"Web only", "仅网页可用"},
	"仅网页可用":               {"Web only", "仅网页可用"},
	"apponly":             {"App only", "仅 App 可用"},
	"app only":            {"App only", "仅 App 可用"},
	"仅app":                {"App only", "仅 App 可用"},
	"仅 app 可用":            {"App only", "仅 App 可用"},
	"idc":                 {"Data center", "机房"},
	"data center":         {"Data center", "机房"},
	"机房":                  {"Data center", "机房"},
}

var continentValues = map[string][2]string{
	"AF": {"Africa", "非洲"}, "AN": {"Antarctica", "南极洲"},
	"AS": {"Asia", "亚洲"}, "EU": {"Europe", "欧洲"},
	"NA": {"North America", "北美洲"}, "OC": {"Oceania", "大洋洲"},
	"SA": {"South America", "南美洲"},
}

func DisplayName(id, locale string) (string, bool) {
	if name, ok := directNames[id]; ok {
		return name.forLocale(locale), true
	}
	segments := strings.Split(id, ".")
	switch {
	case len(segments) == 3 && segments[0] == "Type" && segments[1] == "Usage":
		return localizedFormat(locale, "Usage classification (%s)", "用途分类（%s）", segments[2]), true
	case len(segments) == 3 && segments[0] == "Type" && segments[1] == "Company":
		return localizedFormat(locale, "Company classification (%s)", "公司分类（%s）", segments[2]), true
	case len(segments) == 2 && segments[0] == "Score":
		return localizedFormat(locale, "Risk score (%s)", "风险评分（%s）", segments[1]), true
	case len(segments) == 3 && segments[0] == "Factor":
		name, ok := factorNames[segments[1]]
		if !ok {
			return "", false
		}
		return localizedFormat(locale, name.English+" (%s)", name.Chinese+"（%s）", segments[2]), true
	case len(segments) == 3 && segments[0] == "Media":
		switch segments[2] {
		case "Status":
			return localizedFormat(locale, "%s availability", "%s 可用性", segments[1]), true
		case "Region":
			return localizedFormat(locale, "%s region", "%s 区域", segments[1]), true
		case "Type":
			return localizedFormat(locale, "%s result type", "%s 结果类型", segments[1]), true
		}
	case len(segments) == 2 && segments[0] == "Mail" && segments[1] != "DNSBlacklist":
		return localizedFormat(locale, "%s connectivity", "%s 连通性", segments[1]), true
	}
	return "", false
}

func DisplayValue(id, raw, locale string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "-" {
		return localized(locale, "No data", "无数据")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return trimmed
	}
	switch typed := value.(type) {
	case nil:
		return localized(locale, "No data", "无数据")
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		if normalized == "" {
			return localized(locale, "Empty", "空值")
		}
		if normalized == "-" || normalized == "n/a" || normalized == "null" {
			return localized(locale, "No data", "无数据")
		}
		return displayStringValue(id, typed, locale)
	case bool:
		if strings.HasPrefix(id, "Mail.") && !strings.HasPrefix(id, "Mail.DNSBlacklist.") {
			if typed {
				return localized(locale, "Available", "可用")
			}
			return localized(locale, "Unavailable", "不可用")
		}
		if strings.HasPrefix(id, "Factor.") {
			if typed {
				return localized(locale, "Detected", "检测到")
			}
			return localized(locale, "Not detected", "未检测到")
		}
		if typed {
			return localized(locale, "Yes", "是")
		}
		return localized(locale, "No", "否")
	case json.Number:
		return typed.String()
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return trimmed
		}
		return string(encoded)
	}
}

func displayStringValue(id, value, locale string) string {
	switch {
	case id == "Info.Type":
		return displayAddressType(value, locale)
	case strings.HasPrefix(id, "Type.Usage."), strings.HasPrefix(id, "Type.Company."):
		return displayClassification(value, locale)
	case strings.HasPrefix(id, "Score."):
		return displayRiskScore(id, value, locale)
	case isCountryCodeField(id):
		return displayCountryCode(value, locale)
	case id == "Info.Continent.Code":
		return displayContinentCode(value, locale)
	case strings.HasPrefix(id, "Media.") && strings.HasSuffix(id, ".Status"):
		return displayMediaStatus(value, locale)
	case strings.HasPrefix(id, "Media.") && strings.HasSuffix(id, ".Type"):
		return displayMediaType(value, locale)
	default:
		return value
	}
}

func displayAddressType(value, locale string) string {
	switch normalizedValue(value) {
	case "geo-consistent", "native ip", "原生ip", "原生 ip":
		return localized(locale, "Native IP", "原生 IP")
	case "geo-discrepant", "broadcast ip", "广播ip", "广播 ip":
		return localized(locale, "Broadcast IP", "广播 IP")
	default:
		return value
	}
}

func displayClassification(value, locale string) string {
	if translated, ok := classificationValues[normalizedValue(value)]; ok {
		return localized(locale, translated[0], translated[1])
	}
	return value
}

func displayMediaStatus(value, locale string) string {
	if translated, ok := mediaStatusValues[normalizedValue(value)]; ok {
		return localized(locale, translated[0], translated[1])
	}
	return value
}

func displayMediaType(value, locale string) string {
	switch normalizedValue(value) {
	case "native", "direct", "原生":
		return localized(locale, "Native", "原生")
	case "viadns", "via dns", "dns", "经 dns":
		return localized(locale, "Via DNS", "经 DNS")
	case "original", "originals", "原创", "自制内容":
		return localized(locale, "Originals", "自制内容")
	case "web", "网页":
		return localized(locale, "Web", "网页")
	default:
		return value
	}
}

func displayRiskScore(id, value, locale string) string {
	number, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "%"), 64)
	if err != nil {
		return value
	}
	provider := strings.TrimPrefix(id, "Score.")
	level := ""
	switch provider {
	case "IP2LOCATION":
		if number < 33 {
			level = localized(locale, "Low", "低")
		} else if number < 66 {
			level = localized(locale, "Medium", "中等")
		} else {
			level = localized(locale, "High", "高")
		}
	case "SCAMALYTICS":
		if number < 20 {
			level = localized(locale, "Low", "低")
		} else if number < 60 {
			level = localized(locale, "Medium", "中等")
		} else if number < 90 {
			level = localized(locale, "High", "高")
		} else {
			level = localized(locale, "Very high", "极高")
		}
	case "ipapi":
		if number < .05 {
			level = localized(locale, "Very low", "极低")
		} else if number < .85 {
			level = localized(locale, "Low", "低")
		} else if number < 3 {
			level = localized(locale, "Elevated", "较高")
		} else if number < 20 {
			level = localized(locale, "High", "高")
		} else {
			level = localized(locale, "Very high", "极高")
		}
	case "AbuseIPDB":
		if number < 25 {
			level = localized(locale, "Low", "低")
		} else if number < 75 {
			level = localized(locale, "High", "高")
		} else {
			level = localized(locale, "Block recommended", "建议封禁")
		}
	case "IPQS":
		if number < 75 {
			level = localized(locale, "Low", "低")
		} else if number < 85 {
			level = localized(locale, "Suspicious", "可疑 IP")
		} else if number < 90 {
			level = localized(locale, "Risky", "存在风险")
		} else {
			level = localized(locale, "High risk", "高风险")
		}
	case "DBIP":
		switch number {
		case 0:
			level = localized(locale, "Low", "低")
		case 50:
			level = localized(locale, "Medium", "中等")
		case 100:
			level = localized(locale, "High", "高")
		}
	}
	if level == "" {
		return value
	}
	if locale == "zh-CN" {
		return value + "（" + level + "）"
	}
	return value + " (" + level + ")"
}

func isCountryCodeField(id string) bool {
	return id == "Info.Region.Code" || id == "Info.RegisteredRegion.Code" ||
		strings.HasPrefix(id, "Factor.CountryCode.") ||
		strings.HasPrefix(id, "Media.") && strings.HasSuffix(id, ".Region")
}

func displayCountryCode(value, locale string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	region, err := language.ParseRegion(code)
	if err != nil {
		return value
	}
	tag := language.English
	if locale == "zh-CN" {
		tag = language.SimplifiedChinese
	}
	name := display.Regions(tag).Name(region)
	if name == "" || strings.EqualFold(name, code) {
		return value
	}
	if locale == "zh-CN" {
		return name + "（" + code + "）"
	}
	return name + " (" + code + ")"
}

func displayContinentCode(value, locale string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	name, ok := continentValues[code]
	if !ok {
		return value
	}
	if locale == "zh-CN" {
		return name[1] + "（" + code + "）"
	}
	return name[0] + " (" + code + ")"
}

func normalizedValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (name localizedName) forLocale(locale string) string {
	return localized(locale, name.English, name.Chinese)
}

func localized(locale, english, chinese string) string {
	if locale == "zh-CN" {
		return chinese
	}
	return english
}

func localizedFormat(locale, english, chinese, value string) string {
	return fmt.Sprintf(localized(locale, english, chinese), value)
}
