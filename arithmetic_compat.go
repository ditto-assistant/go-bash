package gobash

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var invalidOctalLiteral = regexp.MustCompile(`^[+-]?0[0-9]*[89][0-9]*$`)

func init() { registerInternal("gobash_validate_integer", cmdGobashValidateInteger) }

// guardInvalidOctalArithmetic prevents mvdan/sh's permissive integer parser
// from silently turning invalid Bash octal values such as 08 into zero. The
// injected validator executes at the same control-flow point as the arithmetic
// expression, so unreachable code and valid reassignments keep Bash semantics.
func guardInvalidOctalArithmetic(file *syntax.File) {
	suspectVariables := make(map[string]bool)
	syntax.Walk(file, func(node syntax.Node) bool {
		assign, ok := node.(*syntax.Assign)
		if ok && assign.Name != nil && assign.Value != nil && invalidOctalLiteral.MatchString(assign.Value.Lit()) {
			suspectVariables[assign.Name.Value] = true
		}
		return true
	})

	syntax.Walk(file, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.ArithmExp:
			node.X = guardArithmeticExpression(node.X, suspectVariables)
			return false
		case *syntax.ArithmCmd:
			node.X = guardArithmeticExpression(node.X, suspectVariables)
			return false
		case *syntax.LetClause:
			for i, expression := range node.Exprs {
				node.Exprs[i] = guardArithmeticExpression(expression, suspectVariables)
			}
			return false
		default:
			return true
		}
	})
}

func guardArithmeticExpression(expression syntax.ArithmExpr, suspectVariables map[string]bool) syntax.ArithmExpr {
	seen := make(map[string]bool)
	var guards []syntax.ArithmExpr
	syntax.Walk(expression, func(node syntax.Node) bool {
		word, ok := node.(*syntax.Word)
		if !ok {
			return true
		}
		value := word.Lit()
		argument := ""
		switch {
		case invalidOctalLiteral.MatchString(value):
			argument = shellQuote(value)
		case suspectVariables[value] && syntax.ValidName(value):
			argument = `"${` + value + `-}"`
		}
		if argument == "" || seen[argument] {
			return true
		}
		seen[argument] = true
		guard, err := syntax.NewParser().Arithmetic(strings.NewReader("$(command gobash_validate_integer " + argument + ")"))
		if err == nil && guard != nil {
			guards = append(guards, guard)
		}
		return true
	})
	for i := len(guards) - 1; i >= 0; i-- {
		expression = &syntax.BinaryArithm{Op: syntax.Add, X: guards[i], Y: expression}
	}
	return expression
}

func cmdGobashValidateInteger(ctx context.Context, e *Env) int {
	if len(e.Args) != 2 {
		e.Errorf("usage: gobash_validate_integer value")
		return 2
	}
	if !invalidOctalLiteral.MatchString(strings.TrimSpace(e.Args[1])) {
		_, _ = fmt.Fprint(e.Stdout, "0")
		return 0
	}
	err := fmt.Errorf("%s: value too great for base (error token is %q)", e.Args[1], e.Args[1])
	if state, _ := ctx.Value(runStateKey{}).(*runState); state != nil {
		state.recordArithmeticError(err)
	}
	_, _ = fmt.Fprint(e.Stdout, "0")
	return 0
}
