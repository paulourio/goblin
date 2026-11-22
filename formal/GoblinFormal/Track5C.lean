/-
GoblinFormal.Track5C: Verified amd64 SIMD Vector ISA and CFG Safety,
mirroring cmd/goblin-optcheck/main.go vector disassembly checks with
machine-checked soundness over inductive CFG operational semantics.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Common.Json

namespace GoblinFormal.Track5C

/-- Checks whether `sub` occurs as a substring of `s`. -/
def hasSubstr (s sub : String) : Bool :=
  sub.isEmpty || (s.splitOn sub).length > 1

private def isWordBoundaryChar (c : Char) : Bool :=
  !(c.isAlphanum || c == '_')

private def matchOpmaskFromList : List Char → Bool
  | '%' :: 'k' :: d :: rest =>
    if '0' <= d && d <= '7' then
      match rest with
      | [] => true
      | c :: _ =>
        if isWordBoundaryChar c then true
        else matchOpmaskFromList (d :: rest)
    else
      matchOpmaskFromList ('k' :: d :: rest)
  | _ :: rest => matchOpmaskFromList rest
  | [] => false

/-- Matches `%k[0-7]\b` in GNU objdump operand strings. -/
def hasOpmaskReg (args : String) : Bool :=
  matchOpmaskFromList args.toList

/-- Normalizes hexadecimal address string by trimming leading zeros and lowercasing. -/
def normalizeHexAddr (addr : String) : String :=
  let s := addr.trimAscii.toString.toLower
  let rec dropZeros : List Char → List Char
    | '0' :: rest => dropZeros rest
    | cs => cs
  let trimmed := String.ofList (dropZeros s.toList)
  if trimmed.isEmpty then "0" else trimmed

private def isHexDigit (c : Char) : Bool :=
  ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')

/-- Parses a direct hexadecimal branch target from GNU objdump instruction args. -/
def parseJumpTargetHex (args : String) : Option String :=
  let trimmed := args.trimAscii.toString
  if trimmed.isEmpty then none
  else
    let tok := (trimmed.splitOn " ").headD ""
    if tok.isEmpty || tok.startsWith "*" || tok.startsWith "%" then none
    else if tok.toList.all isHexDigit then some (normalizeHexAddr tok)
    else none

/-- Detects AVX-512 EVEX instructions from opcode and operand strings. -/
def isEVEXInsn (op args : String) : Bool :=
  hasSubstr args "%zmm" ||
  hasOpmaskReg args ||
  hasSubstr args "{1to" ||
  hasSubstr args "{rn-" ||
  hasSubstr args "{rz-" ||
  hasSubstr args "{rd-" ||
  hasSubstr args "{ru-" ||
  hasSubstr args "{sae}" ||
  op.startsWith "vpternlog" ||
  op.startsWith "vpermi2" ||
  op.startsWith "vpermt2" ||
  op.startsWith "vcompress" ||
  op.startsWith "vexpand"

/-- Detects VEX 256-bit YMM instructions (excluding EVEX and vzeroupper/vzeroall). -/
def isVEXYMMInsn (op args : String) : Bool :=
  if isEVEXInsn op args then false
  else if op == "vzeroupper" || op == "vzeroall" then false
  else op.startsWith "v" && hasSubstr args "%ymm"

/-- Detects legacy non-VEX SSE instructions referencing `%xmm`. -/
def isLegacySSEInsn (op args : String) : Bool :=
  !op.startsWith "v" && hasSubstr args "%xmm"

/-- Detects instructions that dirty upper YMM/ZMM AVX state. -/
def dirtiesUpperVector (op args : String) : Bool :=
  if !op.startsWith "v" || op == "vzeroupper" || op == "vzeroall" then false
  else hasSubstr args "%ymm" || hasSubstr args "%zmm" || isEVEXInsn op args

/-- Detects non-returning runtime panic calls. -/
def isNoreturnRuntimeCall (raw : String) : Bool :=
  hasSubstr raw "runtime.panic" || hasSubstr raw "runtime.gopanic"

/-- Classified x86-64 disassembly instruction for CFG auditing. -/
structure Amd64Insn where
  addr : String
  op : String
  args : String
  raw : String := ""
  isEVEX : Bool
  isVEXYMM : Bool
  isLegacySSE : Bool
  dirtiesUpper : Bool
  isVzeroupper : Bool
  isRet : Bool
  isTrap : Bool
  isCall : Bool
  isMorestackCall : Bool
  isNoreturnPanic : Bool
  isUncondJmp : Bool
  isCondJmp : Bool
  hasHexJmpTarget : Bool
  jmpTargetIdx : Option Nat
  deriving DecidableEq, Repr, Inhabited

/-- Constructs a classified `Amd64Insn` matching `cmd/goblin-optcheck/main.go`. -/
def classifyAmd64Insn (addr op args raw : String) (jmpTargetIdx : Option Nat) : Amd64Insn :=
  let uncond := op == "jmp" || op == "jmpq"
  let cond := !uncond && op.startsWith "j"
  { addr := normalizeHexAddr addr
    op := op
    args := args
    raw := raw
    isEVEX := isEVEXInsn op args
    isVEXYMM := isVEXYMMInsn op args
    isLegacySSE := isLegacySSEInsn op args
    dirtiesUpper := dirtiesUpperVector op args
    isVzeroupper := op == "vzeroupper" || op == "vzeroall"
    isRet := op == "ret" || op == "retq"
    isTrap := op == "ud2" || op == "int3"
    isCall := op == "call" || op == "callq"
    isMorestackCall := hasSubstr raw "runtime.morestack"
    isNoreturnPanic := isNoreturnRuntimeCall raw
    isUncondJmp := uncond
    isCondJmp := cond
    hasHexJmpTarget := (parseJumpTargetHex args).isSome
    jmpTargetIdx := jmpTargetIdx }

/-- Disassembled function CFG for amd64 vector safety checking. -/
structure Amd64FuncCFG where
  name : String
  insns : Array Amd64Insn
  deriving DecidableEq, Repr, Inhabited

