package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
)

// ClientManager HTTP 客户端管理器
type ClientManager struct {
	mu      sync.RWMutex
	clients map[string]*http.Client
}

var globalManager = &ClientManager{
	clients: make(map[string]*http.Client),
}

// GetManager 获取全局客户端管理器
func GetManager() *ClientManager {
	return globalManager
}

// GetStandardClient 获取标准客户端（有超时，用于普通请求）
// 注意：启用自动压缩让Go处理gzip，配合请求头清理确保正确解压
func (cm *ClientManager) GetStandardClient(timeout time.Duration, insecure bool) *http.Client {
	// 从配置获取响应头超时时间
	envConfig := config.NewEnvConfig()
	responseHeaderTimeout := time.Duration(envConfig.ResponseHeaderTimeout) * time.Second

	key := fmt.Sprintf("standard-%d-%t-%d", timeout, insecure, envConfig.ResponseHeaderTimeout)

	cm.mu.RLock()
	if client, ok := cm.clients[key]; ok {
		cm.mu.RUnlock()
		return client
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 双重检查，避免重复创建
	if client, ok := cm.clients[key]; ok {
		return client
	}

	transport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		DisableCompression:    false, // 启用自动压缩，让Go处理gzip
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	cm.clients[key] = client
	return client
}

// GetStreamClient 获取流式客户端（无超时，用于 SSE 流式响应）
func (cm *ClientManager) GetStreamClient(insecure bool) *http.Client {
	// 从配置获取响应头超时时间
	envConfig := config.NewEnvConfig()
	responseHeaderTimeout := time.Duration(envConfig.ResponseHeaderTimeout) * time.Second

	key := fmt.Sprintf("stream-%t-%d", insecure, envConfig.ResponseHeaderTimeout)

	cm.mu.RLock()
	if client, ok := cm.clients[key]; ok {
		cm.mu.RUnlock()
		return client
	}
	cm.mu.RUnlock()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 双重检查
	if client, ok := cm.clients[key]; ok {
		return client
	}

	transport := &http.Transport{
		MaxIdleConns:          200, // 流式连接池更大
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       120 * time.Second,
		DisableCompression:    true, // 流式响应禁用压缩
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // 流式请求无超时
	}

	cm.clients[key] = client
	return client
}
