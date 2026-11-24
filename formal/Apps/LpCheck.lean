/-
Apps.LpCheck: standalone CLI verifier for LP primal-dual optimality,
Neumaier-Shcherbina safe dual bounds, Farkas infeasibility certificates,
direct bound-conflict infeasibility (`bound-conflict`),
LP/MILP unboundedness rays (`unbounded`, `milp-unbounded`), and LP presolve
dual postsolve reconstruction (`lp-presolve`).
-/

import Apps.Common
import GoblinFormal.Track3
import GoblinFormal.Track3B

open Apps.Common
open GoblinFormal
open GoblinFormal.Track3

namespace Apps.LpCheck

private def parseDecimalNat? (chars : List Char) : Option Nat :=
  if chars.isEmpty then
    none
  else
    chars.foldl (fun acc c =>
      match acc with
      | none => none
      | some n =>
        if '0' ≤ c && c ≤ '9' then
          some (n * 10 + (c.toNat - '0'.toNat))
        else
          none) (some 0)

private def parseHexNat? (chars : List Char) : Option Nat :=
  if chars.isEmpty then
    none
  else
    chars.foldl (fun acc c =>
      match acc with
      | none => none
      | some n =>
        if '0' ≤ c && c ≤ '9' then
          some (n * 16 + (c.toNat - '0'.toNat))
        else if 'a' ≤ c && c ≤ 'f' then
          some (n * 16 + 10 + (c.toNat - 'a'.toNat))
        else if 'A' ≤ c && c ≤ 'F' then
          some (n * 16 + 10 + (c.toNat - 'A'.toNat))
        else
          none) (some 0)

private def asUInt64Bits (v : JsonVal) : Except String UInt64 :=
  match v with
  | .num _ _ _ => v.asUInt64
  | .str s =>
    let parsed : Option Nat :=
      match s.toList with
      | '0' :: 'x' :: rest => parseHexNat? rest
      | '0' :: 'X' :: rest => parseHexNat? rest
      | chars => parseDecimalNat? chars
    match parsed with
    | none => .error s!"invalid UInt64 bitstring '{s}'"
    | some n =>
      if n < UInt64.size then
        .ok (UInt64.ofNat n)
      else
        .error s!"bitstring {s} exceeds UInt64 range"
  | _ => .error "expected UInt64 integer or bitstring"

private def asRatOrBits (v : JsonVal) : Except String Rat :=
  asRatOrBitsWith asUInt64Bits v

private def parseSense (s : String) : Except String ConstraintSense :=
  match s with
  | "le" | "<=" => .ok .le
  | "ge" | ">=" => .ok .ge
  | "eq" | "=" | "==" => .ok .eq
  | _ => .error s!"invalid constraint sense '{s}'"

private def parseSizedRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  arr.mapM asRatOrBits

private def parseBitsAsRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  let mut out := Array.mkEmpty arr.size
  for i in [:arr.size] do
    let bits ← asUInt64Bits arr[i]!
    match float64BitsToRat bits with
    | none => throw s!"'{label}[{i}]' encodes Inf or NaN"
    | some r => out := out.push r
  return out

private def parseSizedOptRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array (Option Rat)) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  arr.mapM fun v =>
    match v with
    | .null => pure none
    | _ => some <$> asRatOrBits v

private def parseRowTerms (numVars : Nat) (arr : Array JsonVal) (rowIdx : Nat) :
    Except String (List (SparseTerm numVars)) := do
  let mut acc : List (SparseTerm numVars) := []
  for k in [:arr.size] do
    let termVal := arr[k]!
    let col ← termVal.getNat "col"
    if h : col < numVars then
      let cv ← termVal.getField "coeff"
      let coeff ← asRatOrBits cv
      acc := { col := ⟨col, h⟩, coeff := coeff } :: acc
    else
      throw s!"row {rowIdx} term {k} column index {col} >= numVars {numVars}"
  return acc.reverse

