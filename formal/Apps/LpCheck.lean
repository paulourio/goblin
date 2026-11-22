/-
Apps.LpCheck: standalone CLI verifier for LP primal-dual optimality,
Neumaier-Shcherbina safe dual bounds, Farkas infeasibility certificates,
LP/MILP unboundedness rays (`unbounded`, `milp-unbounded`), and LP presolve
dual postsolve reconstruction (`lp-presolve`).
-/

import GoblinFormal.Track3
import GoblinFormal.Track3B

open GoblinFormal
open GoblinFormal.Track3

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
  match v.getField? "bits" with
  | some bv => do
    let bits ← asUInt64Bits bv
    match float64BitsToRat bits with
    | some r => .ok r
    | none => .error "non-finite float64 bits (Inf/NaN)"
  | none => v.asRat

private def parseSense (s : String) : Except String ConstraintSense :=
  match s with
  | "le" | "<=" => .ok .le
  | "ge" | ">=" => .ok .ge
  | "eq" | "=" | "==" => .ok .eq
  | _ => .error s!"invalid constraint sense '{s}'"

private def parseRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  let mut out := Array.mkEmpty arr.size
  for i in [:arr.size] do
    let r ← asRatOrBits arr[i]!
    out := out.push r
  return out

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

private def parseOptRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array (Option Rat)) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  let mut out := Array.mkEmpty arr.size
  for i in [:arr.size] do
    let v := arr[i]!
    match v with
    | .null => out := out.push none
    | _ =>
      let r ← asRatOrBits v
      out := out.push (some r)
  return out

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

private def parseLPInstance (root : JsonVal) : Except String LPInstance := do
  let numVars ← root.getNat "numVars"
  let numRows ← root.getNat "numRows"
  let objConst ←
    match root.getField? "objConst" with
    | none | some .null => pure (0 : Rat)
    | some v => asRatOrBits v
  let objJson ← root.getArray "obj"
  let objArr ← parseRatArray objJson numVars "obj"
  let lbJson ← root.getArray "lb"
  let lbArr ← parseOptRatArray lbJson numVars "lb"
  let ubJson ← root.getArray "ub"
  let ubArr ← parseOptRatArray ubJson numVars "ub"
  let rowsJson ← root.getArray "rows"
  if rowsJson.size != numRows then
    throw s!"'rows' length {rowsJson.size} != numRows {numRows}"
  let mut senseArr : Array ConstraintSense := Array.mkEmpty numRows
  let mut termsArr : Array (List (SparseTerm numVars)) := Array.mkEmpty numRows
  let mut rhsArr : Array Rat := Array.mkEmpty numRows
  for i in [:numRows] do
    let rowVal := rowsJson[i]!
    let senseStr ← rowVal.getString "sense"
    let s ← parseSense senseStr
    let rv ← rowVal.getField "rhs"
    let rhs ← asRatOrBits rv
    let termsJson ← rowVal.getArray "terms"
    let terms ← parseRowTerms numVars termsJson i
    senseArr := senseArr.push s
    termsArr := termsArr.push terms
    rhsArr := rhsArr.push rhs
  if hObj : objArr.size = numVars then
    if hLb : lbArr.size = numVars then
      if hUb : ubArr.size = numVars then
        if hSense : senseArr.size = numRows then
          if hTerms : termsArr.size = numRows then
            if hRhs : rhsArr.size = numRows then
              return {
                numVars := numVars
                numRows := numRows
                objConst := objConst
                obj := fun j => objArr[j.val]'(by rw [hObj]; exact j.isLt)
                sense := fun i => senseArr[i.val]'(by rw [hSense]; exact i.isLt)
                rows := fun i => termsArr[i.val]'(by rw [hTerms]; exact i.isLt)
                rhs := fun i => rhsArr[i.val]'(by rw [hRhs]; exact i.isLt)
                lb := fun j => lbArr[j.val]'(by rw [hLb]; exact j.isLt)
                ub := fun j => ubArr[j.val]'(by rw [hUb]; exact j.isLt)
              }
            else throw "internal rhs array size mismatch"
          else throw "internal terms array size mismatch"
        else throw "internal sense array size mismatch"
      else throw "internal ub array size mismatch"
    else throw "internal lb array size mismatch"
  else throw "internal obj array size mismatch"

