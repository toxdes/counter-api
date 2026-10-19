package router

import (
	"context"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/service"
	"counter/internal/store"
	"counter/internal/testutil"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type metadataResponse struct {
	status               int
	body, etag, replayed string
}

func metadataRequest(router *Router, method, path, key, body, match, none, requestID string) metadataResponse {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod(method)
	ctx.Request.SetRequestURI(path)
	ctx.Request.Header.Set("X-API-Key", key)
	ctx.Request.Header.Set("If-Match", match)
	ctx.Request.Header.Set("If-None-Match", none)
	ctx.Request.Header.Set("Idempotency-Key", requestID)
	ctx.Request.SetBodyString(body)
	router.ServeHTTP(ctx)
	return metadataResponse{ctx.Response.StatusCode(), string(ctx.Response.Body()), string(ctx.Response.Header.Peek("ETag")), string(ctx.Response.Header.Peek("Idempotency-Replayed"))}
}

func TestMetadataLifecycleWithRestrictedRole(t *testing.T) {
	owner, db := testutil.OpenRestrictedPostgres(t)
	if err := database.VerifyRuntimeRole(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	tenant := createTestTenant(t, owner, "metadata-lifecycle")
	counter := createTestCounter(t, owner, tenant, "metadata", 7)
	path := "/v2/tenants/" + tenant + "/counters/" + counter + "/metadata"
	router, replica := idempotencyReplica(db), idempotencyReplica(db)
	get := metadataRequest(router, "GET", path, "test-key", "", "", "", "")
	if get.status != 404 || !strings.Contains(get.body, "METADATA_NOT_FOUND") {
		t.Fatalf("missing metadata status=%d", get.status)
	}
	key := uuid.NewString()
	created := metadataRequest(router, "PUT", path, "test-key", `null`, "", "*", key)
	if created.status != 201 || created.etag == "" {
		t.Fatalf("create status=%d: %s", created.status, created.body)
	}
	retry := metadataRequest(replica, "PUT", path, "test-key", `null`, "", "*", key)
	if retry.status != 201 || retry.etag != created.etag || retry.body != created.body || retry.replayed != "true" {
		t.Fatal("create replay did not preserve metadata and ETag")
	}
	changedHeader := metadataRequest(replica, "PUT", path, "test-key", `null`, created.etag, "", key)
	if changedHeader.status != 409 {
		t.Fatalf("changed precondition reused key: %d", changedHeader.status)
	}
	get = metadataRequest(router, "GET", path, "test-key", "", "", "", "")
	if get.status != 200 || !strings.Contains(get.body, `"data":null`) {
		t.Fatal("JSON null was confused with missing metadata")
	}
	for _, tc := range []struct {
		body, match, none string
		status            int
	}{
		{`{}`, "", "", 428}, {`{}`, "", "*", 412}, {`{}`, `"` + uuid.NewString() + `"`, "", 412},
		{`{}`, "W/" + created.etag, "", 400}, {`{}`, created.etag, "*", 400},
		{`invalid`, created.etag, "", 400}, {`"\u0000"`, created.etag, "", 400},
		{`"` + strings.Repeat("x", service.MaxMetadataBytes) + `"`, created.etag, "", 413},
	} {
		response := metadataRequest(router, "PUT", path, "test-key", tc.body, tc.match, tc.none, "")
		if response.status != tc.status {
			t.Fatalf("invalid write status=%d, want %d: %s", response.status, tc.status, response.body)
		}
	}
	current := created
	for _, data := range []string{`{"n":9007199254740993,"nested":[true,null]}`, `[1,"two",{}]`, `"text"`, `true`, `123`, `null`} {
		updated := metadataRequest(router, "PUT", path, "test-key", data, current.etag, "", uuid.NewString())
		if updated.status != 200 || updated.etag == current.etag {
			t.Fatalf("replace status=%d: %s", updated.status, updated.body)
		}
		current = updated
	}
	deletionKey := uuid.NewString()
	deleted := metadataRequest(router, "DELETE", path, "test-key", "", current.etag, "", deletionKey)
	if deleted.status != 204 {
		t.Fatalf("delete status=%d", deleted.status)
	}
	if retry := metadataRequest(replica, "DELETE", path, "test-key", "", current.etag, "", deletionKey); retry.status != 204 || retry.replayed != "true" {
		t.Fatal("delete retry did not replay")
	}
	recreated := metadataRequest(router, "PUT", path, "test-key", `{}`, "", "*", "")
	if recreated.status != 201 || recreated.etag == current.etag || recreated.etag == created.etag {
		t.Fatal("delete/recreate reused an old ETag")
	}
	if stale := metadataRequest(router, "PUT", path, "test-key", `{}`, current.etag, "", ""); stale.status != 412 {
		t.Fatal("stale ETag overwrote recreated metadata")
	}
	var value int
	if err := owner.Get(&value, "SELECT value FROM counters WHERE id = $1", counter); err != nil || value != 7 {
		t.Fatalf("metadata changed counter value: %d, %v", value, err)
	}
}

func TestMetadataScopesAndTenantOwnership(t *testing.T) {
	owner, db := testutil.OpenRestrictedPostgres(t)
	router := idempotencyReplica(db)
	tenant := createTestTenant(t, owner, "metadata-scopes")
	other := createTestTenant(t, owner, "metadata-other")
	counter := createTestCounter(t, owner, tenant, "metadata", 7)
	path := "/v2/tenants/" + tenant + "/counters/" + counter + "/metadata"
	credentials := service.NewCredentialService(store.NewCredentialStore(owner))
	issue := func(tenant string, scopes []string) string {
		key, err := credentials.Create(context.Background(), tenant, scopes, nil, "setup")
		if err != nil {
			t.Fatal(err)
		}
		return key.APIKey
	}
	reader := issue(tenant, []string{middleware.ScopeMetadataRead})
	writer := issue(tenant, []string{middleware.ScopeMetadataWrite})
	countReader := issue(tenant, []string{middleware.ScopeCounterRead})
	otherKey := issue(other, []string{middleware.ScopeMetadataRead, middleware.ScopeMetadataWrite})
	for _, key := range []string{reader, countReader, otherKey, ""} {
		response := metadataRequest(router, "PUT", path, key, `{}`, "", "*", "")
		want := 403
		if key == "" {
			want = 401
		}
		if response.status != want {
			t.Fatalf("unauthorized PUT status=%d, want %d", response.status, want)
		}
	}
	created := metadataRequest(router, "PUT", path, writer, `{"secret":"counter-only"}`, "", "*", uuid.NewString())
	if created.status != 201 {
		t.Fatalf("scoped create status=%d: %s", created.status, created.body)
	}
	if got := metadataRequest(router, "GET", path, reader, "", "", "", ""); got.status != 200 {
		t.Fatalf("reader status=%d", got.status)
	}
	for _, key := range []string{writer, countReader, otherKey} {
		if got := metadataRequest(router, "GET", path, key, "", "", "", ""); got.status != 403 {
			t.Fatalf("unauthorized GET status=%d", got.status)
		}
	}
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI(strings.TrimSuffix(path, "/metadata"))
	ctx.Request.Header.Set("X-API-Key", countReader)
	router.ServeHTTP(ctx)
	if ctx.Response.StatusCode() != 200 || strings.Contains(string(ctx.Response.Body()), "secret") {
		t.Fatal("counter response leaked metadata or lost count access")
	}
	missing := "/v2/tenants/" + tenant + "/counters/" + uuid.NewString() + "/metadata"
	if got := metadataRequest(router, "PUT", missing, writer, `{}`, "", "*", ""); got.status != 404 {
		t.Fatalf("missing counter status=%d", got.status)
	}
}

func TestConcurrentMetadataRevisionAndReplay(t *testing.T) {
	owner, db := testutil.OpenRestrictedPostgres(t)
	tenant := createTestTenant(t, owner, "metadata-concurrency")
	counter := createTestCounter(t, owner, tenant, "metadata", 0)
	path := "/v2/tenants/" + tenant + "/counters/" + counter + "/metadata"
	replicas := []*Router{idempotencyReplica(db), idempotencyReplica(db)}
	created := metadataRequest(replicas[0], "PUT", path, "test-key", `{}`, "", "*", "")
	if created.status != 201 {
		t.Fatalf("create status=%d", created.status)
	}
	for _, replay := range []bool{true, false} {
		key := uuid.NewString()
		etag := created.etag
		const requests = 8
		results := make([]metadataResponse, requests)
		var group sync.WaitGroup
		for i := range results {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				requestKey := key
				if !replay {
					requestKey = uuid.NewString()
				}
				results[i] = metadataRequest(replicas[i%2], "PUT", path, "test-key", `{"n":1}`, etag, "", requestKey)
			}(i)
		}
		group.Wait()
		successes := 0
		for _, result := range results {
			if result.status == 200 {
				successes++
				created = result
			} else if result.status != 412 {
				t.Fatalf("concurrent status=%d: %s", result.status, result.body)
			}
		}
		want := 1
		if replay {
			want = requests
		}
		if successes != want {
			t.Fatalf("successful concurrent writes=%d, want %d", successes, want)
		}
	}
	var saved []byte
	if err := owner.Get(&saved, "SELECT data FROM counter_metadata WHERE counter_id=$1", counter); err != nil {
		t.Fatal(err)
	}
	var value map[string]int
	if err := json.Unmarshal(saved, &value); err != nil || value["n"] != 1 {
		t.Fatal("unexpected final metadata")
	}
}

func TestMetadataEditLockDoesNotBlockIncrement(t *testing.T) {
	owner, runtime := testutil.OpenRestrictedPostgres(t)
	tenant := createTestTenant(t, owner, "metadata-locks")
	counter := createTestCounter(t, owner, tenant, "independent-lock", 0)
	replica := idempotencyReplica(runtime)
	path := "/v2/tenants/" + tenant + "/counters/" + counter
	created := metadataRequest(replica, "PUT", path+"/metadata", "test-key", `{}`, "", "*", "")
	if created.status != 201 {
		t.Fatalf("create status=%d", created.status)
	}
	access := database.WithAccess(context.Background(), database.Access{TenantID: tenant, PrincipalID: "test"})
	tx, finish, err := database.BeginWrite(access, runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer finish(false)
	if _, err := tx.ExecContext(access, `UPDATE counter_metadata SET data='{"editing":true}' WHERE counter_id=$1`, counter); err != nil {
		t.Fatal(err)
	}
	// Keep the metadata update uncommitted while the independent counter write
	// completes. A shared row lock would make this request time out.
	got := callMutation(replica, path+"/inc", uuid.NewString(), "", "test-key")
	if got.status != 200 {
		t.Fatalf("increment blocked by metadata edit: %d %s", got.status, got.body)
	}
}
