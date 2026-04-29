package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)

// Logger 日志管理器（支持日志轮转）
type Logger struct {
	mu            sync.Mutex
	file          *os.File
	logPath       string
	backupCount   int
	currentDate   string
	enabled       bool
	level         LogLevel
	debugEnabled  bool
}

// 全局日志实例
var globalLogger *Logger

// InitLogger 初始化日志系统
func InitLogger(cfg *Settings) error {
	level := LogLevelInfo
	if strings.ToUpper(cfg.LogLevel) == "DEBUG" {
		level = LogLevelDebug
	} else if strings.ToUpper(cfg.LogLevel) == "WARNING" {
		level = LogLevelWarning
	} else if strings.ToUpper(cfg.LogLevel) == "ERROR" {
		level = LogLevelError
	}

	globalLogger = &Logger{
		logPath:      cfg.LogFile,
		backupCount:  cfg.LogBackupCount,
		enabled:      cfg.LogEnable,
		level:        level,
		debugEnabled: level == LogLevelDebug,
	}

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[node-agent] ")

	if level == LogLevelDebug {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile | log.Lmsgprefix)
	}

	// 如果启用文件日志
	if cfg.LogEnable && cfg.LogFile != "" {
		if err := globalLogger.openLogFile(); err != nil {
			log.Printf("警告: 无法打开日志文件 %s: %v", cfg.LogFile, err)
			// 继续使用控制台日志
		} else {
			// 设置日志输出到文件和控制台
			log.SetOutput(io.MultiWriter(os.Stdout, globalLogger.file))
			log.Printf("日志文件: %s (保留 %d 天)", cfg.LogFile, cfg.LogBackupCount)
		}
	}

	return nil
}

// openLogFile 打开日志文件
func (l *Logger) openLogFile() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 日志路径（相对路径基于 WorkingDirectory，systemd 已设置为 /opt/rivision/rivision_node）
	logPath := l.logPath

	// 确保目录存在
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 打开日志文件
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	l.file = file
	l.currentDate = time.Now().Format("2006-01-02")

	return nil
}

// checkRotate 检查是否需要轮转
func (l *Logger) checkRotate() {
	if l.file == nil || !l.enabled {
		return
	}

	today := time.Now().Format("2006-01-02")
	if today == l.currentDate {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 日期变化，执行轮转
	l.rotate()
	l.currentDate = today
}

// rotate 执行日志轮转
func (l *Logger) rotate() {
	if l.file == nil {
		return
	}

	// 关闭当前文件
	l.file.Close()

	// 日志路径（相对路径基于 WorkingDirectory）
	logPath := l.logPath

	// 重命名为带日期的备份文件
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	backupPath := logPath + "." + yesterday

	os.Rename(logPath, backupPath)

	// 清理旧备份
	l.cleanOldBackups(logPath)

	// 重新打开日志文件
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("重新打开日志文件失败: %v", err)
		l.file = nil
		log.SetOutput(os.Stdout)
		return
	}

	l.file = file
	log.SetOutput(io.MultiWriter(os.Stdout, l.file))
	log.Printf("日志已轮转: %s", backupPath)
}

// cleanOldBackups 清理旧的备份文件
func (l *Logger) cleanOldBackups(logPath string) {
	dir := filepath.Dir(logPath)
	base := filepath.Base(logPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// 查找所有备份文件
	var backups []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base+".") && len(name) > len(base)+1 {
			// 检查是否是日期后缀 (YYYY-MM-DD)
			suffix := name[len(base)+1:]
			if len(suffix) == 10 && suffix[4] == '-' && suffix[7] == '-' {
				backups = append(backups, filepath.Join(dir, name))
			}
		}
	}

	// 按名称排序（日期格式保证排序正确）
	sort.Strings(backups)

	// 删除超出保留数量的备份
	if len(backups) > l.backupCount {
		toDelete := backups[:len(backups)-l.backupCount]
		for _, path := range toDelete {
			if err := os.Remove(path); err == nil {
				log.Printf("删除旧日志备份: %s", path)
			}
		}
	}
}

// Close 关闭日志文件
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// LogDebug 调试日志（仅在 DEBUG 级别输出）
func LogDebug(format string, v ...interface{}) {
	if globalLogger != nil && globalLogger.debugEnabled {
		globalLogger.checkRotate()
		log.Printf("[DEBUG] "+format, v...)
	}
}

// LogInfo 信息日志
func LogInfo(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.checkRotate()
	}
	log.Printf(format, v...)
}

// LogWarning 警告日志
func LogWarning(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.checkRotate()
	}
	log.Printf("[WARN] "+format, v...)
}

// LogError 错误日志
func LogError(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.checkRotate()
	}
	log.Printf("[ERROR] "+format, v...)
}

// CloseLogger 关闭全局日志
func CloseLogger() {
	if globalLogger != nil {
		globalLogger.Close()
	}
}