private def checkMinimizationSense (root : JsonVal) : Except String Unit := do
  match root.getField? "sense" with
  | none | some .null => pure ()
  | some sv =>
    let s ← sv.asString
    let low := s.trimAscii.toString.toLower
    if low != "min" && low != "minimize" then
      throw s!"unsupported objective sense '{s}' (only minimization 'min'/'minimize' is supported)"
  match root.getField? "maximize" with
  | none | some .null => pure ()
  | some mv =>
    let b ← mv.asBool
    if b then
      throw "unsupported 'maximize': true (only minimization is supported)"

private def parseLPInstance (root : JsonVal) : Except String LPInstance := do
  checkMinimizationSense root
  let numVars ← root.getNat "numVars"
  let numRows ← root.getNat "numRows"
  if numVars > Track3B.maxVerifierDim || numRows > Track3B.maxVerifierDim then
    throw s!"LP dimensions (numVars={numVars}, numRows={numRows}) exceed maxVerifierDim ({Track3B.maxVerifierDim})"
  let objConst ← match optNonNullField? root "objConst" with
    | none => pure (0 : Rat)
    | some v => asRatOrBits v
  let objArr ← parseSizedRatArray (← root.getArray "obj") numVars "obj"
  let lbArr ← parseSizedOptRatArray (← root.getArray "lb") numVars "lb"
  let ubArr ← parseSizedOptRatArray (← root.getArray "ub") numVars "ub"
  let rowsJson ← root.getArray "rows"
  if rowsJson.size != numRows then
    throw s!"'rows' length {rowsJson.size} != numRows {numRows}"
  let mut senseArr : Array ConstraintSense := Array.mkEmpty numRows
  let mut termsArr : Array (List (SparseTerm numVars)) := Array.mkEmpty numRows
  let mut rhsArr : Array Rat := Array.mkEmpty numRows
  for i in [:numRows] do
    let rowVal := rowsJson[i]!
    let s ← parseSense (← rowVal.getString "sense")
    let rhs ← asRatOrBits (← rowVal.getField "rhs")
    let terms ← parseRowTerms numVars (← rowVal.getArray "terms") i
    senseArr := senseArr.push s
    termsArr := termsArr.push terms
    rhsArr := rhsArr.push rhs
  let obj ← toFinFn objArr numVars "internal obj array size mismatch"
  let lb ← toFinFn lbArr numVars "internal lb array size mismatch"
  let ub ← toFinFn ubArr numVars "internal ub array size mismatch"
  let sense ← toFinFn senseArr numRows "internal sense array size mismatch"
  let rows ← toFinFn termsArr numRows "internal terms array size mismatch"
  let rhs ← toFinFn rhsArr numRows "internal rhs array size mismatch"
  return { numVars, numRows, objConst, obj, sense, rows, rhs, lb, ub }

private def parseRatVecWithBits (root : JsonVal) (ratNames : List String)
    (bitsNames : List String) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  match getOptionalField root ratNames with
  | some v =>
    let arr ← v.asArray
    parseSizedRatArray arr expectedLen label
  | none =>
    match getOptionalField root bitsNames with
    | some v =>
      let arr ← v.asArray
      parseBitsAsRatArray arr expectedLen s!"{label}Bits"
    | none =>
      throw s!"missing required vector field '{label}'"

private def formatBound (r : Rat) : String :=
  s!"{r.num}/{r.den}"

private def isKnownLPSubcmd (s : String) : Bool :=
  s == "optimal" || s == "safe" || s == "safe-bound" || s == "farkas" ||
  s == "bound-conflict" || s == "unbounded" || s == "milp-unbounded" || s == "lp-presolve"

private def normalizeLPMode (s : String) : String :=
  if s == "safe" then "safe-bound"
  else if s == "boundConflict" || s == "bound_conflict" then "bound-conflict"
  else s

