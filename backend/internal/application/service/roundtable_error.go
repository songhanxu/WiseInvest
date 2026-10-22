package service

import (
	"context"
	"errors"
	"net"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// Keep provider response bodies and credentials out of client-facing errors.
func roundtableErrorMessage(err error) string {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 401, 403:
			return "AI 服务认证或模型权限不足，请检查后端模型配置。"
		case 429:
			return "AI 服务限流或额度不足，请稍后重试。"
		case 400, 404:
			return "AI 服务不支持当前模型或请求参数，请检查后端模型配置。"
		default:
			return "AI 服务暂时不可用，请稍后重试。"
		}
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return "分析超时，请缩小问题范围后重试。"
	}
	if strings.Contains(err.Error(), "empty answer") {
		return "模型未返回正文，请重试或检查后端模型配置。"
	}
	return "模型或工具调用失败，请重试；持续失败时请检查后端 AI 服务与日志。"
}
