package debug

// pass will pass any incoming tables directly next to the following transformation.
// It is best used to interrupt any planner rules that rely on a specific ordering.
builtin pass : (<-tables: [A]) => [A] where A: Record

// assert will initiate a panic when the condition is false.
// This method is disabled if the testing framework is disabled.
// It is otherwise identical to pass.
builtin panic : (<-tables: [A], msg: string) => [A] where A: Record