private def parseLPMode (defaultMode : Option String) (root : JsonVal) : Except String String := do
  let mut jsonMode? : Option String := none
  for key in ["mode", "subcommand", "op"] do
    if let some mv := root.getField? key then
      if mv != .null then
        let s ← mv.asString
        let nm := normalizeLPMode s
        match jsonMode? with
        | none => jsonMode? := some nm
        | some prev =>
          if prev != nm then
            throw s!"mode mismatch between JSON fields: '{prev}' vs '{key}'='{nm}'"
  if let some sv := root.getField? "safe" then
    if sv != .null then
      let b ← sv.asBool
      let sm := if b then "safe-bound" else "optimal"
      match jsonMode? with
      | none => jsonMode? := some sm
      | some prev =>
        if prev != sm then
          throw s!"mode mismatch between JSON mode '{prev}' and 'safe'={b} ('{sm}')"
  match defaultMode, jsonMode? with
  | some dm, some jm =>
    let ndm := normalizeLPMode dm
    if ndm != jm then
      throw s!"mode mismatch: CLI subcommand '{ndm}' conflicts with JSON mode '{jm}'"
    return jm
  | some dm, none =>
    return normalizeLPMode dm
  | none, some jm =>
    return jm
  | none, none =>
    if (getOptionalField root ["reduced", "lpReduced", "presolved"]).isSome then
      return "lp-presolve"
    .error "missing required JSON field 'mode'"

private def parseOptionalRatPair (root : JsonVal) (k1 k2 : String) : Except String (Option Rat) := do
  match optNonNullField? root k1, optNonNullField? root k2 with
  | some v1, some v2 =>
    let r1 ← asRatOrBits v1
    let r2 ← asRatOrBits v2
    if r1 != r2 then
      throw s!"conflicting values for '{k1}' ({formatBound r1}) and '{k2}' ({formatBound r2})"
    return some r1
  | some v1, none => some <$> asRatOrBits v1
  | none, some v2 => some <$> asRatOrBits v2
  | none, none => return none

