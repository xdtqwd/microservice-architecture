package worker

import (
	"errors"
	"fmt"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestNextBackoff(t *testing.T) {
	base, max := 500*time.Millisecond, 30*time.Second
	d := base
	var seq []time.Duration
	for i := 0; i < 10; i++ {
		d = nextBackoff(d, base, max)
		seq = append(seq, d)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("шаг %d: %v, want %v (seq=%v)", i, seq[i], want[i], seq)
		}
	}
	if got := nextBackoff(0, base, max); got != base {
		t.Fatalf("после паузы 0 задержка должна вернуться к base, got %v", got)
	}
}

func TestIsPoison(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"сеть недоступна", errors.New("dial tcp: connection refused"), false},
		{"нет лидера партиции", kafkago.LeaderNotAvailable, false},
		{"таймаут запроса", kafkago.RequestTimedOut, false},
		{"сообщение слишком большое", kafkago.MessageSizeTooLarge, true},
		{"неверный топик", kafkago.InvalidTopic, true},
		{"обёрнутая временная", fmt.Errorf("write: %w", kafkago.LeaderNotAvailable), false},
	}
	for _, c := range cases {
		if got := isPoison(c.err); got != c.want {
			t.Errorf("%s: isPoison = %v, want %v", c.name, got, c.want)
		}
	}
}
