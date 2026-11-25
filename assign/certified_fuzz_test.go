package assign

import (
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"slices"
	"testing"
)

var (
	dyadicScalePow1074 = new(big.Int).Lsh(big.NewInt(1), 1074)
	dyadicPow2304      = new(big.Int).Lsh(big.NewInt(1), dyadicWords*64)
)

func dyadicToBigInt(d dyadic2304) *big.Int {
	// Little-endian words converted via big-endian byte layout.
	var be [dyadicWords * 8]byte
	for w := range dyadicWords {
		binary.BigEndian.PutUint64(be[(dyadicWords-1-w)*8:], d.w[w])
	}
	v := new(big.Int).SetBytes(be[:])
	if (d.w[dyadicWords-1] >> 63) != 0 {
		v.Sub(v, dyadicPow2304)
	}
	return v
}

func dyadicToBigRat(d dyadic2304) *big.Rat {
	return new(big.Rat).SetFrac(dyadicToBigInt(d), dyadicScalePow1074)
}

func ratHalveFloorFixed(r *big.Rat) *big.Rat {
	// Multiply r by 2^1074 (exact integer for representable dyadics), arithmetic-shift by 1, divide by 2^1074.
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(dyadicScalePow1074))
	if !scaled.IsInt() {
		panic("ratHalveFloorFixed called on non-dyadic2304 value")
	}
	num := new(big.Int).Rsh(scaled.Num(), 1)
	return new(big.Rat).SetFrac(num, dyadicScalePow1074)
}

func ratToFloat64Floor(r *big.Rat) float64 {
	rf, exact := r.Float64()
	if exact {
		return rf
	}
	if math.IsInf(rf, 1) {
		return math.MaxFloat64
	}
	if math.IsInf(rf, -1) {
		return rf
	}
	if new(big.Rat).SetFloat64(rf).Cmp(r) > 0 {
		return math.Nextafter(rf, math.Inf(-1))
	}
	return rf
}

func sameNormFloat64(a, b float64) bool {
	if a == 0 && b == 0 {
		return true
	}
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Float64bits(a) == math.Float64bits(b)
}

func assertDyadicMatchesRat(tb testing.TB, op string, d dyadic2304, want *big.Rat) {
	tb.Helper()
	gotRat := dyadicToBigRat(d)
	if gotRat.Cmp(want) != 0 {
		tb.Fatalf("%s: dyadic=%s != want=%s", op, gotRat.RatString(), want.RatString())
	}
	if d.sign() != want.Sign() {
		tb.Fatalf("%s: sign()=%d != want.Sign()=%d", op, d.sign(), want.Sign())
	}
	gotF, gotExact := d.toFloat64()
	wantF, wantExact := want.Float64()
	if !sameNormFloat64(gotF, wantF) || gotExact != wantExact {
		tb.Fatalf("%s: toFloat64()=(%.17g,%v) != big.Rat=(%.17g,%v)", op, gotF, gotExact, wantF, wantExact)
	}
	gotFloor := d.toFloat64Floor()
	wantFloor := ratToFloat64Floor(want)
	if !sameNormFloat64(gotFloor, wantFloor) {
		tb.Fatalf("%s: toFloat64Floor()=%.17g != want=%.17g", op, gotFloor, wantFloor)
	}
	gotFits := dyadicFitsFloat64(d)
	wantFits := !math.IsInf(wantF, 0) && !math.IsNaN(wantF) && !math.IsInf(wantFloor, 0) && !math.IsNaN(wantFloor)
	if gotFits != wantFits {
		tb.Fatalf("%s: dyadicFitsFloat64()=%v != want=%v (f=%g floor=%g)", op, gotFits, wantFits, wantF, wantFloor)
	}
}

