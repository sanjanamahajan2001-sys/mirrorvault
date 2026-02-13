package drive

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

type FileItem struct {
	ID           string
	Name         string
	Size         int64
	ModifiedTime string
}

type UploadResult struct {
	ID   string
	Name string
	Size int64
}

func ListFiles(ctx context.Context, svc *drive.Service, folderID string) ([]FileItem, error) {
	if svc == nil {
		return nil, fmt.Errorf("drive service is nil")
	}
	if folderID == "" {
		return nil, fmt.Errorf("folder id is required")
	}
	q := fmt.Sprintf("trashed=false and '%s' in parents and mimeType != 'application/vnd.google-apps.folder'", folderID)
	var files []FileItem
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(q).
			Fields("nextPageToken, files(id, name, size, modifiedTime)").
			PageSize(100).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list files: %w", err)
		}
		for _, f := range resp.Files {
			files = append(files, FileItem{
				ID:           f.Id,
				Name:         f.Name,
				Size:         f.Size,
				ModifiedTime: f.ModifiedTime,
			})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return files, nil
}

func UploadFile(ctx context.Context, svc *drive.Service, folderID, localPath string) (*UploadResult, error) {
	if svc == nil {
		return nil, fmt.Errorf("drive service is nil")
	}
	if folderID == "" {
		return nil, fmt.Errorf("folder id is required")
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, expected file")
	}

	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	name := filepath.Base(localPath)
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	driveFile := &drive.File{
		Name:    name,
		Parents: []string{folderID},
	}
	created, err := svc.Files.Create(driveFile).
		Media(file, googleapi.ContentType(mimeType)).
		Fields("id, name").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &UploadResult{
		ID:   created.Id,
		Name: created.Name,
		Size: info.Size(),
	}, nil
}

func DownloadFile(ctx context.Context, svc *drive.Service, fileID, destPath string) error {
	if svc == nil {
		return fmt.Errorf("drive service is nil")
	}
	if fileID == "" || destPath == "" {
		return fmt.Errorf("file id and destination path are required")
	}

	resp, err := svc.Files.Get(fileID).Context(ctx).Download()
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to save downloaded file: %w", err)
	}
	return nil
}
