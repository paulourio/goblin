package assign_test

import (
	"context"
	"fmt"
	"time"

	"github.com/paulourio/goblin/assign"
)

func ExampleLinearSum() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cost := [][]float64{
		{4, 1, 3},
		{2, 0, 5},
		{3, 2, 2},
	}
	res, err := assign.LinearSum(ctx, cost, assign.Minimize)
	if err != nil {
		panic(err)
	}
	fmt.Printf("obj=%.0f rows=%v cols=%v\n", res.Objective, res.RowInd, res.ColInd)
	// Output:
	// obj=5 rows=[0 1 2] cols=[1 0 2]
}
