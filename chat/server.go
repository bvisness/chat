package chat

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bvisness/chat/db"
	"github.com/bvisness/chat/glog"
	"github.com/bvisness/chat/utils"
)

const serverShutdownTimeout = 10 * time.Second

var log = glog.Logger{}
var dbConn *sql.DB

func Run() {
	var wg sync.WaitGroup

	dbConn = utils.Must1(sql.Open("sqlite3", "file:chat.db"))
	defer dbConn.Close()
	utils.Must(migrator.Migrate(context.Background(), dbConn, migrations, db.MigrateAll))

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

		// Shut down event streams
		go func() {
			// NOTE(ben): This goroutine will go until the process exits, which is fine.
			for {
				eventStreamShutdownSignal <- struct{}{}
			}
		}()

		// Shut down other web requests
		go func() {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer cancel()
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
	eventStreamWaitGroup.Wait()
	log.Info("Bye bye!")
}

var requestID atomic.Int64

func mainHandler(rawRes http.ResponseWriter, rawReq *http.Request) {
	log := log.WithFields(glog.F{"id", requestID.Add(1)})

	var h Handler
	for _, route := range Routes {
		if route.Method == rawReq.Method && route.Path == rawReq.URL.Path {
			h = route.Handler
			log = log.WithFields(glog.F{"route", route.String()})
			break
		}
	}
	defer log.Recover("Panic in HTTP request")

	if h == nil {
		serveStaticFiles(rawRes, rawReq)
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

var wwwHandler = http.FileServer(http.Dir("www"))

func serveStaticFiles(rawRes http.ResponseWriter, rawReq *http.Request) {
	wwwHandler.ServeHTTP(rawRes, rawReq)
}