/-- Returns true when function name explicitly marks an AVX-512 routine. -/
def Amd64FuncCFG.isAVX512Name (fn : Amd64FuncCFG) : Bool :=
  hasSubstr fn.name.toLower "avx512"

/-- Returns true when function uses EVEX or VEX-YMM instructions. -/
def Amd64FuncCFG.hasVectorOps (fn : Amd64FuncCFG) : Bool :=
  fn.insns.any (fun ins => ins.isEVEX || ins.isVEXYMM)

/-- Vector ISA CFG hazard categories. -/
inductive VectorHazardKind where
  | legacySSEWhileDirty
  | dirtyRet
  | dirtyCall
  | unguardedEVEX
  | internalError (msg : String)
  deriving DecidableEq, Repr, Inhabited

/-- Structured hazard witness emitted by `checkVectorCFG`. -/
structure VectorHazard where
  funcName : String
  addr : String
  kind : VectorHazardKind
  insn : String
  deriving DecidableEq, Repr, Inhabited

/-- Upper-register dirty state transition after executing `ins`. -/
@[inline] def nextDirty (dirty : Bool) (ins : Amd64Insn) : Bool :=
  if ins.isVzeroupper then false
  else if ins.dirtiesUpper then true
  else dirty

/-- Hazard condition for legacy SSE executed while upper vector registers are dirty. -/
@[inline] def insnLegacySSEHazard (ins : Amd64Insn) (dirty : Bool) : Bool :=
  !ins.isVzeroupper && !ins.dirtiesUpper && dirty && ins.isLegacySSE

/-- Hazard condition for exiting or calling external code with dirty upper vector state. -/
@[inline] def insnDirtyExitOrCallHazard (ins : Amd64Insn) (dirty : Bool) :
    Option VectorHazardKind :=
  let nd := nextDirty dirty ins
  if ins.isRet && nd then some .dirtyRet
  else if ins.isCall && !ins.isNoreturnPanic && nd && !ins.isMorestackCall then some .dirtyCall
  else if ins.isUncondJmp && ins.jmpTargetIdx.isNone && ins.hasHexJmpTarget && nd &&
      !ins.isMorestackCall then some .dirtyRet
  else none

/-- Operational successor list of `(idx, dirty)` in `fn.insns`. -/
def cfgSucc (fn : Amd64FuncCFG) (idx : Nat) (dirty : Bool) : List (Nat × Bool) :=
  if h : idx < fn.insns.size then
    let ins := fn.insns[idx]
    let nd := nextDirty dirty ins
    if ins.isRet || ins.isTrap then []
    else if ins.isCall then
      if ins.isNoreturnPanic then []
      else if idx + 1 < fn.insns.size then [(idx + 1, nd)] else []
    else if ins.isUncondJmp then
      match ins.jmpTargetIdx with
      | some t => if t < fn.insns.size then [(t, nd)] else []
      | none => []
    else if ins.isCondJmp then
      let fall := if idx + 1 < fn.insns.size then [(idx + 1, nd)] else []
      match ins.jmpTargetIdx with
      | some t => if t < fn.insns.size then (t, nd) :: fall else fall
      | none => fall
    else
      if idx + 1 < fn.insns.size then [(idx + 1, nd)] else []
  else []

/-- Single-step CFG transition relation. -/
inductive CfgStep (fn : Amd64FuncCFG) : Nat → Bool → Nat → Bool → Prop where
  | mk {idx : Nat} {dirty : Bool} {idx' : Nat} {dirty' : Bool}
      (hLt : idx < fn.insns.size)
      (hMem : (idx', dirty') ∈ cfgSucc fn idx dirty) :
      CfgStep fn idx dirty idx' dirty'

/-- Reachable `(idx, dirty)` states starting from `(0, false)`. -/
inductive CfgReachable (fn : Amd64FuncCFG) : Nat → Bool → Prop where
  | entry (hPos : 0 < fn.insns.size) : CfgReachable fn 0 false
  | step {i d i' d'} : CfgReachable fn i d → CfgStep fn i d i' d' → CfgReachable fn i' d'

/-- Successor list along unguarded paths (stopping before conditional branches). -/
def unguardedSucc (fn : Amd64FuncCFG) (idx : Nat) : List Nat :=
  if h : idx < fn.insns.size then
    let ins := fn.insns[idx]
    if ins.isRet || ins.isTrap || ins.isCondJmp then []
    else if ins.isUncondJmp then
      match ins.jmpTargetIdx with
      | some t => if t < fn.insns.size then [t] else []
      | none => []
    else
      if idx + 1 < fn.insns.size then [idx + 1] else []
  else []

