package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type sinfo struct {
	Uid  uint32
	Gid  uint32
	Dir  bool
	Link bool
}

func _takeOwnership(file string, table GrantTable, myuid UID, simulate bool, fileVisibleToUser bool, verbose bool) (retval int) {
	trace("_takeOwnership %s, myuid %d, simulate %t, fileVisibleToUser %t", file, myuid, simulate, fileVisibleToUser)

	// Look up file in table.
	uids, err := table.ForPath(file)
	if err != nil {
		trace("  _takeownership error looking up in table: %v", err)
		if !fileVisibleToUser {
			return Success
		}
		fmt.Fprintf(os.Stderr, "error querying delegations for %s: %v\n", file, err)
		return OperationError
	}

	// Check if file is already owned by user.
	stated, err := lstat(file)
	if err != nil {
		trace("  _takeownership error stating: %v", err)
		if !fileVisibleToUser {
			return Success
		}
		if !IsPermission(err) {
			fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, err)
			return OperationError
		}
	} else {
		if UID(stated.Uid) == myuid {
			trace("  _takeownership UID already match")
			// No need to do anything.  Return.
			if verbose {
				fmt.Printf("file %s already owned\n", file)
			}
			return Success
		}
	}

	if !uids.Has(myuid) && !canAdminChownFile(file) {
		// Unauthorized.
		trace("  _takeownership not allowed")
		if !fileVisibleToUser {
			return Success
		}
		fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, syscall.EACCES)
		return PermissionDenied
	}

	// Authorized.
	if simulate {
		if fileVisibleToUser {
			fmt.Printf("would take ownership of %s\n", file)
		}
		return Success
	}

	err = os.Lchown(string(file), int(myuid), int(stated.Gid))
	if err != nil {
		if !fileVisibleToUser {
			return Success
		}
		fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, err)
		return OperationError
	}

	if verbose {
		fmt.Printf("took ownership of %s\n", file)
	}
	return Success
}

func statAsUserIsPermitted(path string) bool {
	dropToCallingUserTemporarily()
	defer returnToRoot()

	_, err := lstat(path)
	trace("  statAsUserIsPermitted %s = %v", path, err)

	if err == nil {
		return true
	}
	return false
}

func takeOwnership(paths []string, recursive bool, simulate bool, verbose bool) (retval int) {
	trace("recursive %v, simulate %v, pathnames passed: %q", recursive, simulate, paths)
	table := NewUNIXGrantTable()
	myuid := UID(os.Getuid())

	retval = Success
	for _, file := range paths {
		if recursive {
			fn := func(path string, dentry os.DirEntry, err error) error {
				revealError := statAsUserIsPermitted(path)
				r := _takeOwnership(path, table, myuid, simulate, revealError || path == file, verbose)
				if r != Success {
					trace("  _takeownership unsuccessful: %d", r)
					retval = r | retval
					if r == PermissionDenied {

					} else if err != nil || dentry.IsDir() {
						return filepath.SkipDir
					}
				} else {
					trace("  _takeownership successful: %d", r)
				}
				return nil
			}
			filepath.WalkDir(file, fn)
		} else {
			retval = _takeOwnership(file, table, myuid, simulate, true, verbose) | retval
		}
	}
	return
}