var dyadicSpecialPalette = []float64{
	0.0,
	math.Copysign(0, -1),
	math.SmallestNonzeroFloat64,
	-math.SmallestNonzeroFloat64,
	math.Float64frombits((uint64(1) << 52) - 1),
	-math.Float64frombits((uint64(1) << 52) - 1),
	math.Float64frombits(uint64(1) << 52),
	-math.Float64frombits(uint64(1) << 52),
	math.Ldexp(1.0, -1011), // crosses w[0] -> w[1] word boundary
	math.Ldexp(1.0, -1010),
	math.Ldexp(1.0, -53),
	math.Ldexp(1.0, -52),
	1.0,
	-1.0,
	0.1,
	-0.3,
	math.Ldexp(1.0, 63),
	math.Ldexp(1.0, 512),
	math.Nextafter(math.MaxFloat64, 0),
	-math.Nextafter(math.MaxFloat64, 0),
	math.MaxFloat64,
	-math.MaxFloat64,
	0.5 * (math.MaxFloat64 - math.Nextafter(math.MaxFloat64, 0)),
	math.Inf(1),
	math.Inf(-1),
	math.NaN(),
}

type dyadicFuzzCursor struct {
	data []byte
	pos  int
}

func (c *dyadicFuzzCursor) nextByte() byte {
	if len(c.data) == 0 {
		return 0
	}
	b := c.data[c.pos%len(c.data)]
	c.pos++
	return b
}

