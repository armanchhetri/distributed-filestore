package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCASPathTransformFunction(t *testing.T) {
	fileKey := "really_cool_pictures"
	expected := FilePath{
		"3c115/8bc72/b14ef/64b58/fd7c9/5a61d/c5ca0/1f838",
		"3c1158bc72b14ef64b58fd7c95a61dc5ca01f838",
	}
	if dirPath := CASPathTransformFunc(FileKey(fileKey)); dirPath != expected {
		t.Errorf("Invalid path formation")
	}
}

func TestStore_GetFileStreamer(t *testing.T) {
	filekey := "normalfilekey"
	reader := strings.NewReader("This is my data")
	storeOpts := StoreOpts{PathTransformFunction: CASPathTransformFunc, Root: ""}
	st := NewStore(storeOpts)
	if err := st.WriteStream(FileKey(filekey), reader); err != nil {
		t.Error(err)
	}

	fstreamer, err := st.GetFileStreamer(FileKey(filekey))
	if err != nil {
		t.Error(err)
	}

	_, err = io.ReadAll(fstreamer)
	if err != nil {
		t.Error(err)
	}
	// this one should close the file streamer
	buf := make([]byte, 5)
	fstreamer, err = st.GetFileStreamer(FileKey(filekey))
	if err != nil {
		t.Error(err)
	}
	for i := 0; i < 2; i++ {
		_, err = fstreamer.Read(buf)
		time.Sleep(1600 * time.Millisecond) // sleeping more than the deadline
	}
	if !errors.Is(err, os.ErrClosed) {
		t.Error(err)
	}
	st.Cleanup()
}

func TestStore_WriteStream(t *testing.T) {
	type fields struct {
		StoreOpts StoreOpts
	}
	type args struct {
		key FileKey
		r   io.Reader
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name:    "sample",
			fields:  fields{StoreOpts: StoreOpts{PathTransformFunction: CASPathTransformFunc, Root: ""}},
			args:    args{key: "storage/mystorage", r: strings.NewReader("This is test content")},
			wantErr: false,
		},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore(tt.fields.StoreOpts)

			if err := s.WriteStream(tt.args.key, tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("Store.WriteStream() error = %v, wantErr %v", err, tt.wantErr)
			}
			s.Cleanup()
		})
	}
}

func TestStore_Delete(t *testing.T) {
	filekey := "willbedeleted"
	reader := strings.NewReader("This is my data")
	storeOpts := StoreOpts{PathTransformFunction: CASPathTransformFunc, Root: ""}
	st := NewStore(storeOpts)
	if err := st.WriteStream(FileKey(filekey), reader); err != nil {
		t.Error(err)
	}
	err := st.Delete(FileKey(filekey))
	if err != nil {
		t.Error(err)
	}
	st.Cleanup()
}
