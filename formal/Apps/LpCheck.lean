/-
Apps.LpCheck: standalone CLI verifier for LP primal-dual optimality,
Neumaier-Shcherbina safe dual bounds, and Farkas infeasibility certificates.
-/

import GoblinFormal.Track3

open GoblinFormal
open GoblinFormal.Track3

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

private def parseSense (s : String) : Except String ConstraintSense :=
  match s with
  | "le" | "<=" => .ok .le
  | "ge" | ">=" => .ok .ge
  | "eq" | "=" => .ok .eq
  | _ => .error s!"invalid constraint sense '{s}'"

private def parseRatArray (arr : Array JsonVal) (expectedLen : Nat) (label : String) :
    Except String (Array Rat) := do
  if arr.size != expectedLen then
    throw s!"'{label}' length {arr.size} != expected {expectedLen}"
  let mut out := Array.mkEmpty arr.size
  for i in [:arr.size] do
    let r ← arr[i]!.asRat
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
      let r ← v.asRat
      out := out.push (some r)
  return out

private def parseRowTerms (numVars : Nat) (arr : Array JsonVal) (rowIdx : Nat) :
    Except String (List (SparseTerm numVars)) := do
  let mut acc : List (SparseTerm numVars) := []
  for k in [:arr.size] do
    let termVal := arr[k]!
    let col ← termVal.getNat "col"
    if h : col < numVars then
      let coeff ← termVal.getRat "coeff"
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
    | some v => v.asRat
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
    let rhs ← rowVal.getRat "rhs"
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

private def verifyPayload (root : JsonVal) (printBound : Bool) : Except String (List String) := do
  let mode ← root.getString "mode"
  let lp ← parseLPInstance root
  match mode with
  | "optimal" =>
    let xArr ← parseRatVecWithBits root ["x", "primal"] ["xBits", "primalBits"] lp.numVars "x"
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    if hx : xArr.size = lp.numVars then
      if hy : yArr.size = lp.numRows then
        let xFn : Fin lp.numVars → Rat := fun j => xArr[j.val]'(by rw [hx]; exact j.isLt)
        let yFn : Fin lp.numRows → Rat := fun i => yArr[i.val]'(by rw [hy]; exact i.isLt)
        if checkLPOptimal lp xFn yFn then
          let obj := LPObjective lp xFn
          let lines := if printBound then ["VERIFIED", s!"BOUND {formatBound obj}"] else ["VERIFIED"]
          return lines
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
    let yArr ← parseRatVecWithBits root ["y", "duals"] ["yBits", "dualBits"] lp.numRows "y"
    if hy : yArr.size = lp.numRows then
      let yFn : Fin lp.numRows → Rat := fun i => yArr[i.val]'(by rw [hy]; exact i.isLt)
      match safeDualBound lp yFn with
      | none =>
        throw "dual multipliers yield unbounded reduced cost on infinite variable bound"
      | some lbVal =>
        if let some minBVal := getOptionalField root ["minBound"] then
          let minB ← minBVal.asRat
          if lbVal < minB then
            throw s!"safe dual bound {formatBound lbVal} < minBound {formatBound minB}"
        if let some primalObjVal := getOptionalField root ["primalObj", "upperObj"] then
          let upperObj ← primalObjVal.asRat
          if upperObj < lbVal then
            throw s!"safe dual bound {formatBound lbVal} > primalObj {formatBound upperObj}"
          if let some tolVal := getOptionalField root ["gapTol"] then
            let tol ← tolVal.asRat
            if upperObj - lbVal > tol then
              throw s!"duality gap {formatBound (upperObj - lbVal)} > gapTol {formatBound tol}"
        let lines := if printBound then ["VERIFIED", s!"BOUND {formatBound lbVal}"] else ["VERIFIED"]
        return lines
    else throw "internal y vector size mismatch"
  | "farkas" =>
    let wArr ← parseRatVecWithBits root ["w", "ray", "y"] ["wBits", "rayBits", "yBits"] lp.numRows "w"
    if hw : wArr.size = lp.numRows then
      let wFn : Fin lp.numRows → Rat := fun i => wArr[i.val]'(by rw [hw]; exact i.isLt)
      if checkFarkas lp wFn then
        return ["VERIFIED"]
      else
        throw "Farkas certificate failed (unbounded column product or non-positive strict gap)"
    else throw "internal w vector size mismatch"
  | other =>
    throw s!"unsupported mode '{other}' (expected 'optimal', 'safe-bound', or 'farkas')"

private def printUsage : IO Unit := do
  IO.println "usage: lp-check [--print-bound] <certificate.json>"

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    printUsage
    return 0
  | _ =>
    let mut printBound := false
    let mut path? : Option String := none
    for arg in args do
      if arg == "--print-bound" then
        printBound := true
      else if arg == "--help" || arg == "-h" then
        printUsage
        return 0
      else if path?.isNone then
        path? := some arg
      else
        IO.eprintln s!"ERROR: unexpected argument '{arg}'"
        return 1
    match path? with
    | none =>
      printUsage
      return 0
    | some path =>
      let contents ←
        try
          IO.FS.readFile path
        catch err =>
          IO.eprintln s!"ERROR: read '{path}': {err}"
          return 1
      match parseJson contents with
      | .error msg =>
        IO.eprintln s!"ERROR: parse JSON '{path}': {msg}"
        return 1
      | .ok root =>
        match verifyPayload root printBound with
        | .error msg =>
          IO.eprintln s!"REJECTED: {msg}"
          return 1
        | .ok lines =>
          for line in lines do
            IO.println line
          return 0
