package relational

import (
	"context"
	"errors"
	"testing"

	accountdomain "github.com/chenyme/grok2api/backend/internal/domain/account"
	proxydomain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// TestLogicalAccountFamilySharesOneProxy 验证 Web 与关联 Build 账号统一继承代理绑定。
func TestLogicalAccountFamilySharesOneProxy(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	proxies := NewProxyRepository(database)
	proxyValue, err := proxies.Create(ctx, proxydomain.Endpoint{Name: "美国 01", Protocol: "socks5", Host: "127.0.0.1", Port: 1080, EncryptedURL: "encrypted-proxy", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	web, _, err := accounts.UpsertByIdentity(ctx, accountdomain.Credential{Provider: accountdomain.ProviderWeb, AuthType: accountdomain.AuthTypeSSO, Name: "web", SourceKey: "web-source", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive})
	if err != nil {
		t.Fatal(err)
	}
	build, _, err := accounts.UpsertByIdentity(ctx, accountdomain.Credential{Provider: accountdomain.ProviderBuild, AuthType: accountdomain.AuthTypeOAuth, Name: "build", SourceKey: "build-source", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.LinkWebToBuild(ctx, web.ID, build.ID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetFamilyProxy(ctx, web.FamilyID, &proxyValue.ID); err != nil {
		t.Fatal(err)
	}
	web, err = accounts.Get(ctx, web.ID)
	if err != nil {
		t.Fatal(err)
	}
	build, err = accounts.Get(ctx, build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if web.FamilyID == 0 || web.FamilyID != build.FamilyID || web.ProxyID == nil || build.ProxyID == nil || *web.ProxyID != proxyValue.ID || *build.ProxyID != proxyValue.ID {
		t.Fatalf("web=%#v build=%#v", web, build)
	}
	if err := proxies.Delete(ctx, proxyValue.ID); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("删除使用中代理错误 = %v", err)
	}
}

// TestDeletingLastAccountReleasesProxy 验证最后一个 Provider 凭据删除后会回收空账号组。
func TestDeletingLastAccountReleasesProxy(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	proxies := NewProxyRepository(database)
	proxyValue, err := proxies.Create(ctx, proxydomain.Endpoint{Name: "日本 01", Protocol: "http", Host: "127.0.0.1", Port: 8080, EncryptedURL: "encrypted-proxy", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := accounts.UpsertByIdentity(ctx, accountdomain.Credential{Provider: accountdomain.ProviderBuild, AuthType: accountdomain.AuthTypeOAuth, Name: "build", SourceKey: "single", EncryptedAccessToken: testEncryptedToken, AuthStatus: accountdomain.AuthStatusActive})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetFamilyProxy(ctx, value.FamilyID, &proxyValue.ID); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Delete(ctx, value.ID); err != nil {
		t.Fatal(err)
	}
	if err := proxies.Delete(ctx, proxyValue.ID); err != nil {
		t.Fatalf("删除已释放代理 = %v", err)
	}
}
