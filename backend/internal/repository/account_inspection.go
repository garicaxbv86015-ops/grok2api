package repository

import (
	"context"

	inspection "github.com/chenyme/grok2api/backend/internal/domain/accountinspection"
)

// AccountInspectionRepository 定义 Build 账号最新巡检快照的持久化边界。
type AccountInspectionRepository interface {
	// Upsert 保存各账号最新一次巡检结论。
	// 参数 ctx 为请求上下文，values 为待保存快照；返回持久化错误。
	Upsert(ctx context.Context, values []inspection.Result) error
	// ListBuild 返回所有仍关联 Build 账号的最新巡检快照。
	// 参数 ctx 为请求上下文；返回巡检快照和错误。
	ListBuild(ctx context.Context) ([]inspection.Result, error)
	// ListUninspectedBuildAccountIDs 返回尚未拥有巡检快照的 Build 账号编号。
	// 参数 ctx 为请求上下文；返回账号编号和错误。
	ListUninspectedBuildAccountIDs(ctx context.Context) ([]uint64, error)
}
