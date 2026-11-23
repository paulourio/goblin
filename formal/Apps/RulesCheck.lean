/-
Apps.RulesCheck: CLI verifier for Track 2A and Track 3B presolve rules (`rules-check`).
Subcommands: `divceil`, `gcd-infeas`, `gcd-div-check`, `activity`, `bin-strengthen`, `orbit-check`.
-/

import GoblinFormal.Common.Json
import GoblinFormal.Track2A
import GoblinFormal.Track3B

open GoblinFormal
open GoblinFormal.Track2A

namespace Apps.RulesCheck

private def usageText : String :=
  "usage: rules-check [<subcommand>] [--jsonl | --batch <array.json | -> | <input.json | ->]\n" ++
  "subcommands:\n" ++
  "  divceil         check divCeilInt64 / divFloorInt64\n" ++
  "  gcd-infeas      check gcdInt64 and equality-row GCD infeasibility\n" ++
  "  gcd-div-check   check Euclidean GCD integer row-coefficient division\n" ++
  "  activity        check evalRowActivitySummary, residualQuotient, and presolveCanSkipTerm\n" ++
  "  bin-strengthen  check Savelsbergh binary coefficient strengthening\n" ++
  "  orbit-check     check verifyOrbitSymmetry and orbit objective conditions\n"

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

private def boolJson (b : Bool) : String :=
  if b then "true" else "false"

private def ratJson (r : Rat) : String :=
  "{\"num\":\"" ++ toString r.num ++ "\",\"den\":\"" ++ toString r.den ++ "\"}"

private def optRatJson : Option Rat → String
  | none => "null"
  | some r => ratJson r

private def parseIntFlex (v : JsonVal) : Except String Int :=
  match v with
  | .num m 0 false => .ok m
  | .str s => do
    let r ← JsonVal.asRat (.str s)
    if r.den == 1 then .ok r.num
    else .error s!"expected integer string, got '{s}'"
  | _ => .error "expected integer"

private def parseOptRat (v : JsonVal) : Except String (Option Rat) :=
  match v with
  | .null => .ok none
  | _ => some <$> v.asRat

private def parseExtRat (v : JsonVal) : Except String ExtRat :=
  match v with
  | .str "negInf" | .str "-inf" | .str "-Inf" => .ok .negInf
  | .str "posInf" | .str "+inf" | .str "+Inf" | .str "inf" | .str "Inf" => .ok .posInf
  | _ => ExtRat.fin <$> v.asRat

private def parseSense (v : JsonVal) : Except String Sense := do
  let s ← v.asString
  match s with
  | "le" | "<=" => .ok .le
  | "ge" | ">=" => .ok .ge
  | "eq" | "==" | "=" => .ok .eq
  | _ => .error s!"invalid constraint sense '{s}'"

private def parseIntArray (arr : Array JsonVal) : Except String (Array Int) :=
  arr.mapM parseIntFlex

private def parseNatArray (arr : Array JsonVal) : Except String (Array Nat) :=
  arr.mapM JsonVal.asNat

private def parseBoolArray (arr : Array JsonVal) : Except String (Array Bool) :=
  arr.mapM JsonVal.asBool

private def parseRatArray (arr : Array JsonVal) : Except String (Array Rat) :=
  arr.mapM JsonVal.asRat

private def parseExtRatArray (arr : Array JsonVal) : Except String (Array ExtRat) :=
  arr.mapM parseExtRat

private def optNonNullField? (v : JsonVal) (key : String) : Option JsonVal :=
  match v.getField? key with
  | none | some .null => none
  | some fv => some fv

private def handleDivCeil (v : JsonVal) : Except String String := do
  let a ← parseIntFlex (← v.getField "a")
  let d ← parseIntFlex (← v.getField "d")
  if d ≤ 0 then
    .error s!"divisor d must be positive, got {d}"
  else
    let q := divCeilSpec a d
    let fl := divFloorSpec a d
    if let some eqv := optNonNullField? v "expect_q" then
      let expQ ← parseIntFlex eqv
      if q != expQ then
        throw s!"divCeil({a}, {d}) = {q}, want {expQ}"
    if let some efv := optNonNullField? v "expect_floor" then
      let expFl ← parseIntFlex efv
      if fl != expFl then
        throw s!"divFloor({a}, {d}) = {fl}, want {expFl}"
    .ok s!"\{\"q\":{q},\"floor\":{fl}}"

