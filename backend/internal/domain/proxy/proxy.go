package proxy

import "time"

// Endpoint 表示可被逻辑账号组复用的固定代理资源。
type Endpoint struct {
	// ID 是代理资源的唯一标识。
	ID uint64
	// Name 是管理端展示的代理名称。
	Name string
	// Protocol 是代理协议，例如 http、https 或 socks5。
	Protocol string
	// Host 是代理服务器主机名或 IP 地址。
	Host string
	// Port 是代理服务器端口。
	Port int
	// EncryptedURL 是包含认证信息的加密代理地址，仅供服务端使用。
	EncryptedURL string
	// AuthConfigured 表示代理是否配置了用户名或密码。
	AuthConfigured bool
	// Enabled 表示代理是否允许被已绑定账号使用。
	Enabled bool
	// LastTestOK 记录最近一次连接测试是否成功，未测试时为 nil。
	LastTestOK *bool
	// LastLatencyMS 记录最近一次成功连接测试的毫秒耗时。
	LastLatencyMS *int64
	// LastTestError 记录最近一次连接测试的安全错误摘要。
	LastTestError string
	// LastTestAt 记录最近一次连接测试时间。
	LastTestAt *time.Time
	// BoundFamilyCount 是当前绑定该代理的逻辑账号组数量。
	BoundFamilyCount int64
	// CreatedAt 是代理资源创建时间。
	CreatedAt time.Time
	// UpdatedAt 是代理资源最后更新时间。
	UpdatedAt time.Time
}
