package hub

import "testing"

type testWriter struct {
	writes int
	fail   bool
}

func (w *testWriter) Write(message []byte) error {
	w.writes++
	if w.fail {
		return errTest
	}
	return nil
}

func (w *testWriter) Close() error { return nil }

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "test" }

func TestHub_RegisterBroadcastUnregister(t *testing.T) {
	h := New()
	w1 := &testWriter{}
	c1 := &Connection{UserID: "u", Writer: w1}

	h.Register(c1)
	h.Broadcast("u", []byte("x"))
	if w1.writes != 1 {
		t.Fatalf("expected 1 write, got %d", w1.writes)
	}

	h.Unregister(c1)
	h.Broadcast("u", []byte("x"))
	if w1.writes != 1 {
		t.Fatalf("expected no more writes, got %d", w1.writes)
	}
}

func TestHub_RemovesFailedConnections(t *testing.T) {
	h := New()
	w1 := &testWriter{fail: true}
	c1 := &Connection{UserID: "u", Writer: w1}
	h.Register(c1)

	h.Broadcast("u", []byte("x"))
	h.Broadcast("u", []byte("x"))
	if w1.writes != 1 {
		t.Fatalf("expected only 1 write before removal, got %d", w1.writes)
	}
}
