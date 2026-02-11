// Package shm provides utilities for shared memory pipe selection.
package shm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/utils/file"
)

const (
	// _fileLockExpireDur represents the expiration duration for the file lock in seconds.
	_fileLockExpireDur = 60
	// _renewShmpipeTimerInterval represents the interval in seconds for renewing the shared memory pipe timer.
	_renewShmpipeTimerInterval = 30
	// _base is the number base for string conversions.
	_base = 10
	// _bitSize is the bit size for integer types.
	_bitSize = 64
	// _fileContentCnt is the expected number of elements in the lock file content.
	_fileContentCnt = 2
)

// pipeStatus represents the status of a shared memory pipe.
type pipeStatus struct {
	version      uint64 // version of the pipe, used to identify the process holding the lock.
	expireTime   int64  // expireTime of the pipe lock in Unix timestamp format.
	isAccessable bool   // isAccessable indicates whether the pipe is currently accessible.
}

// _fileFolderMode is the file mode for creating directories.
var _fileFolderMode fs.FileMode = 0o700

// ShmpipeSelector is a selector for shared memory pipes. It manages the lock of a specific pipe.
type ShmpipeSelector struct {
	Fl *file.FileLock // Fl is the file lock for the shared memory pipe.
	v  uint64         // v is the version (identifier) of the process using this pipe.
}

// LockShmpipe locks a specific shared memory pipe.
// It creates the shared directory if it doesn't exist, and then locks the pipe file.
func (s *ShmpipeSelector) LockShmpipe(shmDir, lockFileName string, pipeIdx int, v uint64) error {
	// Create the shared directory if it doesn't exist.
	if err := mkdirRecursive(shmDir); err != nil {
		return err
	}

	fullPath := fmt.Sprintf("%s/%s%d", shmDir, lockFileName, pipeIdx)

	return s.lockShmpipeImpl(fullPath, v)
}

// lockShmpipeImpl is the implementation for locking a shared memory pipe.
// It locks the file, truncates it, and writes the version and expiration time.
func (s *ShmpipeSelector) lockShmpipeImpl(fullPath string, v uint64) error {
	s.v = v
	s.Fl = file.NewFileLock(fullPath)
	if err := s.Fl.Lock(); err != nil {
		return fmt.Errorf("path:%s lock shmpipe fail", fullPath)
	}

	// Clear the file content and write the new expiration time.
	newExpireTime := getExpireTime()
	content := fmt.Sprintf("%d %d", v, newExpireTime)
	if err := s.Fl.File.Truncate(0); err != nil {
		if err2 := s.Fl.Unlock(); err2 != nil {
			log.Error().Err(err2).Msg("File unlock fail")
		}
		return err
	}

	// Write the new content to the file.
	if _, err := s.Fl.File.Seek(0, 0); err != nil {
		log.Error().Err(err).Msg("File seek fail")
	}
	if _, err := io.WriteString(s.Fl.File, content); err != nil {
		if err2 := s.Fl.Unlock(); err2 != nil {
			log.Error().Err(err2).Msg("File unlock fail")
		}

		return fmt.Errorf("path:%s write file fail", fullPath)
	}

	log.Info().Str("path", fullPath).Msg("LockShmpipe path")
	return nil
}

// RenewShmpipe renews the lock on the shared memory pipe.
// It reads the current lock file, checks if the lock is still valid and owned by the current process,
// and updates the expiration time if it is.
func (s *ShmpipeSelector) RenewShmpipe() error {
	if s.Fl == nil {
		log.Info().Msg("Shmpipe lock not init, wait next call")
		return nil
	}

	bOk := false
	defer func() {
		if !bOk {
			if err := s.Fl.Unlock(); err != nil {
				log.Error().Err(err).Msg("File unlock fail")
			}
		}
	}()

	if _, err := s.Fl.File.Seek(0, 0); err != nil {
		log.Error().Err(err).Msg("File seek fail err")
	}
	content, err1 := io.ReadAll(s.Fl.File)
	if err1 != nil && !errors.Is(err1, io.EOF) {
		return fmt.Errorf("path:%s read file fail", s.Fl.Path)
	}

	res := strings.Split(string(content), " ")
	if len(res) != _fileContentCnt {
		return fmt.Errorf("path:%s content:%s file content not valid", s.Fl.Path, string(content))
	}

	version, err := strconv.ParseUint(res[0], _base, _bitSize)
	if err != nil {
		return err
	}
	expireTime, err := strconv.ParseInt(res[1], _base, _bitSize)
	if err != nil {
		return err
	}

	// If the lock has expired, another process may have taken it.
	if expireTime <= time.Now().Unix() {
		return fmt.Errorf("path:%s expire:%d lock", s.Fl.Path, expireTime)
	}

	// Clear the file content and write the new expiration time.
	newExpireTime := getExpireTime()
	newContent := fmt.Sprintf("%d %d", version, newExpireTime)
	if err := s.Fl.File.Truncate(0); err != nil {
		return err
	}
	if _, err := s.Fl.File.Seek(0, 0); err != nil {
		log.Error().Err(err).Msg("File seek fail")
	}
	if _, err := io.WriteString(s.Fl.File, newContent); err != nil {
		return err
	}

	bOk = true
	return nil
}

