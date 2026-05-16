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
	//
	// ADDENDUM(ben): Ok, but if we just did our own WebSocket impl instead...then we could do
	// EXACTLY what we wanted and be free of Gorilla's terrible design decisions to boot.

	// TODO(ben): It's not clear to me if we actually care what origin the request comes from. After
	// all, we might have clients connecting from mobile or other places that wouldn't set an Origin
	// header. (I assume user agents other than browsers don't typically set an Origin header,
	// because what would they set it to?)
	CheckOrigin: func(r *http.Request) bool { return true },
}

var eventStreamWaitGroup sync.WaitGroup
var eventStreamShutdownSignal = make(chan struct{})

const eventStreamShutdownTimeout = 8 * time.Second

type EventStreamSession struct {
	state           EventStreamState
	conn            *websocket.Conn
	clientNeedsSync utils.Waiter
	clientACK       int64

	unexpectedErr error

	buf [maxMessageSize]byte
}

type EventStreamState int

const (
	ESActive EventStreamState = iota
	ESServerClosing
)

func hEventStream(c *Request) (res Response) {
	log := c.Log
	defer log.Debug("Event stream handler exited")

	eventStreamWaitGroup.Add(1)
	defer eventStreamWaitGroup.Done()

	// NOTE(ben): We know up front that this is being hijacked, so just set the return value now.
	res = c.Hijacked()

	var s EventStreamSession

	var err error
	s.conn, err = upgrader.Upgrade(c.RawRes, c.RawReq, nil)
	if err != nil {
		log.Err("Failed to upgrade WebSocket connection", err)
		return
	}
	defer s.conn.Close() // NOTE(ben): This closes the TCP socket entirely.

	defer func() {
		if s.unexpectedErr != nil {
			log.Err("Unexpected fatal error in WebSocket server", s.unexpectedErr)
		}
	}()

	// Background reader: puts WebSocket messages onto a channel.
	type readResult struct {
		messageType   int
		messageReader io.Reader
		err           error
	}
	reads := make(chan readResult, 1)
	readAgain := make(chan struct{}, 1)
	defer close(reads)
	defer close(readAgain)
	go func() {
		defer log.Recover("Reader goroutine crashed")
		defer log.Debug("Reader goroutine is shutting down")

		for {
			messageType, messageReader, err := s.conn.NextReader()
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

	s.clientNeedsSync = utils.NewWaiter()
	defer close(s.clientNeedsSync)
	newEventNotifications.Subscribe(s.clientNeedsSync)
	defer newEventNotifications.Unsubscribe(s.clientNeedsSync)
	s.clientACK = -1

	defer log.Recover("Event stream crashed")

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

	thisiter:
		select {
		case <-eventStreamShutdownSignal:
			if s.state == ESActive {
				s.state = ESServerClosing
				s.conn.SetReadDeadline(time.Now().Add(eventStreamShutdownTimeout))

				err := s.conn.WriteMessage(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
				)
				if err != nil {
					s.unexpectedErr = fmt.Errorf("in shutdown: %w", err)
					return
				}

				// NOTE(ben): Now we continue to to drain the rest of the queue. We expect to see a close
				// frame eventually, if the client is well-behaved.
			}

		case <-s.clientNeedsSync:
			if s.state > ESActive {
				// NOTE(ben): Don't write new messages if we're closing.
				break thisiter
			}
			EventLog := EventLog // NOTE(ben): Get a stable view of the log
			if len(EventLog) == 0 {
				break thisiter
			}
			latestSN := EventLog[len(EventLog)-1].SN
			if latestSN < s.clientACK {
				// NOTE(ben): Impossible ACK from client, indicating misbehavior
				s.sendErrorToClient("you ACKed %d but %d is the most recent event", s.clientACK, latestSN)
				return
			} else if latestSN == s.clientACK {
				// NOTE(ben): Client is up to date
				break thisiter
			}

			// HACK(ben): Abusing the fact that, for now, SN == index in Events
			for _, event := range EventLog[s.clientACK+1:] {
				w := eventWriter{buf: s.buf[:]}
				// NOTE(ben): This should not fail because the buffer should, you know, exist.
				utils.Must(w.WriteByte(byte(ETRecord), "message type"))
				if err := event.Serialize(&w); err != nil {
					s.unexpectedErr = fmt.Errorf("serializing event for client: %w", err)
					return
				}
				if err := s.conn.WriteMessage(websocket.BinaryMessage, w.Bytes()); err != nil {
					s.unexpectedErr = fmt.Errorf("sending event to client: %w", err)
					return
				}
			}

			// NOTE(ben): Explicitly request an ACK when we are done catching the client up.
			if err := s.conn.WriteMessage(websocket.BinaryMessage, CreateSYNEvent(s.buf[:])); err != nil {
				s.unexpectedErr = fmt.Errorf("requesting ACK from client: %w", err)
				return
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
					utils.Assert(s.state == ESServerClosing, "expected server to be closing, but state was", s.state)
					log.Debug("Timed out waiting for client ws messages while shutting down")
					return true, nil
				} else if read.err != nil {
					return true, fmt.Errorf("on read: %w", read.err)
				}

				event, err := utils.ReadAllInto(read.messageReader, s.buf[:])
				if err == utils.ErrorTooBigForBuffer {
					log.Warning("Got oversize ws message, closing connection")
					err := s.conn.WriteMessage(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseMessageTooBig, fmt.Sprintf("ws messages must be at most %d bytes", maxMessageSize)),
					)
					if err != nil {
						return true, fmt.Errorf("on oversize ws message: %w", err)
					}
				} else if err != nil {
					return true, fmt.Errorf("when reading ws message: %w", err)
				}

				// NOTE(ben): Maybe someday we'll have the textual version of these messages, but for now
				// only binary is supported.
				if read.messageType != websocket.BinaryMessage {
					err := s.conn.WriteMessage(
						websocket.BinaryMessage,
						CreateErrorEvent(s.buf[:], "only binary ws messages are supported"),
					)
					if err != nil {
						return true, fmt.Errorf("on non-binary ws message: %w", err)
					}
					return false, nil
				}

				if err := s.handleClientEvent(event); err != nil {
					return true, fmt.Errorf("in handleClientMessage: %w", err)
				}

				return false, nil
			}()
			utils.Assert(!(!exit && exitErr != nil), "saw an exitErr but we weren't exiting")
			if exit {
				if exitErr != nil {
					s.unexpectedErr = exitErr
				}
				return
			}
		}
	}
}

