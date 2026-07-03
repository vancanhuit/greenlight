package data

import "testing"

func TestRuntimeMarshalJSON(t *testing.T) {
	r := Runtime(102)
	b, err := r.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"102 mins"` {
		t.Fatalf("got %s, want %q", b, `"102 mins"`)
	}
}

func TestRuntimeUnmarshalJSON(t *testing.T) {
	var r Runtime
	if err := r.UnmarshalJSON([]byte(`"102 mins"`)); err != nil {
		t.Fatal(err)
	}
	if r != 102 {
		t.Fatalf("got %d, want 102", r)
	}
	if err := r.UnmarshalJSON([]byte(`"102"`)); err != ErrInvalidRuntimeFormat {
		t.Fatalf("expected ErrInvalidRuntimeFormat, got %v", err)
	}
}
