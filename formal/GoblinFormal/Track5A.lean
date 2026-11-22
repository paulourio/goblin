/-
GoblinFormal.Track5A: Stock GOARCH=arm64 binary decoder, machine semantics,
and bit-exact leaf equivalence proofs for presolveSplitmix64,
canonicalFloatBits, divCeilInt64, and gcdInt64.
-/

import GoblinFormal.Common.Basic
import Std.Tactic.BVDecide

namespace GoblinFormal.Track5A

/-- Machine execution status for decoded AArch64 programs. -/
inductive ExecStatus where
  | running
  | returned
  | trapMorestack
  | trapPanicBounds
  | trapPanicDivide
  | callExternal (target : String)
  deriving DecidableEq, Repr, Inhabited

/-- Register shift kind in AArch64 data-processing instructions. -/
inductive ShiftKind where
  | lsl
  | lsr
  | asr
  | ror
  deriving DecidableEq, Repr, Inhabited

/-- Extension mode for extended-register arithmetic and register-offset loads/stores. -/
inductive ExtKind where
  | uxtb
  | uxth
  | uxtw
  | uxtx
  | sxtb
  | sxth
  | sxtw
  | sxtx
  deriving DecidableEq, Repr, Inhabited

/-- Addressing index mode for load/store instructions. -/
inductive MemIndexMode where
  | offset
  | post
  | pre
  deriving DecidableEq, Repr, Inhabited

/-- AArch64 condition codes (4-bit encoding 0..15). -/
inductive Arm64Cond where
  | eq | ne | cs | cc | mi | pl | vs | vc
  | hi | ls | ge | lt | gt | le | al | nv
  deriving DecidableEq, Repr, Inhabited

/-- Processor condition flags (N, Z, C, V). -/
structure NZCV where
  n : Bool := false
  z : Bool := false
  c : Bool := false
  v : Bool := false
  deriving DecidableEq, Repr, Inhabited

/-- Decoded 32-bit AArch64 instruction AST covering Go 1.27 codegen. -/
inductive Arm64Insn where
  | nop
  | ret (rn : BitVec 5)
  | b (imm26 : Int)
  | bl (imm26 : Int)
  | bCond (cond : Arm64Cond) (imm19 : Int)
  | cbz (is64 isNz : Bool) (rt : BitVec 5) (imm19 : Int)
  | tbz (isNz : Bool) (bit : Nat) (rt : BitVec 5) (imm14 : Int)
  | adrp (rd : BitVec 5) (pageImm : Int)
  | movWide (is64 : Bool) (opc : Nat) (rd : BitVec 5) (imm16 : Nat) (shift : Nat)
  | addSubImm (is64 isSub setFlags : Bool) (rd rn : BitVec 5) (imm : BitVec 64)
  | addSubReg (is64 isSub setFlags : Bool) (shift : ShiftKind) (imm6 : Nat)
      (rd rn rm : BitVec 5)
  | addSubExt (is64 isSub setFlags : Bool) (ext : ExtKind) (imm3 : Nat)
      (rd rn rm : BitVec 5)
  | logicalImm (is64 : Bool) (opc : Nat) (rd rn : BitVec 5) (imm : BitVec 64)
  | logicalReg (is64 inv : Bool) (opc : Nat) (shift : ShiftKind) (imm6 : Nat)
      (rd rn rm : BitVec 5)
  | bitfield (is64 isSigned isInsert : Bool) (rd rn : BitVec 5) (immr imms : Nat)
  | condSel (is64 isInc : Bool) (cond : Arm64Cond) (rd rn rm : BitVec 5)
  | div (is64 isSigned : Bool) (rd rn rm : BitVec 5)
  | mul3 (is64 isSub : Bool) (rd rn rm ra : BitVec 5)
  | ldrStrPair (isLoad isFP : Bool) (mode : MemIndexMode) (imm : Int)
      (rt rt2 rn : BitVec 5)
  | ldrStrUImm (isLoad isFP : Bool) (sizeBytes : Nat) (imm : Nat) (rt rn : BitVec 5)
  | ldrStrImm (isLoad isFP : Bool) (sizeBytes : Nat) (mode : MemIndexMode) (imm9 : Int)
      (rt rn : BitVec 5)
  | ldrStrReg (isLoad isSigned isFP : Bool) (sizeBytes : Nat) (ext : ExtKind)
      (scale : Bool) (rt rn rm : BitVec 5)
  | fcmpD (isZero : Bool) (rn rm : BitVec 5)
  | fpUnaryD (op : Nat) (rd rn : BitVec 5)
  | fpBinaryD (op : Nat) (rd rn rm : BitVec 5)
  | fpIntConv (toInt isSigned gprToFp : Bool) (rd rn : BitVec 5)
  | unknown (word : UInt32)
  deriving DecidableEq, Repr, Inhabited

/-- Sign-extend the lowest `bits` bits of a `Nat` to `Int`. -/
def signExtendNat (val bits : Nat) : Int :=
  if bits == 0 then 0
  else
    let mask := (1 <<< bits) - 1
    let v := val &&& mask
    let signBit := 1 <<< (bits - 1)
    if v >= signBit then (v : Int) - ((1 <<< bits : Nat) : Int) else (v : Int)

/-- Decode the 4-bit AArch64 condition field. -/
def decodeCond (c : Nat) : Arm64Cond :=
  match c &&& 0xf with
  | 0 => .eq | 1 => .ne | 2 => .cs | 3 => .cc
  | 4 => .mi | 5 => .pl | 6 => .vs | 7 => .vc
  | 8 => .hi | 9 => .ls | 10 => .ge | 11 => .lt
  | 12 => .gt | 13 => .le | 14 => .al | _ => .nv

/-- Decode the 2-bit shift field. -/
def decodeShift (s : Nat) : ShiftKind :=
  match s &&& 0x3 with
  | 0 => .lsl | 1 => .lsr | 2 => .asr | _ => .ror

/-- Decode the 3-bit extend option field. -/
def decodeExt (opt : Nat) : ExtKind :=
  match opt &&& 0x7 with
  | 0 => .uxtb | 1 => .uxth | 2 => .uxtw | 3 => .uxtx
  | 4 => .sxtb | 5 => .sxth | 6 => .sxtw | _ => .sxtx

private def highestSetBit6 (v : Nat) : Option Nat :=
  if (v >>> 6) &&& 1 == 1 then some 6
  else if (v >>> 5) &&& 1 == 1 then some 5
  else if (v >>> 4) &&& 1 == 1 then some 4
  else if (v >>> 3) &&& 1 == 1 then some 3
  else if (v >>> 2) &&& 1 == 1 then some 2
  else if (v >>> 1) &&& 1 == 1 then some 1
  else none

private def replicateMask (elem esize total : Nat) : Nat :=
  if esize == 0 then 0
  else
    let count := total / esize
    let rec go (i acc : Nat) : Nat :=
      match i with
      | 0 => acc
      | i' + 1 => go i' ((acc <<< esize) ||| elem)
    go count 0

/-- Decode AArch64 `DecodeBitMasks` immediate for logical instructions. -/
def decodeBitMasks (nBit imms immr regSize : Nat) : Option (BitVec 64) :=
  let v := ((nBit &&& 1) <<< 6) ||| (63 - (imms &&& 63))
  match highestSetBit6 v with
  | none => none
  | some len =>
    let esize := 1 <<< len
    if esize > regSize then none
    else
      let levels := esize - 1
      let s := imms &&& levels
      let r := immr &&& levels
      if s == levels then none
      else
        let welem := (1 <<< (s + 1)) - 1
        let rMod := r % esize
        let rotated :=
          if rMod == 0 then welem
          else
            let low := welem >>> rMod
            let esizeMask := (1 <<< esize) - 1
            let high := (welem <<< (esize - rMod)) &&& esizeMask
            low ||| high
        let full := replicateMask rotated esize regSize
        let outMask := if regSize == 32 then full &&& 0xffffffff else full
        some (BitVec.ofNat 64 outMask)

/-- Decode a 32-bit little-endian AArch64 machine word into `Arm64Insn`. -/
def decodeArm64Word (word : UInt32) : Arm64Insn :=
  let w : Nat := word.toNat
  let rd : BitVec 5 := BitVec.ofNat 5 (w &&& 0x1f)
  let rn : BitVec 5 := BitVec.ofNat 5 ((w >>> 5) &&& 0x1f)
  let rm : BitVec 5 := BitVec.ofNat 5 ((w >>> 16) &&& 0x1f)
  let is64 : Bool := ((w >>> 31) &&& 1) == 1
  if w == 0xd503201f then
    .nop
  else if (w &&& 0xfffffc1f) == 0xd65f0000 then
    .ret rn
  else if (w &&& 0x7c000000) == 0x14000000 then
    let imm26 := signExtendNat (w &&& 0x3ffffff) 26
    if is64 then .bl imm26 else .b imm26
  else if (w &&& 0xff000010) == 0x54000000 then
    let imm19 := signExtendNat ((w >>> 5) &&& 0x7ffff) 19
    .bCond (decodeCond (w &&& 0xf)) imm19
  else if (w &&& 0x7e000000) == 0x34000000 then
    let isNz := ((w >>> 24) &&& 1) == 1
    let imm19 := signExtendNat ((w >>> 5) &&& 0x7ffff) 19
    .cbz is64 isNz rd imm19
  else if (w &&& 0x7e000000) == 0x36000000 then
    let isNz := ((w >>> 24) &&& 1) == 1
    let bit := (((w >>> 31) &&& 1) <<< 5) ||| ((w >>> 19) &&& 0x1f)
    let imm14 := signExtendNat ((w >>> 5) &&& 0x3fff) 14
    .tbz isNz bit rd imm14
  else if (w &&& 0x9f000000) == 0x90000000 then
    let immlo := (w >>> 29) &&& 0x3
    let immhi := (w >>> 5) &&& 0x7ffff
    let imm21 := signExtendNat ((immhi <<< 2) ||| immlo) 21
    .adrp rd (imm21 * 4096)
  else if (w &&& 0x1f800000) == 0x12800000 then
    let opc := (w >>> 29) &&& 0x3
    let hw := (w >>> 21) &&& 0x3
    let imm16 := (w >>> 5) &&& 0xffff
    if opc == 0 || opc == 2 || opc == 3 then
      .movWide is64 opc rd imm16 (hw * 16)
    else .unknown word
  else if (w &&& 0x1f800000) == 0x11000000 then
    let isSub := ((w >>> 30) &&& 1) == 1
    let setFlags := ((w >>> 29) &&& 1) == 1
    let sh := (w >>> 22) &&& 0x3
    let imm12 := (w >>> 10) &&& 0xfff
    let immVal := if sh == 1 then imm12 <<< 12 else imm12
    .addSubImm is64 isSub setFlags rd rn (BitVec.ofNat 64 immVal)
  else if (w &&& 0x1f800000) == 0x12000000 then
    let opc := (w >>> 29) &&& 0x3
    let nBit := (w >>> 22) &&& 1
    let immr := (w >>> 16) &&& 0x3f
    let imms := (w >>> 10) &&& 0x3f
    match decodeBitMasks nBit imms immr (if is64 then 64 else 32) with
    | some mask => .logicalImm is64 opc rd rn mask
    | none => .unknown word
  else if (w &&& 0x1f800000) == 0x13000000 then
    let opc := (w >>> 29) &&& 0x3
    let immr := (w >>> 16) &&& 0x3f
    let imms := (w >>> 10) &&& 0x3f
    if opc == 0 then .bitfield is64 true false rd rn immr imms
    else if opc == 1 then .bitfield is64 false true rd rn immr imms
    else if opc == 2 then .bitfield is64 false false rd rn immr imms
    else .unknown word
  else if (w &&& 0x1f000000) == 0x0a000000 then
    let opc := (w >>> 29) &&& 0x3
    let shift := decodeShift ((w >>> 22) &&& 0x3)
    let inv := ((w >>> 21) &&& 1) == 1
    let imm6 := (w >>> 10) &&& 0x3f
    .logicalReg is64 inv opc shift imm6 rd rn rm
  else if (w &&& 0x1f200000) == 0x0b000000 then
    let isSub := ((w >>> 30) &&& 1) == 1
    let setFlags := ((w >>> 29) &&& 1) == 1
    let shift := decodeShift ((w >>> 22) &&& 0x3)
    let imm6 := (w >>> 10) &&& 0x3f
    .addSubReg is64 isSub setFlags shift imm6 rd rn rm
  else if (w &&& 0x1fe00000) == 0x0b200000 then
    let isSub := ((w >>> 30) &&& 1) == 1
    let setFlags := ((w >>> 29) &&& 1) == 1
    let ext := decodeExt ((w >>> 13) &&& 0x7)
    let imm3 := (w >>> 10) &&& 0x7
    .addSubExt is64 isSub setFlags ext imm3 rd rn rm
  else if (w &&& 0x1fe00800) == 0x1a800000 then
    let op := (w >>> 30) &&& 1
    let s := (w >>> 29) &&& 1
    let o2 := (w >>> 10) &&& 1
    let cond := decodeCond ((w >>> 12) &&& 0xf)
    if op == 0 && s == 0 then
      .condSel is64 (o2 == 1) cond rd rn rm
    else .unknown word
  else if (w &&& 0x5fe00000) == 0x1ac00000 then
    let opcode := (w >>> 10) &&& 0x3f
    if opcode == 3 then .div is64 true rd rn rm
    else if opcode == 2 then .div is64 false rd rn rm
    else .unknown word
  else if (w &&& 0x7fe00000) == 0x1b000000 then
    let o0 := ((w >>> 15) &&& 1) == 1
    let ra : BitVec 5 := BitVec.ofNat 5 ((w >>> 10) &&& 0x1f)
    .mul3 is64 o0 rd rn rm ra
  else if (w &&& 0x3a000000) == 0x28000000 then
    let opc := (w >>> 30) &&& 0x3
    let v := (w >>> 26) &&& 1
    let idxMode := (w >>> 23) &&& 0x3
    let isLoad := ((w >>> 22) &&& 1) == 1
    let imm7 := signExtendNat ((w >>> 15) &&& 0x7f) 7
    let rt2 : BitVec 5 := BitVec.ofNat 5 ((w >>> 10) &&& 0x1f)
    let mode :=
      if idxMode == 1 then MemIndexMode.post
      else if idxMode == 3 then MemIndexMode.pre
      else MemIndexMode.offset
    if v == 1 && opc == 1 && (idxMode == 1 || idxMode == 2 || idxMode == 3) then
      .ldrStrPair isLoad true mode (imm7 * 8) rd rt2 rn
    else if v == 0 && opc == 2 && (idxMode == 1 || idxMode == 2 || idxMode == 3) then
      .ldrStrPair isLoad false mode (imm7 * 8) rd rt2 rn
    else .unknown word
  else if (w &&& 0x3b000000) == 0x39000000 then
    let size := (w >>> 30) &&& 0x3
    let v := (w >>> 26) &&& 1
    let opc := (w >>> 22) &&& 0x3
    let imm12 := (w >>> 10) &&& 0xfff
    if v == 1 && size == 3 && (opc == 0 || opc == 1) then
      .ldrStrUImm (opc == 1) true 8 (imm12 * 8) rd rn
    else if v == 0 && size == 3 && (opc == 0 || opc == 1) then
      .ldrStrUImm (opc == 1) false 8 (imm12 * 8) rd rn
    else if v == 0 && size == 0 && (opc == 0 || opc == 1) then
      .ldrStrUImm (opc == 1) false 1 imm12 rd rn
    else .unknown word
  else if (w &&& 0x3b200000) == 0x38000000 &&
      (((w >>> 10) &&& 0x3) == 0 || ((w >>> 10) &&& 0x3) == 1 || ((w >>> 10) &&& 0x3) == 3) then
    let size := (w >>> 30) &&& 0x3
    let v := (w >>> 26) &&& 1
    let opc := (w >>> 22) &&& 0x3
    let imm9 := signExtendNat ((w >>> 12) &&& 0x1ff) 9
    let idx := (w >>> 10) &&& 0x3
    let mode :=
      if idx == 1 then MemIndexMode.post
      else if idx == 3 then MemIndexMode.pre
      else MemIndexMode.offset
    if v == 0 && size == 3 && (opc == 0 || opc == 1) then
      .ldrStrImm (opc == 1) false 8 mode imm9 rd rn
    else .unknown word
  else if (w &&& 0x3b200c00) == 0x38200800 then
    let size := (w >>> 30) &&& 0x3
    let v := (w >>> 26) &&& 1
    let opc := (w >>> 22) &&& 0x3
    let ext := decodeExt ((w >>> 13) &&& 0x7)
    let scale := ((w >>> 12) &&& 1) == 1
    if v == 1 && size == 3 && (opc == 0 || opc == 1) then
      .ldrStrReg (opc == 1) false true 8 ext scale rd rn rm
    else if v == 0 && size == 3 && (opc == 0 || opc == 1) then
      .ldrStrReg (opc == 1) false false 8 ext scale rd rn rm
    else if v == 0 && size == 0 && (opc == 0 || opc == 1 || opc == 2) then
      .ldrStrReg (opc != 0) (opc == 2) false 1 ext scale rd rn rm
    else .unknown word
  else if (w &&& 0xffe0fc17) == 0x1e602000 then
    let isZero := ((w >>> 3) &&& 1) == 1
    .fcmpD isZero rn rm
  else if (w &&& 0xffe07c00) == 0x1e604000 then
    let opcode := (w >>> 15) &&& 0x3f
    .fpUnaryD opcode rd rn
  else if (w &&& 0xffe00c00) == 0x1e600800 then
    let opcode := (w >>> 12) &&& 0xf
    .fpBinaryD opcode rd rn rm
  else if (w &&& 0x7fe0fc00) == 0x1e600000 then
    let rmode := (w >>> 19) &&& 0x3
    let opcode := (w >>> 16) &&& 0x7
    if is64 && rmode == 3 && opcode == 0 then .fpIntConv true true false rd rn
    else if is64 && rmode == 0 && opcode == 6 then .fpIntConv false false false rd rn
    else if is64 && rmode == 0 && opcode == 7 then .fpIntConv false false true rd rn
    else if is64 && rmode == 0 && opcode == 2 then .fpIntConv false true true rd rn
    else .unknown word
  else
    .unknown word

/-- Decode an array of little-endian 32-bit AArch64 words via `List.map` for fast kernel reduction. -/
def decodeArm64Prog (words : Array UInt32) : Array Arm64Insn :=
  ⟨words.toList.map decodeArm64Word⟩

/-- AArch64 machine state for executing decoded ELF routines. -/
structure Arm64State where
  regs : BitVec 5 → BitVec 64
  sp : BitVec 64
  nzcv : NZCV
  fpRegs : BitVec 5 → BitVec 64
  gStackGuard0 : BitVec 64
  mem : BitVec 64 → BitVec 8
  pcIdx : Nat
  baseAddr : BitVec 64
  status : ExecStatus

/-- Read a 64-bit GPR (`X31` reads `0`). -/
@[inline] def readGpr (s : Arm64State) (r : BitVec 5) : BitVec 64 :=
  if r == 31#5 then 0#64 else s.regs r

/-- Read a 64-bit register where `X31` reads `SP`. -/
@[inline] def readGprSP (s : Arm64State) (r : BitVec 5) : BitVec 64 :=
  if r == 31#5 then s.sp else s.regs r

