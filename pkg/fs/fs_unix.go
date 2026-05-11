//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package fs

import (
	"io/fs"
	"os"
	"strconv"
	"syscall"
)

const FileLockSupported = true

func flock(f *os.File, lt int16) (err error) {
	for {
		err = syscall.Flock(int(f.Fd()), int(lt)) // #nosec G115
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return &fs.PathError{
			Op:   strconv.Itoa(int(lt)),
			Path: f.Name(),
			Err:  err,
		}
	}
	return nil
}

// LockFile locks a file using the flock syscall. Unlock doesn't need to be called if the file is closed.
func LockFile(f *os.File) (err error) {
	return flock(f, syscall.LOCK_EX)
}

// UnlockFile unlocks a file locked by LockFile. This doesn't need to be called if the file is closed
func UnlockFile(f *os.File) error {
	return flock(f, syscall.LOCK_UN)
}
