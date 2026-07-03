package validator

import "testing"

func TestValidatorValid(t *testing.T) {
	v := New()
	if !v.Valid() {
		t.Fatal("expected new validator to be valid")
	}
	v.AddError("field", "boom")
	if v.Valid() {
		t.Fatal("expected validator to be invalid after AddError")
	}
}

func TestValidatorCheck(t *testing.T) {
	v := New()
	v.Check(false, "field", "must be provided")
	if got := v.Errors["field"]; got != "must be provided" {
		t.Fatalf("got %q, want %q", got, "must be provided")
	}
}

func TestPermittedValue(t *testing.T) {
	if !PermittedValue("a", "a", "b") {
		t.Fatal("expected a to be permitted")
	}
	if PermittedValue("z", "a", "b") {
		t.Fatal("expected z to be rejected")
	}
}

func TestUnique(t *testing.T) {
	if !Unique([]string{"a", "b"}) {
		t.Fatal("expected unique")
	}
	if Unique([]string{"a", "a"}) {
		t.Fatal("expected non-unique")
	}
}
