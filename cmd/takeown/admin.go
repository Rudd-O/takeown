package main

import (
	"os"
	"syscall"

	"github.com/syndtr/gocapability/capability"
)

func isAdmin() bool {
	if os.Getuid() == 0 {
		return true
	}
	return false
}

func dropToCallingUser() {
	if !isAdmin() {
		uid := syscall.Getuid()
		trace("dropping privileges to calling user %d", uid)
		if err := syscall.Setuid(syscall.Getuid()); err != nil {
			panic(err)
		}
	}
}

func dropToCallingUserTemporarily() {
	if !isAdmin() {
		uid := syscall.Getuid()
		trace("dropping privileges to calling user %d", uid)
		if err := syscall.Seteuid(syscall.Getuid()); err != nil {
			panic(err)
		}
	}
}

func returnToRoot() {
	if !isAdmin() {
		trace("returning to root")
		if err := syscall.Seteuid(0); err != nil {
			panic(err)
		}
	}
}

func canAdminChownFile(file string) bool {
	dropToCallingUserTemporarily()
	defer returnToRoot()

	cap := false
	if caps, err := capability.NewPid2(os.Getpid()); err == nil {
		if caps.Load() == nil {
			cap = caps.Get(capability.EFFECTIVE, capability.CAP_CHOWN)
		}
	}
	return cap
}
