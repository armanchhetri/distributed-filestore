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

func (br *ByteReader) Read0Byte() error {
	buf := make([]byte, 0)
	_, err := br.reader.Read(buf)
	if err != nil {
		return err
	}
	return nil
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

type UntilReader struct {
	r     io.Reader
	until []byte
}

func ReadUntil(r io.Reader, until []byte) error {
	br := NewByteReader(r)
	subKey := 0

	for {
		ch, err := br.ReadByte()
		if err != nil {
			return err
		}
		if until[subKey] == ch {
			subKey++
			if subKey == len(until) {
				break
			}
		} else if subKey != 0 {
			subKey = 0
		}
	}

	return nil
}

// returns with until
func ReadUntilAndGet(r io.Reader, until []byte) ([]byte, error) {
	br := NewByteReader(r)
	subKey := 0
	var readBytes []byte

	for {
		ch, err := br.ReadByte()
		if err != nil {
			return nil, err
		}
		readBytes = append(readBytes, ch)

		if until[subKey] == ch {
			subKey++
			if subKey == len(until) {
				break
			}
		} else if subKey != 0 {
			subKey = 0
		}
	}

	return readBytes, nil
}

func ReadUntilGetAfter(r io.Reader, until []byte) (byte, error) {
	err := ReadUntil(r, until)
	if err != nil {
		return 0, err
	}
	br := NewByteReader(r)

	return br.ReadByte()
}

func PadBytes(b []byte, size int) []byte {
	p := make([]byte, size)
	copy(p, b)
	return p
}
