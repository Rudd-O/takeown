package main

import (
	"fmt"
	"os"
)

func deleteDelegation(username string, paths []string) (retval int) {
	trace("pathnames passed: %q", paths)
	dropToCallingUser()

	uid, err := userToUidOrStringUid(PotentialUsername(username))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error determining UID for user %s: %v\n", username, err)
		retval = OperationError
		return
	}
	table := NewUNIXGrantTable()
	for _, file := range paths {
		err := table.Remove(file, UID(uid))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error removing delegation for user %s on path %s: %v\n", username, file, err)
			retval = OperationError
			if IsPermission(err) {
				retval = PermissionDenied
			}
			continue
		}
	}
	return
}
