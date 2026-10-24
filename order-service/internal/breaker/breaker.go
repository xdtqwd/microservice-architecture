// Package breaker — предохранитель вокруг ненадёжной зависимости.
//
//	Closed   — вызовы идут; ошибки подряд считаются, после threshold -> Open
//	Open     — вызовы не идут вообще; через openFor -> HalfOpen
//	HalfOpen — пропускаем ровно один пробный вызов:
//	           успех -> Closed, ошибка -> снова Open на openFor
package breaker

import (
	"errors"
	"sync"
	"time"
)

type State int32

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	}
	return "unknown"
}

// ErrOpen — вызов не выполнен, потому что предохранитель разомкнут.
var ErrOpen = errors.New("circuit breaker is open")

type Breaker struct {
	name      string
	threshold int
	openFor   time.Duration
	now       func() time.Time
	onChange  func(name string, from, to State)

	mu       sync.Mutex
	state    State
	failures int
	openedAt time.Time
	probing  bool
}

type Option func(*Breaker)

// WithClock подменяет часы — для тестов.
func WithClock(now func() time.Time) Option { return func(b *Breaker) { b.now = now } }

// WithOnChange вызывается при каждом переходе состояния (под мьютексом).
func WithOnChange(f func(name string, from, to State)) Option {
	return func(b *Breaker) { b.onChange = f }
}

func New(name string, threshold int, openFor time.Duration, opts ...Option) *Breaker {
	if threshold < 1 {
		threshold = 1
	}
	b := &Breaker{name: name, threshold: threshold, openFor: openFor, now: time.Now}
	for _, o := range opts {
		o(b)
	}
	return b
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Allow разрешает вызов или возвращает ErrOpen.
// После каждого разрешённого вызова обязательно вызвать Done.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case Open:
		if b.now().Sub(b.openedAt) < b.openFor {
			return ErrOpen
		}
		b.setState(HalfOpen)
		b.probing = true // этот вызов и есть проба
		return nil
	case HalfOpen:
		if b.probing {
			return ErrOpen // проба уже идёт, остальных не пускаем
		}
		b.probing = true
		return nil
	}
	return nil
}

// Done сообщает результат разрешённого вызова.
// success=false — только отказ самой зависимости, не бизнес-результат вроде промаха кеша.
func (b *Breaker) Done(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case Closed:
		if success {
			b.failures = 0
			return
		}
		b.failures++
		if b.failures >= b.threshold {
			b.trip()
		}
	case HalfOpen:
		b.probing = false
		if success {
			b.failures = 0
			b.setState(Closed)
		} else {
			b.trip()
		}
	case Open:
		// результат вызова, начатого до размыкания, — ничего не меняет
	}
}

func (b *Breaker) trip() {
	b.openedAt = b.now()
	b.failures = 0
	b.setState(Open)
}

func (b *Breaker) setState(to State) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	if b.onChange != nil {
		b.onChange(b.name, from, to)
	}
}
