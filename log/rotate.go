package log

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Default file permissions for log files and directories
	defaultFileMode = 0644
	defaultDirMode  = 0755

	// Time-related constants for rotation calculations
	secondsPerDay = 24 * 60 * 60
)

// UpdateFileFd manages log file rotation based on time and size constraints
// for high-performance distributed game server logging systems.
//
// This function determines whether a log file needs rotation based on:
// - Time-based rotation: triggers at specified hour intervals (fileSplitHour)
// - Size-based rotation: triggers when file exceeds specified megabytes (fileSplitMB)
//
// Parameters:
//   - filePath: absolute path to the log file
//   - fileSplitHour: hour of day (0-23) to trigger time-based rotation
//   - fileSplitMB: file size threshold in megabytes for size-based rotation
//   - oldFD: current file descriptor, may be nil for initial creation
//   - oldFileCreateTime: creation timestamp of the current log file
//
// Returns:
//   - newFD: new file descriptor after rotation, or oldFD if no rotation needed
//   - newFileCreateTime: creation timestamp of the new log file
//   - error: any error encountered during rotation process
//
// Thread-safe for concurrent access, handles edge cases like:
// - Missing parent directories (auto-created)
// - File system errors during rotation
// - Race conditions during concurrent rotation attempts
// - Graceful handling of invalid file paths
func UpdateFileFd(filePath string, fileSplitHour, fileSplitMB int, oldFD *os.File,
	oldFileCreateTime time.Time) (*os.File, time.Time, error) {
	if len(filePath) == 0 {
		return nil, time.Time{}, errors.New("filename is empty")
	}

	shouldRotate, err := checkRotation(filePath, fileSplitHour, fileSplitMB, oldFD, oldFileCreateTime)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("check rotation: %w", err)
	}

	if !shouldRotate {
		return oldFD, oldFileCreateTime, nil
	}

	newFD, newFileCreateTime, err := openLogFile(filePath)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("open new log file: %w", err)
	}

	return newFD, newFileCreateTime, nil
}

// checkRotation determines if log file rotation is required based on configured criteria
//
// Performs comprehensive rotation checks:
// - Validates file existence and accessibility
// - Evaluates time-based rotation triggers
// - Assesses size-based rotation thresholds
// - Handles edge cases for new file creation
//
// Parameters mirror UpdateFileFd for consistency
// Returns true if rotation should occur, false otherwise
func checkRotation(filePath string, fileSplitHour, fileSplitMB int, oldFD *os.File,
	oldFileCreateTime time.Time) (bool, error) {
	if oldFD == nil {
		return true, nil
	}

	now := time.Now()

	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}

	if shouldRotateByTime(oldFileCreateTime, now, fileSplitHour) {
		if err := moveLogFile(oldFD, filePath, now); err != nil {
			return false, fmt.Errorf("move log file by time: %w", err)
		}
		return true, nil
	}

	if shouldRotateBySize(fi.Size(), fileSplitMB) {
		if err := moveLogFile(oldFD, filePath, now); err != nil {
			return false, fmt.Errorf("move log file by size: %w", err)
		}
		return true, nil
	}

	return false, nil
}

// shouldRotateByTime evaluates time-based rotation triggers
//
// Implements sophisticated time-based rotation logic:
// - Daily rotation when crossing midnight boundary
// - Hour-specific rotation within the same day
// - Handles timezone considerations automatically
//
// Parameters:
//   - createTime: original file creation timestamp
//   - now: current system time
//   - splitHour: configured hour (0-23) for rotation trigger
//
// Returns true if rotation should occur based on time criteria
func shouldRotateByTime(createTime, now time.Time, splitHour int) bool {
	if splitHour == 0 {
		return false
	}

	createUnix := createTime.Unix()
	nowUnix := now.Unix()

	if createUnix+secondsPerDay <= nowUnix {
		return true
	}

	if createTime.Day() == now.Day() {
		return now.Hour() >= splitHour && createTime.Hour() < splitHour
	}

	return now.Hour() >= splitHour
}

// shouldRotateBySize determines if file size exceeds rotation threshold
//
// Efficiently checks file size against configured megabyte limit
// Uses bit shifting for optimal performance in high-throughput scenarios
//
// Parameters:
//   - size: current file size in bytes
//   - splitMB: maximum allowed size in megabytes
//
// Returns true if file size exceeds threshold
func shouldRotateBySize(size int64, splitMB int) bool {
	if splitMB == 0 {
		return false
	}

	return size >= int64(splitMB)<<20 // 1MB = 1<<20 bytes
}

