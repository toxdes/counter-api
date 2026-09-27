package handlers

import (
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestCurlBuilderHandlerServesCommandBuilder(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/tools/curl")

	CurlBuilderHandler(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", got, fasthttp.StatusOK)
	}
	if got := string(ctx.Response.Header.ContentType()); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	body := string(ctx.Response.Body())
	for _, want := range []string{
		"cURL tool",
		"Generate cURL commands for commonly used APIs. This is a local-browser-only tool that runs in your browser.",
		"/tenants",
		"/v2/tenants/",
		"counter:create",
		"counter:list",
		"--silent --show-error",
		"id=\"api-key\"",
		"Tenant API Key",
		"family=Inter:wght@400;500;600;700",
		"family=JetBrains+Mono:wght@400;500;600;700",
		"<pre class=\"command-text\" aria-labelledby=\"tenant-command-label\"><code id=\"tenant-command\"",
		"Copied!",
		"3000",
		"byId(\"tenant-command\").textContent = curl(",
		"#181828",
		"X-API-Key",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"V1", "V2", "fetch(", "<textarea", "Command copied to clipboard."} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page unexpectedly contains %q", unwanted)
		}
	}
	if strings.Contains(body, `value="counter:list" checked`) {
		t.Error("counter:list should not be selected by default in the credential builder")
	}
}

func TestDocsHandlerServesConciseVersionNeutralReference(t *testing.T) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")

	DocsHandler(ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("status = %d, want %d", got, fasthttp.StatusOK)
	}
	body := string(ctx.Response.Body())
	for _, want := range []string{"Counter API", "Command builder", "/tools/curl", "/v2/tenants/"} {
		if !strings.Contains(body, want) {
			t.Errorf("docs page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"V1", "V2", "V2 Get Counter", "V2 Increment Counter"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("docs page unexpectedly contains version framing %q", unwanted)
		}
	}
}
