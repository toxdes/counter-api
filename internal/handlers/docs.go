package handlers

import (
	"os"
	"github.com/valyala/fasthttp"
)

// DocsHandler serves the API documentation from docs/counter-api.html
func DocsHandler(ctx *fasthttp.RequestCtx) {
	// Read the HTML file from disk
	content, err := os.ReadFile("docs/counter-api.html")
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBodyString("Error loading documentation")
		return
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(content)
}
