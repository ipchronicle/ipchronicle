package probefields

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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
		return typed
	case bool:
		if strings.HasPrefix(id, "Mail.") && !strings.HasPrefix(id, "Mail.DNSBlacklist.") {
			if typed {
				return localized(locale, "Available", "可用")
			}
			return localized(locale, "Unavailable", "不可用")
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
