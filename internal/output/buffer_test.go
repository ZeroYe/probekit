package output

import (
	"testing"

	"probe-agent/internal/metrics"
)

func TestRingBufferPushPop(t *testing.T) {
	b := NewRingBuffer(3)

	m1 := metrics.Metric{Name: "a", Value: 1}
	m2 := metrics.Metric{Name: "b", Value: 2}
	m3 := metrics.Metric{Name: "c", Value: 3}

	b.Push(m1)
	b.Push(m2)

	if b.Len() != 2 {
		t.Fatalf("expected len 2, got %d", b.Len())
	}

	popped := b.PopN(1)
	if len(popped) != 1 || popped[0].Name != "a" {
		t.Errorf("expected [a], got %+v", popped)
	}

	if b.Len() != 1 {
		t.Fatalf("expected len 1 after pop, got %d", b.Len())
	}

	b.Push(m3)
	popped = b.PopN(10)
	if len(popped) != 2 || popped[0].Name != "b" || popped[1].Name != "c" {
		t.Errorf("expected [b c], got %+v", popped)
	}
}

func TestRingBufferOverwrite(t *testing.T) {
	b := NewRingBuffer(2)
	b.Push(metrics.Metric{Name: "a"})
	b.Push(metrics.Metric{Name: "b"})
	b.Push(metrics.Metric{Name: "c"})

	if b.Len() != 2 {
		t.Fatalf("expected len 2 after overflow, got %d", b.Len())
	}

	popped := b.PopN(2)
	if len(popped) != 2 || popped[0].Name != "b" || popped[1].Name != "c" {
		t.Errorf("expected [b c] after overflow, got %+v", popped)
	}
}

func TestRingBufferEmptyPop(t *testing.T) {
	b := NewRingBuffer(10)
	popped := b.PopN(5)
	if popped != nil {
		t.Errorf("expected nil for empty pop, got %+v", popped)
	}
}

func TestRingBufferPushBatch(t *testing.T) {
	b := NewRingBuffer(5)
	ms := []metrics.Metric{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	b.PushBatch(ms)

	if b.Len() != 3 {
		t.Fatalf("expected len 3, got %d", b.Len())
	}

	popped := b.PopN(3)
	if len(popped) != 3 || popped[0].Name != "a" || popped[2].Name != "c" {
		t.Errorf("unexpected pop result: %+v", popped)
	}
}

func TestRingBufferDefaultSize(t *testing.T) {
	b := NewRingBuffer(0)
	if b.size != 10000 {
		t.Errorf("expected default size 10000, got %d", b.size)
	}
}
