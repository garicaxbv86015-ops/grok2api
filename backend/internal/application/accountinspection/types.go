package accountinspection

import (
	"context"
	"errors"
	"time"

	inspectiondomain "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

// ErrInvalidInput 表示巡检请求的 Provider、模式、账号编号或并发参数无效。
var ErrInvalidInput = errors.New("账号巡检参数无效")

// State 是账号巡检领域状态的应用层别名。
type State = inspectiondomain.State

const (
	// StateHealthy 表示最小对话请求已成功完成。
	StateHealthy = inspectiondomain.StateHealthy
	// StateUnavailable 表示已确定当前账号不能用于请求。
	StateUnavailable = inspectiondomain.StateUnavailable
	// StateUncertain 表示网络或临时限流等因素导致无法可靠判断。
	StateUncertain = inspectiondomain.StateUncertain
	// StateSkipped 表示账号未被实际探测，例如已在管理端禁用。
	StateSkipped = inspectiondomain.StateSkipped
	// StateUninspected 表示账号尚未执行过巡检。
	StateUninspected = inspectiondomain.StateUninspected
)

// Classification 是账号巡检领域分类的应用层别名。
type Classification = inspectiondomain.Classification

const (
	// ClassificationHealthy 表示真实对话探测成功。
	ClassificationHealthy = inspectiondomain.ClassificationHealthy
	// ClassificationDisabled 表示账号在管理端已禁用，未发起上游请求。
	ClassificationDisabled = inspectiondomain.ClassificationDisabled
	// ClassificationReauthRequired 表示凭据已失效，需要重新认证。
	ClassificationReauthRequired = inspectiondomain.ClassificationReauthRequired
	// ClassificationQuotaExhausted 表示上游明确返回可识别的额度耗尽。
	ClassificationQuotaExhausted = inspectiondomain.ClassificationQuotaExhausted
	// ClassificationPermissionDenied 表示上游明确拒绝该账号的对话权限。
	ClassificationPermissionDenied = inspectiondomain.ClassificationPermissionDenied
	// ClassificationProbeModelUnavailable 表示测试模型不可用，不能据此判定账号失效。
	ClassificationProbeModelUnavailable = inspectiondomain.ClassificationProbeModelUnavailable
	// ClassificationTemporaryRateLimited 表示上游出现无法确认额度耗尽的临时限流。
	ClassificationTemporaryRateLimited = inspectiondomain.ClassificationTemporaryRateLimited
	// ClassificationProbeError 表示请求超时、网络失败或其他无法确认的探测异常。
	ClassificationProbeError = inspectiondomain.ClassificationProbeError
	// ClassificationUninspected 表示账号尚未留下巡检快照。
	ClassificationUninspected = inspectiondomain.ClassificationUninspected
)

// Mode 表示本次巡检的账号选择策略。
type Mode string

const (
	// ModeFull 表示重新巡检全部 Build 账号。
	ModeFull Mode = "full"
	// ModeIncremental 表示仅巡检尚未保存结果的新 Build 账号。
	ModeIncremental Mode = "incremental"
)

// Suggestion 表示巡检后可由管理员手动确认执行的操作建议。
type Suggestion string

const (
	// SuggestionKeep 表示账号健康，应保持启用。
	SuggestionKeep Suggestion = "keep"
	// SuggestionDisable 表示账号已确认不可用，建议管理员禁用。
	SuggestionDisable Suggestion = "disable"
	// SuggestionReauth 表示账号需要重新授权。
	SuggestionReauth Suggestion = "reauth"
	// SuggestionReview 表示结果不确定，需要人工复查。
	SuggestionReview Suggestion = "review"
	// SuggestionNone 表示尚不建议进行账号管理操作。
	SuggestionNone Suggestion = "none"
)

// Input 定义一次 Grok Build 账号巡检的范围和并发限制。
type Input struct {
	// Provider 必须为 Grok Build。
	Provider accountdomain.Provider
	// AccountIDs 非空时只巡检指定账号；为空时由 Mode 决定巡检范围。
	AccountIDs []uint64
	// IncludeDisabled 控制是否对管理端已禁用的账号发送上游探测。
	IncludeDisabled bool
	// Concurrency 是最多同时发出的上游探测数，0 时使用默认值。
	Concurrency int
	// Mode 指定完整重测或只检查尚无快照的新账号，空值等同完整重测。
	Mode Mode
}

// Progress 表示一次巡检的累计完成进度。
type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// Item 保存单个账号的无敏感信息巡检结论及管理建议。
type Item struct {
	AccountID      uint64                   `json:"accountId,string"`
	Name           string                   `json:"name"`
	Provider       string                   `json:"provider"`
	Enabled        bool                     `json:"enabled"`
	AuthStatus     accountdomain.AuthStatus `json:"authStatus"`
	State          State                    `json:"state"`
	Classification Classification           `json:"classification"`
	Reason         string                   `json:"reason"`
	HTTPStatus     int                      `json:"httpStatus,omitempty"`
	Model          string                   `json:"model,omitempty"`
	Suggestion     Suggestion               `json:"suggestion"`
	InspectedAt    *time.Time               `json:"inspectedAt,omitempty"`
}

// Report 汇总本次巡检结果及所有逐账号结论。
type Report struct {
	Total       int    `json:"total"`
	Healthy     int    `json:"healthy"`
	Unavailable int    `json:"unavailable"`
	Uncertain   int    `json:"uncertain"`
	Skipped     int    `json:"skipped"`
	Results     []Item `json:"results"`
}

// Overview 保存巡检工作台展示的账号快照、分类统计和当前状态。
type Overview struct {
	Total            int    `json:"total"`
	Healthy          int    `json:"healthy"`
	PermissionDenied int    `json:"permissionDenied"`
	QuotaExhausted   int    `json:"quotaExhausted"`
	ReauthRequired   int    `json:"reauthRequired"`
	Exception        int    `json:"exception"`
	Uninspected      int    `json:"uninspected"`
	Results          []Item `json:"results"`
}

// accountStore 约束巡检所需的最小账号读取能力，避免用例依赖完整仓储接口。
type accountStore interface {
	Get(ctx context.Context, id uint64) (accountdomain.Credential, error)
	ListProviderAccountBatch(ctx context.Context, provider accountdomain.Provider, afterID uint64, limit int) ([]accountdomain.Credential, int64, error)
}

// resultStore 约束巡检快照读取、增量筛选和保存所需的最小能力。
type resultStore interface {
	Upsert(ctx context.Context, values []inspectiondomain.Result) error
	ListBuild(ctx context.Context) ([]inspectiondomain.Result, error)
	ListUninspectedBuildAccountIDs(ctx context.Context) ([]uint64, error)
}

// credentialEnsurer 在发送测试请求前按现有策略刷新即将过期的凭据。
type credentialEnsurer interface {
	EnsureCredential(ctx context.Context, value accountdomain.Credential, force bool) (accountdomain.Credential, error)
}

// Service 执行账号巡检、保存快照，并复用已有 Provider Adapter 的真实出站链路。
type Service struct {
	accounts    accountStore
	results     resultStore
	credentials credentialEnsurer
	providers   *provider.Registry
}

// NewService 创建账号巡检服务。
// 参数 accounts 提供账号读取能力，results 保存巡检快照，credentials 负责刷新凭据，providers 提供真实上游适配器；返回可执行巡检的服务。
func NewService(accounts accountStore, results resultStore, credentials credentialEnsurer, providers *provider.Registry) *Service {
	return &Service{accounts: accounts, results: results, credentials: credentials, providers: providers}
}
