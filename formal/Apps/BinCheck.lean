import GoblinFormal.Common.Json
import GoblinFormal.Track5A
import GoblinFormal.Track5B

open GoblinFormal
open GoblinFormal.Track5A
open GoblinFormal.Track5B

namespace GoblinFormal.Apps.BinCheck

private structure ExpectedRoutine where
  words : Array UInt32
  targets : Nat → Option String
  defaultCfgCheck : Bool

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
      out := out.push (idx, sym)
    return out

private def targetsFn (ts : Array (Nat × String)) (idx : Nat) : Option String :=
  (ts.find? (fun p => p.1 == idx)).map Prod.snd

private def verifyCfgSafety (prog : Array Arm64Insn) (ext : Nat → Option String) : Except String Unit := do
  for i in [0:prog.size] do
    match prog[i]! with
    | .unknown w => throw s!"unknown ARM64 instruction word at idx {i}: {w}"
    | _ => pure ()
  checkZeroGC prog ext
  let _ ← checkStackGuard prog ext
  let _ ← checkPanicBoundsReachability prog ext

private def verifyJsonRequest (raw : String) : Except String Unit := do
  let json ← parseJson raw
  let wordsVal ← json.getField "words"
  let wordsArr ← wordsVal.asArray
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
  | some "cfg" =>
    verifyCfgSafety (decodeArm64Prog words) ext
  | some "match" =>
    let sym ← match symOpt with
      | some s => pure s
      | none => throw "missing required JSON field 'symbol'"
    let exp ← match lookupExpectedRoutine sym with
      | some r => pure r
      | none => throw s!"unsupported ARM64 routine symbol '{sym}'"
    arm64ProgramMatches exp.words exp.targets words ext
  | some "match-and-cfg" | none =>
    let sym ← match symOpt with
      | some s => pure s
      | none => throw "missing required JSON field 'symbol'"
    let exp ← match lookupExpectedRoutine sym with
      | some r => pure r
      | none => throw s!"unsupported ARM64 routine symbol '{sym}'"
    let runCfg := modeOpt == some "match-and-cfg" || exp.defaultCfgCheck
    if runCfg then
      verifyCfgSafety (decodeArm64Prog words) ext
    arm64ProgramMatches exp.words exp.targets words ext
  | some other =>
    throw s!"unsupported mode '{other}' (expected 'match', 'cfg', or 'match-and-cfg')"

private def usageText : String :=
  "usage: bin-check [--help] [<request.json> | -]"

end GoblinFormal.Apps.BinCheck

open GoblinFormal.Apps.BinCheck in
def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    IO.println usageText
    return 0
  | [arg] =>
    let rawRes : Except IO.Error String ←
      if arg == "-" then
        try
          .ok <$> (← IO.getStdin).readToEnd
        catch e =>
          pure (.error e)
      else
        try
          .ok <$> IO.FS.readFile ⟨arg⟩
        catch e =>
          pure (.error e)
    match rawRes with
    | .error err =>
      IO.eprintln s!"REJECTED: failed to read input: {err}"
      return 1
    | .ok raw =>
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
