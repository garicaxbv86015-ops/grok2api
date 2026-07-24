package egress

import (
	"context"
	"fmt"
	"strings"
	"sync"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	domain "github.com/chenyme/grok2api/backend/internal/domain/egress"
)

// Selection is the egress snapshot actually selected for an upstream request. It contains only metadata safe for audit
// and excludes proxy URLs, credentials, User-Agent, and Cookies.
type Selection struct {
	NodeID   uint64
	NodeName string
	Scope    domain.Scope
	Proxied  bool
}

// Trace retains the most recent actual egress selection per scope. When a request retries egress, audit records the final attempt.
// Web asset archival uses an independent scope and does not overwrite the primary Grok Web inference egress.
type Trace struct {
	mu         sync.RWMutex
	selections map[domain.Scope]Selection
}

type traceContextKey struct{}
type accountContextKey struct{}
type credentialContextKey struct{}
type egressNodeContextKey struct{}

// WithAccount passes a stable Provider account identity to the egress layer. It is used only to render
// authentication usernames for sticky proxies such as Resin and is never written to upstream headers or audit.
func WithAccount(ctx context.Context, provider string, accountID uint64) context.Context {
	if ctx == nil || strings.TrimSpace(provider) == "" || accountID == 0 {
		return ctx
	}
	return WithAccountIdentity(ctx, strings.TrimSpace(provider)+"_"+fmt.Sprintf("%d", accountID))
}

// WithCredential 将完整账号出口配置与稳定出口身份传给 Build HTTP Transport。
// 参数 ctx 为请求上下文，credential 为账号凭据；返回携带非公开出口配置和稳定账号身份的上下文。
// 若账号已设置 EgressIdentity（例如 Web/Console 共享 SSO），则优先使用该身份做粘性代理绑定；
// 未设置时保留既有 Provider+ID 身份。
func WithCredential(ctx context.Context, credential accountdomain.Credential) context.Context {
	if ctx == nil || credential.ID == 0 {
		return ctx
	}
	ctx = context.WithValue(ctx, credentialContextKey{}, credential)
	identity := strings.TrimSpace(credential.EgressIdentity)
	if identity != "" {
		return WithEgressNode(WithAccountIdentity(ctx, identity), credential.EgressNodeID)
	}
	provider := credential.Provider
	if provider == "" {
		// CLI 单元测试及部分刷新路径只携带账号 ID；该上下文入口由 Build Transport 使用。
		provider = accountdomain.ProviderBuild
	}
	return WithEgressNode(WithAccount(ctx, string(provider), credential.ID), credential.EgressNodeID)
}

// CredentialFromContext 返回 Build HTTP Transport 所需的账号出口配置。
// 参数 ctx 为请求上下文；返回账号凭据和是否存在。
func CredentialFromContext(ctx context.Context) (accountdomain.Credential, bool) {
	if ctx == nil {
		return accountdomain.Credential{}, false
	}
	value, ok := ctx.Value(credentialContextKey{}).(accountdomain.Credential)
	return value, ok
}

// WithEgressNode attaches the explicitly assigned node ID for transports that
// only receive a request context (notably Grok Build's RoundTripper).
func WithEgressNode(ctx context.Context, nodeID uint64) context.Context {
	if ctx == nil || nodeID == 0 {
		return ctx
	}
	return context.WithValue(ctx, egressNodeContextKey{}, nodeID)
}

func egressNodeFromContext(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	value, _ := ctx.Value(egressNodeContextKey{}).(uint64)
	return value
}

// EgressNodeFromContext exposes a non-sensitive binding identifier to the
// Build transport without exposing the context key itself.
func EgressNodeFromContext(ctx context.Context) uint64 { return egressNodeFromContext(ctx) }

// WithAccountIdentity attaches the stable, non-sensitive identity used by
// account-bound proxy templates such as Resin. Providers that represent the
// same upstream login (for example Web and Console sharing one SSO token) can
// deliberately pass the same identity so their proxy and clearance lease is
// not split by the internal provider name.
func WithAccountIdentity(ctx context.Context, identity string) context.Context {
	if ctx == nil || strings.TrimSpace(identity) == "" {
		return ctx
	}
	return context.WithValue(ctx, accountContextKey{}, strings.TrimSpace(identity))
}

func accountFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(accountContextKey{}).(string)
	return strings.TrimSpace(value)
}

// AccountFromContext exposes the non-sensitive sticky account identity to
// provider transports while keeping the context key private.
func AccountFromContext(ctx context.Context) string { return accountFromContext(ctx) }

// WithTrace creates or reuses a concurrency-safe egress selection trace for one gateway request.
func WithTrace(ctx context.Context) (context.Context, *Trace) {
	if existing := TraceFromContext(ctx); existing != nil {
		return ctx, existing
	}
	trace := &Trace{selections: make(map[domain.Scope]Selection)}
	return context.WithValue(ctx, traceContextKey{}, trace), trace
}

// TraceFromContext returns the egress trace from context, or nil when none is configured.
func TraceFromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceContextKey{}).(*Trace)
	return trace
}

// Selection returns a safe snapshot of the most recent actual egress selection for a scope.
func (t *Trace) Selection(scope domain.Scope) (Selection, bool) {
	if t == nil {
		return Selection{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	value, ok := t.selections[scope]
	return value, ok
}

func recordSelection(ctx context.Context, value Selection) {
	trace := TraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.selections[value.Scope] = value
	trace.mu.Unlock()
}
