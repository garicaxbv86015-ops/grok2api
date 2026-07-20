package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/provider/conversation"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
)

// TestStripReasoningEncryptedContentPreservesOnlyPortableHistory 验证降级请求只保留可移植的历史内容。
func TestStripReasoningEncryptedContentPreservesOnlyPortableHistory(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"reasoning","id":"rs_empty","status":"completed","summary":[],"encrypted_content":"opaque-empty"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":""}],"encrypted_content":"opaque-blank"},
			{"type":"reasoning","id":"rs_summary","status":"completed","summary":[{"type":"summary_text","text":"readable"}],"encrypted_content":"opaque-summary"},
			{"type":"compaction","encrypted_content":"foreign-blob"},
			{"type":"message","role":"assistant","content":"answer","encrypted_content":"message-value"},
			{"type":"message","role":"user","content":"continue"}
		]
	}`)
	downgraded, changed := stripReasoningEncryptedContent(body)
	if !changed {
		t.Fatal("expected encrypted reasoning downgrade")
	}
	var payload struct {
		Input []map[string]any `json:"input"`
	}
	if json.Unmarshal(downgraded, &payload) != nil || len(payload.Input) != 3 {
		t.Fatalf("downgraded = %s", downgraded)
	}
	reasoning := payload.Input[0]
	if reasoning["type"] != "reasoning" || reasoning["id"] != nil || reasoning["status"] != nil || reasoning["encrypted_content"] != nil {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	if payload.Input[1]["role"] != "assistant" || payload.Input[1]["encrypted_content"] != nil {
		t.Fatalf("assistant message = %#v", payload.Input[1])
	}
	if payload.Input[2]["role"] != "user" {
		t.Fatalf("user message = %#v", payload.Input[2])
	}
}

// TestRecoverCompactionBlobWithoutBodyEncryptedStateRotatesSession 验证无显式密文时通过轮换会话恢复 compaction 错误。
func TestRecoverCompactionBlobWithoutBodyEncryptedStateRotatesSession(t *testing.T) {
	adapter, encrypted := newReasoningRecoveryTestAdapter(t)
	var calls atomic.Int32
	var sawFreshSession bool
	adapter.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		data, _ := io.ReadAll(request.Body)
		if call == 1 {
			if !strings.Contains(string(data), `"prompt_cache_key"`) {
				t.Fatalf("first body missing prompt_cache_key: %s", data)
			}
			return jsonHTTPResponse(request, http.StatusBadRequest, `{"code":"invalid-argument","error":"Could not decode the compaction blob. Ensure it is unmodified from the compact response."}`), nil
		}
		// body 无 encrypted 时仍应轮换会话：不带 prompt_cache_key，并换新 session 头。
		if strings.Contains(string(data), `"prompt_cache_key"`) {
			t.Fatalf("retry still carries prompt_cache_key: %s", data)
		}
		if request.Header.Get("x-grok-session-id") == "" {
			t.Fatal("retry missing session id")
		}
		sawFreshSession = true
		return jsonHTTPResponse(request, http.StatusOK, `{"id":"resp_ok","status":"completed","output":[]}`), nil
	})
	response, err := adapter.ForwardResponse(t.Context(), provider.ResponseResourceRequest{
		Credential:    account.Credential{ID: 1, Provider: account.ProviderBuild, EncryptedAccessToken: encrypted},
		Method:        http.MethodPost,
		Path:          "/responses",
		Model:         "grok-4.5",
		PromptCacheKey: "11111111-1111-4111-8111-111111111111",
		Body:          []byte(`{"model":"grok-4.5","prompt_cache_key":"11111111-1111-4111-8111-111111111111","input":[{"type":"message","role":"user","content":"hello again"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls.Load() != 2 || response.StatusCode != http.StatusOK || !sawFreshSession {
		t.Fatalf("calls=%d status=%d freshSession=%v headers=%#v", calls.Load(), response.StatusCode, sawFreshSession, response.Header)
	}
	if !strings.Contains(response.Header.Get("X-Grok2API-Compatibility-Warnings"), "reasoning_encrypted_content_downgraded") {
		t.Fatalf("warnings = %q", response.Header.Get("X-Grok2API-Compatibility-Warnings"))
	}
}

// TestRecoverReasoningDecodeFailureRetriesSameUpstreamOnce 验证密文降级固定在同一账号与上游地址完成。
func TestRecoverReasoningDecodeFailureRetriesSameUpstreamOnce(t *testing.T) {
	adapter, encrypted := newReasoningRecoveryTestAdapter(t)
	var calls atomic.Int32
	adapter.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if request.URL.String() != "https://build.test/v1/responses" || request.Header.Get("Authorization") != "Bearer access-token" {
			t.Fatalf("request = %s headers=%#v", request.URL, request.Header)
		}
		if call == 1 {
			if request.Header.Get("Idempotency-Key") != "original-id" {
				t.Fatalf("first idempotency key = %q", request.Header.Get("Idempotency-Key"))
			}
			if !strings.Contains(string(data), `"encrypted_content":"opaque"`) || !strings.Contains(string(data), `"summary":[]`) {
				t.Fatalf("first body = %s", data)
			}
			return jsonHTTPResponse(request, http.StatusBadRequest, `{"error":"Could not decrypt the provided encrypted_content. Ensure the value is unmodified."}`), nil
		}
		if request.Header.Get("Idempotency-Key") == "" || request.Header.Get("Idempotency-Key") == "original-id" {
			t.Fatalf("retry idempotency key = %q", request.Header.Get("Idempotency-Key"))
		}
		var retryPayload struct {
			Input []map[string]any `json:"input"`
		}
		if json.Unmarshal(data, &retryPayload) != nil {
			t.Fatalf("retry body = %s", data)
		}
		for _, item := range retryPayload.Input {
			if item["type"] == "reasoning" || item["encrypted_content"] != nil {
				t.Fatalf("retry input = %#v", retryPayload.Input)
			}
		}
		return jsonHTTPResponse(request, http.StatusOK, `{"id":"resp_ok","status":"completed","output":[]}`), nil
	})

	response, err := adapter.ForwardResponse(t.Context(), provider.ResponseResourceRequest{
		Credential: account.Credential{ID: 1, Provider: account.ProviderBuild, EncryptedAccessToken: encrypted},
		Method:     http.MethodPost, Path: "/responses", Model: "grok-4.5", PromptCacheKey: "session-1",
		IdempotencyID: "original-id",
		Body:          []byte(`{"model":"public","max_tokens":1024,"thinking":{"type":"enabled","budget_tokens":512},"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque"}]},{"role":"user","content":"continue"}]}`),
		NormalizeBody: true, Operation: conversation.OperationMessages,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls.Load() != 2 || response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("X-Grok2API-Compatibility-Warnings"), "reasoning_encrypted_content_downgraded") {
		t.Fatalf("calls=%d status=%d headers=%#v", calls.Load(), response.StatusCode, response.Header)
	}
	data, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(data), `"type":"message"`) {
		t.Fatalf("converted response = %s", data)
	}
}