/-- Write a 64-bit GPR (`X31` writes are ignored). -/
@[inline] def writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) : Arm64State :=
  if r == 31#5 then s
  else { s with regs := fun r' => if r' == r then v else s.regs r' }

/-- Write a 64-bit register where `X31` updates `SP`. -/
@[inline] def writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) : Arm64State :=
  if r == 31#5 then { s with sp := v }
  else { s with regs := fun r' => if r' == r then v else s.regs r' }

/-- Write a 64-bit floating-point register `D0..D31`. -/
@[inline] def writeFp (s : Arm64State) (r : BitVec 5) (v : BitVec 64) : Arm64State :=
  { s with fpRegs := fun r' => if r' == r then v else s.fpRegs r' }

/-- Read an unsigned byte from memory. -/
@[inline] def readU8 (mem : BitVec 64 → BitVec 8) (addr : BitVec 64) : BitVec 8 :=
  mem addr

/-- Read a little-endian 64-bit word from byte memory. -/
def readU64 (mem : BitVec 64 → BitVec 8) (addr : BitVec 64) : BitVec 64 :=
  let b0 := (mem addr).zeroExtend 64
  let b1 := (mem (addr + 1#64)).zeroExtend 64 <<< 8
  let b2 := (mem (addr + 2#64)).zeroExtend 64 <<< 16
  let b3 := (mem (addr + 3#64)).zeroExtend 64 <<< 24
  let b4 := (mem (addr + 4#64)).zeroExtend 64 <<< 32
  let b5 := (mem (addr + 5#64)).zeroExtend 64 <<< 40
  let b6 := (mem (addr + 6#64)).zeroExtend 64 <<< 48
  let b7 := (mem (addr + 7#64)).zeroExtend 64 <<< 56
  b0 ||| b1 ||| b2 ||| b3 ||| b4 ||| b5 ||| b6 ||| b7

/-- Write an unsigned byte to memory. -/
@[inline] def writeU8 (mem : BitVec 64 → BitVec 8) (addr : BitVec 64) (v : BitVec 8) :
    BitVec 64 → BitVec 8 :=
  fun a => if a == addr then v else mem a

/-- Write a little-endian 64-bit word to byte memory. -/
def writeU64 (mem : BitVec 64 → BitVec 8) (addr : BitVec 64) (v : BitVec 64) :
    BitVec 64 → BitVec 8 :=
  let m0 := writeU8 mem addr (v.extractLsb' 0 8)
  let m1 := writeU8 m0 (addr + 1#64) (v.extractLsb' 8 8)
  let m2 := writeU8 m1 (addr + 2#64) (v.extractLsb' 16 8)
  let m3 := writeU8 m2 (addr + 3#64) (v.extractLsb' 24 8)
  let m4 := writeU8 m3 (addr + 4#64) (v.extractLsb' 32 8)
  let m5 := writeU8 m4 (addr + 5#64) (v.extractLsb' 40 8)
  let m6 := writeU8 m5 (addr + 6#64) (v.extractLsb' 48 8)
  writeU8 m6 (addr + 7#64) (v.extractLsb' 56 8)

/-- Evaluate an AArch64 condition code against `NZCV`. -/
def evalCond (c : Arm64Cond) (f : NZCV) : Bool :=
  match c with
  | .eq => f.z
  | .ne => !f.z
  | .cs => f.c
  | .cc => !f.c
  | .mi => f.n
  | .pl => !f.n
  | .vs => f.v
  | .vc => !f.v
  | .hi => f.c && !f.z
  | .ls => !f.c || f.z
  | .ge => f.n == f.v
  | .lt => f.n != f.v
  | .gt => !f.z && (f.n == f.v)
  | .le => f.z || (f.n != f.v)
  | .al => true
  | .nv => true

/-- Compute AArch64 subtraction flags (`lhs - rhs`). -/
def subFlags64 (lhs rhs : BitVec 64) : NZCV :=
  let res := lhs - rhs
  { n := res.msb
    z := res == 0#64
    c := !(BitVec.ult lhs rhs)
    v := (lhs.msb != rhs.msb) && (res.msb != lhs.msb) }

/-- Compute AArch64 addition flags (`lhs + rhs`). -/
def addFlags64 (lhs rhs : BitVec 64) : NZCV :=
  let res := lhs + rhs
  { n := res.msb
    z := res == 0#64
    c := BitVec.ult res lhs
    v := (lhs.msb == rhs.msb) && (res.msb != lhs.msb) }

/-- Compute AArch64 logical-flags (`ANDS`). -/
def logicFlags64 (res : BitVec 64) : NZCV :=
  { n := res.msb, z := res == 0#64, c := false, v := false }

def shiftVal (is64 : Bool) (sk : ShiftKind) (v : BitVec 64) (amt : Nat) : BitVec 64 :=
  if is64 then
    let sh := amt % 64
    match sk with
    | .lsl => v <<< sh
    | .lsr => v >>> sh
    | .asr => BitVec.sshiftRight v sh
    | .ror => BitVec.rotateRight v sh
  else
    let v32 := v.extractLsb' 0 32
    let sh := amt % 32
    let r32 :=
      match sk with
      | .lsl => v32 <<< sh
      | .lsr => v32 >>> sh
      | .asr => BitVec.sshiftRight v32 sh
      | .ror => BitVec.rotateRight v32 sh
    r32.zeroExtend 64

def extVal (ext : ExtKind) (v : BitVec 64) (imm3 : Nat) : BitVec 64 :=
  let base : BitVec 64 :=
    match ext with
    | .uxtb => (v.extractLsb' 0 8).zeroExtend 64
    | .uxth => (v.extractLsb' 0 16).zeroExtend 64
    | .uxtw => (v.extractLsb' 0 32).zeroExtend 64
    | .uxtx => v
    | .sxtb => (v.extractLsb' 0 8).signExtend 64
    | .sxth => (v.extractLsb' 0 16).signExtend 64
    | .sxtw => (v.extractLsb' 0 32).signExtend 64
    | .sxtx => v
  base <<< (imm3 % 5)

@[inline] def mask32IfNeeded (is64 : Bool) (v : BitVec 64) : BitVec 64 :=
  if is64 then v else (v.extractLsb' 0 32).zeroExtend 64

def execBitfield (is64 isSigned isInsert : Bool) (dstOld src : BitVec 64)
    (immr imms : Nat) : BitVec 64 :=
  let datasize := if is64 then 64 else 32
  let srcN := if is64 then src.toNat else src.toNat &&& 0xffffffff
  let dstN := if is64 then dstOld.toNat else dstOld.toNat &&& 0xffffffff
  let r := immr % datasize
  let s := imms % datasize
  let resN :=
    if s >= r then
      let width := s - r + 1
      let wmask := (1 <<< width) - 1
      let field := (srcN >>> r) &&& wmask
      if isInsert then
        let clrMask := ((1 <<< datasize) - 1) ^^^ wmask
        (dstN &&& clrMask) ||| field
      else if isSigned then
        let sign := (field >>> (width - 1)) &&& 1
        if sign == 1 then
          let upper := (((1 <<< (datasize - width)) - 1) <<< width)
          field ||| upper
        else field
      else field
    else
      let width := s + 1
      let shift := datasize - r
      let wmask := (1 <<< width) - 1
      let field := (srcN &&& wmask) <<< shift
      if isInsert then
        let clrMask := ((1 <<< datasize) - 1) ^^^ (wmask <<< shift)
        (dstN &&& clrMask) ||| field
      else if isSigned then
        let sign := (srcN >>> s) &&& 1
        if sign == 1 && (shift + width < datasize) then
          let upper := ((1 <<< (datasize - shift - width)) - 1) <<< (shift + width)
          field ||| upper
        else field
      else field
  let masked := if is64 then resN else resN &&& 0xffffffff
  BitVec.ofNat 64 masked

/-! ### Exact IEEE-754 Binary64 Helpers for AArch64 FP Instructions -/

def f64Sign (b : BitVec 64) : Bool := b.msb
def f64Exp (b : BitVec 64) : Nat := ((b.toNat >>> 52) &&& 0x7ff)
def f64Mant (b : BitVec 64) : Nat := (b.toNat &&& 0x000fffffffffffff)
def f64IsNaN (b : BitVec 64) : Bool :=
  (b &&& 0x7ff0000000000000#64) == 0x7ff0000000000000#64 &&
  (b &&& 0x000fffffffffffff#64) != 0#64
def f64IsZero (b : BitVec 64) : Bool := (b &&& 0x7fffffffffffffff#64) == 0#64
def f64IsPosInf (b : BitVec 64) : Bool := b == 0x7ff0000000000000#64
def f64IsNegInf (b : BitVec 64) : Bool := b == 0xfff0000000000000#64

/-- IEEE-754 binary64 absolute value (`FABSD`). -/
def f64Abs (b : BitVec 64) : BitVec 64 :=
  b &&& 0x7fffffffffffffff#64

/-- IEEE-754 binary64 negation (`FNEGD`). -/
def f64Neg (b : BitVec 64) : BitVec 64 :=
  b ^^^ 0x8000000000000000#64

/-- IEEE-754 binary64 comparison flags (`FCMPD` / `FCMPD $(0.0)`). -/
def f64Cmp (lhs rhs : BitVec 64) : NZCV :=
  if f64IsNaN lhs || f64IsNaN rhs then
    { n := false, z := false, c := true, v := true }
  else if f64IsZero lhs && f64IsZero rhs then
    { n := false, z := true, c := true, v := false }
  else
    let sL := f64Sign lhs
    let sR := f64Sign rhs
    let mL := lhs.toNat &&& 0x7fffffffffffffff
    let mR := rhs.toNat &&& 0x7fffffffffffffff
    if sL != sR then
      if sL then { n := true, z := false, c := false, v := false }
      else { n := false, z := false, c := true, v := false }
    else if mL == mR then
      { n := false, z := true, c := true, v := false }
    else
      let lt := if !sL then mL < mR else mR < mL
      if lt then { n := true, z := false, c := false, v := false }
      else { n := false, z := false, c := true, v := false }

/-- IEEE-754 `FMIND` (returns NaN if either is NaN, `-0 < +0`). -/
def f64Min (a b : BitVec 64) : BitVec 64 :=
  if f64IsNaN a then a
  else if f64IsNaN b then b
  else if f64IsZero a && f64IsZero b then
    if f64Sign a then a else b
  else
    let flags := f64Cmp a b
    if flags.n then a else b

/-- IEEE-754 `FMAXD`. -/
def f64Max (a b : BitVec 64) : BitVec 64 :=
  if f64IsNaN a then a
  else if f64IsNaN b then b
  else if f64IsZero a && f64IsZero b then
    if !f64Sign a then a else b
  else
    let flags := f64Cmp a b
    if flags.n then b else a

private def natLog2Floor (n : Nat) : Nat :=
  let rec go (k fuel : Nat) : Nat :=
    match fuel with
    | 0 => 0
    | fuel' + 1 => if k <= 1 then 0 else 1 + go (k / 2) fuel'
  go n 4096

/-- Round a positive `Rat` to nearest-even binary64 bits. -/
private def posRatToFloat64Bits (q : Rat) : BitVec 64 :=
  if q <= 0 then 0#64
  else
    let num : Nat := q.num.toNat
    let den : Nat := q.den
    let eRaw : Int := (natLog2Floor num : Int) - (natLog2Floor den : Int) - 52
    let shiftAdjust (e : Int) : Int :=
      let p2_52 : Int := (1 <<< 52 : Nat)
      let p2_53 : Int := (1 <<< 53 : Nat)
      if e >= 0 then
        let lhs : Nat := num
        let rhsLow : Nat := den * (1 <<< (52 + e.toNat))
        if lhs < rhsLow then e - 1
        else if lhs >= 2 * rhsLow then e + 1
        else e
      else
        let s : Nat := (-e).toNat
        let lhs : Nat := num * (1 <<< s)
        let rhsLow : Nat := den * p2_52.toNat
        if lhs < rhsLow then e - 1
        else if lhs >= den * p2_53.toNat then e + 1
        else e
    let e := shiftAdjust (shiftAdjust eRaw)
    if e > 971 then
      0x7ff0000000000000#64
    else if e >= -1074 then
      let s : Int := -e
      let (scaledNum, scaledDen) :=
        if s >= 0 then (num * (1 <<< s.toNat), den)
        else (num, den * (1 <<< (-s).toNat))
      let quo := scaledNum / scaledDen
      let rem := scaledNum % scaledDen
      let twiceRem := 2 * rem
      let roundUp :=
        if twiceRem > scaledDen then true
        else if twiceRem < scaledDen then false
        else (quo % 2 == 1)
      let quo' := if roundUp then quo + 1 else quo
      let (sig, eFinal) :=
        if quo' >= (1 <<< 53) then (quo' >>> 1, e + 1) else (quo', e)
      if eFinal > 971 then 0x7ff0000000000000#64
      else
        let biasedExp := (eFinal + 1075).toNat
        let mant := sig &&& 0x000fffffffffffff
        BitVec.ofNat 64 ((biasedExp <<< 52) ||| mant)
    else
      let scaledNum := num * (1 <<< 1074)
      let quo := scaledNum / den
      let rem := scaledNum % den
      let twiceRem := 2 * rem
      let roundUp :=
        if twiceRem > den then true
        else if twiceRem < den then false
        else (quo % 2 == 1)
      let sig := if roundUp then quo + 1 else quo
      if sig >= (1 <<< 52) then
        BitVec.ofNat 64 ((1 <<< 52) ||| (sig &&& 0x000fffffffffffff))
      else
        BitVec.ofNat 64 sig

/-- Convert a finite `Rat` to nearest-even IEEE-754 binary64 bits. -/
def ratToFloat64Bits (q : Rat) (negZero : Bool := false) : BitVec 64 :=
  if q == 0 then
    if negZero then 0x8000000000000000#64 else 0#64
  else if q > 0 then
    posRatToFloat64Bits q
  else
    0x8000000000000000#64 ||| posRatToFloat64Bits (-q)

/-- Exact IEEE-754 `FADDD`. -/
def f64Add (a b : BitVec 64) : BitVec 64 :=
  if f64IsNaN a then a
  else if f64IsNaN b then b
  else if (f64IsPosInf a && f64IsNegInf b) || (f64IsNegInf a && f64IsPosInf b) then
    0x7ff8000000000000#64
  else if f64IsPosInf a || f64IsPosInf b then 0x7ff0000000000000#64
  else if f64IsNegInf a || f64IsNegInf b then 0xfff0000000000000#64
  else
    match float64BitsToRat (UInt64.ofNat a.toNat), float64BitsToRat (UInt64.ofNat b.toNat) with
    | some qa, some qb =>
      let negZ := f64IsZero a && f64IsZero b && f64Sign a && f64Sign b
      ratToFloat64Bits (qa + qb) negZ
    | _, _ => 0x7ff8000000000000#64

/-- Exact IEEE-754 `FSUBD` (`a - b`). -/
def f64Sub (a b : BitVec 64) : BitVec 64 :=
  f64Add a (f64Neg b)

inductive RoundMode where
  | nearestEven
  | nearestAway
  | ceil
  | floor
  | trunc

def f64RoundToInt (mode : RoundMode) (b : BitVec 64) : BitVec 64 :=
  if f64IsNaN b || f64Exp b == 0x7ff || f64IsZero b then b
  else
    match float64BitsToRat (UInt64.ofNat b.toNat) with
    | none => b
    | some q =>
      let z : Int :=
        match mode with
        | .ceil => q.ceil
        | .floor => q.floor
        | .trunc => if q >= 0 then q.floor else q.ceil
        | .nearestAway =>
          if q >= 0 then (q + (1 : Rat) / 2).floor
          else (q - (1 : Rat) / 2).ceil
        | .nearestEven =>
          let fl := q.floor
          let frac := q - (fl : Rat)
          let half : Rat := (1 : Rat) / 2
          if frac < half then fl
          else if frac > half then fl + 1
          else if fl % 2 == 0 then fl else fl + 1
      if z == 0 then
        if f64Sign b then 0x8000000000000000#64 else 0#64
      else
        ratToFloat64Bits (z : Rat) false

/-- Exact `FCVTZSD`: convert IEEE-754 binary64 toward zero into signed 64-bit int. -/
def f64ToInt64 (b : BitVec 64) : BitVec 64 :=
  if f64IsNaN b then 0#64
  else if f64IsPosInf b then 0x7fffffffffffffff#64
  else if f64IsNegInf b then 0x8000000000000000#64
  else
    match float64BitsToRat (UInt64.ofNat b.toNat) with
    | none => 0#64
    | some q =>
      let z : Int := if q >= 0 then q.floor else q.ceil
      if z >= ((1 <<< 63 : Nat) : Int) then 0x7fffffffffffffff#64
      else if z < -((1 <<< 63 : Nat) : Int) then 0x8000000000000000#64
      else BitVec.ofInt 64 z

def calcRegOffsetAddr (base : BitVec 64) (idx : BitVec 64)
    (ext : ExtKind) (scale : Bool) (sizeBytes : Nat) : BitVec 64 :=
  let shift := if scale && sizeBytes == 8 then 3 else 0
  base + extVal ext idx shift

@[inline] def advancePc (s : Arm64State) : Arm64State :=
  { s with pcIdx := s.pcIdx + 1 }

def jumpRel (progLen : Nat) (extTargets : Nat → Option String) (isCall : Bool)
    (s : Arm64State) (imm : Int) : Arm64State :=
  let tgtInt : Int := (s.pcIdx : Int) + imm
  let sLink :=
    if isCall then
      let retAddr := s.baseAddr + BitVec.ofNat 64 (4 * (s.pcIdx + 1))
      writeGpr s 30#5 retAddr
    else s
  if 0 <= tgtInt && tgtInt.toNat < progLen then
    { sLink with pcIdx := tgtInt.toNat }
  else
    match extTargets s.pcIdx with
    | some tgt =>
      if tgt == "runtime.morestack_noctxt.abi0" || tgt == "runtime.morestack" then
        { sLink with status := .trapMorestack }
      else if tgt == "runtime.panicBounds" then
        { sLink with status := .trapPanicBounds }
      else if tgt == "runtime.panicdivide" then
        { sLink with status := .trapPanicDivide }
      else
        { sLink with status := .callExternal tgt }
    | none =>
      { sLink with status := .callExternal s!"pc_rel_{tgtInt}" }

/-- Execute one decoded instruction `insn` in `Arm64State`. -/
def execArm64Insn (progLen : Nat) (extTargets : Nat → Option String)
    (insn : Arm64Insn) (s : Arm64State) : Arm64State :=
  match insn with
  | .nop => advancePc s
  | .ret _ => { s with status := .returned }
  | .b imm26 => jumpRel progLen extTargets false s imm26
  | .bl imm26 => jumpRel progLen extTargets true s imm26
  | .bCond cond imm19 =>
    if evalCond cond s.nzcv then jumpRel progLen extTargets false s imm19
    else advancePc s
  | .cbz is64 isNz rt imm19 =>
    let v := mask32IfNeeded is64 (readGpr s rt)
    let take := if isNz then v != 0#64 else v == 0#64
    if take then jumpRel progLen extTargets false s imm19 else advancePc s
  | .tbz isNz bit rt imm14 =>
    let v := readGpr s rt
    let bitVal := ((v.toNat >>> (bit % 64)) &&& 1) == 1
    let take := if isNz then bitVal else !bitVal
    if take then jumpRel progLen extTargets false s imm14 else advancePc s
  | .adrp rd pageImm =>
    let pcAddr := s.baseAddr + BitVec.ofNat 64 (4 * s.pcIdx)
    let pageBase := pcAddr &&& 0xfffffffffffff000#64
    let res := pageBase + BitVec.ofInt 64 pageImm
    advancePc (writeGpr s rd res)
  | .movWide is64 opc rd imm16 shift =>
    let shifted := (BitVec.ofNat 64 imm16) <<< shift
    let res :=
      if opc == 2 then shifted
      else if opc == 0 then ~~~shifted
      else
        let old := readGpr s rd
        let clrMask := ~~~(0xffff#64 <<< shift)
        (old &&& clrMask) ||| shifted
    advancePc (writeGpr s rd (mask32IfNeeded is64 res))
  | .addSubImm is64 isSub setFlags rd rn imm =>
    let lhs := mask32IfNeeded is64 (readGprSP s rn)
    let rhs := mask32IfNeeded is64 imm
    let res := if isSub then lhs - rhs else lhs + rhs
    let resM := mask32IfNeeded is64 res
    let s1 :=
      if setFlags then
        let fl := if isSub then subFlags64 lhs rhs else addFlags64 lhs rhs
        writeGpr { s with nzcv := fl } rd resM
      else
        writeGprSP s rd resM
    advancePc s1
  | .addSubReg is64 isSub setFlags sk imm6 rd rn rm =>
    let lhs := mask32IfNeeded is64 (readGpr s rn)
    let rhs := shiftVal is64 sk (readGpr s rm) imm6
    let res := mask32IfNeeded is64 (if isSub then lhs - rhs else lhs + rhs)
    let s1 :=
      if setFlags then
        { s with nzcv := if isSub then subFlags64 lhs rhs else addFlags64 lhs rhs }
      else s
    advancePc (writeGpr s1 rd res)
  | .addSubExt is64 isSub setFlags ext imm3 rd rn rm =>
    let lhs := mask32IfNeeded is64 (readGprSP s rn)
    let rhs := mask32IfNeeded is64 (extVal ext (readGpr s rm) imm3)
    let res := mask32IfNeeded is64 (if isSub then lhs - rhs else lhs + rhs)
    let s1 :=
      if setFlags then
        let fl := if isSub then subFlags64 lhs rhs else addFlags64 lhs rhs
        writeGpr { s with nzcv := fl } rd res
      else
        writeGprSP s rd res
    advancePc s1
  | .logicalImm is64 opc rd rn imm =>
    let lhs := mask32IfNeeded is64 (readGpr s rn)
    let rhs := mask32IfNeeded is64 imm
    let res :=
      match opc with
      | 0 => lhs &&& rhs
      | 1 => lhs ||| rhs
      | 2 => lhs ^^^ rhs
      | _ => lhs &&& rhs
    let resM := mask32IfNeeded is64 res
    let s1 :=
      if opc == 3 then writeGpr { s with nzcv := logicFlags64 resM } rd resM
      else writeGprSP s rd resM
    advancePc s1
  | .logicalReg is64 inv opc sk imm6 rd rn rm =>
    let lhs := mask32IfNeeded is64 (readGpr s rn)
    let op2Raw := shiftVal is64 sk (readGpr s rm) imm6
    let rhs := mask32IfNeeded is64 (if inv then ~~~op2Raw else op2Raw)
    let res :=
      match opc with
      | 0 => lhs &&& rhs
      | 1 => lhs ||| rhs
      | 2 => lhs ^^^ rhs
      | _ => lhs &&& rhs
    let resM := mask32IfNeeded is64 res
    let s1 := if opc == 3 then { s with nzcv := logicFlags64 resM } else s
    advancePc (writeGpr s1 rd resM)
  | .bitfield is64 isSigned isInsert rd rn immr imms =>
    let res := execBitfield is64 isSigned isInsert (readGpr s rd) (readGpr s rn) immr imms
    advancePc (writeGpr s rd res)
  | .condSel is64 isInc cond rd rn rm =>
    let val :=
      if evalCond cond s.nzcv then
        readGpr s rn
      else
        let m := readGpr s rm
        if isInc then m + 1#64 else m
    advancePc (writeGpr s rd (mask32IfNeeded is64 val))
  | .div is64 isSigned rd rn rm =>
    let lhs := mask32IfNeeded is64 (readGpr s rn)
    let rhs := mask32IfNeeded is64 (readGpr s rm)
    let res :=
      if rhs == 0#64 then 0#64
      else if isSigned then lhs.sdiv rhs
      else lhs.udiv rhs
    advancePc (writeGpr s rd (mask32IfNeeded is64 res))
  | .mul3 is64 isSub rd rn rm ra =>
    let vn := mask32IfNeeded is64 (readGpr s rn)
    let vm := mask32IfNeeded is64 (readGpr s rm)
    let va := mask32IfNeeded is64 (readGpr s ra)
    let prod := vn * vm
    let res := if isSub then va - prod else va + prod
    advancePc (writeGpr s rd (mask32IfNeeded is64 res))
  | .ldrStrPair isLoad isFP mode imm rt rt2 rn =>
    let base := readGprSP s rn
    let addr :=
      match mode with
      | .post => base
      | .offset | .pre => base + BitVec.ofInt 64 imm
    let s1 :=
      if isLoad then
        let v1 := readU64 s.mem addr
        let v2 := readU64 s.mem (addr + 8#64)
        if isFP then writeFp (writeFp s rt v1) rt2 v2
        else writeGpr (writeGpr s rt v1) rt2 v2
      else
        let v1 := if isFP then s.fpRegs rt else readGpr s rt
        let v2 := if isFP then s.fpRegs rt2 else readGpr s rt2
        let m1 := writeU64 s.mem addr v1
        let m2 := writeU64 m1 (addr + 8#64) v2
        { s with mem := m2 }
    let s2 :=
      match mode with
      | .offset => s1
      | .post | .pre => writeGprSP s1 rn (base + BitVec.ofInt 64 imm)
    advancePc s2
  | .ldrStrUImm isLoad isFP sizeBytes imm rt rn =>
    let addr := readGprSP s rn + BitVec.ofNat 64 imm
    let s1 :=
      if isLoad then
        if sizeBytes == 8 then
          let v := readU64 s.mem addr
          if isFP then writeFp s rt v else writeGpr s rt v
        else
          let v := (readU8 s.mem addr).zeroExtend 64
          writeGpr s rt v
      else
        if sizeBytes == 8 then
          let v := if isFP then s.fpRegs rt else readGpr s rt
          { s with mem := writeU64 s.mem addr v }
        else
          let v := (readGpr s rt).extractLsb' 0 8
          { s with mem := writeU8 s.mem addr v }
    advancePc s1
  | .ldrStrImm isLoad isFP sizeBytes mode imm9 rt rn =>
    let base := readGprSP s rn
    let addr :=
      match mode with
      | .post => base
      | .offset | .pre => base + BitVec.ofInt 64 imm9
    let s1 :=
      if isLoad then
        if sizeBytes == 8 then
          let v := readU64 s.mem addr
          if isFP then writeFp s rt v else writeGpr s rt v
        else writeGpr s rt ((readU8 s.mem addr).zeroExtend 64)
      else
        let v := if isFP then s.fpRegs rt else readGpr s rt
        { s with mem := writeU64 s.mem addr v }
    let s2 :=
      match mode with
      | .offset => s1
      | .post | .pre => writeGprSP s1 rn (base + BitVec.ofInt 64 imm9)
    advancePc s2
  | .ldrStrReg isLoad isSigned isFP sizeBytes ext scale rt rn rm =>
    let addr := calcRegOffsetAddr (readGprSP s rn) (readGpr s rm) ext scale sizeBytes
    let s1 :=
      if isLoad then
        if sizeBytes == 8 then
          let v := readU64 s.mem addr
          if isFP then writeFp s rt v else writeGpr s rt v
        else
          let b := readU8 s.mem addr
          let v := if isSigned then b.signExtend 64 else b.zeroExtend 64
          writeGpr s rt v
      else
        if sizeBytes == 8 then
          let v := if isFP then s.fpRegs rt else readGpr s rt
          { s with mem := writeU64 s.mem addr v }
        else
          { s with mem := writeU8 s.mem addr ((readGpr s rt).extractLsb' 0 8) }
    advancePc s1
  | .fcmpD isZero rn rm =>
    let lhs := s.fpRegs rn
    let rhs := if isZero then 0#64 else s.fpRegs rm
    advancePc { s with nzcv := f64Cmp lhs rhs }
  | .fpUnaryD op rd rn =>
    let src := s.fpRegs rn
    let res :=
      match op with
      | 0 => src
      | 1 => f64Abs src
      | 2 => f64Neg src
      | 8 => f64RoundToInt .nearestEven src
      | 9 => f64RoundToInt .ceil src
      | 10 => f64RoundToInt .floor src
      | 11 => f64RoundToInt .trunc src
      | 12 => f64RoundToInt .nearestAway src
      | _ => src
    advancePc (writeFp s rd res)
  | .fpBinaryD op rd rn rm =>
    let a := s.fpRegs rn
    let b := s.fpRegs rm
    let res :=
      match op with
      | 2 => f64Add a b
      | 3 => f64Sub a b
      | 4 => f64Max a b
      | 5 => f64Min a b
      | _ => a
    advancePc (writeFp s rd res)
  | .fpIntConv toInt isSigned gprToFp rd rn =>
    if toInt then
      let res := f64ToInt64 (s.fpRegs rn)
      advancePc (writeGpr s rd res)
    else if gprToFp then
      if isSigned then
        let z := (readGpr s rn).toInt
        advancePc (writeFp s rd (ratToFloat64Bits (z : Rat) false))
      else
        advancePc (writeFp s rd (readGpr s rn))
    else
      advancePc (writeGpr s rd (s.fpRegs rn))
  | .unknown _ => { s with status := .trapPanicBounds }

/-- Execute one step of `Arm64State` on decoded program `prog`. -/
def stepArm64 (prog : Array Arm64Insn) (extTargets : Nat → Option String)
    (s : Arm64State) : Arm64State :=
  if s.status != .running then s
  else if h : s.pcIdx < prog.size then
    execArm64Insn prog.size extTargets prog[s.pcIdx] s
  else
    { s with status := .returned }

/-- Run `stepArm64` up to `fuel` cycles, halting when `status != .running`. -/
def runArm64 (fuel : Nat) (prog : Array Arm64Insn) (extTargets : Nat → Option String)
    (s : Arm64State) : Arm64State :=
  match fuel with
  | 0 => s
  | fuel' + 1 =>
    if s.status != .running then s
    else runArm64 fuel' prog extTargets (stepArm64 prog extTargets s)

/-! ### Fast Stepping & Bitvector Lemmas -/

theorem runArm64_succ_running (fuel : Nat) (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (h : s.status = .running) :
    runArm64 (fuel + 1) prog ext s = runArm64 fuel prog ext (stepArm64 prog ext s) := by
  simp [runArm64, h]

theorem runArm64_of_not_running (fuel : Nat) (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (h : s.status ≠ .running) :
    runArm64 fuel prog ext s = s := by
  cases fuel with
  | zero => rfl
  | succ k =>
    have hne : (s.status != .running) = true := bne_iff_ne.mpr h
    simp [runArm64, hne]

theorem runArm64_add (n m : Nat) (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) :
    runArm64 (n + m) prog ext s = runArm64 m prog ext (runArm64 n prog ext s) := by
  induction n generalizing s with
  | zero => simp [runArm64]
  | succ k ih =>
    by_cases hst : s.status = .running
    · rw [Nat.succ_add, runArm64_succ_running _ _ _ _ hst,
          runArm64_succ_running _ _ _ _ hst]
      exact ih (stepArm64 prog ext s)
    · rw [runArm64_of_not_running _ _ _ _ hst, runArm64_of_not_running _ _ _ _ hst,
          runArm64_of_not_running _ _ _ _ hst]

theorem stepArm64_at (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k : Nat) (insn : Arm64Insn)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hFetch : prog[k]? = some insn) :
    stepArm64 prog ext s = execArm64Insn prog.size ext insn s := by
  have hLt : k < prog.size := by
    by_cases h : k < prog.size
    · exact h
    · rw [Array.getElem?_eq_none (Nat.le_of_not_lt h)] at hFetch
      cases hFetch
  have hGet : prog[k] = insn := by
    rw [Array.getElem?_eq_getElem hLt] at hFetch
    exact Option.some.inj hFetch
  simp only [stepArm64, hRun, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte, hPc, hLt,
    ↓reduceDIte, hGet]

@[simp] theorem status_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).status = s.status := by
  unfold writeGpr; split <;> rfl

@[simp] theorem pcIdx_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).pcIdx = s.pcIdx := by
  unfold writeGpr; split <;> rfl

@[simp] theorem sp_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).sp = s.sp := by
  unfold writeGpr; split <;> rfl

@[simp] theorem nzcv_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).nzcv = s.nzcv := by
  unfold writeGpr; split <;> rfl

@[simp] theorem fpRegs_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).fpRegs = s.fpRegs := by
  unfold writeGpr; split <;> rfl

@[simp] theorem mem_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).mem = s.mem := by
  unfold writeGpr; split <;> rfl

@[simp] theorem gStackGuard0_writeGpr (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).gStackGuard0 = s.gStackGuard0 := by
  unfold writeGpr; split <;> rfl

@[simp] theorem status_advancePc (s : Arm64State) :
    (advancePc s).status = s.status := rfl

@[simp] theorem pcIdx_advancePc (s : Arm64State) :
    (advancePc s).pcIdx = s.pcIdx + 1 := rfl

@[simp] theorem regs_advancePc (s : Arm64State) :
    (advancePc s).regs = s.regs := rfl

@[simp] theorem sp_advancePc (s : Arm64State) :
    (advancePc s).sp = s.sp := rfl

@[simp] theorem nzcv_advancePc (s : Arm64State) :
    (advancePc s).nzcv = s.nzcv := rfl

@[simp] theorem fpRegs_advancePc (s : Arm64State) :
    (advancePc s).fpRegs = s.fpRegs := rfl

@[simp] theorem mem_advancePc (s : Arm64State) :
    (advancePc s).mem = s.mem := rfl

@[simp] theorem gStackGuard0_advancePc (s : Arm64State) :
    (advancePc s).gStackGuard0 = s.gStackGuard0 := rfl

@[simp] theorem readGpr_advancePc (s : Arm64State) (r : BitVec 5) :
    readGpr (advancePc s) r = readGpr s r := rfl

theorem readGpr_writeGpr (s : Arm64State) (r r' : BitVec 5) (v : BitVec 64) :
    readGpr (writeGpr s r v) r' =
      if r' == 31#5 then 0#64 else if r' == r then v else readGpr s r' := by
  unfold readGpr writeGpr
  by_cases hr : r == 31#5 <;> by_cases hr' : r' == 31#5
  · simp [hr']
  · simp only [hr, ↓reduceIte, hr']
    have hNe : r' ≠ r := fun h => hr' (by simpa [h] using hr)
    simp [beq_false_of_ne hNe]
  · simp [hr']
  · simp [hr, hr']

theorem readGpr_writeGpr_zero (s : Arm64State) (r r' : BitVec 5) (v : BitVec 64)
    (hNe : r' ≠ r) (hNotZR : r' ≠ 31#5) :
    readGpr (writeGpr s r v) r' = readGpr s r' := by
  simp [readGpr_writeGpr, beq_false_of_ne hNe, beq_false_of_ne hNotZR]

theorem readGpr_writeGpr_eq (s : Arm64State) (r : BitVec 5) (v : BitVec 64)
    (hNe : r ≠ 31#5) : readGpr (writeGpr s r v) r = v := by
  simp [readGpr_writeGpr, beq_false_of_ne hNe]

theorem readGpr_writeGpr_ne (s : Arm64State) (r r' : BitVec 5) (v : BitVec 64)
    (hNe : r' ≠ r) : readGpr (writeGpr s r v) r' = readGpr s r' := by
  rw [readGpr_writeGpr]
  by_cases hr' : r' == 31#5
  · simp [readGpr, hr']
  · simp [hr', beq_false_of_ne hNe]

theorem readGpr_eq_regs (s : Arm64State) (r : BitVec 5) (h : r ≠ 31#5) :
    readGpr s r = s.regs r := by
  simp [readGpr, beq_false_of_ne h]

theorem readGprSP_eq_readGpr (s : Arm64State) (r : BitVec 5) (h : r ≠ 31#5) :
    readGprSP s r = readGpr s r := by
  simp [readGprSP, readGpr, beq_false_of_ne h]

theorem readGpr_writeGprSP_ne (s : Arm64State) (r r' : BitVec 5) (v : BitVec 64)
    (hr : r ≠ 31#5) (hNe : r' ≠ r) :
    readGpr (writeGprSP s r v) r' = readGpr s r' := by
  unfold writeGprSP readGpr
  simp only [beq_false_of_ne hr, Bool.false_eq_true, ↓reduceIte]
  by_cases hr' : r' == 31#5
  · simp [hr']
  · simp [hr', beq_false_of_ne hNe]

theorem readGpr_setMem (s : Arm64State) (m : BitVec 64 → BitVec 8) (r : BitVec 5) :
    readGpr { s with mem := m } r = readGpr s r := rfl

theorem status_setNzcv (s : Arm64State) (f : NZCV) :
    ({ s with nzcv := f } : Arm64State).status = s.status := rfl

theorem pcIdx_setNzcv (s : Arm64State) (f : NZCV) :
    ({ s with nzcv := f } : Arm64State).pcIdx = s.pcIdx := rfl

theorem readGpr_setNzcv (s : Arm64State) (f : NZCV) (r : BitVec 5) :
    readGpr { s with nzcv := f } r = readGpr s r := rfl

theorem readGprSP_writeGpr (s : Arm64State) (r r' : BitVec 5) (v : BitVec 64)
    (hNe : r' ≠ r) : readGprSP (writeGpr s r v) r' = readGprSP s r' := by
  unfold readGprSP writeGpr
  by_cases hr : r == 31#5
  · simp [hr]
  · simp only [hr, Bool.false_eq_true, ↓reduceIte]
    by_cases hr' : r' == 31#5
    · simp [hr']
    · simp [hr', beq_false_of_ne hNe]

theorem readGpr_writeGprSP_sp (s : Arm64State) (r' : BitVec 5) (v : BitVec 64) :
    readGpr (writeGprSP s 31#5 v) r' = readGpr s r' := by
  rfl

theorem sp_writeGprSP_sp (s : Arm64State) (v : BitVec 64) :
    (writeGprSP s 31#5 v).sp = v := rfl

theorem status_writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGprSP s r v).status = s.status := by
  unfold writeGprSP; split <;> rfl

theorem pcIdx_writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGprSP s r v).pcIdx = s.pcIdx := by
  unfold writeGprSP; split <;> rfl

theorem gStackGuard0_writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGprSP s r v).gStackGuard0 = s.gStackGuard0 := by
  unfold writeGprSP; split <;> rfl

theorem mem_writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGprSP s r v).mem = s.mem := by
  unfold writeGprSP; split <;> rfl

theorem fpRegs_writeGprSP (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGprSP s r v).fpRegs = s.fpRegs := by
  unfold writeGprSP; split <;> rfl

theorem readU64_writeU64_eq (mem : BitVec 64 → BitVec 8) (a v : BitVec 64) :
    readU64 (writeU64 mem a v) a = v := by
  unfold readU64 writeU64 writeU8
  bv_decide

private theorem readU8_writeU64_ne (mem : BitVec 64 → BitVec 8) (a b v : BitVec 64)
    (hNe : ∀ i : BitVec 64, BitVec.ult i 8#64 = true → (b == a + i) = false) :
    writeU64 mem a v b = mem b := by
  unfold writeU64 writeU8
  have h0 : (b == a) = false := by simpa using hNe 0#64 (by decide)
  simp [hNe 7#64 (by decide), hNe 6#64 (by decide), hNe 5#64 (by decide), hNe 4#64 (by decide),
        hNe 3#64 (by decide), hNe 2#64 (by decide), hNe 1#64 (by decide), h0]

theorem readU64_writeU64_disjoint (mem : BitVec 64 → BitVec 8) (a b v : BitVec 64)
    (h1 : BitVec.ult 7#64 (b - a) = true) (h2 : BitVec.ult (b - a) (0#64 - 7#64) = true) :
    readU64 (writeU64 mem a v) b = readU64 mem b := by
  unfold readU64
  have hNe : ∀ k : BitVec 64, BitVec.ult k 8#64 = true → ∀ i : BitVec 64, BitVec.ult i 8#64 = true →
      (b + k == a + i) = false := by
    intro k hk i hi
    bv_decide
  simp only [
    readU8_writeU64_ne mem a b v (by intro i hi; simpa using hNe 0#64 (by decide) i hi),
    readU8_writeU64_ne mem a (b + 1#64) v (hNe 1#64 (by decide)),
    readU8_writeU64_ne mem a (b + 2#64) v (hNe 2#64 (by decide)),
    readU8_writeU64_ne mem a (b + 3#64) v (hNe 3#64 (by decide)),
    readU8_writeU64_ne mem a (b + 4#64) v (hNe 4#64 (by decide)),
    readU8_writeU64_ne mem a (b + 5#64) v (hNe 5#64 (by decide)),
    readU8_writeU64_ne mem a (b + 6#64) v (hNe 6#64 (by decide)),
    readU8_writeU64_ne mem a (b + 7#64) v (hNe 7#64 (by decide))]

theorem stepArm64_insn (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (insn : Arm64Insn)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some insn) :
    stepArm64 prog ext s = execArm64Insn n ext insn s := by
  rw [stepArm64_at prog ext s k insn hRun hPc hFetch, hSize]

theorem step_movZ64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd : BitVec 5) (imm16 shift : Nat)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.movWide true 2 rd imm16 shift)) :
    stepArm64 prog ext s = advancePc (writeGpr s rd ((BitVec.ofNat 64 imm16) <<< shift)) := by
  rw [stepArm64_insn prog ext s k n (.movWide true 2 rd imm16 shift) hRun hPc hSize hFetch]
  rfl

theorem step_movK64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd : BitVec 5) (imm16 shift : Nat)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.movWide true 3 rd imm16 shift)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd ((readGpr s rd &&& ~~~(0xffff#64 <<< shift)) |||
        ((BitVec.ofNat 64 imm16) <<< shift))) := by
  rw [stepArm64_insn prog ext s k n (.movWide true 3 rd imm16 shift) hRun hPc hSize hFetch]
  rfl

theorem step_sub64Reg (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.addSubReg true true false .lsl 0 rd rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (readGpr s rn - (readGpr s rm <<< (0 % 64)))) := by
  rw [stepArm64_insn prog ext s k n (.addSubReg true true false .lsl 0 rd rn rm) hRun hPc hSize hFetch]
  rfl

theorem step_eor64Lsr (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n imm6 : Nat) (rd rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.logicalReg true false 2 .lsr imm6 rd rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (readGpr s rn ^^^ (readGpr s rm >>> (imm6 % 64)))) := by
  rw [stepArm64_insn prog ext s k n (.logicalReg true false 2 .lsr imm6 rd rn rm) hRun hPc hSize hFetch]
  rfl

theorem step_mul64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn rm ra : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.mul3 true false rd rn rm ra)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (readGpr s ra + readGpr s rn * readGpr s rm)) := by
  rw [stepArm64_insn prog ext s k n (.mul3 true false rd rn rm ra) hRun hPc hSize hFetch]
  rfl

theorem step_ret (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (r : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ret r)) :
    stepArm64 prog ext s = { s with status := .returned } := by
  rw [stepArm64_insn prog ext s k n (.ret r) hRun hPc hSize hFetch]
  rfl

theorem step_nop (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some .nop) :
    stepArm64 prog ext s = advancePc s := by
  rw [stepArm64_insn prog ext s k n .nop hRun hPc hSize hFetch]
  rfl

theorem step_ldrStrUImm_load64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n imm : Nat) (rt rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ldrStrUImm true false 8 imm rt rn)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rt (readU64 s.mem (readGprSP s rn + BitVec.ofNat 64 imm))) := by
  rw [stepArm64_insn prog ext s k n (.ldrStrUImm true false 8 imm rt rn) hRun hPc hSize hFetch]
  rfl

theorem step_addSubExt_cmp64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.addSubExt true true true .uxtx 0 31#5 rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr { s with nzcv := subFlags64 (readGprSP s rn) ((readGpr s rm) <<< (0 % 5)) }
        31#5 (readGprSP s rn - ((readGpr s rm) <<< (0 % 5)))) := by
  rw [stepArm64_insn prog ext s k n (.addSubExt true true true .uxtx 0 31#5 rn rm) hRun hPc hSize hFetch]
  rfl

theorem step_addSubImm_sub64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn : BitVec 5) (imm : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.addSubImm true true false rd rn imm)) :
    stepArm64 prog ext s = advancePc (writeGprSP s rd (readGprSP s rn - imm)) := by
  rw [stepArm64_insn prog ext s k n (.addSubImm true true false rd rn imm) hRun hPc hSize hFetch]
  rfl

theorem step_addSubImm_cmp64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rn : BitVec 5) (imm : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.addSubImm true true true 31#5 rn imm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr { s with nzcv := subFlags64 (readGprSP s rn) imm }
        31#5 (readGprSP s rn - imm)) := by
  rw [stepArm64_insn prog ext s k n (.addSubImm true true true 31#5 rn imm) hRun hPc hSize hFetch]
  rfl

theorem step_ldrStrImm_pre_store64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm9 : Int) (rt rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ldrStrImm false false 8 .pre imm9 rt rn)) :
    let addr := readGprSP s rn + BitVec.ofInt 64 imm9
    stepArm64 prog ext s =
      advancePc (writeGprSP { s with mem := writeU64 s.mem addr (readGpr s rt) } rn addr) := by
  rw [stepArm64_insn prog ext s k n (.ldrStrImm false false 8 .pre imm9 rt rn) hRun hPc hSize hFetch]
  rfl

theorem step_ldrStrImm_off_store64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm9 : Int) (rt rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ldrStrImm false false 8 .offset imm9 rt rn)) :
    let addr := readGprSP s rn + BitVec.ofInt 64 imm9
    stepArm64 prog ext s = advancePc { s with mem := writeU64 s.mem addr (readGpr s rt) } := by
  rw [stepArm64_insn prog ext s k n (.ldrStrImm false false 8 .offset imm9 rt rn) hRun hPc hSize hFetch]
  rfl

theorem step_ldrStrImm_off_load64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm9 : Int) (rt rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ldrStrImm true false 8 .offset imm9 rt rn)) :
    let addr := readGprSP s rn + BitVec.ofInt 64 imm9
    stepArm64 prog ext s = advancePc (writeGpr s rt (readU64 s.mem addr)) := by
  rw [stepArm64_insn prog ext s k n (.ldrStrImm true false 8 .offset imm9 rt rn) hRun hPc hSize hFetch]
  rfl

theorem step_ldrStrImm_post_load64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm9 : Int) (rt rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.ldrStrImm true false 8 .post imm9 rt rn)) :
    let base := readGprSP s rn
    stepArm64 prog ext s =
      advancePc (writeGprSP (writeGpr s rt (readU64 s.mem base)) rn (base + BitVec.ofInt 64 imm9)) := by
  rw [stepArm64_insn prog ext s k n (.ldrStrImm true false 8 .post imm9 rt rn) hRun hPc hSize hFetch]
  rfl

theorem step_fcmpD_zero (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.fcmpD true rn rm)) :
    stepArm64 prog ext s = advancePc { s with nzcv := f64Cmp (s.fpRegs rn) 0#64 } := by
  rw [stepArm64_insn prog ext s k n (.fcmpD true rn rm) hRun hPc hSize hFetch]
  rfl

theorem step_fmovDToGpr (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.fpIntConv false false false rd rn)) :
    stepArm64 prog ext s = advancePc (writeGpr s rd (s.fpRegs rn)) := by
  rw [stepArm64_insn prog ext s k n (.fpIntConv false false false rd rn) hRun hPc hSize hFetch]
  rfl

theorem step_orr64_lsl0 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.logicalReg true false 1 .lsl 0 rd rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (readGpr s rn ||| ((readGpr s rm) <<< (0 % 64)))) := by
  rw [stepArm64_insn prog ext s k n (.logicalReg true false 1 .lsl 0 rd rn rm) hRun hPc hSize hFetch]
  rfl

private theorem ite_true_eq {α : Sort _} (a b : α) : (if (true = true) then a else b) = a := rfl
private theorem ite_false_eq {α : Sort _} (a b : α) : (if (false = true) then a else b) = b := rfl

theorem step_sdiv64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.div true true rd rn rm))
    (hNz : readGpr s rm ≠ 0#64) :
    stepArm64 prog ext s = advancePc (writeGpr s rd ((readGpr s rn).sdiv (readGpr s rm))) := by
  rw [stepArm64_insn prog ext s k n (.div true true rd rn rm) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn]
  have hNe : (mask32IfNeeded true (readGpr s rm) == 0#64) = false := beq_false_of_ne hNz
  rw [ite_true_eq]
  split
  · rename_i hZero; rw [hNe] at hZero; contradiction
  · rfl

theorem step_msub64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rd rn rm ra : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.mul3 true true rd rn rm ra)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (readGpr s ra - readGpr s rn * readGpr s rm)) := by
  rw [stepArm64_insn prog ext s k n (.mul3 true true rd rn rm ra) hRun hPc hSize hFetch]
  rfl

theorem step_cinc64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (cond : Arm64Cond) (rd rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.condSel true true cond rd rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr s rd (if evalCond cond s.nzcv then readGpr s rn else readGpr s rm + 1#64)) := by
  rw [stepArm64_insn prog ext s k n (.condSel true true cond rd rn rm) hRun hPc hSize hFetch]
  rfl

theorem step_b_internal (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm26 : Int)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.b imm26))
    (hLo : 0 ≤ (k : Int) + imm26) (hHi : ((k : Int) + imm26).toNat < n) :
    stepArm64 prog ext s = { s with pcIdx := ((k : Int) + imm26).toNat } := by
  rw [stepArm64_insn prog ext s k n (.b imm26) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn, jumpRel]
  rw [ite_false_eq, hPc]
  have hIn : (decide (0 ≤ (k : Int) + imm26) && decide (((k : Int) + imm26).toNat < n)) = true := by
    simp [hLo, hHi]
  rw [hIn, ite_true_eq]

theorem step_bCond_true (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (cond : Arm64Cond) (imm19 : Int)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.bCond cond imm19))
    (hCond : evalCond cond s.nzcv = true)
    (hLo : 0 ≤ (k : Int) + imm19) (hHi : ((k : Int) + imm19).toNat < n) :
    stepArm64 prog ext s = { s with pcIdx := ((k : Int) + imm19).toNat } := by
  rw [stepArm64_insn prog ext s k n (.bCond cond imm19) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn, jumpRel]
  rw [hCond, ite_true_eq, ite_false_eq, hPc]
  have hIn : (decide (0 ≤ (k : Int) + imm19) && decide (((k : Int) + imm19).toNat < n)) = true := by
    simp [hLo, hHi]
  rw [hIn, ite_true_eq]

theorem step_bCond_false (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (cond : Arm64Cond) (imm19 : Int)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.bCond cond imm19))
    (hCond : evalCond cond s.nzcv = false) :
    stepArm64 prog ext s = advancePc s := by
  rw [stepArm64_insn prog ext s k n (.bCond cond imm19) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn]
  rw [hCond]
  rfl

theorem step_cbz64_taken (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (isNz : Bool) (rt : BitVec 5) (imm19 : Int)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.cbz true isNz rt imm19))
    (hCond : (if isNz then readGpr s rt != 0#64 else readGpr s rt == 0#64) = true)
    (hLo : 0 ≤ (k : Int) + imm19) (hHi : ((k : Int) + imm19).toNat < n) :
    stepArm64 prog ext s = { s with pcIdx := ((k : Int) + imm19).toNat } := by
  rw [stepArm64_insn prog ext s k n (.cbz true isNz rt imm19) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn]
  cases isNz
  · rw [ite_false_eq] at hCond
    rw [ite_false_eq, ite_eq_left (by simpa [mask32IfNeeded] using hCond)]
    dsimp only [jumpRel]
    rw [ite_false_eq, hPc, ite_eq_left (by simp [hLo, hHi])]
  · rw [ite_true_eq] at hCond
    rw [ite_true_eq, ite_eq_left (by simpa [mask32IfNeeded] using hCond)]
    dsimp only [jumpRel]
    rw [ite_false_eq, hPc, ite_eq_left (by simp [hLo, hHi])]

theorem step_cbz64_fall (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (isNz : Bool) (rt : BitVec 5) (imm19 : Int)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.cbz true isNz rt imm19))
    (hCond : (if isNz then readGpr s rt != 0#64 else readGpr s rt == 0#64) = false) :
    stepArm64 prog ext s = advancePc s := by
  rw [stepArm64_insn prog ext s k n (.cbz true isNz rt imm19) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn]
  cases isNz
  · rw [ite_false_eq] at hCond
    have hF : (mask32IfNeeded true (readGpr s rt) == 0#64) = false := hCond
    rw [ite_false_eq, ite_eq_right (by simp [hF])]
  · rw [ite_true_eq] at hCond
    have hF : (mask32IfNeeded true (readGpr s rt) != 0#64) = false := hCond
    rw [ite_true_eq, ite_eq_right (by simp [hF])]

theorem step_bl_callExt (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (imm26 : Int) (tgt : String)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.bl imm26))
    (hExtRange : !(0 ≤ (k : Int) + imm26 && ((k : Int) + imm26).toNat < n) = true)
    (hTgt : ext k = some tgt)
    (h1 : tgt ≠ "runtime.morestack_noctxt.abi0") (h2 : tgt ≠ "runtime.morestack")
    (h3 : tgt ≠ "runtime.panicBounds") (h4 : tgt ≠ "runtime.panicdivide") :
    stepArm64 prog ext s =
      { writeGpr s 30#5 (s.baseAddr + BitVec.ofNat 64 (4 * (k + 1))) with
        status := .callExternal tgt } := by
  rw [stepArm64_insn prog ext s k n (.bl imm26) hRun hPc hSize hFetch]
  dsimp only [execArm64Insn, jumpRel]
  have hFalse : (0 ≤ (k : Int) + imm26 && ((k : Int) + imm26).toNat < n) = false := by
    cases h : (0 ≤ (k : Int) + imm26 && ((k : Int) + imm26).toNat < n) <;> simp_all
  rw [ite_true_eq, hPc, hFalse, ite_false_eq, hTgt]
  dsimp only
  have hm : (tgt == "runtime.morestack_noctxt.abi0" || tgt == "runtime.morestack") = false := by
    simp [beq_false_of_ne h1, beq_false_of_ne h2]
  have hp : (tgt == "runtime.panicBounds") = false := beq_false_of_ne h3
  have hd : (tgt == "runtime.panicdivide") = false := beq_false_of_ne h4
  rw [hm, ite_false_eq, hp, ite_false_eq, hd, ite_false_eq]

theorem stackGuard_32_ok (sp sg : BitVec 64)
    (hSp : BitVec.ult 31#64 sp = true) (hSg : BitVec.ult sg (sp - 32#64) = true) :
    BitVec.ult sg sp = true := by
  bv_decide

theorem evalCond_ls_subFlags64 (lhs rhs : BitVec 64) :
    evalCond .ls (subFlags64 lhs rhs) = !(BitVec.ult rhs lhs) := by
  unfold evalCond subFlags64
  bv_decide

theorem stackGuard_16_ok (sp sg : BitVec 64)
    (hSp : BitVec.ult 15#64 sp = true) (hSg : BitVec.ult sg (sp - 16#64) = true) :
    BitVec.ult sg sp = true := by
  bv_decide

theorem stackGuard_240_ok (sp sg : BitVec 64)
    (hSp : BitVec.ult 239#64 sp = true) (hSg : BitVec.ult sg (sp - 240#64) = true) :
    BitVec.ult sg (sp - 112#64) = true := by
  bv_decide

theorem evalCond_le_subFlags64_zero (rem : BitVec 64) :
    evalCond .le (subFlags64 rem 0#64) = !(BitVec.slt 0#64 rem) := by
  unfold evalCond subFlags64
  bv_decide

private theorem f64IsZero_iff (b : BitVec 64) :
    f64IsZero b = true ↔ (b = 0#64 ∨ b = 0x8000000000000000#64) := by
  unfold f64IsZero
  constructor
  · intro h
    by_cases h0 : b = 0#64
    · exact Or.inl h0
    · right
      have hbv : (b &&& 0x7fffffffffffffff#64 == 0#64) = true →
          b ≠ 0#64 → b = 0x8000000000000000#64 := by bv_decide
      exact hbv h h0
  · intro h
    rcases h with rfl | rfl <;> decide

private theorem f64IsNaN_zero : f64IsNaN 0#64 = false := by decide

private theorem f64IsNaN_of_isZero {b : BitVec 64} (hz : f64IsZero b = true) :
    f64IsNaN b = false := by
  rcases (f64IsZero_iff b).mp hz with rfl | rfl <;> decide

theorem f64Cmp_zero_z_iff (f0 : BitVec 64) :
    (f64Cmp f0 0#64).z = true ↔ (f0 = 0#64 ∨ f0 = 0x8000000000000000#64) := by
  unfold f64Cmp
  by_cases hNaN : f64IsNaN f0 = true
  · have hNotZ : f0 ≠ 0#64 ∧ f0 ≠ 0x8000000000000000#64 := by
      constructor <;> (rintro rfl; simp [f64IsNaN] at hNaN)
    simp [hNaN, hNotZ]
  · have hNaN0 : f64IsNaN 0#64 = false := f64IsNaN_zero
    have hz0 : f64IsZero 0#64 = true := by decide
    simp only [hNaN, hNaN0, Bool.false_eq_true, Bool.or_self, ↓reduceIte, hz0, Bool.and_true]
    by_cases hz : f64IsZero f0 = true
    · simp [hz, (f64IsZero_iff f0).mp hz]
    · have hNotZ : ¬(f0 = 0#64 ∨ f0 = 0x8000000000000000#64) := by
        intro hOr; exact hz ((f64IsZero_iff f0).mpr hOr)
      have hNeMagNat : f0.toNat &&& 0x7fffffffffffffff ≠ 0 := by
        intro hnat
        apply hz
        unfold f64IsZero
        have hEq0 : f0 &&& 0x7fffffffffffffff#64 = 0#64 := by
          apply BitVec.eq_of_toNat_eq
          simpa [BitVec.toNat_and] using hnat
        simp [hEq0]
      simp only [hz, Bool.false_eq_true, ↓reduceIte]
      have hSign0 : f64Sign 0#64 = false := by decide
      by_cases hs : f64Sign f0 = true
      · simp [hs, hSign0]
        exact ⟨fun h => hNotZ (Or.inl h), fun h => hNotZ (Or.inr h)⟩
      · simp [hs, hSign0, hNeMagNat]
        exact ⟨fun h => hNotZ (Or.inl h), fun h => hNotZ (Or.inr h)⟩

/-! ### Embedded Go 1.27 GOARCH=arm64 Binaries & Tier 5A Equivalence Theorems -/

/-- Exact pure Lean specification of `presolveSplitmix64`. -/
def splitmix64Spec (x : BitVec 64) : BitVec 64 :=
  let x1 := x + 0x9e3779b97f4a7c15#64
  let x2 := (x1 ^^^ (x1 >>> 30)) * 0xbf58476d1ce4e5b9#64
  let x3 := (x2 ^^^ (x2 >>> 27)) * 0x94d049bb133111eb#64
  x3 ^^^ (x3 >>> 31)

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words for `presolveSplitmix64`. -/
def splitmix64Words : Array UInt32 := #[
  0xd2907d61, 0xf2b016a1, 0xf2d0c8c1, 0xf2ec3901,
  0xcb010001, 0xca417821, 0xd29cb722, 0xf2a39c82,
  0xf2c8eda2, 0xf2f7eb02, 0x9b017c41, 0xca416c21,
  0xd2823d62, 0xf2a26622, 0xf2c93762, 0xf2f29a02,
  0x9b017c41, 0xca417c20, 0xd65f03c0
]

def splitmix64Targets (_ : Nat) : Option String := none

/-- Tier 5A theorem: decoded Go 1.27 ARM64 `presolveSplitmix64` matches `splitmix64Spec` on all 2^64 inputs. -/
private def splitmix64Decoded : Array Arm64Insn := #[
  .movWide true 2 1#5 33771 0,
  .movWide true 3 1#5 32949 16,
  .movWide true 3 1#5 34374 32,
  .movWide true 3 1#5 25032 48,
  .addSubReg true true false .lsl 0 1#5 0#5 1#5,
  .logicalReg true false 2 .lsr 30 1#5 1#5 1#5,
  .movWide true 2 2#5 58809 0,
  .movWide true 3 2#5 7396 16,
  .movWide true 3 2#5 18285 32,
  .movWide true 3 2#5 48984 48,
  .mul3 true false 1#5 2#5 1#5 31#5,
  .logicalReg true false 2 .lsr 27 1#5 1#5 1#5,
  .movWide true 2 2#5 4587 0,
  .movWide true 3 2#5 4913 16,
  .movWide true 3 2#5 18875 32,
  .movWide true 3 2#5 38096 48,
  .mul3 true false 1#5 2#5 1#5 31#5,
  .logicalReg true false 2 .lsr 31 0#5 1#5 1#5,
  .ret 30#5
]

private theorem splitmix64Decoded_eq : decodeArm64Prog splitmix64Words = splitmix64Decoded := rfl

private theorem movk64_step (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n imm16 shift : Nat) (rd : BitVec 5) (old val : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.movWide true 3 rd imm16 shift))
    (hOld : readGpr s rd = old)
    (hVal : (old &&& ~~~(0xffff#64 <<< shift)) ||| ((BitVec.ofNat 64 imm16) <<< shift) = val) :
    stepArm64 prog ext s = advancePc (writeGpr s rd val) := by
  rw [step_movK64 prog ext s k n rd imm16 shift hRun hPc hSize hFetch, hOld, hVal]

/-- Tier 5A theorem: decoded Go 1.27 ARM64 `presolveSplitmix64` matches `splitmix64Spec` on all 2^64 inputs. -/
theorem splitmix64_arm64_equiv (s : Arm64State) (x : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0) (hX0 : s.regs 0#5 = x) :
    let s' := runArm64 32 (decodeArm64Prog splitmix64Words) splitmix64Targets s
    s'.status = .returned ∧ s'.regs 0#5 = splitmix64Spec x := by
  dsimp only
  rw [splitmix64Decoded_eq]
  have hLen : splitmix64Decoded.size = 19 := rfl
  have hX0' : readGpr s 0#5 = x := by simp only [readGpr, show (0#5 == 31#5) = false from rfl]; exact hX0
  -- Step 0..3: R1 := 0x61c8864680b583eb#64 without nesting `writeGpr` chains across blocks.
  let c1 : BitVec 64 := 0x61c8864680b583eb#64
  have h0123 : ∃ s4,
      runArm64 4 splitmix64Decoded splitmix64Targets s = s4 ∧
      s4.status = .running ∧ s4.pcIdx = 4 ∧
      readGpr s4 0#5 = x ∧ readGpr s4 1#5 = c1 := by
    let s1 := advancePc (writeGpr s 1#5 (BitVec.ofNat 64 33771 <<< 0))
    have h0 : stepArm64 splitmix64Decoded splitmix64Targets s = s1 :=
      step_movZ64 _ _ s 0 19 1#5 33771 0 hRun hPc hLen rfl
    have hR1_1 : readGpr s1 1#5 = 0x83eb#64 := by
      have hV : BitVec.ofNat 64 33771 <<< 0 = 0x83eb#64 := rfl
      rw [show s1 = advancePc (writeGpr s 1#5 (BitVec.ofNat 64 33771 <<< 0)) from rfl,
        readGpr_advancePc, readGpr_writeGpr_eq s 1#5 _ (by decide), hV]
    let s2 := advancePc (writeGpr s1 1#5 0x80b583eb#64)
    have h1 : stepArm64 splitmix64Decoded splitmix64Targets s1 = s2 :=
      movk64_step splitmix64Decoded splitmix64Targets s1 1 19 32949 16 1#5 0x83eb#64 0x80b583eb#64
        (by simp only [s1, status_advancePc, status_writeGpr, hRun])
        (by simp only [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]) hLen rfl hR1_1 (by decide)
    have hR1_2 : readGpr s2 1#5 = 0x80b583eb#64 := by
      rw [show s2 = advancePc (writeGpr s1 1#5 0x80b583eb#64) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq s1 1#5 _ (by decide)]
    let s3 := advancePc (writeGpr s2 1#5 0x864680b583eb#64)
    have h2 : stepArm64 splitmix64Decoded splitmix64Targets s2 = s3 :=
      movk64_step splitmix64Decoded splitmix64Targets s2 2 19 34374 32 1#5 0x80b583eb#64 0x864680b583eb#64
        (by simp only [s2, s1, status_advancePc, status_writeGpr, hRun])
        (by simp only [s2, s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]) hLen rfl hR1_2 (by decide)
    have hR1_3 : readGpr s3 1#5 = 0x864680b583eb#64 := by
      rw [show s3 = advancePc (writeGpr s2 1#5 0x864680b583eb#64) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq s2 1#5 _ (by decide)]
    let s4 := advancePc (writeGpr s3 1#5 c1)
    have h3 : stepArm64 splitmix64Decoded splitmix64Targets s3 = s4 :=
      movk64_step splitmix64Decoded splitmix64Targets s3 3 19 25032 48 1#5 0x864680b583eb#64 c1
        (by simp only [s3, s2, s1, status_advancePc, status_writeGpr, hRun])
        (by simp only [s3, s2, s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]) hLen rfl hR1_3 (by decide)
    refine ⟨s4, ?_, ?_, ?_, ?_, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun, h0]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s1, status_advancePc, status_writeGpr, hRun]), h1]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s2, s1, status_advancePc, status_writeGpr, hRun]), h2]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s3, s2, s1, status_advancePc, status_writeGpr, hRun]), h3]
      rfl
    · simp only [s4, s3, s2, s1, status_advancePc, status_writeGpr, hRun]
    · simp only [s4, s3, s2, s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
    · rw [show s4 = advancePc (writeGpr s3 1#5 c1) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s3 = advancePc (writeGpr s2 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s2 = advancePc (writeGpr s1 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s1 = advancePc (writeGpr s 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hX0']
    · rw [show s4 = advancePc (writeGpr s3 1#5 c1) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
  obtain ⟨s4, hRun04, hRun4, hPc4, hR0_4, hR1_4⟩ := h0123
  -- Step 4..5: v1 := x - c1; v1b := v1 ^^^ (v1 >>> 30)
  let v1 := x - c1
  let v1b := v1 ^^^ (v1 >>> 30)
  have h45 : ∃ s6,
      runArm64 2 splitmix64Decoded splitmix64Targets s4 = s6 ∧
      s6.status = .running ∧ s6.pcIdx = 6 ∧ readGpr s6 1#5 = v1b := by
    let s5 := advancePc (writeGpr s4 1#5 (readGpr s4 0#5 - (readGpr s4 1#5 <<< (0 % 64))))
    have h4 : stepArm64 splitmix64Decoded splitmix64Targets s4 = s5 :=
      step_sub64Reg _ _ s4 4 19 1#5 0#5 1#5 hRun4 hPc4 hLen rfl
    have hR1_5 : readGpr s5 1#5 = v1 := by
      have hSh : c1 <<< (0 % 64) = c1 := by bv_decide
      rw [show s5 = advancePc (writeGpr s4 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hR0_4, hR1_4, hSh]
    let s6 := advancePc (writeGpr s5 1#5 (readGpr s5 1#5 ^^^ (readGpr s5 1#5 >>> (30 % 64))))
    have h5 : stepArm64 splitmix64Decoded splitmix64Targets s5 = s6 :=
      step_eor64Lsr _ _ s5 5 19 30 1#5 1#5 1#5
        (by simp only [s5, status_advancePc, status_writeGpr, hRun4])
        (by simp only [s5, pcIdx_advancePc, pcIdx_writeGpr, hPc4]) hLen rfl
    refine ⟨s6, ?_, ?_, ?_, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun4, h4]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s5, status_advancePc, status_writeGpr, hRun4]), h5]
      rfl
    · simp only [s6, s5, status_advancePc, status_writeGpr, hRun4]
    · simp only [s6, s5, pcIdx_advancePc, pcIdx_writeGpr, hPc4]
    · rw [show s6 = advancePc (writeGpr s5 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hR1_5]
  obtain ⟨s6, hRun46, hRun6, hPc6, hR1_6⟩ := h45
  -- Step 6..11: R2 := 0xbf58476d1ce4e5b9#64; v2 := v1b * c2; v2b := v2 ^^^ (v2 >>> 27)
  let c2 : BitVec 64 := 0xbf58476d1ce4e5b9#64
  let v2 := v1b * c2
  let v2b := v2 ^^^ (v2 >>> 27)
  have h6_11 : ∃ s12,
      runArm64 6 splitmix64Decoded splitmix64Targets s6 = s12 ∧
      s12.status = .running ∧ s12.pcIdx = 12 ∧ readGpr s12 1#5 = v2b := by
    let s7 := advancePc (writeGpr s6 2#5 (BitVec.ofNat 64 58809 <<< 0))
    have h6 : stepArm64 splitmix64Decoded splitmix64Targets s6 = s7 :=
      step_movZ64 _ _ s6 6 19 2#5 58809 0 hRun6 hPc6 hLen rfl
    have hR2_7 : readGpr s7 2#5 = 0xe5b9#64 := by
      have hV : BitVec.ofNat 64 58809 <<< 0 = 0xe5b9#64 := rfl
      rw [show s7 = advancePc (writeGpr s6 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hV]
    let s8 := advancePc (writeGpr s7 2#5 0x1ce4e5b9#64)
    have h7 : stepArm64 splitmix64Decoded splitmix64Targets s7 = s8 :=
      movk64_step splitmix64Decoded splitmix64Targets s7 7 19 7396 16 2#5 0xe5b9#64 0x1ce4e5b9#64
        (by simp only [s7, status_advancePc, status_writeGpr, hRun6])
        (by simp only [s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]) hLen rfl hR2_7 (by decide)
    have hR2_8 : readGpr s8 2#5 = 0x1ce4e5b9#64 := by
      rw [show s8 = advancePc (writeGpr s7 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s9 := advancePc (writeGpr s8 2#5 0x476d1ce4e5b9#64)
    have h8 : stepArm64 splitmix64Decoded splitmix64Targets s8 = s9 :=
      movk64_step splitmix64Decoded splitmix64Targets s8 8 19 18285 32 2#5 0x1ce4e5b9#64 0x476d1ce4e5b9#64
        (by simp only [s8, s7, status_advancePc, status_writeGpr, hRun6])
        (by simp only [s8, s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]) hLen rfl hR2_8 (by decide)
    have hR2_9 : readGpr s9 2#5 = 0x476d1ce4e5b9#64 := by
      rw [show s9 = advancePc (writeGpr s8 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s10 := advancePc (writeGpr s9 2#5 c2)
    have h9 : stepArm64 splitmix64Decoded splitmix64Targets s9 = s10 :=
      movk64_step splitmix64Decoded splitmix64Targets s9 9 19 48984 48 2#5 0x476d1ce4e5b9#64 c2
        (by simp only [s9, s8, s7, status_advancePc, status_writeGpr, hRun6])
        (by simp only [s9, s8, s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]) hLen rfl hR2_9 (by decide)
    have hR2_10 : readGpr s10 2#5 = c2 := by
      rw [show s10 = advancePc (writeGpr s9 2#5 c2) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    have hR1_10 : readGpr s10 1#5 = v1b := by
      rw [show s10 = advancePc (writeGpr s9 2#5 c2) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s9 = advancePc (writeGpr s8 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s8 = advancePc (writeGpr s7 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s7 = advancePc (writeGpr s6 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_6]
    let s11 := advancePc (writeGpr s10 1#5 (readGpr s10 31#5 + readGpr s10 2#5 * readGpr s10 1#5))
    have h10 : stepArm64 splitmix64Decoded splitmix64Targets s10 = s11 :=
      step_mul64 _ _ s10 10 19 1#5 2#5 1#5 31#5
        (by simp only [s10, s9, s8, s7, status_advancePc, status_writeGpr, hRun6])
        (by simp only [s10, s9, s8, s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]) hLen rfl
    have hR1_11 : readGpr s11 1#5 = v2 := by
      have hZR : readGpr s10 31#5 = 0#64 := rfl
      rw [show s11 = advancePc (writeGpr s10 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hZR, hR2_10, hR1_10]
      bv_decide
    let s12 := advancePc (writeGpr s11 1#5 (readGpr s11 1#5 ^^^ (readGpr s11 1#5 >>> (27 % 64))))
    have h11 : stepArm64 splitmix64Decoded splitmix64Targets s11 = s12 :=
      step_eor64Lsr _ _ s11 11 19 27 1#5 1#5 1#5
        (by simp only [s11, s10, s9, s8, s7, status_advancePc, status_writeGpr, hRun6])
        (by simp only [s11, s10, s9, s8, s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]) hLen rfl
    refine ⟨s12, ?_, ?_, ?_, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun6, h6]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s7, status_advancePc, status_writeGpr, hRun6]), h7]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s8, s7, status_advancePc, status_writeGpr, hRun6]), h8]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s9, s8, s7, status_advancePc, status_writeGpr, hRun6]), h9]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s10, s9, s8, s7, status_advancePc, status_writeGpr, hRun6]), h10]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s11, s10, s9, s8, s7, status_advancePc, status_writeGpr, hRun6]), h11]
      rfl
    · simp only [s12, s11, s10, s9, s8, s7, status_advancePc, status_writeGpr, hRun6]
    · simp only [s12, s11, s10, s9, s8, s7, pcIdx_advancePc, pcIdx_writeGpr, hPc6]
    · rw [show s12 = advancePc (writeGpr s11 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hR1_11]
  obtain ⟨s12, hRun612, hRun12, hPc12, hR1_12⟩ := h6_11
  -- Step 12..18: R2 := 0x94d049bb133111eb#64; v3 := v2b * c3; R0 := v3 ^^^ (v3 >>> 31); RET
  let c3 : BitVec 64 := 0x94d049bb133111eb#64
  let v3 := v2b * c3
  let out := v3 ^^^ (v3 >>> 31)
  have h12_19 : ∃ s19,
      runArm64 20 splitmix64Decoded splitmix64Targets s12 = s19 ∧
      s19.status = .returned ∧ s19.regs 0#5 = out := by
    let s13 := advancePc (writeGpr s12 2#5 (BitVec.ofNat 64 4587 <<< 0))
    have h12 : stepArm64 splitmix64Decoded splitmix64Targets s12 = s13 :=
      step_movZ64 _ _ s12 12 19 2#5 4587 0 hRun12 hPc12 hLen rfl
    have hR2_13 : readGpr s13 2#5 = 0x11eb#64 := by
      have hV : BitVec.ofNat 64 4587 <<< 0 = 0x11eb#64 := rfl
      rw [show s13 = advancePc (writeGpr s12 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hV]
    let s14 := advancePc (writeGpr s13 2#5 0x133111eb#64)
    have h13 : stepArm64 splitmix64Decoded splitmix64Targets s13 = s14 :=
      movk64_step splitmix64Decoded splitmix64Targets s13 13 19 4913 16 2#5 0x11eb#64 0x133111eb#64
        (by simp only [s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl hR2_13 (by decide)
    have hR2_14 : readGpr s14 2#5 = 0x133111eb#64 := by
      rw [show s14 = advancePc (writeGpr s13 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s15 := advancePc (writeGpr s14 2#5 0x49bb133111eb#64)
    have h14 : stepArm64 splitmix64Decoded splitmix64Targets s14 = s15 :=
      movk64_step splitmix64Decoded splitmix64Targets s14 14 19 18875 32 2#5 0x133111eb#64 0x49bb133111eb#64
        (by simp only [s14, s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s14, s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl hR2_14 (by decide)
    have hR2_15 : readGpr s15 2#5 = 0x49bb133111eb#64 := by
      rw [show s15 = advancePc (writeGpr s14 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s16 := advancePc (writeGpr s15 2#5 c3)
    have h15 : stepArm64 splitmix64Decoded splitmix64Targets s15 = s16 :=
      movk64_step splitmix64Decoded splitmix64Targets s15 15 19 38096 48 2#5 0x49bb133111eb#64 c3
        (by simp only [s15, s14, s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s15, s14, s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl hR2_15 (by decide)
    have hR2_16 : readGpr s16 2#5 = c3 := by
      rw [show s16 = advancePc (writeGpr s15 2#5 c3) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    have hR1_16 : readGpr s16 1#5 = v2b := by
      rw [show s16 = advancePc (writeGpr s15 2#5 c3) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s15 = advancePc (writeGpr s14 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s14 = advancePc (writeGpr s13 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide),
        show s13 = advancePc (writeGpr s12 2#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_12]
    let s17 := advancePc (writeGpr s16 1#5 (readGpr s16 31#5 + readGpr s16 2#5 * readGpr s16 1#5))
    have h16 : stepArm64 splitmix64Decoded splitmix64Targets s16 = s17 :=
      step_mul64 _ _ s16 16 19 1#5 2#5 1#5 31#5
        (by simp only [s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s16, s15, s14, s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl
    have hR1_17 : readGpr s17 1#5 = v3 := by
      have hZR : readGpr s16 31#5 = 0#64 := rfl
      rw [show s17 = advancePc (writeGpr s16 1#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hZR, hR2_16, hR1_16]
      bv_decide
    let s18 := advancePc (writeGpr s17 0#5 (readGpr s17 1#5 ^^^ (readGpr s17 1#5 >>> (31 % 64))))
    have h17 : stepArm64 splitmix64Decoded splitmix64Targets s17 = s18 :=
      step_eor64Lsr _ _ s17 17 19 31 0#5 1#5 1#5
        (by simp only [s17, s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s17, s16, s15, s14, s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl
    let s19 := { s18 with status := .returned }
    have h18 : stepArm64 splitmix64Decoded splitmix64Targets s18 = s19 :=
      step_ret _ _ s18 18 19 30#5
        (by simp only [s18, s17, s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12])
        (by simp only [s18, s17, s16, s15, s14, s13, pcIdx_advancePc, pcIdx_writeGpr, hPc12]) hLen rfl
    refine ⟨s19, ?_, rfl, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun12, h12]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s13, status_advancePc, status_writeGpr, hRun12]), h13]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s14, s13, status_advancePc, status_writeGpr, hRun12]), h14]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s15, s14, s13, status_advancePc, status_writeGpr, hRun12]), h15]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12]), h16]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s17, s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12]), h17]
      rw [runArm64_succ_running _ _ _ _ (by simp only [s18, s17, s16, s15, s14, s13, status_advancePc, status_writeGpr, hRun12]), h18]
      rw [runArm64_of_not_running _ _ _ _ (by simp only [s19]; decide)]
    · have hOut : readGpr s18 0#5 = out := by
        rw [show s18 = advancePc (writeGpr s17 0#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_eq _ _ _ (by decide), hR1_17]
      dsimp only [readGpr] at hOut
      rw [show (0#5 == 31#5) = false from rfl] at hOut
      exact hOut
  obtain ⟨s19, hRun1220, hRet19, hReg19⟩ := h12_19
  have hCompose : runArm64 32 splitmix64Decoded splitmix64Targets s = s19 := by
    rw [show (32 : Nat) = 4 + 28 by decide, runArm64_add 4 28, hRun04]
    rw [show (28 : Nat) = 2 + 26 by decide, runArm64_add 2 26, hRun46]
    rw [show (26 : Nat) = 6 + 20 by decide, runArm64_add 6 20, hRun612, hRun1220]
  rw [hCompose]
  refine ⟨hRet19, ?_⟩
  rw [hReg19]
  unfold splitmix64Spec
  dsimp [out, v3, v2b, v2, v1b, v1, c1, c2, c3]
  bv_decide

/-! ### `canonicalFloatBits` Equivalence (Inlined & Frame-32 Variants) -/

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words for inlined `canonicalFloatBits`. -/
def canonicalFloatBitsInlinedWords : Array UInt32 := #[
  0x1e602008, 0x54000061, 0xaa1f03e0, 0xd65f03c0, 0x9e660000, 0xd65f03c0
]

def canonicalFloatBitsInlinedTargets (_ : Nat) : Option String := none

private def canonicalFloatBitsInlinedDecoded : Array Arm64Insn := #[
  .fcmpD true 0#5 0#5,
  .bCond .ne 3,
  .logicalReg true false 1 .lsl 0 0#5 31#5 31#5,
  .ret 30#5,
  .fpIntConv false false false 0#5 0#5,
  .ret 30#5
]

private theorem canonicalFloatBitsInlinedDecoded_eq :
    decodeArm64Prog canonicalFloatBitsInlinedWords = canonicalFloatBitsInlinedDecoded := rfl

/-- Tier 5A theorem: decoded inlined `canonicalFloatBits` maps `±0.0` to `0` and leaves all other float bits intact. -/
theorem canonicalFloatBitsInlined_arm64_equiv (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0) :
    let f0 := s.fpRegs 0#5
    let s' := runArm64 16 (decodeArm64Prog canonicalFloatBitsInlinedWords)
      canonicalFloatBitsInlinedTargets s
    s'.status = .returned ∧
      s'.regs 0#5 = (if f0 = 0#64 ∨ f0 = 0x8000000000000000#64 then 0#64 else f0) := by
  dsimp only
  rw [canonicalFloatBitsInlinedDecoded_eq]
  have hLen : canonicalFloatBitsInlinedDecoded.size = 6 := rfl
  let f0 := s.fpRegs 0#5
  let s1 := advancePc { s with nzcv := f64Cmp f0 0#64 }
  have h0 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s = s1 :=
    step_fcmpD_zero _ _ s 0 6 0#5 0#5 hRun hPc hLen rfl
  have hRun1 : s1.status = .running := hRun
  have hPc1 : s1.pcIdx = 1 := by simp only [s1, advancePc]; omega
  by_cases hz : f0 = 0#64 ∨ f0 = 0x8000000000000000#64
  · have hNzcvZ : (f64Cmp f0 0#64).z = true := (f64Cmp_zero_z_iff f0).mpr hz
    have hCondFalse : evalCond .ne s1.nzcv = false := by simp only [evalCond, s1, advancePc, hNzcvZ]; rfl
    let s2 := advancePc s1
    have hRun2 : s2.status = .running := by rw [status_advancePc]; exact hRun1
    have h1 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s1 = s2 :=
      step_bCond_false _ _ s1 1 6 .ne 3 hRun1 hPc1 hLen rfl hCondFalse
    let s3 := advancePc (writeGpr s2 0#5 (readGpr s2 31#5 ||| ((readGpr s2 31#5) <<< (0 % 64))))
    have hRun3 : s3.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun2
    have h2 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s2 = s3 :=
      step_orr64_lsl0 _ _ s2 2 6 0#5 31#5 31#5 hRun2 (by simp only [s2, advancePc, hPc1]) hLen rfl
    let s4 := { s3 with status := .returned }
    have h3 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s3 = s4 :=
      step_ret _ _ s3 3 6 30#5 hRun3
        (by simp only [s3, s2, pcIdx_advancePc, pcIdx_writeGpr, hPc1]) hLen rfl
    rw [runArm64_succ_running _ _ _ _ hRun, h0]
    rw [runArm64_succ_running _ _ _ _ hRun1, h1]
    rw [runArm64_succ_running _ _ _ _ hRun2, h2]
    rw [runArm64_succ_running _ _ _ _ hRun3, h3]
    rw [runArm64_of_not_running _ _ _ _ (by simp only [s4]; decide)]
    refine ⟨rfl, ?_⟩
    rw [ite_eq_left hz]
    have hZero : readGpr s2 31#5 ||| ((readGpr s2 31#5) <<< (0 % 64)) = 0#64 := by
      have hZR : readGpr s2 31#5 = 0#64 := by simp [readGpr]
      rw [hZR]; rfl
    have hR0 : readGpr s3 0#5 = 0#64 := by
      rw [show s3 = advancePc (writeGpr s2 0#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hZero]
    dsimp only [readGpr] at hR0
    rw [show (0#5 == 31#5) = false from rfl] at hR0
    exact hR0
  · have hNzcvZ : (f64Cmp f0 0#64).z = false := by
      cases h : (f64Cmp f0 0#64).z
      · rfl
      · exact False.elim (hz ((f64Cmp_zero_z_iff f0).mp h))
    have hCondTrue : evalCond .ne s1.nzcv = true := by simp only [evalCond, s1, advancePc, hNzcvZ]; rfl
    let s2 := { s1 with pcIdx := 4 }
    have hRun2 : s2.status = .running := hRun1
    have h1 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s1 = s2 := by
      have h := step_bCond_true canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s1 1 6
        .ne 3 hRun1 hPc1 hLen rfl hCondTrue (by decide) (by decide)
      simpa using h
    let s3 := advancePc (writeGpr s2 0#5 (s2.fpRegs 0#5))
    have hRun3 : s3.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun2
    have h2 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s2 = s3 :=
      step_fmovDToGpr _ _ s2 4 6 0#5 0#5 hRun2 rfl hLen rfl
    let s4 := { s3 with status := .returned }
    have h3 : stepArm64 canonicalFloatBitsInlinedDecoded canonicalFloatBitsInlinedTargets s3 = s4 :=
      step_ret _ _ s3 5 6 30#5 hRun3
        (by simp only [s3, s2, pcIdx_advancePc, pcIdx_writeGpr]) hLen rfl
    rw [runArm64_succ_running _ _ _ _ hRun, h0]
    rw [runArm64_succ_running _ _ _ _ hRun1, h1]
    rw [runArm64_succ_running _ _ _ _ hRun2, h2]
    rw [runArm64_succ_running _ _ _ _ hRun3, h3]
    rw [runArm64_of_not_running _ _ _ _ (by simp only [s4]; decide)]
    refine ⟨rfl, ?_⟩
    rw [ite_eq_right hz]
    have hR0 : readGpr s3 0#5 = f0 := by
      rw [show s3 = advancePc (writeGpr s2 0#5 (s2.fpRegs 0#5)) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
      rfl
    dsimp only [readGpr] at hR0
    rw [show (0#5 == 31#5) = false from rfl] at hR0
    exact hR0

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words for non-inlined `canonicalFloatBits`. -/
def canonicalFloatBitsWords : Array UInt32 := #[
  0xf9400b90, 0xeb3063ff, 0x540001c9, 0xf81e0ffe,
  0xf81f83fd, 0xd10023fd, 0x1e602008, 0x540000a1,
  0xaa1f03e0, 0xf85f83fd, 0xf84207fe, 0xd65f03c0,
  0x97fa21d8, 0xf85f83fd, 0xf84207fe, 0xd65f03c0,
  0xfd0007e0, 0xaa1e03e3, 0x97f92832, 0xfd4007e0,
  0x17ffffec
]

def canonicalFloatBitsTargets (idx : Nat) : Option String :=
  if idx == 12 then some "math.Float64bits"
  else if idx == 18 then some "runtime.morestack_noctxt.abi0"
  else none

private def canonicalFloatBitsDecoded : Array Arm64Insn := #[
  .ldrStrUImm true false 8 16 16#5 28#5,
  .addSubExt true true true .uxtx 0 31#5 31#5 16#5,
  .bCond .ls 14,
  .ldrStrImm false false 8 .pre (-32) 30#5 31#5,
  .ldrStrImm false false 8 .offset (-8) 29#5 31#5,
  .addSubImm true true false 29#5 31#5 8#64,
  .fcmpD true 0#5 0#5,
  .bCond .ne 5,
  .logicalReg true false 1 .lsl 0 0#5 31#5 31#5,
  .ldrStrImm true false 8 .offset (-8) 29#5 31#5,
  .ldrStrImm true false 8 .post 32 30#5 31#5,
  .ret 30#5,
  .bl (-384552),
  .ldrStrImm true false 8 .offset (-8) 29#5 31#5,
  .ldrStrImm true false 8 .post 32 30#5 31#5,
  .ret 30#5,
  .ldrStrUImm false true 8 8 0#5 31#5,
  .logicalReg true false 1 .lsl 0 3#5 31#5 30#5,
  .bl (-448462),
  .ldrStrUImm true true 8 8 0#5 31#5,
  .b (-20)
]

private theorem canonicalFloatBitsDecoded_eq :
    decodeArm64Prog canonicalFloatBitsWords = canonicalFloatBitsDecoded := rfl

/-- Tier 5A theorem: frame-32 `canonicalFloatBits` never triggers `morestack` when `sp - 32 > gStackGuard0`, returns `0` on `±0.0`, and delegates nonzero floats to `math.Float64bits`. -/
theorem canonicalFloatBits_arm64_equiv (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hSp : BitVec.ult 31#64 s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - 32#64) = true)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0) :
    let f0 := s.fpRegs 0#5
    let s' := runArm64 32 (decodeArm64Prog canonicalFloatBitsWords) canonicalFloatBitsTargets s
    s'.status ≠ .trapMorestack ∧ s'.status ≠ .trapPanicBounds ∧ s'.status ≠ .trapPanicDivide ∧
      (if f0 = 0#64 ∨ f0 = 0x8000000000000000#64 then
        s'.status = .returned ∧ s'.regs 0#5 = 0#64
      else
        s'.status = .callExternal "math.Float64bits" ∧ s'.fpRegs 0#5 = f0) := by
  dsimp only
  rw [canonicalFloatBitsDecoded_eq]
  have hLen : canonicalFloatBitsDecoded.size = 21 := rfl
  let f0 := s.fpRegs 0#5
  have hPre : ∃ s7,
      runArm64 7 canonicalFloatBitsDecoded canonicalFloatBitsTargets s = s7 ∧
      s7.status = .running ∧ s7.pcIdx = 7 ∧
      s7.fpRegs 0#5 = f0 ∧ s7.nzcv = f64Cmp f0 0#64 := by
    have hR28 : readGprSP s 28#5 = s.regs 28#5 := rfl
    let s1 := advancePc (writeGpr s 16#5 s.gStackGuard0)
    have h0 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s = s1 := by
      have hs := step_ldrStrUImm_load64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s 0 21
        16 16#5 28#5 hRun hPc hLen rfl
      have hAddr : readGprSP s 28#5 + BitVec.ofNat 64 16 = s.regs 28#5 + 16#64 := by rw [hR28]
      rw [hs, hAddr, hG]
    have hRun1 : s1.status = .running := by simp only [s1, status_advancePc, status_writeGpr, hRun]
    have hPc1 : s1.pcIdx = 1 := by simp only [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
    have hR16_1 : readGpr s1 16#5 = s.gStackGuard0 := by
      rw [show s1 = advancePc (writeGpr s 16#5 s.gStackGuard0) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    have hSp1 : readGprSP s1 31#5 = s.sp := rfl
    have hCmpVal : (readGpr s1 16#5) <<< (0 % 5) = s.gStackGuard0 := by
      rw [hR16_1]; bv_decide
    let s2 := advancePc (writeGpr { s1 with nzcv := subFlags64 s.sp s.gStackGuard0 } 31#5 (s.sp - s.gStackGuard0))
    have h1 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s1 = s2 := by
      have hs := step_addSubExt_cmp64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s1 1 21
        31#5 16#5 hRun1 hPc1 hLen rfl
      rw [hs, hSp1, hCmpVal]
    have hRun2 : s2.status = .running := by simp only [s2, status_advancePc, status_writeGpr, hRun1]
    have hPc2 : s2.pcIdx = 2 := by simp only [s2, pcIdx_advancePc, pcIdx_writeGpr, hPc1]
    have hNoStackTrap : evalCond .ls s2.nzcv = false := by
      have hNz : s2.nzcv = subFlags64 s.sp s.gStackGuard0 := rfl
      rw [hNz, evalCond_ls_subFlags64, stackGuard_32_ok s.sp s.gStackGuard0 hSp hGuard]
      rfl
    let s3 := advancePc s2
    have hRun3 : s3.status = .running := by rw [status_advancePc]; exact hRun2
    have hPc3 : s3.pcIdx = 3 := by rw [pcIdx_advancePc, hPc2]
    have h2 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s2 = s3 :=
      step_bCond_false _ _ s2 2 21 .ls 14 hRun2 hPc2 hLen rfl hNoStackTrap
    let addr3 := readGprSP s3 31#5 + BitVec.ofInt 64 (-32)
    let s4 := advancePc (writeGprSP { s3 with mem := writeU64 s3.mem addr3 (readGpr s3 30#5) } 31#5 addr3)
    have h3 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s3 = s4 :=
      step_ldrStrImm_pre_store64 _ _ s3 3 21 (-32) 30#5 31#5 hRun3 hPc3 hLen rfl
    have hRun4 : s4.status = .running := by rw [status_advancePc, status_writeGprSP]; exact hRun3
    have hPc4 : s4.pcIdx = 4 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, hPc3]
    let addr4 := readGprSP s4 31#5 + BitVec.ofInt 64 (-8)
    let s5 := advancePc { s4 with mem := writeU64 s4.mem addr4 (readGpr s4 29#5) }
    have hRun5 : s5.status = .running := by rw [status_advancePc]; exact hRun4
    have hPc5 : s5.pcIdx = 5 := by rw [pcIdx_advancePc, hPc4]
    have h4 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s4 = s5 :=
      step_ldrStrImm_off_store64 _ _ s4 4 21 (-8) 29#5 31#5 hRun4 hPc4 hLen rfl
    let s6 := advancePc (writeGprSP s5 29#5 (readGprSP s5 31#5 - 8#64))
    have hRun6 : s6.status = .running := by rw [status_advancePc, status_writeGprSP]; exact hRun5
    have hPc6 : s6.pcIdx = 6 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, hPc5]
    have h5 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s5 = s6 :=
      step_addSubImm_sub64 _ _ s5 5 21 29#5 31#5 8#64 hRun5 hPc5 hLen rfl
    have hFp6 : s6.fpRegs 0#5 = f0 := rfl
    let s7 := advancePc { s6 with nzcv := f64Cmp (s6.fpRegs 0#5) 0#64 }
    have h6 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s6 = s7 :=
      step_fcmpD_zero _ _ s6 6 21 0#5 0#5 hRun6 hPc6 hLen rfl
    refine ⟨s7, ?_, ?_, ?_, hFp6, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun, h0]
      rw [runArm64_succ_running _ _ _ _ hRun1, h1]
      rw [runArm64_succ_running _ _ _ _ hRun2, h2]
      rw [runArm64_succ_running _ _ _ _ hRun3, h3]
      rw [runArm64_succ_running _ _ _ _ hRun4, h4]
      rw [runArm64_succ_running _ _ _ _ hRun5, h5]
      rw [runArm64_succ_running _ _ _ _ hRun6, h6]
      rfl
    · rw [status_advancePc]; exact hRun6
    · rw [pcIdx_advancePc, hPc6]
    · rfl
  obtain ⟨s7, hRun07, hRun7, hPc7, hFp7, hNzcv7⟩ := hPre
  rw [show (32 : Nat) = 7 + 25 by decide, runArm64_add 7 25, hRun07]
  by_cases hz : f0 = 0#64 ∨ f0 = 0x8000000000000000#64
  · have hZ : s7.nzcv.z = true := by rw [hNzcv7]; exact (f64Cmp_zero_z_iff f0).mpr hz
    have hCondFalse : evalCond .ne s7.nzcv = false := by simp only [evalCond, hZ]; rfl
    let s8 := advancePc s7
    have hRun8 : s8.status = .running := by rw [status_advancePc]; exact hRun7
    have hPc8 : s8.pcIdx = 8 := by rw [pcIdx_advancePc, hPc7]
    have h7 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s7 = s8 :=
      step_bCond_false _ _ s7 7 21 .ne 5 hRun7 hPc7 hLen rfl hCondFalse
    let s9 := advancePc (writeGpr s8 0#5 (readGpr s8 31#5 ||| ((readGpr s8 31#5) <<< (0 % 64))))
    have hRun9 : s9.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun8
    have hPc9 : s9.pcIdx = 9 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc8]
    have h8 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s8 = s9 :=
      step_orr64_lsl0 _ _ s8 8 21 0#5 31#5 31#5 hRun8 hPc8 hLen rfl
    let s10 := advancePc (writeGpr s9 29#5 (readU64 s9.mem (readGprSP s9 31#5 + BitVec.ofInt 64 (-8))))
    have hRun10 : s10.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun9
    have hPc10 : s10.pcIdx = 10 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc9]
    have h9 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s9 = s10 :=
      step_ldrStrImm_off_load64 _ _ s9 9 21 (-8) 29#5 31#5 hRun9 hPc9 hLen rfl
    let base10 := readGprSP s10 31#5
    let s11 := advancePc (writeGprSP (writeGpr s10 30#5 (readU64 s10.mem base10)) 31#5 (base10 + BitVec.ofInt 64 32))
    have hRun11 : s11.status = .running := by rw [status_advancePc, status_writeGprSP, status_writeGpr]; exact hRun10
    have hPc11 : s11.pcIdx = 11 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, pcIdx_writeGpr, hPc10]
    have h10 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s10 = s11 :=
      step_ldrStrImm_post_load64 _ _ s10 10 21 32 30#5 31#5 hRun10 hPc10 hLen rfl
    let s12 := { s11 with status := .returned }
    have h11 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s11 = s12 :=
      step_ret _ _ s11 11 21 30#5 hRun11 hPc11 hLen rfl
    have hSt12 : s12.status = .returned := rfl
    rw [runArm64_succ_running _ _ _ _ hRun7, h7]
    rw [runArm64_succ_running _ _ _ _ hRun8, h8]
    rw [runArm64_succ_running _ _ _ _ hRun9, h9]
    rw [runArm64_succ_running _ _ _ _ hRun10, h10]
    rw [runArm64_succ_running _ _ _ _ hRun11, h11]
    rw [runArm64_of_not_running _ _ _ _ (by rw [hSt12]; decide)]
    rw [ite_eq_left hz]
    refine ⟨by rw [hSt12]; decide, by rw [hSt12]; decide, by rw [hSt12]; decide, hSt12, ?_⟩
    have hZero : readGpr s8 31#5 ||| ((readGpr s8 31#5) <<< (0 % 64)) = 0#64 := by
      have hZR : readGpr s8 31#5 = 0#64 := by simp [readGpr]
      rw [hZR]; rfl
    have hR0_9 : readGpr s9 0#5 = 0#64 := by
      rw [show s9 = advancePc (writeGpr s8 0#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide), hZero]
    have hR0_10 : readGpr s10 0#5 = 0#64 := by
      rw [show s10 = advancePc (writeGpr s9 29#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_9]
    have hR0_11 : readGpr s11 0#5 = 0#64 := by
      rw [show s11 = advancePc (writeGprSP (writeGpr s10 30#5 _) 31#5 _) from rfl,
        readGpr_advancePc, readGpr_writeGprSP_sp, readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_10]
    have hR0_11_regs : s11.regs 0#5 = 0#64 := by
      rw [← readGpr_eq_regs s11 0#5 (by decide)]
      exact hR0_11
    exact hR0_11_regs
  · have hZ : s7.nzcv.z = false := by
      rw [hNzcv7]
      cases h : (f64Cmp f0 0#64).z
      · rfl
      · exact False.elim (hz ((f64Cmp_zero_z_iff f0).mp h))
    have hCondTrue : evalCond .ne s7.nzcv = true := by simp only [evalCond, hZ]; rfl
    let s8 := { s7 with pcIdx := 12 }
    have hRun8 : s8.status = .running := hRun7
    have h7 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s7 = s8 :=
      step_bCond_true canonicalFloatBitsDecoded canonicalFloatBitsTargets s7 7 21
        .ne 5 hRun7 hPc7 hLen rfl hCondTrue (by decide) (by decide)
    let s9 := { writeGpr s8 30#5 (s8.baseAddr + BitVec.ofNat 64 (4 * 13)) with status := .callExternal "math.Float64bits" }
    have h8 : stepArm64 canonicalFloatBitsDecoded canonicalFloatBitsTargets s8 = s9 := by
      exact step_bl_callExt canonicalFloatBitsDecoded canonicalFloatBitsTargets s8 12 21 (-384552)
        "math.Float64bits" hRun8 rfl hLen rfl (by decide) rfl
        (by decide) (by decide) (by decide) (by decide)
    have hSt9 : s9.status = .callExternal "math.Float64bits" := rfl
    rw [runArm64_succ_running _ _ _ _ hRun7, h7]
    rw [runArm64_succ_running _ _ _ _ hRun8, h8]
    rw [runArm64_of_not_running _ _ _ _ (by rw [hSt9]; decide)]
    rw [ite_eq_right hz]
    refine ⟨by rw [hSt9]; decide, by rw [hSt9]; decide, by rw [hSt9]; decide, hSt9, ?_⟩
    exact hFp7

/-! ### `divCeilInt64` & `gcdInt64` Arithmetic Bridges and Equivalences -/

/-- Exact mathematical specification of `divCeilInt64` (Go truncated division rounded toward `+∞`). -/
def divCeilSpec (a d : Int) : Int :=
  let q := a.tdiv d
  if a.tmod d > 0 then q + 1 else q

private theorem bmod_pow64_eq_self (x : Int) (hLo : -(2 ^ 63 : Int) ≤ x) (hHi : x < 2 ^ 63) :
    x.bmod (2 ^ 64) = x := by
  rw [Int.bmod_def]
  omega

private theorem tdiv_64_in_range (a d : BitVec 64) (hd : 0 < d.toInt) :
    -(2 ^ 63 : Int) ≤ a.toInt.tdiv d.toInt ∧ a.toInt.tdiv d.toInt < 2 ^ 63 := by
  have ha1 := BitVec.le_toInt a
  have ha2 := BitVec.toInt_lt (x := a)
  have hAbs := Int.natAbs_tdiv_le_natAbs a.toInt d.toInt
  constructor
  · omega
  · by_cases ha : 0 ≤ a.toInt
    · rw [Int.tdiv_eq_ediv_of_nonneg ha]
      have hle : a.toInt / d.toInt ≤ a.toInt := Int.ediv_le_self d.toInt ha
      omega
    · have haPos : 0 ≤ -a.toInt := by omega
      have hNeg := Int.neg_tdiv (-a.toInt) d.toInt
      simp only [Int.neg_neg] at hNeg
      have hPosDiv : 0 ≤ (-a.toInt).tdiv d.toInt := Int.tdiv_nonneg haPos (by omega)
      omega

private theorem divCeil_bv64_eq (a d : BitVec 64) (hd : 0 < d.toInt) :
    let q := a.sdiv d
    let rem := a - q * d
    let res := if !(BitVec.slt 0#64 rem) then q else q + 1#64
    res.toInt = divCeilSpec a.toInt d.toInt := by
  have hqRange := tdiv_64_in_range a d hd
  have hqEq : (a.sdiv d).toInt = a.toInt.tdiv d.toInt := by
    rw [BitVec.toInt_sdiv, bmod_pow64_eq_self _ hqRange.1 hqRange.2]
  have ha1 := BitVec.le_toInt a
  have ha2 := BitVec.toInt_lt (x := a)
  have hd2 := BitVec.toInt_lt (x := d)
  have hrBound : -(2 ^ 63 : Int) < a.toInt.tmod d.toInt ∧ a.toInt.tmod d.toInt < 2 ^ 63 := by
    by_cases ha : 0 ≤ a.toInt
    · have h0 := Int.tmod_nonneg d.toInt ha
      have hlt := Int.tmod_lt_of_pos a.toInt hd
      omega
    · have hNegEq : a.toInt.tmod d.toInt = -((-a.toInt).tmod d.toInt) := by
        have h := Int.neg_tmod (-a.toInt) d.toInt
        simp only [Int.neg_neg] at h
        exact h
      have haPos : 0 ≤ -a.toInt := by omega
      have h0 := Int.tmod_nonneg d.toInt haPos
      have hlt := Int.tmod_lt_of_pos (-a.toInt) hd
      omega
  have hMulComm : a.toInt.tdiv d.toInt * d.toInt = d.toInt * a.toInt.tdiv d.toInt := Int.mul_comm _ _
  have hMulEqInt : a.toInt.tdiv d.toInt * d.toInt = a.toInt - a.toInt.tmod d.toInt := by
    have := Int.mul_tdiv_add_tmod a.toInt d.toInt
    rw [hMulComm]; omega
  have hMulRange : -(2 ^ 63 : Int) ≤ (a.sdiv d).toInt * d.toInt ∧ (a.sdiv d).toInt * d.toInt < 2 ^ 63 := by
    rw [hqEq, hMulEqInt]
    by_cases ha : 0 ≤ a.toInt
    · have h0 := Int.tmod_nonneg d.toInt ha
      omega
    · have haPos : 0 ≤ -a.toInt := by omega
      have hNegEq : a.toInt.tmod d.toInt = -((-a.toInt).tmod d.toInt) := by
        have h := Int.neg_tmod (-a.toInt) d.toInt
        simp only [Int.neg_neg] at h
        exact h
      have h0 := Int.tmod_nonneg d.toInt haPos
      omega
  have hMulEq : ((a.sdiv d) * d).toInt = a.toInt - a.toInt.tmod d.toInt := by
    rw [BitVec.toInt_mul, bmod_pow64_eq_self _ hMulRange.1 hMulRange.2, hqEq, hMulEqInt]
  have hRemEq : (a - (a.sdiv d) * d).toInt = a.toInt.tmod d.toInt := by
    rw [BitVec.toInt_sub, hMulEq]
    have heq : a.toInt - (a.toInt - a.toInt.tmod d.toInt) = a.toInt.tmod d.toInt := by omega
    rw [heq]
    exact bmod_pow64_eq_self _ (Int.le_of_lt hrBound.1) hrBound.2
  dsimp only [divCeilSpec]
  have hSlt : BitVec.slt 0#64 (a - (a.sdiv d) * d) = decide (0 < a.toInt.tmod d.toInt) := by
    rw [BitVec.slt_eq_decide, BitVec.toInt_zero, hRemEq]
  rw [hSlt]
  by_cases hr : 0 < a.toInt.tmod d.toInt
  · simp only [hr, decide_true, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
    have hdGe2 : 2 ≤ d.toInt := by
      by_cases h1 : d.toInt = 1
      · exfalso; rw [h1, Int.tmod_one] at hr; omega
      · omega
    have hqTop : a.toInt.tdiv d.toInt < 2 ^ 63 - 1 := by
      have haPos : 0 ≤ a.toInt := by
        by_cases haPos' : 0 ≤ a.toInt
        · exact haPos'
        · exfalso
          have haNegPos : 0 ≤ -a.toInt := by omega
          have hNegT : (-a.toInt).tmod d.toInt ≥ 0 := Int.tmod_nonneg d.toInt haNegPos
          have hRel := Int.neg_tmod (-a.toInt) d.toInt
          simp only [Int.neg_neg] at hRel
          omega
      rw [Int.tdiv_eq_ediv_of_nonneg haPos]
      have hMul2 : (2 ^ 63 - 1 : Int) * 2 ≤ (2 ^ 63 - 1) * d.toInt :=
        Int.mul_le_mul_of_nonneg_left hdGe2 (by omega)
      have hBase : (2 ^ 63 : Int) ≤ (2 ^ 63 - 1) * 2 := by omega
      exact Int.ediv_lt_of_lt_mul hd (by omega)
    rw [BitVec.toInt_add, BitVec.toInt_one (by decide), hqEq]
    exact bmod_pow64_eq_self _ (by omega) (by omega)
  · simp [hr, hqEq]

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words for `divCeilInt64`. -/
def divCeilInt64Words : Array UInt32 := #[
  0xf9400b90, 0xeb3063ff, 0x540001e9, 0xf81f0ffe,
  0xf81f83fd, 0xd10023fd, 0xb4000121, 0x9ac10c02,
  0x9ac10c1b, 0x9b018361, 0xf100003f, 0x9a82d440,
  0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x97f80639,
  0xd503201f, 0xa90087e0, 0xaa1e03e3, 0x97f90b75,
  0xa94087e0, 0x17ffffeb
]

def divCeilInt64Targets (idx : Nat) : Option String :=
  if idx == 15 then some "runtime.panicdivide"
  else if idx == 19 then some "runtime.morestack_noctxt.abi0"
  else none

private def divCeilInt64Decoded : Array Arm64Insn := #[
  .ldrStrUImm true false 8 16 16#5 28#5,
  .addSubExt true true true .uxtx 0 31#5 31#5 16#5,
  .bCond .ls 15,
  .ldrStrImm false false 8 .pre (-16) 30#5 31#5,
  .ldrStrImm false false 8 .offset (-8) 29#5 31#5,
  .addSubImm true true false 29#5 31#5 8#64,
  .cbz true false 1#5 9,
  .div true true 2#5 0#5 1#5,
  .div true true 27#5 0#5 1#5,
  .mul3 true true 1#5 27#5 1#5 0#5,
  .addSubImm true true true 31#5 1#5 0#64,
  .condSel true true .le 0#5 2#5 2#5,
  .ldrStrImm true false 8 .offset (-8) 29#5 31#5,
  .ldrStrImm true false 8 .post 16 30#5 31#5,
  .ret 30#5,
  .bl (-522695),
  .nop,
  .ldrStrPair false false .offset 8 0#5 1#5 31#5,
  .logicalReg true false 1 .lsl 0 3#5 31#5 30#5,
  .bl (-455819),
  .ldrStrPair true false .offset 8 0#5 1#5 31#5,
  .b (-21)
]

private theorem divCeilInt64Decoded_eq :
    decodeArm64Prog divCeilInt64Words = divCeilInt64Decoded := rfl

/-- Tier 5A theorem: decoded Go 1.27 ARM64 `divCeilInt64` never traps and computes `divCeilSpec` for ALL signed 64-bit `a` and all `d > 0`. -/
theorem divCeilInt64_arm64_equiv (s : Arm64State) (a d : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hX0 : s.regs 0#5 = a) (hX1 : s.regs 1#5 = d)
    (hdPos : 0 < d.toInt)
    (hSp : BitVec.ult 15#64 s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - 16#64) = true)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0) :
    let s' := runArm64 32 (decodeArm64Prog divCeilInt64Words) divCeilInt64Targets s
    s'.status = .returned ∧ s'.status ≠ .trapPanicDivide ∧ s'.status ≠ .trapMorestack ∧
      (s'.regs 0#5).toInt = divCeilSpec a.toInt d.toInt := by
  dsimp only
  rw [divCeilInt64Decoded_eq]
  have hLen : divCeilInt64Decoded.size = 22 := rfl
  have hdNe : d ≠ 0#64 := by intro h0; subst h0; simp at hdPos
  let q := a.sdiv d
  let rem := a - q * d
  let out := if !(BitVec.slt 0#64 rem) then q else q + 1#64
  have hSteps : ∃ s15,
      runArm64 15 divCeilInt64Decoded divCeilInt64Targets s = s15 ∧
      s15.status = .returned ∧ s15.regs 0#5 = out := by
    let s1 := advancePc (writeGpr s 16#5 s.gStackGuard0)
    have h0 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s = s1 := by
      have hs := step_ldrStrUImm_load64 divCeilInt64Decoded divCeilInt64Targets s 0 22
        16 16#5 28#5 hRun hPc hLen rfl
      have hAddr : readGprSP s 28#5 + BitVec.ofNat 64 16 = s.regs 28#5 + 16#64 := rfl
      rw [hs, hAddr, hG]
    have hRun1 : s1.status = .running := by simp only [s1, status_advancePc, status_writeGpr, hRun]
    have hPc1 : s1.pcIdx = 1 := by simp only [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
    have hR16_1 : readGpr s1 16#5 = s.gStackGuard0 := by
      rw [show s1 = advancePc (writeGpr s 16#5 s.gStackGuard0) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s2 := advancePc (writeGpr { s1 with nzcv := subFlags64 s.sp s.gStackGuard0 } 31#5 (s.sp - s.gStackGuard0))
    have h1 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s1 = s2 := by
      have hs := step_addSubExt_cmp64 divCeilInt64Decoded divCeilInt64Targets s1 1 22
        31#5 16#5 hRun1 hPc1 hLen rfl
      have hSp1 : readGprSP s1 31#5 = s.sp := rfl
      have hCmp : (readGpr s1 16#5) <<< (0 % 5) = s.gStackGuard0 := by rw [hR16_1]; bv_decide
      rw [hs, hSp1, hCmp]
    have hRun2 : s2.status = .running := by simp only [s2, status_advancePc, status_writeGpr, hRun1]
    have hPc2 : s2.pcIdx = 2 := by simp only [s2, pcIdx_advancePc, pcIdx_writeGpr, hPc1]
    have hNoStack : evalCond .ls s2.nzcv = false := by
      have hNz : s2.nzcv = subFlags64 s.sp s.gStackGuard0 := rfl
      rw [hNz, evalCond_ls_subFlags64, stackGuard_16_ok s.sp s.gStackGuard0 hSp hGuard]
      rfl
    let s3 := advancePc s2
    have hRun3 : s3.status = .running := by rw [status_advancePc]; exact hRun2
    have hPc3 : s3.pcIdx = 3 := by rw [pcIdx_advancePc, hPc2]
    have h2 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s2 = s3 :=
      step_bCond_false _ _ s2 2 22 .ls 15 hRun2 hPc2 hLen rfl hNoStack
    let addr3 := readGprSP s3 31#5 + BitVec.ofInt 64 (-16)
    let s4 := advancePc (writeGprSP { s3 with mem := writeU64 s3.mem addr3 (readGpr s3 30#5) } 31#5 addr3)
    have hRun4 : s4.status = .running := by rw [status_advancePc, status_writeGprSP]; exact hRun3
    have hPc4 : s4.pcIdx = 4 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, hPc3]
    have h3 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s3 = s4 :=
      step_ldrStrImm_pre_store64 _ _ s3 3 22 (-16) 30#5 31#5 hRun3 hPc3 hLen rfl
    let addr4 := readGprSP s4 31#5 + BitVec.ofInt 64 (-8)
    let s5 := advancePc { s4 with mem := writeU64 s4.mem addr4 (readGpr s4 29#5) }
    have hRun5 : s5.status = .running := by rw [status_advancePc]; exact hRun4
    have hPc5 : s5.pcIdx = 5 := by rw [pcIdx_advancePc, hPc4]
    have h4 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s4 = s5 :=
      step_ldrStrImm_off_store64 _ _ s4 4 22 (-8) 29#5 31#5 hRun4 hPc4 hLen rfl
    let s6 := advancePc (writeGprSP s5 29#5 (readGprSP s5 31#5 - 8#64))
    have hRun6 : s6.status = .running := by rw [status_advancePc, status_writeGprSP]; exact hRun5
    have hPc6 : s6.pcIdx = 6 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, hPc5]
    have h5 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s5 = s6 :=
      step_addSubImm_sub64 _ _ s5 5 22 29#5 31#5 8#64 hRun5 hPc5 hLen rfl
    have hR0_6 : readGpr s6 0#5 = a := by
      have h0' : readGpr s 0#5 = a := by
        simp only [readGpr, show (0#5 == 31#5) = false from rfl]; exact hX0
      have h1' : readGpr s1 0#5 = a := by
        rw [show s1 = advancePc (writeGpr s 16#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), h0']
      have h2' : readGpr s2 0#5 = a := by
        rw [show s2 = advancePc (writeGpr _ 31#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide)]; exact h1'
      have h3' : readGpr s3 0#5 = a := by
        rw [show s3 = advancePc s2 from rfl, readGpr_advancePc, h2']
      have h4' : readGpr s4 0#5 = a := by
        rw [show s4 = advancePc (writeGprSP { s3 with mem := _ } 31#5 _) from rfl,
          readGpr_advancePc, readGpr_writeGprSP_sp, readGpr_setMem, h3']
      have h5' : readGpr s5 0#5 = a := by
        rw [show s5 = advancePc { s4 with mem := _ } from rfl,
          readGpr_advancePc, readGpr_setMem, h4']
      rw [show s6 = advancePc (writeGprSP s5 29#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGprSP_ne _ _ _ _ (by decide) (by decide), h5']
    have hR1_6 : readGpr s6 1#5 = d := by
      have h1_0 : readGpr s 1#5 = d := by
        simp only [readGpr, show (1#5 == 31#5) = false from rfl]; exact hX1
      have h1_1 : readGpr s1 1#5 = d := by
        rw [show s1 = advancePc (writeGpr s 16#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), h1_0]
      have h1_2 : readGpr s2 1#5 = d := by
        rw [show s2 = advancePc (writeGpr _ 31#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide)]; exact h1_1
      have h1_3 : readGpr s3 1#5 = d := by
        rw [show s3 = advancePc s2 from rfl, readGpr_advancePc, h1_2]
      have h1_4 : readGpr s4 1#5 = d := by
        rw [show s4 = advancePc (writeGprSP { s3 with mem := _ } 31#5 _) from rfl,
          readGpr_advancePc, readGpr_writeGprSP_sp, readGpr_setMem, h1_3]
      have h1_5 : readGpr s5 1#5 = d := by
        rw [show s5 = advancePc { s4 with mem := _ } from rfl,
          readGpr_advancePc, readGpr_setMem, h1_4]
      rw [show s6 = advancePc (writeGprSP s5 29#5 _) from rfl, readGpr_advancePc,
        readGpr_writeGprSP_ne _ _ _ _ (by decide) (by decide), h1_5]
    have hCbzFalse : (if false then readGpr s6 1#5 != 0#64 else readGpr s6 1#5 == 0#64) = false := by
      simp only [Bool.false_eq_true, ↓reduceIte, hR1_6]
      exact beq_false_of_ne hdNe
    let s7 := advancePc s6
    have hRun7 : s7.status = .running := by rw [status_advancePc]; exact hRun6
    have hPc7 : s7.pcIdx = 7 := by rw [pcIdx_advancePc, hPc6]
    have h6 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s6 = s7 :=
      step_cbz64_fall _ _ s6 6 22 false 1#5 9 hRun6 hPc6 hLen rfl hCbzFalse
    have hR0_7 : readGpr s7 0#5 = a := by
      rw [show s7 = advancePc s6 from rfl, readGpr_advancePc, hR0_6]
    have hR1_7 : readGpr s7 1#5 = d := by
      rw [show s7 = advancePc s6 from rfl, readGpr_advancePc, hR1_6]
    let s8 := advancePc (writeGpr s7 2#5 q)
    have h7 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s7 = s8 := by
      have hs := step_sdiv64 divCeilInt64Decoded divCeilInt64Targets s7 7 22 2#5 0#5 1#5
        hRun7 hPc7 hLen rfl (by rw [hR1_7]; exact hdNe)
      rw [hs, hR0_7, hR1_7]
    have hRun8 : s8.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun7
    have hPc8 : s8.pcIdx = 8 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc7]
    have hR0_8 : readGpr s8 0#5 = a := by
      rw [show s8 = advancePc (writeGpr s7 2#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_7]
    have hR1_8 : readGpr s8 1#5 = d := by
      rw [show s8 = advancePc (writeGpr s7 2#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_7]
    have hR2_8 : readGpr s8 2#5 = q := by
      rw [show s8 = advancePc (writeGpr s7 2#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s9 := advancePc (writeGpr s8 27#5 q)
    have h8 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s8 = s9 := by
      have hs := step_sdiv64 divCeilInt64Decoded divCeilInt64Targets s8 8 22 27#5 0#5 1#5
        hRun8 hPc8 hLen rfl (by rw [hR1_8]; exact hdNe)
      rw [hs, hR0_8, hR1_8]
    have hRun9 : s9.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun8
    have hPc9 : s9.pcIdx = 9 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc8]
    have hR0_9 : readGpr s9 0#5 = a := by
      rw [show s9 = advancePc (writeGpr s8 27#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_8]
    have hR1_9 : readGpr s9 1#5 = d := by
      rw [show s9 = advancePc (writeGpr s8 27#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_8]
    have hR2_9 : readGpr s9 2#5 = q := by
      rw [show s9 = advancePc (writeGpr s8 27#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR2_8]
    have hR27_9 : readGpr s9 27#5 = q := by
      rw [show s9 = advancePc (writeGpr s8 27#5 q) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s10 := advancePc (writeGpr s9 1#5 rem)
    have h9 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s9 = s10 := by
      have hs := step_msub64 divCeilInt64Decoded divCeilInt64Targets s9 9 22 1#5 27#5 1#5 0#5
        hRun9 hPc9 hLen rfl
      rw [hs, hR0_9, hR27_9, hR1_9]
    have hRun10 : s10.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun9
    have hPc10 : s10.pcIdx = 10 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc9]
    have hR1_10 : readGpr s10 1#5 = rem := by
      rw [show s10 = advancePc (writeGpr s9 1#5 rem) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    have hR2_10 : readGpr s10 2#5 = q := by
      rw [show s10 = advancePc (writeGpr s9 1#5 rem) from rfl, readGpr_advancePc,
        readGpr_writeGpr_ne _ _ _ _ (by decide), hR2_9]
    have hSp1_10 : readGprSP s10 1#5 = rem := by
      rw [readGprSP_eq_readGpr _ _ (by decide), hR1_10]
    let s11 := advancePc (writeGpr { s10 with nzcv := subFlags64 rem 0#64 } 31#5 (rem - 0#64))
    have h10 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s10 = s11 := by
      have hs := step_addSubImm_cmp64 divCeilInt64Decoded divCeilInt64Targets s10 10 22 1#5 0#64
        hRun10 hPc10 hLen rfl
      rw [hs, hSp1_10]
    have hRun11 : s11.status = .running := by
      rw [show s11 = advancePc (writeGpr { s10 with nzcv := subFlags64 rem 0#64 } 31#5 (rem - 0#64)) from rfl,
        status_advancePc, status_writeGpr, status_setNzcv, hRun10]
    have hPc11 : s11.pcIdx = 11 := by
      rw [show s11 = advancePc (writeGpr { s10 with nzcv := subFlags64 rem 0#64 } 31#5 (rem - 0#64)) from rfl,
        pcIdx_advancePc, pcIdx_writeGpr, pcIdx_setNzcv, hPc10]
    have hR2_11 : readGpr s11 2#5 = q := by
      rw [show s11 = advancePc (writeGpr { s10 with nzcv := subFlags64 rem 0#64 } 31#5 (rem - 0#64)) from rfl,
        readGpr_advancePc, readGpr_writeGpr_ne _ _ _ _ (by decide), readGpr_setNzcv, hR2_10]
    have hCond11 : evalCond .le s11.nzcv = !(BitVec.slt 0#64 rem) := by
      have hNz : s11.nzcv = subFlags64 rem 0#64 := rfl
      rw [hNz, evalCond_le_subFlags64_zero rem]
    let s12 := advancePc (writeGpr s11 0#5 out)
    have h11 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s11 = s12 := by
      have hs := step_cinc64 divCeilInt64Decoded divCeilInt64Targets s11 11 22 .le 0#5 2#5 2#5
        hRun11 hPc11 hLen rfl
      rw [hs, hR2_11, hCond11]
    have hRun12 : s12.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun11
    have hPc12 : s12.pcIdx = 12 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc11]
    have hR0_12 : readGpr s12 0#5 = out := by
      rw [show s12 = advancePc (writeGpr s11 0#5 out) from rfl, readGpr_advancePc,
        readGpr_writeGpr_eq _ _ _ (by decide)]
    let s13 := advancePc (writeGpr s12 29#5 (readU64 s12.mem (readGprSP s12 31#5 + BitVec.ofInt 64 (-8))))
    have hRun13 : s13.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun12
    have hPc13 : s13.pcIdx = 13 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc12]
    have h12 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s12 = s13 :=
      step_ldrStrImm_off_load64 _ _ s12 12 22 (-8) 29#5 31#5 hRun12 hPc12 hLen rfl
    let base13 := readGprSP s13 31#5
    let s14 := advancePc (writeGprSP (writeGpr s13 30#5 (readU64 s13.mem base13)) 31#5 (base13 + BitVec.ofInt 64 16))
    have hRun14 : s14.status = .running := by rw [status_advancePc, status_writeGprSP, status_writeGpr]; exact hRun13
    have hPc14 : s14.pcIdx = 14 := by rw [pcIdx_advancePc, pcIdx_writeGprSP, pcIdx_writeGpr, hPc13]
    have h13 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s13 = s14 :=
      step_ldrStrImm_post_load64 _ _ s13 13 22 16 30#5 31#5 hRun13 hPc13 hLen rfl
    let s15 := { s14 with status := .returned }
    have h14 : stepArm64 divCeilInt64Decoded divCeilInt64Targets s14 = s15 :=
      step_ret _ _ s14 14 22 30#5 hRun14 hPc14 hLen rfl
    refine ⟨s15, ?_, rfl, ?_⟩
    · rw [runArm64_succ_running _ _ _ _ hRun, h0]
      rw [runArm64_succ_running _ _ _ _ hRun1, h1]
      rw [runArm64_succ_running _ _ _ _ hRun2, h2]
      rw [runArm64_succ_running _ _ _ _ hRun3, h3]
      rw [runArm64_succ_running _ _ _ _ hRun4, h4]
      rw [runArm64_succ_running _ _ _ _ hRun5, h5]
      rw [runArm64_succ_running _ _ _ _ hRun6, h6]
      rw [runArm64_succ_running _ _ _ _ hRun7, h7]
      rw [runArm64_succ_running _ _ _ _ hRun8, h8]
      rw [runArm64_succ_running _ _ _ _ hRun9, h9]
      rw [runArm64_succ_running _ _ _ _ hRun10, h10]
      rw [runArm64_succ_running _ _ _ _ hRun11, h11]
      rw [runArm64_succ_running _ _ _ _ hRun12, h12]
      rw [runArm64_succ_running _ _ _ _ hRun13, h13]
      rw [runArm64_succ_running _ _ _ _ hRun14, h14]
      rfl
    · have hR0_14 : readGpr s14 0#5 = out := by
        rw [show s14 = advancePc (writeGprSP (writeGpr s13 30#5 _) 31#5 _) from rfl,
          readGpr_advancePc, readGpr_writeGprSP_sp, readGpr_writeGpr_ne _ _ _ _ (by decide),
          show s13 = advancePc (writeGpr s12 29#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_12]
      rw [← readGpr_eq_regs s14 0#5 (by decide)]
      exact hR0_14
  obtain ⟨s15, hRun015, hRet15, hReg15⟩ := hSteps
  rw [show (32 : Nat) = 15 + 17 by decide, runArm64_add 15 17, hRun015,
    runArm64_of_not_running _ _ _ _ (by rw [hRet15]; decide)]
  refine ⟨hRet15, by rw [hRet15]; decide, by rw [hRet15]; decide, ?_⟩
  rw [hReg15]
  exact divCeil_bv64_eq a d hdPos

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words for `gcdInt64`. -/
def gcdInt64Words : Array UInt32 := #[
  0x14000005, 0x9ac10c1b, 0x9b018362, 0xaa0103e0, 0xaa0203e1, 0xb5ffff81, 0xd65f03c0
]

def gcdInt64Targets (_ : Nat) : Option String := none

private def gcdInt64Decoded : Array Arm64Insn := #[
  .b 5,
  .div true true 27#5 0#5 1#5,
  .mul3 true true 2#5 27#5 1#5 0#5,
  .logicalReg true false 1 .lsl 0 0#5 31#5 1#5,
  .logicalReg true false 1 .lsl 0 1#5 31#5 2#5,
  .cbz true true 1#5 (-4),
  .ret 30#5
]

private theorem gcdInt64Decoded_eq : decodeArm64Prog gcdInt64Words = gcdInt64Decoded := rfl

private theorem bv64_toInt_eq_toNat (x : BitVec 64) (hx : 0 ≤ x.toInt) :
    x.toInt = (x.toNat : Int) := by
  have hlt : x.toNat < 2 ^ 64 := x.isLt
  rw [BitVec.toInt_eq_toNat_cond] at hx ⊢
  split at hx <;> split <;> omega

private theorem rem_bv64_nonneg_eq (a b : BitVec 64)
    (ha : 0 ≤ a.toInt) (hb : 0 < b.toInt) :
    let rem := a - (a.sdiv b) * b
    0 ≤ rem.toInt ∧ rem.toInt < b.toInt ∧
      rem.toNat = a.toNat % b.toNat ∧
      Int.gcd b.toInt rem.toInt = Int.gcd a.toInt b.toInt := by
  have haNat := bv64_toInt_eq_toNat a ha
  have hbNat := bv64_toInt_eq_toNat b (Int.le_of_lt hb)
  have hqRange := tdiv_64_in_range a b hb
  have hqEq : (a.sdiv b).toInt = a.toInt.tdiv b.toInt := by
    rw [BitVec.toInt_sdiv, bmod_pow64_eq_self _ hqRange.1 hqRange.2]
  have haLt := BitVec.toInt_lt (x := a)
  have hbLt : b.toInt < 2 ^ 63 := BitVec.toInt_lt (x := b)
  have h0Mod := Int.tmod_nonneg b.toInt ha
  have hLtMod := Int.tmod_lt_of_pos a.toInt hb
  have hMulComm : a.toInt.tdiv b.toInt * b.toInt = b.toInt * a.toInt.tdiv b.toInt := Int.mul_comm _ _
  have hMulEqInt : a.toInt.tdiv b.toInt * b.toInt = a.toInt - a.toInt.tmod b.toInt := by
    have := Int.mul_tdiv_add_tmod a.toInt b.toInt
    rw [hMulComm]; omega
  have hMulRange : -(2 ^ 63 : Int) ≤ (a.sdiv b).toInt * b.toInt ∧ (a.sdiv b).toInt * b.toInt < 2 ^ 63 := by
    rw [hqEq, hMulEqInt]; omega
  have hMulEq : ((a.sdiv b) * b).toInt = a.toInt - a.toInt.tmod b.toInt := by
    rw [BitVec.toInt_mul, bmod_pow64_eq_self _ hMulRange.1 hMulRange.2, hqEq, hMulEqInt]
  have hRemInt : (a - (a.sdiv b) * b).toInt = a.toInt.tmod b.toInt := by
    rw [BitVec.toInt_sub, hMulEq]
    have heq : a.toInt - (a.toInt - a.toInt.tmod b.toInt) = a.toInt.tmod b.toInt := by omega
    rw [heq]
    exact bmod_pow64_eq_self _ (by omega) (by omega)
  have hRemNonneg : 0 ≤ (a - (a.sdiv b) * b).toInt := by rw [hRemInt]; exact h0Mod
  have hRemNat : (a - (a.sdiv b) * b).toNat = a.toNat % b.toNat := by
    have hEmod : a.toInt.tmod b.toInt = a.toInt % b.toInt := Int.tmod_eq_emod_of_nonneg ha
    have hRemNatInt := bv64_toInt_eq_toNat (a - (a.sdiv b) * b) hRemNonneg
    rw [hRemInt, hEmod, haNat, hbNat] at hRemNatInt
    omega
  refine ⟨hRemNonneg, by rw [hRemInt]; exact hLtMod, hRemNat, ?_⟩
  rw [hRemInt, Int.tmod_eq_emod_of_nonneg ha]
  simp only [Int.gcd]
  rw [haNat, hbNat]
  have hAbs1 : (↑b.toNat : Int).natAbs = b.toNat := Int.natAbs_natCast b.toNat
  have hAbs2 : (↑a.toNat : Int).natAbs = a.toNat := Int.natAbs_natCast a.toNat
  have hAbsMod : (↑a.toNat % ↑b.toNat : Int).natAbs = a.toNat % b.toNat := by
    have hEq : (↑a.toNat % ↑b.toNat : Int) = ↑(a.toNat % b.toNat) := by omega
    rw [hEq, Int.natAbs_natCast]
  rw [hAbs1, hAbs2, hAbsMod, Nat.gcd_comm b.toNat _, ← Nat.gcd_rec b.toNat a.toNat, Nat.gcd_comm]

private theorem gcdInt64_loop (n : Nat) :
    ∀ (fuel : Nat) (s : Arm64State) (a b : BitVec 64),
      b.toNat ≤ n → 0 ≤ a.toInt → 0 ≤ b.toInt →
      s.status = .running → s.pcIdx = 5 → s.regs 0#5 = a → s.regs 1#5 = b →
      5 * b.toNat + 2 ≤ fuel →
      let s' := runArm64 fuel gcdInt64Decoded gcdInt64Targets s
      s'.status = .returned ∧ s'.status ≠ .trapPanicDivide ∧
        (s'.regs 0#5).toNat = Int.gcd a.toInt b.toInt := by
  induction n with
  | zero =>
    intro fuel s a b hle ha hb hRun hPc hX0 hX1 hFuel
    have hbNat0 : b.toNat = 0 := by omega
    have hb0 : b = 0#64 := BitVec.eq_of_toNat_eq (by rw [hbNat0]; rfl)
    subst hb0
    obtain ⟨f', hf⟩ := Nat.exists_eq_add_of_le hFuel
    rw [hf]
    have hLen : gcdInt64Decoded.size = 7 := rfl
    have hCondF : (if true then readGpr s 1#5 != 0#64 else readGpr s 1#5 == 0#64) = false := by
      simp only [↓reduceIte, readGpr, show (1#5 == 31#5) = false from rfl, hX1]; decide
    let s1 := advancePc s
    have hRun1 : s1.status = .running := hRun
    have h0 : stepArm64 gcdInt64Decoded gcdInt64Targets s = s1 :=
      step_cbz64_fall _ _ s 5 7 true 1#5 (-4) hRun hPc hLen rfl hCondF
    let s2 := { s1 with status := .returned }
    have h1 : stepArm64 gcdInt64Decoded gcdInt64Targets s1 = s2 :=
      step_ret _ _ s1 6 7 30#5 hRun1 (by rw [show s1 = advancePc s from rfl, pcIdx_advancePc, hPc]) hLen rfl
    have hZero0 : (0#64 : BitVec 64).toNat = 0 := rfl
    rw [hZero0]
    rw [show 5 * 0 + 2 + f' = 2 + f' by omega, runArm64_add 2 f']
    have hRun2 : runArm64 2 gcdInt64Decoded gcdInt64Targets s = s2 := by
      rw [runArm64_succ_running _ _ _ _ hRun, h0, runArm64_succ_running _ _ _ _ hRun1, h1]
      rfl
    have hSt2 : s2.status = .returned := rfl
    rw [hRun2, runArm64_of_not_running _ _ _ _ (by rw [hSt2]; decide)]
    refine ⟨hSt2, by rw [hSt2]; decide, ?_⟩
    simp only [s2, s1, advancePc, hX0]
    have haNat := bv64_toInt_eq_toNat a ha
    have hZeroInt : (0#64 : BitVec 64).toInt = 0 := BitVec.toInt_zero
    simp only [Int.gcd, haNat, Int.natAbs_natCast, hZeroInt]
    simp [Nat.gcd_zero_right]
  | succ k ih =>
    intro fuel s a b hle ha hb hRun hPc hX0 hX1 hFuel
    by_cases hb0 : b = 0#64
    · subst hb0
      obtain ⟨f', hf⟩ := Nat.exists_eq_add_of_le hFuel
      rw [hf]
      have hLen : gcdInt64Decoded.size = 7 := rfl
      have hCondF : (if true then readGpr s 1#5 != 0#64 else readGpr s 1#5 == 0#64) = false := by
        simp only [↓reduceIte, readGpr, show (1#5 == 31#5) = false from rfl, hX1]; decide
      let s1 := advancePc s
      have hRun1 : s1.status = .running := hRun
      have h0 : stepArm64 gcdInt64Decoded gcdInt64Targets s = s1 :=
        step_cbz64_fall _ _ s 5 7 true 1#5 (-4) hRun hPc hLen rfl hCondF
      let s2 := { s1 with status := .returned }
      have h1 : stepArm64 gcdInt64Decoded gcdInt64Targets s1 = s2 :=
        step_ret _ _ s1 6 7 30#5 hRun1 (by rw [show s1 = advancePc s from rfl, pcIdx_advancePc, hPc]) hLen rfl
      have hZero0 : (0#64 : BitVec 64).toNat = 0 := rfl
      rw [hZero0]
      rw [show 5 * 0 + 2 + f' = 2 + f' by omega, runArm64_add 2 f']
      have hRun2 : runArm64 2 gcdInt64Decoded gcdInt64Targets s = s2 := by
        rw [runArm64_succ_running _ _ _ _ hRun, h0, runArm64_succ_running _ _ _ _ hRun1, h1]
        rfl
      have hSt2 : s2.status = .returned := rfl
      rw [hRun2, runArm64_of_not_running _ _ _ _ (by rw [hSt2]; decide)]
      refine ⟨hSt2, by rw [hSt2]; decide, ?_⟩
      simp only [s2, s1, advancePc, hX0]
      have haNat := bv64_toInt_eq_toNat a ha
      have hZeroInt : (0#64 : BitVec 64).toInt = 0 := BitVec.toInt_zero
      simp only [Int.gcd, haNat, Int.natAbs_natCast, hZeroInt]
      simp [Nat.gcd_zero_right]
    · have hbNatPos : 0 < b.toNat := by
        by_cases hz : b.toNat = 0
        · exfalso; exact hb0 (BitVec.eq_of_toNat_eq (by rw [hz]; rfl))
        · omega
      have hbIntPos : 0 < b.toInt := by
        have hbNatEq := bv64_toInt_eq_toNat b hb
        omega
      let rem := a - (a.sdiv b) * b
      have hRemProps := rem_bv64_nonneg_eq a b ha hbIntPos
      have hLen : gcdInt64Decoded.size = 7 := rfl
      have hCondT : (if true then readGpr s 1#5 != 0#64 else readGpr s 1#5 == 0#64) = true := by
        simp only [↓reduceIte, readGpr, show (1#5 == 31#5) = false from rfl, hX1]
        exact bne_iff_ne.mpr hb0
      have hR0_s : readGpr s 0#5 = a := by
        simp only [readGpr, show (0#5 == 31#5) = false from rfl]; exact hX0
      have hR1_s : readGpr s 1#5 = b := by
        simp only [readGpr, show (1#5 == 31#5) = false from rfl]; exact hX1
      let s1 := { s with pcIdx := 1 }
      have hRun1 : s1.status = .running := hRun
      have hR0_1 : readGpr s1 0#5 = a := hR0_s
      have hR1_1 : readGpr s1 1#5 = b := hR1_s
      have h0 : stepArm64 gcdInt64Decoded gcdInt64Targets s = s1 :=
        step_cbz64_taken gcdInt64Decoded gcdInt64Targets s 5 7 true 1#5 (-4)
          hRun hPc hLen rfl hCondT (by decide) (by decide)
      let s2 := advancePc (writeGpr s1 27#5 (a.sdiv b))
      have h1 : stepArm64 gcdInt64Decoded gcdInt64Targets s1 = s2 := by
        have hs := step_sdiv64 gcdInt64Decoded gcdInt64Targets s1 1 7 27#5 0#5 1#5
          hRun1 rfl hLen rfl (by rw [hR1_1]; exact hb0)
        rw [hs, hR0_1, hR1_1]
      have hRun2 : s2.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun1
      have hPc2 : s2.pcIdx = 2 := by simp only [s2, pcIdx_advancePc, pcIdx_writeGpr, s1]
      have hR0_2 : readGpr s2 0#5 = a := by
        rw [show s2 = advancePc (writeGpr s1 27#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), hR0_1]
      have hR1_2 : readGpr s2 1#5 = b := by
        rw [show s2 = advancePc (writeGpr s1 27#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_1]
      have hR27_2 : readGpr s2 27#5 = a.sdiv b := by
        rw [show s2 = advancePc (writeGpr s1 27#5 _) from rfl, readGpr_advancePc,
          readGpr_writeGpr_eq _ _ _ (by decide)]
      let s3 := advancePc (writeGpr s2 2#5 rem)
      have h2 : stepArm64 gcdInt64Decoded gcdInt64Targets s2 = s3 := by
        have hs := step_msub64 gcdInt64Decoded gcdInt64Targets s2 2 7 2#5 27#5 1#5 0#5
          hRun2 hPc2 hLen rfl
        rw [hs, hR0_2, hR27_2, hR1_2]
      have hRun3 : s3.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun2
      have hPc3 : s3.pcIdx = 3 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc2]
      have hR1_3 : readGpr s3 1#5 = b := by
        rw [show s3 = advancePc (writeGpr s2 2#5 rem) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), hR1_2]
      have hR2_3 : readGpr s3 2#5 = rem := by
        rw [show s3 = advancePc (writeGpr s2 2#5 rem) from rfl, readGpr_advancePc,
          readGpr_writeGpr_eq _ _ _ (by decide)]
      let s4 := advancePc (writeGpr s3 0#5 b)
      have h3 : stepArm64 gcdInt64Decoded gcdInt64Targets s3 = s4 := by
        have hs := step_orr64_lsl0 gcdInt64Decoded gcdInt64Targets s3 3 7 0#5 31#5 1#5
          hRun3 hPc3 hLen rfl
        have hZR : readGpr s3 31#5 = 0#64 := rfl
        have hSh : b <<< (0 % 64) = b := by bv_decide
        rw [hs, hZR, hR1_3, hSh]
        have hOr0 : 0#64 ||| b = b := by bv_decide
        rw [hOr0]
      have hRun4 : s4.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun3
      have hPc4 : s4.pcIdx = 4 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc3]
      have hR2_4 : readGpr s4 2#5 = rem := by
        rw [show s4 = advancePc (writeGpr s3 0#5 b) from rfl, readGpr_advancePc,
          readGpr_writeGpr_ne _ _ _ _ (by decide), hR2_3]
      let s5 := advancePc (writeGpr s4 1#5 rem)
      have h4 : stepArm64 gcdInt64Decoded gcdInt64Targets s4 = s5 := by
        have hs := step_orr64_lsl0 gcdInt64Decoded gcdInt64Targets s4 4 7 1#5 31#5 2#5
          hRun4 hPc4 hLen rfl
        have hZR : readGpr s4 31#5 = 0#64 := rfl
        have hSh : rem <<< (0 % 64) = rem := by bv_decide
        rw [hs, hZR, hR2_4, hSh]
        have hOr0 : 0#64 ||| rem = rem := by bv_decide
        rw [hOr0]
      have hRun5 : s5.status = .running := by rw [status_advancePc, status_writeGpr]; exact hRun4
      have hPc5 : s5.pcIdx = 5 := by rw [pcIdx_advancePc, pcIdx_writeGpr, hPc4]
      have hR0_5 : s5.regs 0#5 = b := by
        have hg : readGpr s5 0#5 = b := by
          rw [show s5 = advancePc (writeGpr s4 1#5 rem) from rfl, readGpr_advancePc,
            readGpr_writeGpr_ne _ _ _ _ (by decide),
            show s4 = advancePc (writeGpr s3 0#5 b) from rfl, readGpr_advancePc,
            readGpr_writeGpr_eq _ _ _ (by decide)]
        rw [← readGpr_eq_regs s5 0#5 (by decide)]; exact hg
      have hR1_5 : s5.regs 1#5 = rem := by
        have hg : readGpr s5 1#5 = rem := by
          rw [show s5 = advancePc (writeGpr s4 1#5 rem) from rfl, readGpr_advancePc,
            readGpr_writeGpr_eq _ _ _ (by decide)]
        rw [← readGpr_eq_regs s5 1#5 (by decide)]; exact hg
      have hIter : runArm64 5 gcdInt64Decoded gcdInt64Targets s = s5 := by
        rw [runArm64_succ_running _ _ _ _ hRun, h0]
        rw [runArm64_succ_running _ _ _ _ hRun1, h1]
        rw [runArm64_succ_running _ _ _ _ hRun2, h2]
        rw [runArm64_succ_running _ _ _ _ hRun3, h3]
        rw [runArm64_succ_running _ _ _ _ hRun4, h4]
        rfl
      have hRemLtB : rem.toNat < b.toNat := by
        rw [hRemProps.2.2.1]; exact Nat.mod_lt a.toNat hbNatPos
      have hRemLeK : rem.toNat ≤ k := by omega
      have hFuelSplit : 5 ≤ fuel := by omega
      obtain ⟨fuel', hF⟩ := Nat.exists_eq_add_of_le hFuelSplit
      have hFuelNext : 5 * rem.toNat + 2 ≤ fuel' := by omega
      rw [hF, runArm64_add 5 fuel', hIter]
      have hRec := ih fuel' s5 b rem hRemLeK hb hRemProps.1 hRun5 hPc5 hR0_5 hR1_5 hFuelNext
      rw [hRemProps.2.2.2] at hRec
      exact hRec

/-- Tier 5A theorem: decoded Go 1.27 ARM64 `gcdInt64` never triggers `panicdivide` and equals `Int.gcd` on all non-negative signed 64-bit `(a, b)` given `fuel ≥ 5 * b.toNat + 3`. -/
theorem gcdInt64_arm64_equiv (s : Arm64State) (a b : BitVec 64) (fuel : Nat)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hX0 : s.regs 0#5 = a) (hX1 : s.regs 1#5 = b)
    (ha : 0 ≤ a.toInt) (hb : 0 ≤ b.toInt)
    (hFuel : 5 * b.toNat + 3 ≤ fuel) :
    let s' := runArm64 fuel (decodeArm64Prog gcdInt64Words) gcdInt64Targets s
    s'.status = .returned ∧ s'.status ≠ .trapPanicDivide ∧
      (s'.regs 0#5).toNat = Int.gcd a.toInt b.toInt := by
  dsimp only
  rw [gcdInt64Decoded_eq]
  have hLen : gcdInt64Decoded.size = 7 := rfl
  obtain ⟨fuel', hF⟩ := Nat.exists_eq_add_of_le hFuel
  rw [hF, show 5 * b.toNat + 3 + fuel' = 1 + (5 * b.toNat + 2 + fuel') by omega,
    runArm64_add 1 (5 * b.toNat + 2 + fuel')]
  have h0 : stepArm64 gcdInt64Decoded gcdInt64Targets s = { s with pcIdx := 5 } :=
    step_b_internal gcdInt64Decoded gcdInt64Targets s 0 7 5 hRun hPc hLen rfl
      (by decide) (by decide)
  have hRun1 : runArm64 1 gcdInt64Decoded gcdInt64Targets s = { s with pcIdx := 5 } := by
    rw [runArm64_succ_running _ _ _ _ hRun, h0]
    rfl
  rw [hRun1]
  exact gcdInt64_loop b.toNat (5 * b.toNat + 2 + fuel') { s with pcIdx := 5 } a b
    (Nat.le_refl _) ha hb hRun rfl hX0 hX1 (by omega)

/-! ### PC-Relative / RODATA Normalization for Live ELF Verification -/

/-- Canonicalized ARM64 instruction form for validating freshly compiled ELF binaries against Lean-verified code. -/
inductive NormArm64Insn where
  | exact (insn : Arm64Insn)
  | blExt (target : String)
  | bExt (target : String)
  | adrpNorm (rd : BitVec 5)
  | adrpLoad64Norm (isFP : Bool) (rt rn : BitVec 5)
  | adrpAdd64Norm (rd rn : BitVec 5)
  deriving DecidableEq, Repr, Inhabited

/-- Normalize a decoded ARM64 program given external call targets indexed by `pcIdx`. -/
def normalizeArm64Prog (insns : Array Arm64Insn) (extTargets : Nat → Option String) :
    Except String (Array NormArm64Insn) := do
  let mut out : Array NormArm64Insn := #[]
  let n := insns.size
  for i in [0:n] do
    let insn := insns[i]!
    let prevIsAdrp : Option (BitVec 5) :=
      if i > 0 then
        match insns[i - 1]! with
        | .adrp rd _ => some rd
        | _ => none
      else none
    match insn with
    | .unknown w =>
      throw s!"unknown ARM64 instruction word at idx {i}: {w}"
    | .bl imm26 =>
      let tgtInt : Int := (i : Int) + imm26
      if 0 <= tgtInt && tgtInt.toNat < n then
        out := out.push (.exact (.bl imm26))
      else
        match extTargets i with
        | some sym => out := out.push (.blExt sym)
        | none => throw s!"missing external BL target metadata at idx {i}"
    | .b imm26 =>
      let tgtInt : Int := (i : Int) + imm26
      if 0 <= tgtInt && tgtInt.toNat < n then
        out := out.push (.exact (.b imm26))
      else
        match extTargets i with
        | some sym => out := out.push (.bExt sym)
        | none => throw s!"missing external B target metadata at idx {i}"
    | .adrp rd _ =>
      out := out.push (.adrpNorm rd)
    | .ldrStrUImm true isFP 8 _ rt rn =>
      if prevIsAdrp == some rn then
        out := out.push (.adrpLoad64Norm isFP rt rn)
      else
        out := out.push (.exact insn)
    | .addSubImm true false false rd rn _ =>
      if prevIsAdrp == some rn then
        out := out.push (.adrpAdd64Norm rd rn)
      else
        out := out.push (.exact insn)
    | _ =>
      out := out.push (.exact insn)
  return out

/-- Compare freshly extracted ELF words + external targets against a verified embedded routine. -/
def arm64ProgramMatches (expectedWords : Array UInt32) (expectedExt : Nat → Option String)
    (liveWords : Array UInt32) (liveExt : Nat → Option String) : Except String Unit := do
  let expNorm ← normalizeArm64Prog (decodeArm64Prog expectedWords) expectedExt
  let liveNorm ← normalizeArm64Prog (decodeArm64Prog liveWords) liveExt
  if expNorm.size != liveNorm.size then
    throw s!"instruction count mismatch: expected {expNorm.size}, got {liveNorm.size}"
  for i in [0:expNorm.size] do
    if expNorm[i]! != liveNorm[i]! then
      throw s!"normalized ARM64 mismatch at idx {i}: expected {repr (expNorm[i]!)}, got {repr (liveNorm[i]!)}"
  return ()

end GoblinFormal.Track5A
