package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/pkg/xattr"
)

type Error struct {
	Action string
	Path   string
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Action, e.Path, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func NewError(action string, pathname string, err error) error {
	return &Error{action, pathname, err}
}

func IsPermission(err error) bool {
	var e *xattr.Error
	if errors.As(err, &e) {
		if e.Err == syscall.EPERM || e.Err == syscall.EACCES {
			return true
		}
	}
	if os.IsPermission(err) {
		return true
	}
	return false
}
