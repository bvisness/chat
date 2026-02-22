package utils

import (
	"errors"
	"io"
)

var ErrorTooBigForBuffer = errors.New("data was too big for buffer")

func ReadAllInto(r io.Reader, buf []byte) ([]byte, error) {
	sizeSoFar := 0
	for {
		if sizeSoFar >= len(buf) {
			break
		}

		n, err := r.Read(buf[sizeSoFar:])
		sizeSoFar += n
		if err == io.EOF {
			break
		} else if err != nil {
			return buf[:sizeSoFar], err
		}
	}

	if sizeSoFar == len(buf) {
		// Do one extra read to make sure the buffer isn't overfull.
		var extra [1]byte
		n, _ := r.Read(extra[:])
		if n > 0 {
			return buf[:sizeSoFar], ErrorTooBigForBuffer
		}
	}

	return buf[:sizeSoFar], nil
}
