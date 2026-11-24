/-
Apps.LsapCheck: CLI verifier for LSAP optimality and Hall infeasibility certificates
using GoblinFormal.Track1 (checkLSAPForbidden, checkLSAPForbiddenSafe,
checkLSAPForbiddenTranspose, checkLSAPForbiddenTransposeSafe,
checkHallInfeasible, checkHallInfeasibleTranspose).
-/

import Apps.Common
import GoblinFormal.Track1

open Apps.Common
open GoblinFormal
open GoblinFormal.Track1

namespace Apps.LsapCheck

private inductive Mode where
  | exact
  | safe
  | hall
  deriving BEq

private def Mode.toStr : Mode → String
  | .exact => "exact"
  | .safe => "safe"
  | .hall => "hall"

private structure ParsedArgs where
  modeOverride : Option Mode := none
  tolOverride : Option Rat := none
  printBound : Bool := false
  jsonl : Bool := false
  batchPath : Option String := none
  filePath : Option String := none
  showHelp : Bool := false

private structure VerifyResult where
  mode : Mode
  tol : Rat
  bound : Option Rat

private def usageText : String :=
  "Usage: lsap-check [--exact | --safe | --hall] [--tol <rat>] [--print-bound] [--jsonl | --batch <array.json | ->] [<certificate.json | ->]\n" ++
  "  --exact         Verify exact complementary slackness (default unless JSON sets mode='safe' or 'hall')\n" ++
  "  --safe          Verify rigorous dual lower bound via uSafeOpt = min_{j finite} (C[i,j] - v[j])\n" ++
  "  --hall          Verify Hall's Marriage Theorem infeasibility witness (|N_finite(S)| < |S|)\n" ++
  "  --tol <rat>     Override gap tolerance for safe mode (default 0)\n" ++
  "  --print-bound   Include certified bound in output\n" ++
  "  --jsonl         Stream line-delimited JSON requests from stdin\n" ++
  "  --batch <path>  Verify non-empty JSON array of certificates from file or '-'\n" ++
  "  -h, --help      Show this help message"

private def parseRatString (s : String) : Except String Rat :=
  (JsonVal.str s).asRat

private def parseArgs : List String → ParsedArgs → Except String ParsedArgs
  | [], acc => .ok acc
  | "--help" :: rest, acc => parseArgs rest { acc with showHelp := true }
  | "-h" :: rest, acc => parseArgs rest { acc with showHelp := true }
  | "--exact" :: rest, acc => parseArgs rest { acc with modeOverride := some .exact }
  | "--safe" :: rest, acc => parseArgs rest { acc with modeOverride := some .safe }
  | "--hall" :: rest, acc => parseArgs rest { acc with modeOverride := some .hall }
  | "--infeasible" :: rest, acc => parseArgs rest { acc with modeOverride := some .hall }
  | "--print-bound" :: rest, acc => parseArgs rest { acc with printBound := true }
  | "--jsonl" :: rest, acc => parseArgs rest { acc with jsonl := true }
  | "--batch" :: raw :: rest, acc =>
    if acc.batchPath.isSome then .error "duplicate --batch flag"
    else parseArgs rest { acc with batchPath := some raw }
  | ["--batch"], _ => .error "missing path after --batch"
  | "--tol" :: raw :: rest, acc => do
    let r ← parseRatString raw
    parseArgs rest { acc with tolOverride := some r }
  | ["--tol"], _ => .error "missing value after --tol"
  | arg :: rest, acc =>
    if arg.startsWith "-" && arg != "-" then
      .error s!"unknown flag '{arg}'"
    else
      match acc.filePath with
      | some prev => .error s!"unexpected extra positional argument '{arg}' (already got '{prev}')"
      | none => parseArgs rest { acc with filePath := some arg }

private def asRatOrBits (v : JsonVal) : Except String Rat :=
  asRatOrBitsWith JsonVal.asUInt64 v

