/-
cuts-check: CLI verifier for Track 2B cutting-plane soundness
(`row-legal-mu`, `mir-use-lower`, `gmi-check`, `cmir-check`, `vub-check`).
-/

import GoblinFormal.Track2B

open GoblinFormal
open GoblinFormal.Track2B

private def usageText : String :=
  "usage: cuts-check <subcommand> <input.json | ->\n" ++
  "  subcommands: row-legal-mu, mir-use-lower, gmi-check, cmir-check, vub-check"

private def parseSense : JsonVal → Except String ConstraintSense
  | .str "le" | .str "<=" => .ok .le
  | .str "eq" | .str "=" | .str "==" => .ok .eq
  | .str "ge" | .str ">=" => .ok .ge
  | .num (-1) 0 false => .ok .le
  | .num 0 0 false => .ok .eq
  | .num 1 0 false => .ok .ge
  | _ => .error "expected constraint sense ('le'|'eq'|'ge' or -1|0|1)"

private def parseOptRat : JsonVal → Except String (Option Rat)
  | .null => .ok none
  | v => some <$> v.asRat

private def parseRatArray (arr : Array JsonVal) : Except String (Array Rat) :=
  arr.mapM JsonVal.asRat

private def parseOptRatArray (arr : Array JsonVal) : Except String (Array (Option Rat)) :=
  arr.mapM parseOptRat

private def parseBoolArray (arr : Array JsonVal) : Except String (Array Bool) :=
  arr.mapM JsonVal.asBool

private def parseSparseTerm (v : JsonVal) : Except String SparseTerm := do
  let col ← v.getNat "col"
  let val ← v.getRat "val"
  pure { col, val }

private def parseCutSpec (v : JsonVal) : Except String CutSpec := do
  let termsRaw ← v.getArray "terms"
  let terms ← termsRaw.mapM parseSparseTerm
  let rhs ← v.getRat "rhs"
  pure { terms, rhs }

private def parseRowSpec (v : JsonVal) : Except String RowSpec := do
  let termsRaw ← v.getArray "terms"
  let terms ← termsRaw.mapM parseSparseTerm
  let senseVal ← v.getField "sense"
  let sense ← parseSense senseVal
  let rhs ← v.getRat "rhs"
  pure { terms, sense, rhs }

private def parseMIPSpec (v : JsonVal) : Except String MIPSpec := do
  let numVars ← v.getNat "numVars"
  let isInt ← v.getArray "isInt" >>= parseBoolArray
  let lb ← v.getArray "lb" >>= parseOptRatArray
  let ub ← v.getArray "ub" >>= parseOptRatArray
  let rows ← (← v.getArray "rows").mapM parseRowSpec
  pure { numVars, isInt, lb, ub, rows }

private def parseOptionalMIP (v : JsonVal) : Except String (Option MIPSpec) :=
  match v.getField? "mip" with
  | none | some .null => .ok none
  | some mv => some <$> parseMIPSpec mv

private def handleRowLegalMu (v : JsonVal) : Except String String := do
  if let some casesVal := v.getField? "cases" then
    let cases ← casesVal.asArray
    for i in [:cases.size] do
      let c := cases[i]!
      let sense ← c.getField "sense" >>= parseSense
      let mu ← c.getRat "mu"
      let expected ← c.getBool "expected"
      let got := rowLegalMuSignSpec sense mu
      if got != expected then
        throw s!"case {i}: rowLegalMuSignSpec({repr sense}, {mu}) = {got}, want {expected}"
    pure s!"VERIFIED ({cases.size} cases)"
  else if let some rowsVal := v.getField? "rows" then
    let rowsArr ← rowsVal.asArray
    let rows ← rowsArr.mapM fun rv => do
      let row ← parseRowSpec rv
      let mu ← rv.getRat "mu"
      pure (row, mu)
    let allowZero ← match v.getField? "allowZero" with
      | some av => av.asBool
      | none => pure false
    let aggCutOpt ← match v.getField? "aggCut" with
      | none | some .null => pure none
      | some cv => some <$> parseCutSpec cv
    let solsRaw ← v.getArray "solutions"
    let solutions ← solsRaw.mapM (fun sv => sv.asArray >>= parseRatArray)
    checkRowAgg rows allowZero aggCutOpt solutions
    pure s!"VERIFIED (row-agg, {rows.size} rows, {solutions.size} solutions)"
  else
    let sense ← v.getField "sense" >>= parseSense
    let mu ← v.getRat "mu"
    let got := rowLegalMuSignSpec sense mu
    match v.getField? "expected" with
    | some ev =>
      let expected ← ev.asBool
      if got != expected then
        throw s!"rowLegalMuSignSpec({repr sense}, {mu}) = {got}, want {expected}"
      pure "VERIFIED"
    | none =>
      if !got then
        throw s!"illegal multiplier mu={mu} for sense={repr sense}"
      pure "VERIFIED"

