package chat

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/bvisness/chat/glog"
	"github.com/bvisness/chat/utils"
	"github.com/gorilla/websocket"
)

const maxMessageSize = 16 * utils.KiB

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// TODO(ben): The Error property here would allow us to send nicer HTTP errors during the
	// upgrade process. Might be nice.

	// TODO(ben): It's not clear to me if we actually care what origin the request comes from. After
	// all, we might have clients connecting from mobile or other places that wouldn't set an Origin
	// header. (I assume user agents other than browsers don't typically set an Origin header,
	// because what would they set it to?)
	CheckOrigin: func(r *http.Request) bool { return true },
}

var eventStreamWaitGroup sync.WaitGroup
var eventStreamShutdownSignal = make(chan struct{})

const eventStreamShutdownTimeout = 8 * time.Second

func hEventStream(c *Request) (res Response) {
	log := c.Log
	defer log.Debug("Event stream handler exited")

	eventStreamWaitGroup.Add(1)
	defer eventStreamWaitGroup.Done()

	// NOTE(ben): We know up front that this is being hijacked, so just set the return value now.
	res = c.Hijacked()

	conn, err := upgrader.Upgrade(c.RawRes, c.RawReq, nil)
	if err != nil {
		log.Err("Failed to upgrade WebSocket connection", err)
		return
	}
	defer conn.Close() // NOTE(ben): This closes the TCP socket entirely.

	var unexpectedErr error
	defer func() {
		if unexpectedErr != nil {
			log.Err("Unexpected fatal error in WebSocket server", unexpectedErr)
		}
	}()

	type EventStreamState int
	const (
		ESActive EventStreamState = iota
		ESServerClosing
	)
	state := ESActive
	var buf [maxMessageSize]byte

	// Background reader: puts WebSocket messages onto a channel.
	type readResult struct {
		messageType   int
		messageReader io.Reader
		err           error
	}
	reads := make(chan readResult, 1)
	defer close(reads)
	go func() {
		defer log.Debug("Reader goroutine is shutting down")
		for {
			messageType, messageReader, err := conn.NextReader()
			reads <- readResult{messageType, messageReader, err}
			if err != nil {
				return
			}

			// NOTE(ben): We cannot call NextReader again until the main loop is done with the message.
			// So we just reuse the channel to double as synchronization the other way. When the channel
			// is closed, we exit.
			_, goAgain := <-reads
			if !goAgain {
				return
			}
		}
	}()

	for {
		// NOTE(ben): We are going to do a style of closing that is reasonably "clean", at least
		// according to my interpretation of the WebSocket spec. The general guidance it gives is that
		// a closure is "clean" if both sides both send and receive a close frame (in some order).
		// Generally speaking, this means either:
		//
		// - Client sends close frame to server, server immediately sends close frame and closes the
		//   TCP connection.
		// - Server sends close frame to client, client immediately sends close frame, server drains
		//   any remaining messages and closes the TCP connection.
		//
		// In both cases the server is expected to close the TCP connection. It's worth noting that the
		// client may never receive the close message because the TCP connection is closed immediately
		// after - this is basically not a problem since the client will already know it's in a closing
		// state.
		//
		// Also, in both cases, the server can shut down immediately upon receiving a close frame from
		// the client. This is nice symmetry. Having the server close the connection in the second case
		// also gives the server an opportunity to reliably receive and process any final messages from
		// the client. (Within reason; a timeout is probably wise.)
		//
		// None of this really matters from an application reliability perspective, because we will
		// have our own application-level ACKs that reflect things like, say, persisting events to
		// disk. All this is just to be nice and minimize confusion and noise on client and server.

		select {
		case <-eventStreamShutdownSignal:
			if state == ESActive {
				state = ESServerClosing
				conn.SetReadDeadline(time.Now().Add(eventStreamShutdownTimeout))

				err := conn.WriteMessage(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
				)
				if err != nil {
					unexpectedErr = fmt.Errorf("in shutdown: %w", err)
					return
				}

				// NOTE(ben): Now we continue to to drain the rest of the queue. We expect to see a close
				// frame eventually, if the client is well-behaved.
			}

		case read := <-reads:
			if closeError, ok := errors.AsType[*websocket.CloseError](read.err); ok {
				typicalCodes := []int{websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived}
				if !slices.Contains(typicalCodes, closeError.Code) {
					log.Debug("Got unusual close code, does it mean anything?",
						glog.F{"code", closeError.Code},
						glog.F{"reason", closeError.Text},
					)
				}
				log.Debug("Client is closing the WebSocket connection; we will also close and exit",
					glog.F{"code", closeError.Code},
					glog.F{"reason", closeError.Text},
				)
				// NOTE(ben): gorilla/websocket has a default close handler that responds by echoing the
				// received close code, which is apparently conventional. So no need to explicitly send our
				// own close message here.
				return
			} else if errors.Is(read.err, os.ErrDeadlineExceeded) {
				utils.Assert(state == ESServerClosing, "expected server to be closing, but state was", state)
				log.Debug("Timed out waiting for client messages while shutting down")
				return
			} else if read.err != nil {
				unexpectedErr = fmt.Errorf("on read: %w", read.err)
				return
			}

			message, err := utils.ReadAllInto(read.messageReader, buf[:])
			if err == utils.ErrorTooBigForBuffer {
				log.Warning("Got oversize message, closing connection")
				err := conn.WriteMessage(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseMessageTooBig, fmt.Sprintf("messages must be at most %d bytes", maxMessageSize)),
				)
				if err != nil {
					unexpectedErr = fmt.Errorf("on oversize message: %w", err)
				}
				return
			} else if err != nil {
				unexpectedErr = fmt.Errorf("when reading message: %w", err)
				return
			}

			// NOTE(ben): Maybe someday we'll have the textual version of these messages, but for now only
			// binary is supported.
			if read.messageType != websocket.BinaryMessage {
				err := conn.WriteMessage(
					websocket.BinaryMessage,
					ErrorMessage("only binary messages are supported"),
				)
				if err != nil {
					unexpectedErr = fmt.Errorf("on non-binary message: %w", err)
					return
				}
				continue
			}

			err = handleClientMessage(conn, message)
			if err != nil {
				unexpectedErr = fmt.Errorf("in handleClientMessage: %w", err)
				return
			}

			reads <- readResult{}
		}
	}
}

func handleClientMessage(conn *websocket.Conn, message []byte) error {
	p := messageParser{message: message}

	messageType, err := p.ReadByte()
	if err != nil {
		return fmt.Errorf("failed to read message type: %w", err)
	}
	switch messageType {
	default:
		err := conn.WriteMessage(websocket.BinaryMessage, ErrorMessage("unknown message type %v", messageType))
		if err != nil {
			return err
		}
	}

	return nil
}

type messageParser struct {
	message []byte
	cur     int
}

func (p *messageParser) ReadByte() (byte, error) {
	res, err := p.ReadBytes(1)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

func (p *messageParser) ReadBytes(n int) ([]byte, error) {
	if len(p.message[p.cur:]) < n {
		return nil, io.ErrUnexpectedEOF
	}
	res := p.message[p.cur : p.cur+n]
	p.cur += n
	return res, nil
}
