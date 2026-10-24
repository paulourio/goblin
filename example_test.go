package goblin_test

import (
	"context"
	"fmt"
	"time"

	gb "github.com/paulourio/goblin"
)

func ExampleModel_Solve() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := gb.NewModel("knapsack", gb.Maximize)
	x := m.AddBinaryVar("x")
	y := m.AddIntVar("y", 0, 10)
	m.SetObjective(gb.NewExpr(x.T(12), y.T(7)))
	m.AddConstraint("cap", gb.NewExpr(x.T(4), y.T(3)).Le(25))

	res, err := m.Solve(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%s obj=%.0f x=%.0f y=%.0f\n", res.Status, res.Objective, res.Value(x), res.Value(y))
	// Output:
	// status=Optimal obj=61 x=1 y=7
}

func ExampleSolveCompiled() {
	m := gb.NewModel("lp", gb.Minimize)
	x := m.AddContVar("x", 0, 10)
	y := m.AddContVar("y", 0, 10)
	m.SetObjective(gb.NewExpr(x.T(3), y.T(5)))
	m.AddConstraint("c1", gb.NewExpr(x.T(1), y.T(2)).Ge(10))

	compiled := m.Compile()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res, err := gb.SolveCompiled(ctx, compiled)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status=%s obj=%.1f\n", res.Status, res.Objective)
	// Output:
	// status=Optimal obj=25.0
}
