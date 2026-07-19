package accountinspection

import (
	"context"
	"fmt"
	"time"

	inspectiondomain "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
)

// persistResults 将本次真实完成的探测结果写为每个账号的最新巡检快照。
// 参数 ctx 为请求上下文，items 为本次巡检结果；返回持久化错误。
func (s *Service) persistResults(ctx context.Context, items []Item) error {
	inspectedAt := time.Now().UTC()
	values := make([]inspectiondomain.Result, 0, len(items))
	for index := range items {
		if items[index].State == StateSkipped || items[index].State == StateUninspected {
			continue
		}
		items[index].InspectedAt = &inspectedAt
		values = append(values, inspectiondomain.Result{
			AccountID: items[index].AccountID, State: items[index].State, Classification: items[index].Classification,
			Reason: items[index].Reason, HTTPStatus: items[index].HTTPStatus, Model: items[index].Model, InspectedAt: inspectedAt,
		})
	}
	if err := s.results.Upsert(ctx, values); err != nil {
		return fmt.Errorf("保存巡检结果: %w", err)
	}
	return nil
}
