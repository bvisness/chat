package chat

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/bvisness/chat/glog"
)

type Route struct {
	Method  string
	Path    string // Includes leading slash
	Handler Handler
}

func (r *Route) String() string {
	return fmt.Sprintf("%s %s", r.Method, r.Path)
}

type Handler func(c *Request) Response

type Request struct {
	ResponseHeader http.Header

	RawReq *http.Request
	RawRes http.ResponseWriter

	Ctx context.Context
	Log glog.Logger
}

type Response struct {
	StatusCode int
	Body       bytes.Buffer

	Hijacked bool
}

func (c *Request) ErrorResponse(status int, message string) Response {
	res := Response{
		StatusCode: status,
	}
	res.Body.WriteString(message)

	return res
}

// Returns a pseudo-response telling the request handler that the connection was hijacked, and no
// further content should be written. Note that requests do not participate in graceful shutdown
// when this happens, so additional synchronization logic may be needed.
func (c *Request) Hijacked() Response {
	return Response{Hijacked: true}
}
