// Taken from https://gist.github.com/klauspost/1323e05c63d489e211537509dd81e67e
//    See: https://github.com/klauspost/compress/issues/444
// Adapted from : https://gist.github.com/arnehormann/65421048f56ac108f6b5
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
	"time"

	flstd "compress/flate"
	gzstd "compress/gzip"

	"github.com/andybalholm/brotli"
	"github.com/biogo/hts/bgzf"
	"github.com/golang/snappy"
	flkp "github.com/klauspost/compress/flate"
	gzkp "github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/s2"
	zskp "github.com/klauspost/compress/zstd"
	"github.com/klauspost/dedup"
	pgz "github.com/klauspost/pgzip"
	"github.com/klauspost/readahead"
	"golang.org/x/build/pargzip"

	//"github.com/rasky/go-lzo"

	"github.com/pierrec/lz4"
	"github.com/ulikunitz/xz/lzma"
	zstd "github.com/valyala/gozstd"
	dzstd "github.com/DataDog/zstd"
	//"github.com/youtube/vitess/go/cgzip"
)

type NoOp struct{}

func (n NoOp) Read(v []byte) (int, error) {
	return len(v), nil
}

func (n NoOp) Write(v []byte) (int, error) {
	return len(v), nil
}

type SeqGen struct {
	i int
}

func (s *SeqGen) Read(v []byte) (int, error) {
	b := byte(s.i)
	for i := range v {
		v[i], b = b, b+1
	}
	return len(v), nil
}

type closeWrap struct {
	close func()
}

func (c closeWrap) Close() error {
	c.close()
	return nil
}

type Rand struct {
	// uses PCG (http://www.pcg-random.org/)

	state uint64
	inc   uint64
}

const pcgmult64 = 6364136223846793005

func NewRand(seed uint64) *Rand {
	state := uint64(0)
	inc := uint64(seed<<1) | 1
	state = state*pcgmult64 + (inc | 1)
	state += uint64(seed)
	state = state*pcgmult64 + (inc | 1)
	return &Rand{
		state: state,
		inc:   inc,
	}
}

func (r *Rand) Read(v []byte) (int, error) {
	for w := v; len(w) > 0; w = w[4:] {
		old := r.state
		r.state = r.state*pcgmult64 + (r.inc | 1)
		xorshifted := uint32(((old >> 18) ^ old) >> 27)
		rot := uint32(old >> 59)
		rnd := (xorshifted >> rot) | (xorshifted << ((-rot) & 31))
		// ok because len(v) % 4 == 0
		binary.LittleEndian.PutUint32(w, rnd)
	}
	return len(v), nil
}

type wcounter struct {
	n   int
	out io.Writer
}

func (w *wcounter) Write(p []byte) (n int, err error) {
	n, err = w.out.Write(p)
	w.n += n
	return n, err

}

var globalStartTime = time.Now()
func getCpuTime() time.Duration {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		panic(err)
	}
	sec := rusage.Utime.Sec
	usec := rusage.Utime.Usec
	return time.Duration(sec*1000000000 + usec*1000)
}

