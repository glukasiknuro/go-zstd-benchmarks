## Simple load test for https://github.com/buchgr/bazel-remote

Benchmark first uploads a set of files to bazel-remote, and later in parallel downloads those files a number of times
using `bytestream.ReadRequest` streaming gRPC.

Dataset: http://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip (uncompressed)

Results on: Intel(R) Xeon(R) Silver 4114 CPU @ 2.20GHz

Running bazel-remote:

```
rm -rf /tmp/remote-cache-data/ && ./linux-build.sh && cpulimit --limit=800 --monitor-forks --foreground  --verbose -- ./bazel-remote --dir=/tmp/remote-cache-data --max_size=10  --access_log_level=none
```

Running benchmark:

```
go build && ./bazel-remote-load-test -addr localhost:9092 -dir /tmp/ramdisk/silesia -parallel_downloads=100 -iterations=100
```

# Results

For `--storage_mode=uncompressed` - bazel-remote option

```
    downloaded size: 21 GB  avg throughput: 2.0 GB
    CPU time: 1:24 (as measured using htop utility)
```

For `--storage_mode=zstd`

```
    downloaded size: 21 GB  avg throughput: 797 MB/s
	CPU time: 3:43  (140s overhead, 150MB/s)
```


For `--storage_mode=zstd --use_cgo_zstd`

Change to add `--use_cgo_zstd`: https://github.com/buchgr/bazel-remote/commit/591894a55266d3552cbd983c65115e24e252df8e

```
    downloaded size: 21 GB  avg throughput: 1.5 GB/s
    CPU time: 1:56 (32s overhead, 650MB/s)
```