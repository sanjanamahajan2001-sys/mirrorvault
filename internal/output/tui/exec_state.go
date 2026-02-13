package tui

type ExecStatus int

const (
	ExecPending ExecStatus = iota
	ExecRunning
	ExecDone
	ExecFailed
)

type DriveStatus int

const (
	DriveNone DriveStatus = iota
	DriveChecking
	DriveUploading
	DriveDone
	DriveFailed
	DriveSkipped
)

type ExecItem struct {
	Engine   string
	Database string
	Status   ExecStatus
	Path     string
	Size     int64
	Err      error
	DriveStatus        DriveStatus
	DriveMessage       string
	DriveRemoteName    string
	DriveErr           error
	DriveAccountRemain int64
	DriveAccountTotal  int64
	DriveBackupSize    int64
}

type ExecState struct {
	Items     []ExecItem
	Index     int
	Done      bool
	AwaitExit bool
}

func NewExecState(engine string, databases []string) ExecState {
	items := make([]ExecItem, 0, len(databases))

	for _, db := range databases {
		items = append(items, ExecItem{
			Engine:   engine,
			Database: db,
			Status:   ExecPending,
		})
	}

	return ExecState{
		Items: items,
	}
}
