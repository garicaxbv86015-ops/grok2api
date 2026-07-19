package accountinspection

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

const probeTimeout = 30 * time.Second

// Inspect 巡检指定的 Grok Build 账号，并返回每个账号的判定结果。
// 参数 ctx 为请求上下文，input 为巡检范围，observer 用于接收实时进度；返回巡检报告或错误。
func (s *Service) Inspect(ctx context.Context, input Input, observer func(Progress)) (Report, error) {
	if s == nil || s.accounts == nil || s.results == nil || s.credentials == nil || s.providers == nil {
		return Report{}, fmt.Errorf("%w: inspection service is unavailable", ErrInvalidInput)
	}

	concurrency, err := validateInput(input)
	if err != nil {
		return Report{}, err
	}

	candidates, err := s.loadCandidates(ctx, input)
	if err != nil {
		return Report{}, err
	}
	if observer != nil {
		observer(Progress{Total: len(candidates)})
	}

	items := s.inspectCandidates(ctx, candidates, input, concurrency, observer)
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	if err := s.persistResults(ctx, items); err != nil {
		return Report{}, err
	}
	return summarize(items), nil
}

// inspectCandidates 以受控并发逐个巡检候选账号，并保持结果顺序稳定。
// 参数 ctx 为请求上下文，values 为候选账号，input 为巡检参数，concurrency 为并发度，observer 接收进度；返回巡检结果列表。
func (s *Service) inspectCandidates(
	ctx context.Context,
	values []accountdomain.Credential,
	input Input,
	concurrency int,
	observer func(Progress),
) []Item {
	items := make([]Item, len(values))
	if len(values) == 0 {
		return items
	}

	jobs := make(chan int)
	var workers sync.WaitGroup
	var progressMu sync.Mutex
	completed := 0

	for range min(concurrency, len(values)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				items[index] = s.inspectOne(ctx, values[index], input.IncludeDisabled)
				if observer == nil {
					continue
				}

				progressMu.Lock()
				completed++
				observer(Progress{Completed: completed, Total: len(values)})
				progressMu.Unlock()
			}
		}()
	}

	for index := range values {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return items
		}
	}
	close(jobs)
	workers.Wait()
	return items
}

// inspectOne 对一个 Grok Build 账号执行凭据刷新与真实响应接口探测。
// 参数 parent 为父级上下文，credential 为待巡检账号，includeDisabled 表示是否包含已禁用账号；返回该账号的巡检结果。
func (s *Service) inspectOne(parent context.Context, credential accountdomain.Credential, includeDisabled bool) Item {
	if !credential.Enabled && !includeDisabled {
		return newItem(credential, StateSkipped, ClassificationDisabled, "账号已在本地管理中禁用", 0)
	}
	if credential.AuthStatus == accountdomain.AuthStatusReauthRequired {
		return newItem(credential, StateUnavailable, ClassificationReauthRequired, "账号需要重新授权", 0)
	}

	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()

	// 1. 刷新可能过期的 Build 凭据，避免将可恢复的过期令牌误判为不可用。
	refreshed, err := s.credentials.EnsureCredential(ctx, credential, false)
	if err != nil {
		return classifyError(credential, err)
	}
	// 2. 复用现有 Build Provider，沿用系统原有的出口、代理和鉴权处理。
	adapter, ok := s.providers.Responses(refreshed.Provider)
	if !ok {
		return newItem(credential, StateUncertain, ClassificationProbeError, "未找到账号对应的响应服务", 0)
	}

	body, model := probePayload()
	response, err := adapter.ForwardResponse(ctx, provider.ResponseResourceRequest{
		Credential: refreshed,
		Method:     http.MethodPost,
		Path:       "/responses",
		Body:       body,
		Model:      model,
		Operation:  "responses",
	})
	if err != nil {
		return classifyError(credential, err)
	}
	return classifyResponse(credential, response)
}

// summarize 汇总单账号巡检结果中的各类统计值。
// 参数 items 为巡检结果列表；返回完整巡检报告。
func summarize(items []Item) Report {
	report := Report{Results: items, Total: len(items)}
	for _, item := range items {
		switch item.State {
		case StateHealthy:
			report.Healthy++
		case StateUnavailable:
			report.Unavailable++
		case StateUncertain:
			report.Uncertain++
		case StateSkipped:
			report.Skipped++
		}
	}
	return report
}
