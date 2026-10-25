package service

// SignFailure contains only sanitized metadata, never raw transport errors or URLs.
type SignFailure struct {
	Kind       string
	HTTPStatus int
	Code       string
	Message    string
	Cause      error
}

func (e *SignFailure) Error() string { return e.Message }
func (e *SignFailure) Unwrap() error { return e.Cause }

// SignTaskIDKey associates upstream diagnostics with the daily task.
type SignTaskIDKey struct{}
