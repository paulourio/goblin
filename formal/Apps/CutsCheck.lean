/-
cuts-check: CLI verifier for Track 2B and Track 2C cutting-plane soundness
(`row-legal-mu`, `mir-use-lower`, `gmi-check`, `cmir-check`, `vub-check`,
`cover-check`, `lotsizing-check`, `agg-cmir-check`, `drop-check`).
-/

import Apps.Common
import GoblinFormal.Track2B
import GoblinFormal.Track2C

open Apps.Common
open GoblinFormal
open GoblinFormal.Track2B

namespace Apps.CutsCheck

private def usageText : String :=
  "usage: cuts-check [<subcommand>] [--jsonl | --batch <array.json | -> | <input.json | ->]\n" ++
  "  subcommands: row-legal-mu, mir-use-lower, gmi-check, cmir-check, vub-check,\n" ++
  "               cover-check, lotsizing-check, agg-cmir-check, drop-check"

private def parseSense : JsonVal → Except String ConstraintSense
  | .str "le" | .str "<=" => .ok .le
  | .str "eq" | .str "=" | .str "==" => .ok .eq
  | .str "ge" | .str ">=" => .ok .ge
  | .num (-1) 0 false => .ok .le
  | .num 0 0 false => .ok .eq
  | .num 1 0 false => .ok .ge
  | _ => .error "expected constraint sense ('le'|'eq'|'ge' or -1|0|1)"

private def optFieldAs {α : Type} (v : JsonVal) (key : String)
    (decode : JsonVal → Except String α) : Except String (Option α) :=
  match optNonNullField? v key with
  | none => pure none
  | some fv => some <$> decode fv

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
    let solutions ← match v.getField? "solutions" with
      | none | some .null => pure #[]
      | some sv => (← sv.asArray).mapM (fun x => x.asArray >>= parseRatArray)
    checkRowAgg rows allowZero aggCutOpt solutions
    pure s!"VERIFIED (row-agg, {rows.size} rows, {solutions.size} witnesses)"
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

private def maxVerifierDim : Nat := 10_000_000

private def checkDimBounds (v : JsonVal) : Except String Unit := do
  for key in ["numVars", "numRows"] do
    if let some fv := optNonNullField? v key then
      let n ← fv.asNat
      if n > maxVerifierDim then
        throw s!"{key}={n} exceeds maxVerifierDim ({maxVerifierDim}) (arrays must have length <= {maxVerifierDim})"

private def handleMirUseLower (requireAssert : Bool) (v : JsonVal) : Except String String := do
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
    let expOpt ← match optNonNullField? c "expected" with
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
      if requireAssert then
        throw "mir-use-lower requires 'expected' assertion field in standalone/--batch mode"
      else
        pure (if got then "true" else "false")

private def handleGmiCheck (v : JsonVal) : Except String String := do
  if (v.getField? "cuts").isSome then
    throw "gmi-check rejects witness-only 'cuts' payloads; provide algebraic tableau ('beta') or structural derivation ('rho' + 'baseRows')"
  if let some betaVal := v.getField? "beta" then
    let beta ← betaVal.asRat
    let aI ← v.getArray "aI" >>= parseRatArray
    let aC ← v.getArray "aC" >>= parseRatArray
    let cIOpt ← optFieldAs v "cI" (fun a => a.asArray >>= parseRatArray)
    let cCOpt ← optFieldAs v "cC" (fun a => a.asArray >>= parseRatArray)
    let solutions ← match optNonNullField? v "solutions" with
      | none => pure #[]
      | some sv =>
        (← sv.asArray).mapM fun item => do
          let xB ← item.getRat "xB"
          let xI ← item.getArray "xI" >>= parseRatArray
          let xC ← item.getArray "xC" >>= parseRatArray
          pure (xB, xI, xC)
    checkTableauGMI aI aC beta cIOpt cCOpt solutions
    pure s!"VERIFIED (tableau-gmi, {solutions.size} witnesses)"
  else if (v.getField? "rho").isSome && (v.getField? "baseRows").isSome then
    Track2C.checkStructuralGMIJson v
  else
    throw "gmi-check requires algebraic tableau ('beta') or structural derivation ('rho' + 'baseRows')"

private def handleCmirCheck (v : JsonVal) : Except String String := do
  if (v.getField? "cuts").isSome then
    throw "cmir-check rejects witness-only 'cuts' payloads; provide algebraic cMIR certificate ('delta')"
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
    let betaHatOpt ← optFieldAs v "betaHat" JsonVal.asRat
    let alphaHatOpt ← optFieldAs v "alphaHat" (fun a => a.asArray >>= parseRatArray)
    let cIOpt ← optFieldAs v "cI" (fun a => a.asArray >>= parseRatArray)
    let cCOpt ← optFieldAs v "cC" (fun a => a.asArray >>= parseRatArray)
    let cutRHSOpt ← optFieldAs v "cutRHS" JsonVal.asRat
    let lpPrimalOpt ← match v.getField? "lpPrimalI", v.getField? "lpPrimalC" with
      | some li, some lc =>
        let lpi ← li.asArray >>= parseRatArray
        let lpc ← lc.asArray >>= parseRatArray
        pure (some (lpi, lpc))
      | none, none => pure none
      | _, _ => throw "provide both lpPrimalI and lpPrimalC or neither"
    let solutions ← match optNonNullField? v "solutions" with
      | none => pure #[]
      | some sv =>
        (← sv.asArray).mapM fun item => do
          let xI ← item.getArray "xI" >>= parseRatArray
          let xC ← item.getArray "xC" >>= parseRatArray
          pure (xI, xC)
    checkCMIRCertificate aI aC lbI ubI lbC ubC useLowerI useLowerC rhs delta
      betaHatOpt alphaHatOpt cIOpt cCOpt cutRHSOpt lpPrimalOpt solutions
    pure s!"VERIFIED (cmir, {solutions.size} witnesses)"
  else
    throw "cmir-check requires algebraic cMIR certificate ('delta')"

