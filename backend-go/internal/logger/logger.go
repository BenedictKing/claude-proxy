package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 日志配置
type Config struct {
	// 日志目录
	LogDir string
	// 日志文件名
	LogFile string
	// 单个日志文件最大大小 (MB)
	MaxSize int
	// 保留的旧日志文件最大数量
	MaxBackups int
	// 保留的旧日志文件最大天数
	MaxAge int
	// 是否压缩旧日志文件
	Compress bool
	// 是否同时输出到控制台
	Console bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		LogDir:     "logs",
		LogFile:    "app.log",
		MaxSize:    100, // 100MB
		MaxBackups: 10,
		MaxAge:     30, // 30 days
		Compress:   true,
		Console:    true,
	}
}

// Setup 初始化日志系统
func Setup(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 确保日志目录存在
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	logPath := filepath.Join(cfg.LogDir, cfg.LogFile)

	// 配置 lumberjack 日志轮转
	lumberLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	var writer io.Writer
	if cfg.Console {
		// 同时输出到控制台和文件
		writer = io.MultiWriter(os.Stdout, lumberLogger)
	} else {
		// 仅输出到文件
		writer = lumberLogger
	}

	// 设置标准库 log 的输出
	log.SetOutput(writer)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Printf("📝 日志系统已初始化")
	log.Printf("📂 日志文件: %s", logPath)
	log.Printf("📊 轮转配置: 最大 %dMB, 保留 %d 个备份, %d 天", cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)

	return nil
}
