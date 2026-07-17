package repository

import (
	"context"

	"github.com/chenyme/grok2api/backend/internal/domain/proxy"
)

// ProxyRepository 定义通用固定代理及其逻辑账号组绑定的持久化能力。
type ProxyRepository interface {
	List(ctx context.Context, query ProxyListQuery) ([]proxy.Endpoint, int64, error)
	ListEnabled(ctx context.Context) ([]proxy.Endpoint, error)
	Get(ctx context.Context, id uint64) (proxy.Endpoint, error)
	Create(ctx context.Context, value proxy.Endpoint) (proxy.Endpoint, error)
	Update(ctx context.Context, value proxy.Endpoint) (proxy.Endpoint, error)
	Delete(ctx context.Context, id uint64) error
	SaveTestResult(ctx context.Context, value proxy.Endpoint) error
	CountFamilies(ctx context.Context, id uint64) (int64, error)
}
