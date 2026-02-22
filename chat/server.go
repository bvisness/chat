package chat

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const serverShutdownTimeout = 10 * time.Second

func Run() {
	var wg sync.WaitGroup

	wg.Add(1)
	server := http.Server{
		Addr:    ":8667",
		Handler: http.HandlerFunc(mainHandler),
	}
	go func() {
		// TODO(ben): Logging
		serverErr := server.ListenAndServe()
		if serverErr != http.ErrServerClosed {
			// TODO(ben): Logging
		}
		// The wg.Done() happens in the shutdown logic below.
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals // First SIGINT (start shutdown)
		// TODO(ben): Logging

		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer cancel()
			shutDownEventStreams = true
			err := server.Shutdown(timeoutCtx)
			if err != nil {
				// TODO(ben): Logging of un-graceful shutdown
			}
			wg.Done()
		}()

		<-signals // Second SIGINT (force quit)
		// TODO(ben): Logging
		os.Exit(1)
	}()

	wg.Wait()
}

func mainHandler(rawRes http.ResponseWriter, rawReq *http.Request) {
	var h Handler
	for _, route := range Routes {
		if route.Method == rawReq.Method && route.Path == rawReq.URL.Path {
			h = route.Handler
			break
		}
	}
	if h == nil {
		// TODO(ben): Nice 404 page
		rawRes.WriteHeader(404)
		return
	}

	req := Request{
		ResponseHeader: rawRes.Header(),

		RawReq: rawReq,
		RawRes: rawRes,

		Ctx: rawReq.Context(),
	}
	res := h(&req)

	if res.Hijacked {
		// Control of this request has been handed off to something else,
		// e.g. a WebSocket handler.
		return
	}

	rawRes.WriteHeader(res.StatusCode)
	_, err := io.Copy(rawRes, &res.Body) // NOTE(ben): This will not incur an intermediate copy because bytes.Buffer implements WriteTo.
	if err == syscall.EPIPE {
		// The other side hung up. Oh well.
	} else if err != nil {
		// TODO(ben): Logging
	}
}