// RenewShmpipePeriodically renews the shared memory pipe lock periodically in a goroutine.
// It uses a ticker to renew the lock at a fixed interval.
// If renewal fails, it attempts to re-acquire the lock. If that also fails, it sends a signal to stop.
func (s *ShmpipeSelector) RenewShmpipePeriodically(ctx context.Context, stopRecv chan bool) {
	t := time.NewTicker(_renewShmpipeTimerInterval * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := s.Fl.Unlock(); err != nil {
				log.Error().Err(err).Msg("File unlock fail")
			}
			return
		case <-t.C:
			if err := s.RenewShmpipe(); err != nil {
				log.Error().Str("path", s.Fl.Path).Err(err).Msg("Renew shmpipe fail")

				// If renewal fails, try to re-acquire the lock. If that fails, it's a critical error.
				dir := s.Fl.Path
				if err = s.lockShmpipeImpl(dir, s.v); err != nil {
					log.Error().Str("dir", dir).Err(err).Msg("Retry lock shmpipe fail")
					// This indicates a serious problem, like the file lock being stuck.
					// An alarm should be added here.
					stopRecv <- true
					return
				}
				log.Info().Str("path", s.Fl.Path).Msg("Relock shmpipe successes")
			}
		}
	}
}

// UnlockShmPipe unlocks the bound pipe.
func (s *ShmpipeSelector) UnlockShmPipe() error {
	if s.Fl == nil {
		return nil
	}

	return s.Fl.Unlock()
}

// GetAvailableShmpipeIdx gets an available shared memory pipe index.
// It checks the status of all pipe files and chooses an available one based on the current version.
func GetAvailableShmpipeIdx(shmDir, lockFileName string, pipeCnt int, v uint64) int {
	// Create the shared directory if it doesn't exist.
	if err := mkdirRecursive(shmDir); err != nil {
		log.Error().Str("shmDir", shmDir).Err(err).Msg("mkdir")
		return -1
	}

	p := getCurFileStatus(shmDir, lockFileName, pipeCnt)

	log.Info().Str("pipestatus", fmt.Sprintf("%+v", p)).End()
	// Choose an available pipe from the current status.
	return choseAvailableShmPipe(p, v)
}

// getExpireTime calculates the expiration time for a lock.
func getExpireTime() int64 {
	return time.Now().Unix() + _fileLockExpireDur
}

// choseAvailableShmPipe finds a suitable shared memory channel.
// It prioritizes a pipe with the same version 'v'. This is to handle cases where a server restarts
// and tries to re-acquire the same pipe it was using before.
func choseAvailableShmPipe(p []pipeStatus, v uint64) int {
	if len(p) == 1 {
		return 0
	}

	// Try to choose the previously selected pipe.
	choice := -1
	oldShmpipeIdx := -1
	oldShmpipeExpireTime := 0
	defer func() {
		log.Info().Any("pipestatus", p).Uint64("version", v).Int("choice", choice).End()
	}()

	for idx := range p {
		if !p[idx].isAccessable {
			continue
		}

		if v == p[idx].version {
			if oldShmpipeExpireTime < int(p[idx].expireTime) {
				oldShmpipeExpireTime = int(p[idx].expireTime)
				oldShmpipeIdx = idx
			}
		}
		choice = idx
	}

	// Prioritize the previously selected pipe.
	if oldShmpipeIdx != -1 {
		return oldShmpipeIdx
	}

	return choice
}

// mkdirRecursive creates a directory recursively if it doesn't exist.
func mkdirRecursive(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		if err := os.MkdirAll(dir, _fileFolderMode); err != nil {
			return err
		}
	}
	return nil
}

// getCurFileStatus gets the status of all pipe lock files.
func getCurFileStatus(shmDir, lockFileName string, pipeCnt int) []pipeStatus {
	p := make([]pipeStatus, pipeCnt)

	// Query the current lock status for each pipe.
	for idx := 0; idx < pipeCnt; idx++ {
		fullPath := fmt.Sprintf("%s/%s%d", shmDir, lockFileName, idx)
		p[idx] = fillSingleFileStatus(fullPath)
	}
	return p
}

// fillSingleFileStatus gets the status of a single pipe lock file.
// It attempts to lock the file, reads its content, and parses the version and expiration time.
func fillSingleFileStatus(fullPath string) pipeStatus {
	s := pipeStatus{}
	fileLock := file.NewFileLock(fullPath)
	if err := fileLock.Lock(); err != nil {
		log.Info().Str("fullPath", fullPath).Err(err).Msg("GetShmpipeIdx")
		if errors.Is(err, file.ErrFileNotExist) {
			s.isAccessable = true
		}
		return s
	}
	defer func() {
		if err := fileLock.Unlock(); err != nil {
			log.Error().Err(err).Msg("File unlock fail")
		}
	}()
	if _, err := fileLock.File.Seek(0, 0); err != nil {
		log.Error().Err(err).Msg("File seek fail")
	}
	content, err := io.ReadAll(fileLock.File)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Info().Err(err).Msg("Read err")
		return s
	}

	s.isAccessable = true
	if len(content) != 0 {
		res := strings.Split(string(content), " ")
		if len(res) != _fileContentCnt {
			s.isAccessable = false
		}

		s.version, err = strconv.ParseUint(res[0], _base, _bitSize)
		if err != nil {
			log.Error().Str("version", res[0]).Err(err).Msg("Parse version fail")
			return s
		}
		s.expireTime, err = strconv.ParseInt(res[1], _base, _bitSize)
		if err != nil {
			log.Error().Str("version", res[1]).Err(err).Msg("Parse expire time fail")
			return s
		}
		return s
	}

	return s
}
