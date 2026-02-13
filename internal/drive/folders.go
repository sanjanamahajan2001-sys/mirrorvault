package drive

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"
)

type FolderItem struct {
	ID   string
	Name string
}

func ListFolders(ctx context.Context, svc *drive.Service, parentID string) ([]FolderItem, error) {
	if svc == nil {
		return nil, fmt.Errorf("drive service is nil")
	}
	if parentID == "" {
		parentID = "root"
	}

	q := fmt.Sprintf("mimeType='application/vnd.google-apps.folder' and trashed=false and '%s' in parents", parentID)
	var folders []FolderItem
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(q).
			Fields("nextPageToken, files(id, name)").
			PageSize(100).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("failed to list folders: %w", err)
		}
		for _, f := range resp.Files {
			folders = append(folders, FolderItem{ID: f.Id, Name: f.Name})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return folders, nil
}

func CreateFolder(ctx context.Context, svc *drive.Service, name string, parentID string) (*FolderItem, error) {
	if svc == nil {
		return nil, fmt.Errorf("drive service is nil")
	}
	if name == "" {
		return nil, fmt.Errorf("folder name is required")
	}
	if parentID == "" {
		parentID = "root"
	}

	file := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}
	created, err := svc.Files.Create(file).Fields("id, name").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	return &FolderItem{ID: created.Id, Name: created.Name}, nil
}

func FolderSize(ctx context.Context, svc *drive.Service, folderID string) (int64, error) {
	if svc == nil {
		return 0, fmt.Errorf("drive service is nil")
	}
	if folderID == "" {
		return 0, fmt.Errorf("folder id is required")
	}

	var total int64
	stack := []string{folderID}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		q := fmt.Sprintf("trashed=false and '%s' in parents", current)
		pageToken := ""
		for {
			call := svc.Files.List().
				Q(q).
				Fields("nextPageToken, files(id, mimeType, size)").
				PageSize(200).
				Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				return 0, fmt.Errorf("failed to list folder contents: %w", err)
			}
			for _, f := range resp.Files {
				if f.MimeType == "application/vnd.google-apps.folder" {
					stack = append(stack, f.Id)
					continue
				}
				if f.Size > 0 {
					total += f.Size
				}
			}
			if resp.NextPageToken == "" {
				break
			}
			pageToken = resp.NextPageToken
		}
	}
	return total, nil
}
