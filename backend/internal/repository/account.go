package repository

import (
	"context"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

// AccountUpdates 表示批量账号更新中允许持久化的字段。
type AccountUpdates struct {
	Enabled          *bool
	Priority         *int
	MaxConcurrent    *int
	MinimumRemaining *float64
}

type AccountUpsertResult struct {
	ID      uint64
	Created bool
}

// AccountFamilyUpsertResult 表示一次逻辑账号组三 Provider 原子写入结果。
type AccountFamilyUpsertResult struct {
	// FamilyID 是三种 Provider 凭据共同归属的逻辑账号组标识。
	FamilyID uint64
	// Accounts 按输入顺序返回各 Provider 凭据的写入结果。
	Accounts []AccountUpsertResult
}

// AccountFamilyCleanupResult 表示按条件清理逻辑账号组后的删除对象。
type AccountFamilyCleanupResult struct {
	// FamilyIDs 是本次被删除的逻辑账号组标识。
	FamilyIDs []uint64
	// AccountIDs 是本次被删除的 Provider 成员标识。
	AccountIDs []uint64
}

// CredentialRefreshFailure 是最近一次 OAuth 刷新失败时持久化的有界诊断状态。
// Response 必须在到达持久层前由 provider 适配器完成脱敏。
type CredentialRefreshFailure struct {
	Count     int
	RetryAt   time.Time
	Status    int
	Code      string
	Message   string
	Response  string
	Permanent bool
}

// LinkedDeleteResolution 是带可选关联账号时服务端展开的删除范围。
type LinkedDeleteResolution struct {
	RootIDs          []uint64
	FinalIDs         []uint64
	LinkedByProvider map[account.Provider]int
	// RootGroups 将每个根账号映射到最终关联成员，并定义媒体任务跳过的组边界。
	RootGroups map[uint64][]uint64
	// PeerProviders 记录每个关联成员的 Provider，用于删除后的统计。
	PeerProviders map[uint64]account.Provider
}

// LinkedDeleteOutcome 包含实际删除行与因媒体任务而跳过的根组。
type LinkedDeleteOutcome struct {
	Resolution              LinkedDeleteResolution
	DeletedIDs              []uint64
	Deleted                 int64
	RootsDeleted            int64
	LinkedDeletedByProvider map[account.Provider]int64
	// SkippedRoots 标识因排队中或进行中视频任务而受保护的组。
	SkippedRoots []uint64
}

// CleanupPreview 是清理确认对话框使用的 COUNT-only 预览结果。
type CleanupPreview struct {
	RootsByStatus    map[string]int64
	RootCount        int64
	LinkedByProvider map[account.Provider]int64
	Total            int64
}

// ObservedModelWriter reports whether an observed model update changed the authoritative row.
type ObservedModelWriter interface {
	UpdateObservedModelIfNewer(ctx context.Context, id uint64, model string, observedAt time.Time) (bool, error)
}

// RoutingLayerRepository separates reusable account state from model overlays.
type RoutingLayerRepository interface {
	ListRoutingAccountBases(ctx context.Context, provider account.Provider, quotaMode string) ([]account.RoutingAccountBase, error)
	ListRoutingAccountOverlays(ctx context.Context, provider account.Provider, modelRouteID uint64, upstreamModel string) (account.RoutingOverlaySnapshot, error)
}

// AccountRepository 定义 OAuth 账号和额度快照持久化能力。
type AccountRepository interface {
	List(ctx context.Context, query AccountListQuery) ([]account.Credential, int64, error)
	// ListFamilies 分页返回逻辑账号组及其 Web、Build、Console 成员摘要。
	ListFamilies(ctx context.Context, query AccountFamilyListQuery) ([]account.Family, int64, error)
	// DeleteFamily 在单次事务中删除逻辑账号组及其全部 Provider 成员；ctx 为上下文，familyID 为账号组标识；返回已删除成员标识和错误。
	DeleteFamily(ctx context.Context, familyID uint64) ([]uint64, error)
	// DeleteFamiliesWithoutBuild 在单次事务中删除不含 Build 成员的逻辑账号组；ctx 为上下文；返回被删除的组和成员标识。
	DeleteFamiliesWithoutBuild(ctx context.Context) (AccountFamilyCleanupResult, error)
	// ListProviderAccountBatch 以 ID 游标取一批账号；total 仅在 afterID 为 0 时返回。
	ListProviderAccountBatch(ctx context.Context, provider account.Provider, afterID uint64, limit int) ([]account.Credential, int64, error)
	Summarize(ctx context.Context, now time.Time) ([]AccountSummary, error)
	ListEnabled(ctx context.Context, provider account.Provider) ([]account.Credential, error)
	ListEnabledAccountIDs(ctx context.Context, provider account.Provider, refreshableOnly bool) ([]uint64, error)
	// ListEnabledCredentialRefreshAccountIDs includes enabled active and
	// reauthRequired accounts so an explicit administrator refresh can retry a
	// previously rejected refresh token once.
	ListEnabledCredentialRefreshAccountIDs(ctx context.Context, provider account.Provider, refreshableOnly bool) ([]uint64, error)
	CountProviderAccountsByIDs(ctx context.Context, provider account.Provider, ids []uint64) (int64, error)
	// FilterMissingBuildConversionIDs 从指定账号中排除已经关联 Build 的 Web 账号。
	FilterMissingBuildConversionIDs(ctx context.Context, ids []uint64) ([]uint64, error)
	// ListUnlinkedWebAccountIDs 以 ID 游标取未关联 Web 账号；total 仅在 afterID 为 0 时返回。
	ListUnlinkedWebAccountIDs(ctx context.Context, afterID uint64, limit int) ([]uint64, int64, error)
	// ListMissingConsoleSyncAccounts 从指定账号中排除已有对应 Console 账号的 Web 账号。
	ListMissingConsoleSyncAccounts(ctx context.Context, ids []uint64) ([]account.Credential, error)
	// ListMissingConsoleSyncBatch 以 ID 游标取缺少 Console 账号的 Web 账号；total/skipped 仅在 afterID 为 0 时返回。
	ListMissingConsoleSyncBatch(ctx context.Context, afterID uint64, limit int) ([]account.Credential, int64, int64, error)
	HasActive(ctx context.Context, provider account.Provider) (bool, error)
	ListRoutingCandidates(ctx context.Context, provider account.Provider, modelRouteID uint64, upstreamModel, quotaMode string) ([]account.RoutingCandidate, error)
	GetCredentialMaterial(ctx context.Context, accountID uint64, provider account.Provider) (account.CredentialMaterial, error)
	Get(ctx context.Context, id uint64) (account.Credential, error)
	// SetFamilyProxy 更新单个逻辑账号组的固定代理；ctx 为上下文，familyID 为账号组标识，proxyID 为 nil 时解除绑定；返回写入错误。
	SetFamilyProxy(ctx context.Context, familyID uint64, proxyID *uint64) error
	// SetFamilyProxies 在单次事务内更新多个账号组；ctx 为上下文，familyIDs 为组标识，proxyID 为 nil 时解除绑定；返回更新数和错误。
	SetFamilyProxies(ctx context.Context, familyIDs []uint64, proxyID *uint64) (int64, error)
	LinkWebToBuild(ctx context.Context, webAccountID, buildAccountID uint64) error
	GetBillings(ctx context.Context, accountIDs []uint64) (map[uint64]account.Billing, error)
	GetQuotaRecoveries(ctx context.Context, accountIDs []uint64) (map[uint64]account.QuotaRecovery, error)
	UpsertByIdentity(ctx context.Context, value account.Credential) (account.Credential, bool, error)
	Update(ctx context.Context, value account.Credential) (account.Credential, error)
	UpdateMany(ctx context.Context, provider account.Provider, ids []uint64, updates AccountUpdates) (int64, error)
	Delete(ctx context.Context, id uint64) error
	DeleteMany(ctx context.Context, ids []uint64) (int64, error)
	// ResolveLinkedDeleteIDs expands root account IDs with one-hop (or Build/Console two-hop via Web)
	// peers from link tables for optional linked deletion. It never guesses by email/name/userId.
	ResolveLinkedDeleteIDs(ctx context.Context, provider account.Provider, rootIDs []uint64, targets []account.Provider) (LinkedDeleteResolution, error)
	// DeleteManyWithLinked locks roots, resolves linked peers, checks media jobs, and deletes
	// the final set inside a single DB transaction (avoids resolve/delete TOCTOU).
	// skipMedia=false rejects on active media; skipMedia=true skips the complete root group.
	DeleteManyWithLinked(ctx context.Context, provider account.Provider, rootIDs []uint64, targets []account.Provider, skipMedia bool) (LinkedDeleteOutcome, error)
	// DeleteAccountStatusBatchWithLinked selects at most limit roots by state and ID cursor,
	// expands links, skips protected groups, and returns the candidate count and next cursor.
	DeleteAccountStatusBatchWithLinked(ctx context.Context, provider account.Provider, status string, now time.Time, afterID uint64, limit int, targets []account.Provider) (LinkedDeleteOutcome, int, uint64, error)
	// CountCleanupWithLinked returns root and linked-peer counts using SQL COUNT queries only.
	CountCleanupWithLinked(ctx context.Context, provider account.Provider, statuses []string, now time.Time, targets []account.Provider) (CleanupPreview, error)
	// ListAutoCleanReauthCandidates 以 ID 游标列出达到清理年龄的 reauthRequired 账号。
	ListAutoCleanReauthCandidates(ctx context.Context, markedBefore time.Time, includeDisabled bool, afterID uint64, limit int) ([]uint64, error)
	// DeleteAutoCleanReauthCandidates 在事务内重新校验状态与年龄并跳过活动视频任务，返回实际删除 ID。
	DeleteAutoCleanReauthCandidates(ctx context.Context, markedBefore time.Time, includeDisabled bool, candidateIDs []uint64) ([]uint64, error)
	UpdateTokens(ctx context.Context, id uint64, accessToken, refreshToken string, expiresAt time.Time) (account.Credential, error)
	BackfillCredentialRefreshSchedules(ctx context.Context, now time.Time, limit int) (int, error)
	ListCriticalCredentialRefreshIDs(ctx context.Context, now, expiresBefore time.Time, limit int) ([]uint64, error)
	ListDueCredentialRefreshIDs(ctx context.Context, now time.Time, limit int) ([]uint64, error)
	NextCredentialRefreshDueAt(ctx context.Context) (*time.Time, error)
	UpdateCredentialRefreshFailure(ctx context.Context, id uint64, failure CredentialRefreshFailure) error
	UpdateObservedModel(ctx context.Context, id uint64, model string, observedAt time.Time) error
	UpdateHealth(ctx context.Context, id uint64, failureCount int, cooldownUntil *time.Time, lastError string, success bool) error
	// MarkBuildAPIFallback 幂等写入 Build 账号的 XAI 推理回退标记；非 Build 账号返回错误。
	MarkBuildAPIFallback(ctx context.Context, id uint64, enabled bool) error
	// MarkWebNSFWEnabled 幂等记录 Web 账号首次确认 NSFW 已开启的时间。
	MarkWebNSFWEnabled(ctx context.Context, id uint64, enabledAt time.Time) error
	// MarkWebTermsAccepted 幂等记录 Web 账号已完整接受的产品协议版本与时间。
	MarkWebTermsAccepted(ctx context.Context, id uint64, version int, acceptedAt time.Time) error
	// MarkWebBirthDateSet 幂等记录 Web 账号首次确认生日已设置的时间。
	MarkWebBirthDateSet(ctx context.Context, id uint64, setAt time.Time) error
	UpsertModelQuotaBlock(ctx context.Context, value account.ModelQuotaBlock) error
	PruneExpiredModelQuotaBlocks(ctx context.Context, now time.Time, limit int) (int64, error)
	SaveBilling(ctx context.Context, value account.Billing) error
	GetBilling(ctx context.Context, accountID uint64) (account.Billing, error)
	GetQuotaRecovery(ctx context.Context, accountID uint64) (account.QuotaRecovery, error)
	SaveQuotaRecovery(ctx context.Context, value account.QuotaRecovery) error
	ClaimQuotaProbe(ctx context.Context, accountID uint64, now, leaseUntil time.Time) (bool, error)
	ClearQuotaRecovery(ctx context.Context, accountID uint64) error
	ResetQuotaState(ctx context.Context, provider account.Provider, accountIDs []uint64) error
	ResetProviderQuotaState(ctx context.Context, provider account.Provider, activeOnly bool) (int64, error)
	HasQuotaWindows(ctx context.Context, accountID uint64) (bool, error)
	GetQuotaWindows(ctx context.Context, accountIDs []uint64) (map[uint64][]account.QuotaWindow, error)
	ReplaceQuotaWindows(ctx context.Context, accountID uint64, tier account.WebTier, syncedAt time.Time, values []account.QuotaWindow) error
	SaveQuotaWindows(ctx context.Context, accountID uint64, tier account.WebTier, syncedAt time.Time, values []account.QuotaWindow) error
	UpsertManyByIdentity(ctx context.Context, values []account.Credential) ([]AccountUpsertResult, error)
	// UpsertAccountFamily 原子写入逻辑账号；ctx 为上下文，values 为同组 Provider 凭据，proxyID 为代理标识；返回组和成员写入结果及错误。
	UpsertAccountFamily(ctx context.Context, values []account.Credential, proxyID uint64) (AccountFamilyUpsertResult, error)
	DecrementQuotaWindow(ctx context.Context, accountID uint64, mode string, now time.Time) (bool, error)
	ExhaustQuotaWindow(ctx context.Context, accountID uint64, mode string, resetAt *time.Time, now time.Time) error
	ListDueQuotaWindows(ctx context.Context, now time.Time, limit int) ([]account.QuotaWindow, error)
	ListQuotaRecoveryWindows(ctx context.Context, limit int) ([]account.QuotaWindow, error)
	ListStaleWebQuotaAccountIDs(ctx context.Context, before time.Time, limit int) ([]uint64, error)
}
