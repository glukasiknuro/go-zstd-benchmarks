package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/bytestream"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)
import "flag"

var uploadTpl = flag.String("upload_template", "instance-name/uploads/{uuid}/blobs/{sha256}/{size}", "Resource name, upload template")

func uploadFile(client bytestream.ByteStreamClient, path string, size int64, sha256 string) error {
	f, err := os.Open(path)

	uuid := uuid.New()
	rn := strings.ReplaceAll(*uploadTpl, "{uuid}", uuid.String())
	rn = strings.ReplaceAll(rn, "{sha256}", sha256)
	rn = strings.ReplaceAll(rn, "{size}", strconv.FormatInt(size, 10))
	log.Printf("Uploading file: %-40s  resource_name: %s", path, rn)

	wc, err := client.Write(context.Background())
	if err != nil {
		return err
	}

	buff := make([]byte, 1<<16)
	var offset int64
	for {
		n, err := f.Read(buff)
		if err != nil && err != io.EOF {
			_ = wc.CloseSend()
			return err
		}
		finishWrite := n == 0 && err == io.EOF
		err = wc.Send(&bytestream.WriteRequest{
			ResourceName: rn,
			WriteOffset:  offset,
			FinishWrite:  finishWrite,
			Data:         buff[:n],
		})
		if err != nil {
			break
		}

		if finishWrite {
			break
		}
		offset += int64(n)
	}

	resp, err := wc.CloseAndRecv()
	if err != nil {
		return err
	}

	if resp.CommittedSize != size {
		return errors.New(fmt.Sprintf("Commited size %d != actual size %d", resp.CommittedSize, size))
	}

	return nil
}

