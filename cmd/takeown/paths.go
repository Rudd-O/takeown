package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

func realpath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", NewError("abspath", dir, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", NewError("readlink", abs, err)
	}
	return real, nil
}

func islink(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == os.ModeSymlink
}

func lstat(path string) (sinfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sinfo{}, err
	}
	statt := info.Sys().(*syscall.Stat_t)
	return sinfo{statt.Uid, statt.Gid, info.IsDir(), islink(info)}, nil
}
