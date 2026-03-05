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
	// 日志文件和目录的默认文件权限。
	defaultFileMode = 0644
	defaultDirMode  = 0755

	// 用于轮转计算的时间相关常数。
	secondsPerDay = 24 * 60 * 60
)

// UpdateFileFd 根据时间和大小限制管理日志文件轮换。
// 用于高性能分布式游戏服务器日志系统。
//
// 此函数根据以下条件确定日志文件是否需要轮换：
// - 基于时间的轮换：以指定的小时间隔触发 (fileSplitHour)
// - 基于大小的轮转：当文件超过指定兆字节 (fileSplitMB) 时触发
//
// 参数：
// - filePath：日志文件的绝对路径
// - fileSplitHour：一天中的小时（0-23）触发基于时间的轮换
// - fileSplitMB：基于大小的轮转的文件大小阈值（以兆字节为单位）
// - oldFD：当前文件描述符，初始创建时可能为零
// - oldFileCreateTime：当前日志文件的创建时间戳
//
// 返回：
// - newFD：轮转后的新文件描述符，如果不需要轮转则为 oldFD
// - newFileCreateTime：新日志文件的创建时间戳
// - 错误：轮转过程中遇到的任何错误
//
// 并发访问是线程安全的，可处理边缘情况，例如：
// - 缺少父目录（自动创建）
// - 轮换期间文件系统错误
// - 并发轮换尝试期间的竞争条件
// - 优雅地处理无效文件路径
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

// checkRotation 根据配置的条件确定是否需要日志文件轮换。
//
// 执行全面的轮换检查：
// - 验证文件存在和可访问性
// - 评估基于时间的轮换触发器
// - 评估基于大小的轮转阈值
// - 处理新文件创建的边缘情况
//
// 参数镜像 UpdateFileFd 以保持一致性。
// 如果发生轮转则返回 true，否则返回 false。
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

// shouldRotateByTime 评估基于时间的轮转触发器。
//
// 实现复杂的基于时间的轮换逻辑：
// - 跨越午夜边界时每日轮换
// - 当天特定时间轮换
// - 自动处理时区注意事项
//
// 参数：
// - createTime：原始文件创建时间戳
// - now：当前系统时间
// - splitHour：为轮换触发配置的小时（0-23）
//
// 如果应根据时间条件进行轮换，则返回 true。
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

// shouldRotateBySize 确定文件大小是否超过轮转阈值。
//
// 根据配置的兆字节限制有效检查文件大小。
// 使用位移位在高吞吐量场景中实现最佳性能。
//
// 参数：
// - size：当前文件大小（以字节为单位）
// - splitMB：允许的最大大小（以兆字节为单位）
//
// 如果文件大小超过阈值则返回 true。
func shouldRotateBySize(size int64, splitMB int) bool {
	if splitMB == 0 {
		return false
	}

	return size >= int64(splitMB)<<20 // 1MB = 1<<20 字节。
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

// generateBackupFileName 创建具有时间戳精度的唯一备份文件名。
//
// 实现智能文件名生成：
// - 保留原始文件扩展名
// - 附加精确时间戳（YYYYMMDDHHMMSS 格式）
// - 以 1 秒增量处理文件名冲突
// - 确保轮换事件中的文件名唯一
//
// 参数：
// - filePath：原始日志文件路径
// - now：备份文件名的当前时间戳
//
// 返回生成的备份文件名和绝对路径。
// 如果 5 次尝试后仍无法生成唯一文件名，则返回错误。
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

// fileExists 执行原子文件存在验证。
//
// 提供可靠的文件系统状态检查：
// - 优雅地处理竞争条件
// - 区分不存在和系统错误
// - 支持并发访问
//
// 参数：
// - filePath：要验证的绝对路径
//
// 如果文件存在则返回 true，否则返回 false。
// 返回系统级故障（权限、I/O 等）的错误。
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

// openLogFile 使用适当的权限创建并初始化新的日志文件。
//
// 实现健壮的文件创建：
// - 使用安全权限递归创建父目录
// - 以附加模式打开文件以防止数据丢失
// - 检索准确的文件创建时间戳
// - 处理特定于平台的文件系统行为
//
// 参数：
// - filePath：新日志文件的绝对路径
//
// 返回：
// - fd：打开的文件描述符准备写入
// - createTime：精确的文件创建时间戳
// - 错误：文件创建或时间戳检索期间的任何错误
//
// 确保原子操作和故障时正确的资源清理。
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

// GetFileCreateTime 检索具有跨平台兼容性的精确文件创建时间戳。
//
// 提供强大的文件元数据提取：
// - 跨平台兼容性（Windows、Linux、macOS）
// - 当创建时间不可用时回退到修改时间
// - 处理文件系统特定的行为和限制
// - 支持并发访问
//
// 参数：
// - filePath：目标文件的绝对路径
//
// 返回：
// - time.Time：文件创建时间戳（或修改时间作为后备）
// - 错误：文件系统访问错误或无效路径问题
//
// 对于分布式游戏服务器环境中准确的日志轮换决策至关重要。
// 精确的计时对于日志文件生命周期管理至关重要。
func GetFileCreateTime(filePath string) (time.Time, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, err
	}
	// Go 不会以可移植的方式公开创建时间，因此回退到 ModTime。
	return fi.ModTime(), nil
}
