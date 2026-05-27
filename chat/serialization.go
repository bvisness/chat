package chat

import (
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/bvisness/chat/utils"
)

type parser struct {
	data []byte
	cur  int
}

func (p *parser) ReadByte(thing string) (byte, error) {
	res, err := p.ReadBytes(1, thing)
	if err != nil {
		return 0, err
	}
	return res[0], nil
}

func (p *parser) ReadBytes(n int, thing string) ([]byte, error) {
	if len(p.data[p.cur:]) < n {
		return nil, fmt.Errorf("parsing %s: %w", thing, io.ErrUnexpectedEOF)
	}
	res := p.data[p.cur : p.cur+n]
	p.cur += n
	return res, nil
}

func (p *parser) ReadU32(thing string) (uint32, error) {
	b, err := p.ReadBytes(4, thing)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (p *parser) ReadS64(thing string) (int64, error) {
	b, err := p.ReadBytes(8, thing)
	if err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b)), nil
}

func (p *parser) ReadUTF8String(thing string) (string, error) {
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

type Writer struct {
	buf []byte
	cur int
	err error
}

type Writable interface {
	Write(w *Writer)
}

type SType uint8

const (
	STError SType = iota
	STEnd
	STObject
	STArray
	STS8
	STU8
	STS32
	STU32
	STS64
	STU64
	STF32
	STF64
	STBool
	STString
)

func (w *Writer) Written() []byte {
	return w.buf[:w.cur]
}

func (w *Writer) Check(nBytes int) {
	if w.err != nil {
		return
	}
	if len(w.buf[w.cur:]) < nBytes {
		w.err = io.ErrUnexpectedEOF
	}
}

func (w *Writer) Err() error {
	return w.err
}

func (w *Writer) RawByte(b byte) {
	w.Check(1)
	if w.Err() != nil {
		return
	}

	w.buf[w.cur] = b
	w.cur += 1
}

func (w *Writer) RawBytes(v []byte) {
	w.Check(len(v))
	if w.Err() != nil {
		return
	}

	n := copy(w.buf[w.cur:], v)
	utils.Assert(n == len(v))
	w.cur += n
}

func (w *Writer) Byte(b byte) {
	w.Check(1 + 1)
	if w.Err() != nil {
		return
	}

	w.RawByte(byte(STU8))
	w.buf[w.cur] = b
	w.cur += 1
}

func (w *Writer) U64(v uint64) {
	w.Check(1 + 8)
	if w.Err() != nil {
		return
	}

	w.RawByte(byte(STU64))
	binary.LittleEndian.PutUint64(w.buf[w.cur:], v)
	w.cur += 8
}

func (w *Writer) S64(v int64) {
	w.Check(1 + 8)
	if w.Err() != nil {
		return
	}

	w.RawByte(byte(STS64))
	binary.LittleEndian.PutUint64(w.buf[w.cur:], uint64(v))
	w.cur += 8
}

func (w *Writer) FieldByte(name string, b byte) {
	w.Str(name)
	w.Byte(b)
}

func (w *Writer) FieldS64(name string, v int64) {
	w.Str(name)
	w.S64(v)
}

func (w *Writer) FieldStr(name string, s string) {
	w.Str(name)
	w.Str(s)
}

func (w *Writer) Object() {
	w.RawByte(byte(STObject))
}

func (w *Writer) End() {
	w.RawByte(byte(STEnd))
}

func (w *Writer) Str(s string) {
	w.S64(int64(len(s)))
	w.RawBytes([]byte(s))
}

func (w *Writer) Writable(v Writable) {
	v.Write(w)
}

func Serialize(v Writable, buf []byte) ([]byte, error) {
	w := Writer{buf: buf}
	v.Write(&w)
	if w.Err() != nil {
		return nil, w.Err()
	}
	return w.buf[:w.cur], nil
}
