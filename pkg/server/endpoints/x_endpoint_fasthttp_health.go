package endpoints

import "github.com/valyala/fasthttp"

// This contains an example endpoint using the library fasthttp.
// It seems that http2 with echo is faster than http1 with fasthttp, so we should probably just stick with echo.

func FastHttpServerTesthandler(ctx *fasthttp.RequestCtx) {
	//_, _ = fmt.Fprintf(ctx, "Hello, world!\n\n")
	//
	//_, _ = fmt.Fprintf(ctx, "Request method is %q\n", ctx.Method())
	//_, _ = fmt.Fprintf(ctx, "RequestURI is %q\n", ctx.RequestURI())
	//_, _ = fmt.Fprintf(ctx, "Requested path is %q\n", ctx.Path())
	//_, _ = fmt.Fprintf(ctx, "Host is %q\n", ctx.Host())
	//_, _ = fmt.Fprintf(ctx, "Query string is %q\n", ctx.QueryArgs())
	//_, _ = fmt.Fprintf(ctx, "User-Agent is %q\n", ctx.UserAgent())
	//_, _ = fmt.Fprintf(ctx, "Connection has been established at %s\n", ctx.ConnTime())
	//_, _ = fmt.Fprintf(ctx, "Request has been started at %s\n", ctx.Time())
	//_, _ = fmt.Fprintf(ctx, "Serial request number for the current connection is %d\n", ctx.ConnRequestNum())
	//_, _ = fmt.Fprintf(ctx, "Your ip is %q\n\n", ctx.RemoteIP())
	//
	//_, _ = fmt.Fprintf(ctx, "Raw request is:\n---CUT---\n%s\n---CUT---", &ctx.Request)

	//ctx.SetContentType("text/plain; charset=utf8")

	// Set arbitrary headers
	//ctx.Response.Header.Set("X-My-Header", "my-header-value")

	// Set cookies
	//var c fasthttp.Cookie
	//c.SetKey("cookie-name")
	//c.SetValue("cookie-value")
	//ctx.Response.Header.SetCookie(&c)

	ctx.SetStatusCode(200)
}
