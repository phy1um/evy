//go:build tinygo

package main

import (
	"errors"
	"strings"

	"foxygo.at/evy/pkg/evaluator"
	"foxygo.at/evy/pkg/parser"
)

var (
	version string
	eval    *evaluator.Evaluator
	events  []evaluator.Event
)

func main() {
	defer afterStop()
	actions := getActions()

	rt := newJSRuntime()
	input := getEvySource()
	ast, err := parse(input, rt)
	if err != nil {
		jsError(err.Error())
		return
	}
	if actions["fmt"] {
		formattedInput := ast.Format()
		if formattedInput != input {
			setEvySource(formattedInput)
			ast, err = parse(formattedInput, rt)
			if err != nil {
				jsError(err.Error())
				return
			}
		}
	}
	if actions["ui"] {
		prepareUI(ast)
	}
	if actions["eval"] {
		err := evaluate(ast, rt)
		if err != nil && !errors.Is(err, evaluator.ErrStopped) {
			jsError(err.Error())
		}
	}
}

func getActions() map[string]bool {
	m := map[string]bool{}
	addr := jsActions()
	s := getStringFromAddr(addr)
	actions := strings.Split(s, ",")
	for _, action := range actions {
		if action != "" {
			m[action] = true
		}
	}
	return m
}

func getEvySource() string {
	addr := evySource()
	return getStringFromAddr(addr)
}

func parse(input string, rt evaluator.Runtime) (*parser.Program, error) {
	builtins := evaluator.DefaultBuiltins(rt).ParserBuiltins()
	prog, err := parser.Parse(input, builtins)
	if err != nil {
		return nil, parser.TruncateError(err, 8)
	}
	return prog, nil
}

func prepareUI(prog *parser.Program) {
	funcNames := prog.CalledBuiltinFuncs
	eventHandlerNames := parser.EventHandlerNames(prog.EventHandlers)
	names := append(funcNames, eventHandlerNames...)
	jsPrepareUI(strings.Join(names, ","))
}

func evaluate(prog *parser.Program, rt *jsRuntime) error {
	builtins := evaluator.DefaultBuiltins(rt)
	eval = evaluator.NewEvaluator(builtins)
	_, err := eval.Eval(prog)
	if err != nil {
		return err
	}
	return handleEvents(rt.yielder)
}

func handleEvents(yielder *sleepingYielder) error {
	if eval == nil || len(eval.EventHandlerNames()) == 0 {
		return nil
	}
	for _, name := range eval.EventHandlerNames() {
		registerEventHandler(name)
	}
	for {
		if eval.Stopped {
			return nil
		}
		// unsynchronized access to events - ok in WASM as single threaded.
		if len(events) > 0 {
			event := events[0]
			events = events[1:]
			yielder.Reset()
			if err := eval.HandleEvent(event); err != nil {
				return err
			}
		} else {
			yielder.ForceYield()
		}
	}
	return nil
}