private def handleGcdInfeas (v : JsonVal) : Except String String := do
  let coeffsArr ← v.getArray "coeffs"
  let coeffs ← (← parseIntArray coeffsArr).toList |> pure
  let lo ← parseIntFlex (← v.getField "lo")
  let hi ← parseIntFlex (← v.getField "hi")
  let g := listGcd coeffs
  let infeas := hasInfeasibleIntEqSpec coeffs lo hi
  let candG ← match v.getField? "candidate_g" with
    | none | some .null => pure g
    | some cv => parseIntFlex cv
  let dividesAll := candG != 0 && coeffs.all (fun c => Int.tmod c candG == 0)
  let candInfeas := 0 < candG && dividesAll && (hi < divCeilSpec lo candG * candG)
  if let some gv := optNonNullField? v "go_infeasible" then
    let goInf ← gv.asBool
    if goInf && !infeas then
      throw s!"unsound gcd-infeas: solver reported infeasible on feasible range [{lo}, {hi}] (gcd={g})"
  if let some ev := optNonNullField? v "expect_infeasible" then
    let expInf ← ev.asBool
    if expInf != infeas then
      throw s!"gcd-infeas mismatch: got {infeas}, want {expInf}"
  if let some egv := optNonNullField? v "expect_gcd" then
    let expG ← parseIntFlex egv
    if expG != g then
      throw s!"gcd mismatch: got {g}, want {expG}"
  .ok ("{\"gcd\":" ++ toString g ++
       ",\"divides_all\":" ++ boolJson dividesAll ++
       ",\"infeasible\":" ++ boolJson infeas ++
       ",\"candidate_infeasible\":" ++ boolJson candInfeas ++ "}")

private def handleActivitySummary (v : JsonVal) : Except String String := do
  let rhs ← v.getRat "rhs"
  let hasLo ← v.getBool "has_lo"
  let hasHi ← v.getBool "has_hi"
  let termsJson ← v.getArray "terms"
  let terms ← termsJson.mapM fun tv => do
    let coeff ← tv.getRat "a"
    let lb ← parseOptRat (← tv.getField "l")
    let ub ← parseOptRat (← tv.getField "u")
    let isInt ← tv.getBool "is_int"
    pure ({ coeff, lb, ub, isInt } : ActivityTerm)
  let res := evalActivitySummarySpec rhs hasLo hasHi terms
  for j in [:termsJson.size] do
    if hj : j < res.terms.size ∧ j < terms.size then
      let tv := termsJson[j]!
      let tr := res.terms[j]'(hj.1)
      let origTerm := terms[j]'(hj.2)
      if let some gubVal := optNonNullField? tv "go_ub" then
        let goUB ← gubVal.asRat
        match tr.impliedUB with
        | some exactUB =>
          if goUB < exactUB then
            throw s!"term {j}: solver tightened UB {goUB} < exact impliedUB {exactUB}"
        | none =>
          let stricter := match origTerm.ub with
            | none => true
            | some origUB => goUB < origUB
          if stricter then
            throw s!"term {j}: solver tightened UB to {goUB} without a valid impliedUB"
      if let some glbVal := optNonNullField? tv "go_lb" then
        let goLB ← glbVal.asRat
        match tr.impliedLB with
        | some exactLB =>
          if exactLB < goLB then
            throw s!"term {j}: solver tightened LB {goLB} > exact impliedLB {exactLB}"
        | none =>
          let stricter := match origTerm.lb with
            | none => true
            | some origLB => origLB < goLB
          if stricter then
            throw s!"term {j}: solver tightened LB to {goLB} without a valid impliedLB"
  let termObjs := res.terms.map fun t =>
    "{\"quot_upper\":" ++ optRatJson t.quotUpper ++
    ",\"quot_lower\":" ++ optRatJson t.quotLower ++
    ",\"implied_ub\":" ++ optRatJson t.impliedUB ++
    ",\"implied_lb\":" ++ optRatJson t.impliedLB ++ "}"
  let termsStr := "[" ++ String.intercalate "," termObjs.toList ++ "]"
  .ok ("{\"min_inf_count\":" ++ toString res.minInfCount ++
       ",\"max_inf_count\":" ++ toString res.maxInfCount ++
       ",\"min_act\":" ++ optRatJson res.minAct ++
       ",\"max_act\":" ++ optRatJson res.maxAct ++
       ",\"slack\":" ++ optRatJson res.slack ++
       ",\"deficit\":" ++ optRatJson res.deficit ++
       ",\"terms\":" ++ termsStr ++ "}")

