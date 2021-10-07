package main

import (
	"fmt"
	"github.com/klauspost/compress/zstd"
	"io"
	"io/ioutil"
	"os"
	"time"
)
import "flag"

var inFile = flag.String("in", "", "Input file to compress")
var outFile = flag.String("out", "", "Output file")
var level = flag.Int("level", int(zstd.SpeedDefault), "Compression level")
var encodeAll = flag.Bool("encode_all", false, "Instead of encoding to output file, use EncodeAll on bytes read to memory")

func main() {
	flag.Parse()

	in, err := os.OpenFile(*inFile, os.O_RDONLY, 0)
	if err != nil {
		panic(err)
	}

	stat, err := in.Stat()
	if err != nil {
		panic(err)
	}
	inSize := stat.Size()

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevel(*level)))
	if err != nil {
		panic(err)
	}

	var duration time.Duration
	if *encodeAll {
		bytes, err := ioutil.ReadAll(in)
		if err != nil {
			panic(err)
		}
		start := time.Now()
		enc.EncodeAll(bytes, make([]byte, 0, len(bytes)))
		duration = time.Now().Sub(start)
	} else {
		out, err := os.OpenFile(*outFile, os.O_WRONLY|os.O_CREATE, 0)
		if err != nil {
			panic(err)
		}

		enc.Reset(out)

		start := time.Now()
		_, err = io.Copy(enc, in)
		if err != nil {
			panic(err)
		}
		duration = time.Now().Sub(start)

		if err := enc.Close(); err != nil {
			panic(err)
		}
		if err := in.Close(); err != nil {
			panic(err)
		}
		if err := out.Close(); err != nil {
			panic(err)
		}
	}

	speed := float64(inSize) / (1 << 20) / duration.Seconds()
	fmt.Printf("File size: %d time: %s speed: %.0f MiB/s", inSize, duration, speed)
}
