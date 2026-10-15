package bitget

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// AccountError contains only safe diagnostics, never upstream messages,
// request URLs, headers, credentials, or account data.
type AccountError struct {
	Code       string
	HTTPStatus int
	Kind       string
}

func (e *AccountError) Error() string {
	message := "请求失败，请检查账户类型和只读查询权限"
	switch e.Code {
	case "40001", "40006", "40037":
		message = "API Key 无效或不存在，请核对服务器上的 Key"
	case "40002", "40003", "40009":
		message = "签名校验失败，请核对 Secret 是否与 Key 配套，并使用 HMAC 系统生成密钥"
	case "40011", "40036":
		message = "Passphrase 口令缺失或错误，请核对创建 API 时设置的口令"
	case "40012":
		message = "API Key 或口令校验失败，请核对配套的凭证"
	case "40014", "40025", "40030":
		message = "查询权限不足，请核对该账户的资产与合约只读权限"
	case "40018", "40038":
		message = "服务器 IP 校验失败，请核对 Bitget 白名单中的实际出口公网 IP"
	case "40004", "40005", "40008", "40911":
		message = "请求时间戳无效或过期，请检查服务器时间同步"
	case "40007":
		message = "请求 Content-Type 无效，请更新后端请求头修复"
	case "40103":
		message = "Bitget 对当前地区的服务受限，请向 Bitget 核实账户和地区支持情况"
	case "429":
		message = "请求过于频繁，请稍后重试"
	default:
		switch e.Kind {
		case "timeout":
			message = "连接 Bitget 超时，请检查后端服务器网络"
		case "network":
			message = "后端无法连接 Bitget，请检查服务器网络、DNS 和 TLS 连接"
		case "response":
			message = "Bitget 返回了无法识别的数据，请稍后重试"
		case "http":
			message = "Bitget HTTP 请求未成功，请检查服务器到 Bitget 的连接"
		}
		if e.HTTPStatus == 429 {
			message = "请求过于频繁，请稍后重试"
		}
	}
	if safeAccountCode(e.Code) {
		message += fmt.Sprintf("（Bitget %s）", e.Code)
	}
	if e.HTTPStatus != 0 && e.HTTPStatus != 200 {
		message += fmt.Sprintf("（HTTP %d）", e.HTTPStatus)
	}
	return message
}

func safeAccountCode(code string) bool {
	if len(code) == 0 || len(code) > 8 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func accountConnectionError(err error) *AccountError {
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return &AccountError{Kind: "timeout"}
	}
	return &AccountError{Kind: "network"}
}

// Unknown errors must not escape from injected readers or transport wrappers.
func AccountErrorDescription(err error) string {
	var accountError *AccountError
	if errors.As(err, &accountError) {
		return accountError.Error()
	}
	return "请求失败，请检查账户类型和只读查询权限"
}
