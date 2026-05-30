package main

import "testing"

func TestBuildFzfFilterQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"  ", ""},
		{"拓扑学", "拓扑学"},
		{"拓扑学 试题", "'拓扑学' '试题'"},
		{"  拓扑学   2021  试题  ", "'拓扑学' '2021' '试题'"},
	}
	for _, tt := range tests {
		got := buildFzfFilterQuery(tt.in)
		if got != tt.want {
			t.Errorf("buildFzfFilterQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatchesAllTerms(t *testing.T) {
	terms := splitSearchTerms("拓扑学 试题")
	if !matchesAllTerms("拓扑学2021年考试试题.pdf", terms) {
		t.Fatal("expected filename to match all terms")
	}
	if matchesAllTerms("拓扑学2021年考试.pdf", terms) {
		t.Fatal("expected missing 试题 to fail")
	}
	if matchesAllTerms("其他试题.pdf", terms) {
		t.Fatal("expected missing 拓扑学 to fail")
	}
}
