package container

type Container struct {
	ID            string
	PID           int
	Status        Status // 'created', 'running', 'stopped'
	LowerDir      string
	UpperDir      string
	WorkDir       string
	MergedDir     string
	TargetCmd     string
	MemoryLimit   int
	MemoryRequest int
	CpuRequest    int
	CPULimit      int
	CPUPeriod     int
	CreatedAt     string // Используем string, так как SQLite хранит даты текстом
}

type Status string

const CreatedStatus Status = "created"
const RunningStatus Status = "running"
const ErrorStatus Status = "error"
const CompletedStatus Status = "completed"