/*
func (w *wcounter) Close() (err error) {
	cl, ok := w.out.(io.Closer)
	if ok {
		return cl.Close()
	}
	return nil
}
*/

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	rmode := "raw"
	wmode := "gzkp"
	wlevel := -1
	in := "-"
	out := "-"
	cpu := 0
	stats := false
	mem := false
	header := true
	numRuns := 1
	var closers []func() error

	flag.StringVar(&rmode, "r", rmode, "read mode (raw|flatekp|flatestd|gzkp|pgzip|cgzip|gzstd|zero|seq|rand)")
	flag.StringVar(&wmode, "w", wmode, "write mode (raw|flatekp|flatestd|gzkp|pgzip|gzstd|cgzip|none)")
	flag.StringVar(&in, "in", rmode, "input file name, default is '-', stdin")
	flag.StringVar(&out, "out", rmode, "input file name, default is '-', stdin")
	flag.IntVar(&wlevel, "l", wlevel, "compression level (-2|-1|0..9)")
	flag.IntVar(&cpu, "cpu", cpu, "GOMAXPROCS number (0|1...)")
	flag.IntVar(&numRuns, "n", numRuns, "Number of times to run.")
	flag.BoolVar(&stats, "stats", false, "show stats")
	flag.BoolVar(&header, "header", true, "show stats header")
	flag.BoolVar(&mem, "mem", false, "load source file into memory")
	flag.Parse()
	if flag.NArg() > 0 {
		flag.PrintDefaults()
	}
	cpu = runtime.GOMAXPROCS(cpu)

	if wlevel < -3 || 9 < wlevel {
		panic("compression level -l=x must be (-3,0..9)")
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	var err error
	var wg sync.WaitGroup

	var r io.Reader
	if in == "-" {
		r = os.Stdin
	} else {
		if !mem {
			r, err = os.Open(in)
			if err != nil {
				panic(err)
			}
			r, _ = readahead.NewReaderSize(r, 10, 10<<20)
		} else {
			b, err := ioutil.ReadFile(in)
			if err != nil {
				panic(err)
			}
			for i := 0; i < numRuns; i++ {
				if r == nil {
					r = bytes.NewBuffer(b)
				} else {
					r = io.MultiReader(r, bytes.NewBuffer(b))
				}
			}
		}
	}
	var source bool
	switch rmode {
	case "zero":
		// NoOp writes what the original buffer contained unchanged.
		// As that buffer is initialized with 0 and not changed,
		// NoOp is usable as a very fast zero-reader.
		r = NoOp{}
		source = true
	case "seq":
		r = &SeqGen{}
		source = true
	case "rand":
		r = NewRand(0xdeadbeef)
		source = true
	case "raw":
	case "mem":
		b, err := ioutil.ReadFile(in)
		if err != nil {
			panic(err)
		}
		r = bytes.NewBuffer(b)
	case "gzkp":
		var gzr *gzkp.Reader
		if gzr, err = gzkp.NewReader(r); err == nil {
			closers = append(closers, gzr.Close)
			r = gzr
		}
	case "bgzf":
		var gzr *bgzf.Reader
		if gzr, err = bgzf.NewReader(r, cpu); err == nil {
			closers = append(closers, gzr.Close)
			r = gzr
		}
	case "pgzip":
		var gzr *pgz.Reader
		if gzr, err = pgz.NewReader(r); err == nil {
			closers = append(closers, gzr.Close)
			r = gzr
		}
		/*	case "cgzip":
			var gzr io.ReadCloser
			if gzr, err = cgzip.NewReader(r); err == nil {
				closers = append(closers, gzr.Close)
				r = gzr
			}
		*/
	case "gzstd":
		var gzr *gzstd.Reader
		if gzr, err = gzstd.NewReader(r); err == nil {
			closers = append(closers, gzr.Close)
			r = gzr
		}
	case "flatekp":
		fr := flkp.NewReader(r)
		closers = append(closers, fr.Close)
		r = fr
	case "flatestd":
		fr := flstd.NewReader(r)
		closers = append(closers, fr.Close)
		r = fr
	case "lzma":
		lr, err := lzma.NewReader(r)
		if err != nil {
			panic(err)
		}
		r = lr
	case "lzma2":
		lr, err := lzma.NewReader2(r)
		if err != nil {
			panic(err)
		}
		r = lr
	case "lz4":
		lr := lz4.NewReader(r)
		r = lr
	case "zstd":
		zr := zstd.NewReader(r)
		//closers = append(closers, zr.Close)
		r = zr
	case "zskp":
		r, err = zskp.NewReader(r)
		//closers = append(closers, zr.Close)
	case "dzstd":
		r = dzstd.NewReader(r)
	case "s2":
		sr := s2.NewReader(r)
		r = sr
	case "snappy":
		sr := snappy.NewReader(r)
		r = sr
	default:
		panic("read mode -r=x must be (raw|flatekp|flatestd|gzkp|gzstd|zero|seq|rand)")
	}
	if err != nil {
		panic(err)
	}
	//r = ioutil.NopCloser(r)

	var w io.Writer
	if out == "-" {
		w = os.Stdout
	} else if out == "*" {
		w = ioutil.Discard
		out = "discard"
	} else if out == "verify" {
		preader, pwriter := io.Pipe()
		closers = append(closers, pwriter.Close)
		biow := bufio.NewWriterSize(pwriter, 10<<20)
		closers = append(closers, biow.Flush)
		wg.Add(1)
		go func() {
			reahah, _ := readahead.NewReaderSize(preader, 10, 10<<20)
			gzr, err := gzkp.NewReader(reahah)
			if err != nil {
				panic(err)
			}
			n, err := io.Copy(ioutil.Discard, gzr)
			if err != nil {
				fmt.Println("Error reading input:", err)
				os.Exit(1)
			} else {
				fmt.Println("Read back OK (gzip CRC verified)! bytes:", n)
			}
			wg.Done()
		}()
		w = biow
		out = "verify"
	} else {
		f, err := os.Create(out)
		if err != nil {
			panic(err)
		}
		closers = append(closers, f.Close)
		iow := bufio.NewWriter(f)
		closers = append(closers, iow.Flush)
		w = iow
	}
	outSize := &wcounter{out: w}
	w = outSize

	var sink bool
	switch wmode {
	case "none":
		w = NoOp{}
		sink = true
	case "raw":
	case "gzkp":
		var gzw *gzkp.Writer
		if gzw, err = gzkp.NewWriterLevel(w, wlevel); err == nil {
			closers = append(closers, gzw.Close)
			w = gzw
		}
	case "pgzip":
		var gzw *pgz.Writer
		if gzw, err = pgz.NewWriterLevel(w, wlevel); err == nil {
			closers = append(closers, gzw.Close)
			w = gzw
		}
	case "bgzf":
		var gzw *bgzf.Writer
		if gzw, err = bgzf.NewWriterLevel(w, wlevel, cpu); err == nil {
			closers = append(closers, gzw.Close)
			w = gzw
		}
	case "pargzip":
		var gzw *pargzip.Writer
		gzw = pargzip.NewWriter(w)
		//gzw.UseSystemGzip = false
		closers = append(closers, gzw.Close)
		w = gzw
		/*	case "cgzip":
			var gzw *cgzip.Writer
			if gzw, err = cgzip.NewWriterLevel(w, wlevel); err == nil {
				closers = append(closers, gzw.Close)
				w = gzw
			}*/
	case "br":
		brw := brotli.NewWriterLevel(w, wlevel)
		closers = append(closers, brw.Close)
		w = brw
	case "gzstd":
		var gzw *gzstd.Writer

		if gzw, err = gzstd.NewWriterLevel(w, wlevel); err == nil {
			closers = append(closers, gzw.Close)
			w = gzw
		}
	case "dedup":
		var ddw dedup.Writer
		if ddw, err = dedup.NewStreamWriter(w, dedup.ModeDynamic, 8192, 1000*8192); err == nil {
			closers = append(closers, ddw.Close)
			w = ddw
		}
	case "s2":
		const blockSize = 4 << 20
		var sw *s2.Writer
		switch wlevel {
		case 0:
			sw = s2.NewWriter(w, s2.WriterUncompressed(), s2.WriterBlockSize(blockSize), s2.WriterConcurrency(cpu))
		case 1:
			sw = s2.NewWriter(w, s2.WriterBlockSize(blockSize), s2.WriterConcurrency(cpu))
		case 2:
			sw = s2.NewWriter(w, s2.WriterBetterCompression(), s2.WriterBlockSize(blockSize), s2.WriterConcurrency(cpu))
		case 3:
			sw = s2.NewWriter(w, s2.WriterBestCompression(), s2.WriterBlockSize(blockSize), s2.WriterConcurrency(cpu))
		}
		w = sw
		closers = append(closers, sw.Close)
	case "s2s":
		var sw *s2.Writer
		switch wlevel {
		case 0:
			sw = s2.NewWriter(w, s2.WriterUncompressed(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 1:
			sw = s2.NewWriter(w, s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 2:
			sw = s2.NewWriter(w, s2.WriterBetterCompression(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 3:
			sw = s2.NewWriter(w, s2.WriterBestCompression(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		}
		w = sw
		closers = append(closers, sw.Close)

	case "snappy":
		sw := snappy.NewWriter(w)
		w = sw
		/*	case "lzo1x":
			sw := lzo.NewWriter(w, wlevel)
			w = sw*/
	case "flatekp":
		if wlevel == -3 {
			gzw := flkp.NewStatelessWriter(w)
			closers = append(closers, gzw.Close)
			w = gzw
			break
		}
		var fw *flkp.Writer
		if fw, err = flkp.NewWriter(w, wlevel); err == nil {
			closers = append(closers, fw.Close)
			w = fw
		}
	case "flatestd":
		var fw *flstd.Writer
		if fw, err = flstd.NewWriter(w, wlevel); err == nil {
			closers = append(closers, fw.Close)
			w = fw
		}
	case "lzma":
		wc := lzma.WriterConfig{
			Properties:   nil,
			DictCap:      0,
			BufSize:      0,
			Matcher:      0,
			SizeInHeader: false,
			Size:         0,
			EOSMarker:    true,
		}
		if lw, err := wc.NewWriter(w); err == nil {
			closers = append(closers, lw.Close)
			w = lw
		}
	case "lzma2":
		wc := lzma.Writer2Config{
			Properties: nil,
			DictCap:    0,
			BufSize:    0,
			Matcher:    0,
		}
		if lw, err := wc.NewWriter2(w); err == nil {
			closers = append(closers, lw.Close)
			w = lw
		}
	case "lz4":
		lw := lz4.NewWriter(w)
		//lw.WithConcurrency(cpu)
		//lw.Header.CompressionLevel = wlevel
		closers = append(closers, lw.Close)
		w = lw
	case "zstd":
		zw := zstd.NewWriterLevel(w, wlevel)
		closers = append(closers, zw.Close)
		w = zw
	case "zskp":
		zw, err := zskp.NewWriter(w, zskp.WithEncoderLevel(zskp.EncoderLevel(wlevel)))
		if err != nil {
			panic(err)
		}
		closers = append(closers, zw.Close)
		w = zw
	case "dzstd":
		zw := dzstd.NewWriterLevel(w, wlevel)
		closers = append(closers, zw.Close)
		w = zw
	case "s2zs":
		pr, pw := io.Pipe()
		var sw *s2.Writer
		switch wlevel {
		case 0:
			sw = s2.NewWriter(pw, s2.WriterUncompressed(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 1:
			sw = s2.NewWriter(pw, s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 2:
			sw = s2.NewWriter(pw, s2.WriterBetterCompression(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		case 3:
			sw = s2.NewWriter(pw, s2.WriterBestCompression(), s2.WriterSnappyCompat(), s2.WriterConcurrency(cpu))
		}
		ra := readahead.NewReader(pr)
		conv := zskp.SnappyConverter{}
		var wg sync.WaitGroup
		wg.Add(1)
		go func(w io.Writer) {
			defer wg.Done()
			_, err := conv.Convert(ra, w)
			pr.CloseWithError(err)
		}(w)
		w = sw
		closers = append(closers, closeWrap{wg.Wait}.Close, pw.Close, sw.Close)
	//case "qlz":
	//	qlw := quicklz.NewWriter(w, -wlevel)
	//	closers = append(closers, qlw.Close)
	//	w = qlw
	default:
		panic("write mode -w=x must be (raw|flatekp|flatestd|gzkp|pgzip|gzstd|none)")
	}
	if err != nil {
		panic(err)
	}

	if source && sink {
		return
	}

	type directWriter interface {
		EncodeBuffer(buf []byte) (err error)
	}

	inSize := int64(0)
	startCpu := getCpuTime()
	start := time.Now()

	func() {
		if dw, ok := w.(directWriter); ok {
			if eb, ok := r.(*bytes.Buffer); ok {
				inSize += int64(eb.Len())
				err := dw.EncodeBuffer(eb.Bytes())
				if err != nil {
					panic(err)
				}
				for i := len(closers) - 1; i >= 0; i-- {
					closers[i]()
				}
				return
			}
		}
		nr, err := io.Copy(w, r)
		inSize += nr
		if err != nil && err != io.EOF {
			panic(err)
		}
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}()
	if stats {
		elapsed := time.Since(start)
		elapsedCpu := getCpuTime() - startCpu
		wg.Wait()
		if header {
			fmt.Printf( "%20s %5s %5s %10s %10s %10s %8s %10s %8s\n", "file", "wmode", "level", "insize", "outsize", "millis", "mb/s", "ms_cpu", "cpu_mb/s")
			//fmt.Printf("file\tin\tout\tlevel\tcpu\tinsize\toutsize\tmillis\tmb/s\n")
		}
		mbpersec := (float64(inSize) / (1024 * 1024)) / (float64(elapsed) / (float64(time.Second)))
		cpuMbpersec := (float64(inSize) / (1024 * 1024)) / (float64(elapsedCpu) / (float64(time.Second)))
		//fmt.Printf("%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%.02f\n", in, rmode, wmode, wlevel, cpu, inSize, outSize.n, elapsed/time.Millisecond, mbpersec)
		fmt.Printf( "%20s %5s %5d %10d %10d %10d %8.2f %10d %8.2f\n", in, wmode, wlevel, inSize, outSize.n, elapsed/time.Millisecond, mbpersec, elapsedCpu / time.Millisecond, cpuMbpersec)
	} else {
		wg.Wait()
	}
}