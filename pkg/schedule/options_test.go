package schedule

import (
	"testing"
	"time"
)

func TestResolveJobNextRuntime(t *testing.T) {
	t.Run("@every", func(t *testing.T) {
		finished := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
		next := resolveJobNextRuntime("job", "@every 1m", finished)
		want := finished.Add(time.Minute)
		if !next.Equal(want) {
			t.Fatalf("next=%s want=%s", next, want)
		}
	})

	t.Run("cron standard", func(t *testing.T) {
		finished := time.Date(2026, 1, 1, 0, 1, 0, 0, time.Local)
		next := resolveJobNextRuntime("job", "*/5 * * * *", finished)
		want := time.Date(2026, 1, 1, 0, 5, 0, 0, time.Local)
		if !next.Equal(want) {
			t.Fatalf("next=%s want=%s", next, want)
		}
	})

	t.Run("descriptor", func(t *testing.T) {
		finished := time.Date(2026, 1, 1, 0, 10, 0, 0, time.Local)
		next := resolveJobNextRuntime("job", "@hourly", finished)
		want := time.Date(2026, 1, 1, 1, 0, 0, 0, time.Local)
		if !next.Equal(want) {
			t.Fatalf("next=%s want=%s", next, want)
		}
	})
}

