package router

import (
	"context"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/testutil"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type mutationResponse struct {
	status   int
	body     string
	replayed string
}

func idempotencyReplica(db *database.DB) *Router {
	return NewRouter(db, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(10000, 3, 60), "test-key", middleware.NewDefaultLogger("error"), nil)
}

func callMutation(replica *Router, path, key, body, apiKey string) mutationResponse {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.SetRequestURI(path)
	ctx.Request.Header.SetContentType("application/json")
	ctx.Request.Header.Set("X-API-Key", apiKey)
	if key != "" {
		ctx.Request.Header.Set("Idempotency-Key", key)
	}
	ctx.Request.SetBodyString(body)
	replica.ServeHTTP(ctx)
	return mutationResponse{ctx.Response.StatusCode(), string(ctx.Response.Body()), string(ctx.Response.Header.Peek("Idempotency-Replayed"))}
}

func responseFields(t *testing.T, response mutationResponse) map[string]interface{} {
	t.Helper()
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(response.body), &fields); err != nil {
		t.Fatalf("decode response status %d: %v", response.status, err)
	}
	return fields
}

func TestOptionalIdempotencyForEveryNewMutationRoute(t *testing.T) {
	db, runtime := testutil.OpenRestrictedPostgres(t)
	// A single available connection catches accidental nested transactions or
	// reads outside the outer transaction while it holds the only connection.
	runtime.SetMaxOpenConns(1)
	replicas := []*Router{idempotencyReplica(runtime), idempotencyReplica(runtime)}
	tenantID := createTestTenant(t, db, "idempotency-routes")
	credentialPath := "/v2/tenants/" + tenantID + "/credentials"
	source := callMutation(replicas[0], credentialPath, "", `{}`, "test-key")
	if source.status != fasthttp.StatusCreated {
		t.Fatalf("create source credential: %d", source.status)
	}
	sourceID := responseFields(t, source)["credential_id"].(string)
	cases := []struct {
		name       string
		path       string
		body       string
		status     int
		credential bool
	}{
		{"tenant", "/v2/tenants", `{"label":"keyed-tenant"}`, 201, false},
		{"counter", "/v2/tenants/" + tenantID + "/counters", `{"label":"keyed-counter","initial_value":7}`, 201, false},
		{"tenant credential", credentialPath, `{}`, 201, true},
		{"admin credential", "/v2/admin/credentials", `{}`, 201, true},
		{"rotation", credentialPath + "/" + sourceID + "/rotate", "", 201, true},
		{"revocation", credentialPath + "/" + sourceID + "/revoke", "", 204, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := uuid.NewString()
			first := callMutation(replicas[0], tc.path, key, tc.body, "test-key")
			if first.status != tc.status || first.replayed != "false" {
				t.Fatalf("first status = %d, replayed = %q", first.status, first.replayed)
			}
			retry := callMutation(replicas[1], tc.path, key, tc.body, "test-key")
			if retry.status != first.status || retry.replayed != "true" {
				t.Fatalf("retry status = %d, replayed = %q", retry.status, retry.replayed)
			}
			if tc.credential {
				firstFields, retryFields := responseFields(t, first), responseFields(t, retry)
				if firstFields["api_key"] == nil || firstFields["credential_id"] != retryFields["credential_id"] {
					t.Fatal("credential was not issued once with a stable replay ID")
				}
				if _, present := retryFields["api_key"]; present {
					t.Fatal("credential secret appeared in replay")
				}
				var savedBody []byte
				if err := db.Get(&savedBody, "SELECT response_body FROM idempotency_requests WHERE request_key = $1", key); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(savedBody), "api_key") || strings.Contains(string(savedBody), firstFields["api_key"].(string)) {
					t.Fatal("credential secret was persisted in replay storage")
				}
			} else if first.body != retry.body {
				t.Fatal("replay changed the original response")
			}
			conflict := callMutation(replicas[1], tc.path, key, `{"changed":true}`, "test-key")
			if conflict.status != 409 || !strings.Contains(conflict.body, "IDEMPOTENCY_KEY_REUSED") {
				t.Fatalf("changed payload did not conflict: %d", conflict.status)
			}
		})
	}
	var counterCount, initialCount, credentialCount, auditCount int
	for query, result := range map[string]*int{
		"SELECT COUNT(*) FROM counters":           &counterCount,
		"SELECT COUNT(*) FROM counter_operations": &initialCount,
		"SELECT COUNT(*) FROM api_credentials":    &credentialCount,
		"SELECT COUNT(*) FROM auth_audit_events":  &auditCount,
	} {
		if err := db.Get(result, query); err != nil {
			t.Fatal(err)
		}
	}
	if counterCount != 1 || initialCount != 1 || credentialCount != 4 || auditCount != 5 {
		t.Fatalf("unexpected effects: counters=%d initial=%d credentials=%d audits=%d", counterCount, initialCount, credentialCount, auditCount)
	}
}

