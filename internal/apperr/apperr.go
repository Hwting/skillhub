package apperr

import "fmt"

type Error struct {
	Code     string
	Category string
	Message  string
	Cause    error
}

func (e *Error) Error() string {
	if e.Category == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Category, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func New(code, category, message string) *Error {
	return &Error{Code: code, Category: category, Message: message}
}

func Wrap(code, category, message string, cause error) *Error {
	return &Error{Code: code, Category: category, Message: message, Cause: cause}
}

func HTTPStatus(err error) int {
	e, ok := err.(*Error)
	if !ok {
		return 500
	}
	switch e.Code {
	case "not_found":
		return 404
	case "unauthorized":
		return 401
	case "forbidden":
		return 403
	case "validation_failed":
		return 422
	case "conflict":
		return 409
	default:
		return 500
	}
}
