package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
)

var reasoningDecodeFailureMarkers = [][]byte{
	[]byte("could not decode the compaction blob"),
	[]byte("could not decrypt the provided encrypted_content"),
}

// recoverReasoningDecodeFailure 处理上游对不透明密文/compaction 状态的预生成拒绝。
//
// 重试策略（同账号、同 base URL）：
//  1. 剥离 input 中的 reasoning/compaction 等不透明密文后重试；
//  2. 若仍失败（或 body 中本就没有可剥离密文），再清空续接字段并轮换会话后重试。
//
// 跨账号复用同一 session/prompt_cache_key 时，上游可能持有不可解的 compaction 状态；
// 仅剥离 body 密文不够，必须轮换会话身份。
// 参数 ctx 为请求上下文，request 为原始资源请求，accessToken 为当前账号令牌，body 为已规范化请求体，
// base 为本次上游地址，response 为首次响应，requestURL 为首次请求地址；返回值依次为最终响应、请求地址和是否完成降级恢复。
func (a *Adapter) recoverReasoningDecodeFailure(
	ctx context.Context,
	request provider.ResponseResourceRequest,
	accessToken string,
	body []byte,
	base string,
	response *http.Response,
	requestURL string,
) (*http.Response, string, bool) {
	if response == nil || response.StatusCode != http.StatusBadRequest {
		return response, requestURL, false
	}
	errorBody, truncated, err := provider.ReadDiagnosticBody(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return cloneBufferedResponse(response, errorBody, truncated), requestURL, false
	}
	original := cloneBufferedResponse(response, errorBody, truncated)
	if truncated || !isReasoningDecodeFailure(errorBody) {
		return original, requestURL, false
	}

	type recoveryAttempt struct {
		body          []byte
		rotateSession bool
	}
	var attempts []recoveryAttempt
	downgraded, stripped := stripOpaqueEncryptedState(body)
	if stripped {
		attempts = append(attempts, recoveryAttempt{body: downgraded, rotateSession: false})
		// 密文剥离后仍可能踩中跨账号会话状态，再试一轮新会话请求。
		attempts = append(attempts, recoveryAttempt{body: clearContinuationStateFields(downgraded), rotateSession: true})
	} else {
		// body 无密文却报 compaction blob：多半是 session/prompt_cache 侧残留状态。
		attempts = append(attempts, recoveryAttempt{body: clearContinuationStateFields(body), rotateSession: true})
	}

	for _, attempt := range attempts {
		retryRequest := request
		retryRequest.IdempotencyID, _ = security.NewOpaqueToken(18)
		if attempt.rotateSession {
			// 显式生成一次性会话键，避免改变普通空键请求保持无状态的协议语义。
			freshSessionKey, sessionErr := security.NewOpaqueToken(18)
			if sessionErr != nil {
				return original, requestURL, false
			}
			retryRequest.PromptCacheKey = freshSessionKey
		}
		retry, retryURL, retryErr := a.doResponseRequest(ctx, retryRequest, accessToken, attempt.body, base)
		if retryErr != nil {
			return original, requestURL, false
		}
		if err := normalizeGzipResponse(retry); err != nil {
			_ = retry.Body.Close()
			return original, requestURL, false
		}
		if isHTTPSuccess(retry.StatusCode) {
			_ = original.Body.Close()
			// 成功降级后清理服务端回放缓存，避免下一轮再次注入不可解密文。
			if a.replay != nil && strings.TrimSpace(request.PromptCacheKey) != "" {
				a.replay.Clear(ctx, request.Model, request.PromptCacheKey)
			}
			return retry, retryURL, true
		}
		// 仅当“剥离密文但保留会话”的第一次重试仍是同类解码错误时，
		// 才继续轮换会话。其他失败不能证明与会话状态有关，应保留原始 400。
		if attempt.rotateSession {
			_ = retry.Body.Close()
			return original, requestURL, false
		}
		if !responseHasReasoningDecodeFailure(retry) {
			return original, requestURL, false
		}
	}
	return original, requestURL, false
}

// responseHasReasoningDecodeFailure 判断重试响应是否仍为完整、明确的不透明状态解码错误，并负责关闭响应体。
// 参数 response 为待检查的上游响应；返回值表示是否允许继续执行新会话降级重试。
func responseHasReasoningDecodeFailure(response *http.Response) bool {
	if response == nil || response.StatusCode != http.StatusBadRequest {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return false
	}
	errorBody, truncated, err := provider.ReadDiagnosticBody(response.Body)
	_ = response.Body.Close()
	return err == nil && !truncated && isReasoningDecodeFailure(errorBody)
}

