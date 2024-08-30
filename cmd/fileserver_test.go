package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFileServerReadUntilHeader(t *testing.T) {
	fs := FileServer{}
	validHeaderAtFirst := "BEGINMESG\x01"
	validHeaderInMiddle := "aklsjdfsfBEGINMESG\x02sdfasd"
	r := bytes.NewReader([]byte(validHeaderAtFirst))

	_, err := fs.readUntilHeader(r)
	if err != nil {
		t.Fatalf("valid header should pass, but got (%v)", err)
	}

	r = bytes.NewReader([]byte(validHeaderInMiddle))
	message, err := fs.readUntilHeader(r)
	if err != nil {
		t.Fatalf("valid header in middle should pass but got (%v)", err)
	}
	fmt.Println(byte(message))
	if message != 0x02 {
		t.Fatal("wrong message kind ")
	}
}
