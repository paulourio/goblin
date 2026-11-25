/-
Apps.SimdCfgCheck: standalone CLI checking amd64 SIMD vector ISA and CFG invariants
(Track 5C: zero dirty upper YMM/ZMM transitions before legacy SSE, RET, CALL, external
tail/conditional jumps, or fall-off-end; fail closed on indirect jumps, Plan 9 syntax,
duplicate addresses/keys, empty inputs, and unguarded EVEX instructions outside
explicitly named AVX-512 functions).
-/

import Std.Data.HashMap
import Apps.Common
import GoblinFormal.Common.Json
import GoblinFormal.Track5C

open GoblinFormal
open GoblinFormal.Track5C

namespace Apps.SimdCfgCheck

private def printUsage : IO Unit :=
  IO.println "usage: simd-cfg-check [--batch <batch.json | inline-json | ->] | <cfg.json | inline-json | ->"

private def hazardKindLabel : VectorHazardKind → String
  | .legacySSEWhileDirty => "LEGACY_SSE_WHILE_DIRTY"
  | .dirtyRet => "DIRTY_RET"
  | .dirtyCall => "DIRTY_CALL"
  | .dirtyCondExtJmp => "DIRTY_COND_EXT_JMP"
  | .dirtyFallOffEnd => "DIRTY_FALL_OFF_END"
  | .indirectOrUnresolvedJmp => "INDIRECT_OR_UNRESOLVED_JMP"
  | .unguardedEVEX => "UNGUARDED_EVEX"
  | .internalError msg => s!"INTERNAL_ERROR({msg})"

private def parseRawInsns (funcName : String) (arr : Array JsonVal) : Except String (Array Amd64Insn) := do
  if arr.isEmpty then
    throw s!"function '{funcName}' has 0 instructions"
  let mut addrMap : Std.HashMap String Nat := {}
  let mut raws : Array (String × String × String × String) := #[]
  for item in arr do
    let _ ← item.asObj
    let addr ← item.getString "addr"
    let op ← item.getString "op"
    let args ← item.getString "args"
    let raw ←
      match item.getField? "raw" with
      | none | some .null => pure ""
      | some (.str s) => pure s
      | some _ => throw s!"function '{funcName}' @ {addr}: field 'raw' must be a string or null"
    if !isValidHexAddr addr then
      throw s!"function '{funcName}': invalid instruction address '{addr}'"
    let (effOp, effArgs) := normalizeX86PrefixOpArgs op args
    if effOp.isEmpty then
      throw s!"function '{funcName}' @ {addr}: empty instruction opcode"
    if isPlan9Syntax effArgs then
      throw s!"function '{funcName}' @ {addr}: unsupported Plan 9 / non-AT&T syntax '{op} {args}'"
    let norm := normalizeHexAddr addr
    if addrMap[norm]? |>.isSome then
      throw s!"function '{funcName}': duplicate instruction address '{norm}'"
    addrMap := addrMap.insert norm raws.size
    raws := raws.push (addr, op, args, raw)
  let mut out : Array Amd64Insn := #[]
  for (addr, op, args, raw) in raws do
    let (_, effArgs) := normalizeX86PrefixOpArgs op args
    let jmpTgtIdx :=
      match parseJumpTargetHex effArgs with
      | some hex => addrMap[normalizeHexAddr hex]?
      | none => none
    out := out.push (classifyAmd64Insn addr op args raw jmpTgtIdx)
  return out

private def parseFunctions (root : JsonVal) : Except String (Array Amd64FuncCFG) := do
  let fnVals ←
    match root with
    | .arr a => pure a
    | .obj _ =>
      match root.getField? "functions" with
      | some (.arr a) => pure a
      | some _ => throw "field 'functions' must be a JSON array"
      | none => pure #[root]
    | _ => throw "expected JSON object or array of functions"
  if fnVals.isEmpty then
    throw "functions array must be non-empty"
  let mut funcs : Array Amd64FuncCFG := #[]
  for val in fnVals do
    let _ ← val.asObj
    let name ← val.getString "name"
    let insnVals ← val.getArray "insns"
    let insns ← parseRawInsns name insnVals
    funcs := funcs.push { name := name, insns := insns }
  return funcs

private def verifySimdCfgVal (root : JsonVal) : Except String Nat := do
  let funcs ← parseFunctions root
  let mut checked : Nat := 0
  for fn in funcs do
    if fn.insns.any (·.isPlan9) then
      throw s!"{fn.name}: unsupported Plan 9 / non-AT&T syntax"
    if fn.hasVectorOps then
      checked := checked + 1
      match checkVectorCFG fn with
      | .ok () => pure ()
      | .error haz =>
        throw s!"{haz.funcName} @ {haz.addr} [{hazardKindLabel haz.kind}]: {haz.insn}"
  return checked

private def verifySimdCfgJson (raw : String) : Except String Nat := do
  let root ← parseJsonStrict raw
  verifySimdCfgVal root

private def verifySimdCfgBatchJson (raw : String) : Except String (Nat × Nat) := do
  let root ← parseJsonStrict raw
  let items ← root.asArray
  if items.isEmpty then
    throw "batch array must be non-empty"
  let mut totalChecked : Nat := 0
  for i in [:items.size] do
    match verifySimdCfgVal items[i]! with
    | .ok n => totalChecked := totalChecked + n
    | .error msg => throw s!"batch[{i}]: {msg}"
  return (items.size, totalChecked)

private def readArgInput (arg : String) : IO (Except String String) := do
  let trimmed := arg.trimAscii.toString
  if trimmed.startsWith "{" || trimmed.startsWith "[" then
    return .ok trimmed
  try
    return .ok (← Apps.Common.readInput (if trimmed == "-" then "-" else arg))
  catch err =>
    let label := if trimmed == "-" then "read stdin" else s!"read '{arg}'"
    return .error s!"{label}: {err}"

end Apps.SimdCfgCheck

open Apps.SimdCfgCheck in
/-- CLI entry point for `simd-cfg-check`, verifying AMD64 SIMD control-flow and vector-hazard invariants. -/
def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    printUsage
    return 0
  | ["--batch", arg] =>
    match ← readArgInput arg with
    | .error msg =>
      IO.eprintln s!"ERROR: {msg}"
      return 1
    | .ok raw =>
      match verifySimdCfgBatchJson raw with
      | .ok (nItems, checked) =>
        IO.println s!"VERIFIED ({nItems} items, {checked} vector functions)"
        return 0
      | .error msg =>
        IO.eprintln s!"REJECTED: {msg}"
        return 1
  | [arg] =>
    match ← readArgInput arg with
    | .error msg =>
      IO.eprintln s!"ERROR: {msg}"
      return 1
    | .ok raw =>
      match verifySimdCfgJson raw with
      | .ok checked =>
        IO.println s!"VERIFIED ({checked} vector functions)"
        return 0
      | .error msg =>
        IO.eprintln s!"REJECTED: {msg}"
        return 1
  | other :: _ =>
    IO.eprintln s!"ERROR: unexpected arguments starting with '{other}'"
    return 1
