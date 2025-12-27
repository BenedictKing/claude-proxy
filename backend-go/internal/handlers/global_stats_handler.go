package handlers

import (
	"time"

	"github.com/BenedictKing/claude-proxy/internal/metrics"
	"github.com/gin-gonic/gin"
)

// GetGlobalStatsHistory 获取全局统计历史数据
// GET /api/{messages|responses}/global/stats/history?duration={1h|6h|24h|today}
func GetGlobalStatsHistory(metricsManager *metrics.MetricsManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 解析 duration 参数
		durationStr := c.DefaultQuery("duration", "24h")

		var duration time.Duration
		var err error

		// 特殊处理 "today" 参数
		if durationStr == "today" {
			duration = metrics.CalculateTodayDuration()
			// 如果刚过零点，duration 可能非常小，设置最小值
			if duration < time.Minute {
				duration = time.Minute
			}
		} else {
			duration, err = time.ParseDuration(durationStr)
			if err != nil {
				c.JSON(400, gin.H{"error": "Invalid duration parameter. Use: 1h, 6h, 24h, or today"})
				return
			}
		}

		// 限制最大查询范围为 24 小时
		if duration > 24*time.Hour {
			duration = 24 * time.Hour
		}

		// 解析或自动选择 interval
		intervalStr := c.Query("interval")
		var interval time.Duration
		if intervalStr != "" {
			interval, err = time.ParseDuration(intervalStr)
			if err != nil {
				c.JSON(400, gin.H{"error": "Invalid interval parameter"})
				return
			}
			// 限制 interval 最小值为 1 分钟，防止生成过多 bucket
			if interval < time.Minute {
				interval = time.Minute
			}
		} else {
			// 根据 duration 自动选择合适的聚合粒度
			// 目标：每个时间段约 60-100 个数据点，保持图表清晰
			// 1h = 60 points (1m interval)
			// 6h = 72 points (5m interval)
			// 24h = 96 points (15m interval)
			switch {
			case duration <= time.Hour:
				interval = time.Minute
			case duration <= 6*time.Hour:
				interval = 5 * time.Minute
			default:
				interval = 15 * time.Minute
			}
		}

		// 获取全局统计数据
		result := metricsManager.GetGlobalHistoricalStatsWithTokens(duration, interval)

		// 更新 duration 字符串（特别是 today 情况）
		if durationStr == "today" {
			result.Summary.Duration = "today"
		}

		c.JSON(200, result)
	}
}
