package chat

import (
	"bytes"
	"context"
	"net/http"
)

type Route struct {
	Method  string
	Path    string // Includes leading slash
	Handler Handler
}

type Handler func(c *Request) Response

type Request struct {
	ResponseHeader http.Header

	RawReq *http.Request
	RawRes http.ResponseWriter

	Ctx context.Context
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
