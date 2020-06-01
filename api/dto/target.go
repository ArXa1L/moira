package dto

import (
	"fmt"
	"time"

	"github.com/go-graphite/carbonapi/expr/functions"
	"github.com/go-graphite/carbonapi/expr/metadata"

	"github.com/go-graphite/carbonapi/pkg/parser"
)

func init() {
	functions.New(nil)
}

type problemOfType string

const (
	isWarn problemOfType = "warn"
	isBad  problemOfType = "bad"
)

type problemOfTarget struct {
	Argument    string            `json:"argument"`
	Type        problemOfType     `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Position    int               `json:"position"`
	Problems    []problemOfTarget `json:"problems,omitempty"`
}

type treeOfProblems struct {
	SyntaxOk       bool             `json:"syntax_ok"`
	TreeOfProblems *problemOfTarget `json:"tree_of_problems,omitempty"`
}

func TargetVerification(targets []string, ttl time.Duration, isRemote bool) []treeOfProblems {
	functionsOfTargets := make([]treeOfProblems, 0)

	for _, target := range targets {
		functionsOfTarget := treeOfProblems{SyntaxOk: true}

		expr, _, err := parser.ParseExpr(target)
		if err != nil {
			functionsOfTarget.SyntaxOk = false
			functionsOfTargets = append(functionsOfTargets, functionsOfTarget)
			continue
		}

		functionsOfTarget.TreeOfProblems = checkExpression(expr, ttl, isRemote)
		functionsOfTargets = append(functionsOfTargets, functionsOfTarget)
	}

	return functionsOfTargets
}

func checkExpression(expression parser.Expr, ttl time.Duration, isRemote bool) *problemOfTarget {
	target := expression.Target()
	problemFunction := checkFunction(target, isRemote)

	if argument, ok := functionArgumentsInTheRangeTTL(expression, ttl); !ok {
		if problemFunction == nil {
			problemFunction = &problemOfTarget{Argument: target}
		}

		problemFunction.Problems = append(problemFunction.Problems, problemOfTarget{
			Argument: argument,
			Type:     isBad,
			Position: 1,
			Description: fmt.Sprintf(
				"The function %s has a time sampling parameter %s larger than allowed by the config:%s",
				target, expression.Args()[1].StringValue(), ttl.String()),
		})
	}

	for position, argument := range expression.Args() {
		if !argument.IsFunc() {
			continue
		}

		if badFunc := checkExpression(argument, ttl, isRemote); badFunc != nil {
			badFunc.Position = position

			if problemFunction == nil {
				problemFunction = &problemOfTarget{Argument: target}
			}

			problemFunction.Problems = append(problemFunction.Problems, *badFunc)
		}
	}

	return problemFunction
}

func checkFunction(funcName string, isRemote bool) *problemOfTarget {
	switch funcName {
	case "removeAbovePercentile", "removeBelowPercentile", "smartSummarize", "summarize":
		return &problemOfTarget{
			Argument:    funcName,
			Type:        isBad,
			Description: "This function is unstable: it can return different historical values with each evaluation. Moira will show unexpected values that you don't see on your graphs.",
		}
	case "averageAbove", "averageBelow", "averageOutsidePercentile", "currentAbove",
		"currentBelow", "filterSeries", "highest", "highestAverage", "highestCurrent",
		"highestMax", "limit", "lowest", "lowestAverage", "lowestCurrent", "maximumAbove",
		"maximumBelow", "minimumAbove", "minimumBelow", "mostDeviant", "removeBetweenPercentile",
		"removeEmptySeries", "useSeriesAbove":
		return &problemOfTarget{
			Argument:    funcName,
			Type:        isWarn,
			Description: "This function shows and hides entire metric series based on their values. Moira will send frequent false NODATA notifications.",
		}
	case "alpha", "areaBetween", "color", "consolidateBy", "cumulative", "dashed",
		"drawAsInfinite", "lineWidth", "secondYAxis", "setXFilesFactor", "sortBy",
		"sortByMaxima", "sortByMinima", "sortByName", "sortByTotal", "stacked", "threshold", "verticalLine":
		return &problemOfTarget{
			Argument:    funcName,
			Type:        isWarn,
			Description: "This function affects only visual graph representation. It is meaningless in Moira.",
		}
	default:
		if !isRemote && !funcIsSupported(funcName) {
			return &problemOfTarget{
				Argument:    funcName,
				Type:        isBad,
				Description: "Function is not supported, if you want to use it, switch to remote",
				Position:    0,
				Problems:    nil,
			}
		}
	}

	return nil
}

// functionArgumentsInTheRangeTTL: Checking function arguments that they are in the range of TTL
func functionArgumentsInTheRangeTTL(expression parser.Expr, ttl time.Duration) (string, bool) {
	switch expression.Target() {
	case "timeStack", "exponentialMovingAverage", "movingMedian", "delay",
		"timeSlice", "integralByInterval", "movingMin", "movingWindow",
		"time", "timeFunction", "randomWalk", "randomWalkFunction",
		"sin", "sinFunction", "summarize", "divideSeries",
		"linearRegression", "movingAverage", "movingMax", "movingSum",
		"timeShift":

		// Check that we can take the second argument
		if len(expression.Args()) < 2 {
			break
		}

		argument, argumentDuration := positiveDuration(expression.Args()[1])
		return argument, argumentDuration <= ttl || ttl == 0
	}

	return "", true
}

func funcIsSupported(funcName string) bool {
	_, ok := metadata.FunctionMD.Functions[funcName]
	return ok || funcName == ""
}

// positiveDuration:
func positiveDuration(argument parser.Expr) (string, time.Duration) {
	var secondTimeDuration time.Duration
	var value string

	switch argument.Type() {
	case 2: // EtConst
		if second := argument.FloatValue(); second != 0 {
			value = fmt.Sprint(second)
			secondTimeDuration = time.Duration(second) * time.Second
		}
	case 3: // EtString
		value = argument.StringValue()
		second, _ := parser.IntervalString(value, 1)

		secondTimeDuration = time.Second * time.Duration(second)
	default: // 0 = EtName, 1 = EtFunc
	}

	if secondTimeDuration < 0 {
		secondTimeDuration *= -1
	}

	return value, secondTimeDuration
}
