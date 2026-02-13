package drive

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"
)

type AccountQuota struct {
	Limit           int64
	Usage           int64
	UsageInDrive    int64
	UsageInDriveTrash int64
	Remaining       int64
}

func GetAccountQuota(ctx context.Context, svc *drive.Service) (*AccountQuota, error) {
	if svc == nil {
		return nil, fmt.Errorf("drive service is nil")
	}
	about, err := svc.About.Get().Fields("storageQuota").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch account quota: %w", err)
	}
	if about.StorageQuota == nil {
		return nil, fmt.Errorf("storage quota not available")
	}

	limit := about.StorageQuota.Limit
	usage := about.StorageQuota.Usage
	remaining := int64(0)
	if limit > 0 {
		remaining = limit - usage
	}

	return &AccountQuota{
		Limit:            limit,
		Usage:            usage,
		UsageInDrive:     about.StorageQuota.UsageInDrive,
		UsageInDriveTrash: about.StorageQuota.UsageInDriveTrash,
		Remaining:        remaining,
	}, nil
}
