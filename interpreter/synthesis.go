package interpreter

import (
	"context"
//	"fmt"
//	"regexp"
//	"time"

	bctypes "github.com/influxdata/flux/bytecode/types"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

func (itrp *Interpreter) Code() []bctypes.OpCode {
	return itrp.code
}

func (itrp *Interpreter) appendCode( in byte, args interface{} ) {
	itrp.code = append(itrp.code, bctypes.OpCode{In: in, Args: args})
}

func (itrp *Interpreter) Synthesis(ctx context.Context, node semantic.Node, scope values.Scope, importer Importer) error {
	sideEffects, err := itrp.Eval(ctx, node, scope, importer)
	if err != nil {
		return err
	}

	ps := ProgramStart{
		SideEffects: sideEffects,
	}

	itrp.appendCode( bctypes.IN_PROGRAM_START, ps )
	return nil
}


func (itrp *Interpreter) SynthesisTo(ctx context.Context, sideEffects []SideEffect) error {
	ps := ProgramStart{
		SideEffects: sideEffects,
	}

	itrp.appendCode( bctypes.IN_PROGRAM_START, ps )
	return nil
}
