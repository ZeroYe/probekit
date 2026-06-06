package collector

import (
	"testing"
)

func TestPortDefaults(t *testing.T) {
	t.Parallel()

	// itoa helper
	if itoa(0) != "0" {
		t.Errorf("itoa(0) = %q, want 0", itoa(0))
	}
	if itoa(443) != "443" {
		t.Errorf("itoa(443) = %q, want 443", itoa(443))
	}
	if itoa(8080) != "8080" {
		t.Errorf("itoa(8080) = %q, want 8080", itoa(8080))
	}
}

func TestHTTPExpectedStatus(t *testing.T) {
	t.Parallel()

	if !isExpectedStatus(200, []int{200}) {
		t.Error("expected 200 to match [200]")
	}
	if !isExpectedStatus(200, []int{200, 301}) {
		t.Error("expected 200 to match [200, 301]")
	}
	if !isExpectedStatus(301, []int{200, 301}) {
		t.Error("expected 301 to match [200, 301]")
	}
	if isExpectedStatus(500, []int{200}) {
		t.Error("expected 500 to NOT match [200]")
	}
	if isExpectedStatus(404, []int{200, 301}) {
		t.Error("expected 404 to NOT match [200, 301]")
	}
}

func TestTargetLabels(t *testing.T) {
	t.Parallel()

	labels := targetLabels("8.8.8.8", map[string]string{"region": "global"}, nil)
	if labels["target"] != "8.8.8.8" {
		t.Errorf("expected target=8.8.8.8, got %q", labels["target"])
	}
	if labels["region"] != "global" {
		t.Errorf("expected region=global, got %q", labels["region"])
	}

	labels2 := targetLabels("example.com", nil, map[string]string{"server": "1.1.1.1"})
	if labels2["target"] != "example.com" {
		t.Errorf("expected target=example.com, got %q", labels2["target"])
	}
	if labels2["server"] != "1.1.1.1" {
		t.Errorf("expected server=1.1.1.1, got %q", labels2["server"])
	}
}

func TestCopyLabels(t *testing.T) {
	t.Parallel()

	if copyLabels(nil) != nil {
		t.Error("expected nil for nil input")
	}

	orig := map[string]string{"a": "1", "b": "2"}
	copied := copyLabels(orig)
	if len(copied) != 2 {
		t.Errorf("expected 2 labels, got %d", len(copied))
	}
	orig["a"] = "changed"
	if copied["a"] != "1" {
		t.Error("copy should not be affected by original modification")
	}
}

func TestCalcLossRatio(t *testing.T) {
	t.Parallel()

	if calcLossRatio(0, 4) != 0.0 {
		t.Error("expected 0 loss ratio for 0 lost")
	}
	if calcLossRatio(1, 4) != 0.25 {
		t.Error("expected 0.25 loss ratio for 1/4")
	}
	if calcLossRatio(4, 4) != 1.0 {
		t.Error("expected 1.0 loss ratio for 4/4")
	}
	if calcLossRatio(0, 0) != 0.0 {
		t.Error("expected 0 loss ratio for 0 total")
	}
}
