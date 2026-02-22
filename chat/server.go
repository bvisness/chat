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

	"github.com/bvisness/chat/glog"
)

const serverShutdownTimeout = 10 * time.Second

var log = glog.Logger{}

func Run() {
	var wg sync.WaitGroup

	wg.Add(1)
	server := http.Server{
		Addr:    "localhost:8667",
		Handler: http.HandlerFunc(mainHandler),
	}
	go func() {
		log.Info("Starting server", glog.F{"addr", server.Addr})
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Err("Failed to start server", err)
			os.Exit(1)
		}
		// The wg.Done() happens in the shutdown logic below.
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals // First SIGINT (start shutdown)
		log.Info("Shutting down server (gracefully)")

		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer cancel()
			shutDownEventStreams = true
			err := server.Shutdown(timeoutCtx)
			if err != nil {
				log.Err("Server did not shut down gracefully", err)
			}
			wg.Done()
		}()

		<-signals // Second SIGINT (force quit)
		log.Warning("Server was forcibly shut down")
		os.Exit(1)
	}()

	wg.Wait()
	log.Info("Bye bye!")
}

func mainHandler(rawRes http.ResponseWriter, rawReq *http.Request) {
	log := log

	var h Handler
	for _, route := range Routes {
		if route.Method == rawReq.Method && route.Path == rawReq.URL.Path {
			h = route.Handler
			log = log.WithFields(glog.F{"route", route.String()})
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
		Log: log,
	}
	log.Log("REQUEST", "Request received")
	res := h(&req)

	if res.Hijacked {
		// NOTE(ben): Control of this request has been handed off to something else, e.g. a
		// WebSocket handler.
		return
	}

	rawRes.WriteHeader(res.StatusCode)
	_, err := io.Copy(rawRes, &res.Body) // NOTE(ben): This will not incur an intermediate copy because bytes.Buffer implements WriteTo.
	if err == syscall.EPIPE {
		// NOTE(ben): The other side hung up. Oh well.
	} else if err != nil {
		log.Err("Failed to write response", err)
	}
}