func (c *dyadicFuzzCursor) nextUint64() uint64 {
	var buf [8]byte
	for i := range buf {
		buf[i] = c.nextByte()
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func (c *dyadicFuzzCursor) nextFloat() float64 {
	sel := c.nextByte()
	if sel < 160 {
		return dyadicSpecialPalette[int(sel)%len(dyadicSpecialPalette)]
	}
	return math.Float64frombits(c.nextUint64())
}

// FuzzAssignDyadic2304 tests dyadic2304 add, sub, neg, halve, cmp, float64 conversion,
// flooring, and shiftSquareExactDuals against math/big.Rat, together with aliased 2D
// LinearSum and fail-closed -1 overwrites in SolveInto.
func FuzzAssignDyadic2304(f *testing.F) {
	seeds := [][]byte{
		{0, 2, 20, 21, 3, 18, 12, 13, 8, 9, 10, 11},
		{2, 20, 22, 2, 21, 16, 17, 14, 15},
		{8, 9, 20, 2, 3, 21, 19, 4, 5},
		{23, 24, 25, 0, 1, 20, 21, 2},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cur := dyadicFuzzCursor{data: data}
		fA := cur.nextFloat()
		fB := cur.nextFloat()
		fC := cur.nextFloat()

		checkFromFloat := func(v float64) (dyadic2304, *big.Rat, bool) {
			d, ok := dyadicFromFloat(v)
			wantOK := !math.IsNaN(v) && !math.IsInf(v, 0)
			if ok != wantOK {
				t.Fatalf("dyadicFromFloat(%g) ok=%v want=%v", v, ok, wantOK)
			}
			if !ok {
				return dyadic2304{}, nil, false
			}
			r := new(big.Rat).SetFloat64(v)
			assertDyadicMatchesRat(t, "dyadicFromFloat", d, r)
			return d, r, true
		}

		dA, rA, okA := checkFromFloat(fA)
		dB, rB, okB := checkFromFloat(fB)
		dC, rC, okC := checkFromFloat(fC)

		if okA && okB {
			if got, want := dyadicCmp(dA, dB), rA.Cmp(rB); got != want {
				t.Fatalf("dyadicCmp(%g, %g)=%d != %d", fA, fB, got, want)
			}
			dSum := dyadicAdd(dA, dB)
			rSum := new(big.Rat).Add(rA, rB)
			assertDyadicMatchesRat(t, "dyadicAdd", dSum, rSum)

			dSub := dyadicSub(dA, dB)
			rSub := new(big.Rat).Sub(rA, rB)
			assertDyadicMatchesRat(t, "dyadicSub", dSub, rSub)

			dNegA := dyadicNeg(dA)
			rNegA := new(big.Rat).Neg(rA)
			assertDyadicMatchesRat(t, "dyadicNeg", dNegA, rNegA)

			dHalfA := dyadicHalve(dA)
			assertDyadicMatchesRat(t, "dyadicHalve", dHalfA, ratHalveFloorFixed(rA))
			if (dA.w[0] & 1) == 0 {
				assertDyadicMatchesRat(t, "dyadicHalve(even)", dHalfA, new(big.Rat).Quo(rA, big.NewRat(2, 1)))
			}

			// Multi-limb chain mixing giant and subnormal scales across carries.
			if okC {
				dChain := dyadicSub(dyadicAdd(dSum, dC), dB)
				rChain := new(big.Rat).Sub(new(big.Rat).Add(rSum, rC), rB)
				assertDyadicMatchesRat(t, "multiLimbChain", dChain, rChain)
				if dyadicCmp(dChain, dA) != rChain.Cmp(rA) {
					t.Fatalf("chain cmp mismatch")
				}
			}

			// Propagate carries across 64-bit limbs by adding and subtracting 2^-1074 on extreme sums.
			dTiny, _ := dyadicFromFloat(math.SmallestNonzeroFloat64)
			rTiny := new(big.Rat).SetFloat64(math.SmallestNonzeroFloat64)
			assertDyadicMatchesRat(t, "addTiny", dyadicAdd(dSum, dTiny), new(big.Rat).Add(rSum, rTiny))
			assertDyadicMatchesRat(t, "subTiny", dyadicSub(dSum, dTiny), new(big.Rat).Sub(rSum, rTiny))
		}

		// Differential test for shiftSquareExactDuals across 1..6 elements.
		m := 1 + int(cur.nextByte()%6)
		uExact := make([]dyadic2304, m)
		vExact := make([]dyadic2304, m)
		uRat := make([]*big.Rat, m)
		vRat := make([]*big.Rat, m)
		for i := range m {
			fv := cur.nextFloat()
			if math.IsNaN(fv) || math.IsInf(fv, 0) {
				fv = float64(i)*1.5 - 3.0
			}
			uExact[i], _ = dyadicFromFloat(fv)
			uRat[i] = new(big.Rat).SetFloat64(fv)
		}
		for j := range m {
			fv := cur.nextFloat()
			if math.IsNaN(fv) || math.IsInf(fv, 0) {
				fv = -float64(j)*0.75 + 0.5
			}
			vExact[j], _ = dyadicFromFloat(fv)
			vRat[j] = new(big.Rat).SetFloat64(fv)
		}

		uShifted, vShifted := shiftSquareExactDuals(uExact, vExact)
		if len(uShifted) != m || len(vShifted) != m {
			t.Fatalf("shiftSquareExactDuals len mismatch got=(%d,%d) want=%d", len(uShifted), len(vShifted), m)
		}

		sMin := new(big.Rat).Set(uRat[0])
		sMax := new(big.Rat).Set(uRat[0])
		for i := 1; i < m; i++ {
			if uRat[i].Cmp(sMin) < 0 {
				sMin.Set(uRat[i])
			}
			if uRat[i].Cmp(sMax) > 0 {
				sMax.Set(uRat[i])
			}
		}
		for j := range m {
			nv := new(big.Rat).Neg(vRat[j])
			if nv.Cmp(sMin) < 0 {
				sMin.Set(nv)
			}
			if nv.Cmp(sMax) > 0 {
				sMax.Set(nv)
			}
		}
		deltaRat := new(big.Rat).Neg(ratHalveFloorFixed(new(big.Rat).Add(sMin, sMax)))
		for i := range m {
			assertDyadicMatchesRat(t, "shiftSquareU", uShifted[i], new(big.Rat).Add(uRat[i], deltaRat))
			assertDyadicMatchesRat(t, "shiftSquareV", vShifted[i], new(big.Rat).Sub(vRat[i], deltaRat))
		}
		for i := range m {
			for j := range m {
				lhs := dyadicAdd(uShifted[i], vShifted[j])
				rhs := new(big.Rat).Add(uRat[i], vRat[j])
				assertDyadicMatchesRat(t, "shiftPairInvariant", lhs, rhs)
			}
		}

		// Check aliased 2D LinearSum inputs and SolveInto fail-closed -1 overwrites.
		if cur.nextByte()%16 == 0 {
			rows := 2 + int(cur.nextByte()%3)
			cols := 2 + int(cur.nextByte()%3)
			backing := make([]float64, rows+cols-1)
			for idx := range backing {
				backing[idx] = float64(int(cur.nextByte()%21)-10) * 0.5
			}
			aliased := make([][]float64, rows)
			for r := range rows {
				aliased[r] = backing[r : r+cols]
			}
			origBacking := slices.Clone(backing)
			sense := Minimize
			if cur.nextByte()&1 == 1 {
				sense = Maximize
			}
			res2D, err2D := LinearSum(t.Context(), aliased, sense, WithCertified(true))
			if err2D != nil {
				t.Fatalf("aliased LinearSum failed: %v", err2D)
			}
			if !slices.Equal(backing, origBacking) {
				t.Fatalf("aliased 2D slice backing was mutated")
			}

			flat := make([]float64, rows*cols)
			for r := range rows {
				copy(flat[r*cols:(r+1)*cols], aliased[r])
			}
			k := min(rows, cols)
			rInd := make([]int, k+2)
			cInd := make([]int, k+2)
			for idx := range rInd {
				rInd[idx] = 777 + idx
				cInd[idx] = 888 + idx
			}
			faultCtx := withAssignCertFault(t.Context(), func(_, _ int, _ Sense, r *Result, _ *error) {
				if len(r.ColInd) > 0 {
					r.ColInd[0] = -1
				} else {
					r.Objective += 1.0
				}
			})
			obj, errInto := SolveInto(faultCtx, rows, cols, flat, sense, rInd, cInd, nil, WithCertified(true))
			if !errors.Is(errInto, ErrCertificationFailed) || obj != 0 {
				t.Fatalf("SolveInto fault got=(%g,%v) want=(0,ErrCertificationFailed)", obj, errInto)
			}
			for idx := range k {
				if rInd[idx] != -1 || cInd[idx] != -1 {
					t.Fatalf("SolveInto did not wipe slot %d: (%d,%d)", idx, rInd[idx], cInd[idx])
				}
			}
			for idx := k; idx < len(rInd); idx++ {
				if rInd[idx] != 777+idx || cInd[idx] != 888+idx {
					t.Fatalf("SolveInto modified trailing slot %d: (%d,%d)", idx, rInd[idx], cInd[idx])
				}
			}
			_ = res2D
		}
	})
}

// TestLinearSumJaggedAndAliased2D verifies that jagged 2D slices fail with ErrInvalidDimensions
// and overlapping row-aliased 2D slices match flat inputs without mutating caller buffers.
func TestLinearSumJaggedAndAliased2D(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	jaggedCases := [][][]float64{
		{{1, 2, 3}, {4, 5}},
		{{1, 2}, {3, 4, 5}},
		{{1, 2}, nil},
	}
	for _, mat := range jaggedCases {
		for _, cert := range []bool{false, true} {
			if _, err := LinearSum(ctx, mat, Minimize, WithCertified(cert)); !errors.Is(err, ErrInvalidDimensions) {
				t.Fatalf("jagged LinearSum(cert=%v) err=%v, want ErrInvalidDimensions", cert, err)
			}
		}
	}

	// Overlapping sliding-window rows and shared duplicate row backing slices.
	backing := []float64{12.5, -3.25, 4.75, 8.0, -1.5, 6.25, 2.0}
	sliding := [][]float64{
		backing[0:4],
		backing[1:5],
		backing[2:6],
	}
	sharedRow := []float64{3.5, -2.0, 9.25}
	dupRows := [][]float64{sharedRow, sharedRow, {1.0, 4.5, -0.5}}

	for _, tc := range []struct {
		name    string
		mat     [][]float64
		buf     []float64
		wantBuf []float64
	}{
		{"slidingWindow", sliding, backing, slices.Clone(backing)},
		{"duplicateRowSlice", dupRows, sharedRow, slices.Clone(sharedRow)},
	} {
		rows, cols := len(tc.mat), len(tc.mat[0])
		flat := make([]float64, rows*cols)
		for r := range rows {
			copy(flat[r*cols:(r+1)*cols], tc.mat[r])
		}
		for _, sense := range []Sense{Minimize, Maximize} {
			for _, simdOpt := range []Option{WithSIMD(true), WithoutSIMD()} {
				res2D, err2D := LinearSum(ctx, tc.mat, sense, simdOpt, WithCertified(true))
				resFlat, errFlat := LinearSumFlat(ctx, rows, cols, flat, sense, simdOpt, WithCertified(true))
				if err2D != nil || errFlat != nil {
					t.Fatalf("%s (%v): err2D=%v errFlat=%v", tc.name, sense, err2D, errFlat)
				}
				if !slices.Equal(tc.buf, tc.wantBuf) {
					t.Fatalf("%s (%v): caller buffer mutated", tc.name, sense)
				}
				if math.Float64bits(res2D.Objective) != math.Float64bits(resFlat.Objective) ||
					!slices.Equal(res2D.RowInd, resFlat.RowInd) ||
					!slices.Equal(res2D.ColInd, resFlat.ColInd) {
					t.Fatalf("%s (%v): 2D vs flat mismatch", tc.name, sense)
				}
			}
		}
	}
}

// TestSolveIntoFailClosedMatchWiping confirms SolveInto overwrites all k match
// entries in rowInd[:k] and colInd[:k] with -1 whenever ErrCertificationFailed occurs,
// without mutating caller trailing slice elements beyond k.
func TestSolveIntoFailClosedMatchWiping(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	cases := []struct {
		rows, cols int
		cost       []float64
	}{
		{3, 3, []float64{1, 5, 9, 4, 2, 8, 7, 6, 3}},
		{2, 4, []float64{3, 1, 7, 2, 8, 5, 0, 4}},
		{4, 2, []float64{3, 1, 7, 2, 8, 5, 0, 4}},
	}
	faults := []struct {
		name string
		fn   assignFaultFunc
	}{
		{
			"duplicateColMatch",
			func(_, _ int, _ Sense, res *Result, _ *error) {
				if len(res.ColInd) >= 2 {
					res.ColInd[1] = res.ColInd[0]
				}
			},
		},
		{
			"corruptObjective",
			func(_, _ int, _ Sense, res *Result, _ *error) {
				res.Objective += 123.5
			},
		},
		{
			"corruptDualInfeasible",
			func(_, _ int, sense Sense, res *Result, _ *error) {
				if len(res.U) > 0 {
					if sense == Minimize {
						res.U[0] += 1e6
					} else {
						res.U[0] -= 1e6
					}
				}
			},
		},
	}

	for _, tc := range cases {
		k := min(tc.rows, tc.cols)
		for _, sense := range []Sense{Minimize, Maximize} {
			for _, fCase := range faults {
				for _, simdOpt := range []Option{WithSIMD(true), WithoutSIMD()} {
					rInd := make([]int, k+3)
					cInd := make([]int, k+3)
					for idx := range rInd {
						rInd[idx] = 500 + idx
						cInd[idx] = 900 + idx
					}
					ws := AcquireWorkspace(tc.rows, tc.cols)
					fCtx := withAssignCertFault(ctx, fCase.fn)
					obj, err := SolveInto(fCtx, tc.rows, tc.cols, tc.cost, sense, rInd, cInd, ws, simdOpt, WithCertified(true))
					ReleaseWorkspace(ws)
					if !errors.Is(err, ErrCertificationFailed) || obj != 0 {
						t.Fatalf("%dx%d %v %s: got=(%g,%v) want=(0,ErrCertificationFailed)",
							tc.rows, tc.cols, sense, fCase.name, obj, err)
					}
					for idx := range k {
						if rInd[idx] != -1 || cInd[idx] != -1 {
							t.Fatalf("%dx%d %v %s: slot %d not wiped: (%d,%d)",
								tc.rows, tc.cols, sense, fCase.name, idx, rInd[idx], cInd[idx])
						}
					}
					for idx := k; idx < len(rInd); idx++ {
						if rInd[idx] != 500+idx || cInd[idx] != 900+idx {
							t.Fatalf("%dx%d %v %s: trailing slot %d modified: (%d,%d)",
								tc.rows, tc.cols, sense, fCase.name, idx, rInd[idx], cInd[idx])
						}
					}
				}
			}
		}
	}
}
