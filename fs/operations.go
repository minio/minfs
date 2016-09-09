package minfs

// Operation -
type Operation struct {
	Error chan error
}

// MoveOperation -
type MoveOperation struct {
	*Operation

	Source string
	Target string
}

// CopyOperation -
type CopyOperation struct {
	*Operation

	Source string
	Target string
}

// PutOperation -
type PutOperation struct {
	*Operation

	Length int64

	Source string
	Target string
}