private def handleMirUseLower (v : JsonVal) : Except String String := do
  let checkOne (c : JsonVal) : Except String (Bool × Option Bool) := do
    let x ← c.getRat "x"
    let l ← match c.getField? "l" with
      | none | some .null => pure (0 : Rat)
      | some lv => lv.asRat
    let u ← match c.getField? "u" with
      | none | some .null => pure (0 : Rat)
      | some uv => uv.asRat
    let hasL ← match c.getField? "hasL" with
      | some bv => bv.asBool
      | none =>
        match c.getField? "l" with
        | none | some .null => pure false
        | some _ => pure true
    let hasU ← match c.getField? "hasU" with
      | some bv => bv.asBool
      | none =>
        match c.getField? "u" with
        | none | some .null => pure false
        | some _ => pure true
    let expOpt ← match c.getField? "expected" with
      | some ev => some <$> ev.asBool
      | none => pure none
    pure (mirUseLowerSpec x l u hasL hasU, expOpt)
  if let some casesVal := v.getField? "cases" then
    let cases ← casesVal.asArray
    for i in [:cases.size] do
      let (got, expOpt) ← checkOne cases[i]!
      match expOpt with
      | some expected =>
        if got != expected then
          throw s!"case {i}: mirUseLowerSpec = {got}, want {expected}"
      | none =>
        throw s!"case {i}: missing 'expected' field"
    pure s!"VERIFIED ({cases.size} cases)"
  else
    let (got, expOpt) ← checkOne v
    match expOpt with
    | some expected =>
      if got != expected then
        throw s!"mirUseLowerSpec = {got}, want {expected}"
      pure "VERIFIED"
    | none =>
      pure (if got then "true" else "false")

private def handleGmiCheck (v : JsonVal) : Except String String := do
  let mut checked : Nat := 0
  if let some betaVal := v.getField? "beta" then
    let beta ← betaVal.asRat
    let aI ← v.getArray "aI" >>= parseRatArray
    let aC ← v.getArray "aC" >>= parseRatArray
    let cIOpt ← match v.getField? "cI" with
      | none | some .null => pure none
      | some arr => some <$> (arr.asArray >>= parseRatArray)
    let cCOpt ← match v.getField? "cC" with
      | none | some .null => pure none
      | some arr => some <$> (arr.asArray >>= parseRatArray)
    let solsRaw ← v.getArray "solutions"
    let solutions ← solsRaw.mapM fun sv => do
      let xB ← sv.getRat "xB"
      let xI ← sv.getArray "xI" >>= parseRatArray
      let xC ← sv.getArray "xC" >>= parseRatArray
      pure (xB, xI, xC)
    checkTableauGMI aI aC beta cIOpt cCOpt solutions
    checked := checked + solutions.size
  if let some cutsVal := v.getField? "cuts" then
    let cutsArr ← cutsVal.asArray
    if cutsArr.isEmpty then
      throw "structural gmi-check requires at least one cut"
    let cuts ← cutsArr.mapM parseCutSpec
    let mipOpt ← parseOptionalMIP v
    let solsKey := if (v.getField? "structuralSolutions").isSome then "structuralSolutions" else "solutions"
    let solsRaw ← v.getArray solsKey
    let solutions ← solsRaw.mapM (fun sv => sv.asArray >>= parseRatArray)
    checkStructuralCuts mipOpt cuts solutions
    checked := checked + cuts.size * solutions.size
  if checked == 0 then
    throw "gmi-check requires 'beta' (tableau mode) or 'cuts' (structural mode)"
  pure s!"VERIFIED ({checked} checks)"

private def handleCmirCheck (v : JsonVal) : Except String String := do
  let mut checked : Nat := 0
  if let some deltaVal := v.getField? "delta" then
    let delta ← deltaVal.asRat
    let aI ← v.getArray "aI" >>= parseRatArray
    let aC ← v.getArray "aC" >>= parseRatArray
    let lbI ← v.getArray "lbI" >>= parseOptRatArray
    let ubI ← v.getArray "ubI" >>= parseOptRatArray
    let lbC ← v.getArray "lbC" >>= parseOptRatArray
    let ubC ← v.getArray "ubC" >>= parseOptRatArray
    let useLowerI ← v.getArray "useLowerI" >>= parseBoolArray
    let useLowerC ← v.getArray "useLowerC" >>= parseBoolArray
    let rhs ← v.getRat "rhs"
    let betaHatOpt ← match v.getField? "betaHat" with
      | none | some .null => pure none
      | some bv => some <$> bv.asRat
    let alphaHatOpt ← match v.getField? "alphaHat" with
      | none | some .null => pure none
      | some arr => some <$> (arr.asArray >>= parseRatArray)
    let cIOpt ← match v.getField? "cI" with
      | none | some .null => pure none
      | some arr => some <$> (arr.asArray >>= parseRatArray)
    let cCOpt ← match v.getField? "cC" with
      | none | some .null => pure none
      | some arr => some <$> (arr.asArray >>= parseRatArray)
    let cutRHSOpt ← match v.getField? "cutRHS" with
      | none | some .null => pure none
      | some rv => some <$> rv.asRat
    let lpPrimalOpt ← match v.getField? "lpPrimalI", v.getField? "lpPrimalC" with
      | some li, some lc =>
        let lpi ← li.asArray >>= parseRatArray
        let lpc ← lc.asArray >>= parseRatArray
        pure (some (lpi, lpc))
      | none, none => pure none
      | _, _ => throw "provide both lpPrimalI and lpPrimalC or neither"
    let solsRaw ← v.getArray "solutions"
    let solutions ← solsRaw.mapM fun sv => do
      let xI ← sv.getArray "xI" >>= parseRatArray
      let xC ← sv.getArray "xC" >>= parseRatArray
      pure (xI, xC)
    checkCMIRCertificate aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt solutions
    checked := checked + solutions.size
  if let some cutsVal := v.getField? "cuts" then
    let cutsArr ← cutsVal.asArray
    if cutsArr.isEmpty then
      throw "structural cmir-check requires at least one cut"
    let cuts ← cutsArr.mapM parseCutSpec
    let mipOpt ← parseOptionalMIP v
    let solsKey := if (v.getField? "structuralSolutions").isSome then "structuralSolutions" else "solutions"
    let solsRaw ← v.getArray solsKey
    let solutions ← solsRaw.mapM (fun sv => sv.asArray >>= parseRatArray)
    checkStructuralCuts mipOpt cuts solutions
    checked := checked + cuts.size * solutions.size
  if checked == 0 then
    throw "cmir-check requires 'delta' (certificate mode) or 'cuts' (structural mode)"
  pure s!"VERIFIED ({checked} checks)"

