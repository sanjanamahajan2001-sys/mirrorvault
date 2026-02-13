package drive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func LocalPathSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}

	var total int64
	err = filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

func TarGzDirectory(srcDir string) (string, int64, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return "", 0, err
	}
	if !info.IsDir() {
		return "", 0, fmt.Errorf("path is not a directory")
	}

	outFile := filepath.Join(os.TempDir(), fmt.Sprintf("mirrorvault_%d.tar.gz", time.Now().UnixNano()))
	f, err := os.Create(outFile)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	base := filepath.Clean(srcDir)
	err = filepath.Walk(base, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tw, file)
		return err
	})
	if err != nil {
		_ = os.Remove(outFile)
		return "", 0, err
	}

	stat, err := os.Stat(outFile)
	if err != nil {
		return outFile, 0, nil
	}
	return outFile, stat.Size(), nil
}

func ExtractTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	destDir := filepath.Join(os.TempDir(), fmt.Sprintf("mirrorvault-restore-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr == nil {
			continue
		}
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return "", fmt.Errorf("invalid archive path: %s", hdr.Name)
		}
		targetPath := filepath.Join(destDir, cleanName)
		if hdr.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return "", err
		}
		out, err := os.Create(targetPath)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
	}
	return destDir, nil
}
