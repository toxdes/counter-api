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
		"Get tenant",
		"Get counter",
		"id=\"tenant-read-id\"",
		"id=\"counter-read-id\"",
		"id=\"counter-read-warning\"",
		"counter:read",
		"curlGet(root + \"/tenants/\"",
		"curlGet(root + \"/v2/tenants/\"",
		"family=Inter:wght@400;500;600;700",
		"family=JetBrains+Mono:wght@400;500;600;700",
		"https://github.com/toxdes/counter-api",
		"width: min(1180px, calc(100% - 64px))",
		"font-size: .92rem;",
		"main { padding: 62px 0 90px; }",
		"h2 { margin: 0; font-size: 1.5rem;",
		"font-size: 1rem; line-height: 1.4;",
		"font: .86rem/1.6 \"JetBrains Mono\"",
		"@media (max-width: 760px)",
		"width: min(100% - 36px, 600px)",
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
	for _, unwanted := range []string{"V1", "V2", "fetch(", "<textarea", "Command copied to clipboard.", ".step:nth-of-type(2) .step-number"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("page unexpectedly contains %q", unwanted)
		}
	}
	if strings.Contains(body, `value="counter:list" checked`) {
		t.Error("counter:list should not be selected by default in the credential builder")
	}
	if !strings.Contains(body, `value="counter:read" checked`) {
		t.Error("counter:read should be selected by default in the credential builder")
	}
	for _, scope := range []string{"tenant:read", "counter:list", "counter:create", "counter:increment", "counter:adjust", "counter:history"} {
		if strings.Contains(body, `value="`+scope+`" checked`) {
			t.Errorf("%s should not be selected by default in the credential builder", scope)
		}
	}
	if got := strings.Count(body, ` | jq .`); got != 3 {
		t.Errorf("generated curl command templates with jq formatter = %d, want 3", got)
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
	for _, want := range []string{"Counter API", "cURL helper", "use cURL helper", "https://github.com/toxdes/counter-api", "/tools/curl", "/v2/tenants/"} {
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
