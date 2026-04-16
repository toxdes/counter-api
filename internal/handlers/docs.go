package handlers

import (
	_ "embed"
	"github.com/valyala/fasthttp"
)

//go:embed docs.html
var docsHTML []byte

// DocsHandler serves the embedded API documentation
func DocsHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(docsHTML)
}
