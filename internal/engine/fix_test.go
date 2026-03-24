package engine

import (
	"testing"
)

func TestApplyFixes(t *testing.T) {
	t.Run("single replacement", func(t *testing.T) {
		source := []byte("var x = 1;")
		fixes := []Fix{{StartByte: 0, EndByte: 3, NewText: "const"}}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		want := "const x = 1;"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("single deletion", func(t *testing.T) {
		source := []byte("debugger; foo();")
		fixes := []Fix{{StartByte: 0, EndByte: 9, NewText: ""}}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		want := " foo();"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("multiple non-overlapping fixes", func(t *testing.T) {
		source := []byte("var x = 1;\nvar y = 2;")
		fixes := []Fix{
			{StartByte: 0, EndByte: 3, NewText: "const"},
			{StartByte: 11, EndByte: 14, NewText: "let"},
		}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		want := "const x = 1;\nlet y = 2;"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("overlapping fixes first wins", func(t *testing.T) {
		source := []byte("abcdefghij")
		fixes := []Fix{
			{StartByte: 2, EndByte: 6, NewText: "XX"},
			{StartByte: 4, EndByte: 8, NewText: "YY"},
		}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 1 {
			t.Fatalf("expected 1 conflict, got %d", len(conflicts))
		}
		want := "abXXghij"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("adjacent fixes no conflict", func(t *testing.T) {
		source := []byte("aabbcc")
		fixes := []Fix{
			{StartByte: 0, EndByte: 2, NewText: "XX"},
			{StartByte: 2, EndByte: 4, NewText: "YY"},
		}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		want := "XXYYcc"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty source", func(t *testing.T) {
		got, conflicts := ApplyFixes(nil, nil)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		if len(got) != 0 {
			t.Errorf("expected empty result, got %q", got)
		}
	})

	t.Run("empty fixes", func(t *testing.T) {
		source := []byte("hello")
		got, conflicts := ApplyFixes(source, nil)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("fixes applied in correct order regardless of input order", func(t *testing.T) {
		source := []byte("aaa bbb ccc")
		// Provide in reverse order.
		fixes := []Fix{
			{StartByte: 8, EndByte: 11, NewText: "CCC"},
			{StartByte: 0, EndByte: 3, NewText: "AAA"},
		}

		got, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}
		want := "AAA bbb CCC"
		if string(got) != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestExpandToStatement(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		source := []byte("  debugger;\n  foo();")
		s, e := expandToStatement(source, 2, 11)
		if s != 0 {
			t.Errorf("start = %d, want 0", s)
		}
		if e != 12 { // includes trailing \n
			t.Errorf("end = %d, want 12", e)
		}
	})

	t.Run("middle of file", func(t *testing.T) {
		source := []byte("a;\n  debugger;\nb;")
		s, e := expandToStatement(source, 5, 14)
		if s != 3 {
			t.Errorf("start = %d, want 3", s)
		}
		if e != 15 { // includes trailing \n
			t.Errorf("end = %d, want 15", e)
		}
	})

	t.Run("last line no trailing newline", func(t *testing.T) {
		source := []byte("a;\ndebugger;")
		s, e := expandToStatement(source, 3, 12)
		if s != 3 {
			t.Errorf("start = %d, want 3", s)
		}
		if e != 12 {
			t.Errorf("end = %d, want 12", e)
		}
	})
}
