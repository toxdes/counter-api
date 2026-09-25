package handlers

import (
	"counter/internal/testutil"
	"encoding/json"
	"sync"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestIncrementCounterConcurrentTotal(t *testing.T) {
	db := testutil.OpenPostgres(t)
	tenantID := createTestTenant(t, db, "concurrency-tenant")
	counterID := createTestCounter(t, db, tenantID, "concurrent-increments", 0)
	handler := IncrementCounterHandler(db)

	const requests = 32
	start := make(chan struct{})
	errs := make(chan string, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI("/tenants/" + tenantID + "/counters/" + counterID + "/inc")
			ctx.Request.Header.SetMethod("POST")
			ctx.SetUserValue("tenant_id", tenantID)
			ctx.SetUserValue("counter_id", counterID)

			handler(ctx)
			if ctx.Response.StatusCode() != fasthttp.StatusOK {
				errs <- string(ctx.Response.Body())
				return
			}

			var response struct {
				Value int64 `json:"value"`
			}
			if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
				errs <- err.Error()
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent increment failed: %s", err)
	}

	var value int64
	if err := db.Get(&value, "SELECT value FROM counters WHERE id = $1 AND tenant_id = $2", counterID, tenantID); err != nil {
		t.Fatalf("failed to read final counter value: %v", err)
	}
	if value != requests {
		t.Fatalf("concurrent increments lost updates: got %d, want %d", value, requests)
	}
}
