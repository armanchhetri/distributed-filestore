package helpers_test

import (
	"bytes"
	"testing"

	"github.com/armanchhetri/datastore/packages/helpers"
	"github.com/stretchr/testify/assert"
)

func TestReadByte(t *testing.T) {
	r := bytes.NewReader([]byte("hello this is a test for read byte"))
	br := helpers.NewByteReader(r)

	firstByte, _ := br.ReadByte()
	secondByte, err := br.ReadByte()
	assert.Nil(t, err)

	assert.Equal(t, firstByte, byte('h'))
	assert.Equal(t, secondByte, byte('e'))

	br.UnreadByte()

	// should be e again when read later

	secondByte, _ = br.ReadByte()
	assert.Equal(t, secondByte, byte('e'))
}
