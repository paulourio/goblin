/-
Apps.LsapCheck: CLI verifier for LSAP optimality and Hall infeasibility certificates
using GoblinFormal.Track1 (checkLSAPForbidden, checkLSAPForbiddenSafe,
checkLSAPForbiddenTranspose, checkLSAPForbiddenTransposeSafe,
checkHallInfeasible, checkHallInfeasibleTranspose).
-/

import GoblinFormal.Track1

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

private def hexDigit (n : Nat) : Char :=
  if n < 10 then Char.ofNat ('0'.toNat + n)
  else Char.ofNat ('a'.toNat + (n - 10))

private def jsonQuote (s : String) : String :=
  let body := s.foldl (fun acc c =>
    if c == '"' then acc ++ "\\\""
    else if c == '\\' then acc ++ "\\\\"
    else if c == '\n' then acc ++ "\\n"
    else if c == '\r' then acc ++ "\\r"
    else if c == '\t' then acc ++ "\\t"
    else if c.toNat < 0x20 then
      let b := c.toNat
      acc.push '\\' |>.push 'u' |>.push '0' |>.push '0'
        |>.push (hexDigit (b / 16)) |>.push (hexDigit (b % 16))
    else acc.push c) ""
  "\"" ++ body ++ "\""

private def ratObjJson (r : Rat) : String :=
  "{\"num\":\"" ++ toString r.num ++ "\",\"den\":\"" ++ toString r.den ++ "\"}"

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
  match v.getField? "bits" with
  | some bv => do
    let bits ← bv.asUInt64
    match float64BitsToRat bits with
    | some r => .ok r
    | none => .error "non-finite float64 bits (Inf/NaN)"
  | none => v.asRat

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

private def parseRatArray (v : JsonVal) (expectedLen : Nat) (bitsEncoding : Bool) (label : String) :
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

private def parseCostEncoding (root : JsonVal) : Except String Bool := do
  match root.getField? "cost_encoding" with
  | none | some .null => .ok false
  | some ev =>
    let s ← ev.asString
    let low := s.toLower
    if low == "rat" || low == "default" then .ok false
    else if low == "bits" then .ok true
    else .error s!"invalid cost_encoding '{s}' (expected 'rat' or 'bits')"

private def parseDualEncoding (root : JsonVal) : Except String Bool := do
  match root.getField? "dual_encoding" with
  | none | some .null => .ok false
  | some ev =>
    let s ← ev.asString
    let low := s.toLower
    if low == "rat" || low == "default" then .ok false
    else if low == "bits" then .ok true
    else .error s!"invalid dual_encoding '{s}' (expected 'rat' or 'bits')"

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

