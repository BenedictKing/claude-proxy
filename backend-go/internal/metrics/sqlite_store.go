package metrics

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore SQLite 持久化存储
type SQLiteStore struct {
	db     *sql.DB
	dbPath string

	// 写入缓冲区
	writeBuffer []PersistentRecord
	bufferMu    sync.Mutex

	// 配置
	batchSize     int           // 批量写入阈值（记录数）
	flushInterval time.Duration // 定时刷新间隔
	retentionDays int           // 数据保留天数

	// 控制
	stopCh  chan struct{}
	wg      sync.WaitGroup
	closed  bool           // 是否已关闭
	flushWg sync.WaitGroup // 追踪异步 flush goroutine
}

// SQLiteStoreConfig SQLite 存储配置
type SQLiteStoreConfig struct {
	DBPath        string // 数据库文件路径
	RetentionDays int    // 数据保留天数（3-30）
}

// 硬编码的内部配置
const (
	defaultBatchSize     = 100              // 批量写入阈值
	defaultFlushInterval = 30 * time.Second // 定时刷新间隔
)

// NewSQLiteStore 创建 SQLite 存储
func NewSQLiteStore(cfg *SQLiteStoreConfig) (*SQLiteStore, error) {
	if cfg == nil {
		cfg = &SQLiteStoreConfig{
			DBPath:        ".config/metrics.db",
			RetentionDays: 7,
		}
	}

	// 验证保留天数范围
	if cfg.RetentionDays < 3 {
		cfg.RetentionDays = 3
	} else if cfg.RetentionDays > 30 {
		cfg.RetentionDays = 30
	}

	// 确保目录存在
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	// 打开数据库连接（WAL 模式 + NORMAL 同步）
	// modernc.org/sqlite 使用 _pragma= 语法设置 PRAGMA
	dsn := cfg.DBPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(1) // SQLite 单写入连接
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // 不限制连接生命周期

	// 初始化表结构
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库 schema 失败: %w", err)
	}

	store := &SQLiteStore{
		db:            db,
		dbPath:        cfg.DBPath,
		writeBuffer:   make([]PersistentRecord, 0, defaultBatchSize),
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		retentionDays: cfg.RetentionDays,
		stopCh:        make(chan struct{}),
	}

	// 启动后台任务
	store.wg.Add(2)
	go store.flushLoop()
	go store.cleanupLoop()

	log.Printf("[SQLite-Init] 指标存储已初始化: %s (保留 %d 天)", cfg.DBPath, cfg.RetentionDays)
	return store, nil
}

// initSchema 初始化数据库表结构
func initSchema(db *sql.DB) error {
	schema := `
		-- 请求记录表
		CREATE TABLE IF NOT EXISTS request_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			metrics_key TEXT NOT NULL,
			base_url TEXT NOT NULL,
			key_mask TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			success INTEGER NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_creation_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0,
			api_type TEXT NOT NULL DEFAULT 'messages'
		);

		-- 索引：按 api_type 和时间查询
		CREATE INDEX IF NOT EXISTS idx_records_api_type_timestamp
			ON request_records(api_type, timestamp);

		-- 索引：按 metrics_key 查询
		CREATE INDEX IF NOT EXISTS idx_records_metrics_key
			ON request_records(metrics_key);
	`

	_, err := db.Exec(schema)
	return err
}

// AddRecord 添加记录到写入缓冲区（非阻塞）
func (s *SQLiteStore) AddRecord(record PersistentRecord) {
	s.bufferMu.Lock()
	if s.closed {
		s.bufferMu.Unlock()
		return // 已关闭，忽略新记录
	}
	s.writeBuffer = append(s.writeBuffer, record)
	shouldFlush := len(s.writeBuffer) >= s.batchSize
	s.bufferMu.Unlock()

	if shouldFlush {
		s.flushWg.Add(1)
		go func() {
			defer s.flushWg.Done()
			s.flush()
		}()
	}
}