private def handleVubCheck (v : JsonVal) : Except String String := do
  let mut checked : Nat := 0
  if (v.getField? "a").isSome then
    let a ← v.getArray "a" >>= parseRatArray
    let u ← v.getArray "u" >>= parseOptRatArray
    let cap ← match v.getField? "cap" with
      | some cv => cv.asRat
      | none => v.getRat "C"
    let rhsRow ← match v.getField? "rhsRow" with
      | some rv => rv.asRat
      | none => pure 0
    let targetIdx ← v.getNat "targetIdx"
    let effUB ← v.getRat "effUB"
    let cutRHS ← match v.getField? "cutRHS" with
      | some rv => rv.asRat
      | none => pure 0
    let solsRaw ← v.getArray "solutions"
    let solutions ← solsRaw.mapM fun sv => do
      let x ← sv.getArray "x" >>= parseRatArray
      let y ← sv.getRat "y"
      pure (x, y)
    checkVUBDisagg a u cap rhsRow targetIdx effUB cutRHS solutions
    checked := checked + solutions.size
  if let some cutsVal := v.getField? "cuts" then
    let cutsArr ← cutsVal.asArray
    if cutsArr.isEmpty then
      throw "structural vub-check requires at least one cut"
    let cuts ← cutsArr.mapM parseCutSpec
    let mipOpt ← parseOptionalMIP v
    let solsKey := if (v.getField? "structuralSolutions").isSome then "structuralSolutions" else "solutions"
    let solsRaw ← v.getArray solsKey
    let solutions ← solsRaw.mapM (fun sv => sv.asArray >>= parseRatArray)
    checkStructuralCuts mipOpt cuts solutions
    checked := checked + cuts.size * solutions.size
  if checked == 0 then
    throw "vub-check requires 'a' (disaggregation mode) or 'cuts' (structural mode)"
  pure s!"VERIFIED ({checked} checks)"

private def dispatchSubcmd (subcmd : String) (v : JsonVal) : Except String String :=
  match subcmd with
  | "row-legal-mu" => handleRowLegalMu v
  | "mir-use-lower" => handleMirUseLower v
  | "gmi-check" => handleGmiCheck v
  | "cmir-check" => handleCmirCheck v
  | "vub-check" => handleVubCheck v
  | _ => .error s!"unknown subcommand '{subcmd}'"

private def readInput (path : String) : IO String := do
  if path == "-" then
    let stdin ← IO.getStdin
    stdin.readToEnd
  else
    IO.FS.readFile ⟨path⟩

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] | ["help"] =>
    IO.println usageText
    pure 0
  | [subcmd, path] =>
    try
      let raw ← readInput path
      match parseJson raw with
      | .error err =>
        IO.eprintln s!"JSON parse error: {err}"
        pure 1
      | .ok jv =>
        match dispatchSubcmd subcmd jv with
        | .ok msg =>
          IO.println msg
          pure 0
        | .error err =>
          IO.eprintln s!"FAIL: {err}"
          pure 1
    catch e =>
      IO.eprintln s!"I/O error: {e}"
      pure 1
  | [path] =>
    try
      let raw ← readInput path
      match parseJson raw with
      | .error err =>
        IO.eprintln s!"JSON parse error: {err}"
        pure 1
      | .ok jv =>
        match jv.getString "mode" >>= fun m => dispatchSubcmd m jv with
        | .ok msg =>
          IO.println msg
          pure 0
        | .error err =>
          IO.eprintln s!"FAIL: {err}"
          pure 1
    catch e =>
      IO.eprintln s!"I/O error: {e}"
      pure 1
  | _ =>
    IO.eprintln usageText
    pure 1
