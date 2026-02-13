package execute

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		if isPermissionError(err) {
			if err := sudoMkdir(path); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", path, err)
			}
			return nil
		}
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return nil
}

func requireCommand(cmd string) error {
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("required command not found: %s", cmd)
	}
	return nil
}

func requireAnyCommand(cmds ...string) (string, error) {
	for _, cmd := range cmds {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd, nil
		}
	}
	return "", fmt.Errorf("required command not found: %s", strings.Join(cmds, " or "))
}

func validateNonEmptyFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.Size() == 0 {
		return 0, fmt.Errorf("backup file is empty")
	}
	return info.Size(), nil
}

func validateSQLDump(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 1024*1024)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	content := strings.ToUpper(string(buf[:n]))
	if strings.Contains(content, "CREATE ") || strings.Contains(content, "INSERT ") || strings.Contains(content, "DROP ") {
		return nil
	}
	return fmt.Errorf("backup file does not look like a SQL dump")
}

func validateRDB(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 5)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return err
	}
	if n < 5 || !bytes.Equal(header, []byte("REDIS")) {
		return fmt.Errorf("backup file is not a valid RDB file")
	}
	return nil
}

func validateMongoDumpDir(path string) (int64, error) {
	var size int64
	hasBSON := false

	err := filepath.Walk(path, func(entry string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += info.Size()
		if strings.HasSuffix(info.Name(), ".bson") {
			hasBSON = true
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, fmt.Errorf("backup directory is empty")
	}
	if !hasBSON {
		return 0, fmt.Errorf("backup directory contains no BSON files")
	}
	return size, nil
}

func validateNonEmptyDir(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(entry string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, fmt.Errorf("backup directory is empty")
	}
	return size, nil
}

func backupCompression() string {
	return strings.TrimSpace(strings.ToLower(os.Getenv("MV_BACKUP_COMPRESSION")))
}

func keepBackupSource() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_BACKUP_KEEP_SOURCE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func applyBackupCompression(path string, isDir bool) (string, int64, error) {
	compression := backupCompression()
	if compression == "" {
		if isDir {
			size, err := validateNonEmptyDir(path)
			return path, size, err
		}
		size, err := validateNonEmptyFile(path)
		return path, size, err
	}

	var compressedPath string
	var err error
	if isDir {
		compressedPath, err = compressDir(path, compression)
	} else {
		compressedPath, err = compressFile(path, compression)
	}
	if err != nil {
		return path, 0, err
	}

	if !keepBackupSource() {
		if isDir {
			_ = os.RemoveAll(path)
		} else {
			_ = os.Remove(path)
		}
	}

	size, err := validateNonEmptyFile(compressedPath)
	if err != nil {
		return compressedPath, 0, err
	}
	return compressedPath, size, nil
}

func compressFile(path, compression string) (string, error) {
	switch compression {
	case "gz":
		return gzipFile(path)
	case "bz2":
		if err := requireCommand("bzip2"); err != nil {
			return "", err
		}
		return bzip2File(path)
	case "zip":
		return zipFile(path)
	default:
		return "", fmt.Errorf("unsupported compression: %s", compression)
	}
}

func compressDir(path, compression string) (string, error) {
	switch compression {
	case "gz":
		return tarGzipDir(path)
	case "bz2":
		if err := requireCommand("bzip2"); err != nil {
			return "", err
		}
		return tarBzip2Dir(path)
	case "zip":
		return zipDir(path)
	default:
		return "", fmt.Errorf("unsupported compression: %s", compression)
	}
}

func gzipFile(path string) (string, error) {
	outPath := path + ".gz"
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	gzWriter := gzip.NewWriter(out)
	if _, err := io.Copy(gzWriter, in); err != nil {
		_ = gzWriter.Close()
		return "", err
	}
	if err := gzWriter.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

func bzip2File(path string) (string, error) {
	outPath := path + ".bz2"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	cmd := exec.Command("bzip2", "-c", path)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return outPath, nil
}

func zipFile(path string) (string, error) {
	outPath := path + ".zip"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}
	header.Name = filepath.Base(path)
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(writer, file); err != nil {
		return "", err
	}
	return outPath, nil
}

func tarGzipDir(path string) (string, error) {
	outPath := path + ".tar.gz"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	gzWriter := gzip.NewWriter(out)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return outPath, writeTarDir(path, tarWriter)
}

func tarDir(path string) (string, error) {
	outPath := path + ".tar"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	tarWriter := tar.NewWriter(out)
	defer tarWriter.Close()

	return outPath, writeTarDir(path, tarWriter)
}

func tarBzip2Dir(path string) (string, error) {
	outPath := path + ".tar.bz2"
	tempTar, err := os.CreateTemp("", "mirrorvault_dir_*.tar")
	if err != nil {
		return "", err
	}
	tempTarPath := tempTar.Name()
	tarWriter := tar.NewWriter(tempTar)
	if err := writeTarDir(path, tarWriter); err != nil {
		tarWriter.Close()
		tempTar.Close()
		_ = os.Remove(tempTarPath)
		return "", err
	}
	_ = tarWriter.Close()
	_ = tempTar.Close()

	out, err := os.Create(outPath)
	if err != nil {
		_ = os.Remove(tempTarPath)
		return "", err
	}
	defer out.Close()

	cmd := exec.Command("bzip2", "-c", tempTarPath)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tempTarPath)
		return "", err
	}
	_ = os.Remove(tempTarPath)
	return outPath, nil
}

