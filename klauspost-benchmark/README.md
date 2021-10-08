# Benchmark results

Dataset: http://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip (uncompressed and tarred)

Results on: Intel(R) Xeon(R) Silver 4114 CPU @ 2.20GHz

Go version: 1.17

Library versions (from go.mod)
```
	zskp  - github.com/klauspost/compress v1.13.6
	zstd  - github.com/valyala/gozstd v1.13.0
	zstkp - github.com/DataDog/zstd v1.4.8
```

## Compression
```
go build && ./klauspost-benchmark -r raw -w zstd -in /tmp/silesia.tar -out /tmp/silesia.tar.zst -l 1 -stats -mem
```

```
                file wmode level     insize    outsize     millis     mb/s     ms_cpu cpu_mb/s
    /tmp/silesia.tar  zskp     1  211957760   73528095       1274   158.54       2116    95.52
    /tmp/silesia.tar  zstd     1  211957760   73588083       1010   199.96        990   204.00
    /tmp/silesia.tar dzstd     1  211957760   73588080       1017   198.60        945   213.90
```

## Decompression
```
go build && ./klauspost-benchmark -r zskp -w none -in /tmp/silesia.tar.zst -stats -l 1
```

```
                file wmode level     insize    outsize     millis     mb/s     ms_cpu cpu_mb/s
/tmp/silesia.tar.zst  none     1  211957760          0        929   217.39       1425   141.81
/tmp/silesia.tar.zst  none     1  211957760          0        342   589.50        339   595.93
/tmp/silesia.tar.zst  none     1  211957760          0        343   587.98        334   603.83
```
