import Apps.Common
import GoblinFormal.Common.Json
import GoblinFormal.Track5A
import GoblinFormal.Track5B
import GoblinFormal.Track5D

open Apps.Common
open GoblinFormal
open GoblinFormal.Track5A
open GoblinFormal.Track5B
open GoblinFormal.Track5D

namespace GoblinFormal.Apps.BinCheck

private structure ExpectedRoutine where
  words : Array UInt32
  targets : Nat → Option String
  defaultCfgCheck : Bool
  useTrack5D : Bool := false
  isFramelessLeaf : Bool := false

private def lookupExpectedRoutine (sym : String) : Option ExpectedRoutine :=
  match sym with
  | "presolveSplitmix64" =>
    some { words := splitmix64Words, targets := splitmix64Targets, defaultCfgCheck := false }
  | "canonicalFloatBits" =>
    some { words := canonicalFloatBitsWords, targets := canonicalFloatBitsTargets, defaultCfgCheck := false }
  | "canonicalFloatBitsInlined" =>
    some { words := canonicalFloatBitsInlinedWords, targets := canonicalFloatBitsInlinedTargets, defaultCfgCheck := false }
  | "divCeilInt64" =>
    some { words := divCeilInt64Words, targets := divCeilInt64Targets, defaultCfgCheck := false }
  | "gcdInt64" =>
    some { words := gcdInt64Words, targets := gcdInt64Targets, defaultCfgCheck := false }
  | "hasInfeasibleIntegerEqRow" =>
    some { words := hasInfeasibleIntegerEqRowWords, targets := hasInfeasibleIntegerEqRowTargets, defaultCfgCheck := true }
  | "augmentRowScalar" | "assign.augmentRowScalar" =>
    some { words := augmentRowScalarWords, targets := augmentRowScalarTargets, defaultCfgCheck := true }
  | "InitReducedRow64" | "internal/simd.InitReducedRow64" =>
    some { words := initReducedRow64Words, targets := initReducedRow64Targets, defaultCfgCheck := true }
  | "refillQueueIndirectScalar" | "assign.refillQueueIndirectScalar" =>
    some { words := refillQueueIndirectScalarWords, targets := refillQueueIndirectScalarTargets, defaultCfgCheck := true }
  | "relaxRowIndirectScalar" | "assign.relaxRowIndirectScalar" =>
    some { words := relaxRowIndirectScalarWords, targets := relaxRowIndirectScalarTargets, defaultCfgCheck := true }
  | "twoSumAssign" | "assign.twoSumAssign" =>
    some {
      words := twoSumAssignWords
      targets := twoSumAssignTargets
      defaultCfgCheck := false
      useTrack5D := true
      isFramelessLeaf := true
    }
  | "clampFinite" | "assign.clampFinite" =>
    some {
      words := clampFiniteWords
      targets := clampFiniteTargets
      defaultCfgCheck := false
      useTrack5D := true
      isFramelessLeaf := true
    }
  | "exactReducedCostNoinline" | "assign.exactReducedCostNoinline" =>
    some {
      words := exactReducedCostNoinlineWords
      targets := exactReducedCostNoinlineTargets
      defaultCfgCheck := true
      useTrack5D := true
    }
  | "exactReducedCost" | "assign.exactReducedCost" =>
    some {
      words := exactReducedCostWords
      targets := exactReducedCostTargets
      defaultCfgCheck := false
      useTrack5D := true
      isFramelessLeaf := true
    }
  | "hasPotentialCancellation" | "assign.hasPotentialCancellation" =>
    some {
      words := hasPotentialCancellationWords
      targets := hasPotentialCancellationTargets
      defaultCfgCheck := true
      useTrack5D := true
    }
  | _ => none

private def decodeUInt32Words (arr : Array JsonVal) : Except String (Array UInt32) := do
  if arr.isEmpty then
    throw "words array must be non-empty"
  let mut out := Array.mkEmpty arr.size
  for i in [0:arr.size] do
    let n ← arr[i]!.asNat
    if n >= 4294967296 then
      throw s!"words[{i}] = {n} exceeds 32-bit ARM64 instruction width"
    out := out.push (UInt32.ofNat n)
  return out

private def decodeTargets (obj : JsonVal) : Except String (Array (Nat × String)) := do
  match obj.getField? "targets" with
  | none => return #[]
  | some tv =>
    let arr ← tv.asArray
    let mut out := Array.mkEmpty arr.size
    for elem in arr do
      let idx ← elem.getNat "idx"
      let sym ← elem.getString "symbol"
      if out.any (fun p => p.1 == idx) then
        throw s!"duplicate target entry for idx {idx}"
      out := out.push (idx, sym)
    return out

