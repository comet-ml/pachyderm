package errutil

import (
	"strings"

	"github.com/pachyderm/pachyderm/src/client/pkg/errors"
)

var (
	// ErrBreak is an error used to break out of call back based iteration,
	// should be swallowed by iteration functions and treated as successful
	// iteration.
	ErrBreak = errors.Errorf("BREAK")
)

// IsAlreadyExistError returns true if err is due to trying to create a
// resource that already exists. It uses simple string matching, it's not
// terribly smart.
func IsAlreadyExistError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}

// IsNotFoundError returns true if err is due to a resource not being found. It
// uses simple string matching, it's not terribly smart.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}

// IsWriteToOutputBranchError returns true if the err is due to an attempt to
// write to an output repo/branch
func IsWriteToOutputBranchError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "cannot start a commit on an output branch")
}
