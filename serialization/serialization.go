package serialization

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"unicode/utf8"

	"github.com/bvisness/chat/utils"
)

type SType uint8

// TODO(ben): This is an obviously inefficient encoding scheme (eight bytes per
// int!) but you are NOT allowed to bikeshed this until you have a working app!
const (
	STError SType = iota
	STEnd
	STObject
	STArray
	STFloat
	STBool
	STString
)

func (t SType) String() string {
	switch t {
	case STError:
		return "STError"
	case STEnd:
		return "STEnd"
	case STObject:
		return "STObject"
	case STArray:
		return "STArray"
	case STFloat:
		return "STFloat"
	case STBool:
		return "STBool"
	case STString:
		return "STString"
	default:
		return "??UNKNOWN??"
	}
}

type Val struct {
	Type SType
	S    string
	F    float64
	B    bool
	Err  error

	Depth int
}

func (v Val) String() string {
	switch v.Type {
	case STError:
		return fmt.Sprintf("ERROR(%s)", v.Err.Error())
	case STEnd:
		return "End"
	case STObject:
		return "Object"
	case STArray:
		return "Array"
	case STFloat:
		return fmt.Sprintf("Float(%f)", v.F)
	case STBool:
		return fmt.Sprintf("Bool(%v)", v.B)
	case STString:
		return fmt.Sprintf("String(\"%s\")", v.S)
	default:
		return "??UNKNOWN??"
	}
}

func ErrVal(err error) Val {
	return Val{Type: STError, Err: err}
}

func StrVal(s string) Val {
	return Val{Type: STString, S: s}
}

func FieldHasType(key Val, expectedKey Val, val Val, expectedType SType, err *error) bool {
	if key != expectedKey {
		return false
	}
	if val.Type != expectedType {
		*err = fmt.Errorf("incorrect type for key %v: expected %v, but got %v", key, expectedType, val.Type)
		return false
	}
	return true
}

type Reader struct {
	Data  []byte
	Cur   int
	Err   error
	Depth int
}

func (r *Reader) fail(at int, err error) error {
	return fmt.Errorf("at offset 0x%X: %w", at, err)
}

func (r *Reader) Check(n int) bool {
	return len(r.Data[r.Cur:]) >= n
}

func (r *Reader) Byte() (byte, bool) {
	if !r.Check(1) {
		return 0, false
	}
	b := r.Data[r.Cur]
	r.Cur += 1
	return b, true
}

func (r *Reader) Bytes(n int) ([]byte, bool) {
	if !r.Check(n) {
		return nil, false
	}
	res := r.Data[r.Cur : r.Cur+n]
	r.Cur += n
	return res, true
}

func (r *Reader) Read() (Val, error) {
	errAt := r.Cur
	t, ok := r.Byte()
	if !ok {
		return Val{}, r.fail(errAt, errors.New("expected value type"))
	}

	switch t := SType(t); t {
	case STEnd:
		r.Depth -= 1
		return Val{Type: STEnd}, nil
	case STObject, STArray:
		r.Depth += 1
		return Val{Type: t, Depth: r.Depth}, nil
	case STFloat:
		errAt = r.Cur
		bytes, ok := r.Bytes(8)
		if !ok {
			return Val{}, r.fail(errAt, errors.New("expected float bytes"))
		}
		f := math.Float64frombits(binary.LittleEndian.Uint64(bytes))
		return Val{Type: STFloat, F: f}, nil
	case STBool:
		errAt = r.Cur
		b, ok := r.Byte()
		if !ok {
			return Val{}, r.fail(errAt, errors.New("expected bool byte"))
		}
		if b > 1 {
			return Val{}, r.fail(errAt, fmt.Errorf("unexpected bool value %X", b))
		}
		return Val{Type: STBool, B: b == 1}, nil
	case STString:
		errAt = r.Cur
		lenBytes, ok := r.Bytes(8)
		if !ok {
			return Val{}, r.fail(errAt, errors.New("expected string length"))
		}
		n := math.Float64frombits(binary.LittleEndian.Uint64(lenBytes))

		errAt = r.Cur
		b, ok := r.Bytes(int(n))
		if !ok {
			return Val{}, r.fail(errAt, errors.New("expected string data"))
		}
		if !utf8.Valid(b) {
			return Val{}, r.fail(errAt, fmt.Errorf("invalid utf-8"))
		}

		return Val{Type: STString, S: string(b)}, nil
	default:
		return Val{}, r.fail(errAt, fmt.Errorf("invalid value type %X", t))
	}
}

func (r *Reader) Object() (Val, error) {
	errAt := r.Cur
	obj, err := r.Read()
	if err != nil {
		return Val{}, err
	}
	if obj.Type != STObject {
		return Val{}, r.fail(errAt, errors.New("expected object"))
	}
	return obj, nil
}

func (r *Reader) Float() (float64, error) {
	errAt := r.Cur
	f, err := r.Read()
	if err != nil {
		return 0, err
	}
	if f.Type != STFloat {
		return 0, r.fail(errAt, errors.New("expected float"))
	}
	return f.F, nil
}

