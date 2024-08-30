package helpers

import (
	"errors"
	"io"
)

type ByteReader struct {
	reader io.Reader
	buffer []byte
	cursor int
}

func NewByteReader(r io.Reader) ByteReader {
	return ByteReader{
		reader: r,
		buffer: make([]byte, 1),
		cursor: -1,
	}
}

func (br *ByteReader) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, errors.New("cannot read into empty buffer")
	}
	oneByte, err := br.ReadByte()
	if err != nil {
		return 0, err
	}
	b[0] = oneByte
	return 1, nil
}

func (br *ByteReader) ReadByte() (byte, error) {
	if br.cursor < len(br.buffer)-1 {
		br.cursor++
		return br.buffer[br.cursor], nil
	}
	buf := make([]byte, 1)
	_, err := br.reader.Read(buf)
	if err != nil {
		return 0, err
	}
	br.buffer = append(br.buffer, buf[0])
	br.cursor++
	return buf[0], err
}

func (br *ByteReader) UnreadByte() error {
	if br.cursor == -1 {
		return errors.New("no byte to unread")
	}
	br.cursor--
	return nil
}