private def asOptCostCell (v : JsonVal) (negate bitsEncoding : Bool) : Except String (Option Rat) :=
  if bitsEncoding then
    match v with
    | .null => .ok none
    | _ => do
      let bits ← v.asUInt64
      match float64BitsToRat bits with
      | some r => .ok (some r)
      | none => .error "non-finite float64 bits (Inf/NaN) in bits-encoded cost cell"
  else
    match v with
    | .null => .ok none
    | .str s =>
      if s == "+Inf" || s == "+inf" then
        if !negate then .ok none
        else .error "unbounded '+Inf' cost in max sense"
      else if s == "-Inf" || s == "-inf" then
        if negate then .ok none
        else .error "unbounded '-Inf' cost in min sense"
      else
        some <$> v.asRat
    | .obj fields =>
      match v.getField? "forbidden" with
      | some fv => do
        if fields.size != 1 then
          throw "forbidden cost object must contain only the 'forbidden' field"
        let b ← fv.asBool
        if b then .ok none
        else .error "cost object with 'forbidden':false is invalid"
      | none => some <$> asRatOrBits v
    | _ => some <$> asRatOrBits v

private def parseSizedRatArray (v : JsonVal) (expectedLen : Nat) (bitsEncoding : Bool) (label : String) :
    Except String (Array Rat) := do
  let arr ← v.asArray
  if arr.size != expectedLen then
    .error s!"{label}: expected length {expectedLen}, got {arr.size}"
  else
    let mut out : Array Rat := Array.mkEmpty expectedLen
    for idx in [:arr.size] do
      let r ←
        if bitsEncoding then
          let bits ← arr[idx]!.asUInt64
          match float64BitsToRat bits with
          | some val => .ok val
          | none => .error s!"{label}[{idx}]: non-finite float64 bits (Inf/NaN)"
        else
          asRatOrBits arr[idx]!
      out := out.push r
    .ok out

private def parseDistinctFinArray (v : JsonVal) (expectedLen : Nat) (bound : Nat) (label : String) :
    Except String (Array (Fin bound)) := do
  let arr ← v.asArray
  if arr.size != expectedLen then
    .error s!"{label}: expected length {expectedLen}, got {arr.size}"
  else if expectedLen == 0 then
    .ok #[]
  else if expectedLen > bound then
    .error s!"{label}: length {expectedLen} exceeds bound {bound}"
  else
    let mut out : Array (Fin bound) := Array.mkEmpty expectedLen
    let mut seen : Array Bool := Array.replicate bound false
    for idx in [:arr.size] do
      let x ← arr[idx]!.asNat
      if h : x < bound then
        if seen[x]! then
          throw s!"{label}: duplicate index {x}"
        seen := seen.set! x true
        out := out.push ⟨x, h⟩
      else
        throw s!"{label}: index {x} out of range [0, {bound})"
    .ok out

private def parseEncodingFlag (root : JsonVal) (field : String) : Except String Bool := do
  match optNonNullField? root field with
  | none => .ok false
  | some ev =>
    let s ← ev.asString
    let low := s.toLower
    if low == "rat" || low == "default" then .ok false
    else if low == "bits" then .ok true
    else .error s!"invalid {field} '{s}' (expected 'rat' or 'bits')"

private def parseFlatCostMatrix (v : JsonVal) (rows cols : Nat) (negate bitsEncoding : Bool) :
    Except String (Array (Option Rat)) := do
  let rowVals ← v.asArray
  if rowVals.size != rows then
    .error s!"cost: expected {rows} rows, got {rowVals.size}"
  else
    -- Validate all row lengths before allocating flat storage.
    let mut parsedRows : Array (Array JsonVal) := Array.mkEmpty rows
    for r in [:rows] do
      let rowArr ← rowVals[r]!.asArray
      if rowArr.size != cols then
        throw s!"cost[{r}]: expected length {cols}, got {rowArr.size}"
      parsedRows := parsedRows.push rowArr
    let mut flat : Array (Option Rat) := Array.mkEmpty (rows * cols)
    for r in [:rows] do
      let rowArr := parsedRows[r]!
      for c in [:cols] do
        let cell ← asOptCostCell rowArr[c]! negate bitsEncoding
        flat := flat.push cell
    .ok flat

private def maxVerifierDim : Nat := 10_000_000

