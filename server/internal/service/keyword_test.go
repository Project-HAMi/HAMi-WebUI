package service

import "testing"

func TestMatchesKeyword(t *testing.T) {
	fields := []string{"Training-Job_7", "worker-0", "team-a"}
	for _, tc := range []struct {
		keyword string
		want    bool
	}{
		{"", true},
		{"   ", true},
		{"TRAINING", true},
		{"job_7 worker", true},
		{"trainingjob7", true},
		{"worker team-a", true},
		{"job7team", false},
		{"worker-0 missing", false},
		{"--", false},
	} {
		if got := matchesKeyword(tc.keyword, fields...); got != tc.want {
			t.Errorf("matchesKeyword(%q) = %v, want %v", tc.keyword, got, tc.want)
		}
	}
	if matchesKeyword("node-1", "node1") {
		t.Error("separators typed in a keyword must stay literal")
	}
	if !matchesKeyword("node1", "node-1") {
		t.Error("fields must also match without separators")
	}
}
