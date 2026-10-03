package domain_test

import (
	"testing"
	"time"

	"github.com/velosobr/passarim-catalog/internal/domain"
)

func TestNextAttemptDelay(t *testing.T) {
	cases := map[int]time.Duration{0: time.Minute, 1: 2 * time.Minute, 3: 8 * time.Minute, 20: 6 * time.Hour, 100: 6 * time.Hour}
	for attempt, want := range cases {
		if got := domain.NextAttemptDelay(attempt); got != want {
			t.Errorf("NextAttemptDelay(%d) = %v, want %v", attempt, got, want)
		}
	}
}
