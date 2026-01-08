package tui

type execStartMsg struct {
	Index int
}

type execDoneMsg struct {
	Index int
	Err   error
}

type execAllDoneMsg struct{}
