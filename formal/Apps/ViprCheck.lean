/-
Apps.ViprCheck: standalone CLI verifier for Mini-VIPR rational MILP branch-and-bound certificates.
-/

import GoblinFormal.Track4

open GoblinFormal.Track4

private def printUsage : IO Unit :=
  IO.println "usage: vipr-check <certificate.json>"

def main (args : List String) : IO UInt32 := do
  match args with
  | [] | ["--help"] | ["-h"] =>
    printUsage
    return 0
  | [path] =>
    let raw ←
      try
        IO.FS.readFile path
      catch err =>
        IO.eprintln s!"ERROR: read '{path}': {err}"
        return 1
    match verifyMiniVIPRJson raw with
    | .ok () =>
      IO.println "VERIFIED"
      return 0
    | .error msg =>
      IO.eprintln s!"REJECTED: {msg}"
      return 1
  | other :: _ =>
    IO.eprintln s!"ERROR: unexpected arguments starting with '{other}'"
    return 1