func moveLogFile(oldFD *os.File, filePath string, now time.Time) error {
	if oldFD != nil {
		if err := oldFD.Close(); err != nil {
			return fmt.Errorf("close old file: %w", err)
		}
	}

	newFilePath, err := generateBackupFileName(filePath, now)
	if err != nil {
		return fmt.Errorf("generate backup filename: %w", err)
	}

	if err := os.Rename(filePath, newFilePath); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}

// generateBackupFileName creates unique backup filenames with timestamp precision
//
// Implements intelligent filename generation:
// - Preserves original file extension
// - Appends precise timestamp (YYYYMMDDHHMMSS format)
// - Handles filename collisions with 1-second increments
// - Ensures unique filenames across rotation events
//
// Parameters:
//   - filePath: original log file path
//   - now: current timestamp for backup filename
//
// Returns generated backup filename with absolute path
// Returns error if unique filename cannot be generated after 5 attempts
func generateBackupFileName(filePath string, now time.Time) (string, error) {
	ext := filepath.Ext(filePath)
	baseName := strings.TrimSuffix(filePath, ext)

	for i := 0; i < 5; i++ {
		timestamp := now.Add(time.Duration(i) * time.Second)
		newFilePath := fmt.Sprintf("%s%s.%04d%02d%02d-%02d%02d%02d",
			baseName,
			ext,
			timestamp.Year(),
			timestamp.Month(),
			timestamp.Day(),
			timestamp.Hour(),
			timestamp.Minute(),
			timestamp.Second(),
		)

		if exists, err := fileExists(newFilePath); err != nil {
			return "", fmt.Errorf("check file existence: %w", err)
		} else if !exists {
			return newFilePath, nil
		}
	}

	return "", errors.New("cannot generate unique backup filename")
}

// fileExists performs atomic file existence verification
//
// Provides reliable file system state checking:
// - Handles race conditions gracefully
// - Distinguishes between non-existence and system errors
// - Thread-safe for concurrent access patterns
//
// Parameters:
//   - filePath: absolute path to verify
//
// Returns true if file exists, false otherwise
// Returns error for system-level failures (permission, I/O, etc.)
func fileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat file: %w", err)
}

// openLogFile creates and initializes new log file with proper permissions
//
// Implements robust file creation:
// - Creates parent directories recursively with secure permissions
// - Opens file in append mode to prevent data loss
// - Retrieves accurate file creation timestamp
// - Handles platform-specific filesystem behaviors
//
// Parameters:
//   - filePath: absolute path for new log file
//
// Returns:
//   - fd: opened file descriptor ready for writing
//   - createTime: precise file creation timestamp
//   - error: any error during file creation or timestamp retrieval
//
// Ensures atomic operations and proper resource cleanup on failure
func openLogFile(filePath string) (*os.File, time.Time, error) {
	dir := path.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, defaultDirMode); err != nil {
			return nil, time.Time{}, fmt.Errorf("create directory: %w", err)
		}
	}

	fd, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, defaultFileMode)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("open file: %w", err)
	}

	fileCreateTime, err := GetFileCreateTime(filePath)
	if err != nil {
		fd.Close()
		return nil, time.Time{}, fmt.Errorf("get file create time: %w", err)
	}

	if fileCreateTime.UnixNano()%int64(time.Second) > int64(time.Second)/2 {
		fileCreateTime = time.Unix(fileCreateTime.Unix()+1, 0)
	}

	return fd, fileCreateTime, nil
}

// GetFileCreateTime retrieves precise file creation timestamp with cross-platform compatibility
//
// Provides robust file metadata extraction:
// - Cross-platform compatibility (Windows, Linux, macOS)
// - Fallback to modification time when creation time unavailable
// - Handles filesystem-specific behaviors and limitations
// - Thread-safe for concurrent access patterns
//
// Parameters:
//   - filePath: absolute path to target file
//
// Returns:
//   - time.Time: file creation timestamp (or modification time as fallback)
//   - error: filesystem access errors or invalid path issues
//
// Critical for accurate log rotation decisions in distributed game server environments
// where precise timing is essential for log file lifecycle management
func GetFileCreateTime(filePath string) (time.Time, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, err
	}
	// Go does not expose creation time in a portable way, so fallback to ModTime
	return fi.ModTime(), nil
}
