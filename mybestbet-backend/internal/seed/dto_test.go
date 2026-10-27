package seed

import "testing"

func TestStatusFromSofascore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected EventStatus
	}{
		{"notstarted", EventScheduled},
		{"inprogress", EventLive},
		{"finished", EventFinished},
		{"postponed", EventPostponed},
		{"canceled", EventCancelled},
		{"delayed", EventScheduled},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := StatusFromSofascore(tt.input); got != tt.expected {
				t.Errorf("StatusFromSofascore(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
