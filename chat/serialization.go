package chat

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"unicode/utf8"

	"github.com/bvisness/chat/utils"
)

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
		return nil, fmt.Errorf("parsing %s: %w", thing, io.ErrUnexpectedEOF)
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
	return binary.LittleEndian.Uint32(b), nil
}

func (p *messageParser) ReadS64(thing string) (int64, error) {
	b, err := p.ReadBytes(8, thing)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
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
		return "", fmt.Errorf("parsing %s: invalid utf-8", thing)
	}
	return string(b), nil
}

type messageWriter struct {
	buf []byte
	cur int
}

func (w *messageWriter) Check(nBytes int, thing string) error {
	if len(w.buf[w.cur:]) < nBytes {
		return fmt.Errorf("writing %s: %w", thing, io.ErrUnexpectedEOF)
	}
	return nil
}

func (w *messageWriter) Bytes() []byte {
	return w.buf[:w.cur]
}

func (w *messageWriter) WriteByte(v byte, thing string) error {
	if err := w.Check(1, thing); err != nil {
		return err
	}
	w.buf[w.cur] = v
	w.cur += 1
	return nil
}

func (w *messageWriter) WriteBytes(v []byte, thing string) error {
	if err := w.Check(len(v), thing); err != nil {
		return err
	}
	n := copy(w.buf[w.cur:], v)
	utils.Assert(n == len(v))
	w.cur += n
	return nil
}

func (w *messageWriter) WriteU32(v uint32, thing string) error {
	if err := w.Check(4, thing); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(w.buf[w.cur:], v)
	w.cur += 4
	return nil
}

func (w *messageWriter) WriteS64(v int64, thing string) error {
	if err := w.Check(8, thing); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(w.buf[w.cur:], uint64(v))
	w.cur += 8
	return nil
}

func (w *messageWriter) WriteString(v string, thing string) error {
	utils.Assert(len(v) < math.MaxUint32)
	if err := w.WriteU32(uint32(len(v)), thing+" length"); err != nil {
		return err
	}
	if err := w.WriteBytes([]byte(v), thing); err != nil {
		return err
	}
	return nil
}
