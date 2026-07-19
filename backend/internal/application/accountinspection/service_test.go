package accountinspection

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	inspectiondomain "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

// inspectionStoreStub 为巡检服务测试提供最小账号读取实现。
type inspectionStoreStub struct {
	values map[uint64]accountdomain.Credential
}

// Get 按账号编号返回测试凭据。
// 参数 _ 为调用上下文，id 为账号标识；返回凭据或未找到错误。
func (s *inspectionStoreStub) Get(_ context.Context, id uint64) (accountdomain.Credential, error) {
	value, ok := s.values[id]
	if !ok {
		return accountdomain.Credential{}, fmt.Errorf("account %d not found", id)
	}
	return value, nil
}

// ListProviderAccountBatch 返回指定渠道的测试账号集合。
// 参数 _ 为调用上下文，providerValue 为渠道，afterID/limit 为游标参数；返回账号、总数和错误。
func (s *inspectionStoreStub) ListProviderAccountBatch(_ context.Context, providerValue accountdomain.Provider, afterID uint64, limit int) ([]accountdomain.Credential, int64, error) {
	if afterID != 0 || limit < 1 {
		return nil, 0, nil
	}
	values := make([]accountdomain.Credential, 0)
	for _, value := range s.values {
		if value.Provider == providerValue {
			values = append(values, value)
		}
	}
	return values, int64(len(values)), nil
}

// inspectionCredentialStub 模拟凭据刷新后的当前凭据。
type inspectionCredentialStub struct{}

// EnsureCredential 直接返回输入凭据，避免测试依赖 OAuth 刷新。
// 参数 _ 为调用上下文，value 为原始凭据，_ 为是否强制刷新；返回原始凭据和空错误。
func (inspectionCredentialStub) EnsureCredential(_ context.Context, value accountdomain.Credential, _ bool) (accountdomain.Credential, error) {
	return value, nil
}

// inspectionResultStoreStub 为巡检服务测试保存和读取最新快照。
type inspectionResultStoreStub struct {
	values map[uint64]inspectiondomain.Result
}

// Upsert 写入测试快照。
// 参数 _ 为调用上下文，values 为待保存快照；返回空错误。
func (s *inspectionResultStoreStub) Upsert(_ context.Context, values []inspectiondomain.Result) error {
	if s.values == nil {
		s.values = make(map[uint64]inspectiondomain.Result)
	}
	for _, value := range values {
		s.values[value.AccountID] = value
	}
	return nil
}

// ListBuild 返回全部测试快照。
// 参数 _ 为调用上下文；返回快照和空错误。
func (s *inspectionResultStoreStub) ListBuild(_ context.Context) ([]inspectiondomain.Result, error) {
	values := make([]inspectiondomain.Result, 0, len(s.values))
	for _, value := range s.values {
		values = append(values, value)
	}
	return values, nil
}

// ListUninspectedBuildAccountIDs 返回测试中尚未保存快照的账号编号。
// 参数 _ 为调用上下文；返回空编号列表和空错误。
func (s *inspectionResultStoreStub) ListUninspectedBuildAccountIDs(_ context.Context) ([]uint64, error) {
	return []uint64{}, nil
}

// inspectionResponseAdapterStub 按预设响应模拟 Provider 对话适配器。
type inspectionResponseAdapterStub struct {
	providerValue accountdomain.Provider
	response      *provider.Response
}

// Provider 返回测试适配器所属渠道。
// 无参数；返回 Provider 标识。
func (s inspectionResponseAdapterStub) Provider() accountdomain.Provider {
	return s.providerValue
}

// ForwardResponse 返回预设的上游探测响应。
// 参数 _ 为调用上下文，_ 为探测请求；返回预设响应和空错误。
func (s inspectionResponseAdapterStub) ForwardResponse(_ context.Context, _ provider.ResponseResourceRequest) (*provider.Response, error) {
	return cloneInspectionResponse(s.response), nil
}

// TestInspectClassifiesDefinitiveAndUncertainOutcomes 验证巡检不会将临时失败误报为不可用账号。
// 参数 t 为 Go 测试上下文；无返回值。
func TestInspectClassifiesDefinitiveAndUncertainOutcomes(t *testing.T) {
	tests := []struct {
		name           string
		providerValue  accountdomain.Provider
		status         int
		body           string
		authStatus     accountdomain.AuthStatus
		wantState      State
		wantClassify   Classification
	}{
		{name: "healthy", providerValue: accountdomain.ProviderBuild, status: http.StatusOK, wantState: StateHealthy, wantClassify: ClassificationHealthy},
		{name: "expired credential", providerValue: accountdomain.ProviderBuild, authStatus: accountdomain.AuthStatusReauthRequired, wantState: StateUnavailable, wantClassify: ClassificationReauthRequired},
		{name: "free quota exhausted", providerValue: accountdomain.ProviderBuild, status: http.StatusTooManyRequests, body: `{"code":"subscription:free-usage-exhausted"}`, wantState: StateUnavailable, wantClassify: ClassificationQuotaExhausted},
		{name: "generic rate limit", providerValue: accountdomain.ProviderBuild, status: http.StatusTooManyRequests, wantState: StateUncertain, wantClassify: ClassificationTemporaryRateLimited},
		{name: "build permission denied", providerValue: accountdomain.ProviderBuild, status: http.StatusForbidden, wantState: StateUnavailable, wantClassify: ClassificationPermissionDenied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential := accountdomain.Credential{ID: 7, Name: "inspection-account", Provider: test.providerValue, Enabled: true, AuthStatus: test.authStatus}
			store := &inspectionStoreStub{values: map[uint64]accountdomain.Credential{credential.ID: credential}}
			adapter := inspectionResponseAdapterStub{providerValue: test.providerValue, response: newInspectionResponse(test.status, test.body)}
			service := NewService(store, &inspectionResultStoreStub{}, inspectionCredentialStub{}, provider.NewRegistry(adapter))

			report, err := service.Inspect(t.Context(), Input{Provider: test.providerValue, AccountIDs: []uint64{credential.ID}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Results) != 1 {
				t.Fatalf("results = %#v", report.Results)
			}
			result := report.Results[0]
			if result.State != test.wantState || result.Classification != test.wantClassify {
				t.Fatalf("result = %#v, want state=%s classification=%s", result, test.wantState, test.wantClassify)
			}
		})
	}
}

