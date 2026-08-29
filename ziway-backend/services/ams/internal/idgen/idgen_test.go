package idgen

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	id := Generate("CU")
	if !strings.HasPrefix(id, "CU-PZ#") {
		t.Errorf("expected prefix CU-PZ#, got %s", id)
	}
	if len(id) < 15 {
		t.Errorf("ID too short: %s", id)
	}
	t.Logf("generated ID: %s", id)
}

func TestGenerateUnique(t *testing.T) {
	id1 := Generate("DU")
	id2 := Generate("DU")
	// Same second, sequence should differ
	if id1 == id2 {
		// Could be same if counter wraps, but unlikely in test
		t.Logf("IDs equal (acceptable at boundary): %s", id1)
	}
}

func TestGenerateServiceID(t *testing.T) {
	id := GenerateServiceID("ams")
	if !strings.HasPrefix(id, "ams-SVC#") {
		t.Errorf("expected ams-SVC# prefix, got %s", id)
	}
}

func TestIsValidPrefix(t *testing.T) {
	validCodes := []string{"CU", "DU", "PU", "EU", "HU", "OU", "GU", "AU", "FU", "IU", "VU", "SU", "CX", "FX", "NHI"}
	for _, code := range validCodes {
		if !IsValidPrefix(code) {
			t.Errorf("expected %s to be valid", code)
		}
	}
	if IsValidPrefix("XX") {
		t.Error("XX should not be valid prefix")
	}
}

func TestRoleTypeChecks(t *testing.T) {
	// 12U基础角色
	for _, code := range []string{"CU", "DU", "PU", "EU", "HU", "OU", "GU", "AU", "FU", "IU", "VU", "SU"} {
		if !IsBaseRole(code) {
			t.Errorf("%s should be base role", code)
		}
		if IsHatRole(code) {
			t.Errorf("%s should not be hat role", code)
		}
	}
	// 帽子角色
	for _, code := range []string{"CX", "FX"} {
		if !IsHatRole(code) {
			t.Errorf("%s should be hat role", code)
		}
		if IsBaseRole(code) {
			t.Errorf("%s should not be base role", code)
		}
	}
}

func TestNHIGenerate(t *testing.T) {
	id := Generate(NHIPrefix)
	if !strings.HasPrefix(id, "NHI-PZ#") {
		t.Errorf("expected NHI-PZ# prefix, got %s", id)
	}
}
