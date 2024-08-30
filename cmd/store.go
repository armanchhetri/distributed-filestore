package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"time"
)

const (
	DEFAULT_STORAGE_PATH = "./storage"
)

type (
	FileKey               string
	PathTransformFunction func(FileKey) FilePath
)

type FilePath struct {
	DirPath  string
	FileName string
}

func (f *FilePath) FullPath() string {
	return path.Join(f.DirPath, f.FileName)
}

// Content Addressable transform function
func CASPathTransformFunc(key FileKey) FilePath {
	sum := sha1.Sum([]byte(key))
	sumStr := hex.EncodeToString(sum[:])
	pathNameBlockSize := 5
	numOfBlock := len(sumStr) / pathNameBlockSize // should be 8 = 40/5
	dirPathSlice := make([]string, numOfBlock)
	for i := 0; i < numOfBlock; i++ {
		start, end := i*pathNameBlockSize, (i+1)*pathNameBlockSize
		dirPathSlice[i] = sumStr[start:end]
	}
	return FilePath{
		DirPath:  path.Join(dirPathSlice...),
		FileName: sumStr,
	}
}

var DefaultPathTransformFunc = func(key FileKey) FilePath {
	return FilePath{string(key), string(key)}
}

type FileStreamer struct {
	file          *os.File
	readCount     int
	deadlineCtx   context.Context
	cancelContext context.CancelFunc
}

// close the file if the SteamReader is not read within 2 sec after first read
func (s *FileStreamer) Read(b []byte) (int, error) {
	n, err := s.file.Read(b)

	if s.readCount == 0 {
		deadline := time.Now().Add(2 * time.Second)
		ctx, cancelContext := context.WithDeadline(context.Background(), deadline)
		s.deadlineCtx = ctx
		s.cancelContext = cancelContext
		go s.handleDeadLine()
	}
	if errors.Is(err, io.EOF) {
		s.cancelContext()
	}
	s.readCount += n
	return n, err
}

func (s *FileStreamer) handleDeadLine() {
	<-s.deadlineCtx.Done()
	fmt.Println("Closing after a deadline")
	s.file.Close()
}

type Store struct {
	StoreOpts
}

type StoreOpts struct {
	PathTransformFunction PathTransformFunction
	Root                  string
}

func NewStore(opts StoreOpts) *Store {
	fmt.Println(opts)
	if opts.Root == "" {
		opts.Root = DEFAULT_STORAGE_PATH
	}
	transFunction := opts.PathTransformFunction
	opts.PathTransformFunction = func(key FileKey) FilePath {
		filepath := transFunction(key)
		filepath.DirPath = path.Join(opts.Root, filepath.DirPath)
		return filepath
	}

	return &Store{
		opts,
	}
}

func (s *Store) Stat(key FileKey) (os.FileInfo, error) {
	path := s.PathTransformFunction(key)
	return os.Stat(path.FullPath())
}

func (s *Store) StreamFile(key FileKey, w io.Writer) error {
	filepath := s.PathTransformFunction(key)
	f, err := os.Open(filepath.FullPath())
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}

func (s *Store) WriteStream(key FileKey, r io.Reader) error {
	filepath := s.PathTransformFunction(key)

	if err := os.MkdirAll(filepath.DirPath, os.ModePerm); err != nil {
		return err
	}
	f, err := os.Create(filepath.FullPath())
	defer f.Close()
	if err != nil {
		return err
	}
	n, err := io.Copy(f, r)
	if err != nil {
		return err
	}
	log.Printf("Written %d bytes to %s file", n, filepath.FullPath())

	return nil
}

// Should call Read for the StreamReader at least once, otherwise the underlying file won't get closed
func (s *Store) GetFileStreamer(key FileKey) (*FileStreamer, error) {
	if !s.Has(key) {
		return nil, fmt.Errorf("File with the key %s does not exist", key)
	}
	filepath := s.PathTransformFunction(key)
	f, err := os.Open(filepath.FullPath())
	if err != nil {
		return nil, err
	}

	return &FileStreamer{file: f}, nil
}

func (s *Store) Delete(key FileKey) error {
	if !s.Has(key) {
		return fmt.Errorf("File with the key %s does not exist", key)
	}
	filename := s.PathTransformFunction(key)
	err := os.RemoveAll(filename.FullPath())
	if err == nil {
		log.Printf("Deleted [%s] from disk", filename.FileName)
	}
	return err
}

func (s *Store) Has(key FileKey) bool {
	filepath := s.PathTransformFunction(key)
	_, err := os.Stat(filepath.FullPath())
	return !errors.Is(err, os.ErrNotExist)
}

func (s *Store) Cleanup() error {
	return os.RemoveAll(s.Root)
}
