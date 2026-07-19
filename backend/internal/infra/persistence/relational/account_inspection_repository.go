package relational

import (
	"context"

	inspection "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	"gorm.io/gorm/clause"
)

// AccountInspectionRepository 提供 Build 账号巡检快照的关系型持久化实现。
type AccountInspectionRepository struct {
	db *Database
}

// NewAccountInspectionRepository 创建账号巡检快照仓储。
// 参数 db 为关系型数据库；返回巡检仓储。
func NewAccountInspectionRepository(db *Database) *AccountInspectionRepository {
	return &AccountInspectionRepository{db: db}
}

// Upsert 写入每个账号的最新巡检快照。
// 参数 ctx 为请求上下文，values 为巡检快照；返回持久化错误。
func (r *AccountInspectionRepository) Upsert(ctx context.Context, values []inspection.Result) error {
	if len(values) == 0 {
		return nil
	}
	models := make([]accountInspectionModel, 0, len(values))
	for _, value := range values {
		models = append(models, accountInspectionModel{
			AccountID: value.AccountID, State: string(value.State), Classification: string(value.Classification),
			Reason: value.Reason, HTTPStatus: value.HTTPStatus, Model: value.Model, InspectedAt: value.InspectedAt,
		})
	}
	return r.db.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "classification", "reason", "http_status", "model", "inspected_at"}),
	}).Create(&models).Error
}

// ListBuild 读取所有仍关联 Grok Build 账号的最新巡检快照。
// 参数 ctx 为请求上下文；返回巡检快照和错误。
func (r *AccountInspectionRepository) ListBuild(ctx context.Context) ([]inspection.Result, error) {
	rows := make([]accountInspectionModel, 0)
	err := r.db.db.WithContext(ctx).Model(&accountInspectionModel{}).
		Joins("JOIN provider_accounts ON provider_accounts.id = account_inspections.account_id").
		Where("provider_accounts.provider = ?", accountdomain.ProviderBuild).
		Order("account_inspections.account_id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	values := make([]inspection.Result, 0, len(rows))
	for _, row := range rows {
		values = append(values, inspection.Result{
			AccountID: row.AccountID, State: inspection.State(row.State), Classification: inspection.Classification(row.Classification),
			Reason: row.Reason, HTTPStatus: row.HTTPStatus, Model: row.Model, InspectedAt: row.InspectedAt.UTC(),
		})
	}
	return values, nil
}

// ListUninspectedBuildAccountIDs 查询没有最新巡检快照的 Build 账号，用于增量巡检。
// 参数 ctx 为请求上下文；返回账号编号和错误。
func (r *AccountInspectionRepository) ListUninspectedBuildAccountIDs(ctx context.Context) ([]uint64, error) {
	ids := make([]uint64, 0)
	err := r.db.db.WithContext(ctx).Table("provider_accounts").
		Where("provider = ? AND NOT EXISTS (SELECT 1 FROM account_inspections WHERE account_inspections.account_id = provider_accounts.id)", accountdomain.ProviderBuild).
		Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}
