// Package messages 提供 Claude Messages API 的处理器
package messages

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/BenedictKing/claude-proxy/internal/config"
	"github.com/BenedictKing/claude-proxy/internal/httpclient"
	"github.com/BenedictKing/claude-proxy/internal/middleware"
	"github.com/gin-gonic/gin"
)

const modelsRequestTimeout = 30 * time.Second

// ModelsHandler 处理 /v1/models 请求，转发到上游
func ModelsHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		middleware.ProxyAuthMiddleware(envCfg)(c)
		if c.IsAborted() {
			return
		}

		if body, ok := tryModelsRequest(c, cfgManager, "GET", ""); ok {
			c.Data(http.StatusOK, "application/json", body)
			return
		}

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "models endpoint not available from any upstream",
				"type":    "not_found_error",
			},
		})
	}
}

// ModelsDetailHandler 处理 /v1/models/:model 请求，转发到上游
func ModelsDetailHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
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

		if body, ok := tryModelsRequest(c, cfgManager, "GET", "/"+modelID); ok {
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

// ModelsDeleteHandler 处理 DELETE /v1/models/:model 请求
func ModelsDeleteHandler(envCfg *config.EnvConfig, cfgManager *config.ConfigManager) gin.HandlerFunc {
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

		if body, ok := tryModelsRequest(c, cfgManager, "DELETE", "/"+modelID); ok {
			c.Data(http.StatusOK, "application/json", body)
			return
		}

		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "model not found or cannot be deleted",
				"type":    "not_found_error",
			},
		})
	}
}

// tryModelsRequest 遍历所有渠道的所有 key，尝试请求 models 端点
func tryModelsRequest(c *gin.Context, cfgManager *config.ConfigManager, method, suffix string) ([]byte, bool) {
	cfg := cfgManager.GetConfig()

	for _, upstream := range cfg.Upstream {
		// 跳过 Claude 原生 API（不支持 /models）
		if upstream.ServiceType == "claude" || len(upstream.APIKeys) == 0 {
			continue
		}

		url := buildModelsURL(upstream.BaseURL) + suffix
		client := httpclient.GetManager().GetStandardClient(modelsRequestTimeout, upstream.InsecureSkipVerify)

		// 遍历该渠道的所有 key
		for _, apiKey := range upstream.APIKeys {
			req, err := http.NewRequestWithContext(c.Request.Context(), method, url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode == http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					continue
				}
				return body, true
			}
			resp.Body.Close()
		}
	}

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
