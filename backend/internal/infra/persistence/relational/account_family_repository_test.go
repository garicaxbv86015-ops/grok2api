package relational

import (
	"context"
	"errors"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	proxydomain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// TestAccountRepositoryListsLogicalFamiliesAndSharedProxy 验证一个逻辑账号组会聚合三类成员并只绑定一次代理。
func TestAccountRepositoryListsLogicalFamiliesAndSharedProxy(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	web, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "family-web", Email: "family@example.com",
		SourceKey: "family-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	build, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, AuthType: account.AuthTypeOAuth, Name: "family-build",
		SourceKey: "family-build", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.LinkWebToBuild(ctx, web.ID, build.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		FamilyID: web.FamilyID, Provider: account.ProviderConsole, AuthType: account.AuthTypeSSO, Name: "family-console",
		SourceKey: "console-family-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	proxyValue, err := NewProxyRepository(database).Create(ctx, proxydomain.Endpoint{
		Name: "family-proxy", Protocol: "socks5", Host: "127.0.0.1", Port: 1080,
		EncryptedURL: "encrypted-proxy", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetFamilyProxy(ctx, web.FamilyID, &proxyValue.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "unbound-web", Email: "unbound@example.com",
		SourceKey: "unbound-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	}); err != nil {
		t.Fatal(err)
	}

	values, total, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{
		Page: repository.PageQuery{Limit: 20, Search: "family@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(values) != 1 {
		t.Fatalf("logical families total=%d values=%#v", total, values)
	}
	value := values[0]
	if value.ID != web.FamilyID || value.ProxyID == nil || *value.ProxyID != proxyValue.ID || value.ProxyName != "family-proxy" || !value.ProxyEnabled {
		t.Fatalf("logical family proxy=%#v", value)
	}
	if len(value.Members) != 3 || value.Members[0].Provider != account.ProviderWeb || value.Members[1].Provider != account.ProviderBuild || value.Members[2].Provider != account.ProviderConsole {
		t.Fatalf("logical family members=%#v", value.Members)
	}
	proxyMatches, proxyTotal, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{
		Page: repository.PageQuery{Limit: 20, Search: "family-pro"},
	})
	if err != nil || proxyTotal != 1 || len(proxyMatches) != 1 || proxyMatches[0].ID != web.FamilyID {
		t.Fatalf("proxy name search total=%d values=%#v err=%v", proxyTotal, proxyMatches, err)
	}
	boundValues, boundTotal, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{
		Page: repository.PageQuery{Limit: 20}, Filter: repository.AccountFamilyListFilter{ProxyBinding: "bound"},
	})
	if err != nil || boundTotal != 1 || len(boundValues) != 1 || boundValues[0].ID != web.FamilyID {
		t.Fatalf("bound filter total=%d values=%#v err=%v", boundTotal, boundValues, err)
	}
	unboundValues, unboundTotal, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{
		Page: repository.PageQuery{Limit: 20, Search: "unbound-web"}, Filter: repository.AccountFamilyListFilter{ProxyBinding: "unbound"},
	})
	if err != nil || unboundTotal != 1 || len(unboundValues) != 1 || unboundValues[0].ProxyID != nil {
		t.Fatalf("unbound filter total=%d values=%#v err=%v", unboundTotal, unboundValues, err)
	}
}

// TestAccountRepositoryBatchUpdatesFamilyProxyAtomically 验证批量绑定的原子性；t 为测试上下文；无返回值。
func TestAccountRepositoryBatchUpdatesFamilyProxyAtomically(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	first, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "batch-first",
		SourceKey: "batch-first", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "batch-second",
		SourceKey: "batch-second", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxyValue, err := NewProxyRepository(database).Create(ctx, proxydomain.Endpoint{
		Name: "batch-proxy", Protocol: "socks5", Host: "127.0.0.1", Port: 1081,
		EncryptedURL: "encrypted-batch-proxy", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := accounts.SetFamilyProxies(ctx, []uint64{first.FamilyID, second.FamilyID}, &proxyValue.ID)
	if err != nil || updated != 2 {
		t.Fatalf("batch bind updated=%d err=%v", updated, err)
	}
	if _, err := accounts.SetFamilyProxies(ctx, []uint64{first.FamilyID, 999999}, nil); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing family error=%v", err)
	}
	values, total, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{
		Page: repository.PageQuery{Limit: 20}, Filter: repository.AccountFamilyListFilter{ProxyBinding: "bound"},
	})
	if err != nil || total != 2 || len(values) != 2 {
		t.Fatalf("atomic rollback total=%d values=%#v err=%v", total, values, err)
	}
	for _, value := range values {
		if value.ProxyID == nil || *value.ProxyID != proxyValue.ID {
			t.Fatalf("family lost proxy after rollback: %#v", value)
		}
	}
}