/-- Returns `(mode, optionalCertifiedBound, optionalGapTol)` on success. -/
private def verifyPayloadCore (defaultMode : Option String) (root : JsonVal) :
    Except String (String × Option Rat × Option Rat) := do
  let mode ← parseLPMode defaultMode root
  match mode with
  | "unbounded" =>
    Track3B.verifyLPUnboundedRootJson root
    return ("unbounded", none, none)
  | "milp-unbounded" =>
    Track3B.verifyMILPUnboundedRootJson root
    return ("milp-unbounded", none, none)
  | "lp-presolve" =>
    let bnd? ← Track3B.verifyLPPresolveJson root
    return ("lp-presolve", bnd?, none)
  | "bound-conflict" =>
    let lp ← parseLPInstance root
    let colVal ←
      match getOptionalField root ["col", "conflictCol"] with
      | some cv => pure cv
      | none => throw "missing required field 'col' for bound-conflict mode"
    let col ← colVal.asNat
    if checkBoundConflict lp col then
      return ("bound-conflict", none, none)
    else if col ≥ lp.numVars then
      throw s!"bound-conflict column index {col} >= numVars {lp.numVars}"
    else
      throw s!"bound-conflict rejected: column {col} does not satisfy finite lb > ub"
  | "optimal" =>
    let lp ← parseLPInstance root
    let xArr ← parseRatVecWithBits root ["x", "primal"] ["xBits", "primalBits"] lp.numVars "x"
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    let xFn ← toFinFn xArr lp.numVars "internal x vector size mismatch"
    let yFn ← toFinFn yArr lp.numRows "internal y vector size mismatch"
    if checkLPOptimal lp xFn yFn then
      return ("optimal", some (LPObjective lp xFn), none)
    else if !checkPrimalFeasible lp xFn then
      throw "primal vector violates variable bounds or row constraints"
    else
      match safeDualBound lp yFn with
      | none => throw "dual multipliers yield unbounded reduced cost on infinite variable bound"
      | some lbVal =>
        let obj := LPObjective lp xFn
        throw s!"non-zero duality gap: primal={formatBound obj}, dualBound={formatBound lbVal}"
  | "safe-bound" =>
    let lp ← parseLPInstance root
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    let yFn ← toFinFn yArr lp.numRows "internal y vector size mismatch"
    let lbVal ← match safeDualBound lp yFn with
      | none => throw "dual multipliers yield unbounded reduced cost on infinite variable bound"
      | some v => pure v
    if let some minBVal := getOptionalField root ["minBound"] then
      let minB ← asRatOrBits minBVal
      if lbVal < minB then
        throw s!"safe dual bound {formatBound lbVal} < minBound {formatBound minB}"
    let upperObj? ← parseOptionalRatPair root "primalObj" "upperObj"
    let gapTol? ← parseOptionalRatPair root "gapTol" "tol"
    if let some tol := gapTol? then
      if tol < 0 then
        throw s!"negative gapTol {formatBound tol}"
    let hasX := (getOptionalField root ["x", "primal", "xBits", "primalBits"]).isSome
    if upperObj?.isSome || gapTol?.isSome || hasX then
      if !hasX then
        throw "safe-bound with 'primalObj'/'upperObj' or 'gapTol'/'tol' requires a feasible primal vector 'x'"
      let xArr ← parseRatVecWithBits root ["x", "primal"] ["xBits", "primalBits"] lp.numVars "x"
      let xFn ← toFinFn xArr lp.numVars "internal x vector size mismatch"
      if !checkPrimalFeasible lp xFn then
        throw "primal vector violates variable bounds or row constraints"
      let xObj := LPObjective lp xFn
      if let some upperObj := upperObj? then
        if xObj != upperObj then
          throw s!"primalObj {formatBound upperObj} does not match LPObjective(x) {formatBound xObj}"
        if upperObj < lbVal then
          throw s!"safe dual bound {formatBound lbVal} > primalObj {formatBound upperObj}"
      if let some tol := gapTol? then
        if xObj < lbVal then
          throw s!"safe dual bound {formatBound lbVal} > LPObjective(x) {formatBound xObj}"
        if xObj - lbVal > tol then
          throw s!"duality gap {formatBound (xObj - lbVal)} > gapTol {formatBound tol}"
    return ("safe-bound", some lbVal, gapTol?)
  | "farkas" =>
    let lp ← parseLPInstance root
    let wArr ← parseRatVecWithBits root ["w", "ray", "y"] ["wBits", "rayBits", "yBits"] lp.numRows "w"
    let wFn ← toFinFn wArr lp.numRows "internal w vector size mismatch"
    if checkFarkas lp wFn then
      return ("farkas", none, none)
    else
      throw "Farkas certificate failed (unbounded column product or non-positive strict gap)"
  | other =>
    throw s!"unsupported mode '{other}' (expected 'optimal', 'safe-bound', 'farkas', 'bound-conflict', 'unbounded', 'milp-unbounded', or 'lp-presolve')"

private def verifyPayload (defaultMode : Option String) (root : JsonVal) (cliPrintBound : Bool) :
    Except String (List String) := do
  let pb ← effectivePrintBound root cliPrintBound
  let (mode, bnd?, gapTol?) ← verifyPayloadCore defaultMode root
  let mut lines : List String := ["VERIFIED", s!"MODE {mode}"]
  if let some tol := gapTol? then
    lines := lines ++ [s!"GAPTOL {formatBound tol}"]
  if pb then
    if let some b := bnd? then
      lines := lines ++ [s!"BOUND {formatBound b}"]
  return lines

private def verifyJsonlObj (defaultMode : Option String) (root : JsonVal) (cliPrintBound : Bool) :
    Except String String := do
  let _ ← effectivePrintBound root cliPrintBound
  let (mode, bnd?, gapTol?) ← verifyPayloadCore defaultMode root
  let base := "{\"ok\":true,\"detail\":\"VERIFIED\",\"mode\":" ++ jsonQuote mode
  let withBound :=
    match bnd? with
    | some b => base ++ ",\"bound\":" ++ ratObjJson b
    | none => base
  let withTol :=
    match gapTol? with
    | some tol => withBound ++ ",\"gapTol\":" ++ ratObjJson tol
    | none => withBound
  return withTol ++ "}"

