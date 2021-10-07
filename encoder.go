package main

import (
	"compress/gzip"
	zstdcgo "github.com/DataDog/zstd"
	"github.com/klauspost/compress/zstd"
	"io"
)

type Encoder int

const (
	Identity = iota
	ZstdDefault
	ZstdSpeed
	ZstdCgoDefault
	ZstdCgoSpeed
	Gzip
)

func (e Encoder) String() string {
	return [...]string{"IDENTITY", "ZSTD DEFAULT", "ZSTD BEST_SPEED", "ZSTD CGO DEFAULT", "ZSTD CGO SPEED", "GZIP"}[e]
}

var zstdEncoderDefault, zstdEncoderSpeedFastest *zstd.Encoder

func newEncoder(opts ...zstd.EOption) *zstd.Encoder {
	encoder, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		panic(err)
	}
	return encoder
}

func (e Encoder) NewWriter(w io.WriteCloser) io.WriteCloser {
	switch e {
	case Identity:
		return w
	case ZstdDefault:
		if zstdEncoderDefault == nil {
			zstdEncoderDefault = newEncoder()
		}
		zstdEncoderDefault.Reset(w)
		return zstdEncoderDefault
	case ZstdSpeed:
		if zstdEncoderSpeedFastest == nil {
			zstdEncoderSpeedFastest = newEncoder(zstd.WithEncoderLevel(zstd.SpeedFastest))
		}
		zstdEncoderSpeedFastest.Reset(w)
		return zstdEncoderSpeedFastest
	case ZstdCgoSpeed:
		return zstdcgo.NewWriterLevel(w, zstdcgo.BestSpeed)
	case ZstdCgoDefault:
		return zstdcgo.NewWriterLevel(w, zstdcgo.DefaultCompression)
	case Gzip:
		return gzip.NewWriter(w)
	default:
		panic(e)
	}
}