private def handleActivitySkip (v : JsonVal) : Except String String := do
  let slackLo ← v.getRat "slack_lo"
  let absA ← v.getRat "abs_a"
  let l ← v.getRat "l"
  let u ← v.getRat "u"
  let targetBound ← v.getRat "target_bound"
  let isInt ← v.getBool "is_int"
  let canSkip := canSkipTermSpec slackLo absA l u targetBound isInt
  let rawStrict := 0 < slackLo && 0 ≤ u - l && absA * (u - l) < slackLo
  if let some gsv := optNonNullField? v "go_skip" then
    let goSkip ← gsv.asBool
    if goSkip && !rawStrict then
      throw "incomplete skip-term: solver skipped term without abs_a*(u-l) < slack_lo"
  if let some esv := optNonNullField? v "expect_can_skip" then
    let expSkip ← esv.asBool
    if expSkip != canSkip then
      throw s!"can_skip mismatch: got {canSkip}, want {expSkip}"
  .ok ("{\"can_skip\":" ++ boolJson canSkip ++
       ",\"raw_strict_exceeds\":" ++ boolJson rawStrict ++ "}")

private def handleActivity (v : JsonVal) : Except String String := do
  let op ← match optNonNullField? v "activity_op" with
    | some av => av.asString
    | none =>
      match optNonNullField? v "op" with
      | none => pure "summary"
      | some ov => ov.asString
  match op with
  | "summary" | "activity-summary" => handleActivitySummary v
  | "skip" | "activity-skip" => handleActivitySkip v
  | "activity" =>
    if (v.getField? "slack_lo").isSome then handleActivitySkip v
    else handleActivitySummary v
  | _ => .error s!"unknown activity op '{op}'"

private def handleBinStrengthen (v : JsonVal) : Except String String := do
  let rhs ← v.getRat "rhs"
  let termsJson ← v.getArray "terms"
  let terms ← termsJson.mapM fun tv => do
    let coeff ← tv.getRat "a"
    let lb ← parseOptRat (← tv.getField "l")
    let ub ← parseOptRat (← tv.getField "u")
    let isInt ← tv.getBool "is_int"
    pure ({ coeff, lb, ub, isInt } : ActivityTerm)
  let targetIdx ← v.getNat "target_idx"
  if hIdx : targetIdx < terms.size then
    let a := terms[targetIdx].coeff
    if let some av := optNonNullField? v "a" then
      let inputA ← av.asRat
      if inputA != a then
        throw s!"input a={inputA} does not match terms[{targetIdx}].a={a}"
    let surplus ← v.getRat "surplus"
    let optNewA ← match v.getField? "new_a" with
      | none => pure none
      | some fv => parseOptRat fv
    let optNewRHS ← match v.getField? "new_rhs" with
      | none => pure none
      | some fv => parseOptRat fv
    let specOpt :=
      if 0 < a then strengthenBinaryPosSpec rhs a surplus
      else if a < 0 then strengthenBinaryNegSpec rhs a surplus
      else none
    let (newA, newRHS) ← match optNewA, optNewRHS with
      | some na, some nr => pure (na, nr)
      | none, none =>
        match specOpt with
        | some (sa, sr) => pure (sa, sr)
        | none => throw s!"binary coefficient strengthening not applicable (a={a}, surplus={surplus})"
      | _, _ => throw "provide both new_a and new_rhs or neither"
    let exactMaxOther ← checkBinStrengthenFromRowSpec rhs terms targetIdx surplus newA newRHS
    if let some mv := optNonNullField? v "max_other" then
      let inputMaxOther ← mv.asRat
      if inputMaxOther < exactMaxOther then
        throw s!"spoofed max_other={inputMaxOther} < exact computed max_other={exactMaxOther}"
    let exactIff :=
      match specOpt with
      | some (sa, sr) => newA == sa && newRHS == sr
      | none => false
    let specNewA := specOpt.map Prod.fst
    let specNewRHS := specOpt.map Prod.snd
    .ok ("{\"applicable\":true" ++
         ",\"exact_max_other\":" ++ ratJson exactMaxOther ++
         ",\"spec_new_a\":" ++ optRatJson specNewA ++
         ",\"spec_new_rhs\":" ++ optRatJson specNewRHS ++
         ",\"proposed_valid\":true" ++
         ",\"exact_iff\":" ++ boolJson exactIff ++ "}")
  else
    throw s!"target_idx {targetIdx} out of bounds ({terms.size})"

