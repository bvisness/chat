package chat

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"unicode/utf8"

	"github.com/bvisness/chat/utils"
)

type eventParser struct {
	data []byte
	cur  int
}

func (p *eventParser) ReadByte(thing string) (byte, error) {
	res, err := p.ReadBytes(1, thing)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

func (p *eventParser) ReadBytes(n int, thing string) ([]byte, error) {
	if len(p.data[p.cur:]) < n {
		return nil, fmt.Errorf("parsing %s: %w", thing, io.ErrUnexpectedEOF)
	}
	res := p.data[p.cur : p.cur+n]
	p.cur += n
	return res, nil
}

func (p *eventParser) ReadU32(thing string) (uint32, error) {
	b, err := p.ReadBytes(4, thing)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (p *eventParser) ReadS64(thing string) (int64, error) {
	b, err := p.ReadBytes(8, thing)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

func (p *eventParser) ReadUTF8String(thing string) (string, error) {
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

type eventWriter struct {
	buf []byte
	cur int
}

func (w *eventWriter) Check(nBytes int, thing string) error {
	if len(w.buf[w.cur:]) < nBytes {
		return fmt.Errorf("writing %s: %w", thing, io.ErrUnexpectedEOF)
	}
	return nil
}

func (w *eventWriter) Bytes() []byte {
	return w.buf[:w.cur]
}

func (w *eventWriter) WriteByte(v byte, thing string) error {
	if err := w.Check(1, thing); err != nil {
		return err
	}
	w.buf[w.cur] = v
	w.cur += 1
	return nil
}

func (w *eventWriter) WriteBytes(v []byte, thing string) error {
	if err := w.Check(len(v), thing); err != nil {
		return err
	}
	n := copy(w.buf[w.cur:], v)
	utils.Assert(n == len(v))
	w.cur += n
	return nil
}

func (w *eventWriter) WriteU32(v uint32, thing string) error {
	if err := w.Check(4, thing); err != nil {
		return err
	}
	binary.LittleEndian.PutUint32(w.buf[w.cur:], v)
	w.cur += 4
	return nil
}

func (w *eventWriter) WriteS64(v int64, thing string) error {
	if err := w.Check(8, thing); err != nil {
		return err
	}
	binary.LittleEndian.PutUint64(w.buf[w.cur:], uint64(v))
	w.cur += 8
	return nil
}

func (w *eventWriter) WriteString(v string, thing string) error {
	utils.Assert(len(v) < math.MaxUint32)
	if err := w.WriteU32(uint32(len(v)), thing+" length"); err != nil {
		return err
	}
	if err := w.WriteBytes([]byte(v), thing); err != nil {
		return err
	}
	return nil
}
