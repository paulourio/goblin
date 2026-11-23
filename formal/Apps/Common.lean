/-
Apps.Common: shared CLI I/O, JSON string/rational formatting, field decoding,
JSONL streaming loop, and batch runner utilities for standalone Lean 4 checkers.
Kept outside `GoblinFormal` so the proof kernel remains free of CLI/IO code.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

open GoblinFormal

namespace Apps.Common

def hexDigit (n : Nat) : Char :=
  if n < 10 then Char.ofNat ('0'.toNat + n)
  else Char.ofNat ('a'.toNat + (n - 10))

def jsonQuote (s : String) : String :=
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

def boolJson (b : Bool) : String :=
  if b then "true" else "false"

def ratObjJson (r : Rat) : String :=
  "{\"num\":\"" ++ toString r.num ++ "\",\"den\":\"" ++ toString r.den ++ "\"}"

def optRatJson : Option Rat → String
  | none => "null"
  | some r => ratObjJson r

def optNonNullField? (v : JsonVal) (key : String) : Option JsonVal :=
  match v.getField? key with
  | none | some .null => none
  | some fv => some fv

def hasNonNullField (v : JsonVal) (key : String) : Bool :=
  (optNonNullField? v key).isSome

def getOptionalField (root : JsonVal) (names : List String) : Option JsonVal :=
  names.findSome? (optNonNullField? root)

def parseOptRat : JsonVal → Except String (Option Rat)
  | .null => .ok none
  | v => some <$> v.asRat

def parseRatArray (arr : Array JsonVal) : Except String (Array Rat) :=
  arr.mapM JsonVal.asRat

def parseOptRatArray (arr : Array JsonVal) : Except String (Array (Option Rat)) :=
  arr.mapM parseOptRat

def parseBoolArray (arr : Array JsonVal) : Except String (Array Bool) :=
  arr.mapM JsonVal.asBool

def parseNatArray (arr : Array JsonVal) : Except String (Array Nat) :=
  arr.mapM JsonVal.asNat

def asRatOrBitsWith (parseBits : JsonVal → Except String UInt64) (v : JsonVal) :
    Except String Rat :=
  match v.getField? "bits" with
  | some bv => do
    let bits ← parseBits bv
    match float64BitsToRat bits with
    | some r => .ok r
    | none => .error "non-finite float64 bits (Inf/NaN)"
  | none => v.asRat

def effectivePrintBound (root : JsonVal) (cliFlag : Bool) : Except String Bool := do
  if cliFlag then
    return true
  match optNonNullField? root "printBound" with
  | none => return false
  | some pv => pv.asBool

def toFinFn {α : Type} (arr : Array α) (n : Nat) (errMsg : String) :
    Except String (Fin n → α) :=
  if h : arr.size = n then
    .ok (fun i => arr[i.val]'(h ▸ i.isLt))
  else
    .error errMsg

def resolveSubcmd (defaultSubcmd : Option String) (v : JsonVal) (missingMsg : String) :
    Except String String := do
  match defaultSubcmd with
  | some s => .ok s
  | none =>
    for key in ["subcommand", "op", "mode"] do
      if let some fv := optNonNullField? v key then
        return ← fv.asString
    .error missingMsg

def readInput (path : String) : IO String := do
  if path == "-" then
    (← IO.getStdin).readToEnd
  else
    IO.FS.readFile ⟨path⟩

partial def runJsonlLoop (stdin stdout : IO.FS.Stream)
    (handle : JsonVal → Except String String) : IO UInt32 := do
  let line ← stdin.getLine
  if line.isEmpty then
    return 0
  let trimmed := line.trimAscii.toString
  if !trimmed.isEmpty then
    let resp :=
      match parseJsonStrict trimmed >>= handle with
      | .ok okJson => okJson
      | .error err => "{\"ok\":false,\"error\":" ++ jsonQuote err ++ "}"
    stdout.putStr resp
    stdout.putStr "\n"
    stdout.flush
  runJsonlLoop stdin stdout handle

structure BatchFormat where
  readErr : String → IO.Error → String
  parseErr : String → String → String
  emptyErr : String → String
  itemErr : Nat → String → String := fun i msg => s!"BATCH FAIL at index {i}: {msg}"

def runBatchWith (fmt : BatchFormat) (path : String)
    (handle : JsonVal → Except String Unit) : IO UInt32 := do
  let raw ←
    try
      readInput path
    catch err =>
      IO.eprintln (fmt.readErr path err)
      return 1
  match parseJsonStrict raw >>= JsonVal.asArray with
  | .error msg =>
    IO.eprintln (fmt.parseErr path msg)
    return 1
  | .ok items =>
    if items.isEmpty then
      IO.eprintln (fmt.emptyErr path)
      return 1
    for i in [:items.size] do
      match handle items[i]! with
      | .ok () => pure ()
      | .error msg =>
        IO.eprintln (fmt.itemErr i msg)
        return 1
    IO.println s!"VERIFIED ({items.size} items)"
    return 0

end Apps.Common