private def effectivePrintBound (root : JsonVal) (cliFlag : Bool) : Except String Bool := do
  if cliFlag then
    return true
  match root.getField? "printBound" with
  | none | some .null => return false
  | some pv => pv.asBool

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
  if rowSide then
    match root.getField? "row_mask" with
    | some mv => parseBoolMask mv rows "row_mask"
    | none =>
      match root.getField? "mask" with
      | some mv => parseBoolMask mv rows "mask"
      | none =>
        match root.getField? "row_subset" with
        | some sv => parseSubsetMask sv rows "row_subset"
        | none =>
          match root.getField? "subset" with
          | some sv => parseSubsetMask sv rows "subset"
          | none => .error "missing Hall witness ('subset', 'row_subset', 'row_mask', or 'mask')"
  else
    match root.getField? "col_mask" with
    | some mv => parseBoolMask mv cols "col_mask"
    | none =>
      match root.getField? "mask" with
      | some mv => parseBoolMask mv cols "mask"
      | none =>
        match root.getField? "col_subset" with
        | some sv => parseSubsetMask sv cols "col_subset"
        | none =>
          match root.getField? "subset" with
          | some sv => parseSubsetMask sv cols "subset"
          | none => .error "missing Hall witness ('subset', 'col_subset', 'col_mask', or 'mask')"

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
  let costBits ← parseCostEncoding root
  let dualBits ← parseDualEncoding root

  let tolRaw ←
    match args.tolOverride with
    | some t => .ok t
    | none =>
      match root.getField? "tol" with
      | some .null | none => .ok 0
      | some tv => asRatOrBits tv
  if tolRaw < 0 then
    .error "tolerance must be non-negative"
  let tol : Rat := if mode == .safe then tolRaw else 0

  -- Validate cost matrix against declared dimensions before allocating any index or dual buffers.
  let costVal ← root.getField "cost"
  let flatOrig ← parseFlatCostMatrix costVal rows cols negate costBits

  -- Handle Hall infeasibility mode cleanly before dual/primal checks.
  if mode == .hall then
    if rows == 0 || cols == 0 then
      throw "INVALID: empty matrix cannot be Hall-infeasible"
    let maskArr ← parseHallMask root rows cols
    if hFlatOrig : flatOrig.size = rows * cols then
      let C : Fin rows → Fin cols → Option Rat := fun i j =>
        flatOrig[i.val * cols + j.val]'(hFlatOrig ▸ flatIdx_lt i j)
      if hle : rows ≤ cols then
        if hMask : maskArr.size = rows then
          let rowMask : Fin rows → Bool := fun i => maskArr[i.val]'(hMask ▸ i.isLt)
          if checkHallInfeasible rows cols C rowMask then
            return { mode := .hall, tol := 0, bound := none }
          else
            throw "INVALID: Hall infeasibility witness failed (|N_finite(S)| >= |S|)"
        else
          throw "internal error: row mask length mismatch"
      else
        if hMask : maskArr.size = cols then
          let colMask : Fin cols → Bool := fun j => maskArr[j.val]'(hMask ▸ j.isLt)
          if checkHallInfeasibleTranspose rows cols C colMask then
            return { mode := .hall, tol := 0, bound := none }
          else
            throw "INVALID: Hall infeasibility witness failed on tall matrix (|N_finite(S)| >= |S|)"
        else
          throw "internal error: column mask length mismatch"
    else
      throw "internal error: cost matrix length mismatch"

  -- Validate dual arrays against rows/cols BEFORE parsing primal pairs so huge dims fail fast without OOM.
  let uRaw? ←
    match root.getField? "u" with
    | none | some .null => .ok none
    | some uv => some <$> parseRatArray uv rows dualBits "u"
  let vRaw? ←
    match root.getField? "v" with
    | none | some .null => .ok none
    | some vv => some <$> parseRatArray vv cols dualBits "v"

  let transposed := rows > cols

  let uArr : Array Rat ←
    if !transposed then
      match uRaw? with
      | some ua => .ok ua
      | none =>
        match mode with
        | .exact => .error "missing required JSON field 'u'"
        | .safe => .ok (Array.replicate rows 0)
        | .hall => .ok #[]
    else
      match uRaw? with
      | some ua => .ok ua
      | none => .error "missing required JSON field 'u'"

  let vArr : Array Rat ←
    if !transposed then
      match vRaw? with
      | some va => .ok va
      | none => .error "missing required JSON field 'v'"
    else
      match vRaw? with
      | some va => .ok va
      | none =>
        match mode with
        | .exact => .error "missing required JSON field 'v'"
        | .safe => .ok (Array.replicate cols 0)
        | .hall => .ok #[]

  let (rowInd, colInd) ← parsePrimalPairs root rows cols

  let flatNorm : Array (Option Rat) :=
    if negate then flatOrig.map (·.map (- ·)) else flatOrig

  if hFlatNorm : flatNorm.size = rows * cols then
    if hU : uArr.size = rows then
      if hV : vArr.size = cols then
        let Cnorm : Fin rows → Fin cols → Option Rat := fun i j =>
          flatNorm[i.val * cols + j.val]'(hFlatNorm ▸ flatIdx_lt i j)
        let u : Fin rows → Rat := fun i => uArr[i.val]'(hU ▸ i.isLt)
        let v : Fin cols → Rat := fun j => vArr[j.val]'(hV ▸ j.isLt)

        if hle : rows ≤ cols then
          let p : Fin rows → Fin cols ←
            if hRows0 : rows = 0 then
              .ok (fun i => Fin.elim0 (hRows0 ▸ i))
            else
              have hColsPos : 0 < cols := by omega
              if hRL : rowInd.size = rows then
                if hCL : colInd.size = rows then
                  let ⟨pArr, hpLen⟩ ← buildMatchingMap rowInd colInd hRL hCL hColsPos
                  .ok (fun i => pArr[i.val]'(hpLen ▸ i.isLt))
                else .error "internal error: col_ind length mismatch"
              else .error "internal error: row_ind length mismatch"
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
          let p : Fin cols → Fin rows ←
            if hCols0 : cols = 0 then
              .ok (fun j => Fin.elim0 (hCols0 ▸ j))
            else
              have hRowsPos : 0 < rows := by omega
              if hCL : colInd.size = cols then
                if hRL : rowInd.size = cols then
                  let ⟨pArr, hpLen⟩ ← buildMatchingMap colInd rowInd hCL hRL hRowsPos
                  .ok (fun j => pArr[j.val]'(hpLen ▸ j.isLt))
                else .error "internal error: row_ind length mismatch"
              else .error "internal error: col_ind length mismatch"
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
      else
        .error "internal error: v array length mismatch"
    else
      .error "internal error: u array length mismatch"
  else
    .error "internal error: cost matrix length mismatch"

private def readInput (path : String) : IO String := do
  if path == "-" then
    (← IO.getStdin).readToEnd
  else
    IO.FS.readFile path

private def formatCliStatus (res : VerifyResult) : String :=
  match res.mode with
  | .hall => "VERIFIED mode=hall"
  | .exact => "VERIFIED mode=exact tol=0/1"
  | .safe => s!"VERIFIED mode=safe tol={res.tol.num}/{res.tol.den}"

private def verifyJsonlObj (root : JsonVal) (args : ParsedArgs) : Except String String := do
  let pb ← effectivePrintBound root args.printBound
  let res ← verifyCertCore root args
  let modePart := "\"mode\":" ++ jsonQuote res.mode.toStr
  match res.mode with
  | .hall =>
    .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "}")
  | .exact =>
    let tolPart := "\"tol\":" ++ ratObjJson res.tol
    match res.bound with
    | some bnd =>
      if pb then
        .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "," ++ tolPart ++ ",\"bound\":" ++ ratObjJson bnd ++ "}")
      else
        .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "," ++ tolPart ++ "}")
    | none =>
      .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "," ++ tolPart ++ "}")
  | .safe =>
    let tolPart := "\"tol\":" ++ ratObjJson res.tol
    match res.bound with
    | some bnd =>
      .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "," ++ tolPart ++ ",\"bound\":" ++ ratObjJson bnd ++ "}")
    | none =>
      .ok ("{\"ok\":true,\"detail\":\"VERIFIED\"," ++ modePart ++ "," ++ tolPart ++ "}")

