/-
Apps.RulesCheck: CLI verifier for Track 2A presolve rules (`rules-check`).
Subcommands: `divceil`, `gcd-infeas`, `activity`, `bin-strengthen`, `orbit-check`.
-/

import GoblinFormal.Common.Json
import GoblinFormal.Track2A

open GoblinFormal
open GoblinFormal.Track2A

namespace Apps.RulesCheck

private def usageText : String :=
  "usage: rules-check <subcommand> (reads JSON object or array from stdin)\n" ++
  "subcommands:\n" ++
  "  divceil         check divCeilInt64 / divFloorInt64\n" ++
  "  gcd-infeas      check gcdInt64 and equality-row GCD infeasibility\n" ++
  "  activity        check evalRowActivitySummary, residualQuotient, and presolveCanSkipTerm\n" ++
  "  bin-strengthen  check Savelsbergh binary coefficient strengthening\n" ++
  "  orbit-check     check verifyOrbitSymmetry and orbit objective conditions\n"

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

private def handleDivCeil (v : JsonVal) : Except String String := do
  let a ← parseIntFlex (← v.getField "a")
  let d ← parseIntFlex (← v.getField "d")
  if d ≤ 0 then
    .error s!"divisor d must be positive, got {d}"
  else
    let q := divCeilSpec a d
    let fl := divFloorSpec a d
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
  .ok ("{\"can_skip\":" ++ boolJson canSkip ++
       ",\"raw_strict_exceeds\":" ++ boolJson rawStrict ++ "}")

private def handleActivity (v : JsonVal) : Except String String := do
  let op ← match v.getField? "op" with
    | none => pure "summary"
    | some ov => ov.asString
  match op with
  | "summary" => handleActivitySummary v
  | "skip" => handleActivitySkip v
  | _ => .error s!"unknown activity op '{op}'"

private def handleBinStrengthen (v : JsonVal) : Except String String := do
  let rhs ← v.getRat "rhs"
  let a ← v.getRat "a"
  let maxOther ← v.getRat "max_other"
  let surplus ← v.getRat "surplus"
  let optNewA ← match v.getField? "new_a" with
    | none => pure none
    | some fv => parseOptRat fv
  let optNewRHS ← match v.getField? "new_rhs" with
    | none => pure none
    | some fv => parseOptRat fv
  let (specOpt, applicable, proposedValid, exactIff) :=
    if 0 < a then
      let sp := strengthenBinaryPosSpec rhs a surplus
      let app := sp.isSome && decide (maxOther + a - rhs ≤ surplus)
      let (pv, ex) :=
        match optNewA, optNewRHS with
        | some na, some nr =>
          (app && decide (na == surplus) && decide (rhs - (a - surplus) ≤ nr),
           app && decide (na == surplus) && decide (nr == rhs - (a - surplus)))
        | _, _ => (app, app)
      (sp, app, pv, ex)
    else if a < 0 then
      let sp := strengthenBinaryNegSpec rhs a surplus
      let app := sp.isSome && decide (maxOther - rhs ≤ surplus)
      let (pv, ex) :=
        match optNewA, optNewRHS with
        | some na, some nr =>
          (app && decide (na == -surplus) && decide (rhs ≤ nr),
           app && decide (na == -surplus) && decide (nr == rhs))
        | _, _ => (app, app)
      (sp, app, pv, ex)
    else
      (none, false, false, false)
  let specNewA := specOpt.map Prod.fst
  let specNewRHS := specOpt.map Prod.snd
  .ok ("{\"applicable\":" ++ boolJson applicable ++
       ",\"spec_new_a\":" ++ optRatJson specNewA ++
       ",\"spec_new_rhs\":" ++ optRatJson specNewRHS ++
       ",\"proposed_valid\":" ++ boolJson proposedValid ++
       ",\"exact_iff\":" ++ boolJson exactIff ++ "}")

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
    let exactObj := valid && exactOrbitObjSpec gCount orbits obj
    let monoNonNeg := valid && monotoneOrbitNonNegSpec gCount orbits lb obj
    .ok ("{\"valid\":" ++ boolJson valid ++
         ",\"exact_obj\":" ++ boolJson exactObj ++
         ",\"monotone_non_neg\":" ++ boolJson monoNonNeg ++ "}")

private def runSingleOrBatch (handler : JsonVal → Except String String)
    (input : JsonVal) : Except String String :=
  match input with
  | .arr items => do
    let outs ← items.mapM handler
    .ok ("[" ++ String.intercalate "," outs.toList ++ "]")
  | _ => handler input

private def dispatch (subcmd : String) (payload : String) : Except String String := do
  let v ← parseJson payload
  match subcmd with
  | "divceil" => runSingleOrBatch handleDivCeil v
  | "gcd-infeas" => runSingleOrBatch handleGcdInfeas v
  | "activity" => runSingleOrBatch handleActivity v
  | "bin-strengthen" => runSingleOrBatch handleBinStrengthen v
  | "orbit-check" => runSingleOrBatch handleOrbitCheck v
  | _ => .error s!"unknown subcommand '{subcmd}'"

private def runSubcommand (subcmd : String) : IO UInt32 := do
  let stdin ← IO.getStdin
  let raw ← stdin.readToEnd
  match dispatch subcmd raw with
  | .ok out =>
    IO.println out
    pure 0
  | .error msg =>
    IO.eprintln s!"rules-check {subcmd}: {msg}"
    pure 1

end Apps.RulesCheck

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["-h"] | ["--help"] | ["help"] =>
    IO.print Apps.RulesCheck.usageText
    pure 0
  | [subcmd] =>
    match subcmd with
    | "divceil" | "gcd-infeas" | "activity" | "bin-strengthen" | "orbit-check" =>
      Apps.RulesCheck.runSubcommand subcmd
    | _ =>
      IO.eprintln s!"rules-check: unknown subcommand '{subcmd}'"
      IO.eprint Apps.RulesCheck.usageText
      pure 1
  | _ =>
    IO.eprintln "rules-check: expected at most one subcommand argument"
    IO.eprint Apps.RulesCheck.usageText
    pure 1
