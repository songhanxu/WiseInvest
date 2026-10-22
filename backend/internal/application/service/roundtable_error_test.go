package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestRoundtableErrorsAreActionableWithoutProviderSecrets(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("wrapped: %w", &openai.APIError{HTTPStatusCode: 401, Message: "secret-provider-body"}), "认证"},
		{&openai.APIError{HTTPStatusCode: 429}, "额度"},
		{context.DeadlineExceeded, "超时"},
		{fmt.Errorf("model returned an empty answer"), "正文"},
		{fmt.Errorf("tool failed: secret-provider-body"), "工具调用失败"},
	} {
		got := roundtableErrorMessage(test.err)
		if !strings.Contains(got, test.want) || strings.Contains(got, "secret-provider-body") {
			t.Errorf("got %q, want %q", got, test.want)
		}
	}
}
