package data

import "testing"

func TestFiltersLimitOffset(t *testing.T) {
	f := Filters{Page: 3, PageSize: 20}
	if f.limit() != 20 {
		t.Fatalf("limit = %d, want 20", f.limit())
	}
	if f.offset() != 40 {
		t.Fatalf("offset = %d, want 40", f.offset())
	}
}

func TestFiltersSort(t *testing.T) {
	f := Filters{Sort: "-year", SortSafelist: []string{"year", "-year"}}
	if f.sortColumn() != "year" {
		t.Fatalf("sortColumn = %q, want year", f.sortColumn())
	}
	if f.sortDirection() != "DESC" {
		t.Fatalf("sortDirection = %q, want DESC", f.sortDirection())
	}
}

func TestFiltersSortAscending(t *testing.T) {
	f := Filters{Sort: "year", SortSafelist: []string{"year", "-year"}}
	if f.sortColumn() != "year" {
		t.Fatalf("sortColumn = %q, want year", f.sortColumn())
	}
	if f.sortDirection() != "ASC" {
		t.Fatalf("sortDirection = %q, want ASC", f.sortDirection())
	}
}
