package redactor

import (
	"strings"
	"testing"

	"github.com/leaktk/leaktk/pkg/proto"
)

func testResponse(spans ...[2]int) *proto.Response {
	results := make([]*proto.Result, 0, len(spans))

	for _, s := range spans {
		results = append(results, &proto.Result{
			Location: proto.Location{
				Start: proto.Point{
					Line:   1,
					Column: int(s[0] + 1),
				},
				End: proto.Point{
					Line:   1,
					Column: int(s[1]),
				},
			},
		})
	}

	return &proto.Response{
		Results: results,
	}
}

func newTestRedactor() *Redactor {
	return &Redactor{
		RedactionWord: "[REDACTED]",
		RedactionMark: "*",
	}
}

func TestRedactText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		response *proto.Response
		expected string
	}{
		{
			name:     "no findings",
			input:    "hello world",
			response: &proto.Response{},
			expected: "hello world",
		},
		{
			name:     "single finding",
			input:    "hello secret world",
			response: testResponse([2]int{6, 12}),
			expected: "hello [REDACTED] world",
		},
		{
			name:  "multiple findings",
			input: "secret abc secret",
			response: testResponse(
				[2]int{0, 6},
				[2]int{11, 17},
			),
			expected: "[REDACTED] abc [REDACTED]",
		},
		{
			name:     "secret at beginning",
			input:    "secret hello",
			response: testResponse([2]int{0, 6}),
			expected: "[REDACTED] hello",
		},
		{
			name:     "secret at end",
			input:    "hello secret",
			response: testResponse([2]int{6, 12}),
			expected: "hello [REDACTED]",
		},
		{
			name:     "entire input",
			input:    "secret",
			response: testResponse([2]int{0, 6}),
			expected: "[REDACTED]",
		},
		{
			name:  "adjacent findings",
			input: "secretsecret",
			response: testResponse(
				[2]int{0, 6},
				[2]int{6, 12},
			),
			expected: "[REDACTED][REDACTED]",
		},
	}

	r := newTestRedactor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RedactText(tt.input, tt.response)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.expected {
				t.Fatalf(
					"\nexpected: %q\nactual:   %q",
					tt.expected,
					got,
				)
			}
		})
	}
}

func TestDuplicateFindings(t *testing.T) {
	r := newTestRedactor()

	input := "secret"

	response := testResponse(
		[2]int{0, 6},
		[2]int{0, 6},
	)

	got, err := r.RedactText(input, response)
	if err != nil {
		t.Fatal(err)
	}

	expected := "[REDACTED]"

	if got != expected {
		t.Fatalf(
			"expected %q got %q",
			expected,
			got,
		)
	}
}

func TestOverlappingFindings(t *testing.T) {
	r := newTestRedactor()

	input := "abcdefghijklmnop"

	response := testResponse(
		[2]int{3, 10},
		[2]int{7, 14},
	)

	got, err := r.RedactText(input, response)
	if err != nil {
		t.Fatal(err)
	}

	expected := "abc[REDACTED]op"

	if got != expected {
		t.Fatalf(
			"expected %q got %q",
			expected,
			got,
		)
	}
}

func TestEmptySpanDoesNotPanic(t *testing.T) {
	r := newTestRedactor()

	input := "hello world"

	response := testResponse(
		[2]int{5, 5},
	)

	_, err := r.RedactText(input, response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeakDoesNotRemainInOutput(t *testing.T) {
	r := newTestRedactor()

	secret := "super-secret-token"

	input := "before " + secret + " after"

	response := testResponse(
		[2]int{7, 25},
	)

	got, err := r.RedactText(input, response)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, secret) {
		t.Fatalf("secret still present in output: %q", got)
	}
}
