package probefields

import (
	"sort"
	"strings"
)

type JSONType string

const (
	JSONTypeString  JSONType = "string"
	JSONTypeNumber  JSONType = "number"
	JSONTypeBoolean JSONType = "boolean"
	JSONTypeNull    JSONType = "null"
	JSONTypeObject  JSONType = "object"
	JSONTypeArray   JSONType = "array"
)

type Definition struct {
	ID            string
	Group         string
	Path          []string
	ExpectedTypes []JSONType
	Compare       bool
}

var (
	catalog            = buildCatalog()
	comparableFieldIDs = buildComparableFieldIDs(catalog)
)

func Catalog() []Definition {
	result := make([]Definition, len(catalog))
	for index, definition := range catalog {
		result[index] = definition
		result[index].Path = append([]string(nil), definition.Path...)
		result[index].ExpectedTypes = append([]JSONType(nil), definition.ExpectedTypes...)
	}
	return result
}

func IsComparable(id string) bool {
	return comparableFieldIDs[id]
}

func buildComparableFieldIDs(definitions []Definition) map[string]bool {
	result := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if definition.Compare {
			result[definition.ID] = true
		}
	}
	return result
}

func buildCatalog() []Definition {
	definitions := make([]Definition, 0, 160)
	add := func(path string, compare bool, expected ...JSONType) {
		segments := strings.Split(path, ".")
		definitions = append(definitions, Definition{
			ID: path, Group: segments[0], Path: segments,
			ExpectedTypes: expected, Compare: compare,
		})
	}
	for _, path := range []string{
		"Head.IP", "Info.ASN", "Info.Organization", "Info.Latitude", "Info.Longitude",
		"Info.DMS", "Info.Map", "Info.TimeZone", "Info.City.Name", "Info.City.PostalCode",
		"Info.City.SubCode", "Info.City.Subdivisions", "Info.Region.Code", "Info.Region.Name",
		"Info.Continent.Code", "Info.Continent.Name", "Info.RegisteredRegion.Code",
		"Info.RegisteredRegion.Name", "Info.Type", "Type.Usage.IPinfo", "Type.Usage.ipregistry",
		"Type.Usage.ipapi", "Type.Usage.AbuseIPDB", "Type.Usage.IP2LOCATION",
		"Type.Company.IPinfo", "Type.Company.ipregistry", "Type.Company.ipapi",
		"Score.IP2LOCATION", "Score.SCAMALYTICS", "Score.ipapi", "Score.AbuseIPDB",
		"Score.IPQS", "Score.DBIP",
	} {
		add(path, true, JSONTypeString)
	}
	for _, path := range []string{"Head.Command", "Head.GitHub", "Head.Time", "Head.Version"} {
		add(path, false, JSONTypeString)
	}
	providers := []string{"IP2LOCATION", "ipapi", "ipregistry", "IPQS", "SCAMALYTICS", "ipdata", "IPinfo", "DBIP"}
	for _, provider := range providers {
		add("Factor.CountryCode."+provider, true, JSONTypeString)
		for _, factor := range []string{"Proxy", "Tor", "VPN", "Server", "Abuser", "Robot"} {
			add("Factor."+factor+"."+provider, true, JSONTypeBoolean)
		}
	}
	for _, service := range []string{"TikTok", "DisneyPlus", "Netflix", "Youtube", "AmazonPrimeVideo", "Reddit", "ChatGPT"} {
		for _, attribute := range []string{"Status", "Region", "Type"} {
			add("Media."+service+"."+attribute, true, JSONTypeString)
		}
	}
	for _, service := range []string{"Port25", "Gmail", "Outlook", "Yahoo", "Apple", "QQ", "MailRU", "AOL", "GMX", "MailCOM", "163", "Sohu", "Sina"} {
		add("Mail."+service, true, JSONTypeBoolean)
	}
	for _, field := range []string{"Total", "Clean", "Marked", "Blacklisted"} {
		add("Mail.DNSBlacklist."+field, true, JSONTypeNumber)
	}
	sort.Slice(definitions, func(left, right int) bool { return definitions[left].ID < definitions[right].ID })
	return definitions
}
