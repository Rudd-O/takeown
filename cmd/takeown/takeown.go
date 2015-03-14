package main

import "flag"
import "fmt"
import "os"
import "path/filepath"
import "runtime"
import "strings"
import "syscall"

const (
	Success          = 0
	BadConfig        = 8
	OperationError   = 32
	Usage            = 64
	PermissionDenied = 128
)

var addFlag = flag.Bool("a", false, "add a delegation for a specific user and path")
var listFlag = flag.Bool("l", false, "list user delegations established on paths")
var deleteFlag = flag.Bool("d", false, "remove a delegation for a specific user and path")
var recurseFlag = flag.Bool("r", false, "take ownership recursively")
var simulateFlag = flag.Bool("s", false, "simulate taking ownership")
var traceFlag = flag.Bool("T", false, "show trace of internal execution; requires file `/.trace` to exist")

type SubpathRelativeToVolume string
type PotentialPathname string
type AbsolutePathname string
type AbsoluteMaybeResolvedPathname AbsolutePathname
type AbsoluteResolvedPathname AbsoluteMaybeResolvedPathname
type VolumePath AbsoluteResolvedPathname

var tracing bool

// setTrace attempts to set tracing on.  If the file /.trace does not exist,
// it fails and returns false.  Otherwise it sets tracing on and returns true.
func setTrace() bool {
	if _, err := os.Stat("/.trace"); err != nil {
		return false
	}
	tracing = true
	return true
}

func trace(s string, args ...interface{}) {
	if tracing {
		pc := make([]uintptr, 10)  // at least 1 entry needed
		runtime.Callers(2, pc)
		f := runtime.FuncForPC(pc[0])
		file, line := f.FileLine(pc[0])
		prefix := fmt.Sprintf("%s:%d %s: ", file, line, f.Name())
		fmt.Fprintf(os.Stderr, prefix + s + "\n", args...)
	}
}

func contains(container, path AbsoluteResolvedPathname) (bool, error) {
	containerx, err := filepath.Abs(string(container))
	if err != nil {
		return false, err
	}
	pathx, err := filepath.Abs(string(path))
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(pathx, containerx+string(os.PathSeparator)), nil
}

type StatedPathname struct {
	path PotentialPathname
	uid  Uid
	gid  Uid
	fsid syscall.Fsid
}

func statWithFileInfo(path PotentialPathname) (*StatedPathname, error) {
	var stated syscall.Stat_t
	err := syscall.Lstat(string(path), &stated)
	if err != nil {
		return nil, err
	}
	var statedvfs syscall.Statfs_t
	err = syscall.Statfs(string(path), &statedvfs)
	if err != nil {
		return nil, err
	}
	return &StatedPathname{path, Uid(stated.Uid), Uid(stated.Gid), statedvfs.Fsid}, nil
}

//FIXME
func absolutizePath(s PotentialPathname) (AbsolutePathname, error) {
	p, err := filepath.Abs(string(s))
	return AbsolutePathname(p), err
}

func resolveSymlinks(s AbsolutePathname) (AbsoluteResolvedPathname, error) {
	res, err := filepath.EvalSymlinks(string(s))
	if err != nil {
		return "", err
	}
	return AbsoluteResolvedPathname(res), nil
}

func stringsToPotentialPathnames(x []string) []PotentialPathname {
	var m []PotentialPathname
	for _, y := range x {
		m = append(m, PotentialPathname(y))
	}
	return m
}

func relativeToVolume(vol VolumePath, p AbsolutePathname) (SubpathRelativeToVolume, error) {
	x, err := filepath.Rel(string(vol), string(p))
	return SubpathRelativeToVolume(x), err
}

// findContainingVolume will return the volume path that contains the file
// after resolving all the symlinks intermediating access to the file.
func findContainingVolume(file AbsolutePathname) (VolumePath, error) {
	trace("pathname passed: %q", file)
	res, err := resolveSymlinks(file)
	if err != nil {
		return findContainingVolume(AbsolutePathname(filepath.Dir(string(file))))
	}
	var thisstat, parentstat syscall.Statfs_t
	thiserr := syscall.Statfs(string(res), &thisstat)
	if thiserr != nil {
		return "", fmt.Errorf("cannot statvfs %s: %v", res, err)
	}
	parenterr := syscall.Statfs(filepath.Dir(string(res)), &parentstat)
	if parenterr != nil {
		return "", fmt.Errorf("cannot statvfs %s: %v", res, parenterr)
	}
	if thisstat.Fsid == parentstat.Fsid && filepath.Dir(string(res)) != string(res) {
		return findContainingVolume(AbsolutePathname(filepath.Dir(string(res))))
	}
	return VolumePath(res), nil
}

func isAdmin() bool {
	if os.Getuid() == 0 {
		return true
	}
	return false
}

func _takeOwnership(file PotentialPathname, d *OwnershipDelegations, simulate bool) (retval int) {
	if strings.HasSuffix(string(file), string(os.PathSeparator)+TAKEOWN_STORAGE) {
		return
	}
	stated, err := statWithFileInfo(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, err)
		retval = OperationError
	} else {
		if stated.uid == Uid(os.Getuid()) {
			// Already owned by user.
			return
		}
		newUid, oldGid, err := d.CanTakeOwnership(Uid(os.Getuid()), *stated)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, err)
			retval = PermissionDenied
		} else {
			if simulate {
				fmt.Printf("would take ownership of %s\n", file)
			} else {
				err = os.Lchown(string(stated.path), int(newUid), int(oldGid))
				if err != nil {
					fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", file, err)
					retval = PermissionDenied
				}
			}
		}
	}
	return
}

