package main

/*
#define _POSIX_SOURCE
#include <sys/types.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pwd.h>
#include <unistd.h>
#include <mntent.h>
#include <linux/limits.h>

int name_to_uid(char const *name, uid_t *uid)
{
  if (!name) {
    return 0;
  }
  long const buflen = sysconf(_SC_GETPW_R_SIZE_MAX);
  if (buflen == -1) {
    return 0;
  }
  char buf[buflen];
  struct passwd pwbuf, *pwbufp;
  if (0 != getpwnam_r(name, &pwbuf, buf, buflen, &pwbufp)
      || !pwbufp) {
    return 0;
  }
  *uid = pwbufp->pw_uid;
  return 1;
}

// Caller must free returned char* unless it's null.
char * uid_to_name(uid_t uid)
{
  long const buflen = sysconf(_SC_GETPW_R_SIZE_MAX);
  if (buflen == -1) {
    return NULL;
  }
  char buf[buflen];
  struct passwd pwbuf, *pwbufp;
  if (0 != getpwuid_r(uid, &pwbuf, buf, buflen, &pwbufp)
      || !pwbufp) {
    return NULL;
  }
  char * pwname = malloc(strlen(pwbufp->pw_name) + 1);
  if (pwname == NULL) {
    return NULL;
  }
  memcpy(pwname, pwbufp->pw_name, strlen(pwbufp->pw_name) + 1);
  return pwname;
}
*/
import "C"

import "encoding/json"
import "errors"
import "flag"
import "fmt"
import "io/ioutil"
import "os"
import "path/filepath"
import "strconv"
import "strings"
import "syscall"
import "unsafe"

const (
	Success          = 0
	BadConfig        = 8
	OperationError   = 32
	Usage            = 64
	PermissionDenied = 128
)

const TAKEOWN_STORAGE = ".takeown.delegations"

var addFlag = flag.Bool("a", false, "add a delegation for a specific user and path")
var listFlag = flag.Bool("l", false, "list user delegations established on paths")
var deleteFlag = flag.Bool("d", false, "remove a delegation for a specific user and path")
var recurseFlag = flag.Bool("r", false, "take ownership recursively")
var simulateFlag = flag.Bool("s", false, "simulate taking ownership")

var uidDoesNotExist = errors.New("UID has no corresponding user name")
var usernameDoesNotExist = errors.New("user name has no corresponding UID")

type SubpathRelativeToVolume string
type Uid uint32
type PotentialPathname string
type AbsolutePathname string
type AbsoluteMaybeResolvedPathname AbsolutePathname
type AbsoluteResolvedPathname AbsoluteMaybeResolvedPathname
type VolumePath AbsoluteResolvedPathname
type PotentialUsername string
type Username string
type UsernameOrStringifiedUid string
type Mountpoint AbsolutePathname

type OwnershipDelegation struct {
	Object   SubpathRelativeToVolume
	Delegate Uid
}

func (o *OwnershipDelegation) matches(p SubpathRelativeToVolume, u Uid) bool {
	if o.Object == p && o.Delegate == u {
		return true
	}
	return false
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

type OwnershipDelegations struct {
	volume      VolumePath
	delegations []OwnershipDelegation
	fsid        syscall.Fsid
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

// mounts returns a list of mount points
func mounts() []Mountpoint {
	var mounts []Mountpoint
	mtab := C.CString("/etc/mtab")
	r := C.CString("r")
	stream := C.setmntent(mtab, r)
	for {
		mntent := C.getmntent(stream) //, &mntbuf, &buf, buflen)
		if mntent == nil {
			break
		}
		mntpnt := C.GoString(mntent.mnt_dir)
		mounts = append(mounts, Mountpoint(mntpnt))
	}
	C.endmntent(stream)
	return mounts
}

// userToUid takes an UNIX UID and looks its name up.  If lookup fails, it
// returns an error explaining the failure.
func uidToUser(uid Uid) (Username, error) {
	var user string
	username := C.uid_to_name(C.uid_t(uid))
	if username == nil {
		return Username(""), uidDoesNotExist
	}
	user = C.GoString(username)
	C.free(unsafe.Pointer(username))
	return Username(user), nil
}

func uidToUserOrStringifiedUid(uid Uid) UsernameOrStringifiedUid {
	username, err := uidToUser(uid)
	if err != nil {
		return UsernameOrStringifiedUid(fmt.Sprintf("%d", uid))
	}
	return UsernameOrStringifiedUid(username)
}

// uidToUser takes an UNIX user name and looks its UID up.  If lookup fails,
// it returns an error explaining the failure.
func userToUid(username PotentialUsername) (Uid, error) {
	var uid C.uid_t
	ucs := C.CString(string(username))
	defer C.free(unsafe.Pointer(ucs))
	worked := C.name_to_uid(ucs, &uid)
	if worked != 1 {
		return 0, usernameDoesNotExist
	}
	return Uid(uid), nil
}

func userToUidOrStringUid(username PotentialUsername) (Uid, error) {
	uid, err := userToUid(username)
	if err != nil {
		uuid, err := strconv.Atoi(string(username))
		if err != nil {
			return 0, usernameDoesNotExist
		}
		if uuid < 0 {
			return 0, usernameDoesNotExist
		}
		return Uid(uuid), nil
	}
	return Uid(uid), nil
}

func DelegationsFile(v VolumePath) string {
	return filepath.Join(string(v), TAKEOWN_STORAGE)
}

func relativeToVolume(vol VolumePath, p AbsolutePathname) (SubpathRelativeToVolume, error) {
	x, err := filepath.Rel(string(vol), string(p))
	return SubpathRelativeToVolume(x), err
}

// findContainingVolume will return the volume path that contains the file
// after resolving all the symlinks intermediating access to the file.
func findContainingVolume(file AbsolutePathname) (VolumePath, error) {
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

func LookupDelegationsForPath(file PotentialPathname) (*OwnershipDelegations, error) {
	p, err := absolutizePath(file)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %v\n", err)
	}
	container, err := findContainingVolume(p)
	if err != nil {
		return nil, fmt.Errorf("error looking up volume: %v\n", err)
	}
	return loadDelegations(container)
}

func loadDelegations(filesystem VolumePath) (*OwnershipDelegations, error) {
	var d OwnershipDelegations

	var statfsdata syscall.Statfs_t
	err := syscall.Statfs(string(filesystem), &statfsdata)
	if err != nil {
		return nil, fmt.Errorf("cannot statvfs %s: %v", filesystem, err)
	}
	d.fsid = statfsdata.Fsid

	p := DelegationsFile(filesystem)
	f, err := os.Open(string(p))
	if err != nil {
		if os.IsNotExist(err) {
			d.volume = VolumePath(filesystem)
			return &d, nil
		}
		return nil, fmt.Errorf("while opening %s: %v", p, err)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("while reading %s: %v", p, err)
	}
	err = json.Unmarshal(data, &d.delegations)
	if err != nil {
		return nil, fmt.Errorf("while parsing %s: %v", p, err)
	}
	d.volume = VolumePath(filesystem)
	return &d, nil
}

func (d *OwnershipDelegations) Add(username PotentialUsername, file PotentialPathname) error {
	uid, err := userToUidOrStringUid(username)
	if err != nil {
		return fmt.Errorf("cannot look up user %s: %v", username, err)
	}

	p, err := absolutizePath(file)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %v", file, err)
	}

	r, err := resolveSymlinks(p)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %v", file, err)
	}

	strkt, err := statWithFileInfo(PotentialPathname(r))
	if err != nil {
		return fmt.Errorf("cannot stat %s: %v", file, err)
	}
	if strkt.fsid != d.fsid {
		return fmt.Errorf("file %s is not contained in volume %s", file, d.volume)
	}

	relPath, err := relativeToVolume(d.volume, AbsolutePathname(p))
	if err != nil {
		return fmt.Errorf("cannot determine relative path for %s: %v", file, err)
	}

	if !d.present(relPath, Uid(uid)) {
		d.delegations = append(d.delegations, OwnershipDelegation{SubpathRelativeToVolume(relPath), Uid(uid)})
	}
	return nil
}

