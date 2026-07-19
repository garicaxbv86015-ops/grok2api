package accountinspection

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

// probePayload 返回 Grok Build 可处理的最小 Responses 探测请求及固定测试模型。
// 无参数；返回 JSON 请求体与对应上游模型标识。
func probePayload() ([]byte, string) {
	model := "grok-4.5"
	body, _ := json.Marshal(map[string]any{"model": model, "input": "ping", "stream": false})
	return body, model
}

// classifyResponse 根据真实上游 HTTP 响应做保守的可用性分类，并始终关闭响应正文。
// 参数 credential 为被探测账号，response 为上游响应；返回单账号巡检结论。
func classifyResponse(credential accountdomain.Credential, response *provider.Response) Item {
	if response == nil {
		return newItem(credential, StateUncertain, ClassificationProbeError, "上游未返回探测结果", 0)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return newItem(credential, StateHealthy, ClassificationHealthy, "最小对话探测成功", response.StatusCode)
	}
	code, message := readError(response.Body)
	return classifyHTTPResponse(credential, response.StatusCode, code, message)
}

// readError 提取上游失败正文中的稳定错误码和短消息，不传播原始响应内容。
// 参数 body 为上游响应正文；返回可用于分类的小写错误码和消息。
func readError(body io.Reader) (string, string) {
	data, _, err := provider.ReadDiagnosticBody(body)
	if err != nil || len(data) == 0 {
		return "", ""
	}
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   any    `json:"error"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return "", ""
	}
	code := payload.Code
	message := payload.Message
	switch value := payload.Error.(type) {
	case string:
		if message == "" {
			message = value
		}
	case map[string]any:
		if code == "" {
			code, _ = value["code"].(string)
		}
		if message == "" {
			message, _ = value["message"].(string)
		}
	}
	return strings.ToLower(strings.TrimSpace(code)), strings.ToLower(strings.TrimSpace(message))
}

// classifyHTTPResponse 将 Grok Build 的 HTTP 状态及上游错误文本映射为保守的账号巡检结论。
// 参数 credential 为账号，status 为 HTTP 状态，code/message 为已脱敏的错误信息；返回巡检结论。
func classifyHTTPResponse(credential accountdomain.Credential, status int, code, message string) Item {
	text := code + " " + message
	if status == http.StatusUnauthorized || containsAny(text, "invalid_grant", "token is expired", "token has been invalidated", "unauthorized", "authentication") {
		return newItem(credential, StateUnavailable, ClassificationReauthRequired, "上游拒绝账号认证", status)
	}
	if isQuotaExhausted(text) {
		return newItem(credential, StateUnavailable, ClassificationQuotaExhausted, "上游明确返回账号额度已用尽", status)
	}
	if status == http.StatusTooManyRequests {
		return newItem(credential, StateUncertain, ClassificationTemporaryRateLimited, "上游临时限流，未判定账号失效", status)
	}
	if status == http.StatusPaymentRequired || status == http.StatusForbidden || containsAny(text, "permission-denied", "permission denied", "chat endpoint is denied", "deactivated", "suspended", "banned") {
		return newItem(credential, StateUnavailable, ClassificationPermissionDenied, "上游拒绝账号的对话权限", status)
	}
	if status == http.StatusNotFound || containsAny(text, "model not found", "model_not_found", "does not exist") {
		return newItem(credential, StateUncertain, ClassificationProbeModelUnavailable, "测试模型不可用，未判定账号失效", status)
	}
	return newItem(credential, StateUncertain, ClassificationProbeError, "上游探测未得到可确认的结论", status)
}

// classifyError 将本地凭据刷新、网络和超时错误保守映射为巡检结论。
// 参数 credential 为账号，err 为探测过程中产生的错误；返回巡检结论。
func classifyError(credential accountdomain.Credential, err error) Item {
	if errors.Is(err, provider.ErrUnauthorized) || isAuthenticationText(err.Error()) {
		return newItem(credential, StateUnavailable, ClassificationReauthRequired, "账号凭据已失效，需要重新认证", 0)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newItem(credential, StateUncertain, ClassificationProbeError, "探测超时，未判定账号失效", 0)
	}
	return newItem(credential, StateUncertain, ClassificationProbeError, "探测请求失败，未判定账号失效", 0)
}

// isAuthenticationText 判断上游或刷新错误是否明确指向认证失效。
// 参数 value 为错误文本；返回是否可确定为认证问题。
func isAuthenticationText(value string) bool {
	return containsAny(value, "invalid_grant", "token is expired", "token has been invalidated", "unauthorized", "oauth refresh token", "重新认证", "永久失效")
}

// isQuotaExhausted 仅匹配带有额度耗尽语义的错误，避免把通用 429 误判为账号不可用。
// 参数 value 为小写或任意大小写错误文本；返回是否可确认额度耗尽。
func isQuotaExhausted(value string) bool {
	return containsAny(value, "free-usage-exhausted", "usage exhausted", "quota exhausted", "quota_exhausted", "included free usage", "credits exhausted", "额度已用尽", "额度耗尽")
}

// containsAny 判断文本是否包含任一非空关键词，比较时忽略大小写和首尾空白。
// 参数 value 为待检查文本，needles 为关键词；返回是否命中。
func containsAny(value string, needles ...string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// newItem 生成不包含凭据或上游正文的账号巡检结果。
// 参数 credential 为账号标识信息，state/classification/reason/status 为巡检结论；返回结果项。
func newItem(credential accountdomain.Credential, state State, classification Classification, reason string, status int) Item {
	return Item{
		AccountID: credential.ID, Name: credential.Name, Provider: string(credential.Provider), Enabled: credential.Enabled, AuthStatus: credential.AuthStatus,
		State: state, Classification: classification, Reason: reason, HTTPStatus: status, Model: "grok-4.5",
		Suggestion: suggestionFor(state, classification),
	}
}

// suggestionFor 根据巡检结论生成必须由管理员手动确认的账号管理建议。
// 参数 state 为巡检状态，classification 为具体分类；返回建议操作。
func suggestionFor(state State, classification Classification) Suggestion {
	if state == StateHealthy {
		return SuggestionKeep
	}
	if classification == ClassificationQuotaExhausted || classification == ClassificationPermissionDenied {
		return SuggestionDisable
	}
	if classification == ClassificationReauthRequired {
		return SuggestionReauth
	}
	if state == StateUncertain {
		return SuggestionReview
	}
	return SuggestionNone
}
