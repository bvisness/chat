package utils

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadAllInto(t *testing.T) {
	t.Run("underfull", func(t *testing.T) {
		var buf [10]byte
		r := bytes.NewBufferString("hello")
		res, err := ReadAllInto(r, buf[:])
		assert.Equal(t, []byte("hello"), res)
		assert.Nil(t, err)
	})
	t.Run("full", func(t *testing.T) {
		var buf [10]byte
		r := bytes.NewBufferString("hellohello")
		res, err := ReadAllInto(r, buf[:])
		assert.Equal(t, []byte("hellohello"), res)
		assert.Nil(t, err)
	})
	t.Run("overfull by 1", func(t *testing.T) {
		var buf [10]byte
		r := bytes.NewBufferString("hellohello!")
		_, err := ReadAllInto(r, buf[:])
		assert.ErrorIs(t, err, ErrorTooBigForBuffer)
	})
	t.Run("overfull", func(t *testing.T) {
		var buf [10]byte
		r := bytes.NewBufferString("hellohellohello")
		_, err := ReadAllInto(r, buf[:])
		assert.ErrorIs(t, err, ErrorTooBigForBuffer)
	})

	t.Run("multiple reads (evil!)", func(t *testing.T) {
		var buf [10]byte
		r := ChunkyReader{r: bytes.NewBufferString("hellohello"), chunkSize: 5}
		res, err := ReadAllInto(r, buf[:])
		assert.Equal(t, []byte("hellohello"), res)
		assert.Nil(t, err)
	})
	t.Run("multiple reads (evil! the sequel)", func(t *testing.T) {
		var buf [10]byte
		r := ChunkyReader{r: bytes.NewBufferString("hellohello"), chunkSize: 4}
		res, err := ReadAllInto(r, buf[:])
		assert.Equal(t, []byte("hellohello"), res)
		assert.Nil(t, err)
	})

	t.Run("error immediately", func(t *testing.T) {
		var buf [10]byte
		expected := errors.New("zoinks!")
		r := CrashyReader{r: bytes.NewBufferString("hellohello"), err: expected, after: 0}
		res, err := ReadAllInto(r, buf[:])
		assert.Len(t, res, 0)
		assert.Equal(t, expected, err)
	})
	t.Run("error after a bit", func(t *testing.T) {
		var buf [10]byte
		expected := errors.New("zoinks!")
		r := CrashyReader{r: bytes.NewBufferString("hellohello"), err: expected, after: 5}
		res, err := ReadAllInto(r, buf[:])
		assert.Equal(t, []byte("hello"), res)
		assert.Equal(t, expected, err)
	})
}

type ChunkyReader struct {
	r         io.Reader
	chunkSize int
}

func (c ChunkyReader) Read(p []byte) (n int, err error) {
	b := make([]byte, c.chunkSize)
	n, err = c.r.Read(b)
	copy(p, b[:n])
	return
}

var _ io.Reader = ChunkyReader{}

type CrashyReader struct {
	r     io.Reader
	err   error
	after int
}

func (c CrashyReader) Read(p []byte) (n int, err error) {
	b := make([]byte, c.after)
	n, err = c.r.Read(b)
	copy(p, b[:n])
	if err == nil {
		err = c.err
	}
	return
}
