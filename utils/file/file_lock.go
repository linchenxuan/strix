// Package file provides utilities for file operations, including file locking.
package file

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/linchenxuan/strix/log"
)

var (
	// ErrFileNotExist is returned when a file does not exist.
	ErrFileNotExist = errors.New("file not exist")
	// _fileMode is the default file mode for creating files (read/write for owner).
	_fileMode fs.FileMode = 0o600
)

// FileLock represents a lock on a file.
type FileLock struct {
	Path string   // Path is the path to the file to be locked.
	File *os.File // File is the file handle used for the lock.
}

// NewFileLock creates a new FileLock instance for the given path.
func NewFileLock(p string) *FileLock {
	return &FileLock{
		Path: p,
	}
}

// IsLock tries to acquire a lock on the file to check if it's already locked.
// It returns true if the file is already locked by another process, false otherwise.
// Note that this function releases the lock before returning.
func IsLock(p string) bool {
	fl := NewFileLock(p)
	defer func() {
		_ = fl.Unlock()
	}()
	if err := fl.Lock(); err != nil {
		return true
	}

	return false
}

// Lock acquires an exclusive lock on the file.
// It will create the file if it does not exist.
// This is a non-blocking call; it will return an error if the lock cannot be acquired immediately.
func (l *FileLock) Lock() error {
	f, err := os.OpenFile(l.Path, os.O_RDWR|os.O_CREATE, _fileMode)
	if err != nil {
		return err
	}
	l.File = f
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if err2 := l.File.Close(); err2 != nil { //nolint:revive,staticcheck
			log.Error().Err(err2).Msg("Close file fail")
		}
		return fmt.Errorf("directory:%s cannot flock", l.Path)
	}
	log.Info().Msg("file lock succ")
	return nil
}

// Unlock releases the lock on the file.
// It also closes the file handle.
func (l *FileLock) Unlock() error {
	defer l.File.Close()
	return syscall.Flock(int(l.File.Fd()), syscall.LOCK_UN|syscall.LOCK_NB)
}

// RLock acquires a read (shared) lock on the file.
// This is a non-blocking call; it will return an error if the lock cannot be acquired immediately.
// The file must exist for a read lock to be acquired.
func (l *FileLock) RLock() error {
	_, err := os.Stat(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotExist
		}
		return err
	}

	f, err := os.OpenFile(l.Path, os.O_RDONLY, _fileMode)
	if err != nil {
		return err
	}
	l.File = f
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if err != nil {
		if err2 := l.File.Close(); err2 != nil { //nolint:revive,staticcheck
			log.Error().Err(err2).Msg("Close file fail")
		}
		return fmt.Errorf("directory:%s cannot flock", l.Path)
	}
	log.Info().Msg("file rlock succ")
	return nil
}

// RUnlock releases a read (shared) lock on the file.
// It also closes the file handle.
func (l *FileLock) RUnlock() error {
	defer l.File.Close()
	return syscall.Flock(int(l.File.Fd()), syscall.LOCK_UN)
}
