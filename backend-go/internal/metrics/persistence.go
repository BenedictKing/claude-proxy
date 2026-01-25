package metrics

import (
	"time"
)

// PersistenceStore 持久化存储接口
type PersistenceStore interface {
	// AddRecord 添加记录到写入缓冲区（非阻塞）
	AddRecord(record PersistentRecord)

	// LoadRecords 加载指定时间范围内的记录
	LoadRecords(since time.Time, apiType string) ([]PersistentRecord, error)

	// CleanupOldRecords 清理过期数据
	CleanupOldRecords(before time.Time) (int64, error)

	// DeleteRecordsByMetricsKeys 按 metrics_key 和 api_type 批量删除记录（用于删除渠道时清理数据）
	// apiType: 接口类型（messages/responses/gemini），避免误删其他接口的数据
	DeleteRecordsByMetricsKeys(metricsKeys []string, apiType string) (int64, error)

	// Close 关闭存储（会先刷新缓冲区）
	Close() error
}

// PersistentRecord 持久化记录结构
type PersistentRecord struct {
	MetricsKey          string    // hash(baseURL + apiKey)
	BaseURL             string    // 上游 BaseURL
	KeyMask             string    // 脱敏的 API Key
	Timestamp           time.Time // 请求时间
	Success             bool      // 是否成功
	InputTokens         int64     // 输入 Token 数
	OutputTokens        int64     // 输出 Token 数
	CacheCreationTokens int64     // 缓存创建 Token
	CacheReadTokens     int64     // 缓存读取 Token
	APIType             string    // "messages" 或 "responses"
}
