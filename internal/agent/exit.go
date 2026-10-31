package agent

import "errors"

// ExitNotAuthenticated is the print-mode exit code for missing or rejected credentials.
const ExitNotAuthenticated = 2

type notAuthenticatedError struct{ message string }

func (e notAuthenticatedError) Error() string { return e.message }

// NotAuthenticatedError reports missing or rejected Rootly credentials.
func NotAuthenticatedError(message string) error {
	return notAuthenticatedError{message: message}
}

// IsNotAuthenticated reports whether err comes from missing or rejected credentials.
func IsNotAuthenticated(err error) bool {
	var target notAuthenticatedError
	return errors.As(err, &target)
}

// ExitCodeError carries a process exit code for the command runner.
type ExitCodeError struct {
	Err  error
	Code int
}

func (e ExitCodeError) Error() string { return e.Err.Error() }

func (e ExitCodeError) Unwrap() error { return e.Err }

// ExitCode returns the process exit code.
func (e ExitCodeError) ExitCode() int { return e.Code }

// PrintExitError maps print-mode errors to exit codes: 2 when not authenticated, 1 otherwise.
func PrintExitError(err error) error {
	if err != nil && IsNotAuthenticated(err) {
		return ExitCodeError{Err: err, Code: ExitNotAuthenticated}
	}
	return err
}
