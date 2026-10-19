package handlers

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/valyala/fasthttp"
)

func requestHashForBody(body string) []byte {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/v2/tenants")
	ctx.Request.SetBodyString(body)
	return idempotencyRequestHash(ctx)
}

func TestIdempotencyHashPreservesOrderSensitiveJSON(t *testing.T) {
	// encoding/json accepts case-insensitive field aliases and merges repeated
	// object fields. Sorting or collapsing these keys can conceal a changed write.
	for _, tc := range []struct {
		name, first, retry string
	}{
		{"case aliases", `{"label":"first","Label":"second"}`, `{"Label":"second","label":"first"}`},
		{"duplicate objects", `{"config":{"a":1},"config":{"b":2}}`, `{"config":{"b":2}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type request struct {
				Label  string             `json:"label"`
				Config struct{ A, B int } `json:"config"`
			}
			var first, retry request
			if err := json.Unmarshal([]byte(tc.first), &first); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.retry), &retry); err != nil {
				t.Fatal(err)
			}
			if first == retry {
				t.Fatal("fixture must represent different decoded writes")
			}
			if bytes.Equal(requestHashForBody(tc.first), requestHashForBody(tc.retry)) {
				t.Fatal("different decoded writes have the same idempotency hash")
			}
		})
	}
}

func TestIdempotencyHashCanonicalJSONPreservesLargeNumbers(t *testing.T) {
	first := requestHashForBody(`{"initial_value":9007199254740993,"label":"likes"}`)
	reordered := requestHashForBody(" { \"label\": \"likes\", \"initial_value\":9007199254740993 } ")
	different := requestHashForBody(`{"initial_value":9007199254740992,"label":"likes"}`)
	if !bytes.Equal(first, reordered) {
		t.Fatal("JSON whitespace or unambiguous key order changed the hash")
	}
	if bytes.Equal(first, different) {
		t.Fatal("large integer precision was lost in the hash")
	}
}

func TestIdempotencyHashHandlesNestedDocumentsAndEmptyArrays(t *testing.T) {
	first := requestHashForBody(`{"items":[{"b":2,"a":1}],"scopes":[]}`)
	reordered := requestHashForBody(`{"scopes":[],"items":[{"a":1,"b":2}]}`)
	withNull := requestHashForBody(`{"scopes":null,"items":[{"a":1,"b":2}]}`)
	if !bytes.Equal(first, reordered) {
		t.Fatal("unambiguous nested object order changed the hash")
	}
	if bytes.Equal(first, withNull) {
		t.Fatal("empty array and null were collapsed into the same hash")
	}
	aliased := requestHashForBody(`{"items":[{"label":"first","Label":"second"}]}`)
	reversed := requestHashForBody(`{"items":[{"Label":"second","label":"first"}]}`)
	if bytes.Equal(aliased, reversed) {
		t.Fatal("order-sensitive fields inside an array were collapsed")
	}
}
