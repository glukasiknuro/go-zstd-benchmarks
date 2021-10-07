package main

import (
	"compress/gzip"
	zstdcgo "github.com/DataDog/zstd"
	"github.com/klauspost/compress/zstd"
	"io"
)

type Decoder int

const (
	IdentityDecoder = iota
	ZstdDecoder
	ZstdCgoDecoder
	GzipDecoder
)

func (d Decoder) String() string {
	return [...]string{"IDENTITY", "ZSTD", "ZSTD CGO DEFAULT", "GZIP"}[d]
}

var zstdDecoder *zstd.Decoder

func (d Decoder) NewReader(r io.Reader) io.Reader {
	switch d {
	case IdentityDecoder:
		return r
	case ZstdDecoder:
		// This is inherently not thread safe
		if zstdDecoder == nil {
			var err error
			zstdDecoder, err = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
			if err != nil {
				panic(err)
			}
		}
		if err := zstdDecoder.Reset(r); err != nil {
			panic(err)
		}
		return zstdDecoder
	case ZstdCgoDecoder:
		return zstdcgo.NewReader(r)
	case GzipDecoder:
		if reader, err := gzip.NewReader(r); err != nil {
			panic(err)
		} else {
			return reader
		}
	default:
		panic(d)
	}
}
