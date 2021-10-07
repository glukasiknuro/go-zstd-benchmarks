## Sample benchmark for https://github.com/klauspost/compress/tree/master/zstd

Dataset: http://sun.aei.polsl.pl/~sdeor/corpus/silesia.zip (uncompressed and tarred)

Results on: Intel(R) Xeon(R) Silver 4114 CPU @ 2.20GHz

## Using files
```shell
$ rm -f /tmp/silesia.tar.zst && go build && time ./simple-zstd-demo -in /tmp/silesia.tar -out /tmp/silesia.tar.zst --level 1
File size: 211957760 time: 2.109366462s speed: 96 MiB/s
real	0m2.143s
user	0m3.230s
sys	0m0.277s
```

## In memory
```shell
$ go build && time ./simple-zstd-demo -in /tmp/silesia.tar -out /tmp/silesia.tar.zst --level 1 --encode_all
File size: 211957760 time: 1.471898092s speed: 137 MiB/s
real	0m1.977s
user	0m2.115s
sys	0m0.292s
```