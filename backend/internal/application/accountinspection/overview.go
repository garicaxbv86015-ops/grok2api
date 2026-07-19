package accountinspection

import (
	"context"
	"fmt"

	inspectiondomain "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
)

// GetOverview 读取所有 Build 账号及其最近巡检快照，供独立巡检工作台展示。
// 参数 ctx 为请求上下文；返回完整工作台快照和错误。
func (s *Service) GetOverview(ctx context.Context) (Overview, error) {
	if s == nil || s.accounts == nil || s.results == nil {
		return Overview{}, fmt.Errorf("%w: inspection service is unavailable", ErrInvalidInput)
	}
	credentials, err := s.loadProviderCandidates(ctx, accountdomain.ProviderBuild)
	if err != nil {
		return Overview{}, err
	}
	persisted, err := s.results.ListBuild(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("读取巡检结果: %w", err)
	}
	byAccountID := make(map[uint64]inspectiondomain.Result, len(persisted))
	for _, result := range persisted {
		byAccountID[result.AccountID] = result
	}

	overview := Overview{Total: len(credentials), Results: make([]Item, 0, len(credentials))}
	for _, credential := range credentials {
		item := overviewItem(credential, byAccountID[credential.ID])
		overview.Results = append(overview.Results, item)
		accumulateOverview(&overview, item)
	}
	return overview, nil
}

// overviewItem 将账号基础状态与可选的持久化快照合成为工作台列表项。
// 参数 credential 为 Build 账号，result 为最近巡检快照；返回工作台展示项。
func overviewItem(credential accountdomain.Credential, result inspectiondomain.Result) Item {
	if result.AccountID == 0 {
		return Item{
			AccountID: credential.ID, Name: credential.Name, Provider: string(credential.Provider), Enabled: credential.Enabled, AuthStatus: credential.AuthStatus,
			State: StateUninspected, Classification: ClassificationUninspected, Reason: "尚未巡检", Suggestion: SuggestionNone,
		}
	}
	inspectedAt := result.InspectedAt.UTC()
	return Item{
		AccountID: credential.ID, Name: credential.Name, Provider: string(credential.Provider), Enabled: credential.Enabled, AuthStatus: credential.AuthStatus,
		State: result.State, Classification: result.Classification, Reason: result.Reason, HTTPStatus: result.HTTPStatus, Model: result.Model,
		Suggestion: suggestionFor(result.State, result.Classification), InspectedAt: &inspectedAt,
	}
}

// accumulateOverview 根据单条巡检项累加截图中的分类统计卡数值。
// 参数 overview 为待更新的汇总，item 为单条巡检项；无返回值。
func accumulateOverview(overview *Overview, item Item) {
	if item.State == StateUninspected {
		overview.Uninspected++
		return
	}
	if item.State == StateHealthy {
		overview.Healthy++
		return
	}
	switch item.Classification {
	case ClassificationPermissionDenied:
		overview.PermissionDenied++
	case ClassificationQuotaExhausted:
		overview.QuotaExhausted++
	case ClassificationReauthRequired:
		overview.ReauthRequired++
	default:
		overview.Exception++
	}
}
