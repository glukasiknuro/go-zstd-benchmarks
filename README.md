## Small benchmarks to compare go implementations of zstd

Those benchmarks were created to checks benetifs of changing zstd implementation in
https://github.com/buchgr/bazel-remote

Dataset: http://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip

Results on: Intel(R) Xeon(R) Silver 4114 CPU @ 2.20GHz

Measuring CPU time (rusage utime) instead of wall time, which maybe more important in case of multiple concurrent
compressions.

```shell
$ rm -rf /tmp/ramdisk/tmp && mkdir /tmp/ramdisk/tmp && go build && ./go-zstd-benchmarks -dir /tmp/ramdisk/silesia_tar -tmp_dir /tmp/ramdisk/tmp -iterations 10
Scanning files in: /tmp/ramdisk/silesia_tar
Got 1 file(s), total size: 212 MB (avg: 212 MB)
          compressor      inSize    outSize  ratio       enc_time       dec_time  enc_speed  dec_speed
            IDENTITY      2.1 GB     2.1 GB 100.00             0s             0s   9.2 EB/s   9.2 EB/s
        ZSTD DEFAULT      2.1 GB     675 MB  31.83     40.682418s     18.365409s    52 MB/s   115 MB/s
     ZSTD BEST_SPEED      2.1 GB     735 MB  34.69     31.773793s     16.416214s    67 MB/s   129 MB/s
    ZSTD CGO DEFAULT      2.1 GB     640 MB  30.19     30.840302s      3.179873s    69 MB/s   667 MB/s
      ZSTD CGO SPEED      2.1 GB     736 MB  34.72      10.11669s      2.776654s   210 MB/s   763 MB/s
```