private def handleVubCheck (v : JsonVal) : Except String String := do
  if (v.getField? "cuts").isSome then
    throw "vub-check rejects witness-only 'cuts' payloads; provide algebraic VUB certificate ('a') or structural derivation ('baseRows' + 'rowIdx')"
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
    let solutions ← match v.getField? "solutions" with
      | none | some .null => pure #[]
      | some sv =>
        (← sv.asArray).mapM fun item => do
          let x ← item.getArray "x" >>= parseRatArray
          let y ← item.getRat "y"
          pure (x, y)
    checkVUBDisagg a u cap rhsRow targetIdx effUB cutRHS solutions
    pure s!"VERIFIED (vub, {solutions.size} witnesses)"
  else if (v.getField? "baseRows").isSome && (v.getField? "rowIdx").isSome then
    Track2C.checkStructuralVUBJson v
  else
    throw "vub-check requires algebraic VUB certificate ('a') or structural derivation ('baseRows' + 'rowIdx')"

private def isKnownSubcmd (s : String) : Bool :=
  s == "row-legal-mu" || s == "mir-use-lower" || s == "gmi-check" ||
  s == "cmir-check" || s == "vub-check" || s == "cover-check" ||
  s == "lotsizing-check" || s == "agg-cmir-check" || s == "drop-check"

private def dispatchSubcmd (requireAssert : Bool) (subcmd : String) (v : JsonVal) :
    Except String String := do
  checkDimBounds v
  match subcmd with
  | "row-legal-mu" => handleRowLegalMu v
  | "mir-use-lower" => handleMirUseLower requireAssert v
  | "gmi-check" => handleGmiCheck v
  | "cmir-check" => handleCmirCheck v
  | "vub-check" => handleVubCheck v
  | "cover-check" => Track2C.checkKnapsackCoverJson v
  | "lotsizing-check" => Track2C.checkLotSizingCutJson v
  | "agg-cmir-check" => Track2C.checkAggCMIRJson v
  | "drop-check" => Track2C.checkFinalizedCutJson v
  | _ => .error s!"unknown subcommand '{subcmd}'"

private def dispatchItem (requireAssert : Bool) (defaultSubcmd : Option String) (v : JsonVal) :
    Except String String := do
  let sub ← resolveSubcmd defaultSubcmd v "missing required 'op', 'subcommand', or 'mode' field"
  dispatchSubcmd requireAssert sub v

private def runJsonl (defaultSubcmd : Option String) : IO UInt32 := do
  runJsonlLoop (← IO.getStdin) (← IO.getStdout) fun jv => do
    let detail ← dispatchItem false defaultSubcmd jv
    pure ("{\"ok\":true,\"detail\":" ++ jsonQuote detail ++ "}")

private def batchFmt : BatchFormat := {
  readErr := fun _ e => s!"I/O error: {e}"
  parseErr := fun _ err => s!"JSON parse error: {err}"
  emptyErr := fun _ => "FAIL: empty batch array"
}

private def runBatch (defaultSubcmd : Option String) (path : String) : IO UInt32 :=
  runBatchWith batchFmt path (fun item => dispatchItem true defaultSubcmd item *> .ok ())

private def runSingleFile (path : String) (handler : JsonVal → Except String String) : IO UInt32 := do
  try
    let raw ← readInput path
    match parseJsonStrict raw with
    | .error err =>
      IO.eprintln s!"JSON parse error: {err}"
      pure 1
    | .ok jv =>
      match handler jv with
      | .ok msg =>
        IO.println msg
        pure 0
      | .error err =>
        IO.eprintln s!"FAIL: {err}"
        pure 1
  catch e =>
    IO.eprintln s!"I/O error: {e}"
    pure 1

end Apps.CutsCheck

open Apps.CutsCheck in
/-- CLI entry point for `cuts-check`, verifying cutting-plane validity certificates. -/
def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] | ["help"] =>
    IO.println usageText
    pure 0
  | ["--jsonl"] =>
    runJsonl none
  | [subcmd, "--jsonl"] | ["--jsonl", subcmd] =>
    if isKnownSubcmd subcmd then
      runJsonl (some subcmd)
    else
      IO.eprintln s!"FAIL: unknown subcommand '{subcmd}'"
      pure 1
  | ["--batch", path] =>
    runBatch none path
  | [subcmd, "--batch", path] =>
    if isKnownSubcmd subcmd then
      runBatch (some subcmd) path
    else
      IO.eprintln s!"FAIL: unknown subcommand '{subcmd}'"
      pure 1
  | ["--batch"] | [_, "--batch"] =>
    IO.eprintln "FAIL: missing path after --batch"
    pure 1
  | [subcmd, path] =>
    runSingleFile path (dispatchSubcmd true subcmd)
  | [path] =>
    if isKnownSubcmd path then
      runSingleFile "-" (dispatchSubcmd true path)
    else if path.startsWith "-" && path != "-" then
      IO.eprintln usageText
      return 1
    else
      runSingleFile path (dispatchItem true none)
  | _ =>
    IO.eprintln usageText
    pure 1