private def parseDims (root : JsonVal) : Except String (Nat × Nat) := do
  let (r, c) ←
    match root.getField? "rows", root.getField? "cols" with
    | some rv, some cv =>
      let r ← rv.asNat
      let c ← cv.asNat
      .ok (r, c)
    | none, none =>
      match root.getField? "m", root.getField? "n" with
      | some mv, some nv =>
        let m ← mv.asNat
        let n ← nv.asNat
        .ok (m, n)
      | _, _ => .error "missing required dimensions ('rows'/'cols' or 'm'/'n')"
    | _, _ => .error "both 'rows' and 'cols' must be provided together"
  if r > maxVerifierDim || c > maxVerifierDim then
    .error s!"dimensions ({r}, {c}) exceed maxVerifierDim ({maxVerifierDim})"
  else
    .ok (r, c)

private def parseSense (root : JsonVal) : Except String Bool := do
  match root.getField? "sense" with
  | none => .ok false
  | some sv =>
    let s ← sv.asString
    let low := s.toLower
    if low == "min" || low == "minimize" then
      .ok false
    else if low == "max" || low == "maximize" then
      .ok true
    else
      .error s!"invalid sense '{s}' (expected 'min' or 'max')"

private def parseMode (root : JsonVal) (override : Option Mode) : Except String Mode := do
  match override with
  | some m => .ok m
  | none =>
    match root.getField? "mode" with
    | some mv =>
      let s ← mv.asString
      let low := s.toLower
      if low == "exact" then
        .ok .exact
      else if low == "safe" then
        .ok .safe
      else if low == "hall" || low == "infeasible" then
        .ok .hall
      else
        .error s!"invalid mode '{s}' (expected 'exact', 'safe', 'hall', or 'infeasible')"
    | none =>
      match root.getField? "safe" with
      | none | some .null => .ok .exact
      | some sv =>
        let b ← sv.asBool
        .ok (if b then .safe else .exact)

private def parsePrimalPairs (root : JsonVal) (rows cols : Nat) :
    Except String (Array (Fin rows) × Array (Fin cols)) := do
  let k := min rows cols
  match root.getField? "row_ind", root.getField? "col_ind" with
  | some rv, some cv =>
    let rowInd ← parseDistinctFinArray rv k rows "row_ind"
    let colInd ← parseDistinctFinArray cv k cols "col_ind"
    .ok (rowInd, colInd)
  | none, none =>
    match root.getField? "p" with
    | some pv =>
      if rows ≤ cols then
        let colInd ← parseDistinctFinArray pv rows cols "p"
        let mut rowInd : Array (Fin rows) := Array.mkEmpty rows
        for idx in [:rows] do
          if hr : idx < rows then
            rowInd := rowInd.push ⟨idx, hr⟩
        .ok (rowInd, colInd)
      else
        .error "field 'p' requires rows <= cols; use 'row_ind' and 'col_ind' for tall matrices"
    | none => .error "missing required matching ('row_ind'/'col_ind' or 'p')"
  | _, _ => .error "both 'row_ind' and 'col_ind' must be provided together"

private def parseBoolMask (v : JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Bool) := do
  let arr ← v.asArray
  if arr.size != expectedLen then
    .error s!"{label}: expected length {expectedLen}, got {arr.size}"
  else
    let mut out : Array Bool := Array.mkEmpty expectedLen
    for idx in [:arr.size] do
      let b ← arr[idx]!.asBool
      out := out.push b
    .ok out

private def parseSubsetMask (v : JsonVal) (bound : Nat) (label : String) :
    Except String (Array Bool) := do
  let arr ← v.asArray
  if arr.size > bound then
    .error s!"{label}: subset length {arr.size} exceeds bound {bound}"
  else
    let mut mask : Array Bool := Array.replicate bound false
    for idx in [:arr.size] do
      let x ← arr[idx]!.asNat
      if x < bound then
        if mask[x]! then
          throw s!"{label}: duplicate index {x}"
        mask := mask.set! x true
      else
        throw s!"{label}: index {x} out of range [0, {bound})"
    .ok mask

