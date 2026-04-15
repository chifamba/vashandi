package services

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAccessServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file::memory:?cache=shared&%s=1", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS principal_permission_grants")
	db.Exec("DROP TABLE IF EXISTS company_memberships")
	db.Exec("DROP TABLE IF EXISTS instance_user_roles")

	db.Exec(`CREATE TABLE company_memberships (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		principal_type text NOT NULL,
		principal_id text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		membership_role text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE principal_permission_grants (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		principal_type text NOT NULL,
		principal_id text NOT NULL,
		permission_key text NOT NULL,
		scope text,
		granted_by_user_id text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE instance_user_roles (
		id text PRIMARY KEY,
		user_id text NOT NULL,
		role text NOT NULL DEFAULT 'instance_admin',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)

	return db
}

func TestAccessService_GetMembership_CompanyScoped(t *testing.T) {
	db := setupAccessServiceTestDB(t)
	svc := NewAccessService(db)

	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status, membership_role) VALUES ('m-a', 'comp-a', 'user', 'user-1', 'active', 'member')")
	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status, membership_role) VALUES ('m-b', 'comp-b', 'user', 'user-1', 'active', 'owner')")

	membership, err := svc.GetMembership(context.Background(), "comp-a", "user", "user-1")
	if err != nil {
		t.Fatalf("GetMembership failed: %v", err)
	}
	if membership == nil {
		t.Fatal("expected membership")
	}
	if membership.ID != "m-a" {
		t.Fatalf("expected comp-a membership, got %q", membership.ID)
	}

	missing, err := svc.GetMembership(context.Background(), "comp-c", "user", "user-1")
	if err != nil {
		t.Fatalf("GetMembership missing failed: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil membership for unknown company, got %+v", missing)
	}
}

func TestAccessService_HasPermission_WithExplicitGrant(t *testing.T) {
	db := setupAccessServiceTestDB(t)
	svc := NewAccessService(db)

	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status, membership_role) VALUES ('m-1', 'comp-a', 'user', 'user-1', 'active', 'member')")
	db.Exec("INSERT INTO principal_permission_grants (id, company_id, principal_type, principal_id, permission_key) VALUES ('g-1', 'comp-a', 'user', 'user-1', 'agents:create')")

	allowed, err := svc.HasPermission(context.Background(), "comp-a", "user", "user-1", "agents:create")
	if err != nil {
		t.Fatalf("HasPermission failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected explicit grant to allow permission")
	}
}

func TestAccessService_HasPermission_RequiresActiveMembership(t *testing.T) {
	db := setupAccessServiceTestDB(t)
	svc := NewAccessService(db)

	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status, membership_role) VALUES ('m-1', 'comp-a', 'user', 'user-1', 'suspended', 'member')")
	db.Exec("INSERT INTO principal_permission_grants (id, company_id, principal_type, principal_id, permission_key) VALUES ('g-1', 'comp-a', 'user', 'user-1', 'agents:create')")

	allowed, err := svc.HasPermission(context.Background(), "comp-a", "user", "user-1", "agents:create")
	if err != nil {
		t.Fatalf("HasPermission failed: %v", err)
	}
	if allowed {
		t.Fatal("expected inactive membership to deny permission")
	}
}

func TestAccessService_CanUser_InstanceAdminBypass(t *testing.T) {
	db := setupAccessServiceTestDB(t)
	svc := NewAccessService(db)

	db.Exec("INSERT INTO instance_user_roles (id, user_id, role) VALUES ('r-1', 'admin-1', 'instance_admin')")

	allowed, err := svc.CanUser(context.Background(), "comp-a", "admin-1", "agents:create")
	if err != nil {
		t.Fatalf("CanUser failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected instance admin to bypass company grants")
	}
}

func TestAccessService_EnsureMembership_UpsertsMembershipState(t *testing.T) {
	db := setupAccessServiceTestDB(t)
	svc := NewAccessService(db)

	owner := "owner"
	member, err := svc.EnsureMembership(context.Background(), "comp-a", "user", "user-1", &owner, "active")
	if err != nil {
		t.Fatalf("EnsureMembership create failed: %v", err)
	}
	if member == nil || member.MembershipRole == nil || *member.MembershipRole != "owner" {
		t.Fatalf("expected created owner membership, got %+v", member)
	}

	memberRole := "member"
	updated, err := svc.EnsureMembership(context.Background(), "comp-a", "user", "user-1", &memberRole, "suspended")
	if err != nil {
		t.Fatalf("EnsureMembership update failed: %v", err)
	}
	if updated == nil || updated.Status != "suspended" {
		t.Fatalf("expected updated suspended membership, got %+v", updated)
	}
	if updated.MembershipRole == nil || *updated.MembershipRole != "member" {
		t.Fatalf("expected updated member role, got %+v", updated)
	}

	var count int64
	if err := db.Table("company_memberships").Where("company_id = ? AND principal_id = ?", "comp-a", "user-1").Count(&count).Error; err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one upserted membership row, got %d", count)
	}
}
