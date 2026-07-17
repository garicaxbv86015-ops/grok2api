package relational

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
)

// TestInitializeSchemaPreservesAccountChildrenWhenAddingFamilies 验证旧库新增逻辑账号组字段时不会级联删除账号子表。
func TestInitializeSchemaPreservesAccountChildrenWhenAddingFamilies(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "legacy-family.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.db.WithContext(ctx).AutoMigrate(schemaModels...); err != nil {
		t.Fatal(err)
	}

	repository := NewAccountRepository(database)
	now := time.Now().UTC()
	web, _, err := repository.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, WebTier: account.WebTierBasic,
		Name: "legacy-web", SourceKey: "legacy-web", EncryptedAccessToken: "encrypted-web",
		Enabled: true, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	build, _, err := repository.UpsertByIdentity(ctx, account.Credential{
		Provider: account.ProviderBuild, AuthType: account.AuthTypeOAuth,
		Name: "legacy-build", SourceKey: "legacy-build", EncryptedAccessToken: "encrypted-build",
		Enabled: true, AuthStatus: account.AuthStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.LinkWebToBuild(ctx, web.ID, build.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveQuotaWindows(ctx, web.ID, account.WebTierBasic, now, []account.QuotaWindow{{
		AccountID: web.ID, Mode: "chat", Remaining: 7, Total: 20, WindowSeconds: 3600,
		Source: account.QuotaSourceUpstream, SyncedAt: &now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveBilling(ctx, account.Billing{AccountID: build.ID, MonthlyLimit: 100, Used: 10, SyncedAt: now}); err != nil {
		t.Fatal(err)
	}

	childTables := []string{"account_credentials", "account_provider_links", "web_account_profiles", "account_quota_windows", "account_billing_snapshots"}
	before := make(map[string]int64, len(childTables))
	for _, table := range childTables {
		var count int64
		if err := database.db.WithContext(ctx).Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		before[table] = count
	}
	if err := downgradeProviderAccountsBeforeFamilyMigration(ctx, database); err != nil {
		t.Fatal(err)
	}

	if err := database.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range childTables {
		var after int64
		if err := database.db.WithContext(ctx).Table(table).Count(&after).Error; err != nil {
			t.Fatal(err)
		}
		if after != before[table] {
			t.Fatalf("table %s row count changed during family migration: before=%d after=%d", table, before[table], after)
		}
	}
	preservedWeb, err := repository.Get(ctx, web.ID)
	if err != nil {
		t.Fatal(err)
	}
	preservedBuild, err := repository.Get(ctx, build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preservedWeb.EncryptedAccessToken != "encrypted-web" || preservedBuild.EncryptedAccessToken != "encrypted-build" {
		t.Fatalf("account credentials were not preserved: web=%q build=%q", preservedWeb.EncryptedAccessToken, preservedBuild.EncryptedAccessToken)
	}
	if preservedWeb.FamilyID == 0 || preservedWeb.FamilyID != preservedBuild.FamilyID {
		t.Fatalf("linked accounts were not grouped: web_family=%d build_family=%d", preservedWeb.FamilyID, preservedBuild.FamilyID)
	}
}

// downgradeProviderAccountsBeforeFamilyMigration 将当前账号主表转换为新增 family_id 前的旧结构。
func downgradeProviderAccountsBeforeFamilyMigration(ctx context.Context, database *Database) error {
	return database.withSQLiteForeignKeysDisabled(ctx, func() error {
		statements := []string{
			`CREATE TABLE provider_accounts_legacy (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				identity_key TEXT NOT NULL,
				provider TEXT NOT NULL,
				name TEXT NOT NULL,
				email TEXT,
				user_id TEXT,
				team_id TEXT,
				source_key TEXT NOT NULL,
				enabled NUMERIC NOT NULL,
				auth_status TEXT NOT NULL,
				priority INTEGER NOT NULL DEFAULT 1,
				max_concurrent INTEGER NOT NULL DEFAULT 8,
				minimum_remaining REAL NOT NULL DEFAULT 0,
				failure_count INTEGER NOT NULL DEFAULT 0,
				cooldown_until DATETIME,
				last_error TEXT,
				last_used_at DATETIME,
				observed_model TEXT,
				observed_model_at DATETIME,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			)`,
			`INSERT INTO provider_accounts_legacy (id, identity_key, provider, name, email, user_id, team_id, source_key, enabled, auth_status, priority, max_concurrent, minimum_remaining, failure_count, cooldown_until, last_error, last_used_at, observed_model, observed_model_at, created_at, updated_at) SELECT id, identity_key, provider, name, email, user_id, team_id, source_key, enabled, auth_status, priority, max_concurrent, minimum_remaining, failure_count, cooldown_until, last_error, last_used_at, observed_model, observed_model_at, created_at, updated_at FROM provider_accounts`,
			`DROP TABLE provider_accounts`,
			`ALTER TABLE provider_accounts_legacy RENAME TO provider_accounts`,
			`DROP TABLE account_families`,
		}
		for _, statement := range statements {
			if err := database.db.WithContext(ctx).Exec(statement).Error; err != nil {
				return fmt.Errorf("降级账号主表: %w", err)
			}
		}
		return nil
	})
}
