package lang

import (
	"context"
	"encoding/json"
	"time"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/values"
	"github.com/influxdata/flux/bytecode"
	bctypes "github.com/influxdata/flux/bytecode/types"
	"go.uber.org/zap"
)

type BytecodeCompiler struct {
	Now    time.Time
	Extern json.RawMessage `json:"extern,omitempty"`
	Query  string          `json:"query"`
}

// TableObjectCompiler compiles a TableObject into an executable flux.Program.
// It is not added to CompilerMappings and it is not serializable, because
// it is impossible to use it outside of the context of an ongoing execution.
type BytecodeTableObjectCompiler struct {
	Tables *flux.TableObject
	Now    time.Time
}

type BytecodeAstProgram struct {
	*BytecodeProgram

	Ast flux.ASTHandle
	Now time.Time
	// A list of profilers that are profiling this query
	Profilers []execute.Profiler
	// The operator profiler that is profiling this query, if any.
	// Note this operator profiler is also cached in the Profilers array.
	tfProfiler *execute.OperatorProfiler
}

// Program implements the flux.Program interface.
// It will execute a compiled plan using an executor.
type BytecodeTableObjectProgram struct {
	*BytecodeProgram

	Tables *flux.TableObject
	Now    time.Time
}


// Program implements the flux.Program interface.
// It will execute a compiled plan using an executor.
type BytecodeProgram struct {
	Logger   *zap.Logger
	PlanSpec *plan.Spec
	Runtime  flux.Runtime

	opts *compileOptions
}

func (c BytecodeCompiler) Compile(ctx context.Context, runtime flux.Runtime) (flux.Program, error) {
	println("-> to bytecode compiler Compile()")
	query := c.Query

	q := query
	now := c.Now

	astPkg, err := runtime.Parse(q)
	if err != nil {
		return nil, err
	}

	return &BytecodeAstProgram{
		BytecodeProgram: &BytecodeProgram{
			Runtime: runtime,
			opts:    nil, // applyOptions(opts...),
		},
		Ast: astPkg,
		Now: now,
	}, nil
}

func (c BytecodeCompiler) CompilerType() flux.CompilerType {
	return BytecodeCompilerType
}

func (c *BytecodeTableObjectCompiler) Compile(ctx context.Context) (flux.Program, error) {
	return &BytecodeTableObjectProgram{
		BytecodeProgram: &BytecodeProgram{},
		Tables: c.Tables,
		Now: c.Now,
	}, nil
}

func (*BytecodeTableObjectCompiler) CompilerType() flux.CompilerType {
	panic("TableObjectCompiler is not associated with a CompilerType")
}

func (p *BytecodeAstProgram) updateOpts(scope values.Scope) error {
	pkg, ok := getPackageFromScope("planner", scope)
	if !ok {
		return nil
	}
	lo, po, err := getPlanOptions(pkg)
	if err != nil {
		return err
	}
	if lo != nil {
		p.opts.planOptions.logical = append(p.opts.planOptions.logical, lo)
	}
	if po != nil {
		p.opts.planOptions.physical = append(p.opts.planOptions.physical, po)
	}
	return nil
}

func (p *BytecodeAstProgram) updateProfilers(ctx context.Context, scope values.Scope) error {
	if execute.HaveExecutionDependencies(ctx) {
		deps := execute.GetExecutionDependencies(ctx)
		p.tfProfiler = deps.ExecutionOptions.OperatorProfiler
		p.Profilers = deps.ExecutionOptions.Profilers
	}
	return nil
}

// Prepare the Ast for semantic analysis
func (p *BytecodeAstProgram) GetAst() (flux.ASTHandle, error) {
	if p.Now.IsZero() {
		p.Now = time.Now()
	}
	if p.opts == nil {
		p.opts = defaultOptions()
	}
	if p.opts.extern != nil {
		extern := p.opts.extern
		if err := p.Runtime.MergePackages(extern, p.Ast); err != nil {
			return nil, err
		}
		p.Ast = extern
		p.opts.extern = nil
	}
	return p.Ast, nil
}

func (p *BytecodeAstProgram) Start(ctx context.Context, alloc *memory.Allocator) (flux.Query, error) {
	// The program must inject execution dependencies to make it available to
	// function calls during the evaluation phase (see `tableFind`).
	deps := execute.NewExecutionDependencies(alloc, &p.Now, p.Logger)
	deps.BytecodeExecution = true
	ctx = deps.Inject(ctx)

	nextPlanNodeID := new(int)
	ctx = context.WithValue(ctx, plan.NextPlanNodeIDKey, nextPlanNodeID)

	ast, astErr := p.GetAst()
	if astErr != nil {
		return nil, astErr
	}

	// Program synthesis: AST -> byte codes
	var opCodes []bctypes.OpCode
	var scope values.Scope
	var nowOpt values.Value
	var err error

	checkNow := func(r flux.Runtime, scope values.Scope) {
		nowOpt, _ = scope.Lookup(interpreter.NowOption)
		if _, ok := nowOpt.(*values.Option); !ok {
			panic("now must be an option")
		}
	}

	// Set the now option to our own default and capture the option itself
	// to allow us to find it after the run.
	opCodes, scope, err = p.Runtime.Synthesis(ctx, ast,
			&ExecOptsConfig{}, flux.SetNowOption(p.Now), checkNow )
	if err != nil {
		return nil, err
	}

	nowTime, err := nowOpt.Function().Call(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, codes.Inherit, "error in evaluating AST while starting program")
	}

	p.Now = nowTime.Time().Time()

	if err := p.updateOpts(scope); err != nil {
		return nil, errors.Wrap(err, codes.Inherit, "error in reading options while starting program")
	}
	if err := p.updateProfilers(ctx, scope); err != nil {
		return nil, errors.Wrap(err, codes.Inherit, "error in reading profiler settings while starting program")
	}

	now := p.Now
	//o := p.opts

	return bytecode.Execute( ctx, alloc, now, opCodes, p.Logger )
}

func (p *BytecodeTableObjectProgram) Start(ctx context.Context, alloc *memory.Allocator) (flux.Query, error) {
	to := p.Tables
	now := p.Now

	// o := applyOptions()

	sideEffect := interpreter.SideEffect{Value: to}

	itrp := interpreter.NewInterpreter(nil, nil)
	err := itrp.SynthesisTo(ctx, sideEffect)
	if err != nil {
		return nil, err
	}

	opCodes := itrp.Code()

	return bytecode.Execute( ctx, alloc, now, opCodes, p.Logger )
}