// TestRecoverReasoningDecodeFailureDoesNotRetryOtherBadRequests 验证无关的 400 错误不会触发降级重试。
func TestRecoverReasoningDecodeFailureDoesNotRetryOtherBadRequests(t *testing.T) {
	adapter, encrypted := newReasoningRecoveryTestAdapter(t)
	var calls atomic.Int32
	adapter.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonHTTPResponse(request, http.StatusBadRequest, `{"error":{"message":"unrelated invalid request"}}`), nil
	})
	response, err := adapter.ForwardResponse(t.Context(), provider.ResponseResourceRequest{
		Credential: account.Credential{ID: 1, Provider: account.ProviderBuild, EncryptedAccessToken: encrypted},
		Method:     http.MethodPost, Path: "/responses", Model: "grok-4.5",
		Body: []byte(`{"model":"grok-4.5","input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls.Load() != 1 || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("calls=%d status=%d", calls.Load(), response.StatusCode)
	}
}

// TestRecoverReasoningDecodeFailureStaysOnXAIFallbackPlane 验证恢复请求不会从 XAI 回退平面跳回 Build 主平面。
func TestRecoverReasoningDecodeFailureStaysOnXAIFallbackPlane(t *testing.T) {
	adapter, encrypted := newReasoningRecoveryTestAdapter(t)
	adapter.SetFallbackMarker(reasoningRecoveryFallbackMarker{})
	var calls atomic.Int32
	adapter.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch call := calls.Add(1); call {
		case 1:
			if request.URL.Host != "build.test" {
				t.Fatalf("primary host = %q", request.URL.Host)
			}
			return jsonHTTPResponse(request, http.StatusForbidden, `{"error":"build denied"}`), nil
		case 2:
			if request.URL.Host != "xai.test" {
				t.Fatalf("fallback host = %q", request.URL.Host)
			}
			return jsonHTTPResponse(request, http.StatusBadRequest, `{"error":"Could not decode the compaction blob. Ensure it is unmodified from the compact response."}`), nil
		case 3:
			data, _ := io.ReadAll(request.Body)
			if request.URL.Host != "xai.test" || strings.Contains(string(data), `"type":"reasoning"`) {
				t.Fatalf("recovery host=%q body=%s", request.URL.Host, data)
			}
			return jsonHTTPResponse(request, http.StatusOK, `{"id":"resp_ok","status":"completed","output":[]}`), nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})
	response, err := adapter.ForwardResponse(t.Context(), provider.ResponseResourceRequest{
		Credential: account.Credential{
			ID: 1, Provider: account.ProviderBuild, EncryptedAccessToken: encrypted,
			BuildRouteMode: account.BuildRouteAuto, BuildSuperEntitled: true,
		},
		Method: http.MethodPost, Path: "/responses", Model: "grok-4.5",
		Body: []byte(`{"model":"grok-4.5","input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque"},{"role":"user","content":"continue"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls.Load() != 3 || response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("X-Grok2API-Compatibility-Warnings"), "reasoning_encrypted_content_downgraded") {
		t.Fatalf("calls=%d status=%d headers=%#v", calls.Load(), response.StatusCode, response.Header)
	}
}

// TestRecoverReasoningDecodeFailurePreservesOriginalWhenRetryFails 验证全部安全降级失败后仍返回最初的解码错误。
func TestRecoverReasoningDecodeFailurePreservesOriginalWhenRetryFails(t *testing.T) {
	adapter, encrypted := newReasoningRecoveryTestAdapter(t)
	var calls atomic.Int32
	adapter.http.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) <= 2 {
			return jsonHTTPResponse(request, http.StatusBadRequest, `{"error":"Could not decode the compaction blob. Ensure it is unmodified from the compact response."}`), nil
		}
		// 第一次密文剥离仍返回相同解码错误后，才允许继续执行会话轮换重试。
		return jsonHTTPResponse(request, http.StatusServiceUnavailable, `{"error":"temporary failure"}`), nil
	})
	response, err := adapter.ForwardResponse(t.Context(), provider.ResponseResourceRequest{
		Credential: account.Credential{ID: 1, Provider: account.ProviderBuild, EncryptedAccessToken: encrypted},
		Method:     http.MethodPost, Path: "/responses", Model: "grok-4.5",
		Body: []byte(`{"model":"grok-4.5","input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	// 1 次原始失败 + 剥离重试 + 清会话重试
	if calls.Load() != 3 || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(data), "Could not decode") || response.Header.Get("X-Grok2API-Compatibility-Warnings") != "" {
		t.Fatalf("calls=%d status=%d headers=%#v body=%s", calls.Load(), response.StatusCode, response.Header, data)
	}
}

// newReasoningRecoveryTestAdapter 创建带可解密测试凭据的 CLI Adapter，并返回 Adapter 与加密令牌。
func newReasoningRecoveryTestAdapter(t *testing.T) (*Adapter, string) {
	t.Helper()
	cipher, err := security.NewCipher(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt("access-token")
	if err != nil {
		t.Fatal(err)
	}
	return NewAdapter(Config{
		BaseURL: "https://build.test/v1", FallbackBaseURL: "https://xai.test/v1",
		ClientVersion: "0.2.106", ClientIdentifier: "grok-shell", TokenAuth: "xai-grok-cli",
		UserAgent: "grok-shell/0.2.106 (linux; x86_64)",
	}, cipher), encrypted
}

// jsonHTTPResponse 构造绑定原请求的 JSON HTTP 响应；参数依次为请求、状态码和正文，返回模拟响应。
func jsonHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Request: request,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
}

type reasoningRecoveryFallbackMarker struct{}

// MarkBuildAPIFallback 模拟记录 Build API 回退状态；参数为上下文、账号 ID 与启用标记，无返回数据。
func (reasoningRecoveryFallbackMarker) MarkBuildAPIFallback(context.Context, uint64, bool) error {
	return nil
}