private def targetsFn (ts : Array (Nat × String)) (idx : Nat) : Option String :=
  (ts.find? (fun p => p.1 == idx)).map Prod.snd

private def checkNoUnknownInsns (prog : Array Arm64Insn) : Except String Unit := do
  for i in [0:prog.size] do
    match prog[i]! with
    | .unknown w => throw s!"unknown ARM64 instruction word at idx {i}: {w}"
    | _ => pure ()

private def verifyCfgSafety (prog : Array Arm64Insn) (ext : Nat → Option String) : Except String Unit := do
  checkNoUnknownInsns prog
  checkZeroGC prog ext
  let _ ← checkStackGuard prog ext
  let _ ← checkPanicBoundsReachability prog ext

private def verifyFramelessLeafSafety (prog : Array Arm64Insn) : Except String Unit := do
  checkNoUnknownInsns prog
  if !checkFramelessLeaf prog then
    throw "frameless leaf safety check failed (external call, non-X30 RET, or invalid instruction)"

private def verifyRoutineMatch (words : Array UInt32) (ext : Nat → Option String)
    (symOpt : Option String) (forceCfg : Bool) (allowDefaultCfg : Bool) : Except String Unit := do
  let sym ← match symOpt with
    | some s => pure s
    | none => throw "missing required JSON field 'symbol'"
  let exp ← match lookupExpectedRoutine sym with
    | some r => pure r
    | none => throw s!"unsupported ARM64 routine symbol '{sym}'"
  if forceCfg || (allowDefaultCfg && exp.defaultCfgCheck) then
    let prog := if exp.useTrack5D then decodeArm64ProgCfg words else decodeArm64Prog words
    if exp.isFramelessLeaf then
      verifyFramelessLeafSafety prog
    else
      verifyCfgSafety prog ext
  if exp.useTrack5D then
    arm64ProgramMatchesD exp.words exp.targets words ext
  else
    arm64ProgramMatches exp.words exp.targets words ext

private def verifyJsonVal (json : JsonVal) : Except String Unit := do
  let wordsArr ← json.getArray "words"
  let words ← decodeUInt32Words wordsArr
  let targets ← decodeTargets json
  let ext := targetsFn targets
  let modeOpt ← match json.getField? "mode" with
    | none => pure none
    | some mv => some <$> mv.asString
  let symOpt ← match json.getField? "symbol" with
    | none => pure none
    | some sv => some <$> sv.asString
  match modeOpt with
  | some "cfg" => verifyCfgSafety (decodeArm64ProgCfg words) ext
  | some "match" => verifyRoutineMatch words ext symOpt false false
  | some "match-and-cfg" => verifyRoutineMatch words ext symOpt true true
  | none => verifyRoutineMatch words ext symOpt false true
  | some other =>
    throw s!"unsupported mode '{other}' (expected 'match', 'cfg', or 'match-and-cfg')"

private def verifyJsonRequest (raw : String) : Except String Unit := do
  let json ← parseJsonStrict raw
  verifyJsonVal json

private def batchFmt : BatchFormat := {
  readErr := fun arg err => s!"REJECTED: failed to read batch input '{arg}': {err}"
  parseErr := fun _ msg => s!"REJECTED: invalid batch JSON: {msg}"
  emptyErr := fun _ => "REJECTED: batch array must be non-empty"
  itemErr := fun i msg => s!"REJECTED: batch item {i}: {msg}"
}

private def runBatch (arg : String) : IO UInt32 :=
  runBatchWith batchFmt arg verifyJsonVal

private def usageText : String :=
  "usage: bin-check [--help] [--batch <requests.json | -> | <request.json | ->]"

end GoblinFormal.Apps.BinCheck

open GoblinFormal.Apps.BinCheck in
/-- CLI entry point for `bin-check`, verifying compiled ARM64/AMD64 ELF binary encodings against Track 5 proofs. -/
def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    IO.println usageText
    return 0
  | ["--batch", arg] =>
    runBatch arg
  | ["--batch"] =>
    IO.eprintln "ERROR: missing path after --batch"
    return 1
  | [arg] =>
    if arg.startsWith "-" && arg != "-" then
      IO.eprintln s!"ERROR: unknown flag '{arg}'"
      return 1
    let raw ←
      try
        Apps.Common.readInput arg
      catch err =>
        IO.eprintln s!"REJECTED: failed to read input: {err}"
        return 1
    match verifyJsonRequest raw with
    | .ok () =>
      IO.println "VERIFIED"
      return 0
    | .error msg =>
      IO.eprintln s!"REJECTED: {msg}"
      return 1
  | other :: _ =>
    IO.eprintln s!"ERROR: unexpected arguments starting with '{other}'"
    return 1
