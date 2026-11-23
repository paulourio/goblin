/-
Apps.ViprCheck: standalone CLI verifier for Mini-VIPR rational MILP branch-and-bound certificates.
-/

import GoblinFormal.Track4

open GoblinFormal
open GoblinFormal.Track4

namespace Apps.ViprCheck

private def usageText : String :=
  "usage: vipr-check [--jsonl | --batch <array.json | ->] [<certificate.json | ->]"

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

private def readInput (path : String) : IO String := do
  if path == "-" then
    (← IO.getStdin).readToEnd
  else
    IO.FS.readFile path

/-- Pinpoints the failing step or root check when `checkMiniVIPR cert` returns `false`. -/
private def diagnoseFailure (cert : MiniVIPRCert) : String := Id.run do
  let mut entries : Array (DerivedEntry cert.inst.n) :=
    cert.inst.baseRows.map fun r => { con := r, assumptions := [] }
  for i in [:cert.steps.size] do
    if hi : i < cert.steps.size then
      match evalProofStep cert.inst entries cert.steps[i] with
      | some e => entries := entries.push e
      | none => return s!"Mini-VIPR step {i} (entry {cert.inst.baseRows.size + i}) failed verification"
  if cert.rootStep >= cert.steps.size then
    return s!"rootStep {cert.rootStep} out of range for {cert.steps.size} steps"
  let rootIdx := cert.inst.baseRows.size + cert.rootStep
  if h : rootIdx < entries.size then
    let rootEntry := entries[rootIdx]
    if !rootEntry.assumptions.isEmpty then
      return s!"root entry {rootIdx} has undischarged assumptions {rootEntry.assumptions}"
    match cert.mode with
    | .infeasible =>
      return s!"root entry {rootIdx} does not derive contradiction in infeasible mode"
    | .optimal xStar =>
      if !checkIncumbentFeasible cert.inst xStar then
        return "incumbent violates bounds, linear constraints, or integrality in optimal mode"
      return s!"root entry {rootIdx} does not match objective or close optimal bound"
    | .bound targetLB =>
      if let some xInc := cert.incumbent? then
        if !checkIncumbentFeasible cert.inst xInc then
          return "optional incumbent violates bounds, linear constraints, or integrality in bound mode"
      let gotLB := cert.inst.objConst + rootEntry.con.rhs
      return s!"root entry {rootIdx} fails bound check (targetLB={targetLB}, derived={gotLB})"
  else
    return "Mini-VIPR trace failed verification"

/-- Pinpoints the failing presolve step, reduced-instance mismatch, or Mini-VIPR step. -/
private def diagnosePresolveFailure (cert : VIPRWithPresolveCert) : String := Id.run do
  let mut st := PresolveState.ofInstance cert.orig
  for i in [:cert.presolve.size] do
    if hi : i < cert.presolve.size then
      match applyPresolveStep st cert.presolve[i] with
      | some st' => st := st'
      | none => return s!"presolve step {i} failed verification"
  if !checkVIPRInstanceEq st.toInstance cert.reduced.inst then
    return "presolve final instance does not match reduced Mini-VIPR instance"
  return diagnoseFailure cert.reduced

private def hasNonNullField (jv : JsonVal) (key : String) : Bool :=
  match jv.getField? key with
  | none | some .null => false
  | some _ => true

private def verifyCertVal (jv : JsonVal) : Except String MiniVIPRCert := do
  if hasNonNullField jv "orig" then
    let envCert ← decodeVIPRWithPresolveCert jv
    if checkVIPRWithPresolve envCert then
      .ok envCert.reduced
    else
      .error (diagnosePresolveFailure envCert)
  else if hasNonNullField jv "presolve" then
    .error "presolve field requires orig instance"
  else
    let cert ← decodeMiniVIPRCert jv
    if checkMiniVIPR cert then
      .ok cert
    else
      .error (diagnoseFailure cert)

private def formatJsonlOk (cert : MiniVIPRCert) : String :=
  match cert.mode with
  | .bound targetLB =>
    "{\"ok\":true,\"detail\":\"VERIFIED\",\"bound\":" ++ ratObjJson targetLB ++ "}"
  | _ =>
    "{\"ok\":true,\"detail\":\"VERIFIED\"}"

private partial def runJsonlLoop (stdin stdout : IO.FS.Stream) : IO UInt32 := do
  let line ← stdin.getLine
  if line.isEmpty then
    return 0
  let trimmed := line.trimAscii.toString
  if !trimmed.isEmpty then
    let resp :=
      match parseJson trimmed >>= verifyCertVal with
      | .ok cert => formatJsonlOk cert
      | .error err => "{\"ok\":false,\"error\":" ++ jsonQuote err ++ "}"
    stdout.putStr resp
    stdout.putStr "\n"
    stdout.flush
  runJsonlLoop stdin stdout

private def runBatch (path : String) : IO UInt32 := do
  let raw ←
    try
      readInput path
    catch err =>
      IO.eprintln s!"ERROR: read '{path}': {err}"
      return 1
  match parseJson raw >>= JsonVal.asArray with
  | .error msg =>
    IO.eprintln s!"ERROR: parse batch JSON '{path}': {msg}"
    return 1
  | .ok items =>
    for i in [:items.size] do
      match verifyCertVal items[i]! with
      | .ok _ => pure ()
      | .error msg =>
        IO.eprintln s!"BATCH FAIL at index {i}: {msg}"
        return 1
    IO.println s!"VERIFIED ({items.size} items)"
    return 0

end Apps.ViprCheck

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    IO.println Apps.ViprCheck.usageText
    return 0
  | ["--jsonl"] =>
    Apps.ViprCheck.runJsonlLoop (← IO.getStdin) (← IO.getStdout)
  | ["--batch", path] =>
    Apps.ViprCheck.runBatch path
  | ["--batch"] =>
    IO.eprintln "ERROR: missing path after --batch"
    return 1
  | [path] =>
    if path.startsWith "-" && path != "-" then
      IO.eprintln s!"ERROR: unknown flag '{path}'"
      return 1
    let raw ←
      try
        Apps.ViprCheck.readInput path
      catch err =>
        IO.eprintln s!"ERROR: read '{path}': {err}"
        return 1
    match parseJson raw >>= Apps.ViprCheck.verifyCertVal with
    | .ok _ =>
      IO.println "VERIFIED"
      return 0
    | .error msg =>
      IO.eprintln s!"REJECTED: {msg}"
      return 1
  | other :: _ =>
    IO.eprintln s!"ERROR: unexpected arguments starting with '{other}'"
    return 1