private def handleOrbitCheck (v : JsonVal) : Except String String := do
  let numVars ← v.getNat "num_vars"
  let isInt ← parseBoolArray (← v.getArray "is_int")
  let lb ← parseExtRatArray (← v.getArray "lb")
  let ub ← parseExtRatArray (← v.getArray "ub")
  let obj ← parseRatArray (← v.getArray "obj")
  let rowsJson ← v.getArray "rows"
  let rows ← rowsJson.mapM fun rv => do
    let active ← rv.getBool "active"
    let cols ← parseNatArray (← rv.getArray "cols")
    let vals ← parseRatArray (← rv.getArray "vals")
    let sense ← parseSense (← rv.getField "sense")
    let rhs ← rv.getRat "rhs"
    pure ({ active, cols, vals, sense, rhs } : OrbitWorkRow)
  let gCount ← v.getNat "g_count"
  let orbits ← parseNatArray (← v.getArray "orbits")
  let rowTuples ← parseNatArray (← v.getArray "row_tuples")
  let colOrbit ← parseIntArray (← v.getArray "col_orbit")
  let colGroup ← parseIntArray (← v.getArray "col_group")
  let rowState ← parseIntArray (← v.getArray "row_state")
  if obj.size != numVars then
    .error s!"obj length {obj.size} does not match num_vars {numVars}"
  else
    let valid := verifyOrbitSymmetrySpec
      numVars isInt lb ub rows gCount orbits rowTuples colOrbit colGroup rowState
    if !valid then
      throw "orbit symmetry verification failed"
    let exactObj := exactOrbitObjSpec gCount orbits obj
    let monoNonNeg := monotoneOrbitNonNegSpec gCount orbits lb obj
    if let some rmv := optNonNullField? v "require_monotone" then
      let reqMono ← rmv.asBool
      if reqMono && !monoNonNeg then
        throw "monotone orbit non-negativity check failed"
    if let some rev := optNonNullField? v "require_exact_obj" then
      let reqEx ← rev.asBool
      if reqEx && !exactObj then
        throw "exact orbit objective invariance check failed"
    .ok ("{\"valid\":true" ++
         ",\"exact_obj\":" ++ boolJson exactObj ++
         ",\"monotone_non_neg\":" ++ boolJson monoNonNeg ++ "}")

private def isKnownSubcmd (s : String) : Bool :=
  s == "divceil" || s == "gcd-infeas" || s == "gcd-div-check" || s == "activity" ||
  s == "activity-summary" || s == "activity-skip" ||
  s == "bin-strengthen" || s == "orbit-check"

private def dispatchOne (subcmd : String) (v : JsonVal) : Except String String :=
  match subcmd with
  | "divceil" => handleDivCeil v
  | "gcd-infeas" => handleGcdInfeas v
  | "gcd-div-check" => Track3B.handleGcdRowDivJson v
  | "activity" => handleActivity v
  | "activity-summary" | "summary" => handleActivitySummary v
  | "activity-skip" | "skip" => handleActivitySkip v
  | "bin-strengthen" => handleBinStrengthen v
  | "orbit-check" => handleOrbitCheck v
  | _ => .error s!"unknown subcommand '{subcmd}'"

private def resolveItemSubcmd (defaultSubcmd : Option String) (v : JsonVal) : Except String String := do
  match defaultSubcmd with
  | some s => .ok s
  | none =>
    for key in ["subcommand", "op", "mode"] do
      if let some fv := optNonNullField? v key then
        return ← fv.asString
    .error "missing required 'op' or 'subcommand' field"

private def dispatchItem (defaultSubcmd : Option String) (v : JsonVal) : Except String String := do
  let sub ← resolveItemSubcmd defaultSubcmd v
  dispatchOne sub v

private def runSingleOrBatch (handler : JsonVal → Except String String)
    (input : JsonVal) : Except String String :=
  match input with
  | .arr items => do
    if items.isEmpty then
      throw "empty batch array"
    let outs ← items.mapM handler
    .ok ("[" ++ String.intercalate "," outs.toList ++ "]")
  | _ => handler input

private def dispatch (subcmd : String) (payload : String) : Except String String := do
  let v ← parseJsonStrict payload
  match subcmd with
  | "divceil" => runSingleOrBatch handleDivCeil v
  | "gcd-infeas" => runSingleOrBatch handleGcdInfeas v
  | "gcd-div-check" => runSingleOrBatch Track3B.handleGcdRowDivJson v
  | "activity" => runSingleOrBatch handleActivity v
  | "activity-summary" | "summary" => runSingleOrBatch handleActivitySummary v
  | "activity-skip" | "skip" => runSingleOrBatch handleActivitySkip v
  | "bin-strengthen" => runSingleOrBatch handleBinStrengthen v
  | "orbit-check" => runSingleOrBatch handleOrbitCheck v
  | _ => .error s!"unknown subcommand '{subcmd}'"

