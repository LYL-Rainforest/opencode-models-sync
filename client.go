package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// modelEntry 对应 OpenAI 兼容接口返回的单个模型对象。
type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// modelsResponse 对应 GET /v1/models 的响应结构。
type modelsResponse struct {
	Data []modelEntry `json:"data"`
}

// FetchModels 向 LM Studio 的 OpenAI 兼容接口查询模型列表，返回去重并排序后的模型 ID。
// 任何网络或解析异常都以 error 返回，绝不 panic。
func FetchModels(ctx context.Context, baseURL string, timeout time.Duration) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, describeConnError(endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return nil, fmt.Errorf("LM Studio 返回异常状态码 %s（端点 %s），响应片段: %s",
			resp.Status, endpoint, snippet)
	}

	var parsed modelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析模型列表 JSON 失败（接口可能不是 OpenAI 兼容格式）: %w", err)
	}

	ids := make([]string, 0, len(parsed.Data))
	seen := make(map[string]bool, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// describeConnError 把底层网络错误翻译成对用户友好的提示。
func describeConnError(endpoint string, err error) error {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf(
			"连接 LM Studio 超时（%s）：服务未响应，请确认 LM Studio 已启动并开启 Local Server。原始错误: %w",
			endpoint, err)
	}

	// Windows 下连接被拒通常表现为 "connectex: No connection could be made..."
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connectex") ||
		strings.Contains(msg, "no connection could be made") ||
		strings.Contains(msg, "target machine actively refused") {
		return fmt.Errorf(
			"无法连接 LM Studio（%s）：连接被拒绝。请确认 LM Studio 已启动、并已开启本地服务（默认端口 1234）。原始错误: %w",
			endpoint, err)
	}

	return fmt.Errorf("请求 LM Studio 失败（%s）: %w", endpoint, err)
}