private def getOptionalField (root : JsonVal) (names : List String) : Option JsonVal :=
  names.findSome? fun name =>
    match root.getField? name with
    | some .null => none
    | some v => some v
    | none => none

private def parseRatVecWithBits (root : JsonVal) (ratNames : List String)
    (bitsNames : List String) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  match getOptionalField root ratNames with
  | some v =>
    let arr ← v.asArray
    parseRatArray arr expectedLen label
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
  s == "unbounded" || s == "milp-unbounded" || s == "lp-presolve"

private def normalizeLPMode (s : String) : String :=
  if s == "safe" then "safe-bound" else s

private def parseLPMode (defaultMode : Option String) (root : JsonVal) : Except String String := do
  if let some dm := defaultMode then
    return normalizeLPMode dm
  for key in ["mode", "subcommand", "op"] do
    if let some mv := root.getField? key then
      if mv != .null then
        let s ← mv.asString
        return normalizeLPMode s
  if let some sv := root.getField? "safe" then
    if sv != .null then
      let b ← sv.asBool
      return (if b then "safe-bound" else "optimal")
  if (getOptionalField root ["reduced", "lpReduced", "presolved"]).isSome then
    return "lp-presolve"
  .error "missing required JSON field 'mode'"

private def effectivePrintBound (root : JsonVal) (cliFlag : Bool) : Except String Bool := do
  if cliFlag then
    return true
  match root.getField? "printBound" with
  | none | some .null => return false
  | some pv => pv.asBool

/-- Returns `(mode, optionalCertifiedBound)` on success. -/
private def verifyPayloadCore (defaultMode : Option String) (root : JsonVal) :
    Except String (String × Option Rat) := do
  let mode ← parseLPMode defaultMode root
  match mode with
  | "unbounded" =>
    Track3B.verifyLPUnboundedRootJson root
    return ("unbounded", none)
  | "milp-unbounded" =>
    Track3B.verifyMILPUnboundedRootJson root
    return ("milp-unbounded", none)
  | "lp-presolve" =>
    let bnd? ← Track3B.verifyLPPresolveJson root
    return ("lp-presolve", bnd?)
  | "optimal" =>
    let lp ← parseLPInstance root
    let xArr ← parseRatVecWithBits root ["x", "primal"] ["xBits", "primalBits"] lp.numVars "x"
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    if hx : xArr.size = lp.numVars then
      if hy : yArr.size = lp.numRows then
        let xFn : Fin lp.numVars → Rat := fun j => xArr[j.val]'(by rw [hx]; exact j.isLt)
        let yFn : Fin lp.numRows → Rat := fun i => yArr[i.val]'(by rw [hy]; exact i.isLt)
        if checkLPOptimal lp xFn yFn then
          return ("optimal", some (LPObjective lp xFn))
        else if !checkPrimalFeasible lp xFn then
          throw "primal vector violates variable bounds or row constraints"
        else
          match safeDualBound lp yFn with
          | none => throw "dual multipliers yield unbounded reduced cost on infinite variable bound"
          | some lbVal =>
            let obj := LPObjective lp xFn
            throw s!"non-zero duality gap: primal={formatBound obj}, dualBound={formatBound lbVal}"
      else throw "internal y vector size mismatch"
    else throw "internal x vector size mismatch"
  | "safe-bound" =>
    let lp ← parseLPInstance root
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    if hy : yArr.size = lp.numRows then
      let yFn : Fin lp.numRows → Rat := fun i => yArr[i.val]'(by rw [hy]; exact i.isLt)
      match safeDualBound lp yFn with
      | none =>
        throw "dual multipliers yield unbounded reduced cost on infinite variable bound"
      | some lbVal =>
        if let some minBVal := getOptionalField root ["minBound"] then
          let minB ← asRatOrBits minBVal
          if lbVal < minB then
            throw s!"safe dual bound {formatBound lbVal} < minBound {formatBound minB}"
        if let some primalObjVal := getOptionalField root ["primalObj", "upperObj"] then
          let upperObj ← asRatOrBits primalObjVal
          if upperObj < lbVal then
            throw s!"safe dual bound {formatBound lbVal} > primalObj {formatBound upperObj}"
          if let some tolVal := getOptionalField root ["gapTol", "tol"] then
            let tol ← asRatOrBits tolVal
            if upperObj - lbVal > tol then
              throw s!"duality gap {formatBound (upperObj - lbVal)} > gapTol {formatBound tol}"
        return ("safe-bound", some lbVal)
    else throw "internal y vector size mismatch"
  | "farkas" =>
    let lp ← parseLPInstance root
    let wArr ← parseRatVecWithBits root ["w", "ray", "y"] ["wBits", "rayBits", "yBits"] lp.numRows "w"
    if hw : wArr.size = lp.numRows then
      let wFn : Fin lp.numRows → Rat := fun i => wArr[i.val]'(by rw [hw]; exact i.isLt)
      if checkFarkas lp wFn then
        return ("farkas", none)
      else
        throw "Farkas certificate failed (unbounded column product or non-positive strict gap)"
    else throw "internal w vector size mismatch"
  | other =>
    throw s!"unsupported mode '{other}' (expected 'optimal', 'safe-bound', 'farkas', 'unbounded', 'milp-unbounded', or 'lp-presolve')"

