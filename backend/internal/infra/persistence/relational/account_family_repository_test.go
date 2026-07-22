package relational

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	mediadomain "github.com/chenyme/grok2api/backend/internal/domain/media"
	proxydomain "github.com/chenyme/grok2api/backend/internal/domain/proxy"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// TestAccountRepositoryDeletesLogicalFamilyWithAllMembers 验证删除逻辑账号组会清理全部成员及其子数据，同时保留代理和其他账号组；参数 t 为测试上下文；无返回值。
func TestAccountRepositoryDeletesLogicalFamilyWithAllMembers(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	models := NewModelRepository(database)
	proxies := NewProxyRepository(database)
	web, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "delete-family-web",
		SourceKey: "delete-family-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	build, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, AuthType: account.AuthTypeOAuth, Name: "delete-family-build",
		SourceKey: "delete-family-build", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.LinkWebToBuild(ctx, web.ID, build.ID); err != nil {
		t.Fatal(err)
	}
	console, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		FamilyID: web.FamilyID, Provider: account.ProviderConsole, AuthType: account.AuthTypeSSO, Name: "delete-family-console",
		SourceKey: "delete-family-console", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxyValue, err := proxies.Create(ctx, proxydomain.Endpoint{
		Name: "delete-family-proxy", Protocol: "socks5", Host: "127.0.0.1", Port: 1082,
		EncryptedURL: "encrypted-delete-family-proxy", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetFamilyProxy(ctx, web.FamilyID, &proxyValue.ID); err != nil {
		t.Fatal(err)
	}
	peer, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "delete-family-peer",
		SourceKey: "delete-family-peer", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := accounts.SaveBilling(ctx, account.Billing{AccountID: build.ID, SyncedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := accounts.SaveQuotaRecovery(ctx, account.QuotaRecovery{
		AccountID: web.ID, Kind: account.QuotaRecoveryKindFree, Status: account.QuotaRecoveryStatusExhausted, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := models.ReplaceAccountCapabilities(ctx, console.ID, []string{"grok-delete-family"}, now); err != nil {
		t.Fatal(err)
	}
	clientKey := clientKeyModel{
		Name: "delete-family-key", Prefix: "delete-family-key", SecretHash: testSecretHash,
		EncryptedSecret: testEncryptedToken, Enabled: true, RPMLimit: 60, MaxConcurrent: 4,
	}
	if err := database.db.WithContext(ctx).Create(&clientKey).Error; err != nil {
		t.Fatal(err)
	}
	asset := mediadomain.Asset{
		ID: "delete_family_asset_0001", Kind: "video", StorageKey: "video/delete-family.mp4",
		MIMEType: "video/mp4", SizeBytes: 1024, SHA256: testSecretHash, CreatedAt: now,
	}
	if err := NewMediaAssetRepository(database).CreateMediaAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}
	job := mediadomain.Job{
		ID: "delete_family_job_0001", RequestID: "delete-family-request", ClientKeyID: clientKey.ID, ClientKeyName: clientKey.Name,
		AccountID: web.ID, AccountName: web.Name, Provider: string(account.ProviderWeb), Model: "grok-imagine-video",
		ModelRouteID: 1, UpstreamModel: "grok-imagine-video", Prompt: "delete family", Seconds: 8, Size: "16:9",
		Quality: "720p", Status: mediadomain.StatusQueued, InputJSON: `{}`, ResultAssetID: asset.ID, CreatedAt: now, UpdatedAt: now,
	}
	mediaJobs := NewMediaJobRepository(database)
	if err := mediaJobs.CreateMediaJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	tickets := NewMediaUploadTicketRepository(database)
	if err := tickets.CreateUploadTicket(ctx, repository.MediaUploadTicket{
		TokenHash: testSecretHash, AssetID: asset.ID, JobID: job.ID, MaxBytes: 1024,
		AllowedMIME: "video/mp4", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	deletedIDs, err := accounts.DeleteFamily(ctx, web.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedIDs) != 3 || deletedIDs[0] != web.ID || deletedIDs[1] != build.ID || deletedIDs[2] != console.ID {
		t.Fatalf("deleted account ids = %#v", deletedIDs)
	}
	for _, id := range deletedIDs {
		if _, err := accounts.Get(ctx, id); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("deleted account %d error = %v", id, err)
		}
	}
	if _, err := accounts.Get(ctx, peer.ID); err != nil {
		t.Fatalf("peer account was affected: %v", err)
	}
	if _, err := proxies.Get(ctx, proxyValue.ID); err != nil {
		t.Fatalf("bound proxy was deleted: %v", err)
	}
	for _, table := range []string{"account_billing_snapshots", "account_quota_recovery", "account_model_capabilities", "account_model_sync_states", "account_provider_links"} {
		if count := tableRowCount(t, database, table); count != 0 {
			t.Fatalf("%s rows after family delete = %d", table, count)
		}
	}
	if _, err := mediaJobs.GetMediaJob(ctx, job.ID, clientKey.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("media job error after family delete = %v", err)
	}
	if _, err := tickets.GetUploadTicketByHash(ctx, testSecretHash); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("media upload ticket error after family delete = %v", err)
	}
	if _, err := NewMediaAssetRepository(database).GetMediaAsset(ctx, asset.ID); err != nil {
		t.Fatalf("generated media asset was deleted: %v", err)
	}
	families, total, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{Page: repository.PageQuery{Limit: 20, Search: "delete-family-peer"}})
	if err != nil || total != 1 || len(families) != 1 || families[0].ID != peer.FamilyID {
		t.Fatalf("remaining families total=%d values=%#v err=%v", total, families, err)
	}
	deletedFamilies, deletedTotal, err := accounts.ListFamilies(ctx, repository.AccountFamilyListQuery{Page: repository.PageQuery{Limit: 20, Search: "delete-family-web"}})
	if err != nil || deletedTotal != 0 || len(deletedFamilies) != 0 {
		t.Fatalf("deleted families total=%d values=%#v err=%v", deletedTotal, deletedFamilies, err)
	}
	if _, err := accounts.DeleteFamily(ctx, web.FamilyID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing family error = %v", err)
	}
}

// TestAccountRepositoryCleansFamiliesWithoutBuild 验证一键清理只删除不含 Build 的组，并联动删除 Web、Console 成员。
// 参数 t 为测试上下文；无返回值。
func TestAccountRepositoryCleansFamiliesWithoutBuild(t *testing.T) {
	ctx := context.Background()
	database := openTestDatabase(t)
	accounts := NewAccountRepository(database)
	missingWeb, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "cleanup-missing-web",
		SourceKey: "cleanup-missing-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	missingConsole, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		FamilyID: missingWeb.FamilyID, Provider: account.ProviderConsole, AuthType: account.AuthTypeSSO, Name: "cleanup-missing-console",
		SourceKey: "cleanup-missing-console", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	keptWeb, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, Name: "cleanup-kept-web",
		SourceKey: "cleanup-kept-web", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	keptBuild, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, AuthType: account.AuthTypeOAuth, Name: "cleanup-kept-build",
		SourceKey: "cleanup-kept-build", EncryptedAccessToken: testEncryptedToken, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.LinkWebToBuild(ctx, keptWeb.ID, keptBuild.ID); err != nil {
		t.Fatal(err)
	}

	result, err := accounts.DeleteFamiliesWithoutBuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FamilyIDs) != 1 || result.FamilyIDs[0] != missingWeb.FamilyID {
		t.Fatalf("deleted family ids = %#v", result.FamilyIDs)
	}
	if len(result.AccountIDs) != 2 {
		t.Fatalf("deleted account ids = %#v", result.AccountIDs)
	}
	for _, id := range []uint64{missingWeb.ID, missingConsole.ID} {
		if _, err := accounts.Get(ctx, id); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("missing-build member %d error = %v", id, err)
		}
	}
	for _, id := range []uint64{keptWeb.ID, keptBuild.ID} {
		if _, err := accounts.Get(ctx, id); err != nil {
			t.Fatalf("Build family member %d was removed: %v", id, err)
		}
	}
}

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
