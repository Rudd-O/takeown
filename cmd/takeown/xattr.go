package main

import (
	"encoding/json"
	"syscall"

	"github.com/pkg/xattr"
)

func getxattr(path string, attrname string) (*[]byte, error) {
	data, err := xattr.Get(path, ATTRNAME)
	if err != nil {
		if xerr, ok := err.(*xattr.Error); ok {
			if serr, ok := xerr.Err.(syscall.Errno); ok {
				if serr == syscall.ENODATA {
					// Attribute not present.  We ignore and continue.
					return nil, nil
				}
			}
		}
		return nil, err
	}
	return &data, nil
}

func UnmarshalFromXattr(path string, attrname string, s interface{}) error {
	attr, err := getxattr(path, attrname)
	if err != nil {
		return err
	}
	if attr != nil {
		if err := json.Unmarshal(*attr, s); err != nil {
			return NewError("unmarshal", path, err)
		}
	}
	return nil
}

func setxattr(path string, attrname string, data []byte) error {
	return xattr.Set(path, attrname, data)
}

func MarshalToXattr(path string, attrname string, s interface{}) error {
	data, err := json.Marshal(s)
	if err != nil {
		return NewError("marshal", path, err)
	}
	return setxattr(path, attrname, data)
}