// TestInspectSkipsDisabledAccounts 验证默认巡检不会为管理端禁用账号发送额外上游请求。
// 参数 t 为 Go 测试上下文；无返回值。
func TestInspectSkipsDisabledAccounts(t *testing.T) {
	credential := accountdomain.Credential{ID: 9, Name: "disabled-account", Provider: accountdomain.ProviderBuild, Enabled: false, AuthStatus: accountdomain.AuthStatusActive}
	store := &inspectionStoreStub{values: map[uint64]accountdomain.Credential{credential.ID: credential}}
	service := NewService(store, &inspectionResultStoreStub{}, inspectionCredentialStub{}, provider.NewRegistry(inspectionResponseAdapterStub{
		providerValue: accountdomain.ProviderBuild, response: newInspectionResponse(http.StatusOK, ""),
	}))

	report, err := service.Inspect(t.Context(), Input{Provider: accountdomain.ProviderBuild, AccountIDs: []uint64{credential.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Skipped != 1 || report.Results[0].Classification != ClassificationDisabled {
		t.Fatalf("report = %#v", report)
	}
}

// TestInspectPersistsLatestResult 验证巡检完成后会将结果保存为账号最新快照。
// 参数 t 为 Go 测试上下文；无返回值。
func TestInspectPersistsLatestResult(t *testing.T) {
	credential := accountdomain.Credential{ID: 11, Name: "persisted-account", Provider: accountdomain.ProviderBuild, Enabled: true, AuthStatus: accountdomain.AuthStatusActive}
	store := &inspectionStoreStub{values: map[uint64]accountdomain.Credential{credential.ID: credential}}
	results := &inspectionResultStoreStub{}
	service := NewService(store, results, inspectionCredentialStub{}, provider.NewRegistry(inspectionResponseAdapterStub{
		providerValue: accountdomain.ProviderBuild, response: newInspectionResponse(http.StatusOK, ""),
	}))

	report, err := service.Inspect(t.Context(), Input{Provider: accountdomain.ProviderBuild, AccountIDs: []uint64{credential.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	persisted, ok := results.values[credential.ID]
	if !ok || persisted.State != StateHealthy || persisted.InspectedAt.IsZero() || report.Results[0].InspectedAt == nil {
		t.Fatalf("persisted = %#v, report = %#v", persisted, report)
	}
}

// TestGetOverviewUsesPersistedSnapshot 验证工作台读取保存的巡检快照而非重新发送上游请求。
// 参数 t 为 Go 测试上下文；无返回值。
func TestGetOverviewUsesPersistedSnapshot(t *testing.T) {
	credential := accountdomain.Credential{ID: 12, Name: "overview-account", Provider: accountdomain.ProviderBuild, Enabled: true, AuthStatus: accountdomain.AuthStatusActive}
	store := &inspectionStoreStub{values: map[uint64]accountdomain.Credential{credential.ID: credential}}
	results := &inspectionResultStoreStub{values: map[uint64]inspectiondomain.Result{
		credential.ID: {AccountID: credential.ID, State: inspectiondomain.StateUnavailable, Classification: inspectiondomain.ClassificationQuotaExhausted, Reason: "额度已用尽", HTTPStatus: http.StatusTooManyRequests, Model: "grok-4.5", InspectedAt: time.Now().UTC()},
	}}
	service := NewService(store, results, inspectionCredentialStub{}, provider.NewRegistry())

	overview, err := service.GetOverview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if overview.Total != 1 || overview.QuotaExhausted != 1 || overview.Results[0].Suggestion != SuggestionDisable {
		t.Fatalf("overview = %#v", overview)
	}
}

// newInspectionResponse 创建可由巡检服务安全读取和关闭的测试上游响应。
// 参数 status 为 HTTP 状态，body 为响应正文；返回 Provider 响应。
func newInspectionResponse(status int, body string) *provider.Response {
	return &provider.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(body))}
}

// cloneInspectionResponse 为每个测试请求提供独立正文，避免重复关闭同一 Reader。
// 参数 value 为预设响应；返回可独立消费的响应副本。
func cloneInspectionResponse(value *provider.Response) *provider.Response {
	if value == nil {
		return nil
	}
	body, _, _ := provider.ReadDiagnosticBody(value.Body)
	return &provider.Response{StatusCode: value.StatusCode, Body: io.NopCloser(bytes.NewReader(body))}
}
