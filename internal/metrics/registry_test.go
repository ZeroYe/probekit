package metrics

import (
	"testing"
)

func TestRegistryStoreGet(t *testing.T) {
	r := NewRegistry()

	ms := []Metric{{Name: "test", Value: 1}}
	r.Store("key1", ms)

	got := r.Get("key1")
	if len(got) != 1 || got[0].Name != "test" {
		t.Errorf("Get returned unexpected: %+v", got)
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	got := r.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing key, got %+v", got)
	}
}

func TestRegistryKeys(t *testing.T) {
	r := NewRegistry()
	r.Store("b", []Metric{{Name: "b"}})
	r.Store("a", []Metric{{Name: "a"}})
	r.Store("c", []Metric{{Name: "c"}})

	keys := r.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("expected sorted [a b c], got %v", keys)
	}
}

func TestRegistryDelete(t *testing.T) {
	r := NewRegistry()
	r.Store("x", []Metric{{Name: "x"}})
	r.Delete("x")

	if got := r.Get("x"); got != nil {
		t.Errorf("expected nil after delete")
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Store("k1", []Metric{{Name: "m1", Value: 1}})
	r.Store("k2", []Metric{{Name: "m2", Value: 2}})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(all))
	}

	all["k1"][0].Value = 99

	original := r.Get("k1")
	if original[0].Value != 1 {
		t.Errorf("All returned non-copied data")
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(n int) {
			key := string(rune('a' + n))
			r.Store(key, []Metric{{Name: key, Value: float64(n)}})
			_ = r.Get(key)
			_ = r.Keys()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
