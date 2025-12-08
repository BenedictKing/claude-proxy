package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/metrics"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/gin-gonic/gin"
)

// GetChannelMetrics 获取渠道指标
func GetChannelMetrics(metricsManager *metrics.MetricsManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		allMetrics := metricsManager.GetAllMetrics()

		// 转换为 API 响应格式
		result := make([]gin.H, 0, len(allMetrics))
		for _, m := range allMetrics {
			if m == nil {
				continue
			}
			failureRate := metricsManager.CalculateFailureRate(m.ChannelIndex)
			successRate := (1 - failureRate) * 100

			// 获取分时段统计
			timeWindowStats := metricsManager.GetAllTimeWindowStats(m.ChannelIndex)

			item := gin.H{
				"channelIndex":        m.ChannelIndex,
				"requestCount":        m.RequestCount,
				"successCount":        m.SuccessCount,
				"failureCount":        m.FailureCount,
				"successRate":         successRate,
				"errorRate":           failureRate * 100,
				"consecutiveFailures": m.ConsecutiveFailures,
				"latency":             0, // 需要从其他地方获取
				"timeWindows":         timeWindowStats,
			}

			if m.LastSuccessAt != nil {
				item["lastSuccessAt"] = m.LastSuccessAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if m.LastFailureAt != nil {
				item["lastFailureAt"] = m.LastFailureAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if m.CircuitBrokenAt != nil {
				item["circuitBrokenAt"] = m.CircuitBrokenAt.Format("2006-01-02T15:04:05Z07:00")
			}

			result = append(result, item)
		}

		c.JSON(200, result)
	}
}

// GetResponsesChannelMetrics 获取 Responses 渠道指标
// 传入 Responses 专用的 MetricsManager 实例
func GetResponsesChannelMetrics(metricsManager *metrics.MetricsManager) gin.HandlerFunc {
	return GetChannelMetrics(metricsManager)
}

// ResumeChannel 恢复熔断渠道（重置错误计数）
// isResponses 参数指定是 Messages 渠道还是 Responses 渠道
func ResumeChannel(sch *scheduler.ChannelScheduler, isResponses bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "Invalid channel ID"})
			return
		}

		// 重置渠道指标
		sch.ResetChannelMetrics(id, isResponses)

		c.JSON(200, gin.H{
			"success": true,
			"message": "渠道已恢复，错误计数已重置",
		})
	}
}

// GetSchedulerStats 获取调度器统计信息
func GetSchedulerStats(sch *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取 isResponses 参数
		isResponses := strings.ToLower(c.Query("type")) == "responses"

		// 根据类型选择对应的指标管理器
		var metricsManager *metrics.MetricsManager
		if isResponses {
			metricsManager = sch.GetResponsesMetricsManager()
		} else {
			metricsManager = sch.GetMessagesMetricsManager()
		}

		stats := gin.H{
			"multiChannelMode":    sch.IsMultiChannelMode(isResponses),
			"activeChannelCount":  sch.GetActiveChannelCount(isResponses),
			"traceAffinityCount":  sch.GetTraceAffinityManager().Size(),
			"traceAffinityTTL":    sch.GetTraceAffinityManager().GetTTL().String(),
			"failureThreshold":    metricsManager.GetFailureThreshold() * 100,
			"windowSize":          metricsManager.GetWindowSize(),
			"circuitRecoveryTime": metricsManager.GetCircuitRecoveryTime().String(),
		}

		c.JSON(200, stats)
	}
}

// SetChannelPromotion 设置渠道促销期
// 促销期内的渠道会被优先选择，忽略 trace 亲和性
func SetChannelPromotion(cfgManager ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "无效的渠道 ID"})
			return
		}

		var req struct {
			Duration int `json:"duration"` // 促销期时长（秒），0 表示清除
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "无效的请求参数"})
			return
		}

		// 调用配置管理器设置促销期
		duration := time.Duration(req.Duration) * time.Second
		if err := cfgManager.SetChannelPromotion(id, duration); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		if req.Duration <= 0 {
			c.JSON(200, gin.H{
				"success": true,
				"message": "渠道促销期已清除",
			})
		} else {
			c.JSON(200, gin.H{
				"success":  true,
				"message":  "渠道促销期已设置",
				"duration": req.Duration,
			})
		}
	}
}

// SetResponsesChannelPromotion 设置 Responses 渠道促销期
func SetResponsesChannelPromotion(cfgManager ResponsesConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			c.JSON(400, gin.H{"error": "无效的渠道 ID"})
			return
		}

		var req struct {
			Duration int `json:"duration"` // 促销期时长（秒），0 表示清除
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "无效的请求参数"})
			return
		}

		duration := time.Duration(req.Duration) * time.Second
		if err := cfgManager.SetResponsesChannelPromotion(id, duration); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		if req.Duration <= 0 {
			c.JSON(200, gin.H{
				"success": true,
				"message": "Responses 渠道促销期已清除",
			})
		} else {
			c.JSON(200, gin.H{
				"success":  true,
				"message":  "Responses 渠道促销期已设置",
				"duration": req.Duration,
			})
		}
	}
}

// ConfigManager 促销期配置管理接口
type ConfigManager interface {
	SetChannelPromotion(index int, duration time.Duration) error
}

// ResponsesConfigManager Responses 渠道促销期配置管理接口
type ResponsesConfigManager interface {
	SetResponsesChannelPromotion(index int, duration time.Duration) error
}
