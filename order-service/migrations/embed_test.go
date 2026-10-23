package migrations

import "testing"

func TestLatest(t *testing.T) {
	v, err := Latest()
	if err != nil {
		t.Fatal(err)
	}
	if v < 9 {
		t.Fatalf("ожидали последнюю миграцию не ниже 9, получили %d", v)
	}
}
