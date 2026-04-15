package storage

type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	return e.Message
}

func newStatusError(status int, message string) error {
	return &StatusError{Status: status, Message: message}
}
