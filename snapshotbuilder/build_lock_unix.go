//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd

/* START ALPHA CHANGE 2073 -- hold stable Unix builder locks until publication completes -- */
package snapshotbuilder

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type platformBuildLock struct {
	file *os.File
}

func acquirePlatformBuildLock(path string) (buildLock, error) {
	file, err := openBuildLockFile(path)
	if err != nil {
		return nil, err
	}

	if err := unix.Flock(
		int(file.Fd()),
		unix.LOCK_EX|unix.LOCK_NB,
	); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("cannot acquire builder lock %s: %w", path, err)
	}

	return &platformBuildLock{file: file}, nil
}

func (l *platformBuildLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	return errors.Join(unlockErr, closeErr)
}

/* FINISH ALPHA CHANGE 2073 -- hold stable Unix builder locks until publication completes -- */
