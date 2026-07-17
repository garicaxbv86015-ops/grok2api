package relational

import (
	"context"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// ProxyRepository 持有通用代理资源的关系型数据库实现。
type ProxyRepository struct{ db *Database }

// NewProxyRepository 创建通用代理仓储。
// 参数 db 为共享关系型数据库；返回可供应用层使用的代理仓储。
func NewProxyRepository(db *Database) *ProxyRepository { return &ProxyRepository{db: db} }

// List 分页查询代理资源，并附加逻辑账号组绑定数量。
// 参数 ctx 为请求上下文，input 为过滤、排序和分页条件；返回代理列表、总数和错误。
func (r *ProxyRepository) List(ctx context.Context, input repository.ProxyListQuery) ([]proxy.Endpoint, int64, error) {
	query := r.db.db.WithContext(ctx).Model(&proxyModel{})
	if search := strings.TrimSpace(input.Page.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(name) LIKE ? OR LOWER(host) LIKE ?", pattern, pattern)
	}
	if input.Filter.Enabled != nil {
		query = query.Where("enabled = ?", *input.Filter.Enabled)
	}
	if input.Filter.Protocol != "" {
		query = query.Where("protocol = ?", input.Filter.Protocol)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	query = applyStableSort(query, input.Page.Sort, map[string]sortSpec{
		"name": {expression: "LOWER(proxies.name)"},
		"createdAt": {expression: "proxies.created_at", defaultDirection: repository.SortDescending},
	}, sortSpec{expression: "proxies.created_at", defaultDirection: repository.SortDescending}, "proxies.id")
	var rows []proxyModel
	if err := query.Offset(input.Page.Offset).Limit(input.Page.Limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	counts, err := r.familyCounts(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	values := make([]proxy.Endpoint, 0, len(rows))
	for _, row := range rows {
		value := toProxyDomain(row)
		value.BoundFamilyCount = counts[row.ID]
		values = append(values, value)
	}
	return values, total, nil
}

// ListEnabled 返回账号编辑器可选择的全部启用代理。
// 参数 ctx 为请求上下文；返回按名称排序的代理列表和错误。
func (r *ProxyRepository) ListEnabled(ctx context.Context) ([]proxy.Endpoint, error) {
	var rows []proxyModel
	if err := r.db.db.WithContext(ctx).Where("enabled = ?", true).Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	values := make([]proxy.Endpoint, 0, len(rows))
	for _, row := range rows {
		values = append(values, toProxyDomain(row))
	}
	return values, nil
}

// Get 按标识读取单个代理资源。
// 参数 ctx 为请求上下文，id 为代理标识；返回代理领域对象和错误。
func (r *ProxyRepository) Get(ctx context.Context, id uint64) (proxy.Endpoint, error) {
	var row proxyModel
	if err := r.db.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return proxy.Endpoint{}, mapError(err)
	}
	value := toProxyDomain(row)
	count, err := r.CountFamilies(ctx, id)
	if err != nil {
		return proxy.Endpoint{}, err
	}
	value.BoundFamilyCount = count
	return value, nil
}

// Create 新建通用代理资源。
// 参数 ctx 为请求上下文，value 为已验证且加密的代理；返回持久化结果和错误。
func (r *ProxyRepository) Create(ctx context.Context, value proxy.Endpoint) (proxy.Endpoint, error) {
	row := fromProxyDomain(value)
	if err := r.db.db.WithContext(ctx).Create(&row).Error; err != nil {
		return proxy.Endpoint{}, mapError(err)
	}
	return r.Get(ctx, row.ID)
}

// Update 更新已有通用代理资源。
// 参数 ctx 为请求上下文，value 为完整代理对象；返回持久化结果和错误。
func (r *ProxyRepository) Update(ctx context.Context, value proxy.Endpoint) (proxy.Endpoint, error) {
	result := r.db.db.WithContext(ctx).Model(&proxyModel{}).Where("id = ?", value.ID).Updates(map[string]any{
		"name": value.Name, "protocol": value.Protocol, "host": value.Host, "port": value.Port,
		"encrypted_url": value.EncryptedURL, "auth_configured": value.AuthConfigured, "enabled": value.Enabled,
		"last_test_ok": value.LastTestOK, "last_latency_ms": value.LastLatencyMS, "last_test_error": value.LastTestError,
		"last_test_at": value.LastTestAt, "updated_at": value.UpdatedAt,
	})
	if result.Error != nil {
		return proxy.Endpoint{}, mapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return proxy.Endpoint{}, repository.ErrNotFound
	}
	return r.Get(ctx, value.ID)
}

// Delete 删除未被逻辑账号组引用的代理资源。
// 参数 ctx 为请求上下文，id 为代理标识；返回删除冲突或数据库错误。
func (r *ProxyRepository) Delete(ctx context.Context, id uint64) error {
	count, err := r.CountFamilies(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return repository.ErrConflict
	}
	result := r.db.db.WithContext(ctx).Delete(&proxyModel{}, id)
	if result.Error != nil {
		return mapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// SaveTestResult 保存单个代理最近一次连接测试结果。
// 参数 ctx 为请求上下文，value 携带代理标识和测试字段；返回持久化错误。
func (r *ProxyRepository) SaveTestResult(ctx context.Context, value proxy.Endpoint) error {
	result := r.db.db.WithContext(ctx).Model(&proxyModel{}).Where("id = ?", value.ID).Updates(map[string]any{
		"last_test_ok": value.LastTestOK,
		"last_latency_ms": value.LastLatencyMS,
		"last_test_error": value.LastTestError,
		"last_test_at": value.LastTestAt,
	})
	if result.Error != nil {
		return mapError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// CountFamilies 统计引用指定代理的逻辑账号组数量。
// 参数 ctx 为请求上下文，id 为代理标识；返回绑定数量和错误。
func (r *ProxyRepository) CountFamilies(ctx context.Context, id uint64) (int64, error) {
	var count int64
	err := r.db.db.WithContext(ctx).Model(&accountFamilyModel{}).Where("proxy_id = ?", id).Count(&count).Error
	return count, err
}

// familyCounts 批量统计当前分页中每个代理绑定的逻辑账号组数量。
// 参数 ctx 为请求上下文，rows 为当前代理模型；返回代理标识到绑定数量的映射和错误。
func (r *ProxyRepository) familyCounts(ctx context.Context, rows []proxyModel) (map[uint64]int64, error) {
	counts := make(map[uint64]int64, len(rows))
	if len(rows) == 0 {
		return counts, nil
	}
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var values []struct {
		ProxyID uint64
		Count   int64
	}
	if err := r.db.db.WithContext(ctx).Model(&accountFamilyModel{}).Select("proxy_id", "COUNT(*) AS count").Where("proxy_id IN ?", ids).Group("proxy_id").Scan(&values).Error; err != nil {
		return nil, err
	}
	for _, value := range values {
		counts[value.ProxyID] = value.Count
	}
	return counts, nil
}
