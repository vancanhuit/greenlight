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

func TestRuntimeUnmarshalJSONInvalid(t *testing.T) {
	cases := []string{
		`"102"`,       // missing unit
		`""`,          // empty string
		`"abc mins"`,  // non-numeric quantity
		`"102 hours"`, // wrong unit
		`102`,         // not a quoted string
	}
	for _, in := range cases {
		var r Runtime
		if err := r.UnmarshalJSON([]byte(in)); err != ErrInvalidRuntimeFormat {
			t.Errorf("UnmarshalJSON(%s): got %v, want ErrInvalidRuntimeFormat", in, err)
		}
	}
}
