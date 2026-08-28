package config

import "testing"

func TestOrganizationUUID(t *testing.T) {
	t.Setenv("ORGANIZATION_UUID", "")
	if got := OrganizationUUID(); got != DefaultOrganizationUUID {
		t.Fatalf("default organization = %q", got)
	}
	t.Setenv("ORGANIZATION_UUID", "organization-test")
	if got := OrganizationUUID(); got != "organization-test" {
		t.Fatalf("configured organization = %q", got)
	}
}

func TestOrganizationDataEnabledRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("MEMBERS_DATA_LAYOUT", "")
	if OrganizationDataEnabled() {
		t.Fatal("organization layout must be opt-in")
	}
	t.Setenv("MEMBERS_DATA_LAYOUT", "organization")
	if !OrganizationDataEnabled() {
		t.Fatal("organization layout should be enabled")
	}
}