private def verifyPayload (defaultMode : Option String) (root : JsonVal) (cliPrintBound : Bool) :
    Except String (List String) := do
  let pb ← effectivePrintBound root cliPrintBound
  let (_, bnd?) ← verifyPayloadCore defaultMode root
  match bnd? with
  | some b =>
    if pb then return ["VERIFIED", s!"BOUND {formatBound b}"]
    else return ["VERIFIED"]
  | none =>
    return ["VERIFIED"]

private def readInput (path : String) : IO String := do
  if path == "-" then
    (← IO.getStdin).readToEnd
  else
    IO.FS.readFile path

private def verifyJsonlObj (defaultMode : Option String) (root : JsonVal) (cliPrintBound : Bool) :
    Except String String := do
  let pb ← effectivePrintBound root cliPrintBound
  let (mode, bnd?) ← verifyPayloadCore defaultMode root
  match bnd? with
  | some b =>
    if mode == "safe-bound" || pb then
      return "{\"ok\":true,\"detail\":\"VERIFIED\",\"bound\":" ++ ratObjJson b ++ "}"
    else
      return "{\"ok\":true,\"detail\":\"VERIFIED\",\"bound\":" ++ ratObjJson b ++ "}"
  | none =>
    return "{\"ok\":true,\"detail\":\"VERIFIED\"}"

private partial def runJsonlLoop (stdin stdout : IO.FS.Stream) (defaultMode : Option String)
    (cliPrintBound : Bool) : IO UInt32 := do
  let line ← stdin.getLine
  if line.isEmpty then
    return 0
  let trimmed := line.trimAscii.toString
  if !trimmed.isEmpty then
    let resp :=
      match parseJson trimmed >>= fun root => verifyJsonlObj defaultMode root cliPrintBound with
      | .ok okJson => okJson
      | .error err => "{\"ok\":false,\"error\":" ++ jsonQuote err ++ "}"
    stdout.putStr resp
    stdout.putStr "\n"
    stdout.flush
  runJsonlLoop stdin stdout defaultMode cliPrintBound

private def runBatch (defaultMode : Option String) (path : String) : IO UInt32 := do
  let contents ←
    try
      readInput path
    catch err =>
      IO.eprintln s!"ERROR: read '{path}': {err}"
      return 1
  match parseJson contents >>= JsonVal.asArray with
  | .error msg =>
    IO.eprintln s!"ERROR: parse batch JSON '{path}': {msg}"
    return 1
  | .ok items =>
    for i in [:items.size] do
      match verifyPayloadCore defaultMode items[i]! with
      | .ok _ => pure ()
      | .error msg =>
        IO.eprintln s!"BATCH FAIL at index {i}: {msg}"
        return 1
    IO.println s!"VERIFIED ({items.size} items)"
    return 0

private def printUsage : IO Unit := do
  IO.println "usage: lp-check [<subcommand>] [--print-bound] [--jsonl | --batch <array.json | ->] [<certificate.json | ->]"
  IO.println "  subcommands: optimal, safe-bound, farkas, unbounded, milp-unbounded, lp-presolve"

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
      return ← runJsonlLoop (← IO.getStdin) (← IO.getStdout) cfg.subcmd cfg.printBound
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
          readInput path
        catch err =>
          IO.eprintln s!"ERROR: read '{path}': {err}"
          return 1
      match parseJson contents with
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
