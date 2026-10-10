package domain

import "fmt"

// ValidationError — ввод клиента нарушает правило. Пояснение (Reason)
// публичное: оно уходит клиенту, чтобы тот понял, что исправить.
//
// Один тип на все правила, а не тип на правило: для HTTP достаточно одной
// строки в errToStatus, а различие между правилами — в тексте пояснения.
// errors.Is(err, ErrInvalidRequest) работает для любого правила.
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string { return "invalid request: " + e.Reason }

// Is делает любую ValidationError равной ErrInvalidRequest для errors.Is.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidRequest }

// Invalid создаёт ошибку валидации с публичным пояснением.
func Invalid(format string, args ...any) error {
	return &ValidationError{Reason: fmt.Sprintf(format, args...)}
}
