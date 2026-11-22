/-
Apps.LsapCheck: CLI verifier for LSAP optimality certificates using
GoblinFormal.Track1.checkLSAP and checkLSAPSafe.
-/

import GoblinFormal.Track1

open GoblinFormal
open GoblinFormal.Track1

namespace Apps.LsapCheck

private inductive Mode where
  | exact
  | safe
  deriving BEq

private structure ParsedArgs where
  modeOverride : Option Mode := none
  tolOverride : Option Rat := none
  filePath : Option String := none
  showHelp : Bool := false

private def usageText : String :=
  "Usage: lsap-check [--exact | --safe] [--tol <rat>] <certificate.json>\n" ++
  "  --exact       Verify exact complementary slackness (default unless JSON sets mode='safe')\n" ++
  "  --safe        Verify rigorous dual lower bound via uSafe = min_j (C[i,j] - v[j])\n" ++
  "  --tol <rat>   Override gap tolerance for safe mode (default 0)\n" ++
  "  -h, --help    Show this help message"

private def parseRatString (s : String) : Except String Rat :=
  (JsonVal.str s).asRat

private def parseArgs : List String → ParsedArgs → Except String ParsedArgs
  | [], acc => .ok acc
  | "--help" :: rest, acc => parseArgs rest { acc with showHelp := true }
  | "-h" :: rest, acc => parseArgs rest { acc with showHelp := true }
  | "--exact" :: rest, acc => parseArgs rest { acc with modeOverride := some .exact }
  | "--safe" :: rest, acc => parseArgs rest { acc with modeOverride := some .safe }
  | "--tol" :: raw :: rest, acc => do
    let r ← parseRatString raw
    parseArgs rest { acc with tolOverride := some r }
  | ["--tol"], _ => .error "missing value after --tol"
  | arg :: rest, acc =>
    if arg.startsWith "-" then
      .error s!"unknown flag '{arg}'"
    else
      match acc.filePath with
      | some prev => .error s!"unexpected extra positional argument '{arg}' (already got '{prev}')"
      | none => parseArgs rest { acc with filePath := some arg }

private def asRatOrBits (v : JsonVal) : Except String Rat :=
  match v.getField? "bits" with
  | some bv => do
    let bits ← bv.asUInt64
    match float64BitsToRat bits with
    | some r => .ok r
    | none => .error "non-finite float64 bits (Inf/NaN)"
  | none => v.asRat

