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

func (c *Request) Hijacked() Response {
	return Response{Hijacked: true}
}
