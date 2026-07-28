package service

import "testing"

func TestMatchesWorkloadName(t *testing.T) {
	tests := []struct {
		name    string
		podName string
		filter  string
		want    bool
	}{
		{
			name:    "empty filter matches all",
			podName: "demo-workload-abc",
			want:    true,
		},
		{
			name:    "partial pod name matches",
			podName: "demo-workload-abc",
			filter:  "demo-workload",
			want:    true,
		},
		{
			name:    "unrelated name does not match",
			podName: "demo-workload-abc",
			filter:  "worker",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesWorkloadName(tt.podName, tt.filter); got != tt.want {
				t.Fatalf("matchesWorkloadName() = %v, want %v", got, tt.want)
			}
		})
	}
}
