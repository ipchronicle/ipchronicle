package probefields

import "testing"

func TestComparableFieldsHaveLocalizedDisplayNames(t *testing.T) {
	for _, definition := range Catalog() {
		if !definition.Compare {
			continue
		}
		for _, locale := range []string{"en", "zh-CN"} {
			name, ok := DisplayName(definition.ID, locale)
			if !ok || name == "" || name == definition.ID {
				t.Fatalf("display name for %s in %s = %q, %v", definition.ID, locale, name, ok)
			}
		}
	}
}

func TestDisplayValueUsesFieldSemantics(t *testing.T) {
	tests := []struct {
		id     string
		raw    string
		locale string
		want   string
	}{
		{id: "Info.Organization", raw: `"Example Network"`, locale: "en", want: "Example Network"},
		{id: "Info.Type", raw: `"Geo-consistent"`, locale: "zh-CN", want: "原生 IP"},
		{id: "Info.Continent.Code", raw: `"AS"`, locale: "en", want: "Asia (AS)"},
		{id: "Type.Usage.ipapi", raw: `"Hosting"`, locale: "zh-CN", want: "机房"},
		{id: "Type.Company.IPinfo", raw: `"商业"`, locale: "en", want: "Business"},
		{id: "Type.Company.IPinfo", raw: `"Mobile network"`, locale: "zh-CN", want: "移动网络"},
		{id: "Type.Company.IPinfo", raw: `"搜索爬虫"`, locale: "en", want: "Search crawler"},
		{id: "Score.IPQS", raw: `"86"`, locale: "zh-CN", want: "86（存在风险）"},
		{id: "Score.ipapi", raw: `"4.00%"`, locale: "zh-CN", want: "4.00%（高）"},
		{id: "Factor.VPN.IPQS", raw: "true", locale: "zh-CN", want: "检测到"},
		{id: "Factor.VPN.IPQS", raw: "false", locale: "en", want: "Not detected"},
		{id: "Factor.CountryCode.IPQS", raw: `"US"`, locale: "en", want: "United States (US)"},
		{id: "Media.Netflix.Status", raw: `"NF.only"`, locale: "zh-CN", want: "仅自制内容"},
		{id: "Media.Netflix.Status", raw: `"解锁"`, locale: "en", want: "Unlocked"},
		{id: "Media.Netflix.Status", raw: `"仅自制内容"`, locale: "en", want: "Originals only"},
		{id: "Media.ChatGPT.Status", raw: `"Premium 不可用"`, locale: "en", want: "Premium unavailable"},
		{id: "Media.Netflix.Region", raw: `"US"`, locale: "zh-CN", want: "美国（US）"},
		{id: "Media.Netflix.Type", raw: `"原生"`, locale: "en", want: "Native"},
		{id: "Media.Netflix.Type", raw: `"经 DNS"`, locale: "en", want: "Via DNS"},
		{id: "Mail.Sohu", raw: "false", locale: "zh-CN", want: "不可用"},
		{id: "Mail.DNSBlacklist.Clean", raw: "422", locale: "en", want: "422"},
		{id: "Media.Youtube.Status", raw: "-", locale: "zh-CN", want: "无数据"},
		{id: "Media.Youtube.Status", raw: `"-"`, locale: "en", want: "No data"},
	}
	for _, test := range tests {
		if actual := DisplayValue(test.id, test.raw, test.locale); actual != test.want {
			t.Errorf("DisplayValue(%q, %q, %q) = %q, want %q", test.id, test.raw, test.locale, actual, test.want)
		}
	}
}
