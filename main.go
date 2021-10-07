package main

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"
)
import "flag"

var rootDir = flag.String("dir", ".", "Root directory to scan for files")
var iterations = flag.Int("iterations", 1, "Number of times to process set of input files")
var tmpDir = flag.String("tmp_dir", "/tmp", "Temporary directory to compress and decompress to")
var maxFiles = flag.Int("max_files", 0, "Maximum number of files to process - 0 for all")
var includeStime = flag.Bool("include_stime", false, "Include system time")
var useWallTime = flag.Bool("use_walltime", false, "Use walltime instead of CPU time")

type FileData struct {
	Path string
	Size int64
}

func getFiles() []FileData {
	var files []FileData
	err := filepath.Walk(*rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("Ignoring error for: %s, err: %s\n", path, err)
			return nil
		}
		if info.Mode().IsRegular() {
			files = append(files, FileData{Path: path, Size: info.Size()})
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return files
}

func close(f io.Closer) {
	if err := f.Close(); err != nil {
		panic(err)
	}
}

func getSize(name string) int64 {
	f, err := os.OpenFile(name, os.O_RDONLY, 0)
	defer close(f)
	if err != nil {
		panic(err)
	}
	if stat, err := f.Stat(); err != nil {
		panic(err)
	} else {
		return stat.Size()
	}
}

func remove(path string) {
	if err := os.Remove(path); err != nil {
		panic(err)
	}
}

var globalStartTime = time.Now()
func getCpuTime() time.Duration {
	if *useWallTime {
		return time.Now().Sub(globalStartTime)
	}
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		panic(err)
	}
	sec := rusage.Utime.Sec
	usec := rusage.Utime.Usec
	if *includeStime {
		sec += rusage.Stime.Sec
		usec += rusage.Stime.Usec
	}
	return time.Duration(sec*1000000000 + usec*1000)
}

func compressFiles(files []FileData, encoder Encoder, decoder Decoder) (inSize, outSize int64, encTime, decTime time.Duration) {
	toProcess := len(files)
	if *maxFiles > 0 && *maxFiles < toProcess {
		toProcess = *maxFiles
	}

	for j := 0; j < *iterations; j++ {
		for i := 0; i < toProcess; i++ {
			inSize += getSize(files[i].Path)
			f, err := os.OpenFile(files[i].Path, os.O_RDONLY, 0)
			if err != nil {
				panic(err)
			}

			// Compress
			compressed, err := ioutil.TempFile(*tmpDir, filepath.Base(files[i].Path)+"_zstd_*")
			if err != nil {
				panic(err)
			}

			enStart := getCpuTime()
			w := encoder.NewWriter(compressed)
			if _, err = io.Copy(w, f); err != nil {
				panic(err)
			}
			close(w)
			close(f)

			encTime += getCpuTime() - enStart
			outSize += getSize(compressed.Name())

			// Decompress
			cmp, err := os.OpenFile(compressed.Name(), os.O_RDONLY, 0)
			if err != nil {
				panic(err)
			}
			decompressed, err := ioutil.TempFile(*tmpDir, filepath.Base(files[i].Path)+"_org_*")
			if err != nil {
				panic(err)
			}

			decStart := getCpuTime()
			r := decoder.NewReader(cmp)
			if _, err = io.Copy(decompressed, r); err != nil {
				panic(err)
			}
			decTime += getCpuTime() - decStart

			close(cmp)
			close(decompressed)

			if getSize(files[i].Path) != getSize(decompressed.Name()) {
				fmt.Printf("\nInvalid output size: %d vs %d\n", getSize(f.Name()), getSize(decompressed.Name()))
				panic(compressed.Name() + " " + decompressed.Name())
			}

			remove(compressed.Name())
			remove(decompressed.Name())
		}
	}

	return
}

type Compressor struct {
	e Encoder
	d Decoder
}

func processFiles(files []FileData) {
	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}
	fmt.Printf(
		"Got %s file(s), total size: %s (avg: %s)\n",
		humanize.Comma(int64(len(files))),
		humanize.Bytes(uint64(totalSize)),
		humanize.Bytes(uint64(totalSize)/uint64(len(files))))

	compressors := []Compressor{
		{Identity, IdentityDecoder},
		{ZstdDefault, ZstdDecoder},
		{ZstdSpeed, ZstdDecoder},
		{ZstdCgoDefault, ZstdCgoDecoder},
		{ZstdCgoSpeed, ZstdCgoDecoder},
		//{ Gzip, GzipDecoder },
	}

	fmt.Printf(
		"%20s  %10s %10s %6s %14s %14s %10s %10s\n",
		"compressor", "inSize", "outSize", "ratio", "enc_time", "dec_time", "enc_speed", "dec_speed")

	for _, c := range compressors {
		fmt.Printf("%20s  ", c.e)
		inSize, outSize, encTime, decTime := compressFiles(files, c.e, c.d)
		ratio := float64(outSize) / float64(inSize)

		encSpeed := float64(inSize) / encTime.Seconds()
		decSpeed := float64(inSize) / decTime.Seconds()

		encTime.String()

		fmt.Printf(
			"%10s %10s %6.2f %14s %14s %10s %10s\n",
			humanize.Bytes(uint64(inSize)),
			humanize.Bytes(uint64(outSize)),
			ratio*100,
			encTime,
			decTime,
			humanize.Bytes(uint64(encSpeed)) + "/s",
			humanize.Bytes(uint64(decSpeed)) + "/s")
	}
}

func main() {
	flag.Parse()

	root, err := filepath.Abs(*rootDir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Scanning files in: %s\n", root)

	files := getFiles()
	if len(files) == 0 {
		fmt.Printf("No files found")
		return
	}
	processFiles(files)
}
