package types

const (
	IN_NONE = iota

	// Side effect lists.
	IN_CONS_SIDE_EFFECTS
	IN_APPEND_SIDE_EFFECT

	// Read any flux value embedded in the bytecode and push it to the stack.
	IN_LOAD_VALUE

	// Lookup a value in the current scope.
	IN_SCOPE_LOOKUP

	// Set a value in the current scope.
	IN_SCOPE_SET

	// Pop a value from the stack and discard it.
	IN_POP

	// Call a function, either inbuilt or user.
	IN_CALL
	IN_RET

	// Plan and execute a flux query.
	IN_EXECUTE_FLUX

	// Convert the results of some flux query to a value.
	IN_FIND_RECORD

	// Halt execution.
	IN_STOP
)

type OpCode struct {
	In   byte
	Args interface{}
}
