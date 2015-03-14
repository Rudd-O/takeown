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
import "errors"
import "fmt"
import "strconv"
import "unsafe"

type PotentialUsername string
type Uid uint32
type Username string
type UsernameOrStringifiedUid string
type Mountpoint AbsolutePathname

var uidDoesNotExist = errors.New("UID has no corresponding user name")
var usernameDoesNotExist = errors.New("user name has no corresponding UID")

// uidToUser takes an UNIX UID and looks its name up.  If lookup fails, it
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

// userToUid takes an UNIX user name and looks its UID up.  If lookup fails,
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