private def readInput (path : String) : IO String := do
  if path == "-" then
    (← IO.getStdin).readToEnd
  else
    IO.FS.readFile path

private def runSubcommandFrom (subcmd : String) (path : String) : IO UInt32 := do
  let raw ←
    try
      readInput path
    catch e =>
      IO.eprintln s!"rules-check I/O error '{path}': {e}"
      return 1
  match dispatch subcmd raw with
  | .ok out =>
    IO.println out
    pure 0
  | .error msg =>
    IO.eprintln s!"rules-check {subcmd}: {msg}"
    pure 1

private def runSubcommand (subcmd : String) : IO UInt32 :=
  runSubcommandFrom subcmd "-"

private partial def runJsonlLoop (stdin stdout : IO.FS.Stream) (defaultSubcmd : Option String) : IO UInt32 := do
  let line ← stdin.getLine
  if line.isEmpty then
    return 0
  let trimmed := line.trimAscii.toString
  if !trimmed.isEmpty then
    let resp :=
      match parseJsonStrict trimmed >>= dispatchItem defaultSubcmd with
      | .ok resJson => "{\"ok\":true,\"detail\":\"VERIFIED\",\"result\":" ++ resJson ++ "}"
      | .error err => "{\"ok\":false,\"error\":" ++ jsonQuote err ++ "}"
    stdout.putStr resp
    stdout.putStr "\n"
    stdout.flush
  runJsonlLoop stdin stdout defaultSubcmd

private def runBatchFlag (defaultSubcmd : Option String) (path : String) : IO UInt32 := do
  let raw ←
    try
      readInput path
    catch e =>
      IO.eprintln s!"rules-check I/O error '{path}': {e}"
      return 1
  match parseJsonStrict raw >>= JsonVal.asArray with
  | .error msg =>
    IO.eprintln s!"rules-check JSON parse error '{path}': {msg}"
    return 1
  | .ok items =>
    if items.isEmpty then
      IO.eprintln "rules-check: empty batch array"
      return 1
    for i in [:items.size] do
      match dispatchItem defaultSubcmd items[i]! with
      | .ok _ => pure ()
      | .error msg =>
        IO.eprintln s!"BATCH FAIL at index {i}: {msg}"
        return 1
    IO.println s!"VERIFIED ({items.size} items)"
    return 0

end Apps.RulesCheck

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["-h"] | ["--help"] | ["help"] =>
    IO.print Apps.RulesCheck.usageText
    pure 0
  | ["--jsonl"] =>
    Apps.RulesCheck.runJsonlLoop (← IO.getStdin) (← IO.getStdout) none
  | [subcmd, "--jsonl"] | ["--jsonl", subcmd] =>
    if Apps.RulesCheck.isKnownSubcmd subcmd then
      Apps.RulesCheck.runJsonlLoop (← IO.getStdin) (← IO.getStdout) (some subcmd)
    else
      IO.eprintln s!"rules-check: unknown subcommand '{subcmd}'"
      pure 1
  | ["--batch", path] =>
    Apps.RulesCheck.runBatchFlag none path
  | [subcmd, "--batch", path] =>
    if Apps.RulesCheck.isKnownSubcmd subcmd then
      Apps.RulesCheck.runBatchFlag (some subcmd) path
    else
      IO.eprintln s!"rules-check: unknown subcommand '{subcmd}'"
      pure 1
  | ["--batch"] | [_, "--batch"] =>
    IO.eprintln "rules-check: missing path after --batch"
    pure 1
  | [subcmd, path] =>
    if Apps.RulesCheck.isKnownSubcmd subcmd then
      Apps.RulesCheck.runSubcommandFrom subcmd path
    else
      IO.eprintln s!"rules-check: unknown subcommand '{subcmd}'"
      IO.eprint Apps.RulesCheck.usageText
      pure 1
  | [subcmd] =>
    if Apps.RulesCheck.isKnownSubcmd subcmd then
      Apps.RulesCheck.runSubcommand subcmd
    else
      IO.eprintln s!"rules-check: unknown subcommand '{subcmd}'"
      IO.eprint Apps.RulesCheck.usageText
      pure 1
  | _ =>
    IO.eprintln "rules-check: invalid arguments"
    IO.eprint Apps.RulesCheck.usageText
    pure 1
