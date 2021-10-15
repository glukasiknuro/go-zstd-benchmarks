package main

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"fmt"
	"github.com/dustin/go-humanize"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)
import "flag"

var addr = flag.String("addr", "", "GRPC address of the ByteStream service")
var rootDir = flag.String("dir", ".", "Root directory to scan for files")
var iterations = flag.Int("download_iterations", 0, "Number of times to download each file")
var uploadIterations = flag.Int("upload_iterations", 0, "Number of times to upload each file")
var parallel = flag.Int("parallel", 2, "Number of parallel downloads/uploads to perform")

type FileData struct {
	Path   string
	Size   int64
	Sha256 string

	MarshalledHash []byte
}

type EnhancedFileData struct {
	File       *FileData
	ExtraBytes []byte // extra bytes appended to the file
	ModifiedSha256 string // sha256 of file with appended data
}

func getFileSha256(path string) ([]byte, string) {
	f, err := os.Open(path)
	noError(err)

	h := sha256.New()
	_, err = io.Copy(h, f)
	noError(err)

	m, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	noError(err)

	return m, hex.EncodeToString(h.Sum(nil))
}

func extendSha256(marshalledHash []byte, extraBytes []byte) string {
	h := sha256.New()
	noError(h.(encoding.BinaryUnmarshaler).UnmarshalBinary(marshalledHash))
	_, err := h.Write(extraBytes)
	noError(err)
	return hex.EncodeToString(h.Sum(nil))
}

func getFiles() []*FileData {
	var files []*FileData
	err := filepath.Walk(*rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Ignoring error for: %s, err: %s\n", path, err)
			return nil
		}
		if info.Mode().IsRegular() {
			h, s := getFileSha256(path)
			files = append(files, &FileData{Path: path, Size: info.Size(), MarshalledHash: h, Sha256: s})
		}
		return nil
	})
	noError(err)
	return files
}

func uploadFiles(client bytestream.ByteStreamClient, files []*FileData) {
	for _, f := range files {
		err := uploadFile(client, f.Path, f.Size, f.Sha256)
		if err != nil {
			panic(fmt.Sprintf("Unable to upload %v: %s", f, err))
		}
	}
}

func createClient() bytestream.ByteStreamClient {
	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	noError(err)
	return bytestream.NewByteStreamClient(conn)
}

func downloadBenchmark(files []*FileData) {
	numDownloads := *iterations * len(files)
	toDownload := make(chan *FileData, numDownloads)
	downloaded := make(chan *FileData, numDownloads)
	for i := 0; i < *iterations; i++ {
		for _, f := range files {
			toDownload <- f
		}
	}

	startDownload := time.Now()
	for i := 0; i < *parallel; i++ {
		go func(clientIdx int) {
			client := createClient()
		F:
			for {
				select {
				case f := <-toDownload:
					noError(downloadFile(client, f.Size, f.Sha256))
					downloaded <- f

				default:
					break F
				}
			}
			log.Printf("DOWNLOAD  Client %d finished", clientIdx)
		}(i)
	}

	var downloadedSize uint64
	for i := 0; i < numDownloads; i++ {
		f := <-downloaded
		downloadedSize += uint64(f.Size)
		speed := uint64(float64(downloadedSize) / time.Now().Sub(startDownload).Seconds())
		log.Printf("DOWNLOAD  [%d/%d] downloaded size: %s  avg throughput: %s/s",
			i+1, numDownloads, humanize.Bytes(downloadedSize), humanize.Bytes(speed))
	}
}

func uploadBenchmark(files []*FileData) {
	numUploads := *uploadIterations * len(files)
	toUpload := make(chan *EnhancedFileData, numUploads)
	uploaded := make(chan *EnhancedFileData, numUploads)

	for i := 0; i < *uploadIterations; i++ {
		extraBytes := make([]byte, 16)
		_, err := rand.Read(extraBytes)
		noError(err)
		for _, f := range files {
			e := &EnhancedFileData{
				File:           f,
				ExtraBytes:     extraBytes,
				ModifiedSha256: extendSha256(f.MarshalledHash, extraBytes),
			}
			toUpload <- e
		}
	}

	startUpload := time.Now()
	for i := 0; i < *parallel; i++ {
		go func(clientIdx int) {
			client := createClient()
		F:
			for {
				select {
				case f := <-toUpload:
					r, err := os.Open(f.File.Path)
					noError(err)
					mr := io.MultiReader(r, bytes.NewReader(f.ExtraBytes))
					noError(uploadFromReader(client, mr, f.File.Size+int64(len(f.ExtraBytes)), f.ModifiedSha256))
					uploaded <- f
					noError(r.Close())
				default:
					break F
				}
			}
			log.Printf("UPLOAD    Client %d finished", clientIdx)
		}(i)
	}

	var uploadedSize uint64
	for i := 0; i < numUploads; i++ {
		f := <-uploaded
		uploadedSize += uint64(f.File.Size + int64(len(f.ExtraBytes)))
		speed := uint64(float64(uploadedSize) / time.Now().Sub(startUpload).Seconds())
		log.Printf("UPLOAD   [%d/%d] uploaded size: %s  avg throughput: %s/s",
			i+1, numUploads, humanize.Bytes(uploadedSize), humanize.Bytes(speed))
	}
}

func main() {
	flag.Parse()
	if *uploadIterations == 0 && *iterations == 0 {
		log.Fatal("Need to specify at least one of -upload_iterations or -download_iterations")
	}

	rand.Seed(time.Now().UTC().UnixNano())

	root, err := filepath.Abs(*rootDir)
	if err != nil {
		panic(err)
	}
	log.Printf("Scanning files in: %s\n", root)

	files := getFiles()
	if len(files) == 0 {
		fmt.Printf("No files found")
		return
	}

	start := time.Now()
	uploadFiles(createClient(), files)
	log.Printf("Uploaded base files in %s", time.Now().Sub(start))

	var w sync.WaitGroup
	if *iterations > 0 {
		w.Add(1)
		go func() {
			downloadBenchmark(files)
			w.Done()
		}()
	}
	if *uploadIterations > 0 {
		w.Add(1)
		go func() {
			uploadBenchmark(files)
			w.Done()
		}()
	}
	w.Wait()
}

func noError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