func (r *Reader) String() (string, error) {
	errAt := r.Cur
	s, err := r.Read()
	if err != nil {
		return "", err
	}
	if s.Type != STString {
		return "", r.fail(errAt, errors.New("expected string"))
	}
	return s.S, nil
}

func (r *Reader) FloatField(key Val) (float64, error) {
	errAt := r.Cur
	actual, err := r.Read()
	if err != nil {
		return 0, err
	}
	if actual != key {
		return 0, r.fail(errAt, fmt.Errorf("expected key %v, but got %v", key, actual))
	}
	res, err := r.Float()
	if err != nil {
		return 0, err
	}
	return res, nil
}

func (r *Reader) StringField(key Val) (string, error) {
	errAt := r.Cur
	actual, err := r.Read()
	if err != nil {
		return "", err
	}
	if actual != key {
		return "", r.fail(errAt, fmt.Errorf("expected key %v, but got %v", key, actual))
	}
	res, err := r.String()
	if err != nil {
		return "", err
	}
	return res, nil
}

func (r *Reader) DiscardUntilDepth(depth int) error {
	for r.Depth != depth {
		_, err := r.Read()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Reader) IterObject(obj Val) iter.Seq2[Val, Val] {
	return func(yield func(Val, Val) bool) {
		for {
			if err := r.DiscardUntilDepth(obj.Depth); err != nil {
				yield(ErrVal(err), ErrVal(err))
				break
			}

			key, err := r.Read()
			if err != nil {
				yield(ErrVal(err), ErrVal(err))
				break
			}
			if key.Type == STEnd {
				break
			}

			val, err := r.Read()
			if err != nil {
				yield(ErrVal(err), ErrVal(err))
				break
			}

			if !yield(key, val) {
				break
			}
		}
	}
}

func (r *Reader) IterArray(arr Val) iter.Seq[Val] {
	utils.Assert(arr.Type == STArray, "only array values should be passed to IterArray, but got", arr.Type)
	return func(yield func(Val) bool) {
		for {
			if err := r.DiscardUntilDepth(arr.Depth); err != nil {
				yield(ErrVal(err))
				break
			}

			val, err := r.Read()
			if err != nil {
				yield(ErrVal(err))
				break
			}
			if val.Type == STEnd {
				break
			}
			if !yield(val) {
				break
			}
		}
	}
}

type Writer struct {
	Buf []byte
	Cur int
	Err error
}

type Writable interface {
	Write(w *Writer)
}

func (w *Writer) Written() []byte {
	return w.Buf[:w.Cur]
}

func (w *Writer) Check(nBytes int) {
	if w.Err != nil {
		return
	}
	if len(w.Buf[w.Cur:]) < nBytes {
		w.Err = io.ErrUnexpectedEOF
	}
}

func (w *Writer) RawByte(b byte) {
	w.Check(1)
	if w.Err != nil {
		return
	}

	w.Buf[w.Cur] = b
	w.Cur += 1
}

func (w *Writer) RawBytes(v []byte) {
	w.Check(len(v))
	if w.Err != nil {
		return
	}

	n := copy(w.Buf[w.Cur:], v)
	utils.Assert(n == len(v))
	w.Cur += n
}

func (w *Writer) RawFloat(v float64) {
	w.Check(8)
	if w.Err != nil {
		return
	}

	binary.LittleEndian.PutUint64(w.Buf[w.Cur:], math.Float64bits(v))
	w.Cur += 8
}

func (w *Writer) Byte(b byte) {
	w.Check(1 + 8)
	if w.Err != nil {
		return
	}

	w.RawByte(byte(STFloat))
	binary.LittleEndian.PutUint64(w.Buf[w.Cur:], math.Float64bits(float64(b)))
	w.Cur += 8
}

func (w *Writer) U64(v uint64) {
	w.Check(1 + 8)
	if w.Err != nil {
		return
	}

	w.RawByte(byte(STFloat))
	binary.LittleEndian.PutUint64(w.Buf[w.Cur:], math.Float64bits(float64(v)))
	w.Cur += 8
}

func (w *Writer) S64(v int64) {
	w.Check(1 + 8)
	if w.Err != nil {
		return
	}

	w.RawByte(byte(STFloat))
	binary.LittleEndian.PutUint64(w.Buf[w.Cur:], math.Float64bits(float64(v)))
	w.Cur += 8
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

func (w *Writer) FieldWritable(name string, val Writable) {
	w.Str(name)
	w.Writable(val)
}

func (w *Writer) Object() {
	w.RawByte(byte(STObject))
}

func (w *Writer) End() {
	w.RawByte(byte(STEnd))
}

func (w *Writer) Str(s string) {
	w.RawByte(byte(STString))
	w.RawFloat(float64(len(s)))
	w.RawBytes([]byte(s))
}

func (w *Writer) Writable(v Writable) {
	v.Write(w)
}

func Serialize(v Writable, buf []byte) ([]byte, error) {
	w := Writer{Buf: buf}
	v.Write(&w)
	if w.Err != nil {
		return nil, w.Err
	}
	return w.Buf[:w.Cur], nil
}