// isReasoningDecodeFailure 判断错误正文是否包含上游已知的不透明状态解码失败标记。
// 参数 body 为上游错误正文；返回值表示该错误是否可进入安全降级流程。
func isReasoningDecodeFailure(body []byte) bool {
	lower := bytes.ToLower(body)
	for _, marker := range reasoningDecodeFailureMarkers {
		if bytes.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// stripReasoningEncryptedContent 兼容旧调用并委托给统一的不透明状态剥离逻辑。
// 参数 body 为 Responses 请求体；返回值依次为降级请求体以及内容是否发生变化。
func stripReasoningEncryptedContent(body []byte) ([]byte, bool) {
	return stripOpaqueEncryptedState(body)
}

// stripOpaqueEncryptedState 剥离 input 中会触发上游密文校验的不透明状态：
//   - type=reasoning 的 encrypted_content（保留可读 summary/content）
//   - type=compaction/compaction_summary 整项（外来 blob 无法被 Grok 解密）
//   - 其他 input item 顶层 encrypted_content
//
// 参数 body 为 Responses 请求体；返回值依次为降级请求体以及内容是否发生变化。
func stripOpaqueEncryptedState(body []byte) ([]byte, bool) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body, false
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) == 0 {
		return body, false
	}
	changed := false
	rebuilt := make([]any, 0, len(input))
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			rebuilt = append(rebuilt, raw)
			continue
		}
		itemType := stringField(item, "type")
		// 外来 compaction blob 原样回放会稳定 400；直接丢弃该项。
		if isGatewayCompactionItemType(itemType) {
			changed = true
			continue
		}
		encrypted, hasEncrypted := item["encrypted_content"].(string)
		if !hasEncrypted || strings.TrimSpace(encrypted) == "" {
			// 兼容 null 或非字符串密文：只要键存在就清掉。
			if _, exists := item["encrypted_content"]; exists {
				cleaned := cloneJSONObject(item)
				delete(cleaned, "encrypted_content")
				delete(cleaned, "id")
				delete(cleaned, "status")
				changed = true
				if itemType == "reasoning" && !hasReadableReasoningContent(cleaned) {
					continue
				}
				rebuilt = append(rebuilt, cleaned)
				continue
			}
			rebuilt = append(rebuilt, raw)
			continue
		}
		cleaned := cloneJSONObject(item)
		delete(cleaned, "encrypted_content")
		delete(cleaned, "id")
		delete(cleaned, "status")
		changed = true
		if itemType == "reasoning" && !hasReadableReasoningContent(cleaned) {
			continue
		}
		rebuilt = append(rebuilt, cleaned)
	}
	if !changed {
		return body, false
	}
	payload["input"] = rebuilt
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, false
	}
	return encoded, true
}

// clearContinuationStateFields 删除 prompt_cache_key 和 previous_response_id，
// 使新会话降级请求不再续接旧服务端状态。
// 参数 body 为 Responses 请求体；返回值为删除字段后的请求体，解析失败时返回原内容。
func clearContinuationStateFields(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	_, hasPromptCacheKey := payload["prompt_cache_key"]
	_, hasPreviousResponseID := payload["previous_response_id"]
	if !hasPromptCacheKey && !hasPreviousResponseID {
		return body
	}
	delete(payload, "prompt_cache_key")
	delete(payload, "previous_response_id")
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return encoded
}

// hasReadableReasoningContent 判断 reasoning 项是否仍包含可安全回放的可读摘要或正文。
// 参数 item 为 reasoning 输入项；返回值表示剥离密文后是否应保留该项。
func hasReadableReasoningContent(item map[string]any) bool {
	for _, field := range []string{"summary", "content"} {
		parts, _ := item[field].([]any)
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			if strings.TrimSpace(stringField(part, "text")) != "" {
				return true
			}
		}
	}
	return false
}

// appendCompatibilityWarning 向响应头追加去重后的兼容性降级标记。
// 参数 header 为响应头，warning 为待追加标记；该函数无返回值。
func appendCompatibilityWarning(header http.Header, warning string) {
	if header == nil || strings.TrimSpace(warning) == "" {
		return
	}
	existing := strings.TrimSpace(header.Get("X-Grok2API-Compatibility-Warnings"))
	if existing == "" {
		header.Set("X-Grok2API-Compatibility-Warnings", warning)
		return
	}
	for _, value := range strings.Split(existing, ",") {
		if strings.TrimSpace(value) == warning {
			return
		}
	}
	header.Set("X-Grok2API-Compatibility-Warnings", existing+","+warning)
}