// Handles a single WebSocket message from the client. The error return is ONLY for unexpected
// errors that should terminate the connection.
func (s *EventStreamSession) handleClientEvent(eventData []byte) error {
	p := &eventParser{data: eventData}

	eventType, err := p.ReadByte("event type")
	if err != nil {
		return s.sendErrorToClient("missing event type")
	}
	switch EventType(eventType) {
	case ETRecord:
		s.handleClientRecord(p)

	case ETACK:
		// Client acknowledging receipt of server events.
		sn, err := p.ReadS64("client ACK SN")
		if err != nil {
			return s.sendErrorToClient("missing SN in client ACK")
		}
		s.clientACK = max(sn, -1) // NOTE(ben): -1 is reserved as having ACKed nothing
		s.clientNeedsSync.Wake()

	default:
		return s.sendErrorToClient("unknown event type %v", eventType)
	}

	return nil
}

func (s *EventStreamSession) handleClientRecord(p *eventParser) error {
	recordType, err := p.ReadByte("record type")
	if err != nil {
		return s.sendErrorToClient("missing event type")
	}
	switch RecordType(recordType) {
	case RTMessage:
		text, err := p.ReadUTF8String("message text")
		if err != nil {
			return s.sendErrorToClient("%s", err)
		}
		newEvents <- Event{
			Type:        RTMessage,
			MessageText: text,
		}
	default:
		return s.sendErrorToClient("unknown event type %v", recordType)
	}

	return nil
}

func (s *EventStreamSession) sendErrorToClient(msg string, args ...any) error {
	err := s.conn.WriteMessage(websocket.BinaryMessage, CreateErrorEvent(s.buf[:], msg, args...))
	if err != nil {
		return err
	}
	return nil
}
