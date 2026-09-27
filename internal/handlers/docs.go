package handlers

import (
	_ "embed"
	"github.com/valyala/fasthttp"
)

//go:embed docs.html
var docsHTML []byte

//go:embed curl_builder.html
var curlBuilderHTML []byte

// DocsHandler serves the embedded API documentation
func DocsHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(docsHTML)
}

// CurlBuilderHandler serves the local-only curl command builder.
func CurlBuilderHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(curlBuilderHTML)
}
