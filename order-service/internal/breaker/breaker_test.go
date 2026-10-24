package breaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTest(threshold int, openFor time.Duration) (*Breaker, *clock, *[]string) {
	c := &clock{t: time.Unix(0, 0)}
	var log []string
	b := New("test", threshold, openFor, WithClock(c.now), WithOnChange(func(_ string, from, to State) {
		log = append(log, from.String()+"->"+to.String())
	}))
	return b, c, &log
}

func TestClosed_OpensAfterThresholdConsecutiveFailures(t *testing.T) {
	b, _, _ := newTest(3, time.Second)
	for i := 0; i < 2; i++ {
		require.NoError(t, b.Allow())
		b.Done(false)
	}
	assert.Equal(t, Closed, b.State(), "две ошибки из трёх — ещё замкнут")

	require.NoError(t, b.Allow())
	b.Done(false)
	assert.Equal(t, Open, b.State())
}

func TestClosed_SuccessResetsCounter(t *testing.T) {
	b, _, _ := newTest(3, time.Second)
	for round := 0; round < 3; round++ {
		for i := 0; i < 2; i++ {
			require.NoError(t, b.Allow())
			b.Done(false)
		}
		require.NoError(t, b.Allow())
		b.Done(true) // успех обнуляет счёт — ошибки должны идти подряд
	}
	assert.Equal(t, Closed, b.State())
}

func TestOpen_RejectsUntilOpenForPasses(t *testing.T) {
	b, c, _ := newTest(1, 5*time.Second)
	require.NoError(t, b.Allow())
	b.Done(false)

	assert.ErrorIs(t, b.Allow(), ErrOpen)
	c.advance(4999 * time.Millisecond)
	assert.ErrorIs(t, b.Allow(), ErrOpen, "за миллисекунду до конца паузы — всё ещё разомкнут")

	c.advance(time.Millisecond)
	assert.NoError(t, b.Allow(), "пауза вышла — пропускаем пробу")
	assert.Equal(t, HalfOpen, b.State())
}

func TestHalfOpen_OnlyOneProbe(t *testing.T) {
	b, c, _ := newTest(1, time.Second)
	require.NoError(t, b.Allow())
	b.Done(false)
	c.advance(time.Second)

	require.NoError(t, b.Allow(), "первый — проба")
	assert.ErrorIs(t, b.Allow(), ErrOpen, "пока проба идёт, остальных не пускаем")
	assert.ErrorIs(t, b.Allow(), ErrOpen)
}

func TestHalfOpen_ProbeSuccessCloses(t *testing.T) {
	b, c, log := newTest(1, time.Second)
	require.NoError(t, b.Allow())
	b.Done(false)
	c.advance(time.Second)

	require.NoError(t, b.Allow())
	b.Done(true)

	assert.Equal(t, Closed, b.State())
	assert.NoError(t, b.Allow(), "после восстановления вызовы идут как обычно")
	assert.Equal(t, []string{"closed->open", "open->half_open", "half_open->closed"}, *log)
}

func TestHalfOpen_ProbeFailureReopensForFullPause(t *testing.T) {
	b, c, log := newTest(1, time.Second)
	require.NoError(t, b.Allow())
	b.Done(false)
	c.advance(time.Second)

	require.NoError(t, b.Allow())
	b.Done(false)
	assert.Equal(t, Open, b.State())

	c.advance(999 * time.Millisecond)
	assert.ErrorIs(t, b.Allow(), ErrOpen, "пауза отсчитывается заново от неудачной пробы")
	assert.Equal(t, []string{"closed->open", "open->half_open", "half_open->open"}, *log)
}

func TestOpen_LateResultIgnored(t *testing.T) {
	b, _, _ := newTest(2, time.Second)
	require.NoError(t, b.Allow()) // вызов A начат
	require.NoError(t, b.Allow()) // вызов B начат
	b.Done(false)
	b.Done(false) // разомкнулись
	require.Equal(t, Open, b.State())

	b.Done(true) // запоздавший успех вызова, начатого раньше
	assert.Equal(t, Open, b.State(), "результат старого вызова не замыкает предохранитель")
}
