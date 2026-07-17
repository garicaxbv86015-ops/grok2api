package account

import "time"

// Family 表示共享同一固定代理的逻辑账号组。
type Family struct {
	// ID 是逻辑账号组唯一标识。
	ID uint64
	// ProxyID 是当前绑定的固定代理标识，未绑定时为 nil。
	ProxyID *uint64
	// ProxyName 是当前绑定代理的管理端展示名称。
	ProxyName string
	// ProxyEnabled 表示当前绑定代理是否启用。
	ProxyEnabled bool
	// Members 是归属于该逻辑账号组的 Provider 凭据列表。
	Members []FamilyMember
	// CreatedAt 是逻辑账号组创建时间。
	CreatedAt time.Time
	// UpdatedAt 是逻辑账号组最后更新时间。
	UpdatedAt time.Time
}

// FamilyMember 表示逻辑账号组内的一条 Provider 凭据摘要。
type FamilyMember struct {
	// ID 是 Provider 凭据唯一标识。
	ID uint64
	// Provider 是凭据所属的 Grok 产品渠道。
	Provider Provider
	// Name 是凭据管理端展示名称。
	Name string
	// Email 是凭据关联邮箱，缺失时为空。
	Email string
	// Enabled 表示凭据是否允许参与调度。
	Enabled bool
	// AuthStatus 是凭据当前认证状态。
	AuthStatus AuthStatus
}
