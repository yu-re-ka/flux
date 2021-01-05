package types

const (
	IN_NONE = iota
	IN_PROGRAM_START
	IN_CONS_SIDE_EFFECTS
	IN_APPEND_SIDE_EFFECT
	IN_LOAD_VALUE
)

type OpCode struct {
	In byte
	Args interface{}
}

