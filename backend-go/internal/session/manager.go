package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/types"
)

// Session 会话数据结构
type Session struct {
	ID             string                // sess_xxxxx
	Messages       []types.ResponsesItem // 完整对话历史
	LastResponseID string                // 最后一个 response ID
	CreatedAt      time.Time
	LastAccessAt   time.Time
	TotalTokens    int
}

// SessionManager 会话管理器
type SessionManager struct {
	sessions        map[string]*Session // sessionID → Session
	responseMapping map[string]string   // responseID → sessionID
	mu              sync.RWMutex

	// 清理配置
	maxAge      time.Duration // 24小时
	maxMessages int           // 100条
	maxTokens   int           // 100k
}

// NewSessionManager 创建会话管理器
func NewSessionManager(maxAge time.Duration, maxMessages int, maxTokens int) *SessionManager {
	sm := &SessionManager{
		sessions:        make(map[string]*Session),
		responseMapping: make(map[string]string),
		maxAge:          maxAge,
		maxMessages:     maxMessages,
		maxTokens:       maxTokens,
	}

	// 启动定期清理
	go sm.cleanupLoop()

	return sm
}

// GetOrCreateSession 获取或创建会话
func (sm *SessionManager) GetOrCreateSession(previousResponseID string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 如果提供了 previousResponseID，尝试查找对应的会话
	if previousResponseID != "" {
		if sessionID, ok := sm.responseMapping[previousResponseID]; ok {
			if session, exists := sm.sessions[sessionID]; exists {
				session.LastAccessAt = time.Now()
				return session, nil
			}
		}
		// 如果找不到对应会话，返回错误
		return nil, fmt.Errorf("无效的 previous_response_id: %s", previousResponseID)
	}

	// 创建新会话
	sessionID := generateID("sess")
	session := &Session{
		ID:           sessionID,
		Messages:     []types.ResponsesItem{},
		CreatedAt:    time.Now(),
		LastAccessAt: time.Now(),
		TotalTokens:  0,
	}

	sm.sessions[sessionID] = session
	log.Printf("📝 创建新会话: %s", sessionID)

	return session, nil
}

// RecordResponseMapping 记录 responseID 到 sessionID 的映射
func (sm *SessionManager) RecordResponseMapping(responseID, sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.responseMapping[responseID] = sessionID
	log.Printf("🔗 记录映射: %s → %s", responseID, sessionID)
}

// AppendMessage 追加消息到会话
func (sm *SessionManager) AppendMessage(sessionID string, item types.ResponsesItem, tokensUsed int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	session.Messages = append(session.Messages, item)
	session.TotalTokens += tokensUsed
	session.LastAccessAt = time.Now()

	return nil
}

// UpdateLastResponseID 更新会话的最后一个 responseID
func (sm *SessionManager) UpdateLastResponseID(sessionID, responseID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	session.LastResponseID = responseID
	return nil
}

// GetSession 获取会话（只读）
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	return session, nil
}

// cleanupLoop 定期清理过期会话
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanup()
	}
}

// cleanup 执行清理逻辑
func (sm *SessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	removedSessions := 0
	removedMappings := 0

	// 清理过期会话
	for sessionID, session := range sm.sessions {
		shouldRemove := false

		// 时间过期
		if now.Sub(session.LastAccessAt) > sm.maxAge {
			shouldRemove = true
			log.Printf("🧹 清理过期会话 (时间): %s (最后访问: %v 前)", sessionID, now.Sub(session.LastAccessAt))
		}

		// 消息数超限
		if len(session.Messages) > sm.maxMessages {
			shouldRemove = true
			log.Printf("🧹 清理过期会话 (消息数): %s (%d 条)", sessionID, len(session.Messages))
		}

		// Token 超限
		if session.TotalTokens > sm.maxTokens {
			shouldRemove = true
			log.Printf("🧹 清理过期会话 (Token): %s (%d tokens)", sessionID, session.TotalTokens)
		}

		if shouldRemove {
			delete(sm.sessions, sessionID)
			removedSessions++
		}
	}

	// 清理孤立的 responseID 映射
	for responseID, sessionID := range sm.responseMapping {
		if _, exists := sm.sessions[sessionID]; !exists {
			delete(sm.responseMapping, responseID)
			removedMappings++
		}
	}

	if removedSessions > 0 || removedMappings > 0 {
		log.Printf("🧹 清理完成: 删除 %d 个会话, %d 个映射", removedSessions, removedMappings)
		log.Printf("📊 当前活跃会话: %d 个, 映射: %d 个", len(sm.sessions), len(sm.responseMapping))
	}
}

// GetStats 获取统计信息
func (sm *SessionManager) GetStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"total_sessions": len(sm.sessions),
		"total_mappings": len(sm.responseMapping),
	}
}

// generateID 生成唯一ID
func generateID(prefix string) string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// 降级方案：使用时间戳
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(bytes))
}
