/-
Apps.SimdCfgCheck: standalone CLI checking amd64 SIMD vector ISA and CFG invariants
(Track 5C: zero dirty upper YMM/ZMM transitions before legacy SSE, RET, or CALL;
no unguarded EVEX/%zmm instructions outside explicitly named AVX-512 functions).
-/

import Std.Data.HashMap
import GoblinFormal.Common.Json
import GoblinFormal.Track5C

open GoblinFormal
open GoblinFormal.Track5C

private def printUsage : IO Unit :=
  IO.println "usage: simd-cfg-check <cfg.json | inline-json>"

private def hazardKindLabel : VectorHazardKind → String
  | .legacySSEWhileDirty => "LEGACY_SSE_WHILE_DIRTY"
  | .dirtyRet => "DIRTY_RET"
  | .dirtyCall => "DIRTY_CALL"
  | .unguardedEVEX => "UNGUARDED_EVEX"
  | .internalError msg => s!"INTERNAL_ERROR({msg})"

private def parseRawInsns (arr : Array JsonVal) : Except String (Array Amd64Insn) := do
  let mut addrMap : Std.HashMap String Nat := {}
  let mut raws : Array (String × String × String × String) := #[]
  for item in arr do
    let _ ← item.asObj
    let addr ← item.getString "addr"
    let op ← item.getString "op"
    let args ← item.getString "args"
    let raw :=
      match item.getField? "raw" with
      | some v => match v.asString with | .ok s => s | .error _ => ""
      | none => ""
    let norm := normalizeHexAddr addr
    if !norm.isEmpty then
      if addrMap[norm]? |>.isNone then
        addrMap := addrMap.insert norm raws.size
    raws := raws.push (addr, op, args, raw)
  let mut out : Array Amd64Insn := #[]
  for (addr, op, args, raw) in raws do
    let jmpTgtIdx :=
      match parseJumpTargetHex args with
      | some hex => addrMap[normalizeHexAddr hex]?
      | none => none
    out := out.push (classifyAmd64Insn addr op args raw jmpTgtIdx)
  return out

private def parseFunctions (root : JsonVal) : Except String (Array Amd64FuncCFG) := do
  let fnVals ←
    match root with
    | .arr a => pure a
    | .obj fields =>
      match fields.find? (fun (k, _) => k == "functions") with
      | some (_, .arr a) => pure a
      | some _ => throw "field 'functions' must be a JSON array"
      | none => pure #[root]
    | _ => throw "expected JSON object or array of functions"
  let mut funcs : Array Amd64FuncCFG := #[]
  for val in fnVals do
    let _ ← val.asObj
    let name ← val.getString "name"
    let insnVals ← val.getArray "insns"
    let insns ← parseRawInsns insnVals
    funcs := funcs.push { name := name, insns := insns }
  return funcs

private def verifySimdCfgJson (raw : String) : Except String Nat := do
  let root ← parseJson raw
  let funcs ← parseFunctions root
  let mut checked : Nat := 0
  for fn in funcs do
    if fn.hasVectorOps then
      checked := checked + 1
      match checkVectorCFG fn with
      | .ok () => pure ()
      | .error haz =>
        throw s!"{haz.funcName} @ {haz.addr} [{hazardKindLabel haz.kind}]: {haz.insn}"
  return checked

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    printUsage
    return 0
  | [arg] =>
    let trimmed := arg.trimAscii.toString
    let raw ←
      if trimmed.startsWith "{" || trimmed.startsWith "[" then
        pure trimmed
      else
        try
          IO.FS.readFile arg
        catch err =>
          IO.eprintln s!"ERROR: read '{arg}': {err}"
          return 1
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
