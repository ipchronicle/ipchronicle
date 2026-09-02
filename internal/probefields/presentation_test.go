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
		{id: "Factor.VPN.IPQS", raw: "true", locale: "zh-CN", want: "是"},
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
