package client

import "fmt"

type ErrUnexpectedStatusCode struct {
	Code int
}

func NewErrUnexpectedStatusCode(code int) error {
	return ErrUnexpectedStatusCode{
		Code: code,
	}
}

func (e ErrUnexpectedStatusCode) Error() string {
	return fmt.Sprintf("Unexpected status code: %d", e.Code)
}
