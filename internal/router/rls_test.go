package router

import (
	"context"
	"counter/internal/cache"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/migrations"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/testutil"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Intentionally omit tenant predicates: these checks exercise PostgreSQL's
// policies independently of the filters in our stores and HTTP authorization.
func TestRLSRejectsUnscopedAndCrossTenantSQL(t *testing.T) {
	owner, runtime := testutil.OpenRestrictedPostgres(t)
	runtime.SetMaxOpenConns(1)
	runtime.SetMaxIdleConns(1)
	a := createTestTenant(t, owner, "rls-a")
	b := createTestTenant(t, owner, "rls-b")
	ca := createTestCounter(t, owner, a, "a", 1)
	cb := createTestCounter(t, owner, b, "b", 2)
	for _, pair := range [][2]string{{a, ca}, {b, cb}} {
		if _, err := owner.Exec(`INSERT INTO counter_metadata (tenant_id,counter_id,data) VALUES ($1,$2,'null')`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	ctxA := database.WithAccess(context.Background(), database.Access{TenantID: a, PrincipalID: "a"})
	ctxB := database.WithAccess(context.Background(), database.Access{TenantID: b, PrincipalID: "b"})
	count := func(ctx context.Context, table string, want int) {
		t.Helper()
		var got int
		if err := database.ExecutorFor(ctx, runtime).GetContext(ctx, &got, "SELECT count(*) FROM "+table); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s rows=%d, want %d", table, got, want)
		}
	}
	for _, table := range []string{"tenants", "counters", "counter_operations", "counter_metadata", "api_credentials", "auth_audit_events", "idempotency_requests"} {
		count(context.Background(), table, 0)
	}
	for _, ctx := range []context.Context{ctxA, ctxB, ctxA} {
		count(ctx, "tenants", 1)
		count(ctx, "counters", 1)
		count(ctx, "counter_metadata", 1)
		count(context.Background(), "counters", 0)
	}
	requireCode := func(err error, code string) {
		t.Helper()
		var pg *pq.Error
		if !errors.As(err, &pg) || string(pg.Code) != code {
			t.Fatalf("error=%v, want SQLSTATE %s", err, code)
		}
	}
	_, err := database.ExecutorFor(context.Background(), runtime).ExecContext(context.Background(), `INSERT INTO tenants (id,label) VALUES ($1,'denied')`, uuid.NewString())
	requireCode(err, "42501")
	_, err = database.ExecutorFor(ctxA, runtime).ExecContext(ctxA, `INSERT INTO counters (id,tenant_id,label,value) VALUES ($1,$2,'forged',0)`, uuid.NewString(), b)
	requireCode(err, "42501")
	_, err = database.ExecutorFor(ctxA, runtime).ExecContext(ctxA, `UPDATE counters SET tenant_id=$1`, b)
	requireCode(err, "42501")
	// FK validation must reject a valid counter ID from a different tenant.
	_, err = database.ExecutorFor(ctxA, runtime).ExecContext(ctxA, `INSERT INTO counter_metadata (tenant_id,counter_id,data) VALUES ($1,$2,'{}')`, a, cb)
	requireCode(err, "23503")
	result, err := database.ExecutorFor(ctxA, runtime).ExecContext(ctxA, `DELETE FROM counter_metadata WHERE counter_id=$1`, cb)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := result.RowsAffected(); n != 0 {
		t.Fatal("cross-tenant delete succeeded")
	}
	count(ctxB, "counter_metadata", 1)
	// Rollback clears context before the same physical connection is reused.
	tx, finish, err := database.BeginWrite(ctxA, runtime)
	if err != nil {
		t.Fatal(err)
	}
	var got int
	if err := tx.GetContext(ctxA, &got, `SELECT count(*) FROM counters`); err != nil || got != 1 {
		t.Fatalf("scoped tx: %d %v", got, err)
	}
	if err := finish(false); err != nil {
		t.Fatal(err)
	}
	count(context.Background(), "counters", 0)
	admin := database.WithAccess(context.Background(), database.Access{Admin: true, PrincipalID: "admin"})
	count(admin, "counters", 2)
	targetedAdmin := database.WithAccess(context.Background(), database.Access{Admin: true, TenantID: a, PrincipalID: "admin"})
	count(targetedAdmin, "counters", 1)
	if err := database.VerifyRuntimeRole(context.Background(), owner); err == nil {
		t.Fatal("privileged owner accepted as runtime")
	}
	if err := database.VerifyRuntimeRole(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	var runtimeRole, ownerRole string
	if err := runtime.Get(&runtimeRole, `SELECT quote_ident(current_user)`); err != nil {
		t.Fatal(err)
	}
	if err := owner.Get(&ownerRole, `SELECT quote_ident(current_user)`); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`ALTER TABLE counter_metadata OWNER TO ` + runtimeRole); err != nil {
		t.Fatal(err)
	}
	if err := database.VerifyRuntimeRole(context.Background(), runtime); err == nil {
		t.Error("table-owning runtime role accepted")
	}
	if _, err := owner.Exec(`ALTER TABLE counter_metadata OWNER TO ` + ownerRole); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(`GRANT SELECT, INSERT, UPDATE, DELETE ON counter_metadata TO ` + runtimeRole); err != nil {
		t.Fatal(err)
	}
	// Counter deletion cleans up only its own optional document.
	if _, err := owner.Exec(`DELETE FROM counters WHERE id=$1`, ca); err != nil {
		t.Fatal(err)
	}
	count(ctxA, "counter_metadata", 0)
	count(ctxB, "counter_metadata", 1)
}

func TestRLSMigrationRoundTripPreservesExistingReplays(t *testing.T) {
	owner, runtime := testutil.OpenRestrictedPostgres(t)
	replica := idempotencyReplica(runtime)
	tenant := createTestTenant(t, owner, "rls-migration")
	path := "/v2/tenants/" + tenant + "/counters"
	key := uuid.NewString()
	first := callMutation(replica, path, key, `{"label":"before-upgrade"}`, "test-key")
	if first.status != 201 {
		t.Fatalf("initial create: %d %s", first.status, first.body)
	}
	for range 2 {
		if err := migrations.RunDown(owner); err != nil {
			t.Fatal(err)
		}
	}
	var version int
	if err := owner.Get(&version, `SELECT max(version) FROM schema_migrations`); err != nil || version != 8 {
		t.Fatalf("rollback version=%d error=%v", version, err)
	}
	if err := migrations.RunUp(owner); err != nil {
		t.Fatal(err)
	}
	retry := callMutation(replica, path, key, `{"label":"before-upgrade"}`, "test-key")
	if retry.status != 201 || retry.replayed != "true" || retry.body != first.body {
		t.Fatalf("existing replay not preserved: %+v", retry)
	}
	var savedTenant, principal string
	if err := owner.QueryRow(`SELECT tenant_id,principal_id FROM idempotency_requests WHERE request_key=$1`, key).Scan(&savedTenant, &principal); err != nil {
		t.Fatal(err)
	}
	if savedTenant != tenant || principal != "legacy-env-admin" {
		t.Fatalf("incorrect replay backfill: %s %s", savedTenant, principal)
	}
}

func TestCachedCounterReadDoesNotAcquireRLSConnection(t *testing.T) {
	owner, runtime := testutil.OpenRestrictedPostgres(t)
	runtime.SetMaxOpenConns(1)
	tenant := createTestTenant(t, owner, "rls-cache")
	counter := createTestCounter(t, owner, tenant, "cached", 5)
	readCache, err := cache.NewMemoryCounterReadCache(10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	replica := NewRouterWithObservability(runtime, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(1000, 3, 60), "test-key", true, middleware.NewDefaultLogger("error"), nil, middleware.DefaultRouteTimeouts(), 128, OperationalOptions{CounterReadCache: readCache})
	path := "/v2/tenants/" + tenant + "/counters/" + counter
	if got := metadataRequest(replica, "GET", path, "test-key", "", "", "", ""); got.status != 200 {
		t.Fatalf("warm cache status=%d", got.status)
	}
	conn, err := runtime.Connx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	result := make(chan metadataResponse, 1)
	go func() { result <- metadataRequest(replica, "GET", path, "test-key", "", "", "", "") }()
	select {
	case got := <-result:
		if got.status != 200 {
			t.Fatalf("cached status=%d", got.status)
		}
	case <-time.After(time.Second):
		_ = conn.Close()
		<-result
		t.Fatal("cached counter read waited for a database connection")
	}
}

func TestRLSCredentialLookupAndReplayPrincipalIsolation(t *testing.T) {
	owner, runtime := testutil.OpenRestrictedPostgres(t)
	a := createTestTenant(t, owner, "rls-credentials")
	credentials := service.NewCredentialService(store.NewCredentialStore(owner))
	first, err := credentials.Create(context.Background(), a, nil, nil, "setup")
	if err != nil {
		t.Fatal(err)
	}
	second, err := credentials.Create(context.Background(), a, nil, nil, "setup")
	if err != nil {
		t.Fatal(err)
	}
	lookup := database.WithAccess(context.Background(), database.Access{AuthenticationID: first.CredentialID})
	var ids []string
	if err := database.ExecutorFor(lookup, runtime).SelectContext(lookup, &ids, `SELECT id FROM api_credentials`); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != first.CredentialID {
		t.Fatalf("lookup exposed credentials: %v", ids)
	}
	var count int
	if err := database.ExecutorFor(lookup, runtime).GetContext(lookup, &count, `SELECT count(*) FROM tenants`); err != nil || count != 0 {
		t.Fatalf("lookup exposed tenants: %d %v", count, err)
	}
	result, err := database.ExecutorFor(lookup, runtime).ExecContext(lookup, `UPDATE api_credentials SET revoked_at=NOW()`)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := result.RowsAffected(); n != 0 {
		t.Fatal("lookup granted credential writes")
	}
	for _, principal := range []string{first.CredentialID, second.CredentialID} {
		if _, err := owner.Exec(`INSERT INTO idempotency_requests (scope,request_key,request_hash,status_code,content_type,response_body,tenant_id,principal_id) VALUES ($1,$2,'hash',200,'application/json','{}',$3,$4)`, principal+":POST:/v2/tenants/"+a+"/counters", uuid.NewString(), a, principal); err != nil {
			t.Fatal(err)
		}
	}
	for _, principal := range []string{first.CredentialID, second.CredentialID} {
		ctx := database.WithAccess(context.Background(), database.Access{TenantID: a, PrincipalID: principal})
		var principals []string
		if err := database.ExecutorFor(ctx, runtime).SelectContext(ctx, &principals, `SELECT principal_id FROM idempotency_requests`); err != nil {
			t.Fatal(err)
		}
		if len(principals) != 1 || principals[0] != principal {
			t.Fatalf("replay rows escaped principal scope: %v", principals)
		}
		if err := database.ExecutorFor(ctx, runtime).GetContext(ctx, &count, `SELECT count(*) FROM api_credentials`); err != nil || count != 0 {
			t.Fatalf("tenant context exposed verifiers: %d %v", count, err)
		}
	}
}