func (d *OwnershipDelegations) Remove(username PotentialUsername, file PotentialPathname) error {
	uid, err := userToUidOrStringUid(username)
	if err != nil {
		return fmt.Errorf("cannot lookup user %s: %v", username, err)
	}

	p, err := absolutizePath(file)
	if err != nil {
		return fmt.Errorf("cannot inspect %s: %v", file, err)
	}

	r, err := resolveSymlinks(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot resolve symlinks %s: %v", file, err)
		}
		r = AbsoluteResolvedPathname(p)
	}

	relPath, err := relativeToVolume(d.volume, AbsolutePathname(r))
	if err != nil {
		return fmt.Errorf("cannot determine relative path for %s: %v", file, err)
	}

	found := false
	var newdels []OwnershipDelegation
	for _, del := range d.delegations {
		if del.matches(SubpathRelativeToVolume(relPath), uid) {
			found = true
			continue
		}
		newdels = append(newdels, del)
	}
	d.delegations = newdels
	if !found {
		return fmt.Errorf("cannot find delegation on %s for user %s", p, username)
	}
	return nil
}

func (d *OwnershipDelegations) CanTakeOwnership(uid Uid, file StatedPathname) (Uid, Uid, error) {
	p, err := absolutizePath(file.path)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot inspect %s: %v", file, err)
	}

	r, err := resolveSymlinks(p)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot inspect %s: %v", file, err)
	}

	relPath, err := relativeToVolume(d.volume, AbsolutePathname(r))
	if err != nil {
		return 0, 0, fmt.Errorf("cannot determine relative path for %s: %v", file.path, err)
	}

	if file.fsid != d.fsid {
		return 0, 0, fmt.Errorf("file %s is not contained in volume %s", file.path, d.volume)
	}

	if isAdmin() {
		return Uid(os.Getuid()), Uid(file.gid), nil
	}

	for _, del := range d.delegations {
		if del.Delegate != uid {
			continue
		}
		if del.Object == SubpathRelativeToVolume(relPath) {
			return uid, Uid(file.gid), nil
		}
		container := filepath.Join(string(d.volume), string(del.Object))
		contained, err := contains(AbsoluteResolvedPathname(container), r)
		if err != nil {
			return 0, 0, err
		}
		if contained {
			return uid, Uid(file.gid), nil
		}
	}
	return 0, 0, fmt.Errorf("cannot take ownership of %s: permission denied", file.path)
}

func (d *OwnershipDelegations) present(p SubpathRelativeToVolume, uid Uid) bool {
	for _, delegation := range d.delegations {
		if delegation.Object == p && delegation.Delegate == uid {
			return true
		}
	}
	return false
}

func (d *OwnershipDelegations) Save() error {
	if d.volume == "" {
		return fmt.Errorf("delegations cannot be saved unless associated with a volume")
	}
	data, err := json.Marshal(d.delegations)
	if err != nil {
		return fmt.Errorf("while marshaling %v: %v", d.delegations, err)
	}
	p := DelegationsFile(d.volume)
	err = ioutil.WriteFile(p, data, 0600)
	if err != nil {
		return fmt.Errorf("while writing %s: %v", p, err)
	}
	return nil
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

