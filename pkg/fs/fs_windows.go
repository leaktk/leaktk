//go:build windows

package fs

import (
	"os"
	"errors"
)

// FileLockSupport is set to false on windows because at the moment the file lock use case is mainly
// for servers running things like the git.pre-recieve hook. This may change latter.
const FileLockSupported = false

// LockFile is not implemented for this platform
func LockFile(f *os.File) (err error) {
	return errors.New("file locking currently not supported for windows")
}

// Unlockfile is not implemented for this platform
func UnlockFile(f *os.File) error {
	return errors.New("file locking currently not supported for windows")
}
