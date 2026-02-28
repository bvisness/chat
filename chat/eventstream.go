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
	"unicode/utf8"
	"unsafe"

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
	//
	// ADDENDUM(ben): Ok, but if we just did our own WebSocket impl instead...then we could do
	// EXACTLY what we wanted and be free of Gorilla's terrible design decisions to boo.

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
	reads := make(chan readResult)
	readAgain := make(chan struct{}, 1)
	defer close(reads)
	defer close(readAgain)
	go func() {
		defer log.Debug("Reader goroutine is shutting down")
		for {
			messageType, messageReader, err := conn.NextReader()
			reads <- readResult{messageType, messageReader, err}
			if err != nil {
				defer log.DebugErr("Reader saw error, quitting", err)
				return
			}

			_, ok := <-readAgain
			if !ok {
				defer log.Debug("Reader's readAgain channel closed")
				return
			}
		}
	}()

	newEventsToSync := eventNotifications.Subscribe()
	defer eventNotifications.Unsubscribe(newEventsToSync)

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

		case <-newEventsToSync:
			log.Debug("I heard there were new events to sync...")
			if state > ESActive {
				// NOTE(ben): Don't write new messages if we're closing.
				// TODO(ben): Unless...do we want to send ACKs still? Before closing? Maybe a final ACK
				// can just be part of the close sequence? Except the client will have already gone into
				// its close state probably.
				//
				// Maybe this whole "drain client messages before quitting" thing is stupid and I should
				// just hard-abort. What happens if I can't process a message? Who do I send an error to?
				// Does it make sense to handle messages that I can't ACK? Theoretically the client would
				// come back later and things would reconcile. And that's probably good, because then even
				// in the case where the server initiates a shutdown, the messages from the client do get
				// received and don't need to be retried by the client when the server comes back online.
				// Overall that seems more reliable. So maybe it's fine. It should be fine in general if
				// the server handles a message but then the client can't be ACKed. That seems somewhat
				// obvious. So ok, sure, whatever, I guess. Tomorrow's problem.
				break
			}

		case read := <-reads:
			exit, exitErr := func() (bool, error) {
				defer func() { readAgain <- struct{}{} }()

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
					return true, nil
				} else if errors.Is(read.err, os.ErrDeadlineExceeded) {
					utils.Assert(state == ESServerClosing, "expected server to be closing, but state was", state)
					log.Debug("Timed out waiting for client messages while shutting down")
					return true, nil
				} else if read.err != nil {
					return true, fmt.Errorf("on read: %w", read.err)
				}

				message, err := utils.ReadAllInto(read.messageReader, buf[:])
				if err == utils.ErrorTooBigForBuffer {
					log.Warning("Got oversize message, closing connection")
					err := conn.WriteMessage(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseMessageTooBig, fmt.Sprintf("messages must be at most %d bytes", maxMessageSize)),
					)
					if err != nil {
						return true, fmt.Errorf("on oversize message: %w", err)
					}
				} else if err != nil {
					return true, fmt.Errorf("when reading message: %w", err)
				}

				// NOTE(ben): Maybe someday we'll have the textual version of these messages, but for now
				// only binary is supported.
				if read.messageType != websocket.BinaryMessage {
					err := conn.WriteMessage(
						websocket.BinaryMessage,
						ErrorMessage("only binary messages are supported"),
					)
					if err != nil {
						return true, fmt.Errorf("on non-binary message: %w", err)
					}
					return false, nil
				}

				err = handleClientMessage(conn, message)
				if err != nil {
					return true, fmt.Errorf("in handleClientMessage: %w", err)
				}

				return false, nil
			}()
			utils.Assert(!(!exit && exitErr != nil), "saw an exitErr but we weren't exiting")
			if exit {
				if exitErr != nil {
					unexpectedErr = exitErr
				}
				return
			}
		}
	}
}

// Handles a single WebSocket message from the client. The error return is ONLY for unexpected
// errors that should terminate the connection.
func handleClientMessage(conn *websocket.Conn, message []byte) error {
	p := messageParser{message: message}

	nonFatalError := func(msg string, args ...any) error {
		err := conn.WriteMessage(websocket.BinaryMessage, ErrorMessage(msg, args...))
		if err != nil {
			return err
		}
		return nil
	}

	messageType, err := p.ReadByte("message type")
	if err != nil {
		return nonFatalError("missing message type")
	}
	switch MessageType(messageType) {
	case MTChatEvent:
		eventType, err := p.ReadByte("event type")
		if err != nil {
			return nonFatalError("missing event type")
		}
		switch EventType(eventType) {
		case ETMessage:
			text, err := p.ReadUTF8String("message text")
			if err != nil {
				return err
			}
			newEvents <- Event{
				Type:        ETMessage,
				MessageText: text,
			}
		default:
			return nonFatalError("unknown event type %v", eventType)
		}
	default:
		return nonFatalError("unknown message type %v", messageType)
	}

	return nil
}

type messageParser struct {
	message []byte
	cur     int
}

func (p *messageParser) ReadByte(thing string) (byte, error) {
	res, err := p.ReadBytes(1, thing)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

func (p *messageParser) ReadBytes(n int, thing string) ([]byte, error) {
	if len(p.message[p.cur:]) < n {
		return nil, fmt.Errorf("parsing %s: %w", io.ErrUnexpectedEOF)
	}
	res := p.message[p.cur : p.cur+n]
	p.cur += n
	return res, nil
}

func (p *messageParser) ReadU32(thing string) (uint32, error) {
	b, err := p.ReadBytes(4, thing)
	if err != nil {
		return 0, err
	}
	return *(*uint32)(unsafe.Pointer(&b[0])), nil
}

func (p *messageParser) ReadUTF8String(thing string) (string, error) {
	n, err := p.ReadU32(thing)
	if err != nil {
		return "", err
	}
	b, err := p.ReadBytes(int(n), thing)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("parsing %s: invalid utf-8")
	}
	return string(b), nil
}
