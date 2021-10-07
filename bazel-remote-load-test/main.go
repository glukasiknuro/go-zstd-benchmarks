package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dustin/go-humanize"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)
import "flag"

var addr = flag.String("addr", "", "GRPC address of the ByteStream service")
var rootDir = flag.String("dir", ".", "Root directory to scan for files")
var iterations = flag.Int("iterations", 1, "Number of times to download each file")
var parallel = flag.Int("parallel_downloads", 2, "Number of parallel downloads to perform")

type FileData struct {
	Path   string
	Size   int64
	Sha256 string
}

func getFileSha256(path string) string {
	f, err := os.Open(path)
	noError(err)

	h := sha256.New()
	_, err = io.Copy(h, f)
	noError(err)

	return hex.EncodeToString(h.Sum(nil))
}

func getFiles() []FileData {
	var files []FileData
	err := filepath.Walk(*rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Ignoring error for: %s, err: %s\n", path, err)
			return nil
		}
		if info.Mode().IsRegular() {
			files = append(files, FileData{Path: path, Size: info.Size(), Sha256: getFileSha256(path)})
		}
		return nil
	})
	noError(err)
	return files
}

func close(f io.Closer) {
	if err := f.Close(); err != nil {
		panic(err)
	}
}

func uploadFiles(client bytestream.ByteStreamClient, files []FileData) {
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

func main() {
	flag.Parse()

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
	log.Printf("Uploaded files in %s", time.Now().Sub(start))

	numDownloads := *iterations * len(files)
	toDownload := make(chan FileData, numDownloads)
	downloaded := make(chan FileData, numDownloads)
	for i := 0; i < *iterations; i++ {
		for _, f := range files {
			toDownload <- f
		}
	}

	startDownload := time.Now()
	for i := 0; i < *parallel; i++ {
		go func(clientIdx int) {
			client := createClient()
			F: for {
				select {
				case f := <-toDownload:
					noError(downloadFile(client, f.Size, f.Sha256))
					downloaded <- f

				default:
					break F
				}
			}
			log.Printf("Client %d finished", clientIdx)
		}(i)
	}

	var downloadedSize uint64
	for i := 0; i < numDownloads; i++ {
		f := <-downloaded
		downloadedSize += uint64(f.Size)
		speed := uint64(float64(downloadedSize) / time.Now().Sub(startDownload).Seconds())
		log.Printf("[%d/%d] downloaded size: %s  avg throughput: %s/s",
			i+1, numDownloads , humanize.Bytes(downloadedSize), humanize.Bytes(speed))
	}
}

func noError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