private def parseHallMask (root : JsonVal) (rows cols : Nat) : Except String (Array Bool) := do
  let rowSide := rows ≤ cols
  match root.getField? "side" with
  | none | some .null => pure ()
  | some sv =>
    let s ← sv.asString
    let low := s.toLower
    if low == "row" || low == "rows" then
      if !rowSide then
        throw "Hall witness side='rows' requires rows <= cols"
    else if low == "col" || low == "cols" then
      if rowSide then
        throw "Hall witness side='cols' requires rows > cols"
    else
      throw s!"invalid Hall witness side '{s}' (expected 'rows' or 'cols')"
  let (sidePrefix, dim) := if rowSide then ("row", rows) else ("col", cols)
  let maskKey := s!"{sidePrefix}_mask"
  let subsetKey := s!"{sidePrefix}_subset"
  if let some mv := root.getField? maskKey then
    parseBoolMask mv dim maskKey
  else if let some mv := root.getField? "mask" then
    parseBoolMask mv dim "mask"
  else if let some sv := root.getField? subsetKey then
    parseSubsetMask sv dim subsetKey
  else if let some sv := root.getField? "subset" then
    parseSubsetMask sv dim "subset"
  else
    .error s!"missing Hall witness ('subset', '{subsetKey}', '{maskKey}', or 'mask')"

private theorem flatIdx_lt {rows cols : Nat} (i : Fin rows) (j : Fin cols) :
    i.val * cols + j.val < rows * cols := by
  have hi : i.val + 1 ≤ rows := i.isLt
  have hmul : (i.val + 1) * cols ≤ rows * cols := Nat.mul_le_mul_right cols hi
  rw [Nat.add_mul, Nat.one_mul] at hmul
  have hj : j.val < cols := j.isLt
  omega

