package router

import (
	"counter/internal/middleware"
	"counter/internal/testutil"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestIdempotentIncrementCanRetryAcrossRouterReplicas(t *testing.T) {
	db := testutil.OpenPostgres(t)
	tenantID := createTestTenant(t, db, "replica-retry")

	newReplica := func() *Router {
		return NewRouter(db, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(1000, 3, 60), "test-key", middleware.NewDefaultLogger("error"), nil)
	}
	replicas := []*Router{newReplica(), newReplica(), newReplica()}

	createCtx := &fasthttp.RequestCtx{}
	createCtx.Request.SetRequestURI("/tenants/" + tenantID + "/counters")
	createCtx.Request.Header.SetMethod("POST")
	createCtx.Request.Header.Set("X-API-Key", "test-key")
	createCtx.Request.Header.SetContentType("application/json")
	createCtx.Request.SetBody([]byte(`{"label":"requests","initial_value":0}`))
	replicas[0].ServeHTTP(createCtx)
	if createCtx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("create counter status = %d: %s", createCtx.Response.StatusCode(), createCtx.Response.Body())
	}
	var created struct {
		ID string `json:"counter_id"`
	}
	if err := json.Unmarshal(createCtx.Response.Body(), &created); err != nil {
		t.Fatalf("decode created counter: %v", err)
	}
	counterID := created.ID
	if counterID == "" {
		t.Fatal("created counter response has empty id")
	}

	const operationID = "01912345-6789-7000-8000-000000000013"

	request := func(replica *Router) (int, string) {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/inc?delta=4")
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.Header.Set("Idempotency-Key", operationID)
		replica.ServeHTTP(ctx)
		return ctx.Response.StatusCode(), string(ctx.Response.Body())
	}

	status, body := request(replicas[0])
	if status != fasthttp.StatusOK {
		t.Fatalf("first replica status = %d: %s", status, body)
	}
	var first struct {
		Value       int64  `json:"value"`
		OperationID string `json:"operation_id"`
		Replayed    bool   `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if first.Value != 4 || first.OperationID != operationID || first.Replayed {
		t.Fatalf("first replica response = %#v", first)
	}

	status, body = request(replicas[1])
	if status != fasthttp.StatusOK {
		t.Fatalf("retry on second replica status = %d: %s", status, body)
	}
	var retry struct {
		Value       int64  `json:"value"`
		OperationID string `json:"operation_id"`
		Replayed    bool   `json:"replayed"`
	}
	if err := json.Unmarshal([]byte(body), &retry); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retry.Value != first.Value || retry.OperationID != first.OperationID || !retry.Replayed {
		t.Fatalf("second replica retry = %#v, first response = %#v", retry, first)
	}

	var value, operations int64
	if err := db.Get(&value, "SELECT value FROM counters WHERE tenant_id = $1 AND id = $2", tenantID, counterID); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if err := db.Get(&operations, "SELECT COUNT(*) FROM counter_operations WHERE tenant_id = $1 AND counter_id = $2 AND operation_id = $3", tenantID, counterID, operationID); err != nil {
		t.Fatalf("count operation rows: %v", err)
	}
	if value != 4 || operations != 1 {
		t.Fatalf("durable value/operation count = %d/%d, want 4/1", value, operations)
	}
}

func TestConcurrentIncrementsAcrossReplicasDoNotDeadlock(t *testing.T) {
	db := testutil.OpenPostgres(t)
	tenantID := createTestTenant(t, db, "replica-concurrency")
	newReplica := func() *Router {
		return NewRouter(db, &middleware.CORSConfig{AllowedOrigins: "*"}, middleware.NewRateLimiter(1000, 3, 60), "test-key", middleware.NewDefaultLogger("error"), nil)
	}
	replicas := []*Router{newReplica(), newReplica(), newReplica()}

	createCtx := &fasthttp.RequestCtx{}
	createCtx.Request.SetRequestURI("/tenants/" + tenantID + "/counters")
	createCtx.Request.Header.SetMethod("POST")
	createCtx.Request.Header.Set("X-API-Key", "test-key")
	createCtx.Request.Header.SetContentType("application/json")
	createCtx.Request.SetBody([]byte(`{"label":"parallel","initial_value":0}`))
	replicas[0].ServeHTTP(createCtx)
	if createCtx.Response.StatusCode() != fasthttp.StatusCreated {
		t.Fatalf("create counter status = %d: %s", createCtx.Response.StatusCode(), createCtx.Response.Body())
	}
	var created struct {
		ID string `json:"counter_id"`
	}
	if err := json.Unmarshal(createCtx.Response.Body(), &created); err != nil {
		t.Fatalf("decode created counter: %v", err)
	}

	const requests = 12
	start := make(chan struct{})
	errors := make(chan string, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(fmt.Sprintf("/tenants/%s/counters/%s/inc?delta=1", tenantID, created.ID))
			ctx.Request.Header.SetMethod("POST")
			replicas[i%len(replicas)].ServeHTTP(ctx)
			if ctx.Response.StatusCode() != fasthttp.StatusOK {
				errors <- fmt.Sprintf("replica %d returned %d: %s", i%len(replicas), ctx.Response.StatusCode(), ctx.Response.Body())
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}

	var value int64
	if err := db.Get(&value, "SELECT value FROM counters WHERE tenant_id = $1 AND id = $2", tenantID, created.ID); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if value != requests {
		t.Fatalf("concurrent final value = %d, want %d", value, requests)
	}
}
