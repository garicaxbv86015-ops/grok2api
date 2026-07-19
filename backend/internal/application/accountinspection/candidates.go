package accountinspection

import (
	"context"
	"fmt"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
)

const (
	defaultConcurrency = 6
	maxConcurrency     = 16
	accountBatchSize   = 1000
)

// validateInput 规范化并校验 Grok Build 巡检参数，防止一次任务占满全部出站资源。
// 参数 input 为原始巡检请求；返回最终并发数和参数错误。
func validateInput(input Input) (int, error) {
	if input.Provider != accountdomain.ProviderBuild {
		return 0, ErrInvalidInput
	}
	if input.Mode != "" && input.Mode != ModeFull && input.Mode != ModeIncremental {
		return 0, ErrInvalidInput
	}
	concurrency := input.Concurrency
	if concurrency == 0 {
		concurrency = defaultConcurrency
	}
	if concurrency < 1 || concurrency > maxConcurrency {
		return 0, ErrInvalidInput
	}
	for _, id := range input.AccountIDs {
		if id == 0 {
			return 0, ErrInvalidInput
		}
	}
	return concurrency, nil
}

// loadCandidates 读取指定账号或 Grok Build 全部账号，并确保指定账号仍属于目标渠道。
// 参数 ctx 为调用生命周期，input 定义筛选范围；返回按稳定顺序排列的账号和错误。
func (s *Service) loadCandidates(ctx context.Context, input Input) ([]accountdomain.Credential, error) {
	if len(input.AccountIDs) > 0 {
		return s.loadSelectedCandidates(ctx, input.Provider, input.AccountIDs)
	}
	if input.Mode == ModeIncremental {
		ids, err := s.results.ListUninspectedBuildAccountIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("读取未巡检账号: %w", err)
		}
		return s.loadSelectedCandidates(ctx, input.Provider, ids)
	}
	return s.loadProviderCandidates(ctx, input.Provider)
}

// loadSelectedCandidates 读取手动选择的账号，并对重复编号去重。
// 参数 ctx 为调用生命周期，providerValue 为所属渠道，ids 为候选账号标识；返回可巡检账号和错误。
func (s *Service) loadSelectedCandidates(ctx context.Context, providerValue accountdomain.Provider, ids []uint64) ([]accountdomain.Credential, error) {
	seen := make(map[uint64]struct{}, len(ids))
	values := make([]accountdomain.Credential, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		value, err := s.accounts.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("读取待巡检账号: %w", err)
		}
		if value.Provider != providerValue {
			return nil, ErrInvalidInput
		}
		values = append(values, value)
	}
	return values, nil
}

// loadProviderCandidates 分页读取 Grok Build 全部账号，避免在账号量较大时构造无界查询。
// 参数 ctx 为调用生命周期，providerValue 为目标渠道；返回完整账号集合和错误。
func (s *Service) loadProviderCandidates(ctx context.Context, providerValue accountdomain.Provider) ([]accountdomain.Credential, error) {
	values := make([]accountdomain.Credential, 0)
	var afterID uint64
	for {
		batch, _, err := s.accounts.ListProviderAccountBatch(ctx, providerValue, afterID, accountBatchSize)
		if err != nil {
			return nil, fmt.Errorf("读取待巡检账号: %w", err)
		}
		values = append(values, batch...)
		if len(batch) < accountBatchSize {
			return values, nil
		}
		afterID = batch[len(batch)-1].ID
	}
}
