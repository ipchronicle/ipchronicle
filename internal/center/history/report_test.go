package history

import (
	"errors"
	"testing"
)

func TestInterpretDistinguishesMissingIncompatibleAndUnknownFields(t *testing.T) {
	report, err := Interpret([]byte(`{
		"Head":{"IP":"203.0.113.*","Version":7},
		"Info":{"ASN":"64500","Organization":null,"City":null,"NewField":"new","NewNull":null},
		"Factor":{"Proxy":{"DBIP":null}},
		"Mail":{"DNSBlacklist":{"Total":4}},
		"Extra":{"Nested":true}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	assertIssue(t, report, "Head.Version", "incompatible", JSONTypeNumber)
	assertIssue(t, report, "Info.Latitude", "missing", "")
	assertIssue(t, report, "Info.NewField", "unknown", JSONTypeString)
	assertIssue(t, report, "Extra.Nested", "unknown", JSONTypeBoolean)
	assertNoIssue(t, report, "Info.Organization")
	assertNoIssue(t, report, "Info.City.Name")
	assertNoIssue(t, report, "Info.NewNull")
	assertUnavailable(t, report, "Info.Organization")
	assertUnavailable(t, report, "Info.City.Name")
	assertUnavailable(t, report, "Factor.Proxy.DBIP")
	assertAvailable(t, report, "Mail.DNSBlacklist.Total", "4", JSONTypeNumber)
}

func TestCompareOnlyUsesCompatibleComparableValues(t *testing.T) {
	before, err := Interpret([]byte(`{"Head":{"IP":"one","Time":"old"},"Info":{"ASN":"1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Interpret([]byte(`{"Head":{"IP":"two","Time":"new"},"Info":{"ASN":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	changes := Compare(before, after)
	if len(changes) != 1 || changes[0].FieldID != "Head.IP" || changes[0].Before != "one" || changes[0].After != "two" {
		t.Fatalf("changes = %#v", changes)
	}
}

func TestCompareDoesNotTreatNullAvailabilityTransitionsAsFieldChanges(t *testing.T) {
	before, err := Interpret([]byte(`{"Info":{"ASN":null,"Organization":"Example"}}`))
	if err != nil {
		t.Fatal(err)
	}
	after, err := Interpret([]byte(`{"Info":{"ASN":"64500","Organization":null}}`))
	if err != nil {
		t.Fatal(err)
	}
	if changes := Compare(before, after); len(changes) != 0 {
		t.Fatalf("null availability transitions produced changes: %#v", changes)
	}
}

func TestInterpretRejectsNonObjectJSON(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `"text"`} {
		if _, err := Interpret([]byte(raw)); !errors.Is(err, ErrInvalidReportJSON) {
			t.Fatalf("Interpret(%s) error = %v", raw, err)
		}
	}
}

func TestCatalogTracksCurrentRedditMediaFields(t *testing.T) {
	definitions := Catalog()
	found := map[string]bool{}
	for _, definition := range definitions {
		found[definition.ID] = true
	}
	for _, id := range []string{
		"Media.Reddit.Status",
		"Media.Reddit.Region",
		"Media.Reddit.Type",
	} {
		if !found[id] {
			t.Errorf("catalog is missing %s", id)
		}
	}
	for _, id := range []string{
		"Media.Spotify.Status",
		"Media.Spotify.Region",
		"Media.Spotify.Type",
	} {
		if found[id] {
			t.Errorf("catalog still contains obsolete field %s", id)
		}
	}
}

func assertIssue(t *testing.T, report Report, path, kind string, actual JSONType) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Path == path && issue.Kind == kind {
			if actual != "" && (issue.ActualType == nil || *issue.ActualType != actual) {
				t.Fatalf("issue %s actual type = %v", path, issue.ActualType)
			}
			return
		}
	}
	t.Fatalf("missing issue %s/%s in %#v", path, kind, report.Issues)
}

func assertNoIssue(t *testing.T, report Report, path string) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Path == path {
			t.Fatalf("unexpected issue for %s: %#v", path, issue)
		}
	}
}

func assertUnavailable(t *testing.T, report Report, id string) {
	t.Helper()
	for _, field := range report.Fields {
		if field.ID == id {
			if field.Status != "unavailable" || field.Value != nil ||
				field.ActualType == nil || *field.ActualType != JSONTypeNull {
				t.Fatalf("field %s = %#v", id, field)
			}
			if containsType(field.ExpectedTypes, JSONTypeNull) {
				t.Fatalf("field %s still declares null as a semantic value: %#v", id, field)
			}
			return
		}
	}
	t.Fatalf("missing field %s", id)
}

func assertAvailable(t *testing.T, report Report, id, value string, actual JSONType) {
	t.Helper()
	for _, field := range report.Fields {
		if field.ID == id {
			if field.Status != "available" || field.Value == nil || *field.Value != value ||
				field.ActualType == nil || *field.ActualType != actual {
				t.Fatalf("field %s = %#v", id, field)
			}
			return
		}
	}
	t.Fatalf("missing field %s", id)
}
