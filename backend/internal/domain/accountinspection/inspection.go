// Package accountinspection 定义 Grok Build 账号巡检的持久化领域模型。
package accountinspection

import "time"

// State 表示巡检对账号当前可用性的结论。
type State string

const (
	// StateHealthy 表示最小真实对话请求成功。
	StateHealthy State = "healthy"
	// StateUnavailable 表示已确认当前账号无法用于请求。
	StateUnavailable State = "unavailable"
	// StateUncertain 表示临时限流、网络等因素导致需要人工复查。
	StateUncertain State = "uncertain"
	// StateSkipped 表示账号未发起上游探测。
	StateSkipped State = "skipped"
	// StateUninspected 表示账号尚未留下任何巡检快照。
	StateUninspected State = "uninspected"
)

// Classification 表示巡检结论的具体且稳定的原因分类。
type Classification string

const (
	// ClassificationHealthy 表示真实对话探测成功。
	ClassificationHealthy Classification = "healthy"
	// ClassificationDisabled 表示账号已在本地管理端禁用。
	ClassificationDisabled Classification = "disabled"
	// ClassificationReauthRequired 表示账号凭据需要重新授权。
	ClassificationReauthRequired Classification = "reauth_required"
	// ClassificationQuotaExhausted 表示上游明确返回额度耗尽。
	ClassificationQuotaExhausted Classification = "quota_exhausted"
	// ClassificationPermissionDenied 表示上游拒绝账号的对话权限。
	ClassificationPermissionDenied Classification = "permission_denied"
	// ClassificationProbeModelUnavailable 表示测试模型不可用。
	ClassificationProbeModelUnavailable Classification = "probe_model_unavailable"
	// ClassificationTemporaryRateLimited 表示未能确认额度耗尽的临时限流。
	ClassificationTemporaryRateLimited Classification = "temporary_rate_limited"
	// ClassificationProbeError 表示超时、网络或其他探测异常。
	ClassificationProbeError Classification = "probe_error"
	// ClassificationUninspected 表示尚未进行巡检。
	ClassificationUninspected Classification = "uninspected"
)

// Result 保存一个账号最新一次可安全展示的巡检结果。
type Result struct {
	// AccountID 是被巡检的 Build 账号标识。
	AccountID uint64
	// State 是当前可用性结论。
	State State
	// Classification 是结论的具体原因分类。
	Classification Classification
	// Reason 是不包含凭据和原始上游正文的展示原因。
	Reason string
	// HTTPStatus 是上游探测响应状态，未得到响应时为 0。
	HTTPStatus int
	// Model 是本次探测使用的固定模型标识。
	Model string
	// InspectedAt 是本条快照完成写入的 UTC 时间。
	InspectedAt time.Time
}