func TestConcurrentIdempotencyAcrossReplicas(t *testing.T) {
	db := testutil.OpenPostgres(t)
	replicas := []*Router{idempotencyReplica(db), idempotencyReplica(db), idempotencyReplica(db)}
	tenantID := createTestTenant(t, db, "concurrent-idempotency")
	for _, tc := range []struct {
		name, path, body, idField string
		credential                bool
	}{
		{"counter", "/v2/tenants/" + tenantID + "/counters", `{"label":"concurrent","initial_value":11}`, "counter_id", false},
		{"credential", "/v2/tenants/" + tenantID + "/credentials", `{}`, "credential_id", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := uuid.NewString()
			const requests = 16
			results := make([]mutationResponse, requests)
			var group sync.WaitGroup
			start := make(chan struct{})
			for i := range results {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					<-start
					results[i] = callMutation(replicas[i%len(replicas)], tc.path, key, tc.body, "test-key")
				}(i)
			}
			close(start)
			group.Wait()
			var id string
			firstCount, secretCount := 0, 0
			for _, result := range results {
				if result.status != 201 {
					t.Fatalf("concurrent status = %d", result.status)
				}
				fields := responseFields(t, result)
				if id == "" {
					id = fields[tc.idField].(string)
				}
				if fields[tc.idField] != id {
					t.Fatal("concurrent retry created a different resource")
				}
				if result.replayed == "false" {
					firstCount++
				}
				if _, present := fields["api_key"]; present {
					secretCount++
				}
			}
			if firstCount != 1 || tc.credential && secretCount != 1 {
				t.Fatalf("first responses = %d, secrets = %d", firstCount, secretCount)
			}
		})
	}
	var audits int
	if err := db.Get(&audits, "SELECT COUNT(*) FROM auth_audit_events"); err != nil || audits != 1 {
		t.Fatalf("credential audit count = %d, error = %v", audits, err)
	}
}

func TestIdempotencyCanonicalPayloadFailuresAndExpiry(t *testing.T) {
	db := testutil.OpenPostgres(t)
	replica := idempotencyReplica(db)
	tenantID := createTestTenant(t, db, "canonical-idempotency")
	path := "/v2/tenants/" + tenantID + "/counters"
	key := uuid.NewString()
	if response := callMutation(replica, path, key, `{"max_delta":-1}`, "test-key"); response.status != 400 {
		t.Fatalf("invalid request status = %d", response.status)
	}
	first := callMutation(replica, path, key, `{"label":"canonical","initial_value":9}`, "test-key")
	retry := callMutation(replica, path, key, " { \"initial_value\":9, \"label\":\"canonical\" } ", "test-key")
	if first.status != 201 || retry.replayed != "true" || first.body != retry.body {
		t.Fatal("failed request reserved key or JSON formatting changed replay")
	}
	invalid := callMutation(replica, path, "invalid-key", `{}`, "test-key")
	if invalid.status != 400 || !strings.Contains(invalid.body, "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("invalid key status = %d", invalid.status)
	}
	credentialPath := "/v2/tenants/" + tenantID + "/credentials"
	key = uuid.NewString()
	issued := callMutation(replica, credentialPath, key, `{}`, "test-key")
	if _, err := db.Exec("UPDATE idempotency_requests SET expires_at = NOW() - INTERVAL '1 second' WHERE request_key = $1", key); err != nil {
		t.Fatal(err)
	}
	afterExpiry := callMutation(replica, credentialPath, key, `{}`, "test-key")
	if afterExpiry.status != 201 || afterExpiry.replayed != "false" || responseFields(t, issued)["credential_id"] == responseFields(t, afterExpiry)["credential_id"] {
		t.Fatal("expired key did not permit a new request")
	}
}

func TestIdempotencyRecordFailureRollsBackMutation(t *testing.T) {
	db := testutil.OpenPostgres(t)
	replica := idempotencyReplica(db)
	if _, err := db.Exec("ALTER TABLE idempotency_requests ADD CONSTRAINT reject_replay CHECK (false)"); err != nil {
		t.Fatal(err)
	}
	tenantID := createTestTenant(t, db, "rollback-idempotency")
	for _, tc := range []struct{ path, body, table string }{
		{"/v2/tenants", `{"label":"rolled-back"}`, "tenants WHERE label = 'rolled-back'"},
		{"/v2/tenants/" + tenantID + "/counters", `{"label":"rolled-back"}`, "counters"},
		{"/v2/tenants/" + tenantID + "/credentials", `{}`, "api_credentials"},
	} {
		response := callMutation(replica, tc.path, uuid.NewString(), tc.body, "test-key")
		if response.status != 503 || strings.Contains(response.body, "api_key") {
			t.Fatalf("storage failure returned status %d or a secret", response.status)
		}
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM "+tc.table); err != nil || count != 0 {
			t.Fatalf("mutation committed without replay record: %s count=%d error=%v", tc.table, count, err)
		}
	}
	for _, table := range []string{"counter_operations", "auth_audit_events"} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil || count != 0 {
			t.Fatalf("dependent writes survived rollback: %s count=%d error=%v", table, count, err)
		}
	}
}

