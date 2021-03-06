## Simple load test for https://github.com/buchgr/bazel-remote

Benchmark first uploads a set of files to bazel-remote, and later in parallel downloads those files a number of times
using `bytestream.ReadRequest` streaming gRPC.

Dataset: http://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip (uncompressed)

Results on: Intel(R) Xeon(R) Silver 4114 CPU @ 2.20GHz

Running bazel-remote:

Patch adding `--zstd_implementation=cgo` to bazel-remote: https://github.com/buchgr/bazel-remote/commit/f52b0b587783f57376c8ffe2e2373414be024da9

*NOTE*: Change `linux-build.sh` to `CGO_ENABLED=1` to enable cgo zstd implementation


```
rm -rf /tmp/ramdisk/* && ./linux-build.sh && cpulimit --limit=800 --monitor-forks --foreground  --verbose -- ./bazel-remote --dir=/tmp/ramdisk --max_size=10  --access_log_level=none
```

# Results - download

Running benchmark:

```
go build && ./bazel-remote-load-test -addr localhost:9092 -dir /tmp/silesia -parallel=100 -download_iterations=100
```

For `--storage_mode=uncompressed` - bazel-remote option
```
    downloaded size: 21 GB  avg throughput: 2.0 GB
    CPU time: 1:24 (as measured using htop utility)
```

For `--storage_mode=zstd`
```
    downloaded size: 21 GB  avg throughput: 797 MB/s
	CPU time: 3:43  (140s overhead, 150MB/cpu_s)
```

For `--storage_mode=zstd --zstd_implementation=cgo`
```
    downloaded size: 21 GB  avg throughput: 1.5 GB/s
    CPU time: 1:56 (32s overhead, 650MB/cpu_s)
```

# Results - upload

Running benchmark:

```
go build && ./bazel-remote-load-test -addr localhost:9092 -dir /tmp/silesia -parallel=100 -upload_iterations=100
```


For `--storage_mode=uncompressed`
```
    uploaded size: 21 GB  avg throughput: 2.0 GB/s
    CPU time: 1:23
```

For `--storage_mode=zstd`
```
    uploaded size: 21 GB  avg throughput: 368 MB/s
	CPU time: 8:13  (410s overhead, 51MB/s)
```


For `--storage_mode=zstd --zstd_implementation=cgo`
```
    uploaded size: 21 GB  avg throughput: 516 MB/s
    CPU time: 5:40 (257s overhead, 81MB/s)
```