private def parseRatArray (v : JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  let arr ← v.asArray
  if arr.size != expectedLen then
    .error s!"{label}: expected length {expectedLen}, got {arr.size}"
  else
    let mut out : Array Rat := Array.mkEmpty expectedLen
    for idx in [:arr.size] do
      let r ← asRatOrBits arr[idx]!
      out := out.push r
    .ok out

private def parseNatArray (v : JsonVal) (expectedLen : Nat) (bound : Nat) (label : String) :
    Except String (Array Nat) := do
  let arr ← v.asArray
  if arr.size != expectedLen then
    .error s!"{label}: expected length {expectedLen}, got {arr.size}"
  else
    let mut out : Array Nat := Array.mkEmpty expectedLen
    let mut seen : Array Bool := Array.replicate bound false
    for idx in [:arr.size] do
      let x ← arr[idx]!.asNat
      if x < bound then
        if seen[x]! then
          throw s!"{label}: duplicate index {x}"
        seen := seen.set! x true
        out := out.push x
      else
        throw s!"{label}: index {x} out of range [0, {bound})"
    .ok out

private def parseCostMatrix (v : JsonVal) (rows cols : Nat) :
    Except String (Array (Array Rat)) := do
  let rowVals ← v.asArray
  if rowVals.size != rows then
    .error s!"cost: expected {rows} rows, got {rowVals.size}"
  else
    let mut mat : Array (Array Rat) := Array.mkEmpty rows
    for r in [:rows] do
      let rowArr ← parseRatArray rowVals[r]! cols s!"cost[{r}]"
      mat := mat.push rowArr
    .ok mat

private def parseDims (root : JsonVal) : Except String (Nat × Nat) := do
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
    | none => .ok .exact
    | some mv =>
      let s ← mv.asString
      let low := s.toLower
      if low == "exact" then
        .ok .exact
      else if low == "safe" then
        .ok .safe
      else
        .error s!"invalid mode '{s}' (expected 'exact' or 'safe')"

private def parsePrimalPairs (root : JsonVal) (rows cols : Nat) :
    Except String (Array Nat × Array Nat) := do
  let k := min rows cols
  match root.getField? "row_ind", root.getField? "col_ind" with
  | some rv, some cv =>
    let rowInd ← parseNatArray rv k rows "row_ind"
    let colInd ← parseNatArray cv k cols "col_ind"
    .ok (rowInd, colInd)
  | none, none =>
    match root.getField? "p" with
    | some pv =>
      if rows ≤ cols then
        let colInd ← parseNatArray pv rows cols "p"
        let mut rowInd : Array Nat := Array.mkEmpty rows
        for i in [:rows] do
          rowInd := rowInd.push i
        .ok (rowInd, colInd)
      else
        .error "field 'p' requires rows <= cols; use 'row_ind' and 'col_ind' for tall matrices"
    | none => .error "missing required matching ('row_ind'/'col_ind' or 'p')"
  | _, _ => .error "both 'row_ind' and 'col_ind' must be provided together"

private def verifyCert (root : JsonVal) (args : ParsedArgs) : Except String Unit := do
  let (rows, cols) ← parseDims root
  let negate ← parseSense root
  let mode ← parseMode root args.modeOverride
  let tol ←
    match args.tolOverride with
    | some t => .ok t
    | none =>
      match root.getField? "tol" with
      | some tv => asRatOrBits tv
      | none => .ok 0
  if tol < 0 then
    .error "tolerance must be non-negative"

  let costVal ← root.getField "cost"
  let origCost ← parseCostMatrix costVal rows cols
  let (rowInd, colInd) ← parsePrimalPairs root rows cols

  let transposed := rows > cols
  let m := min rows cols
  let n := max rows cols

  -- Parse dual arrays according to mode and transposition.
  let uRaw? ←
    match root.getField? "u" with
    | some uv => some <$> parseRatArray uv rows "u"
    | none => .ok none
  let vRaw? ←
    match root.getField? "v" with
    | some vv => some <$> parseRatArray vv cols "v"
    | none => .ok none

  let uArr : Array Rat ←
    if !transposed then
      match uRaw? with
      | some ua => .ok ua
      | none =>
        match mode with
        | .exact => .error "missing required JSON field 'u'"
        | .safe => .ok (Array.replicate m 0)
    else
      match vRaw? with
      | some va => .ok va
      | none =>
        match mode with
        | .exact => .error "missing required JSON field 'v'"
        | .safe => .ok (Array.replicate m 0)

  let vArr : Array Rat ←
    if !transposed then
      match vRaw? with
      | some va => .ok va
      | none => .error "missing required JSON field 'v'"
    else
      match uRaw? with
      | some ua => .ok ua
      | none => .error "missing required JSON field 'u'"

  -- Build normalized matching `pArr : Array (Fin n)` of size `m`.
  let mut pRaw : Array Nat := Array.replicate m 0
  for idx in [:m] do
    let r := rowInd[idx]!
    let c := colInd[idx]!
    if !transposed then
      pRaw := pRaw.set! r c
    else
      pRaw := pRaw.set! c r

  -- Check optional `objective` field against original matched costs.
  let primalOrig : Rat := Id.run do
    let mut s : Rat := 0
    for idx in [:m] do
      let r := rowInd[idx]!
      let c := colInd[idx]!
      s := s + (origCost[r]!)[c]!
    return s

  match root.getField? "objective" with
  | none => pure ()
  | some ov =>
    let obj ← asRatOrBits ov
    match mode with
    | .exact =>
      if primalOrig != obj then
        .error s!"objective mismatch: primal sum is {primalOrig}, declared objective is {obj}"
    | .safe =>
      let diff := (primalOrig - obj).abs
      if diff > tol then
        .error s!"objective mismatch exceeds tol: |{primalOrig} - {obj}| = {diff} > {tol}"

  let C : Fin m → Fin n → Rat := fun i j =>
    let raw :=
      if !transposed then
        (origCost[i.val]!)[j.val]!
      else
        (origCost[j.val]!)[i.val]!
    if negate then -raw else raw

  let p : Fin m → Fin n := fun i =>
    let idx := pRaw[i.val]!
    if h : idx < n then
      ⟨idx, h⟩
    else
      have hmle : m ≤ n := Nat.le_trans (Nat.min_le_left rows cols) (Nat.le_max_left rows cols)
      ⟨0, Nat.lt_of_le_of_lt (Nat.zero_le i.val) (Nat.lt_of_lt_of_le i.isLt hmle)⟩

  let u : Fin m → Rat := fun i => uArr[i.val]!
  let v : Fin n → Rat := fun j => vArr[j.val]!

  match mode with
  | .exact =>
    if checkLSAP m n C p u v then
      .ok ()
    else if !checkInjective m n p then
      .error "INVALID: primal assignment is not injective"
    else if !checkDualFeasible m n C u v then
      .error "INVALID: dual feasibility u[i] + v[j] <= C[i,j] violated"
    else if !checkRectDual m n p v then
      .error "INVALID: rectangular dual invariant violated (v[j] <= 0 and unassigned v[j] == 0)"
    else if !checkCompSlack m n C p u v then
      .error "INVALID: complementary slackness u[i] + v[p(i)] == C[i,p(i)] violated"
    else
      .error "INVALID: exact LSAP certificate check failed"
  | .safe =>
    if checkLSAPSafe m n C p v tol then
      .ok ()
    else if !checkInjective m n p then
      .error "INVALID: primal assignment is not injective"
    else if m < n && !allFin n (fun j => decide (v j ≤ 0)) then
      .error "INVALID: rectangular dual invariant v[j] <= 0 violated in safe mode"
    else
      let primalNorm := finSum m (fun i => C i (p i))
      let lb := safeLowerBound m n C v
      .error s!"INVALID: safe duality gap {primalNorm - lb} exceeds tolerance {tol}"

end Apps.LsapCheck

def main (args : List String) : IO UInt32 := do
  match Apps.LsapCheck.parseArgs args {} with
  | .error msg =>
    IO.eprintln s!"error: {msg}"
    IO.eprintln Apps.LsapCheck.usageText
    pure 1
  | .ok parsed =>
    if parsed.showHelp || parsed.filePath.isNone then
      if parsed.modeOverride.isSome || parsed.tolOverride.isSome then
        if !parsed.showHelp then
          IO.eprintln "error: missing certificate JSON path"
          IO.eprintln Apps.LsapCheck.usageText
          return 1
      IO.println Apps.LsapCheck.usageText
      return 0
    let path := parsed.filePath.getD ""
    let contents ←
      try
        IO.FS.readFile path
      catch e =>
        IO.eprintln s!"error reading '{path}': {e}"
        return 1
    let jsonVal ←
      match GoblinFormal.parseJson contents with
      | .ok v => pure v
      | .error msg =>
        IO.eprintln s!"error parsing JSON '{path}': {msg}"
        return 1
    match Apps.LsapCheck.verifyCert jsonVal parsed with
    | .ok () =>
      IO.println "VERIFIED"
      pure 0
    | .error msg =>
      IO.eprintln msg
      pure 1
