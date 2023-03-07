package life

type phase int32

const (
	phaseStarting phase = iota
	phaseRunning
	phaseTeardown
)