private partial def runJsonlLoop (stdin stdout : IO.FS.Stream) (args : ParsedArgs) : IO UInt32 := do
  let line ← stdin.getLine
  if line.isEmpty then
    return 0
  let trimmed := line.trimAscii.toString
  if !trimmed.isEmpty then
    let resp :=
      match GoblinFormal.parseJsonStrict trimmed >>= fun root => verifyJsonlObj root args with
      | .ok okJson => okJson
      | .error err => "{\"ok\":false,\"error\":" ++ jsonQuote err ++ "}"
    stdout.putStr resp
    stdout.putStr "\n"
    stdout.flush
  runJsonlLoop stdin stdout args

private def runBatch (path : String) (args : ParsedArgs) : IO UInt32 := do
  let contents ←
    try
      readInput path
    catch e =>
      IO.eprintln s!"error reading '{path}': {e}"
      return 1
  match GoblinFormal.parseJsonStrict contents >>= JsonVal.asArray with
  | .error msg =>
    IO.eprintln s!"error parsing batch JSON '{path}': {msg}"
    return 1
  | .ok items =>
    if items.isEmpty then
      IO.eprintln s!"error: empty batch JSON array in '{path}'"
      return 1
    for i in [:items.size] do
      match verifyCertCore items[i]! args with
      | .ok _ => pure ()
      | .error msg =>
        IO.eprintln s!"BATCH FAIL at index {i}: {msg}"
        return 1
    IO.println s!"VERIFIED ({items.size} items)"
    return 0

end Apps.LsapCheck

def main (args : List String) : IO UInt32 := do
  match Apps.LsapCheck.parseArgs args {} with
  | .error msg =>
    IO.eprintln s!"error: {msg}"
    IO.eprintln Apps.LsapCheck.usageText
    pure 1
  | .ok parsed =>
    if parsed.showHelp then
      IO.println Apps.LsapCheck.usageText
      return 0
    if parsed.jsonl then
      if parsed.batchPath.isSome || parsed.filePath.isSome then
        IO.eprintln "error: --jsonl cannot be combined with positional or --batch arguments"
        return 1
      return ← Apps.LsapCheck.runJsonlLoop (← IO.getStdin) (← IO.getStdout) parsed
    if let some bPath := parsed.batchPath then
      if parsed.filePath.isSome then
        IO.eprintln "error: --batch cannot be combined with positional certificate path"
        return 1
      return ← Apps.LsapCheck.runBatch bPath parsed
    if parsed.filePath.isNone then
      if parsed.modeOverride.isSome || parsed.tolOverride.isSome || parsed.printBound then
        IO.eprintln "error: missing certificate JSON path"
        IO.eprintln Apps.LsapCheck.usageText
        return 1
      IO.println Apps.LsapCheck.usageText
      return 0
    let path := parsed.filePath.getD ""
    let contents ←
      try
        Apps.LsapCheck.readInput path
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
      match Apps.LsapCheck.effectivePrintBound jsonVal parsed.printBound with
      | .ok b => pure b
      | .error msg =>
        IO.eprintln msg
        return 1
    match Apps.LsapCheck.verifyCertCore jsonVal parsed with
    | .ok res =>
      IO.println (Apps.LsapCheck.formatCliStatus res)
      if pb then
        if let some bnd := res.bound then
          IO.println s!"BOUND {bnd.num}/{bnd.den}"
      pure 0
    | .error msg =>
      IO.eprintln msg
      pure 1