// flush 刷新缓冲区到数据库
func (s *SQLiteStore) flush() {
	s.bufferMu.Lock()
	if len(s.writeBuffer) == 0 {
		s.bufferMu.Unlock()
		return
	}

	// 取出缓冲区数据
	records := s.writeBuffer
	s.writeBuffer = make([]PersistentRecord, 0, s.batchSize)
	s.bufferMu.Unlock()

	// 批量写入
	if err := s.batchInsertRecords(records); err != nil {
		log.Printf("[SQLite-Flush] 警告: 批量写入指标记录失败: %v", err)
		// 失败时将记录放回缓冲区（限制重试，避免无限增长）
		s.bufferMu.Lock()
		if len(s.writeBuffer) < s.batchSize*10 { // 最多保留 10 倍缓冲
			s.writeBuffer = append(records, s.writeBuffer...)
		} else {
			log.Printf("[SQLite-Flush] 警告: 写入缓冲区已满，丢弃 %d 条记录", len(records))
		}
		s.bufferMu.Unlock()
	}
}

// batchInsertRecords 批量插入记录
func (s *SQLiteStore) batchInsertRecords(records []PersistentRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO request_records
		(metrics_key, base_url, key_mask, timestamp, success,
		 input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, api_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		success := 0
		if r.Success {
			success = 1
		}
		_, err := stmt.Exec(
			r.MetricsKey, r.BaseURL, r.KeyMask, r.Timestamp.Unix(), success,
			r.InputTokens, r.OutputTokens, r.CacheCreationTokens, r.CacheReadTokens, r.APIType,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// LoadRecords 加载指定时间范围内的记录
func (s *SQLiteStore) LoadRecords(since time.Time, apiType string) ([]PersistentRecord, error) {
	rows, err := s.db.Query(`
		SELECT metrics_key, base_url, key_mask, timestamp, success,
		       input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens
		FROM request_records
		WHERE timestamp >= ? AND api_type = ?
		ORDER BY timestamp ASC
	`, since.Unix(), apiType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []PersistentRecord
	for rows.Next() {
		var r PersistentRecord
		var ts int64
		var success int

		err := rows.Scan(
			&r.MetricsKey, &r.BaseURL, &r.KeyMask, &ts, &success,
			&r.InputTokens, &r.OutputTokens, &r.CacheCreationTokens, &r.CacheReadTokens,
		)
		if err != nil {
			return nil, err
		}

		r.Timestamp = time.Unix(ts, 0)
		r.Success = success == 1
		r.APIType = apiType
		records = append(records, r)
	}

	return records, rows.Err()
}

// CleanupOldRecords 清理过期数据
func (s *SQLiteStore) CleanupOldRecords(before time.Time) (int64, error) {
	result, err := s.db.Exec(
		"DELETE FROM request_records WHERE timestamp < ?",
		before.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// flushLoop 定时刷新循环
func (s *SQLiteStore) flushLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.stopCh:
			s.flush() // 关闭前最后一次刷新
			return
		}
	}
}

// cleanupLoop 定期清理循环
func (s *SQLiteStore) cleanupLoop() {
	defer s.wg.Done()

	// 启动时先清理一次
	s.doCleanup()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.doCleanup()
		case <-s.stopCh:
			return
		}
	}
}

// doCleanup 执行清理
func (s *SQLiteStore) doCleanup() {
	cutoff := time.Now().AddDate(0, 0, -s.retentionDays)
	deleted, err := s.CleanupOldRecords(cutoff)
	if err != nil {
		log.Printf("[SQLite-Cleanup] 警告: 清理过期指标记录失败: %v", err)
	} else if deleted > 0 {
		log.Printf("[SQLite-Cleanup] 已清理 %d 条过期指标记录（超过 %d 天）", deleted, s.retentionDays)
	}
}

// Close 关闭存储
func (s *SQLiteStore) Close() error {
	// 标记为已关闭，阻止新记录
	s.bufferMu.Lock()
	s.closed = true
	s.bufferMu.Unlock()

	// 停止后台循环
	close(s.stopCh)
	s.wg.Wait()

	// 等待所有异步 flush 完成
	s.flushWg.Wait()

	return s.db.Close()
}

// GetRecordCount 获取记录总数（用于调试）
func (s *SQLiteStore) GetRecordCount() (int64, error) {
	var count int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM request_records").Scan(&count)
	return count, err
}