func takeOwnership(paths []PotentialPathname, recursive bool, simulate bool) (retval int) {
	trace("recursive %v, simulate %v, pathnames passed: %q", paths)
	for _, file := range paths {
		d, err := LookupDelegationsForPath(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading delegations in volume of %s: %v\n", file, err)
			retval = OperationError
			continue
		}
		if recursive {
			fn := func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", path, err)
					if os.IsPermission(err) {
						retval = PermissionDenied
					} else {
						retval = OperationError
					}
					return nil
				}
				var statpath syscall.Statfs_t
				err = syscall.Statfs(string(path), &statpath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error taking ownership of %s: %v\n", path, err)
					if os.IsPermission(err) {
						retval = PermissionDenied
					} else {
						retval = OperationError
					}
					return nil
				}
				if statpath.Fsid != d.fsid {
					// We do not recurse across mountpoints.   Ever.
					return filepath.SkipDir
				}
				retval = _takeOwnership(PotentialPathname(path), d, simulate)
				return nil
			}
			filepath.Walk(string(file), fn)
		} else {
			retval = _takeOwnership(file, d, simulate)
		}
	}
	return
}

func addDelegation(username PotentialUsername, paths []PotentialPathname) (retval int) {
	trace("pathnames passed: %q", paths)
	for _, file := range paths {
		d, err := LookupDelegationsForPath(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading delegations in volume of %s: %v\n", file, err)
			retval = OperationError
			continue
		}
		err = d.Add(username, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error adding delegation for user %s on path %s: %v\n", username, file, err)
			retval = OperationError
			continue
		}
		err = d.Save()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error saving delegations: %v\n", err)
			retval = OperationError
			continue
		}
	}
	return
}

func deleteDelegation(username PotentialUsername, paths []PotentialPathname) (retval int) {
	trace("pathnames passed: %q", paths)
	for _, file := range paths {
		d, err := LookupDelegationsForPath(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading delegations in volume of %s: %v\n", file, err)
			retval = OperationError
			continue
		}
		err = d.Remove(username, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error removing delegation for user %s on path %s: %v\n", username, file, err)
			retval = OperationError
			continue
		}
		err = d.Save()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error saving delegations: %v\n", err)
			retval = OperationError
			continue
		}
	}
	return
}

func listDelegations(paths []PotentialPathname) (retval int) {
	trace("pathnames passed: %q", paths)
	if len(paths) == 0 {
		var m []PotentialPathname
		for _, x := range mounts() {
			m = append(m, PotentialPathname(x))
		}
		paths = m
	}
	volumes := make(map[VolumePath]bool)
	for _, file := range paths {
		d, err := LookupDelegationsForPath(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading delegations in volume of %s: %v\n", file, err)
			retval = OperationError
			continue
		}
		if _, ok := volumes[d.volume]; ok {
			continue
		}
		volumes[d.volume] = true
		for _, del := range d.delegations {
			if !isAdmin() && del.Delegate != Uid(os.Getuid()) {
				continue
			}
			user := uidToUserOrStringifiedUid(del.Delegate)
			fmt.Printf("%s:	%s\n", user, filepath.Join(string(d.volume), string(del.Object)))
		}
	}
	return
}

func usage() {
	fmt.Fprintf(os.Stderr, USAGE)
}

func main() {
	flag.Parse()

	if *traceFlag {
		if set := setTrace(); !set {
			fmt.Fprintf(os.Stderr, "error: the file /.trace must exist to enable tracing\n")
			os.Exit(PermissionDenied)
		}
	}

	if *listFlag {
		if *recurseFlag || *addFlag || *deleteFlag || *simulateFlag {
			usage()
			os.Exit(Usage)
		}
		os.Exit(listDelegations(stringsToPotentialPathnames(flag.Args())))
	}

	if *addFlag {
		if *recurseFlag || *listFlag || *deleteFlag || *simulateFlag {
			usage()
			os.Exit(Usage)
		}
		if flag.NArg() < 2 {
			usage()
			os.Exit(Usage)
		}
		if !isAdmin() {
			fmt.Fprintf(os.Stderr, "error: adding delegations is a privileged operation\n")
			os.Exit(PermissionDenied)
		}
		os.Exit(addDelegation(PotentialUsername(flag.Args()[0]), stringsToPotentialPathnames(flag.Args()[1:])))
	}

	if *deleteFlag {
		if *recurseFlag || *addFlag || *listFlag || *simulateFlag {
			usage()
			os.Exit(Usage)
		}
		if flag.NArg() < 2 {
			usage()
			os.Exit(Usage)
		}
		if !isAdmin() {
			fmt.Fprintf(os.Stderr, "error: removing delegations is a privileged operation\n")
			os.Exit(PermissionDenied)
		}
		os.Exit(deleteDelegation(PotentialUsername(flag.Args()[0]), stringsToPotentialPathnames(flag.Args()[1:])))
	}

	if flag.NArg() < 1 {
		usage()
		os.Exit(Usage)
	}
	os.Exit(takeOwnership(stringsToPotentialPathnames(flag.Args()), *recurseFlag, *simulateFlag))
}
