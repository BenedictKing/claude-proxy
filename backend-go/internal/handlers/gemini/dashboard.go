package gemini

import (
	"github.com/gin-gonic/gin"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/metrics"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
)

// GetDashboard 获取 Gemini 渠道仪表盘数据（合并 channels + metrics + stats + recentActivity）
// GET /api/gemini/channels/dashboard
// 将原本需要 3 个请求的数据合并为 1 个请求，减少网络开销
func GetDashboard(cfgManager *config.ConfigManager, sch *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := cfgManager.GetConfig()
		upstreams := cfg.GeminiUpstream
		loadBalance := cfg.GeminiLoadBalance
		metricsManager := sch.GetGeminiMetricsManager()

		// 1. 构建 channels 数据
		channels := make([]gin.H, len(upstreams))
		for i, up := range upstreams {
			status := config.GetChannelStatus(&up)
			priority := config.GetChannelPriority(&up, i)

			channels[i] = gin.H{
				"index":                       i,
				"name":                        up.Name,
				"serviceType":                 up.ServiceType,
				"baseUrl":                     up.BaseURL,
				"baseUrls":                    up.BaseURLs,
				"apiKeys":                     up.APIKeys,
				"description":                 up.Description,
				"website":                     up.Website,
				"insecureSkipVerify":          up.InsecureSkipVerify,
				"modelMapping":                up.ModelMapping,
				"latency":                     nil,
				"status":                      status,
				"priority":                    priority,
				"promotionUntil":              up.PromotionUntil,
				"lowQuality":                  up.LowQuality,
				"injectDummyThoughtSignature": up.InjectDummyThoughtSignature,
				"stripThoughtSignature":       up.StripThoughtSignature,
			}
		}

		// 2. 构建 metrics 数据
		metricsResult := make([]gin.H, 0, len(upstreams))
		for i, upstream := range upstreams {
			resp := metricsManager.ToResponseMultiURL(i, upstream.GetAllBaseURLs(), upstream.APIKeys, 0, upstream.HistoricalAPIKeys)

			item := gin.H{
				"channelIndex":        i,
				"channelName":         upstream.Name,
				"requestCount":        resp.RequestCount,
				"successCount":        resp.SuccessCount,
				"failureCount":        resp.FailureCount,
				"successRate":         resp.SuccessRate,
				"errorRate":           resp.ErrorRate,
				"consecutiveFailures": resp.ConsecutiveFailures,
				"latency":             resp.Latency,
				"keyMetrics":          resp.KeyMetrics,
				"timeWindows":         resp.TimeWindows,
			}

			if resp.LastSuccessAt != nil {
				item["lastSuccessAt"] = *resp.LastSuccessAt
			}
			if resp.LastFailureAt != nil {
				item["lastFailureAt"] = *resp.LastFailureAt
			}
			if resp.CircuitBrokenAt != nil {
				item["circuitBrokenAt"] = *resp.CircuitBrokenAt
			}

			metricsResult = append(metricsResult, item)
		}

		// 3. 构建 stats 数据
		stats := gin.H{
			"multiChannelMode":    sch.IsMultiChannelMode(scheduler.ChannelKindGemini),
			"activeChannelCount":  sch.GetActiveChannelCount(scheduler.ChannelKindGemini),
			"traceAffinityCount":  sch.GetTraceAffinityManager().Size(),
			"traceAffinityTTL":    sch.GetTraceAffinityManager().GetTTL().String(),
			"failureThreshold":    metricsManager.GetFailureThreshold() * 100,
			"windowSize":          metricsManager.GetWindowSize(),
			"circuitRecoveryTime": metricsManager.GetCircuitRecoveryTime().String(),
		}

		// 4. 构建 recentActivity 数据（最近 15 分钟分段活跃度）
		recentActivity := make([]*metrics.ChannelRecentActivity, len(upstreams))
		for i, upstream := range upstreams {
			recentActivity[i] = metricsManager.GetRecentActivityMultiURL(i, upstream.GetAllBaseURLs(), upstream.APIKeys)
		}

		// 返回合并数据
		c.JSON(200, gin.H{
			"channels":       channels,
			"loadBalance":    loadBalance,
			"metrics":        metricsResult,
			"stats":          stats,
			"recentActivity": recentActivity,
		})
	}
}
