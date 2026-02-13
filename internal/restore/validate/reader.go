package validate

import (
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func OpenDecompressedReader(dumpPath string, dumpInfo *DumpInfo) (io.Reader, func() error, error) {
	file, err := os.Open(dumpPath)
	if err != nil {
		return nil, func() error { return nil }, err
	}

	if !dumpInfo.Compressed || dumpInfo.Compression == "" {
		return file, file.Close, nil
	}

	switch dumpInfo.Compression {
	case "gz":
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			_ = file.Close()
			return nil, func() error { return nil }, err
		}
		return gzReader, func() error {
			_ = gzReader.Close()
			return file.Close()
		}, nil
	case "bz2":
		reader := bzip2.NewReader(file)
		return reader, file.Close, nil
	default:
		_ = file.Close()
		return nil, func() error { return nil }, fmt.Errorf("unsupported compression: %s", dumpInfo.Compression)
	}
}

func OpenDecompressedReaderBestEffort(dumpPath string, dumpInfo *DumpInfo) (io.Reader, func() error, bool, error) {
	reader, closer, err := OpenDecompressedReader(dumpPath, dumpInfo)
	if err == nil {
		return reader, closer, false, nil
	}
	if dumpInfo != nil && dumpInfo.Compression == "gz" {
		file, openErr := os.Open(dumpPath)
		if openErr != nil {
			return nil, func() error { return nil }, false, err
		}
		return file, file.Close, true, nil
	}
	return nil, func() error { return nil }, false, err
}