private def batchFmt : BatchFormat := {
  readErr := fun path err => s!"ERROR: read '{path}': {err}"
  parseErr := fun path msg => s!"ERROR: parse batch JSON '{path}': {msg}"
  emptyErr := fun path => s!"ERROR: empty batch JSON '{path}'"
}

private def runBatch (defaultMode : Option String) (path : String) : IO UInt32 :=
  runBatchWith batchFmt path (fun item => verifyPayloadCore defaultMode item *> .ok ())

private def printUsage : IO Unit := do
  IO.println "usage: lp-check [<subcommand>] [--print-bound] [--jsonl | --batch <array.json | ->] [<certificate.json | ->]"
  IO.println "  subcommands: optimal, safe-bound, farkas, bound-conflict, unbounded, milp-unbounded, lp-presolve"

private structure CliArgs where
  subcmd : Option String := none
  printBound : Bool := false
  jsonl : Bool := false
  batchPath : Option String := none
  filePath : Option String := none
  showHelp : Bool := false

private def parseCliArgs : List String → CliArgs → Except String CliArgs
  | [], acc => .ok acc
  | "--help" :: rest, acc => parseCliArgs rest { acc with showHelp := true }
  | "-h" :: rest, acc => parseCliArgs rest { acc with showHelp := true }
  | "--print-bound" :: rest, acc => parseCliArgs rest { acc with printBound := true }
  | "--jsonl" :: rest, acc => parseCliArgs rest { acc with jsonl := true }
  | "--batch" :: p :: rest, acc =>
    if acc.batchPath.isSome then .error "duplicate --batch flag"
    else parseCliArgs rest { acc with batchPath := some p }
  | ["--batch"], _ => .error "missing path after --batch"
  | arg :: rest, acc =>
    if arg.startsWith "-" && arg != "-" then
      .error s!"unexpected flag '{arg}'"
    else if acc.subcmd.isNone && acc.filePath.isNone && isKnownLPSubcmd arg then
      parseCliArgs rest { acc with subcmd := some arg }
    else if acc.filePath.isNone then
      parseCliArgs rest { acc with filePath := some arg }
    else
      .error s!"unexpected argument '{arg}'"

end Apps.LpCheck

open Apps.LpCheck in
def main (args : List String) : IO UInt32 := do
  match parseCliArgs args {} with
  | .error msg =>
    IO.eprintln s!"ERROR: {msg}"
    return 1
  | .ok cfg =>
    if cfg.showHelp then
      printUsage
      return 0
    if cfg.jsonl then
      if cfg.batchPath.isSome || cfg.filePath.isSome then
        IO.eprintln "ERROR: --jsonl cannot be combined with positional or --batch arguments"
        return 1
      return ← Apps.Common.runJsonlLoop (← IO.getStdin) (← IO.getStdout) fun root =>
        verifyJsonlObj cfg.subcmd root cfg.printBound
    if let some bPath := cfg.batchPath then
      if cfg.filePath.isSome then
        IO.eprintln "ERROR: --batch cannot be combined with a positional file argument"
        return 1
      return ← runBatch cfg.subcmd bPath
    let targetPath? : Option String :=
      match cfg.filePath, cfg.subcmd with
      | some p, _ => some p
      | none, some _ => some "-"
      | none, none => none
    match targetPath? with
    | none =>
      printUsage
      return 0
    | some path =>
      let contents ←
        try
          Apps.Common.readInput path
        catch err =>
          IO.eprintln s!"ERROR: read '{path}': {err}"
          return 1
      match parseJsonStrict contents with
      | .error msg =>
        IO.eprintln s!"ERROR: parse JSON '{path}': {msg}"
        return 1
      | .ok root =>
        match verifyPayload cfg.subcmd root cfg.printBound with
        | .error msg =>
          IO.eprintln s!"REJECTED: {msg}"
          return 1
        | .ok lines =>
          for line in lines do
            IO.println line
          return 0
