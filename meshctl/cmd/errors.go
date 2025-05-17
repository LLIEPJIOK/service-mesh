package cmd

import "fmt"

type ErrInvalidCode struct {
	Code int
}

func NewErrInvalidCode(code int) error {
	return ErrInvalidCode{
		Code: code,
	}
}

func (e ErrInvalidCode) Error() string {
	return fmt.Sprintf("invalid status code: %d", e.Code)
}
