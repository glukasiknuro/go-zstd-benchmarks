package main

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/genproto/googleapis/bytestream"
	"io"
	"strconv"
	"strings"
)
import "flag"

var downloadTpl = flag.String("download_template", "instance-name/blobs/{sha256}/{size}", "Resource name, download template")

func downloadFile(client bytestream.ByteStreamClient, size int64, sha256 string) error {
	rn := strings.ReplaceAll(*downloadTpl, "{sha256}", sha256)
	rn = strings.ReplaceAll(rn, "{size}", strconv.FormatInt(size, 10))
	//log.Printf("Downloading file, resource_name: %s", rn)

	stream, err := client.Read(context.Background(), &bytestream.ReadRequest{ResourceName: rn})
	if err != nil {
		return err
	}

	var read int64
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		read += int64(len(resp.GetData()))
	}

	if read != size {
		return errors.New(fmt.Sprintf("Read %d != expected size %d", read, size))
	}

	return nil
}

