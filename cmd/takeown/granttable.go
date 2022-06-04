package main

import (
	"path/filepath"
	"syscall"
)

const ATTRNAME = "security.takeown.grants"

type GrantTable interface {
	ForDir(string) (UIDList, error)
	ForPath(string) (UIDList, error)
	Add(string, UID) error
}

type dirgrant struct {
	directory string
	grants    UIDList
	parent    *dirgrant
}

type UNIXGrantTable struct {
	directories map[string]*dirgrant
}

func NewUNIXGrantTable() *UNIXGrantTable {
	t := UNIXGrantTable{}
	t.directories = make(map[string]*dirgrant)
	return &t
}

func (t *UNIXGrantTable) getDirgrant(real string) (*dirgrant, error) {
	if dg, ok := t.directories[real]; ok {
		return dg, nil
	}
	d := &dirgrant{}
	d.directory = real
	d.grants = UIDList{}
	err := UnmarshalFromXattr(real, ATTRNAME, &d.grants)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(real)
	if parent != real {
		d.parent, err = t.getDirgrant(parent)
		if err != nil {
			return nil, err
		}
	}
	t.directories[real] = d
	return d, nil
}

func (t *UNIXGrantTable) _for(path string, mustBeDir bool) (UIDList, error) {
	fs, err := lstat(path)
	if err != nil {
		return nil, NewError("stat", path, err)
	}
	if !fs.Dir {
		if mustBeDir {
			return nil, NewError("stat", path, syscall.Errno(syscall.ENOTDIR))
		}
		path = filepath.Dir(path)
	}
	real, err := realpath(path)
	if err != nil {
		return nil, err
	}
	dirgrant, err := t.getDirgrant(real)
	if err != nil {
		return nil, err
	}
	result := UIDList{}
	for dirgrant != nil {
		result = result.Merge(dirgrant.grants)
		dirgrant = dirgrant.parent
	}
	return result, nil
}

func (t *UNIXGrantTable) Table(path string) (map[UID][]string, error) {
	mustBeDir := false
	fs, err := lstat(path)
	if err != nil {
		return nil, NewError("stat", path, err)
	}
	if !fs.Dir {
		if mustBeDir {
			return nil, NewError("stat", path, syscall.Errno(syscall.ENOTDIR))
		}
		path = filepath.Dir(path)
	}
	real, err := realpath(path)
	if err != nil {
		return nil, err
	}
	result := make(map[UID][]string)
	dirgrant, err := t.getDirgrant(real)
	if err != nil {
		return nil, err
	}
	for dirgrant != nil {
		for _, uid := range dirgrant.grants {
			existing, ok := result[uid]
			if !ok {
				existing = []string{}
			}
			result[uid] = append(existing, dirgrant.directory)
		}
		dirgrant = dirgrant.parent
	}
	return result, nil
}

func (t *UNIXGrantTable) ForDir(path string) (UIDList, error) {
	return t._for(path, true)
}

func (t *UNIXGrantTable) ForPath(path string) (UIDList, error) {
	return t._for(path, false)
}

func (t *UNIXGrantTable) Add(path string, uid UID) error {
	fs, err := lstat(path)
	if err != nil {
		return NewError("stat", path, err)
	}
	if !fs.Dir {
		return NewError("stat", path, syscall.Errno(syscall.ENOTDIR))
	}
	real, err := realpath(path)
	if err != nil {
		return err
	}
	u := UIDList{}
	if err := UnmarshalFromXattr(real, ATTRNAME, &u); err != nil {
		return err
	}
	u2 := u.Merge(UIDList{uid})
	if u.Equal(u2) {
		return nil
	}
	if err := MarshalToXattr(real, ATTRNAME, &u2); err != nil {
		return err
	}
	delete(t.directories, real)
	return nil
}

func (t *UNIXGrantTable) Remove(path string, uid UID) error {
	fs, err := lstat(path)
	if err != nil {
		return NewError("stat", path, err)
	}
	if !fs.Dir {
		return NewError("stat", path, syscall.Errno(syscall.ENOTDIR))
	}
	real, err := realpath(path)
	if err != nil {
		return err
	}
	u := UIDList{}
	if err := UnmarshalFromXattr(real, ATTRNAME, &u); err != nil {
		return err
	}
	u2 := u.Remove(UIDList{uid})
	if u.Equal(u2) {
		return nil
	}
	if err := MarshalToXattr(real, ATTRNAME, &u2); err != nil {
		return err
	}
	delete(t.directories, real)
	return nil
}
