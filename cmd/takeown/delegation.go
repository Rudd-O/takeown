package main

import "encoding/json"
import "fmt"
import "io/ioutil"
import "os"
import "path/filepath"
import "syscall"

const TAKEOWN_STORAGE = ".takeown.delegations"

type OwnershipDelegation struct {
	Object   SubpathRelativeToVolume
	Delegate Uid
}

type OwnershipDelegations struct {
	volume      VolumePath
	delegations []OwnershipDelegation
	fsid        syscall.Fsid
}

func delegationsFile(v VolumePath) string {
	return filepath.Join(string(v), TAKEOWN_STORAGE)
}

func (o *OwnershipDelegation) matches(p SubpathRelativeToVolume, u Uid) bool {
	if o.Object == p && o.Delegate == u {
		return true
	}
	return false
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

	p := delegationsFile(filesystem)
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

	if !d.matches(relPath, Uid(uid)) {
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

func (d *OwnershipDelegations) matches(p SubpathRelativeToVolume, uid Uid) bool {
	for _, delegation := range d.delegations {
		if delegation.matches(p, uid) {
			return true
		}
	}
	return false
}

func (d *OwnershipDelegations) Save() error {
	if d.volume == "" {
		return fmt.Errorf("delegations cannot be saved unless associated with a volume")
	}
	p := delegationsFile(d.volume)
	if len(d.delegations) == 0 {
		err := os.Remove(p)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("while eliminating %s: %v", p, err)
		}
		return nil
	}
	data, err := json.Marshal(d.delegations)
	if err != nil {
		return fmt.Errorf("while marshaling delegations: %v", err)
	}
	err = ioutil.WriteFile(p, data, 0600)
	if err != nil {
		return fmt.Errorf("while writing %s: %v", p, err)
	}
	return nil
}
