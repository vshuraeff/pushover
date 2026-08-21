package notify

import "testing"

func TestPriorityToLevel(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		want     Level
	}{
		{name: "below minimum", priority: -3, want: LevelPassive},
		{name: "minimum", priority: -2, want: LevelPassive},
		{name: "low", priority: -1, want: LevelPassive},
		{name: "normal", priority: 0, want: LevelActive},
		{name: "high", priority: 1, want: LevelTimeSensitive},
		{name: "emergency", priority: 2, want: LevelTimeSensitive},
		{name: "above maximum", priority: 3, want: LevelTimeSensitive},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PriorityToLevel(test.priority); got != test.want {
				t.Fatalf("PriorityToLevel(%d) = %q, want %q", test.priority, got, test.want)
			}
		})
	}
}

func TestSuppressSoundForLevel(t *testing.T) {
	tests := []struct {
		level Level
		want  bool
	}{
		{level: LevelPassive, want: true},
		{level: LevelActive, want: false},
		{level: LevelTimeSensitive, want: false},
		{level: Level("unknown"), want: false},
	}

	for _, test := range tests {
		if got := SuppressSoundForLevel(test.level); got != test.want {
			t.Fatalf("SuppressSoundForLevel(%q) = %t, want %t", test.level, got, test.want)
		}
	}
}
