package goblin

import (
	"errors"
	"fmt"
	"math"
)

// VerifyTableauRowForTest calls s.ComputeTableauRow(r) on a factorized basis and checks
// that B^T * rho = e_r, tabRow matches rho^T * [A I] on nonbasic columns and e_r on
// basic columns, and sparse/chunk index tracking covers every nonzero tabRow entry.
func VerifyTableauRowForTest(s *LPSolver, r int) error {
	if s == nil || s.mat == nil || s.ws == nil {
		return errors.New("VerifyTableauRowForTest: nil solver or workspace")
	}
	if s.invalidBounds || s.invalidCut {
		return errors.New("VerifyTableauRowForTest: solver has invalid bounds or cuts")
	}
	m := s.TotalRows()
	n := s.mat.numVars
	tot := n + m
	if m <= 0 || r < 0 || r >= m {
		return fmt.Errorf("VerifyTableauRowForTest: row %d out of range [0, %d)", r, m)
	}

	ws := s.ws
	if !s.luValid || ws.lu == nil || ws.lu.m != m || ws.lu.NeedsRefactor() {
		if err := s.factorizeBasis(); err != nil {
			s.reinitCrashKeepingBounds()
			if err2 := s.factorizeBasis(); err2 != nil {
				return fmt.Errorf("factorizeBasis failed: %w", err2)
			}
		}
	}

	s.ComputeTableauRow(r)

	rows := make([]int, 0, 16)
	vals := make([]float64, 0, 16)

	// 1. Verify B^T * rho = e_r across every basis column B[:, q].
	for q := range m {
		bVar := ws.basis[q]
		if uint(bVar) >= uint(tot) {
			return fmt.Errorf("invalid basis[%d]=%d (tot=%d)", q, bVar, tot)
		}
		rows, vals = s.appendBasisColumn(bVar, rows[:0], vals[:0])
		var acc dot2Acc
		absSum := 0.0
		for k, row := range rows {
			if uint(row) >= uint(m) {
				return fmt.Errorf("basis column %d produced out-of-range row %d", bVar, row)
			}
			rv := ws.rho[row]
			cv := vals[k]
			acc.addProd(rv, cv)
			absSum += math.Abs(rv * cv)
		}
		dot, eb, ok := acc.result()
		if !ok || math.IsNaN(dot) || math.IsInf(dot, 0) || isNonFinite(eb) {
			return fmt.Errorf("non-finite B^T*rho at basis slot %d", q)
		}
		dot -= eb
		want := 0.0
		if q == r {
			want = 1.0
		}
		tol := 1e-6 * max(1.0, absSum)
		if math.Abs(dot-want) > tol {
			return fmt.Errorf("B^T*rho[%d]=%.12g, want %.12g (tol=%.3e)", q, dot, want, tol)
		}
	}

	// 2. Verify tabRow[k] equals delta_{k, basis[r]} on basic columns and rho^T * [A I][:, k] on nonbasic columns.
	for k := range tot {
		tk := ws.tabRow[k]
		if math.IsNaN(tk) || math.IsInf(tk, 0) {
			return fmt.Errorf("non-finite tabRow[%d]=%v", k, tk)
		}
		if ws.status[k] == statusBasic {
			want := 0.0
			if k == ws.basis[r] {
				want = 1.0
			}
			if math.Abs(tk-want) > 1e-12 {
				return fmt.Errorf("basic column %d tabRow=%.12g, want %.12g", k, tk, want)
			}
			continue
		}
		rows, vals = s.appendBasisColumn(k, rows[:0], vals[:0])
		var acc dot2Acc
		absSum := 0.0
		for idx, row := range rows {
			rv := ws.rho[row]
			cv := vals[idx]
			acc.addProd(rv, cv)
			absSum += math.Abs(rv * cv)
		}
		want, eb, ok := acc.result()
		if !ok || math.IsNaN(want) || math.IsInf(want, 0) || isNonFinite(eb) {
			return fmt.Errorf("non-finite rho^T*a_%d", k)
		}
		want -= eb
		tol := 1e-6 * max(1.0, absSum)
		if math.Abs(tk-want) > tol {
			return fmt.Errorf("nonbasic column %d tabRow=%.12g, want %.12g (tol=%.3e)", k, tk, want, tol)
		}
	}

	// 3. Verify sparse support and 64-column chunk masks cover every nonzero entry.
	rhoSeen := make([]bool, m)
	for _, idx := range ws.rhoIdx {
		if uint(idx) >= uint(m) {
			return fmt.Errorf("out-of-range rhoIdx=%d (m=%d)", idx, m)
		}
		rhoSeen[idx] = true
	}
	for i := range m {
		if ws.rho[i] != 0 && !rhoSeen[i] {
			return fmt.Errorf("nonzero rho[%d]=%.3e missing from rhoIdx", i, ws.rho[i])
		}
	}

	slackSeen := make([]bool, tot)
	for _, idx := range ws.tabRowIdx {
		if uint(idx) >= uint(tot) {
			return fmt.Errorf("out-of-range tabRowIdx=%d (tot=%d)", idx, tot)
		}
		slackSeen[idx] = true
	}
	if ws.tabRowSparse {
		for k := range tot {
			if ws.tabRow[k] != 0 && !slackSeen[k] {
				return fmt.Errorf("sparse tabRow[%d]=%.3e missing from tabRowIdx", k, ws.tabRow[k])
			}
		}
	} else {
		numChunks := (n + tabRowChunkMask) >> tabRowChunkBits
		chunkSeen := make([]bool, numChunks)
		for _, ch := range ws.tabRowChunks {
			if uint(ch) >= uint(numChunks) {
				return fmt.Errorf("out-of-range tabRowChunks entry %d (numChunks=%d)", ch, numChunks)
			}
			chunkSeen[ch] = true
		}
		for j := range n {
			if ws.tabRow[j] != 0 {
				ch := j >> tabRowChunkBits
				if !chunkSeen[ch] || !ws.tabRowChunkMark[ch] {
					return fmt.Errorf("chunked tabRow[%d]=%.3e missing from chunk %d", j, ws.tabRow[j], ch)
				}
			}
		}
		for k := n; k < tot; k++ {
			if ws.tabRow[k] != 0 && !slackSeen[k] {
				return fmt.Errorf("chunked slack tabRow[%d]=%.3e missing from tabRowIdx", k, ws.tabRow[k])
			}
		}
	}

	return nil
}
