/-
Apps.ViprCheck: standalone CLI verifier for Mini-VIPR rational MILP branch-and-bound certificates.
-/

import Apps.Common
import GoblinFormal.Track4

open Apps.Common
open GoblinFormal
open GoblinFormal.Track4

namespace Apps.ViprCheck

private def usageText : String :=
  "usage: vipr-check [--jsonl | --batch <array.json | ->] [<certificate.json | ->]"

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

private def batchFmt : BatchFormat := {
  readErr := fun path err => s!"ERROR: read '{path}': {err}"
  parseErr := fun path msg => s!"ERROR: parse batch JSON '{path}': {msg}"
  emptyErr := fun path => s!"ERROR: empty batch JSON '{path}'"
}

private def runBatch (path : String) : IO UInt32 :=
  runBatchWith batchFmt path (fun item => verifyCertVal item *> .ok ())

end Apps.ViprCheck

/-- CLI entry point for `vipr-check`, verifying VIPR branch-and-bound and presolve proof traces. -/
def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    IO.println Apps.ViprCheck.usageText
    return 0
  | ["--jsonl"] =>
    Apps.Common.runJsonlLoop (← IO.getStdin) (← IO.getStdout) fun jv =>
      Apps.ViprCheck.formatJsonlOk <$> Apps.ViprCheck.verifyCertVal jv
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
        Apps.Common.readInput path
      catch err =>
        IO.eprintln s!"ERROR: read '{path}': {err}"
        return 1
    match parseJsonStrict raw >>= Apps.ViprCheck.verifyCertVal with
    | .ok _ =>
      IO.println "VERIFIED"
      return 0
    | .error msg =>
      IO.eprintln s!"REJECTED: {msg}"
      return 1
  | other :: _ =>
    IO.eprintln s!"ERROR: unexpected arguments starting with '{other}'"
    return 1
