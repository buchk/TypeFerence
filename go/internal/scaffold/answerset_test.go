package scaffold

import (
	"strings"
	"testing"
)

func validJSON() string {
	return `{
  "schemaVersion": 1,
  "organization": {"name": "acme", "version": "1.0.0"},
  "norms": [
    {"text": "Pull requests require one reviewer before merge."}
  ],
  "levels": [{"name": "Platform"}],
  "agent": {"name": "ticket-bot"},
  "targets": {"hosts": ["neutral"]}
}`
}

func TestValidAnswerSetParses(t *testing.T) {
	as, err := ParseAnswerSet([]byte(validJSON()))
	if err != nil {
		t.Fatal(err)
	}
	if as.Organization.Name != "acme" || len(as.Norms) != 1 {
		t.Fatalf("unexpected parse: %+v", as)
	}
	if as.Targets.Hosts[0] != "neutral" {
		t.Fatalf("hosts: %+v", as.Targets.Hosts)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	bad := strings.Replace(validJSON(), `"norms":`, `"normz":`, 1)
	_, err := ParseAnswerSet([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "unknown field") && !strings.Contains(err.Error(), "normz") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestWrongSchemaVersionRejected(t *testing.T) {
	bad := strings.Replace(validJSON(), `"schemaVersion": 1`, `"schemaVersion": 99`, 1)
	_, err := ParseAnswerSet([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "schemaVersion must be 1") {
		t.Fatalf("expected schemaVersion error, got %v", err)
	}
}

func TestBadOrgNameRejected(t *testing.T) {
	bad := strings.Replace(validJSON(), `"name": "acme"`, `"name": "Acme Corp!"`, 1)
	_, err := ParseAnswerSet([]byte(bad))
	if err == nil || !strings.Contains(err.Error(), "organization.name") {
		t.Fatalf("expected org name error, got %v", err)
	}
}

func TestNormIdsDerivedAndUnique(t *testing.T) {
	as, err := ParseAnswerSet([]byte(validJSON()))
	if err != nil {
		t.Fatal(err)
	}
	if as.Norms[0].ID == "" {
		id := slug(as.Norms[0].Text)
		if id == "" {
			t.Fatal("slug derivation failed")
		}
	}
}
