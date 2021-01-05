package types

const (
	IN_NONE = iota
	IN_PROGRAM_START
)

type OpCode struct {
	In byte
	Args interface{}
}

