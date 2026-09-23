package domain

import "testing"

func TestCI09MustFail(t *testing.T) {
	t.Fatal("CI-09: этот PR не должен смержиться")
}