/-- Single-step unguarded CFG transition before any conditional branch. -/
inductive UnguardedStep (fn : Amd64FuncCFG) : Nat → Nat → Prop where
  | mk {idx idx' : Nat} (hLt : idx < fn.insns.size)
      (hMem : idx' ∈ unguardedSucc fn idx) : UnguardedStep fn idx idx'

/-- Instructions reachable from entry without crossing any conditional branch. -/
inductive UnguardedReachable (fn : Amd64FuncCFG) : Nat → Prop where
  | entry (hPos : 0 < fn.insns.size) : UnguardedReachable fn 0
  | step {i i'} : UnguardedReachable fn i → UnguardedStep fn i i' → UnguardedReachable fn i'

/-! ### Executable CFG Analyzers -/

@[inline] private def stateKey (idx : Nat) (dirty : Bool) : Nat :=
  idx * 2 + (if dirty then 1 else 0)

private theorem stateKey_lt {n idx : Nat} (dirty : Bool) (h : idx < n) :
    stateKey idx dirty < 2 * n := by
  unfold stateKey
  cases dirty <;> simp <;> omega

private theorem stateKey_inj {i1 i2 : Nat} {d1 d2 : Bool}
    (h : stateKey i1 d1 = stateKey i2 d2) : i1 = i2 ∧ d1 = d2 := by
  unfold stateKey at h
  cases d1 <;> cases d2 <;> simp at h
  · exact ⟨by omega, rfl⟩
  · omega
  · omega
  · exact ⟨by omega, rfl⟩

private def pushMany (succs : List (Nat × Bool)) (seen : Array Bool)
    (queue : List (Nat × Bool)) : Array Bool × List (Nat × Bool) :=
  succs.foldl (fun (acc : Array Bool × List (Nat × Bool)) (st : Nat × Bool) =>
    let k := stateKey st.1 st.2
    if hk : k < acc.1.size then
      if acc.1[k] then acc
      else (acc.1.set k true hk, st :: acc.2)
    else acc) (seen, queue)

private def bfsLoop (fn : Amd64FuncCFG) :
    Nat → Array Bool → List (Nat × Bool) → Option (Array Bool)
  | 0, seen, [] => some seen
  | 0, _, _ :: _ => none
  | _ + 1, seen, [] => some seen
  | fuel + 1, seen, (idx, dirty) :: rest =>
    let succs := cfgSucc fn idx dirty
    let (seen', queue') := pushMany succs seen rest
    bfsLoop fn fuel seen' queue'

private def unguardedPushMany (succs : List Nat) (seen : Array Bool) (queue : List Nat) :
    Array Bool × List Nat :=
  succs.foldl (fun (acc : Array Bool × List Nat) (idx : Nat) =>
    if hk : idx < acc.1.size then
      if acc.1[idx] then acc
      else (acc.1.set idx true hk, idx :: acc.2)
    else acc) (seen, queue)

private def unguardedBfsLoop (fn : Amd64FuncCFG) :
    Nat → Array Bool → List Nat → Option (Array Bool)
  | 0, seen, [] => some seen
  | 0, _, _ :: _ => none
  | _ + 1, seen, [] => some seen
  | fuel + 1, seen, idx :: rest =>
    let succs := unguardedSucc fn idx
    let (seen', queue') := unguardedPushMany succs seen rest
    unguardedBfsLoop fn fuel seen' queue'

private def scanEVEX (fn : Amd64FuncCFG) (seen : Array Bool) (i : Nat) : Bool :=
  if hi : i < fn.insns.size then
    let hit := (seen[i]? == some true) && fn.insns[i].isEVEX
    hit || scanEVEX fn seen (i + 1)
  else false
termination_by fn.insns.size - i

/-- Returns true iff an EVEX instruction is reachable before any conditional branch. -/
def checkUnguardedEVEX (fn : Amd64FuncCFG) : Bool :=
  let n := fn.insns.size
  if hPos : 0 < n then
    let initSeen := (Array.replicate n false).set 0 true (by simp [hPos])
    match unguardedBfsLoop fn (2 * n + 2) initSeen [0] with
    | none => true
    | some seen => scanEVEX fn seen 0
  else false

private def checkStateHazard (fn : Amd64FuncCFG) (seen : Array Bool)
    (idx : Nat) (hIdx : idx < fn.insns.size) (dirty : Bool) : Except VectorHazard Unit :=
  let ins := fn.insns[idx]
  let insnStr := ins.op ++ " " ++ ins.args
  if seen[stateKey idx dirty]? == some true then
    if insnLegacySSEHazard ins dirty then
      .error (VectorHazard.mk fn.name ins.addr .legacySSEWhileDirty insnStr)
    else
      match insnDirtyExitOrCallHazard ins dirty with
      | some hkKind =>
        .error (VectorHazard.mk fn.name ins.addr hkKind insnStr)
      | none => .ok ()
  else .ok ()

private def checkReachableHazardsGo (fn : Amd64FuncCFG) (seen : Array Bool) (idx : Nat) :
    Except VectorHazard Unit :=
  if hIdx : idx < fn.insns.size then
    match checkStateHazard fn seen idx hIdx false with
    | .error e => .error e
    | .ok () =>
      match checkStateHazard fn seen idx hIdx true with
      | .error e => .error e
      | .ok () => checkReachableHazardsGo fn seen (idx + 1)
  else .ok ()
termination_by fn.insns.size - idx

/-- Executable amd64 vector CFG safety checker matching `cmd/goblin-optcheck/main.go`. -/
def checkVectorCFG (fn : Amd64FuncCFG) : Except VectorHazard Unit :=
  let n := fn.insns.size
  if hPos : 0 < n then
    if !fn.isAVX512Name && checkUnguardedEVEX fn then
      .error (VectorHazard.mk fn.name fn.insns[0].addr .unguardedEVEX fn.name)
    else
      let initSeen := (Array.replicate (2 * n) false).set 0 true (by simp; omega)
      match bfsLoop fn (4 * n + 2) initSeen [(0, false)] with
      | none =>
        .error (VectorHazard.mk fn.name fn.insns[0].addr (.internalError "CFG BFS fuel exhausted") "")
      | some seen =>
        checkReachableHazardsGo fn seen 0
  else
    .ok ()

/-! ### Soundness Proofs Over Operational CFG Semantics -/

private theorem array_getElem?_set (a : Array Bool) (i j : Nat) (v : Bool) (hi : i < a.size) :
    (a.set i v hi)[j]? = if i = j then some v else a[j]? := by
  by_cases hj : j < a.size
  · have hj' : j < (a.set i v hi).size := by simpa using hj
    rw [Array.getElem?_eq_getElem hj', Array.getElem?_eq_getElem hj, Array.getElem_set]
    split <;> simp
  · have hne : i ≠ j := by omega
    have hj' : ¬ j < (a.set i v hi).size := by simpa using hj
    rw [Array.getElem?_eq_none (Nat.le_of_not_lt hj'),
        Array.getElem?_eq_none (Nat.le_of_not_lt hj)]
    simp [hne]

private theorem scanEVEX_false_imp {fn : Amd64FuncCFG} {seen : Array Bool} {start i : Nat}
    (hScan : scanEVEX fn seen start = false) (hLe : start ≤ i)
    (hi : i < fn.insns.size) (hTrue : seen[i]? = some true) :
    fn.insns[i].isEVEX = false := by
  unfold scanEVEX at hScan
  split at hScan
  · next hStart =>
    simp only [Bool.or_eq_false_iff] at hScan
    by_cases heq : start = i
    · subst heq
      have hHit := hScan.1
      simp only [hTrue, beq_self_eq_true, Bool.true_and] at hHit
      exact hHit
    · have hLt : start + 1 ≤ i := by omega
      exact scanEVEX_false_imp hScan.2 hLt hi hTrue
  · next hNot => omega
termination_by fn.insns.size - start

private theorem pushMany_props (n : Nat) (succs : List (Nat × Bool))
    (seen : Array Bool) (queue : List (Nat × Bool))
    (hSz : seen.size = 2 * n)
    (hQ : ∀ idx dirty, (idx, dirty) ∈ queue →
      idx < n ∧ seen[stateKey idx dirty]? = some true)
    (hSuccs : ∀ idx' dirty', (idx', dirty') ∈ succs → idx' < n) :
    let res := pushMany succs seen queue
    res.1.size = 2 * n ∧
    (∀ idx dirty, (idx, dirty) ∈ res.2 →
      idx < n ∧ res.1[stateKey idx dirty]? = some true) ∧
    (∀ st ∈ queue, st ∈ res.2) ∧
    (∀ k : Nat, seen[k]? = some true → res.1[k]? = some true) ∧
    (∀ idx' dirty', (idx', dirty') ∈ succs → res.1[stateKey idx' dirty']? = some true) ∧
    (∀ idx dirty, idx < n → res.1[stateKey idx dirty]? = some true →
      seen[stateKey idx dirty]? = some true ∨ (idx, dirty) ∈ res.2) := by
  induction succs generalizing seen queue with
  | nil =>
    dsimp only [pushMany]
    refine ⟨hSz, hQ, fun _ h => h, fun _ h => h, ?_, ?_⟩
    · intro _ _ h; cases h
    · intro _ _ _ h; exact Or.inl h
  | cons hd tl ih =>
    simp only [pushMany, List.foldl_cons]
    have hHdLt : hd.1 < n := hSuccs hd.1 hd.2 (List.Mem.head tl)
    have hKlt : stateKey hd.1 hd.2 < seen.size := by rw [hSz]; exact stateKey_lt hd.2 hHdLt
    have hTlSuccs : ∀ idx' dirty', (idx', dirty') ∈ tl → idx' < n :=
      fun idx' dirty' h => hSuccs idx' dirty' (List.Mem.tail hd h)
    by_cases hAlready : seen[stateKey hd.1 hd.2]
    · simp only [hKlt, ↓reduceDIte, hAlready, ↓reduceIte]
      have res := ih seen queue hSz hQ hTlSuccs
      refine ⟨res.1, res.2.1, res.2.2.1, res.2.2.2.1, ?_, res.2.2.2.2.2⟩
      intro idx' dirty' hMem
      cases hMem with
      | head =>
        apply res.2.2.2.1
        rw [Array.getElem?_eq_getElem hKlt, hAlready]
      | tail _ hMemTl => exact res.2.2.2.2.1 idx' dirty' hMemTl
    · simp only [hKlt, ↓reduceDIte, hAlready, Bool.false_eq_true, ↓reduceIte]
      let seen1 := seen.set (stateKey hd.1 hd.2) true hKlt
      let queue1 := hd :: queue
      have hSz1 : seen1.size = 2 * n := by simp [seen1, hSz]
      have hQ1 : ∀ idx dirty, (idx, dirty) ∈ queue1 →
          idx < n ∧ seen1[stateKey idx dirty]? = some true := by
        intro idx dirty h
        dsimp only [queue1] at h
        cases h with
        | head =>
          refine ⟨hHdLt, ?_⟩
          simp only [seen1, array_getElem?_set, ↓reduceIte]
        | tail _ hMem =>
          rcases hQ idx dirty hMem with ⟨hi, hv⟩
          refine ⟨hi, ?_⟩
          simp only [seen1, array_getElem?_set]
          split <;> simp [hv]
      have res := ih seen1 queue1 hSz1 hQ1 hTlSuccs
      refine ⟨res.1, res.2.1, ?_, ?_, ?_, ?_⟩
      · intro st hSt
        exact res.2.2.1 st (List.Mem.tail hd hSt)
      · intro k hv
        apply res.2.2.2.1
        simp only [seen1, array_getElem?_set]
        split <;> simp [hv]
      · intro idx' dirty' hMem
        cases hMem with
        | head =>
          apply res.2.2.2.1
          simp only [seen1, array_getElem?_set, ↓reduceIte]
        | tail _ hMemTl =>
          exact res.2.2.2.2.1 idx' dirty' hMemTl
      · intro idx dirty hIdx hV'
        rcases res.2.2.2.2.2 idx dirty hIdx hV' with hV1 | hInQ'
        · simp only [seen1, array_getElem?_set] at hV1
          split at hV1
          · next heq =>
            have hPair := stateKey_inj heq
            have hEqPair : (idx, dirty) = hd := Prod.ext hPair.1.symm hPair.2.symm
            rw [hEqPair]
            exact Or.inr (res.2.2.1 hd (List.Mem.head queue))
          · exact Or.inl hV1
        · exact Or.inr hInQ'

private theorem cfgSucc_lt (fn : Amd64FuncCFG) (idx : Nat) (dirty : Bool)
    (idx' : Nat) (dirty' : Bool) (hMem : (idx', dirty') ∈ cfgSucc fn idx dirty) :
    idx' < fn.insns.size := by
  unfold cfgSucc at hMem
  dsimp only at hMem
  split at hMem
  · split at hMem
    · simp at hMem
    · split at hMem
      · split at hMem
        · simp at hMem
        · split at hMem
          · simp at hMem; omega
          · simp at hMem
      · split at hMem
        · split at hMem
          · split at hMem
            · simp at hMem; omega
            · simp at hMem
          · simp at hMem
        · split at hMem
          · split at hMem
            · split at hMem
              · simp only [List.mem_cons] at hMem
                rcases hMem with hEq | hFall
                · cases hEq; assumption
                · split at hFall
                  · simp at hFall; omega
                  · simp at hFall
              · split at hMem
                · simp at hMem; omega
                · simp at hMem
            · split at hMem
              · simp at hMem; omega
              · simp at hMem
          · split at hMem
            · simp at hMem; omega
            · simp at hMem
  · simp at hMem

private def BfsInv (fn : Amd64FuncCFG) (seen : Array Bool) (queue : List (Nat × Bool)) : Prop :=
  seen.size = 2 * fn.insns.size ∧
  (∀ idx dirty, (idx, dirty) ∈ queue →
    idx < fn.insns.size ∧ seen[stateKey idx dirty]? = some true) ∧
  (0 < fn.insns.size → seen[0]? = some true) ∧
  (∀ idx dirty, idx < fn.insns.size → seen[stateKey idx dirty]? = some true →
    (idx, dirty) ∈ queue ∨
    ∀ idx' dirty', (idx', dirty') ∈ cfgSucc fn idx dirty → seen[stateKey idx' dirty']? = some true)

private theorem bfsLoop_inv (fn : Amd64FuncCFG) (fuel : Nat) (seen : Array Bool)
    (queue : List (Nat × Bool)) (finalSeen : Array Bool)
    (hInv : BfsInv fn seen queue)
    (hEq : bfsLoop fn fuel seen queue = some finalSeen) :
    BfsInv fn finalSeen [] := by
  induction fuel generalizing seen queue with
  | zero =>
    cases queue with
    | nil =>
      simp [bfsLoop] at hEq
      subst hEq
      exact hInv
    | cons _ _ => simp [bfsLoop] at hEq
  | succ fuel' ih =>
    cases queue with
    | nil =>
      simp [bfsLoop] at hEq
      subst hEq
      exact hInv
    | cons st rest =>
      simp only [bfsLoop] at hEq
      rcases st with ⟨idx, dirty⟩
      have hQRest : ∀ i d, (i, d) ∈ rest → i < fn.insns.size ∧ seen[stateKey i d]? = some true :=
        fun i d h => hInv.2.1 i d (List.Mem.tail (idx, dirty) h)
      have hSuccs : ∀ i' d', (i', d') ∈ cfgSucc fn idx dirty → i' < fn.insns.size :=
        fun i' d' h => cfgSucc_lt fn idx dirty i' d' h
      have hp := pushMany_props fn.insns.size (cfgSucc fn idx dirty) seen rest
        hInv.1 hQRest hSuccs
      apply ih _ _ ?_ hEq
      refine ⟨hp.1, hp.2.1, ?_, ?_⟩
      · intro hPos
        exact hp.2.2.2.1 0 (hInv.2.2.1 hPos)
      · intro i d hi hv'
        rcases hp.2.2.2.2.2 i d hi hv' with hv | hInQ'
        · rcases hInv.2.2.2 i d hi hv with hMem | hAll
          · cases hMem with
            | head =>
              right
              intro i' d' hSucc
              exact hp.2.2.2.2.1 i' d' hSucc
            | tail _ hInRest =>
              exact Or.inl (hp.2.2.1 (i, d) hInRest)
          · right
            intro i' d' hSucc
            exact hp.2.2.2.1 (stateKey i' d') (hAll i' d' hSucc)
        · exact Or.inl hInQ'

private theorem bfsLoop_covers_reachable (fn : Amd64FuncCFG) (fuel : Nat) (seen : Array Bool)
    (queue : List (Nat × Bool)) (finalSeen : Array Bool)
    (hInv : BfsInv fn seen queue)
    (hEq : bfsLoop fn fuel seen queue = some finalSeen)
    {idx : Nat} {dirty : Bool} (hReach : CfgReachable fn idx dirty) :
    idx < fn.insns.size ∧ finalSeen[stateKey idx dirty]? = some true := by
  have hFinal := bfsLoop_inv fn fuel seen queue finalSeen hInv hEq
  induction hReach with
  | entry hPos =>
    refine ⟨hPos, ?_⟩
    simpa [stateKey] using hFinal.2.2.1 hPos
  | step _ hStep ih =>
    rcases ih with ⟨hi, hv⟩
    cases hStep with
    | mk _ hMem =>
      refine ⟨cfgSucc_lt fn _ _ _ _ hMem, ?_⟩
      rcases hFinal.2.2.2 _ _ hi hv with hInNil | hAll
      · cases hInNil
      · exact hAll _ _ hMem

private theorem unguardedPushMany_props (n : Nat) (succs : List Nat)
    (seen : Array Bool) (queue : List Nat)
    (hSz : seen.size = n)
    (hQ : ∀ idx, idx ∈ queue → idx < n ∧ seen[idx]? = some true)
    (hSuccs : ∀ idx', idx' ∈ succs → idx' < n) :
    let res := unguardedPushMany succs seen queue
    res.1.size = n ∧
    (∀ idx, idx ∈ res.2 → idx < n ∧ res.1[idx]? = some true) ∧
    (∀ idx ∈ queue, idx ∈ res.2) ∧
    (∀ k : Nat, seen[k]? = some true → res.1[k]? = some true) ∧
    (∀ idx', idx' ∈ succs → res.1[idx']? = some true) ∧
    (∀ idx, idx < n → res.1[idx]? = some true → seen[idx]? = some true ∨ idx ∈ res.2) := by
  induction succs generalizing seen queue with
  | nil =>
    dsimp only [unguardedPushMany]
    refine ⟨hSz, hQ, fun _ h => h, fun _ h => h, ?_, ?_⟩
    · intro _ h; cases h
    · intro _ _ h; exact Or.inl h
  | cons hd tl ih =>
    simp only [unguardedPushMany, List.foldl_cons]
    have hHdLt : hd < n := hSuccs hd (List.Mem.head tl)
    have hKlt : hd < seen.size := by rw [hSz]; exact hHdLt
    have hTlSuccs : ∀ idx', idx' ∈ tl → idx' < n :=
      fun idx' h => hSuccs idx' (List.Mem.tail hd h)
    by_cases hAlready : seen[hd]
    · simp only [hKlt, ↓reduceDIte, hAlready, ↓reduceIte]
      have res := ih seen queue hSz hQ hTlSuccs
      refine ⟨res.1, res.2.1, res.2.2.1, res.2.2.2.1, ?_, res.2.2.2.2.2⟩
      intro idx' hMem
      cases hMem with
      | head =>
        apply res.2.2.2.1
        rw [Array.getElem?_eq_getElem hKlt, hAlready]
      | tail _ hMemTl => exact res.2.2.2.2.1 idx' hMemTl
    · simp only [hKlt, ↓reduceDIte, hAlready, Bool.false_eq_true, ↓reduceIte]
      let seen1 := seen.set hd true hKlt
      let queue1 := hd :: queue
      have hSz1 : seen1.size = n := by simp [seen1, hSz]
      have hQ1 : ∀ idx, idx ∈ queue1 → idx < n ∧ seen1[idx]? = some true := by
        intro idx h
        dsimp only [queue1] at h
        cases h with
        | head =>
          refine ⟨hHdLt, ?_⟩
          simp only [seen1, array_getElem?_set, ↓reduceIte]
        | tail _ hMem =>
          rcases hQ idx hMem with ⟨hi, hv⟩
          refine ⟨hi, ?_⟩
          simp only [seen1, array_getElem?_set]
          split <;> simp [hv]
      have res := ih seen1 queue1 hSz1 hQ1 hTlSuccs
      refine ⟨res.1, res.2.1, ?_, ?_, ?_, ?_⟩
      · intro idx hSt
        exact res.2.2.1 idx (List.Mem.tail hd hSt)
      · intro k hv
        apply res.2.2.2.1
        simp only [seen1, array_getElem?_set]
        split <;> simp [hv]
      · intro idx' hMem
        cases hMem with
        | head =>
          apply res.2.2.2.1
          simp only [seen1, array_getElem?_set, ↓reduceIte]
        | tail _ hMemTl =>
          exact res.2.2.2.2.1 idx' hMemTl
      · intro idx hIdx hV'
        rcases res.2.2.2.2.2 idx hIdx hV' with hV1 | hInQ'
        · simp only [seen1, array_getElem?_set] at hV1
          split at hV1
          · next heq =>
            rw [← heq]
            exact Or.inr (res.2.2.1 hd (List.Mem.head queue))
          · exact Or.inl hV1
        · exact Or.inr hInQ'

private theorem unguardedSucc_lt (fn : Amd64FuncCFG) (idx idx' : Nat)
    (hMem : idx' ∈ unguardedSucc fn idx) : idx' < fn.insns.size := by
  unfold unguardedSucc at hMem
  dsimp only at hMem
  split at hMem
  · split at hMem
    · simp at hMem
    · split at hMem
      · split at hMem
        · split at hMem
          · simp at hMem; omega
          · simp at hMem
        · simp at hMem
      · split at hMem
        · simp at hMem; omega
        · simp at hMem
  · simp at hMem

private def UnguardedBfsInv (fn : Amd64FuncCFG) (seen : Array Bool) (queue : List Nat) : Prop :=
  seen.size = fn.insns.size ∧
  (∀ idx, idx ∈ queue → idx < fn.insns.size ∧ seen[idx]? = some true) ∧
  (0 < fn.insns.size → seen[0]? = some true) ∧
  (∀ idx, idx < fn.insns.size → seen[idx]? = some true →
    idx ∈ queue ∨
    ∀ idx', idx' ∈ unguardedSucc fn idx → seen[idx']? = some true)

private theorem unguardedBfsLoop_inv (fn : Amd64FuncCFG) (fuel : Nat) (seen : Array Bool)
    (queue : List Nat) (finalSeen : Array Bool)
    (hInv : UnguardedBfsInv fn seen queue)
    (hEq : unguardedBfsLoop fn fuel seen queue = some finalSeen) :
    UnguardedBfsInv fn finalSeen [] := by
  induction fuel generalizing seen queue with
  | zero =>
    cases queue with
    | nil =>
      simp [unguardedBfsLoop] at hEq
      subst hEq
      exact hInv
    | cons _ _ => simp [unguardedBfsLoop] at hEq
  | succ fuel' ih =>
    cases queue with
    | nil =>
      simp [unguardedBfsLoop] at hEq
      subst hEq
      exact hInv
    | cons idx rest =>
      simp only [unguardedBfsLoop] at hEq
      have hQRest : ∀ i, i ∈ rest → i < fn.insns.size ∧ seen[i]? = some true :=
        fun i h => hInv.2.1 i (List.Mem.tail idx h)
      have hSuccs : ∀ i', i' ∈ unguardedSucc fn idx → i' < fn.insns.size :=
        fun i' h => unguardedSucc_lt fn idx i' h
      have hp := unguardedPushMany_props fn.insns.size (unguardedSucc fn idx) seen rest
        hInv.1 hQRest hSuccs
      apply ih _ _ ?_ hEq
      refine ⟨hp.1, hp.2.1, ?_, ?_⟩
      · intro hPos
        exact hp.2.2.2.1 0 (hInv.2.2.1 hPos)
      · intro i hi hv'
        rcases hp.2.2.2.2.2 i hi hv' with hv | hInQ'
        · rcases hInv.2.2.2 i hi hv with hMem | hAll
          · cases hMem with
            | head =>
              right
              intro i' hSucc
              exact hp.2.2.2.2.1 i' hSucc
            | tail _ hInRest =>
              exact Or.inl (hp.2.2.1 i hInRest)
          · right
            intro i' hSucc
            exact hp.2.2.2.1 i' (hAll i' hSucc)
        · exact Or.inl hInQ'

private theorem unguardedBfsLoop_covers_reachable (fn : Amd64FuncCFG) (fuel : Nat) (seen : Array Bool)
    (queue : List Nat) (finalSeen : Array Bool)
    (hInv : UnguardedBfsInv fn seen queue)
    (hEq : unguardedBfsLoop fn fuel seen queue = some finalSeen)
    {idx : Nat} (hReach : UnguardedReachable fn idx) :
    idx < fn.insns.size ∧ finalSeen[idx]? = some true := by
  have hFinal := unguardedBfsLoop_inv fn fuel seen queue finalSeen hInv hEq
  induction hReach with
  | entry hPos => exact ⟨hPos, hFinal.2.2.1 hPos⟩
  | step _ hStep ih =>
    rcases ih with ⟨hi, hv⟩
    cases hStep with
    | mk _ hMem =>
      refine ⟨unguardedSucc_lt fn _ _ hMem, ?_⟩
      rcases hFinal.2.2.2 _ hi hv with hInNil | hAll
      · cases hInNil
      · exact hAll _ hMem

private theorem checkReachableHazardsGo_sound (fn : Amd64FuncCFG) (seen : Array Bool)
    (start idx : Nat) (dirty : Bool)
    (hOk : checkReachableHazardsGo fn seen start = .ok ())
    (hLe : start ≤ idx) (hIdx : idx < fn.insns.size)
    (hv : seen[stateKey idx dirty]? = some true) :
    insnLegacySSEHazard fn.insns[idx] dirty = false ∧
    insnDirtyExitOrCallHazard fn.insns[idx] dirty = none := by
  unfold checkReachableHazardsGo at hOk
  split at hOk
  · next hStart =>
    by_cases heq : start = idx
    · subst heq
      cases hF : checkStateHazard fn seen start hStart false with
      | error e => simp [hF] at hOk
      | ok _ =>
        simp only [hF] at hOk
        cases hT : checkStateHazard fn seen start hStart true with
        | error e => simp [hT] at hOk
        | ok _ =>
          have hTarget : checkStateHazard fn seen start hStart dirty = .ok () := by
            cases dirty <;> simp [hF, hT]
          unfold checkStateHazard at hTarget
          dsimp only at hTarget
          simp only [hv, beq_self_eq_true, ↓reduceIte] at hTarget
          split at hTarget
          · simp at hTarget
          · next hNotSSE =>
            split at hTarget
            · simp at hTarget
            · next hNone =>
              exact ⟨by simpa using hNotSSE, hNone⟩
    · have hLt : start + 1 ≤ idx := by omega
      cases hF : checkStateHazard fn seen start hStart false with
      | error e => simp [hF] at hOk
      | ok _ =>
        simp only [hF] at hOk
        cases hT : checkStateHazard fn seen start hStart true with
        | error e => simp [hT] at hOk
        | ok _ =>
          simp only [hT] at hOk
          exact checkReachableHazardsGo_sound fn seen (start + 1) idx dirty hOk hLt hIdx hv
  · next hNot => omega
termination_by fn.insns.size - start

/-- Soundness theorem for `checkVectorCFG`:
If `checkVectorCFG fn = .ok ()`, then for every state `(idx, dirty)` reachable from `(0, false)`:
(a) no legacy SSE instruction executes while `dirty = true`,
(b) every `ret`/`retq`, non-morestack `call`/`callq`, and external `jmp` exits/calls with `nextDirty = false`, and
(c) if `!fn.isAVX512Name`, no EVEX instruction is reachable along any unguarded path from entry. -/
theorem checkVectorCFG_sound (fn : Amd64FuncCFG) (hOk : checkVectorCFG fn = .ok ()) :
    (∀ idx dirty, CfgReachable fn idx dirty → ∀ (hIdx : idx < fn.insns.size),
      insnLegacySSEHazard fn.insns[idx] dirty = false ∧
      (fn.insns[idx].isRet = true → nextDirty dirty fn.insns[idx] = false) ∧
      (fn.insns[idx].isCall = true → fn.insns[idx].isNoreturnPanic = false →
        fn.insns[idx].isMorestackCall = false → nextDirty dirty fn.insns[idx] = false) ∧
      (fn.insns[idx].isUncondJmp = true → fn.insns[idx].jmpTargetIdx = none →
        fn.insns[idx].hasHexJmpTarget = true → fn.insns[idx].isMorestackCall = false →
        nextDirty dirty fn.insns[idx] = false)) ∧
    (fn.isAVX512Name = false →
      checkUnguardedEVEX fn = false ∧
      ∀ idx, UnguardedReachable fn idx → ∀ (hIdx : idx < fn.insns.size),
        fn.insns[idx].isEVEX = false) := by
  unfold checkVectorCFG at hOk
  dsimp only at hOk
  split at hOk
  · next hPos =>
    have hUnguardedFalse : fn.isAVX512Name = false → checkUnguardedEVEX fn = false := by
      intro hNot512
      by_cases hU : checkUnguardedEVEX fn = true
      · simp [hNot512, hU] at hOk
      · simp only [Bool.not_eq_true] at hU; exact hU
    have hNoEvexErr : (!fn.isAVX512Name && checkUnguardedEVEX fn) = false := by
      by_cases hG : (!fn.isAVX512Name && checkUnguardedEVEX fn) = true
      · simp [hG] at hOk
      · simp only [Bool.not_eq_true] at hG; exact hG
    simp only [hNoEvexErr, Bool.false_eq_true, ↓reduceIte] at hOk
    let initSeen := (Array.replicate (2 * fn.insns.size) false).set 0 true (by simp; omega)
    cases hBfs : bfsLoop fn (4 * fn.insns.size + 2)
        ((Array.replicate (2 * fn.insns.size) false).set 0 true (by simp; omega)) [(0, false)] with
    | none => simp [hBfs] at hOk
    | some seen =>
      simp only [hBfs] at hOk
      have hBfs' : bfsLoop fn (4 * fn.insns.size + 2) initSeen [(0, false)] = some seen := hBfs
      have hInitInv : BfsInv fn initSeen [(0, false)] := by
        refine ⟨by simp [initSeen], ?_, ?_, ?_⟩
        · intro idx dirty hMem
          cases hMem with
          | head =>
            refine ⟨hPos, ?_⟩
            simp [initSeen, stateKey]
          | tail _ hNil => cases hNil
        · intro _
          simp [initSeen]
        · intro idx dirty _ hv
          simp only [initSeen, array_getElem?_set] at hv
          split at hv
          · next heq =>
            have hPair : idx = 0 ∧ dirty = false := by
              cases dirty <;> simp [stateKey] at heq
              exact ⟨by omega, rfl⟩
            rw [hPair.1, hPair.2]
            exact Or.inl (List.Mem.head [])
          · by_cases hk : stateKey idx dirty < 2 * fn.insns.size
            · simp [hk] at hv
            · have hk' : (Array.replicate (2 * fn.insns.size) false).size ≤ stateKey idx dirty := by
                simpa using Nat.le_of_not_lt hk
              rw [Array.getElem?_eq_none hk'] at hv
              cases hv
      have hMain : ∀ idx dirty (hReach : CfgReachable fn idx dirty) (hIdx : idx < fn.insns.size),
          insnLegacySSEHazard fn.insns[idx] dirty = false ∧
          (fn.insns[idx].isRet = true → nextDirty dirty fn.insns[idx] = false) ∧
          (fn.insns[idx].isCall = true → fn.insns[idx].isNoreturnPanic = false →
            fn.insns[idx].isMorestackCall = false → nextDirty dirty fn.insns[idx] = false) ∧
          (fn.insns[idx].isUncondJmp = true → fn.insns[idx].jmpTargetIdx = none →
            fn.insns[idx].hasHexJmpTarget = true → fn.insns[idx].isMorestackCall = false →
            nextDirty dirty fn.insns[idx] = false) := by
        intro idx dirty hReach hIdx
        have hCov := bfsLoop_covers_reachable fn _ initSeen [(0, false)] seen hInitInv hBfs' hReach
        have hGo := checkReachableHazardsGo_sound fn seen 0 idx dirty hOk (Nat.zero_le idx) hIdx hCov.2
        refine ⟨hGo.1, ?_, ?_, ?_⟩
        · intro hRet
          have hNone := hGo.2
          unfold insnDirtyExitOrCallHazard at hNone
          by_cases hNd : nextDirty dirty fn.insns[idx] = true
          · simp [hRet, hNd] at hNone
          · cases h : nextDirty dirty fn.insns[idx] <;> simp_all
        · intro hCall hNotP hNotM
          have hNone := hGo.2
          unfold insnDirtyExitOrCallHazard at hNone
          by_cases hNd : nextDirty dirty fn.insns[idx] = true
          · simp only [hCall, hNotP, hNotM, hNd, Bool.not_false, Bool.and_true, ↓reduceIte] at hNone
            split at hNone <;> simp at hNone
          · cases h : nextDirty dirty fn.insns[idx] <;> simp_all
        · intro hUncond hJmpNone hHasHex hNotM
          have hNone := hGo.2
          unfold insnDirtyExitOrCallHazard at hNone
          by_cases hNd : nextDirty dirty fn.insns[idx] = true
          · simp only [hUncond, hJmpNone, hHasHex, hNotM, hNd, Option.isNone_none,
              Bool.not_false, Bool.and_true] at hNone
            split at hNone
            · simp at hNone
            · split at hNone
              · simp at hNone
              · simp at hNone
          · cases h : nextDirty dirty fn.insns[idx] <;> simp_all
      refine ⟨hMain, ?_⟩
      intro hNot512
      have hUFalse := hUnguardedFalse hNot512
      refine ⟨hUFalse, ?_⟩
      intro idx hUReach hIdx
      unfold checkUnguardedEVEX at hUFalse
      simp only [hPos, ↓reduceDIte] at hUFalse
      let initU := (Array.replicate fn.insns.size false).set 0 true (by simp [hPos])
      cases hUBfs : unguardedBfsLoop fn (2 * fn.insns.size + 2)
          ((Array.replicate fn.insns.size false).set 0 true (by simp [hPos])) [0] with
      | none => simp [hUBfs] at hUFalse
      | some uSeen =>
        simp only [hUBfs] at hUFalse
        have hUBfs' : unguardedBfsLoop fn (2 * fn.insns.size + 2) initU [0] = some uSeen := hUBfs
        have hUInv : UnguardedBfsInv fn initU [0] := by
          refine ⟨by simp [initU], ?_, ?_, ?_⟩
          · intro i hi
            cases hi with
            | head => exact ⟨hPos, by simp [initU]⟩
            | tail _ hNil => cases hNil
          · intro _; simp [initU]
          · intro i _ hv
            simp only [initU, array_getElem?_set] at hv
            split at hv
            · next heq => subst heq; exact Or.inl (List.Mem.head [])
            · by_cases hk : i < fn.insns.size
              · simp [hk] at hv
              · have hk' : (Array.replicate fn.insns.size false).size ≤ i := by
                  simpa using Nat.le_of_not_lt hk
                rw [Array.getElem?_eq_none hk'] at hv
                cases hv
        have hUCov := unguardedBfsLoop_covers_reachable fn _ initU [0] uSeen hUInv hUBfs' hUReach
        exact scanEVEX_false_imp hUFalse (Nat.zero_le idx) hIdx hUCov.2
  · next hNotPos =>
    have hEmpty : fn.insns.size = 0 := Nat.eq_zero_of_not_pos hNotPos
    constructor
    · intro idx _ _ hIdx; omega
    · intro _; constructor
      · unfold checkUnguardedEVEX; simp [hNotPos]
      · intro idx _ hIdx; omega

end GoblinFormal.Track5C
