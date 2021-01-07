package types

const (
	IN_NONE = iota
	IN_PROGRAM_START
	IN_CONS_SIDE_EFFECTS
	IN_APPEND_SIDE_EFFECT
	IN_LOAD_VALUE
	IN_SCOPE_LOOKUP
	IN_POP
	IN_CALL
	IN_RET
	IN_SCOPE_SET
	IN_STOP
)

type OpCode struct {
	In byte
	Args interface{}
}