private def buildMatchingMap {a b : Nat} (keys : Array (Fin a)) (vals : Array (Fin b))
    (hLen : keys.size = a) (hVals : vals.size = a) (hb : 0 < b) :
    Except String { arr : Array (Fin b) // arr.size = a } := do
  let init : Fin b := ⟨0, hb⟩
  let mut out : Array (Fin b) := Array.replicate a init
  for idx in [:a] do
    if h : idx < a then
      let k := keys[idx]'(by omega)
      let v := vals[idx]'(by omega)
      out := out.set! k.val v
  if hOut : out.size = a then
    .ok ⟨out, hOut⟩
  else
    .error "internal error: matching map length mismatch"

private def buildMatchingFn {a b : Nat} (keys : Array (Fin a)) (vals : Array (Fin b))
    (hle : a ≤ b) (keyLabel valLabel : String) : Except String (Fin a → Fin b) := do
  if ha0 : a = 0 then
    return fun i => Fin.elim0 (ha0 ▸ i)
  else
    have hbPos : 0 < b := by omega
    if hK : keys.size = a then
      if hV : vals.size = a then
        let ⟨pArr, hpLen⟩ ← buildMatchingMap keys vals hK hV hbPos
        return fun i => pArr[i.val]'(hpLen ▸ i.isLt)
      else throw s!"internal error: {valLabel} length mismatch"
    else throw s!"internal error: {keyLabel} length mismatch"

private def toFlatMatrix {α : Type} (flat : Array α) (rows cols : Nat) :
    Except String (Fin rows → Fin cols → α) :=
  if h : flat.size = rows * cols then
    .ok fun i j => flat[i.val * cols + j.val]'(h ▸ flatIdx_lt i j)
  else
    .error "internal error: cost matrix length mismatch"

private def resolveDualArr (raw? : Option (Array Rat)) (canDefault : Bool)
    (mode : Mode) (n : Nat) (field : String) : Except String (Array Rat) :=
  match raw? with
  | some arr => .ok arr
  | none =>
    if !canDefault then .error s!"missing required JSON field '{field}'"
    else
      match mode with
      | .exact => .error s!"missing required JSON field '{field}'"
      | .safe => .ok (Array.replicate n 0)
      | .hall => .ok #[]

private def verifySafeDeclaredObj (root : JsonVal) (negate : Bool)
    (primalNorm lbNorm tol : Rat) : Except String Unit := do
  match root.getField? "objective" with
  | none | some .null => .ok ()
  | some ov =>
    let obj ← asRatOrBits ov
    let objNorm := if negate then -obj else obj
    if objNorm < primalNorm - tol || objNorm > lbNorm + tol then
      .error s!"objective mismatch in safe mode: normalized objective {objNorm} outside [{primalNorm - tol}, {lbNorm + tol}] (eliminates 2*tol drift)"
    else
      .ok ()

private def verifyExactDeclaredObj (root : JsonVal) (primalOrig : Rat) : Except String Unit := do
  match root.getField? "objective" with
  | none | some .null => .ok ()
  | some ov =>
    let obj ← asRatOrBits ov
    if primalOrig != obj then
      .error s!"objective mismatch: primal sum is {primalOrig}, declared objective is {obj}"
    else
      .ok ()

/-- Verifies an LSAP optimality or Hall infeasibility certificate directly via `GoblinFormal.Track1`. -/
private def verifyCertCore (root : JsonVal) (args : ParsedArgs) : Except String VerifyResult := do
  let (rows, cols) ← parseDims root
  let negate ← parseSense root
  let mode ← parseMode root args.modeOverride
  let costBits ← parseEncodingFlag root "cost_encoding"
  let dualBits ← parseEncodingFlag root "dual_encoding"

  let tolRaw ←
    match args.tolOverride with
    | some t => .ok t
    | none =>
      match root.getField? "tol" with
      | some .null | none => .ok 0
      | some tv => asRatOrBits tv
  if tolRaw < 0 then
    .error "tolerance must be non-negative"
  if mode == .exact && tolRaw != 0 then
    .error s!"non-zero tolerance {tolRaw} is not allowed in exact mode"
  let tol : Rat := if mode == .safe then tolRaw else 0

  -- Validate cost matrix against declared dimensions before allocating any index or dual buffers.
  let costVal ← root.getField "cost"
  let flatOrig ← parseFlatCostMatrix costVal rows cols negate costBits

  -- Handle Hall infeasibility mode cleanly before dual/primal checks.
  if mode == .hall then
    if rows == 0 || cols == 0 then
      throw "INVALID: empty matrix cannot be Hall-infeasible"
    let maskArr ← parseHallMask root rows cols
    let C ← toFlatMatrix flatOrig rows cols
    if rows ≤ cols then
      let rowMask ← toFinFn maskArr rows "internal error: row mask length mismatch"
      if checkHallInfeasible rows cols C rowMask then
        return { mode := .hall, tol := 0, bound := none }
      throw "INVALID: Hall infeasibility witness failed (|N_finite(S)| >= |S|)"
    else
      let colMask ← toFinFn maskArr cols "internal error: column mask length mismatch"
      if checkHallInfeasibleTranspose rows cols C colMask then
        return { mode := .hall, tol := 0, bound := none }
      throw "INVALID: Hall infeasibility witness failed on tall matrix (|N_finite(S)| >= |S|)"

  -- Validate dual arrays against rows/cols BEFORE parsing primal pairs so huge dims fail fast without OOM.
  let uRaw? ← match optNonNullField? root "u" with
    | none => .ok none
    | some uv => some <$> parseSizedRatArray uv rows dualBits "u"
  let vRaw? ← match optNonNullField? root "v" with
    | none => .ok none
    | some vv => some <$> parseSizedRatArray vv cols dualBits "v"

  let transposed := rows > cols
  let uArr ← resolveDualArr uRaw? (!transposed) mode rows "u"
  let vArr ← resolveDualArr vRaw? transposed mode cols "v"

  let (rowInd, colInd) ← parsePrimalPairs root rows cols
  let flatNorm : Array (Option Rat) :=
    if negate then flatOrig.map (·.map (- ·)) else flatOrig

  let Cnorm ← toFlatMatrix flatNorm rows cols
  let u ← toFinFn uArr rows "internal error: u array length mismatch"
  let v ← toFinFn vArr cols "internal error: v array length mismatch"

  if hle : rows ≤ cols then
    let p ← buildMatchingFn rowInd colInd hle "row_ind" "col_ind"
    let primalNorm : Rat := optCostSum rows cols Cnorm p
    let primalOrig : Rat := if negate then -primalNorm else primalNorm
    match mode with
    | .exact =>
      if checkLSAPForbidden rows cols Cnorm p u v then
        verifyExactDeclaredObj root primalOrig
        .ok { mode := .exact, tol := 0, bound := some primalOrig }
      else if !checkInjective rows cols p then
        .error "INVALID: primal assignment is not injective"
      else if !checkFiniteMatching rows cols Cnorm p then
        .error "INVALID: primal assignment uses a forbidden edge"
      else if !checkDualFeasibleOpt rows cols Cnorm u v then
        .error "INVALID: dual feasibility u[i] + v[j] <= C[i,j] violated"
      else if !checkRectDual rows cols p v then
        .error "INVALID: rectangular dual invariant violated (v[j] <= 0 and unassigned v[j] == 0)"
      else if !checkCompSlackOpt rows cols Cnorm p u v then
        .error "INVALID: complementary slackness u[i] + v[p(i)] == C[i,p(i)] violated"
      else
        .error "INVALID: exact LSAP certificate check failed"
    | .safe =>
      if checkLSAPForbiddenSafe rows cols Cnorm p v tol then
        match safeLowerBoundOpt rows cols Cnorm v with
        | none => .error "INVALID: row has no finite edges in safe mode"
        | some lbNorm =>
          verifySafeDeclaredObj root negate primalNorm lbNorm tol
          let certBnd := if negate then -lbNorm else lbNorm
          .ok { mode := .safe, tol := tol, bound := some certBnd }
      else if !checkInjective rows cols p then
        .error "INVALID: primal assignment is not injective"
      else if !checkFiniteMatching rows cols Cnorm p then
        .error "INVALID: primal assignment uses a forbidden edge"
      else if rows < cols && !allFin cols (fun j => decide (v j ≤ 0)) then
        .error "INVALID: rectangular dual invariant v[j] <= 0 violated in safe mode"
      else
        match safeLowerBoundOpt rows cols Cnorm v with
        | none => .error "INVALID: row has no finite edges in safe mode"
        | some lbNorm =>
          .error s!"INVALID: safe duality gap {primalNorm - lbNorm} exceeds tolerance {tol}"
    | .hall => .error "internal error: unreachable hall branch"
  else
    -- Tall matrix (rows > cols): invoke proved transpose checkers directly on original (rows, cols).
    have hge : cols ≤ rows := by omega
    let p ← buildMatchingFn colInd rowInd hge "col_ind" "row_ind"
    let primalNorm : Rat := finSum cols (fun j => (Cnorm (p j) j).getD 0)
    let primalOrig : Rat := if negate then -primalNorm else primalNorm
    match mode with
    | .exact =>
      if checkLSAPForbiddenTranspose rows cols Cnorm p u v then
        verifyExactDeclaredObj root primalOrig
        .ok { mode := .exact, tol := 0, bound := some primalOrig }
      else if !checkInjective cols rows p then
        .error "INVALID: primal assignment is not injective"
      else if !checkFiniteMatching cols rows (fun j i => Cnorm i j) p then
        .error "INVALID: primal assignment uses a forbidden edge"
      else if !checkDualFeasibleOpt cols rows (fun j i => Cnorm i j) v u then
        .error "INVALID: dual feasibility u[i] + v[j] <= C[i,j] violated"
      else if !checkRectDual cols rows p u then
        .error "INVALID: rectangular dual invariant violated (u[i] <= 0 and unassigned u[i] == 0)"
      else if !checkCompSlackOpt cols rows (fun j i => Cnorm i j) p v u then
        .error "INVALID: complementary slackness u[p(j)] + v[j] == C[p(j),j] violated"
      else
        .error "INVALID: exact tall LSAP certificate check failed"
    | .safe =>
      if checkLSAPForbiddenTransposeSafe rows cols Cnorm p u tol then
        match safeLowerBoundOptTranspose rows cols Cnorm u with
        | none => .error "INVALID: column has no finite edges in safe mode"
        | some lbNorm =>
          verifySafeDeclaredObj root negate primalNorm lbNorm tol
          let certBnd := if negate then -lbNorm else lbNorm
          .ok { mode := .safe, tol := tol, bound := some certBnd }
      else if !checkInjective cols rows p then
        .error "INVALID: primal assignment is not injective"
      else if !checkFiniteMatching cols rows (fun j i => Cnorm i j) p then
        .error "INVALID: primal assignment uses a forbidden edge"
      else if cols < rows && !allFin rows (fun i => decide (u i ≤ 0)) then
        .error "INVALID: rectangular dual invariant u[i] <= 0 violated in safe mode"
      else
        match safeLowerBoundOptTranspose rows cols Cnorm u with
        | none => .error "INVALID: column has no finite edges in safe mode"
        | some lbNorm =>
          .error s!"INVALID: safe duality gap {primalNorm - lbNorm} exceeds tolerance {tol}"
    | .hall => .error "internal error: unreachable hall branch"

private def formatCliStatus (res : VerifyResult) : String :=
  match res.mode with
  | .hall => "VERIFIED mode=hall"
  | .exact => "VERIFIED mode=exact tol=0/1"
  | .safe => s!"VERIFIED mode=safe tol={res.tol.num}/{res.tol.den}"

private def verifyJsonlObj (root : JsonVal) (args : ParsedArgs) : Except String String := do
  let pb ← effectivePrintBound root args.printBound
  let res ← verifyCertCore root args
  let base := "{\"ok\":true,\"detail\":\"VERIFIED\",\"mode\":" ++ jsonQuote res.mode.toStr
  match res.mode with
  | .hall =>
    .ok (base ++ "}")
  | .exact | .safe =>
    let withTol := base ++ ",\"tol\":" ++ ratObjJson res.tol
    let includeBound := res.mode == .safe || pb
    match res.bound with
    | some bnd =>
      if includeBound then .ok (withTol ++ ",\"bound\":" ++ ratObjJson bnd ++ "}")
      else .ok (withTol ++ "}")
    | none =>
      .ok (withTol ++ "}")

private def batchFmt : BatchFormat := {
  readErr := fun path e => s!"error reading '{path}': {e}"
  parseErr := fun path msg => s!"error parsing batch JSON '{path}': {msg}"
  emptyErr := fun path => s!"error: empty batch JSON array in '{path}'"
}

private def runBatch (path : String) (args : ParsedArgs) : IO UInt32 :=
  runBatchWith batchFmt path (fun item => verifyCertCore item args *> .ok ())

end Apps.LsapCheck

open Apps.LsapCheck in
def main (args : List String) : IO UInt32 := do
  match parseArgs args {} with
  | .error msg =>
    IO.eprintln s!"error: {msg}"
    IO.eprintln usageText
    pure 1
  | .ok parsed =>
    if parsed.showHelp then
      IO.println usageText
      return 0
    if parsed.jsonl then
      if parsed.batchPath.isSome || parsed.filePath.isSome then
        IO.eprintln "error: --jsonl cannot be combined with positional or --batch arguments"
        return 1
      return ← Apps.Common.runJsonlLoop (← IO.getStdin) (← IO.getStdout) fun root =>
        verifyJsonlObj root parsed
    if let some bPath := parsed.batchPath then
      if parsed.filePath.isSome then
        IO.eprintln "error: --batch cannot be combined with positional certificate path"
        return 1
      return ← runBatch bPath parsed
    if parsed.filePath.isNone then
      if parsed.modeOverride.isSome || parsed.tolOverride.isSome || parsed.printBound then
        IO.eprintln "error: missing certificate JSON path"
        IO.eprintln usageText
        return 1
      IO.println usageText
      return 0
    let path := parsed.filePath.getD ""
    let contents ←
      try
        Apps.Common.readInput path
      catch e =>
        IO.eprintln s!"error reading '{path}': {e}"
        return 1
    let jsonVal ←
      match GoblinFormal.parseJsonStrict contents with
      | .ok v => pure v
      | .error msg =>
        IO.eprintln s!"error parsing JSON '{path}': {msg}"
        return 1
    let pb ←
      match Apps.Common.effectivePrintBound jsonVal parsed.printBound with
      | .ok b => pure b
      | .error msg =>
        IO.eprintln msg
        return 1
    match verifyCertCore jsonVal parsed with
    | .ok res =>
      IO.println (formatCliStatus res)
      if pb then
        if let some bnd := res.bound then
          IO.println s!"BOUND {bnd.num}/{bnd.den}"
      pure 0
    | .error msg =>
      IO.eprintln msg
      pure 1