func zipDir(path string) (string, error) {
	outPath := path + ".zip"
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)
	defer zipWriter.Close()

	err = filepath.Walk(path, func(entry string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(path, entry)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		src, err := os.Open(entry)
		if err != nil {
			return err
		}
		defer src.Close()
		if _, err := io.Copy(writer, src); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return outPath, nil
}

func writeTarDir(basePath string, tarWriter *tar.Writer) error {
	return filepath.Walk(basePath, func(entry string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(basePath, entry)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(entry)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}
		return nil
	})
}

func strictValidationEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("MV_STRICT_VALIDATE")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func createWritableFile(path string) (*os.File, error) {
	f, err := os.Create(path)
	if err == nil {
		return f, nil
	}
	if !isPermissionError(err) {
		return nil, err
	}

	if err := sudoTouch(path); err != nil {
		return nil, err
	}
	if err := sudoChown(path); err != nil {
		return nil, err
	}
	if err := sudoChmod(path, "0644"); err != nil {
		return nil, err
	}

	return os.Create(path)
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	var errno syscall.Errno
	if ok := errorAs(err, &errno); ok {
		return errno == syscall.EACCES || errno == syscall.EPERM
	}
	return false
}

func errorAs(err error, target interface{}) bool {
	type causer interface {
		Unwrap() error
	}
	for err != nil {
		if ok := syscallErrnoAs(err, target); ok {
			return true
		}
		c, ok := err.(causer)
		if !ok {
			break
		}
		err = c.Unwrap()
	}
	return false
}

func syscallErrnoAs(err error, target interface{}) bool {
	if target == nil {
		return false
	}
	if errno, ok := err.(syscall.Errno); ok {
		if t, ok := target.(*syscall.Errno); ok {
			*t = errno
			return true
		}
	}
	return false
}

func sudoMkdir(path string) error {
	cmd := exec.Command("sudo", "mkdir", "-p", path)
	if err := cmd.Run(); err != nil {
		return err
	}
	return sudoChown(path)
}

func sudoTouch(path string) error {
	cmd := exec.Command("sudo", "touch", path)
	return cmd.Run()
}

func sudoChown(path string) error {
	uid := strconv.Itoa(os.Getuid())
	gid := strconv.Itoa(os.Getgid())
	cmd := exec.Command("sudo", "chown", uid+":"+gid, path)
	return cmd.Run()
}

func sudoChmod(path string, mode string) error {
	cmd := exec.Command("sudo", "chmod", mode, path)
	return cmd.Run()
}