func TestIdempotencyAuthorizationScopeAndOptionalKeys(t *testing.T) {
	db := testutil.OpenPostgres(t)
	replica := idempotencyReplica(db)
	tenantID := createTestTenant(t, db, "authorization-idempotency")
	credentialPath := "/v2/tenants/" + tenantID + "/credentials"
	// Omitting the header still issues distinct credentials with secrets.
	first := callMutation(replica, credentialPath, "", `{}`, "test-key")
	second := callMutation(replica, credentialPath, "", `{}`, "test-key")
	firstFields, secondFields := responseFields(t, first), responseFields(t, second)
	if first.status != 201 || second.status != 201 || first.replayed != "" || firstFields["credential_id"] == secondFields["credential_id"] {
		t.Fatal("unkeyed creation behavior changed")
	}
	key := uuid.NewString()
	created := callMutation(replica, "/v2/tenants", key, `{"label":"admin-only"}`, "test-key")
	denied := callMutation(replica, "/v2/tenants", key, `{"label":"admin-only"}`, firstFields["api_key"].(string))
	if created.status != 201 || denied.status != 403 || denied.replayed != "" {
		t.Fatalf("replay bypassed authorization: status=%d", denied.status)
	}
	admin := callMutation(replica, "/v2/admin/credentials", "", `{}`, "test-key")
	adminFields := responseFields(t, admin)
	adminKey := adminFields["api_key"].(string)
	key = uuid.NewString()
	one := callMutation(replica, credentialPath, key, `{}`, "test-key")
	two := callMutation(replica, credentialPath, key, `{}`, adminKey)
	if one.status != 201 || two.status != 201 || responseFields(t, one)["credential_id"] == responseFields(t, two)["credential_id"] {
		t.Fatal("different principals shared a replay record")
	}
	if _, err := db.Exec("UPDATE api_credentials SET revoked_at = NOW() WHERE id = $1", adminFields["credential_id"]); err != nil {
		t.Fatal(err)
	}
	denied = callMutation(replica, credentialPath, key, `{}`, adminKey)
	if denied.status != 401 || denied.replayed != "" {
		t.Fatalf("revoked caller retrieved replay: status=%d", denied.status)
	}
	counter := callMutation(replica, "/v2/tenants/"+tenantID+"/counters", "", `{"label":"reads"}`, "test-key")
	counterID := responseFields(t, counter)["counter_id"].(string)
	for _, path := range []string{"/", "/tools/curl", "/livez", "/readyz", "/metrics", "/v2/tenants/" + tenantID, "/v2/tenants/" + tenantID + "/counters", "/v2/tenants/" + tenantID + "/counters/" + counterID, "/v2/tenants/" + tenantID + "/counters/" + counterID, "/v2/tenants/" + tenantID + "/counters/" + counterID + "/operations"} {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(path)
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.Header.Set("X-API-Key", "test-key")
		ctx.Request.Header.Set("Idempotency-Key", "invalid-and-ignored-on-reads")
		ctx.Request.Header.Set("Origin", "https://client.example")
		replica.ServeHTTP(ctx)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("read %s unexpectedly rejected header: %d", path, ctx.Response.StatusCode())
		}
		if !strings.Contains(string(ctx.Response.Header.Peek("Access-Control-Expose-Headers")), "Idempotency-Replayed") {
			t.Fatal("browser clients cannot inspect replay header")
		}
	}
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI(credentialPath)
	ctx.Request.Header.SetMethod("OPTIONS")
	ctx.Request.Header.Set("Idempotency-Key", "invalid-and-ignored-on-preflight")
	replica.ServeHTTP(ctx)
	if ctx.Response.StatusCode() != 200 {
		t.Fatal("preflight rejected idempotency header")
	}
}

func TestIdempotencyLockTimeoutAndScopeIsolation(t *testing.T) {
	db := testutil.OpenPostgres(t)
	tenantID := createTestTenant(t, db, "lock-idempotency")
	path := "/v2/tenants/" + tenantID + "/counters"
	key := uuid.NewString()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "legacy-env-admin:POST:"+path+":"+key); err != nil {
		t.Fatal(err)
	}
	replica := NewRouterWithTimeouts(db, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(10000, 3, 60), "test-key", true, middleware.NewDefaultLogger("error"), nil, middleware.RouteTimeouts{Database: 100 * time.Millisecond}, 128)
	blocked := callMutation(replica, path, key, `{"label":"locked"}`, "test-key")
	if blocked.status != 503 {
		t.Fatalf("blocked request status = %d", blocked.status)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	first := callMutation(replica, path, key, `{"label":"locked"}`, "test-key")
	if first.status != 201 || first.replayed != "false" {
		t.Fatalf("retry after timeout failed: status=%d", first.status)
	}
	otherTenant := createTestTenant(t, db, "other-idempotency")
	other := callMutation(replica, "/v2/tenants/"+otherTenant+"/counters", key, `{"label":"locked"}`, "test-key")
	if other.status != 201 || responseFields(t, other)["counter_id"] == responseFields(t, first)["counter_id"] {
		t.Fatal("different tenants shared a replay record")
	}
	conflict := callMutation(replica, path+"?changed=true", key, `{"label":"locked"}`, "test-key")
	if conflict.status != 409 {
		t.Fatalf("changed query did not conflict: status=%d", conflict.status)
	}
}
