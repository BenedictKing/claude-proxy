// Package messages 提供 Claude Messages API 的处理器
package messages

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/BenedictKing/claude-proxy/internal/scheduler"
	"github.com/BenedictKing/claude-proxy/internal/utils"
	"github.com/gin-gonic/gin"
)

const modelsRequestTimeout = 30 * time.Second

// ModelsResponse OpenAI 兼容的 models 响应格式
type ModelsResponse struct {
	Object string       `json:"object"`
	Data   []ModelEntry `json:"data"`
}

// ModelEntry 单个模型条目
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsHandler 处理 /v1/models 请求，从 Messages 和 Responses 渠道获取并合并模型列表
func ModelsHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		// 并行从两种渠道获取模型列表
		messagesModels := fetchModelsFromChannels(c, cfgManager, channelScheduler, false)
		responsesModels := fetchModelsFromChannels(c, cfgManager, channelScheduler, true)

		// 合并去重
		mergedModels := mergeModels(messagesModels, responsesModels)

		if len(mergedModels) == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "models endpoint not available from any upstream",
					"type":    "not_found_error",
				},
			})
			return
		}

		response := ModelsResponse{
			Object: "list",
			Data:   mergedModels,
		}

		log.Printf("[Models] 合并完成: messages=%d, responses=%d, merged=%d",
			len(messagesModels), len(responsesModels), len(mergedModels))

		c.JSON(http.StatusOK, response)
	}
}

// ModelsDetailHandler 处理 /v1/models/:model 请求，转发到上游
func ModelsDetailHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler) gin.HandlerFunc {
	return func(c *gin.Context) {
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		modelID := c.Param("model")
		if modelID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"message": "model id is required",
					"type":    "invalid_request_error",
				},
			})
			return
		}

		// 先尝试 Messages 渠道
		if body, ok := tryModelsRequest(c, cfgManager, channelScheduler, "GET", "/"+modelID, false); ok {
			c.Data(http.StatusOK, "application/json", body)
			return
		}

		// 再尝试 Responses 渠道
		if body, ok := tryModelsRequest(c, cfgManager, channelScheduler, "GET", "/"+modelID, true); ok {
			c.Data(http.StatusOK, "application/json", body)
			return
		}

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "model not found",
				"type":    "not_found_error",
			},
		})
	}
}

// fetchModelsFromChannels 从指定类型的渠道获取模型列表
func fetchModelsFromChannels(c *gin.Context, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler, isResponses bool) []ModelEntry {
	body, ok := tryModelsRequest(c, cfgManager, channelScheduler, "GET", "", isResponses)
	if !ok {
		return nil
	}

	var resp ModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		channelType := "Messages"
		if isResponses {
			channelType = "Responses"
		}
		log.Printf("[Models] 解析 %s 渠道响应失败: %v", channelType, err)
		return nil
	}

	return resp.Data
}

// mergeModels 合并两个模型列表并去重（按 ID）
func mergeModels(models1, models2 []ModelEntry) []ModelEntry {
	seen := make(map[string]bool)
	var result []ModelEntry

	// 先添加第一个列表的模型
	for _, m := range models1 {
		if !seen[m.ID] {
			seen[m.ID] = true
			result = append(result, m)
		}
	}

	// 再添加第二个列表中不重复的模型
	for _, m := range models2 {
		if !seen[m.ID] {
			seen[m.ID] = true
			result = append(result, m)
		}
	}

	return result
}

// tryModelsRequest 使用调度器选择渠道，按故障转移顺序尝试请求 models 端点
func tryModelsRequest(c *gin.Context, cfgManager *config.ConfigManager, channelScheduler *scheduler.ChannelScheduler, method, suffix string, isResponses bool) ([]byte, bool) {
	failedChannels := make(map[int]bool)
	maxChannelRetries := 10 // 最多尝试 10 个渠道

	channelType := "Messages"
	if isResponses {
		channelType = "Responses"
	}

	for attempt := 0; attempt < maxChannelRetries; attempt++ {
		// 使用调度器选择渠道
		selection, err := channelScheduler.SelectChannel(c.Request.Context(), "", failedChannels, isResponses)
		if err != nil {
			log.Printf("[Models] %s 渠道无可用: %v", channelType, err)
			break
		}

		upstream := selection.Upstream

		// 尝试该渠道的第一个 key
		if len(upstream.APIKeys) == 0 {
			failedChannels[selection.ChannelIndex] = true
			continue
		}

		url := buildModelsURL(upstream.BaseURL) + suffix
		client := httpclient.GetManager().GetStandardClient(modelsRequestTimeout, upstream.InsecureSkipVerify)

		// 获取第一个可用的 key
		apiKey, err := cfgManager.GetNextAPIKey(upstream, nil, "Messages")
		if err != nil {
			log.Printf("[Models] %s 获取 API Key 失败: channel=%s, error=%v", channelType, upstream.Name, err)
			failedChannels[selection.ChannelIndex] = true
			continue
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), method, url, nil)
		if err != nil {
			log.Printf("[Models] %s 创建请求失败: channel=%s, url=%s, error=%v", channelType, upstream.Name, url, err)
			failedChannels[selection.ChannelIndex] = true
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[Models] %s 请求失败: channel=%s, key=%s, url=%s, error=%v",
				channelType, upstream.Name, utils.MaskAPIKey(apiKey), url, err)
			failedChannels[selection.ChannelIndex] = true
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("[Models] %s 读取响应失败: channel=%s, error=%v", channelType, upstream.Name, err)
				failedChannels[selection.ChannelIndex] = true
				continue
			}
			log.Printf("[Models] %s 请求成功: method=%s, channel=%s, key=%s, url=%s, reason=%s",
				channelType, method, upstream.Name, utils.MaskAPIKey(apiKey), url, selection.Reason)
			return body, true
		}

		log.Printf("[Models] %s 上游返回非 200: channel=%s, key=%s, status=%d, url=%s",
			channelType, upstream.Name, utils.MaskAPIKey(apiKey), resp.StatusCode, url)
		resp.Body.Close()
		failedChannels[selection.ChannelIndex] = true
	}

	log.Printf("[Models] %s 所有渠道均失败: method=%s, suffix=%s", channelType, method, suffix)
	return nil, false
}

// buildModelsURL 构建 models 端点的 URL
func buildModelsURL(baseURL string) string {
	skipVersionPrefix := strings.HasSuffix(baseURL, "#")
	if skipVersionPrefix {
		baseURL = strings.TrimSuffix(baseURL, "#")
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	versionPattern := regexp.MustCompile(`/v\d+[a-z]*$`)
	hasVersionSuffix := versionPattern.MatchString(baseURL)

	endpoint := "/models"
	if !hasVersionSuffix && !skipVersionPrefix {
		endpoint = "/v1" + endpoint
	}

	return baseURL + endpoint
}
