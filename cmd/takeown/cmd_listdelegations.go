package main

import (
	"fmt"
	"os"
	"strings"
)

func uidstonames(u UIDList) map[UID]string {
	result := make(map[UID]string)
	for _, uid := range u {
		name := uidToUserOrStringifiedUid(uid)
		result[uid] = string(name)
	}
	return result
}

func keys(m map[UID][]string) UIDList {
	r := UIDList{}
	for key := range m {
		r = append(r, key)
	}
	return r
}

func listDelegations(paths []string) (retval int) {
	trace("pathnames passed: %q", paths)
	dropToCallingUser()

	table := NewUNIXGrantTable()
	for _, path := range paths {
		table, err := table.Table(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading delegations for %s: %v\n", path, err)
			retval = OperationError
			if IsPermission(err) {
				retval = PermissionDenied
			}
			continue
		}
		if len(table) > 0 {
			fmt.Printf("%s:\n", path)
			uname := uidstonames(keys(table))
			for uid, p := range table {
				fmt.Printf("\t%s: via %s\n", uname[uid], strings.Join(p, ", "))
			}
		}
	}
	return
}
