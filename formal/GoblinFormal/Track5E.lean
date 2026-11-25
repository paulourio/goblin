/-
GoblinFormal.Track5E: Decoded `GOARCH=amd64 GOEXPERIMENT=simd` VEX (256-bit AVX2)
and EVEX (512-bit AVX-512) Machine-Code Decoder, Operational Semantics,
`VZEROUPPER` ABI Safety, and Inductive Functional Equivalence Proofs for
shipping `internal/simd` kernels (`allAbsLE64`, `minNoNaN64`, `initReducedRow64`).
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Track5A
import GoblinFormal.Track5D

namespace GoblinFormal.Track5E

open GoblinFormal
open GoblinFormal.Track5A
open GoblinFormal.Track5D

/-!
### Part I: Extracted Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` Bytes
-/

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (180 bytes, 53 insns) for `internal/simd.allAbsLE64AVX2`. -/
def allAbsLE64AVX2ByteChunks : List (List UInt8) := [
  [0x55, 0x48, 0x89, 0xe5, 0x48, 0x89, 0x44, 0x24, 0x10, 0x48, 0x85, 0xdb, 0x0f, 0x86, 0x9c, 0x00],
  [0x00, 0x00, 0xc5, 0x83, 0x11, 0xc0, 0xc4, 0xe2, 0x7d, 0x19, 0xc0, 0x48, 0xb9, 0xff, 0xff, 0xff],
  [0xff, 0xff, 0xff, 0xff, 0x7f, 0xc4, 0xe1, 0xf9, 0x6e, 0xc9, 0x90, 0x90, 0x90, 0xc4, 0xe2, 0x7d],
  [0x59, 0xc9, 0xc5, 0xfd, 0xc2, 0xd0, 0x00, 0x31, 0xc9, 0xc5, 0xfe, 0x7f, 0xd3, 0xeb, 0x29, 0x48],
  [0x8d, 0x34, 0xc8, 0xc5, 0xf5, 0xdb, 0x26, 0x48, 0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76, 0x20, 0xc5],
  [0xf5, 0xdb, 0x2e, 0xc5, 0xdd, 0xc2, 0xe0, 0x02, 0xc5, 0xd5, 0xc2, 0xe8, 0x02, 0xc5, 0xdd, 0xdb],
  [0xdb, 0xc5, 0xd5, 0xdb, 0xd2, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x48, 0x39, 0xd3, 0x7d],
  [0xce, 0xeb, 0x14, 0x48, 0x8d, 0x34, 0xc8, 0xc5, 0xf5, 0xdb, 0x26, 0xc5, 0xdd, 0xc2, 0xe0, 0x02],
  [0xc5, 0xe5, 0xdb, 0xdc, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x04, 0x48, 0x39, 0xd3, 0x7d, 0xe3],
  [0xc5, 0xe5, 0xdb, 0xc2, 0xc5, 0xfd, 0x50, 0xc8, 0x80, 0xf9, 0x0f, 0x74, 0x07, 0xc5, 0xf8, 0x77],
  [0x31, 0xc0, 0x5d, 0xc3, 0xc5, 0xf8, 0x77, 0xb8, 0x01, 0x00, 0x00, 0x00, 0x5d, 0xc3, 0xe8, 0x2d],
  [0xe5, 0xf3, 0xff, 0x90]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.allAbsLE64AVX2`. -/
def allAbsLE64AVX2Bytes : Array UInt8 := ⟨allAbsLE64AVX2ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.allAbsLE64AVX2`. -/
def allAbsLE64AVX2Targets (off : Nat) : Option String :=
  match off with
  | 0xae => some "runtime.panicBounds"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.allAbsLE64AVX2` (none). -/
def allAbsLE64AVX2Rodata (_ : Nat) : Option (BitVec 64) := none

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (220 bytes, 56 insns) for `internal/simd.allAbsLE64AVX512`. -/
def allAbsLE64AVX512ByteChunks : List (List UInt8) := [
  [0x55, 0x48, 0x89, 0xe5, 0x48, 0x89, 0x44, 0x24, 0x10, 0x48, 0x85, 0xdb, 0x0f, 0x86, 0xc4, 0x00],
  [0x00, 0x00, 0xc5, 0x83, 0x11, 0xc0, 0x62, 0xf2, 0xfd, 0x48, 0x19, 0xc0, 0x48, 0xb9, 0xff, 0xff],
  [0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xc4, 0xe1, 0xf9, 0x6e, 0xc9, 0x62, 0xf1, 0xfd, 0x48, 0xc2],
  [0xc8, 0x00, 0x90, 0x90, 0x90, 0x62, 0xf2, 0xfd, 0x48, 0x59, 0xc9, 0x62, 0xf2, 0xfe, 0x48, 0x38],
  [0xd1, 0x31, 0xc9, 0x62, 0xf1, 0xfe, 0x48, 0x7f, 0xd3, 0xeb, 0x35, 0x48, 0x8d, 0x34, 0xc8, 0x62],
  [0xf1, 0xf5, 0x48, 0xdb, 0x26, 0x48, 0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76, 0x40, 0x62, 0xf1, 0xf5],
  [0x48, 0xdb, 0x2e, 0x62, 0xf1, 0xdd, 0x48, 0xc2, 0xc8, 0x02, 0x62, 0xf1, 0xd5, 0x48, 0xc2, 0xd0],
  [0x02, 0x62, 0xf1, 0xfe, 0xc9, 0x7f, 0xdb, 0x62, 0xf1, 0xfe, 0xca, 0x7f, 0xd2, 0x48, 0x89, 0xd1],
  [0x48, 0x8d, 0x51, 0x10, 0x48, 0x39, 0xd3, 0x7d, 0xc2, 0xeb, 0x1a, 0x48, 0x8d, 0x34, 0xc8, 0x62],
  [0xf1, 0xf5, 0x48, 0xdb, 0x26, 0x62, 0xf1, 0xdd, 0x48, 0xc2, 0xc8, 0x02, 0x62, 0xf1, 0xfe, 0xc9],
  [0x7f, 0xdb, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x48, 0x39, 0xd3, 0x7d, 0xdd, 0x62, 0xf1],
  [0x65, 0x48, 0xdb, 0xc2, 0x62, 0xf2, 0xfe, 0x48, 0x39, 0xc8, 0xc5, 0xf9, 0x93, 0xc9, 0x66, 0x90],
  [0x80, 0xf9, 0xff, 0x74, 0x07, 0xc5, 0xf8, 0x77, 0x31, 0xc0, 0x5d, 0xc3, 0xc5, 0xf8, 0x77, 0xb8],
  [0x01, 0x00, 0x00, 0x00, 0x5d, 0xc3, 0xe8, 0xa5, 0xb6, 0xf3, 0xff, 0x90]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.allAbsLE64AVX512`. -/
def allAbsLE64AVX512Bytes : Array UInt8 := ⟨allAbsLE64AVX512ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.allAbsLE64AVX512`. -/
def allAbsLE64AVX512Targets (off : Nat) : Option String :=
  match off with
  | 0xd6 => some "runtime.panicBounds"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.allAbsLE64AVX512` (none). -/
def allAbsLE64AVX512Rodata (_ : Nat) : Option (BitVec 64) := none

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (284 bytes, 81 insns) for `internal/simd.minNoNaN64AVX2`. -/
def minNoNaN64AVX2ByteChunks : List (List UInt8) := [
  [0x55, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x40, 0x48, 0x89, 0x44, 0x24, 0x50, 0x48, 0x85, 0xdb],
  [0x0f, 0x86, 0x00, 0x01, 0x00, 0x00, 0x90, 0xc5, 0xfb, 0x10, 0x05, 0x61, 0xef, 0x04, 0x00, 0xc4],
  [0xe2, 0x7d, 0x19, 0xc0, 0x31, 0xc9, 0xc5, 0xfe, 0x7f, 0xc1, 0xc5, 0xfe, 0x7f, 0xca, 0xeb, 0x35],
  [0x48, 0x8d, 0x34, 0xc8, 0xc5, 0xfe, 0x6f, 0x1e, 0x48, 0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76, 0x20],
  [0xc5, 0xfe, 0x6f, 0x26, 0xc5, 0xe5, 0xc2, 0xeb, 0x03, 0xc5, 0xdd, 0xc2, 0xf4, 0x03, 0xc4, 0xe3],
  [0x65, 0x4c, 0xd9, 0x50, 0xc4, 0xe3, 0x5d, 0x4c, 0xe1, 0x60, 0xc5, 0xe5, 0x5d, 0xc0, 0xc5, 0xdd],
  [0x5d, 0xd2, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x48, 0x39, 0xd3, 0x7d, 0xc2, 0xeb, 0x1a],
  [0x48, 0x8d, 0x34, 0xc8, 0xc5, 0xfe, 0x6f, 0x1e, 0xc5, 0xe5, 0xc2, 0xe3, 0x03, 0xc4, 0xe3, 0x65],
  [0x4c, 0xd9, 0x40, 0xc5, 0xfd, 0x5d, 0xc3, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x04, 0x48, 0x39],
  [0xd3, 0x7d, 0xdd, 0xc5, 0xfd, 0x5d, 0xc2, 0xc5, 0xfe, 0x7f, 0x04, 0x24, 0xc5, 0xf8, 0x77, 0x90],
  [0x90, 0x48, 0x8d, 0x54, 0x24, 0x20, 0x48, 0x8d, 0x34, 0x24, 0x44, 0x0f, 0x10, 0x36, 0x44, 0x0f],
  [0x11, 0x32, 0x44, 0x0f, 0x10, 0x76, 0x10, 0x44, 0x0f, 0x11, 0x72, 0x10, 0x31, 0xd2, 0xf2, 0x0f],
  [0x10, 0x05, 0xba, 0xee, 0x04, 0x00, 0xeb, 0x06, 0x48, 0xff, 0xc2, 0x0f, 0x10, 0xc1, 0x48, 0x83],
  [0xfa, 0x04, 0x7d, 0x19, 0xf2, 0x0f, 0x10, 0x4c, 0xd4, 0x20, 0x66, 0x0f, 0x2e, 0xc1, 0x66, 0x90],
  [0x77, 0xe6, 0x0f, 0x10, 0xc8, 0xeb, 0xe1, 0x48, 0xff, 0xc1, 0x0f, 0x10, 0xc1, 0x48, 0x39, 0xcb],
  [0x7e, 0x10, 0xf2, 0x0f, 0x10, 0x0c, 0xc8, 0x66, 0x0f, 0x2e, 0xc1, 0x77, 0xea, 0x0f, 0x10, 0xc8],
  [0xeb, 0xe5, 0x0f, 0x57, 0xc9, 0x66, 0x0f, 0x2e, 0xc1, 0x75, 0x05, 0x7a, 0x03, 0x0f, 0x57, 0xc0],
  [0x48, 0x83, 0xc4, 0x40, 0x5d, 0xc3, 0xe8, 0xe5, 0x7d, 0xf4, 0xff, 0x90]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.minNoNaN64AVX2`. -/
def minNoNaN64AVX2Bytes : Array UInt8 := ⟨minNoNaN64AVX2ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.minNoNaN64AVX2`. -/
def minNoNaN64AVX2Targets (off : Nat) : Option String :=
  match off with
  | 0x116 => some "runtime.panicBounds"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.minNoNaN64AVX2` (`+Inf` at `0x17`, `0xbe`). -/
def minNoNaN64AVX2Rodata (off : Nat) : Option (BitVec 64) :=
  match off with
  | 0x17 | 0xbe => some 0x7ff0000000000000#64
  | _ => none

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (389 bytes, 97 insns) for `internal/simd.minNoNaN64AVX512`. -/
def minNoNaN64AVX512ByteChunks : List (List UInt8) := [
  [0x4c, 0x8d, 0x64, 0x24, 0xf8, 0x4d, 0x3b, 0x66, 0x10, 0x0f, 0x86, 0x4d, 0x01, 0x00, 0x00, 0x55],
  [0x48, 0x89, 0xe5, 0x48, 0x83, 0xc4, 0x80, 0x48, 0x89, 0x84, 0x24, 0x90, 0x00, 0x00, 0x00, 0x90],
  [0x48, 0x85, 0xdb, 0x0f, 0x86, 0x2d, 0x01, 0x00, 0x00, 0x90, 0xc5, 0xfb, 0x10, 0x05, 0x8e, 0x87],
  [0x04, 0x00, 0x62, 0xf2, 0xfd, 0x48, 0x19, 0xc0, 0x31, 0xc9, 0x62, 0xf1, 0xfe, 0x48, 0x7f, 0xc1],
  [0x62, 0xf1, 0xfe, 0x48, 0x7f, 0xca, 0xeb, 0x41, 0x48, 0x8d, 0x34, 0xc8, 0x62, 0xf1, 0xfe, 0x48],
  [0x6f, 0x1e, 0x48, 0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76, 0x40, 0x62, 0xf1, 0xfe, 0x48, 0x6f, 0x26],
  [0x62, 0xf1, 0xe5, 0x48, 0xc2, 0xcb, 0x03, 0x62, 0xf1, 0xdd, 0x48, 0xc2, 0xd4, 0x03, 0x62, 0xf2],
  [0xe5, 0x49, 0x64, 0xd9, 0x62, 0xf2, 0xdd, 0x4a, 0x64, 0xe1, 0x62, 0xf1, 0xe5, 0x48, 0x5d, 0xc0],
  [0x62, 0xf1, 0xdd, 0x48, 0x5d, 0xd2, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x10, 0x48, 0x39, 0xd3],
  [0x7d, 0xb6, 0xeb, 0x20, 0x48, 0x8d, 0x34, 0xc8, 0x62, 0xf1, 0xfe, 0x48, 0x6f, 0x1e, 0x62, 0xf1],
  [0xe5, 0x48, 0xc2, 0xcb, 0x03, 0x62, 0xf2, 0xe5, 0x49, 0x64, 0xd9, 0x62, 0xf1, 0xfd, 0x48, 0x5d],
  [0xc3, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x48, 0x39, 0xd3, 0x7d, 0xd7, 0x62, 0xf1, 0xfd],
  [0x48, 0x5d, 0xc2, 0x62, 0xf1, 0xfe, 0x48, 0x7f, 0x04, 0x24, 0xc5, 0xf8, 0x77, 0x90, 0x90, 0x48],
  [0x8d, 0x54, 0x24, 0x40, 0x48, 0x8d, 0x34, 0x24, 0x44, 0x0f, 0x10, 0x36, 0x44, 0x0f, 0x11, 0x32],
  [0x44, 0x0f, 0x10, 0x76, 0x10, 0x44, 0x0f, 0x11, 0x72, 0x10, 0x44, 0x0f, 0x10, 0x76, 0x20, 0x44],
  [0x0f, 0x11, 0x72, 0x20, 0x44, 0x0f, 0x10, 0x76, 0x30, 0x44, 0x0f, 0x11, 0x72, 0x30, 0x31, 0xd2],
  [0xf2, 0x0f, 0x10, 0x05, 0xb8, 0x86, 0x04, 0x00, 0xeb, 0x06, 0x48, 0xff, 0xc2, 0x0f, 0x10, 0xc1],
  [0x48, 0x83, 0xfa, 0x08, 0x7d, 0x17, 0xf2, 0x0f, 0x10, 0x4c, 0xd4, 0x40, 0x66, 0x0f, 0x2e, 0xc1],
  [0x77, 0xe8, 0x0f, 0x10, 0xc8, 0xeb, 0xe3, 0x48, 0xff, 0xc1, 0x0f, 0x10, 0xc1, 0x48, 0x39, 0xcb],
  [0x7e, 0x10, 0xf2, 0x0f, 0x10, 0x0c, 0xc8, 0x66, 0x0f, 0x2e, 0xc1, 0x77, 0xea, 0x0f, 0x10, 0xc8],
  [0xeb, 0xe5, 0x0f, 0x57, 0xc9, 0x66, 0x0f, 0x2e, 0xc1, 0x75, 0x05, 0x7a, 0x03, 0x0f, 0x57, 0xc0],
  [0x48, 0x83, 0xec, 0x80, 0x5d, 0xc3, 0xe8, 0xe5, 0x15, 0xf4, 0xff, 0x90, 0x48, 0x89, 0x44, 0x24],
  [0x08, 0x48, 0x89, 0x5c, 0x24, 0x10, 0x48, 0x89, 0x4c, 0x24, 0x18, 0xe8, 0x90, 0xf9, 0xf3, 0xff],
  [0x48, 0x8b, 0x44, 0x24, 0x08, 0x48, 0x8b, 0x5c, 0x24, 0x10, 0x48, 0x8b, 0x4c, 0x24, 0x18, 0x90],
  [0xe9, 0x7b, 0xfe, 0xff, 0xff]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.minNoNaN64AVX512`. -/
def minNoNaN64AVX512Bytes : Array UInt8 := ⟨minNoNaN64AVX512ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.minNoNaN64AVX512`. -/
def minNoNaN64AVX512Targets (off : Nat) : Option String :=
  match off with
  | 0x156 => some "runtime.panicBounds"
  | 0x16b => some "runtime.morestack_noctxt.abi0"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.minNoNaN64AVX512` (`+Inf` at `0x2a`, `0x100`). -/
def minNoNaN64AVX512Rodata (off : Nat) : Option (BitVec 64) :=
  match off with
  | 0x2a | 0x100 => some 0x7ff0000000000000#64
  | _ => none

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (414 bytes, 107 insns) for `internal/simd.initReducedRow64AVX2`. -/
def initReducedRow64AVX2ByteChunks : List (List UInt8) := [
  [0x55, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x20, 0x48, 0x89, 0x44, 0x24, 0x30, 0x48, 0x89, 0x7c],
  [0x24, 0x48, 0x4c, 0x89, 0x4c, 0x24, 0x60, 0x66, 0x0f, 0x1f, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00],
  [0x48, 0x85, 0xdb, 0x0f, 0x86, 0x6f, 0x01, 0x00, 0x00, 0x48, 0x85, 0xf6, 0x0f, 0x86, 0x61, 0x01],
  [0x00, 0x00, 0x4d, 0x85, 0xd2, 0x0f, 0x86, 0x53, 0x01, 0x00, 0x00, 0xc5, 0x83, 0x11, 0xc0, 0xc5],
  [0xfb, 0x10, 0x0d, 0x19, 0x4a, 0x04, 0x00, 0xc4, 0xe2, 0x7d, 0x19, 0xc9, 0x90, 0x90, 0x90, 0xc4],
  [0xe2, 0x7d, 0x19, 0xc0, 0xc5, 0xf5, 0xc2, 0xd1, 0x00, 0x31, 0xc9, 0xc5, 0xfe, 0x7f, 0xd3, 0xc5],
  [0xfe, 0x7f, 0xcc, 0xeb, 0x63, 0x48, 0x8d, 0x34, 0xcf, 0x4d, 0x8d, 0x04, 0xc9, 0x4c, 0x8d, 0x14],
  [0xc8, 0xc5, 0xfe, 0x6f, 0x2e, 0xc4, 0xc1, 0x55, 0x5c, 0x28, 0xc5, 0xd5, 0x5c, 0xe8, 0x48, 0x8d],
  [0x34, 0xcf, 0x48, 0x8d, 0x76, 0x20, 0xc5, 0xfe, 0x6f, 0x36, 0x49, 0x8d, 0x34, 0xc9, 0x48, 0x8d],
  [0x76, 0x20, 0xc5, 0xcd, 0x5c, 0x36, 0xc4, 0xc1, 0x7e, 0x7f, 0x2a, 0xc5, 0xcd, 0x5c, 0xf0, 0x48],
  [0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76, 0x20, 0xc5, 0xfe, 0x7f, 0x36, 0xc5, 0xd5, 0xc2, 0xfd, 0x00],
  [0xc5, 0x4d, 0xc2, 0xc6, 0x00, 0xc5, 0xc5, 0xdb, 0xd2, 0xc5, 0xbd, 0xdb, 0xdb, 0xc5, 0xd5, 0x5d],
  [0xc9, 0xc5, 0xcd, 0x5d, 0xe4, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x48, 0x39, 0xd3, 0x7d],
  [0x94, 0xeb, 0x2e, 0x48, 0x8d, 0x34, 0xcf, 0x4d, 0x8d, 0x04, 0xc9, 0x4c, 0x8d, 0x14, 0xc8, 0xc5],
  [0xfe, 0x6f, 0x2e, 0xc4, 0xc1, 0x55, 0x5c, 0x28, 0xc5, 0xd5, 0x5c, 0xe8, 0xc4, 0xc1, 0x7e, 0x7f],
  [0x2a, 0xc5, 0xd5, 0xc2, 0xf5, 0x00, 0xc5, 0xed, 0xdb, 0xd6, 0xc5, 0xf5, 0x5d, 0xcd, 0x48, 0x89],
  [0xd1, 0x48, 0x8d, 0x51, 0x04, 0x48, 0x39, 0xd3, 0x7d, 0xc9, 0xc5, 0xf5, 0x5d, 0xc4, 0xc5, 0xfe],
  [0x7f, 0x04, 0x24, 0xc5, 0xed, 0xdb, 0xc3, 0xc5, 0xfd, 0x50, 0xc0, 0x3c, 0x0f, 0x74, 0x19, 0xc5],
  [0xf8, 0x77, 0x48, 0xb8, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f, 0x66, 0x48, 0x0f, 0x6e],
  [0xc0, 0x90, 0x48, 0x83, 0xc4, 0x20, 0x5d, 0xc3, 0xc5, 0xf8, 0x77, 0xf2, 0x0f, 0x10, 0x04, 0x24],
  [0xf2, 0x0f, 0x10, 0x4c, 0x24, 0x08, 0x0f, 0x10, 0xd0, 0xf2, 0x0f, 0x5d, 0xd1, 0x0f, 0x10, 0xca],
  [0xf2, 0x0f, 0x5d, 0xd0, 0x66, 0x0f, 0xeb, 0xd1, 0xf2, 0x0f, 0x10, 0x44, 0x24, 0x10, 0xf2, 0x0f],
  [0x10, 0x4c, 0x24, 0x18, 0x0f, 0x10, 0xd8, 0xf2, 0x0f, 0x5d, 0xc1, 0x0f, 0x10, 0xc8, 0xf2, 0x0f],
  [0x5d, 0xc3, 0x66, 0x0f, 0xeb, 0xc1, 0x0f, 0x10, 0xca, 0xf2, 0x0f, 0x5d, 0xd0, 0x0f, 0x10, 0xc2],
  [0xf2, 0x0f, 0x5d, 0xc1, 0x66, 0x0f, 0xeb, 0xc2, 0x48, 0x83, 0xc4, 0x20, 0x5d, 0xc3, 0xe8, 0x4d],
  [0xd8, 0xf3, 0xff, 0xe8, 0x48, 0xd8, 0xf3, 0xff, 0xe8, 0x43, 0xd8, 0xf3, 0xff, 0x90]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.initReducedRow64AVX2`. -/
def initReducedRow64AVX2Bytes : Array UInt8 := ⟨initReducedRow64AVX2ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.initReducedRow64AVX2`. -/
def initReducedRow64AVX2Targets (off : Nat) : Option String :=
  match off with
  | 0x18e | 0x193 | 0x198 => some "runtime.panicBounds"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.initReducedRow64AVX2` (`+Inf` at `0x3f`). -/
def initReducedRow64AVX2Rodata (off : Nat) : Option (BitVec 64) :=
  match off with
  | 0x3f => some 0x7ff0000000000000#64
  | _ => none

/-- Compiled Go 1.27 `GOARCH=amd64 GOEXPERIMENT=simd` ELF `.text` bytes (587 bytes, 136 insns) for `internal/simd.initReducedRow64AVX512`. -/
def initReducedRow64AVX512ByteChunks : List (List UInt8) := [
  [0x55, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x40, 0x48, 0x89, 0x44, 0x24, 0x50, 0x48, 0x89, 0x7c],
  [0x24, 0x68, 0x4c, 0x89, 0x8c, 0x24, 0x80, 0x00, 0x00, 0x00, 0x66, 0x0f, 0x1f, 0x44, 0x00, 0x00],
  [0x48, 0x85, 0xdb, 0x0f, 0x86, 0x1c, 0x02, 0x00, 0x00, 0x48, 0x85, 0xf6, 0x0f, 0x86, 0x0b, 0x02],
  [0x00, 0x00, 0x4d, 0x85, 0xd2, 0x0f, 0x86, 0xfd, 0x01, 0x00, 0x00, 0xc5, 0x83, 0x11, 0xc0, 0xc5],
  [0xfb, 0x10, 0x0d, 0x19, 0x21, 0x04, 0x00, 0x62, 0xf2, 0xfd, 0x48, 0x19, 0xc9, 0x62, 0xf1, 0xf5],
  [0x48, 0xc2, 0xc9, 0x00, 0x90, 0x90, 0x90, 0x62, 0xf2, 0xfd, 0x48, 0x19, 0xc0, 0x62, 0xf2, 0xfe],
  [0x48, 0x38, 0xd1, 0x31, 0xc9, 0x62, 0xf1, 0xfe, 0x48, 0x7f, 0xd3, 0x62, 0xf1, 0xfe, 0x48, 0x7f],
  [0xcc, 0xeb, 0x7d, 0x48, 0x8d, 0x34, 0xcf, 0x4d, 0x8d, 0x04, 0xc9, 0x4c, 0x8d, 0x14, 0xc8, 0x62],
  [0xf1, 0xfe, 0x48, 0x6f, 0x2e, 0x62, 0xd1, 0xd5, 0x48, 0x5c, 0x28, 0x62, 0xf1, 0xd5, 0x48, 0x5c],
  [0xe8, 0x48, 0x8d, 0x34, 0xcf, 0x48, 0x8d, 0x76, 0x40, 0x62, 0xf1, 0xfe, 0x48, 0x6f, 0x36, 0x49],
  [0x8d, 0x34, 0xc9, 0x48, 0x8d, 0x76, 0x40, 0x62, 0xf1, 0xcd, 0x48, 0x5c, 0x36, 0x62, 0xd1, 0xfe],
  [0x48, 0x7f, 0x2a, 0x62, 0xf1, 0xcd, 0x48, 0x5c, 0xf0, 0x48, 0x8d, 0x34, 0xc8, 0x48, 0x8d, 0x76],
  [0x40, 0x62, 0xf1, 0xfe, 0x48, 0x7f, 0x36, 0x62, 0xf1, 0xd5, 0x48, 0xc2, 0xcd, 0x00, 0x62, 0xf1],
  [0xcd, 0x48, 0xc2, 0xd6, 0x00, 0x62, 0xf1, 0xfe, 0xc9, 0x7f, 0xd2, 0x62, 0xf1, 0xfe, 0xca, 0x7f],
  [0xdb, 0x62, 0xf1, 0xd5, 0x48, 0x5d, 0xc9, 0x62, 0xf1, 0xcd, 0x48, 0x5d, 0xe4, 0x48, 0x89, 0xd1],
  [0x48, 0x8d, 0x51, 0x10, 0x48, 0x39, 0xd3, 0x0f, 0x8d, 0x76, 0xff, 0xff, 0xff, 0xeb, 0x3a, 0x48],
  [0x8d, 0x34, 0xcf, 0x4d, 0x8d, 0x04, 0xc9, 0x4c, 0x8d, 0x14, 0xc8, 0x62, 0xf1, 0xfe, 0x48, 0x6f],
  [0x2e, 0x62, 0xd1, 0xd5, 0x48, 0x5c, 0x28, 0x62, 0xf1, 0xd5, 0x48, 0x5c, 0xe8, 0x62, 0xd1, 0xfe],
  [0x48, 0x7f, 0x2a, 0x62, 0xf1, 0xd5, 0x48, 0xc2, 0xcd, 0x00, 0x62, 0xf1, 0xfe, 0xc9, 0x7f, 0xd2],
  [0x62, 0xf1, 0xf5, 0x48, 0x5d, 0xcd, 0x48, 0x89, 0xd1, 0x48, 0x8d, 0x51, 0x08, 0x0f, 0x1f, 0x00],
  [0x48, 0x39, 0xd3, 0x7d, 0xba, 0x62, 0xf1, 0xf5, 0x48, 0x5d, 0xc4, 0x62, 0xf1, 0xfe, 0x48, 0x7f],
  [0x04, 0x24, 0x62, 0xf1, 0x6d, 0x48, 0xdb, 0xc3, 0x62, 0xf2, 0xfe, 0x48, 0x39, 0xc8, 0xc5, 0xf9],
  [0x93, 0xc1, 0x3c, 0xff, 0x74, 0x19, 0xc5, 0xf8, 0x77, 0x48, 0xb8, 0x01, 0x00, 0x00, 0x00, 0x00],
  [0x00, 0xf8, 0x7f, 0x66, 0x48, 0x0f, 0x6e, 0xc0, 0x90, 0x48, 0x83, 0xc4, 0x40, 0x5d, 0xc3, 0xc5],
  [0xf8, 0x77, 0xf2, 0x0f, 0x10, 0x04, 0x24, 0xf2, 0x0f, 0x10, 0x4c, 0x24, 0x08, 0x0f, 0x10, 0xd0],
  [0xf2, 0x0f, 0x5d, 0xd1, 0x0f, 0x10, 0xca, 0xf2, 0x0f, 0x5d, 0xd0, 0x66, 0x0f, 0xeb, 0xd1, 0xf2],
  [0x0f, 0x10, 0x44, 0x24, 0x10, 0xf2, 0x0f, 0x10, 0x4c, 0x24, 0x18, 0x0f, 0x10, 0xd8, 0xf2, 0x0f],
  [0x5d, 0xc1, 0x0f, 0x10, 0xc8, 0xf2, 0x0f, 0x5d, 0xc3, 0x66, 0x0f, 0xeb, 0xc8, 0x0f, 0x10, 0xc2],
  [0xf2, 0x0f, 0x5d, 0xc1, 0x0f, 0x10, 0xc8, 0xf2, 0x0f, 0x5d, 0xca, 0x66, 0x0f, 0xeb, 0xc1, 0xf2],
  [0x0f, 0x10, 0x4c, 0x24, 0x20, 0xf2, 0x0f, 0x10, 0x54, 0x24, 0x28, 0x0f, 0x10, 0xd9, 0xf2, 0x0f],
  [0x5d, 0xca, 0x0f, 0x10, 0xd1, 0xf2, 0x0f, 0x5d, 0xcb, 0x66, 0x0f, 0xeb, 0xd1, 0xf2, 0x0f, 0x10],
  [0x4c, 0x24, 0x30, 0xf2, 0x0f, 0x10, 0x5c, 0x24, 0x38, 0x0f, 0x10, 0xe1, 0xf2, 0x0f, 0x5d, 0xcb],
  [0x0f, 0x10, 0xd9, 0xf2, 0x0f, 0x5d, 0xcc, 0x66, 0x0f, 0xeb, 0xcb, 0x0f, 0x10, 0xda, 0xf2, 0x0f],
  [0x5d, 0xd1, 0x0f, 0x10, 0xca, 0xf2, 0x0f, 0x5d, 0xd3, 0x66, 0x0f, 0xeb, 0xd1, 0x0f, 0x10, 0xc8],
  [0xf2, 0x0f, 0x5d, 0xca, 0x0f, 0x10, 0xd1, 0xf2, 0x0f, 0x5d, 0xc8, 0x66, 0x0f, 0xeb, 0xca, 0x0f],
  [0x10, 0xc1, 0x48, 0x83, 0xc4, 0x40, 0x5d, 0xc3, 0xe8, 0xa3, 0xae, 0xf3, 0xff, 0x0f, 0x1f, 0x00],
  [0xe8, 0x9b, 0xae, 0xf3, 0xff, 0xe8, 0x96, 0xae, 0xf3, 0xff, 0x90]
]
/-- Flattened x86-64 machine-code byte array for `internal/simd.initReducedRow64AVX512`. -/
def initReducedRow64AVX512Bytes : Array UInt8 := ⟨initReducedRow64AVX512ByteChunks.flatten⟩
/-- External call relocation target table for `internal/simd.initReducedRow64AVX512`. -/
def initReducedRow64AVX512Targets (off : Nat) : Option String :=
  match off with
  | 0x238 | 0x240 | 0x245 => some "runtime.panicBounds"
  | _ => none
/-- RIP-relative `.rodata` constant table for `internal/simd.initReducedRow64AVX512` (`+Inf` at `0x3f`). -/
def initReducedRow64AVX512Rodata (off : Nat) : Option (BitVec 64) :=
  match off with
  | 0x3f => some 0x7ff0000000000000#64
  | _ => none

/-!
### Part II: Byte-Level amd64 / VEX (`C4`/`C5`) / EVEX (`62`) Decoder
-/

/-- Vector width encoded by VEX `L` or EVEX `L'L`. -/
inductive VecWidth where
  | xmm128
  | ymm256
  | zmm512
  deriving DecidableEq, Repr, Inhabited

/-- x86-64 conditional branch predicates (4-bit encoding `0x0..0xF`). -/
inductive X86Cond where
  | o | no | b | ae | e | ne | be | a
  | s | ns | p | np | l | ge | le | g
  deriving DecidableEq, Repr, Inhabited

/-- Effective memory operand decoded from ModR/M + SIB + displacement. -/
inductive MemAddr where
  | ripRel (disp32 : Int)
  | baseIdxDisp (base : Option (BitVec 4)) (index : Option (BitVec 4 × Nat)) (disp : Int)
  deriving DecidableEq, Repr, Inhabited

/-- ModR/M `r/m` operand: either a 5-bit register index or a memory address. -/
inductive RmOp where
  | reg (r : BitVec 5)
  | mem (addr : MemAddr)
  deriving DecidableEq, Repr, Inhabited

/-- EVEX masking, zeroing (`{z}`), and embedded broadcast (`{1toN}`) modifiers. -/
structure EvexMask where
  /-- 3-bit opmask register index (`k0..k7`; `0` means unmasked). -/
  kmask : BitVec 3 := 0#3
  /-- Whether zero-masking (`{z}`) is enabled instead of merge-masking. -/
  zeroing : Bool := false
  /-- Whether embedded memory broadcast (`{1toN}`) is enabled. -/
  broadcast : Bool := false
  deriving DecidableEq, Repr, Inhabited

/-- Typed amd64 scalar, SSE, VEX-256, and EVEX-512 instruction AST. -/
inductive Amd64VecInsn where
  | nop
  | pushReg (r : BitVec 4)
  | popReg (r : BitVec 4)
  | ret
  | movRR64 (dst src : BitVec 4)
  | movMR64 (addr : MemAddr) (src : BitVec 4)
  | movRM64 (dst : BitVec 4) (addr : MemAddr)
  | movImm32 (dst : BitVec 4) (imm : BitVec 32)
  | movImm64 (dst : BitVec 4) (imm : BitVec 64)
  | lea64 (dst : BitVec 4) (addr : MemAddr)
  | xorRR32 (dst src : BitVec 4)
  | addImm64 (dst : BitVec 4) (imm : BitVec 64)
  | subImm64 (dst : BitVec 4) (imm : BitVec 64)
  | inc64 (dst : BitVec 4)
  | testRR64 (lhs rhs : BitVec 4)
  | cmpRR64 (lhs rhs : BitVec 4)
  | cmpRM64 (lhs : BitVec 4) (addr : MemAddr)
  | cmpImm64 (lhs : BitVec 4) (imm : BitVec 64)
  | cmpImm8 (lhs : BitVec 4) (imm : BitVec 8)
  | jmpRel (rel : Int)
  | jccRel (cond : X86Cond) (rel : Int)
  | callRel (rel : Int)
  -- Legacy non-VEX SSE instructions (trigger AVX-SSE transition penalty if upperDirty = true).
  | sseMovupsRR (dst src : BitVec 4)
  | sseMovupsLoad (dst : BitVec 4) (addr : MemAddr)
  | sseMovupsStore (addr : MemAddr) (src : BitVec 4)
  | sseMovsdLoad (dst : BitVec 4) (addr : MemAddr)
  | sseMovqFromGpr (dst src : BitVec 4)
  | sseXorpsRR (dst src : BitVec 4)
  | ssePorRR (dst src : BitVec 4)
  | sseMinsdRR (dst src : BitVec 4)
  | sseUcomisdRR (lhs rhs : BitVec 4)
  -- VEX (C4/C5) and EVEX (62) vector and opmask instructions.
  | vzeroupper
  | vmovsdLoad (dst : BitVec 5) (addr : MemAddr)
  | vmovsdRR (dst src1 src2 : BitVec 5)
  | vmovqFromGpr (dst : BitVec 5) (src : BitVec 4)
  | vbroadcastsd (isEVEX : Bool) (w : VecWidth) (dst : BitVec 5) (src : RmOp) (mask : EvexMask)
  | vpbroadcastq (isEVEX : Bool) (w : VecWidth) (dst : BitVec 5) (src : RmOp) (mask : EvexMask)
  | vmovdquLoad (isEVEX : Bool) (w : VecWidth) (dst : BitVec 5) (addr : MemAddr) (mask : EvexMask)
  | vmovdquStore (isEVEX : Bool) (w : VecWidth) (addr : MemAddr) (src : BitVec 5) (mask : EvexMask)
  | vmovdquRR (isEVEX : Bool) (w : VecWidth) (dst src : BitVec 5) (mask : EvexMask)
  | vpand (isEVEX : Bool) (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (mask : EvexMask)
  | vsubpd (isEVEX : Bool) (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (mask : EvexMask)
  | vminpd (isEVEX : Bool) (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (mask : EvexMask)
  | vcmppdVex (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (imm8 : UInt8)
  | vcmppdEvex (w : VecWidth) (dstK : BitVec 3) (src1 : BitVec 5) (src2 : RmOp)
      (imm8 : UInt8) (mask : EvexMask)
  | vpblendvb (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (maskReg : BitVec 5)
  | vpblendmq (w : VecWidth) (dst src1 : BitVec 5) (src2 : RmOp) (mask : EvexMask)
  | vmovmskpd (w : VecWidth) (dst : BitVec 4) (src : BitVec 5)
  | vpmovm2q (w : VecWidth) (dst : BitVec 5) (srcK : BitVec 3)
  | vpmovq2m (w : VecWidth) (dstK : BitVec 3) (src : BitVec 5)
  | kmovbToGpr (dst : BitVec 4) (srcK : BitVec 3)
  deriving DecidableEq, Repr, Inhabited

/-- Decoded instruction paired with its byte offset and next instruction byte offset. -/
structure DecodedAmd64Insn where
  /-- Starting byte offset of the instruction within the function's `.text` slice. -/
  offset : Nat
  /-- Byte offset immediately following this instruction (fall-through PC). -/
  nextOffset : Nat
  /-- Decoded typed instruction AST. -/
  insn : Amd64VecInsn
  deriving DecidableEq, Repr, Inhabited

private def decodeX86Cond (n : Nat) : X86Cond :=
  match n &&& 0xf with
  | 0 => .o | 1 => .no | 2 => .b | 3 => .ae
  | 4 => .e | 5 => .ne | 6 => .be | 7 => .a
  | 8 => .s | 9 => .ns | 10 => .p | 11 => .np
  | 12 => .l | 13 => .ge | 14 => .le | _ => .g

private def readU32LEWith? (getB : Nat → Option UInt8) (pos : Nat) : Option Nat :=
  match getB pos, getB (pos + 1), getB (pos + 2), getB (pos + 3) with
  | some b0, some b1, some b2, some b3 =>
    some (b0.toNat ||| (b1.toNat <<< 8) ||| (b2.toNat <<< 16) ||| (b3.toNat <<< 24))
  | _, _, _, _ => none

private def readU64LEWith? (getB : Nat → Option UInt8) (pos : Nat) : Option (BitVec 64) :=
  match readU32LEWith? getB pos, readU32LEWith? getB (pos + 4) with
  | some lo, some hi => some (BitVec.ofNat 64 (lo ||| (hi <<< 32)))
  | _, _ => none

private def signExt8Int (b : UInt8) : Int :=
  let v := b.toNat
  if v >= 128 then (v : Int) - 256 else (v : Int)

private def signExt32Int (u : Nat) : Int :=
  let v := u &&& 0xffffffff
  if v >= 0x80000000 then (v : Int) - 0x100000000 else (v : Int)

private def signExt8BV64 (b : UInt8) : BitVec 64 :=
  (BitVec.ofNat 8 b.toNat).signExtend 64

private def decodeModRMWith (getB : Nat → Option UInt8) (pos : Nat)
    (rexR rexX rexB : Bool) (evexR2 evexX2 : Bool := false) (disp8Scale : Nat := 1) :
    Option (BitVec 5 × RmOp × Nat) :=
  match getB pos with
  | none => none
  | some modrm =>
    let mod := (modrm.toNat >>> 6) &&& 0x3
    let reg3 := (modrm.toNat >>> 3) &&& 0x7
    let rm3 := modrm.toNat &&& 0x7
    let reg5 : BitVec 5 := BitVec.ofNat 5
      ((if evexR2 then 16 else 0) ||| (if rexR then 8 else 0) ||| reg3)
    let cur := pos + 1
    if mod == 3 then
      let rm5 : BitVec 5 := BitVec.ofNat 5
        ((if evexX2 then 16 else 0) ||| (if rexB then 8 else 0) ||| rm3)
      some (reg5, .reg rm5, cur)
    else if mod == 0 && rm3 == 5 then
      match readU32LEWith? getB cur with
      | some d32 => some (reg5, .mem (.ripRel (signExt32Int d32)), cur + 4)
      | none => none
    else if rm3 == 4 then
      match getB cur with
      | none => none
      | some sib =>
        let scale := 1 <<< ((sib.toNat >>> 6) &&& 0x3)
        let idx3 := (sib.toNat >>> 3) &&& 0x7
        let base3 := sib.toNat &&& 0x7
        let idx4 := (if rexX then 8 else 0) ||| idx3
        let idxOpt : Option (BitVec 4 × Nat) :=
          if idx4 == 4 then none else some (BitVec.ofNat 4 idx4, scale)
        let cur1 := cur + 1
        if mod == 0 && base3 == 5 then
          match readU32LEWith? getB cur1 with
          | some d32 => some (reg5, .mem (.baseIdxDisp none idxOpt (signExt32Int d32)), cur1 + 4)
          | none => none
        else
          let base4 : BitVec 4 := BitVec.ofNat 4 ((if rexB then 8 else 0) ||| base3)
          if mod == 0 then
            some (reg5, .mem (.baseIdxDisp (some base4) idxOpt 0), cur1)
          else if mod == 1 then
            match getB cur1 with
            | some d8 => some (reg5, .mem (.baseIdxDisp (some base4) idxOpt (signExt8Int d8 * disp8Scale)), cur1 + 1)
            | none => none
          else
            match readU32LEWith? getB cur1 with
            | some d32 => some (reg5, .mem (.baseIdxDisp (some base4) idxOpt (signExt32Int d32)), cur1 + 4)
            | none => none
    else
      let base4 : BitVec 4 := BitVec.ofNat 4 ((if rexB then 8 else 0) ||| rm3)
      if mod == 0 then
        some (reg5, .mem (.baseIdxDisp (some base4) none 0), cur)
      else if mod == 1 then
        match getB cur with
        | some d8 => some (reg5, .mem (.baseIdxDisp (some base4) none (signExt8Int d8 * disp8Scale)), cur + 1)
        | none => none
      else
        match readU32LEWith? getB cur with
        | some d32 => some (reg5, .mem (.baseIdxDisp (some base4) none (signExt32Int d32)), cur + 4)
        | none => none

/-- Decode one amd64 scalar, SSE, VEX (`C4`/`C5`), or EVEX (`62`) instruction via byte reader `getB`. -/
def decodeAmd64VecInsnWith (getB : Nat → Option UInt8) (off : Nat) : Option (Amd64VecInsn × Nat) :=
  match getB off with
  | none => none
  | some b0 =>
    -- 2-byte VEX prefix (0xC5).
    if b0 == 0xc5 then
      match getB (off + 1), getB (off + 2) with
      | some b1, some op =>
        let rexR := ((b1.toNat >>> 7) &&& 1) == 0
        let vvvv : BitVec 5 := BitVec.ofNat 5 (((b1.toNat >>> 3) &&& 0xf) ^^^ 0xf)
        let isL := ((b1.toNat >>> 2) &&& 1) == 1
        let pp := b1.toNat &&& 0x3
        let w : VecWidth := if isL then .ymm256 else .xmm128
        if b1 == 0xf8 && op == 0x77 then
          some (.vzeroupper, off + 3)
        else
          match decodeModRMWith getB (off + 3) rexR false false with
          | none => none
          | some (reg5, rmOp, nextPos) =>
            match pp, op.toNat with
            | 3, 0x10 =>
              match rmOp with
              | .mem addr => some (.vmovsdLoad reg5 addr, nextPos)
              | .reg rm => some (.vmovsdRR reg5 vvvv rm, nextPos)
            | 3, 0x11 =>
              match rmOp with
              | .reg rm => some (.vmovsdRR rm vvvv reg5, nextPos)
              | .mem _ => none
            | 1, 0x50 =>
              match rmOp with
              | .reg rm => some (.vmovmskpd w (BitVec.ofNat 4 reg5.toNat) rm, nextPos)
              | .mem _ => none
            | 1, 0x5c => some (.vsubpd false w reg5 vvvv rmOp {}, nextPos)
            | 1, 0x5d => some (.vminpd false w reg5 vvvv rmOp {}, nextPos)
            | 2, 0x6f =>
              match rmOp with
              | .mem addr => some (.vmovdquLoad false w reg5 addr {}, nextPos)
              | .reg rm => some (.vmovdquRR false w reg5 rm {}, nextPos)
            | 2, 0x7f =>
              match rmOp with
              | .mem addr => some (.vmovdquStore false w addr reg5 {}, nextPos)
              | .reg rm => some (.vmovdquRR false w rm reg5 {}, nextPos)
            | 1, 0x93 =>
              match rmOp with
              | .reg rm => some (.kmovbToGpr (BitVec.ofNat 4 reg5.toNat) (BitVec.ofNat 3 rm.toNat), nextPos)
              | .mem _ => none
            | 1, 0xc2 =>
              match getB nextPos with
              | some imm8 => some (.vcmppdVex w reg5 vvvv rmOp imm8, nextPos + 1)
              | none => none
            | 1, 0xdb => some (.vpand false w reg5 vvvv rmOp {}, nextPos)
            | _, _ => none
      | _, _ => none
    -- 3-byte VEX prefix (0xC4).
    else if b0 == 0xc4 then
      match getB (off + 1), getB (off + 2), getB (off + 3) with
      | some b1, some b2, some op =>
        let rexR := ((b1.toNat >>> 7) &&& 1) == 0
        let rexX := ((b1.toNat >>> 6) &&& 1) == 0
        let rexB := ((b1.toNat >>> 5) &&& 1) == 0
        let vmap := b1.toNat &&& 0x1f
        let rexW := ((b2.toNat >>> 7) &&& 1) == 1
        let vvvv : BitVec 5 := BitVec.ofNat 5 (((b2.toNat >>> 3) &&& 0xf) ^^^ 0xf)
        let isL := ((b2.toNat >>> 2) &&& 1) == 1
        let pp := b2.toNat &&& 0x3
        let w : VecWidth := if isL then .ymm256 else .xmm128
        match decodeModRMWith getB (off + 4) rexR rexX rexB with
        | none => none
        | some (reg5, rmOp, nextPos) =>
          match vmap, pp, op.toNat with
          | 1, 1, 0x5c => some (.vsubpd false w reg5 vvvv rmOp {}, nextPos)
          | 1, 1, 0x5d => some (.vminpd false w reg5 vvvv rmOp {}, nextPos)
          | 1, 1, 0x6e =>
            if rexW then
              match rmOp with
              | .reg rm => some (.vmovqFromGpr reg5 (BitVec.ofNat 4 rm.toNat), nextPos)
              | .mem _ => none
            else none
          | 1, 2, 0x6f =>
            match rmOp with
            | .mem addr => some (.vmovdquLoad false w reg5 addr {}, nextPos)
            | .reg rm => some (.vmovdquRR false w reg5 rm {}, nextPos)
          | 1, 2, 0x7f =>
            match rmOp with
            | .mem addr => some (.vmovdquStore false w addr reg5 {}, nextPos)
            | .reg rm => some (.vmovdquRR false w rm reg5 {}, nextPos)
          | 1, 1, 0xdb => some (.vpand false w reg5 vvvv rmOp {}, nextPos)
          | 2, 1, 0x19 => some (.vbroadcastsd false w reg5 rmOp {}, nextPos)
          | 2, 1, 0x59 => some (.vpbroadcastq false w reg5 rmOp {}, nextPos)
          | 3, 1, 0x4c =>
            match getB nextPos with
            | some imm8 =>
              let maskReg : BitVec 5 := BitVec.ofNat 5 ((imm8.toNat >>> 4) &&& 0xf)
              some (.vpblendvb w reg5 vvvv rmOp maskReg, nextPos + 1)
            | none => none
          | _, _, _ => none
      | _, _, _ => none
    -- 4-byte EVEX prefix (0x62).
    else if b0 == 0x62 then
      match getB (off + 1), getB (off + 2), getB (off + 3), getB (off + 4) with
      | some p0, some p1, some p2, some op =>
        if (p0.toNat &&& 0x08) != 0 || (p1.toNat &&& 0x04) != 0x04 then none
        else
          let rexR := ((p0.toNat >>> 7) &&& 1) == 0
          let rexX := ((p0.toNat >>> 6) &&& 1) == 0
          let rexB := ((p0.toNat >>> 5) &&& 1) == 0
          let evexR2 := ((p0.toNat >>> 4) &&& 1) == 0
          let emap := p0.toNat &&& 0x7
          let evexW := ((p1.toNat >>> 7) &&& 1) == 1
          let vvvvLow := ((p1.toNat >>> 3) &&& 0xf) ^^^ 0xf
          let pp := p1.toNat &&& 0x3
          let zeroing := ((p2.toNat >>> 7) &&& 1) == 1
          let ll := (p2.toNat >>> 5) &&& 0x3
          let bcast := ((p2.toNat >>> 4) &&& 1) == 1
          let evexV2 := ((p2.toNat >>> 3) &&& 1) == 0
          let aaa := p2.toNat &&& 0x7
          let vvvv5 : BitVec 5 := BitVec.ofNat 5 ((if evexV2 then 16 else 0) ||| vvvvLow)
          let w? : Option VecWidth :=
            match ll with | 0 => some .xmm128 | 1 => some .ymm256 | 2 => some .zmm512 | _ => none
          match w? with
          | none => none
          | some w =>
            let mask : EvexMask := { kmask := BitVec.ofNat 3 aaa, zeroing := zeroing, broadcast := bcast }
            let isTuple1Scalar64 := emap == 2 && pp == 1 && (op.toNat == 0x19 || op.toNat == 0x59)
            let dispScale :=
              if bcast || isTuple1Scalar64 then 8
              else match w with | .xmm128 => 16 | .ymm256 => 32 | .zmm512 => 64
            match decodeModRMWith getB (off + 5) rexR rexX rexB evexR2 rexX dispScale with
            | none => none
            | some (reg5, rmOp, nextPos) =>
              -- Reject unmodeled EVEX reg-reg embedded rounding (`EVEX.b = 1` with `mod = 3`).
              if bcast && (match rmOp with | .reg _ => true | .mem _ => false) then none
              else
              match emap, pp, op.toNat with
              | 1, 1, 0x5c => some (.vsubpd true w reg5 vvvv5 rmOp mask, nextPos)
              | 1, 1, 0x5d => some (.vminpd true w reg5 vvvv5 rmOp mask, nextPos)
              | 1, 2, 0x6f =>
                match rmOp with
                | .mem addr => some (.vmovdquLoad true w reg5 addr mask, nextPos)
                | .reg rm => some (.vmovdquRR true w reg5 rm mask, nextPos)
              | 1, 2, 0x7f =>
                match rmOp with
                | .mem addr => some (.vmovdquStore true w addr reg5 mask, nextPos)
                | .reg rm => some (.vmovdquRR true w rm reg5 mask, nextPos)
              | 1, 1, 0xc2 =>
                match getB nextPos with
                | some imm8 => some (.vcmppdEvex w (BitVec.ofNat 3 reg5.toNat) vvvv5 rmOp imm8 mask, nextPos + 1)
                | none => none
              | 1, 1, 0xdb =>
                -- Unmasked non-broadcast `vpandd` (`EVEX.W=0`) is bit-identical to `vpandq`;
                -- reject masked or broadcast 32-bit `vpandd` since 32-bit lane masks are unmodeled.
                if !evexW && (aaa != 0 || bcast) then none
                else some (.vpand true w reg5 vvvv5 rmOp mask, nextPos)
              | 2, 1, 0x19 => some (.vbroadcastsd true w reg5 rmOp mask, nextPos)
              | 2, 1, 0x59 => some (.vpbroadcastq true w reg5 rmOp mask, nextPos)
              | 2, 2, 0x38 =>
                match rmOp with
                | .reg rm => some (.vpmovm2q w reg5 (BitVec.ofNat 3 rm.toNat), nextPos)
                | .mem _ => none
              | 2, 2, 0x39 =>
                match rmOp with
                | .reg rm => some (.vpmovq2m w (BitVec.ofNat 3 reg5.toNat) rm, nextPos)
                | .mem _ => none
              | 2, 1, 0x64 => some (.vpblendmq w reg5 vvvv5 rmOp mask, nextPos)
              | _, _, _ => none
      | _, _, _, _ => none
    -- Legacy prefixes (66, F2, F3) + optional REX (40..4F) + scalar/SSE opcodes.
    else
      let (p66, pf2, pf3, pos1) :=
        if b0 == 0x66 then (true, false, false, off + 1)
        else if b0 == 0xf2 then (false, true, false, off + 1)
        else if b0 == 0xf3 then (false, false, true, off + 1)
        else (false, false, false, off)
      match getB pos1 with
      | none => none
      | some bNext =>
        let (rexW, rexR, rexX, rexB, pos2) :=
          if 0x40 <= bNext.toNat && bNext.toNat <= 0x4f then
            let r := bNext.toNat
            (((r >>> 3) &&& 1) == 1, ((r >>> 2) &&& 1) == 1,
             ((r >>> 1) &&& 1) == 1, (r &&& 1) == 1, pos1 + 1)
          else
            (false, false, false, false, pos1)
        match getB pos2 with
        | none => none
        | some op =>
          let posOp := pos2 + 1
          if op == 0x90 && !rexB then
            some (.nop, posOp)
          else if 0x50 <= op.toNat && op.toNat <= 0x57 then
            some (.pushReg (BitVec.ofNat 4 ((if rexB then 8 else 0) ||| (op.toNat - 0x50))), posOp)
          else if 0x58 <= op.toNat && op.toNat <= 0x5f then
            some (.popReg (BitVec.ofNat 4 ((if rexB then 8 else 0) ||| (op.toNat - 0x58))), posOp)
          else if op == 0xc3 then
            some (.ret, posOp)
          else if op == 0x3c then
            match getB posOp with
            | some imm8 => some (.cmpImm8 0#4 (BitVec.ofNat 8 imm8.toNat), posOp + 1)
            | none => none
          else if 0x70 <= op.toNat && op.toNat <= 0x7f then
            match getB posOp with
            | some rel8 => some (.jccRel (decodeX86Cond (op.toNat - 0x70)) (signExt8Int rel8), posOp + 1)
            | none => none
          else if op == 0xeb then
            match getB posOp with
            | some rel8 => some (.jmpRel (signExt8Int rel8), posOp + 1)
            | none => none
          else if op == 0xe8 then
            match readU32LEWith? getB posOp with
            | some rel32 => some (.callRel (signExt32Int rel32), posOp + 4)
            | none => none
          else if op == 0xe9 then
            match readU32LEWith? getB posOp with
            | some rel32 => some (.jmpRel (signExt32Int rel32), posOp + 4)
            | none => none
          else if 0xb8 <= op.toNat && op.toNat <= 0xbf then
            let dst : BitVec 4 := BitVec.ofNat 4 ((if rexB then 8 else 0) ||| (op.toNat - 0xb8))
            if rexW then
              match readU64LEWith? getB posOp with
              | some imm64 => some (.movImm64 dst imm64, posOp + 8)
              | none => none
            else
              match readU32LEWith? getB posOp with
              | some imm32 => some (.movImm32 dst (BitVec.ofNat 32 imm32), posOp + 4)
              | none => none
          else if op == 0x31 then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .reg rm, nextPos) =>
              if !rexW then some (.xorRR32 (BitVec.ofNat 4 rm.toNat) (BitVec.ofNat 4 reg5.toNat), nextPos)
              else none
            | _ => none
          else if op == 0x39 && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .reg rm, nextPos) => some (.cmpRR64 (BitVec.ofNat 4 rm.toNat) (BitVec.ofNat 4 reg5.toNat), nextPos)
            | _ => none
          else if op == 0x3b && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .mem addr, nextPos) => some (.cmpRM64 (BitVec.ofNat 4 reg5.toNat) addr, nextPos)
            | _ => none
          else if op == 0x85 && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .reg rm, nextPos) => some (.testRR64 (BitVec.ofNat 4 rm.toNat) (BitVec.ofNat 4 reg5.toNat), nextPos)
            | _ => none
          else if op == 0x89 && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .reg rm, nextPos) => some (.movRR64 (BitVec.ofNat 4 rm.toNat) (BitVec.ofNat 4 reg5.toNat), nextPos)
            | some (reg5, .mem addr, nextPos) => some (.movMR64 addr (BitVec.ofNat 4 reg5.toNat), nextPos)
            | none => none
          else if op == 0x8b && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .reg rm, nextPos) => some (.movRR64 (BitVec.ofNat 4 reg5.toNat) (BitVec.ofNat 4 rm.toNat), nextPos)
            | some (reg5, .mem addr, nextPos) => some (.movRM64 (BitVec.ofNat 4 reg5.toNat) addr, nextPos)
            | none => none
          else if op == 0x8d && rexW then
            match decodeModRMWith getB posOp rexR rexX rexB with
            | some (reg5, .mem addr, nextPos) => some (.lea64 (BitVec.ofNat 4 reg5.toNat) addr, nextPos)
            | _ => none
          else if op == 0x80 then
            let hasRex := pos2 > pos1
            match decodeModRMWith getB posOp false rexX rexB with
            | some (reg5, .reg rm, nextPos) =>
              -- Without a REX prefix, rm ∈ {4,5,6,7} addresses AH/CH/DH/BH rather than SPL/BPL/SIL/DIL.
              if reg5.toNat == 7 && (hasRex || rm.toNat < 4) then
                match getB nextPos with
                | some imm8 => some (.cmpImm8 (BitVec.ofNat 4 rm.toNat) (BitVec.ofNat 8 imm8.toNat), nextPos + 1)
                | none => none
              else none
            | _ => none
          else if op == 0x83 && rexW then
            match decodeModRMWith getB posOp false rexX rexB with
            | some (reg5, .reg rm, nextPos) =>
              match getB nextPos with
              | none => none
              | some imm8 =>
                let imm64 := signExt8BV64 imm8
                let dst := BitVec.ofNat 4 rm.toNat
                match reg5.toNat with
                | 0 => some (.addImm64 dst imm64, nextPos + 1)
                | 5 => some (.subImm64 dst imm64, nextPos + 1)
                | 7 => some (.cmpImm64 dst imm64, nextPos + 1)
                | _ => none
            | _ => none
          else if op == 0xff && rexW then
            match decodeModRMWith getB posOp false rexX rexB with
            | some (reg5, .reg rm, nextPos) =>
              if reg5.toNat == 0 then some (.inc64 (BitVec.ofNat 4 rm.toNat), nextPos) else none
            | _ => none
          else if op == 0x0f then
            match getB posOp with
            | none => none
            | some op2 =>
              let posOp2 := posOp + 1
              if 0x80 <= op2.toNat && op2.toNat <= 0x8f then
                match readU32LEWith? getB posOp2 with
                | some rel32 => some (.jccRel (decodeX86Cond (op2.toNat - 0x80)) (signExt32Int rel32), posOp2 + 4)
                | none => none
              else if op2 == 0x1f then
                match decodeModRMWith getB posOp2 rexR rexX rexB with
                | some (_, _, nextPos) => some (.nop, nextPos)
                | none => none
              else
                match decodeModRMWith getB posOp2 rexR rexX rexB with
                | none => none
                | some (reg5, rmOp, nextPos) =>
                  let r := BitVec.ofNat 4 reg5.toNat
                  match p66, pf2, pf3, op2.toNat, rmOp with
                  | false, true, false, 0x10, .mem addr => some (.sseMovsdLoad r addr, nextPos)
                  | false, false, false, 0x10, .reg rm => some (.sseMovupsRR r (BitVec.ofNat 4 rm.toNat), nextPos)
                  | false, false, false, 0x10, .mem addr => some (.sseMovupsLoad r addr, nextPos)
                  | false, false, false, 0x11, .mem addr => some (.sseMovupsStore addr r, nextPos)
                  | true, false, false, 0x2e, .reg rm => some (.sseUcomisdRR r (BitVec.ofNat 4 rm.toNat), nextPos)
                  | false, false, false, 0x57, .reg rm => some (.sseXorpsRR r (BitVec.ofNat 4 rm.toNat), nextPos)
                  | false, true, false, 0x5d, .reg rm => some (.sseMinsdRR r (BitVec.ofNat 4 rm.toNat), nextPos)
                  | true, false, false, 0x6e, .reg rm =>
                    if rexW then some (.sseMovqFromGpr r (BitVec.ofNat 4 rm.toNat), nextPos) else none
                  | true, false, false, 0xeb, .reg rm => some (.ssePorRR r (BitVec.ofNat 4 rm.toNat), nextPos)
                  | _, _, _, _, _ => none
          else none

/-- Decode one amd64 scalar, SSE, VEX (`C4`/`C5`), or EVEX (`62`) instruction at `off` in `bytes`. -/
def decodeAmd64VecInsn (bytes : Array UInt8) (off : Nat) : Option (Amd64VecInsn × Nat) :=
  decodeAmd64VecInsnWith (fun i => bytes[i]?) off

private def decodeAmd64ProgramAux (getB : Nat → Option UInt8) (byteLen : Nat) :
    Nat → Nat → List DecodedAmd64Insn → Option (Array DecodedAmd64Insn)
  | 0, off, acc =>
    if off == byteLen then some acc.reverse.toArray else none
  | fuel + 1, off, acc =>
    if off == byteLen then
      some acc.reverse.toArray
    else if off > byteLen then
      none
    else
      match decodeAmd64VecInsnWith getB off with
      | none => none
      | some (insn, nextOff) =>
        if nextOff <= off || nextOff > byteLen then none
        else decodeAmd64ProgramAux getB byteLen fuel nextOff
          ({ offset := off, nextOffset := nextOff, insn := insn } :: acc)

/-- Decode a complete amd64 ELF `.text` byte array into typed instructions with byte offsets. -/
def decodeAmd64Program (bytes : Array UInt8) : Option (Array DecodedAmd64Insn) :=
  decodeAmd64ProgramAux (fun i => bytes[i]?) bytes.size (bytes.size + 1) 0 []

/-- O(N/16 + 16) chunked byte accessor avoiding deep list traversal on 16-byte chunk tables. -/
def chunkGet16 (chunks : List (List UInt8)) (pos : Nat) : Option UInt8 :=
  match chunks[pos / 16]? with
  | some c => c[pos % 16]?
  | none => none

private def chunks16WellFormed : List (List UInt8) → Bool
  | [] => true
  | [c] => c.length <= 16
  | c :: rest => c.length == 16 && chunks16WellFormed rest

private def chunksTotalLen (chunks : List (List UInt8)) : Nat :=
  chunks.foldl (fun acc c => acc + c.length) 0

private theorem chunksTotalLen_eq_flatten (chunks : List (List UInt8)) :
    chunksTotalLen chunks = chunks.flatten.length := by
  unfold chunksTotalLen
  suffices ∀ acc, chunks.foldl (fun a c => a + c.length) acc = acc + chunks.flatten.length by
    simpa using this 0
  induction chunks with
  | nil => intro acc; simp
  | cons c rest ih =>
    intro acc
    simp only [List.foldl_cons, List.flatten_cons, List.length_append, ih]
    omega

private theorem chunkGet16_eq_flatten (chunks : List (List UInt8))
    (hWf : chunks16WellFormed chunks = true) (pos : Nat) :
    chunkGet16 chunks pos = (⟨chunks.flatten⟩ : Array UInt8)[pos]? := by
  have hArr : (⟨chunks.flatten⟩ : Array UInt8)[pos]? = chunks.flatten[pos]? := by
    by_cases h : pos < chunks.flatten.length
    · rw [Array.getElem?_eq_getElem (by simpa using h), List.getElem?_eq_getElem h]; rfl
    · rw [Array.getElem?_eq_none (by simpa using Nat.le_of_not_lt h),
          List.getElem?_eq_none (Nat.le_of_not_lt h)]
  rw [hArr]
  induction chunks generalizing pos with
  | nil => simp [chunkGet16]
  | cons c rest ih =>
    cases rest with
    | nil =>
      simp only [chunks16WellFormed, decide_eq_true_eq] at hWf
      simp only [chunkGet16, List.flatten_cons, List.flatten_nil, List.append_nil]
      by_cases h16 : pos < 16
      · have hDiv : pos / 16 = 0 := Nat.div_eq_of_lt h16
        have hMod : pos % 16 = pos := Nat.mod_eq_of_lt h16
        simp [hDiv, hMod]
      · have hDivPos : 1 ≤ pos / 16 := by omega
        have hGe : c.length ≤ pos := by omega
        rw [List.getElem?_eq_none (by simp; omega), List.getElem?_eq_none hGe]
    | cons c2 rest2 =>
      simp only [chunks16WellFormed, Bool.and_eq_true, beq_iff_eq] at hWf
      rcases hWf with ⟨hc16, hRestWf⟩
      simp only [List.flatten_cons]
      by_cases h16 : pos < 16
      · have hDiv : pos / 16 = 0 := Nat.div_eq_of_lt h16
        have hMod : pos % 16 = pos := Nat.mod_eq_of_lt h16
        have hLtC : pos < c.length := by omega
        simp only [chunkGet16, hDiv, hMod, List.getElem?_cons_zero]
        rw [List.getElem?_append_left hLtC]
      · have hGeC : c.length ≤ pos := by omega
        rw [List.getElem?_append_right hGeC, hc16]
        have hDivSucc : pos / 16 = (pos - 16) / 16 + 1 := by omega
        have hModEq : pos % 16 = (pos - 16) % 16 := by omega
        have hSub := ih hRestWf (pos - 16)
        unfold chunkGet16 at hSub ⊢
        rw [hDivSucc, hModEq]
        simpa [List.getElem?_cons_succ] using hSub

/-- Chunk-accelerated program decoder equal to `decodeAmd64Program ⟨chunks.flatten⟩`. -/
def decodeAmd64ProgramChunks (chunks : List (List UInt8)) : Option (Array DecodedAmd64Insn) :=
  let n := chunksTotalLen chunks
  decodeAmd64ProgramAux (chunkGet16 chunks) n (n + 1) 0 []

theorem decodeAmd64ProgramChunks_eq (chunks : List (List UInt8))
    (hWf : chunks16WellFormed chunks = true) :
    decodeAmd64Program ⟨chunks.flatten⟩ = decodeAmd64ProgramChunks chunks := by
  unfold decodeAmd64Program decodeAmd64ProgramChunks
  have hLen : (⟨chunks.flatten⟩ : Array UInt8).size = chunksTotalLen chunks := by
    simp [chunksTotalLen_eq_flatten]
  have hFun : (fun i => (⟨chunks.flatten⟩ : Array UInt8)[i]?) = chunkGet16 chunks := by
    funext i; exact (chunkGet16_eq_flatten chunks hWf i).symm
  rw [hLen, hFun]

/-!
### Part III: Hardware-Faithful Operational Machine Semantics (`Amd64VecState`)
-/

/-- Eight 64-bit IEEE-754 lanes modeling one `ZMM0..ZMM31` vector register. -/
structure Vec512 where
  /-- 64-bit lane 0 (bits 0..63). -/
  l0 : BitVec 64 := 0#64
  /-- 64-bit lane 1 (bits 64..127). -/
  l1 : BitVec 64 := 0#64
  /-- 64-bit lane 2 (bits 128..191). -/
  l2 : BitVec 64 := 0#64
  /-- 64-bit lane 3 (bits 192..255). -/
  l3 : BitVec 64 := 0#64
  /-- 64-bit lane 4 (bits 256..319). -/
  l4 : BitVec 64 := 0#64
  /-- 64-bit lane 5 (bits 320..383). -/
  l5 : BitVec 64 := 0#64
  /-- 64-bit lane 6 (bits 384..447). -/
  l6 : BitVec 64 := 0#64
  /-- 64-bit lane 7 (bits 448..511). -/
  l7 : BitVec 64 := 0#64
  deriving DecidableEq, Repr, Inhabited

/-- Broadcasts a single 64-bit word across all eight lanes of a `Vec512`. -/
@[inline] def Vec512.broadcast (x : BitVec 64) : Vec512 :=
  { l0 := x, l1 := x, l2 := x, l3 := x, l4 := x, l5 := x, l6 := x, l7 := x }

/-- Zeros the upper 384 bits (lanes 2..7), preserving the low 128-bit XMM portion. -/
@[inline] def Vec512.zeroHi128 (v : Vec512) : Vec512 :=
  { l0 := v.l0, l1 := v.l1, l2 := 0#64, l3 := 0#64, l4 := 0#64, l5 := 0#64, l6 := 0#64, l7 := 0#64 }

/-- Zeros the upper 256 bits (lanes 4..7), preserving the low 256-bit YMM portion. -/
@[inline] def Vec512.zeroHi256 (v : Vec512) : Vec512 :=
  { l0 := v.l0, l1 := v.l1, l2 := v.l2, l3 := v.l3, l4 := 0#64, l5 := 0#64, l6 := 0#64, l7 := 0#64 }

/-- Zeros lanes above the specified `VecWidth` (`xmm128`, `ymm256`, or `zmm512`). -/
@[inline] def Vec512.applyWidth (w : VecWidth) (v : Vec512) : Vec512 :=
  match w with
  | .xmm128 => v.zeroHi128
  | .ymm256 => v.zeroHi256
  | .zmm512 => v

/-- Applies a binary 64-bit lane operation element-wise across two `Vec512` values. -/
@[inline] def Vec512.zipWith (f : BitVec 64 → BitVec 64 → BitVec 64) (a b : Vec512) : Vec512 :=
  { l0 := f a.l0 b.l0, l1 := f a.l1 b.l1, l2 := f a.l2 b.l2, l3 := f a.l3 b.l3
    l4 := f a.l4 b.l4, l5 := f a.l5 b.l5, l6 := f a.l6 b.l6, l7 := f a.l7 b.l7 }

/-- Extracts bit `i` of an 8-bit AVX-512 opmask value as a `Bool`. -/
@[inline] def Vec512.maskBit (k : BitVec 8) (i : Nat) : Bool :=
  ((k.toNat >>> i) &&& 1) == 1

/-- Apply EVEX opmask merging (`{k}`) or zero-masking (`{k}{z}`) to 8 lanes. -/
@[inline] def Vec512.applyEvexMask (kmask : BitVec 3) (kVal : BitVec 8) (zeroing : Bool)
    (oldDst newVal : Vec512) : Vec512 :=
  if kmask == 0#3 then newVal
  else
    let pick (i : Nat) (o n : BitVec 64) : BitVec 64 :=
      if Vec512.maskBit kVal i then n else if zeroing then 0#64 else o
    { l0 := pick 0 oldDst.l0 newVal.l0, l1 := pick 1 oldDst.l1 newVal.l1
      l2 := pick 2 oldDst.l2 newVal.l2, l3 := pick 3 oldDst.l3 newVal.l3
      l4 := pick 4 oldDst.l4 newVal.l4, l5 := pick 5 oldDst.l5 newVal.l5
      l6 := pick 6 oldDst.l6 newVal.l6, l7 := pick 7 oldDst.l7 newVal.l7 }

/-- Convert an 8-bit opmask register into 8 all-1s/all-0s 64-bit lanes (`VPMOVM2Q`). -/
@[inline] def Vec512.ofOpmask8 (k : BitVec 8) : Vec512 :=
  let lane (i : Nat) : BitVec 64 := if Vec512.maskBit k i then 0xffffffffffffffff#64 else 0#64
  { l0 := lane 0, l1 := lane 1, l2 := lane 2, l3 := lane 3
    l4 := lane 4, l5 := lane 5, l6 := lane 6, l7 := lane 7 }

/-- Extract the sign bit (bit 63) of each 64-bit lane into an 8-bit mask (`VPMOVQ2M`). -/
@[inline] def Vec512.toOpmask8 (v : Vec512) : BitVec 8 :=
  let b (x : BitVec 64) (sh : Nat) : Nat := if x.msb then 1 <<< sh else 0
  BitVec.ofNat 8
    (b v.l0 0 ||| b v.l1 1 ||| b v.l2 2 ||| b v.l3 3 |||
     b v.l4 4 ||| b v.l5 5 ||| b v.l6 6 ||| b v.l7 7)

/-- Extract the sign bits of the 2 XMM lanes (`VMOVMSKPD reg, xmm`). -/
@[inline] def Vec512.movmskpd2 (v : Vec512) : BitVec 64 :=
  let b (x : BitVec 64) (sh : Nat) : Nat := if x.msb then 1 <<< sh else 0
  BitVec.ofNat 64 (b v.l0 0 ||| b v.l1 1)

/-- Extract the sign bits of the 4 YMM lanes (`VMOVMSKPD reg, ymm`). -/
@[inline] def Vec512.movmskpd4 (v : Vec512) : BitVec 64 :=
  let b (x : BitVec 64) (sh : Nat) : Nat := if x.msb then 1 <<< sh else 0
  BitVec.ofNat 64 (b v.l0 0 ||| b v.l1 1 ||| b v.l2 2 ||| b v.l3 3)

/-- Byte-wise MSB blend on a 64-bit lane (`VPBLENDVB`). -/
def vpblendvb64 (src1 src2 mask : BitVec 64) : BitVec 64 :=
  let blendByte (sh : Nat) : BitVec 64 :=
    let sel := ((mask >>> (sh + 7)) &&& 1#64) == 1#64
    let chosen := if sel then (src2 >>> sh) &&& 0xff#64 else (src1 >>> sh) &&& 0xff#64
    chosen <<< sh
  blendByte 0 ||| blendByte 8 ||| blendByte 16 ||| blendByte 24 |||
  blendByte 32 ||| blendByte 40 ||| blendByte 48 ||| blendByte 56

/-- Intel SDM Vol. 2A Table 3-1 full 32-entry `VCMPPD` predicate evaluation on one binary64 lane
(`imm8[4:0]`; bit 4 selects signaling vs quiet invalid-operation behavior without altering the
boolean comparison result, so `imm8[3:0]` determines the exact mask bit for all 32 entries). -/
def x86VcmppdPred (imm8 : UInt8) (a b : BitVec 64) : Bool :=
  match imm8.toNat &&& 0x0f with
  | 0  => f64Eq a b
  | 1  => f64Lt a b
  | 2  => f64Le a b
  | 3  => f64IsNaN a || f64IsNaN b
  | 4  => !f64Eq a b
  | 5  => !f64Lt a b
  | 6  => !f64Le a b
  | 7  => !(f64IsNaN a || f64IsNaN b)
  | 8  => f64Eq a b || f64IsNaN a || f64IsNaN b
  | 9  => !f64Ge a b
  | 10 => !f64Lt b a
  | 11 => false
  | 12 => !(f64IsNaN a || f64IsNaN b) && !f64Eq a b
  | 13 => f64Ge a b
  | 14 => f64Lt b a
  | _  => true

/-- Set the quiet bit (bit 51) on an x86-64 IEEE-754 binary64 NaN payload. -/
@[inline] def x86QuietNaN64 (x : BitVec 64) : BitVec 64 :=
  x ||| 0x0008000000000000#64

/-- Intel x86-64 QNaN floating-point indefinite (`-qNaN`, `0xfff8000000000000`). -/
def x86DefaultIndef64 : BitVec 64 :=
  0xfff8000000000000#64

/-- Intel SDM `VSUBPD` / `SUBSD` binary64 subtraction (`a - b`) with silicon-exact NaN
semantics: `NaN(a)` quiets `a`; else `NaN(b)` quiets `b` preserving `b`'s sign bit (no negation);
else same-sign infinity cancellation returns `x86DefaultIndef64` (`0xfff8000000000000`);
else exact nearest-even rational `a - b` preserving IEEE-754 signed zero (`-0.0 - +0.0 = -0.0`). -/
def x86F64Sub (a b : BitVec 64) : BitVec 64 :=
  if f64IsNaN a then x86QuietNaN64 a
  else if f64IsNaN b then x86QuietNaN64 b
  else if (f64IsPosInf a && f64IsPosInf b) || (f64IsNegInf a && f64IsNegInf b) then
    x86DefaultIndef64
  else if f64IsPosInf a || f64IsNegInf b then
    f64PosInf
  else if f64IsNegInf a || f64IsPosInf b then
    f64NegInf
  else
    match float64BitsToRat (UInt64.ofNat a.toNat), float64BitsToRat (UInt64.ofNat b.toNat) with
    | some qa, some qb =>
      let negZ := f64IsZero a && f64IsZero b && f64Sign a && !f64Sign b
      ratToFloat64Bits (qa - qb) negZ
    | _, _ => x86DefaultIndef64

/-- Intel SDM `VADDPD` / `ADDSD` binary64 addition (`a + b`) with silicon-exact NaN semantics
and `x86DefaultIndef64` (`0xfff8000000000000`) on opposite-sign infinity cancellation. -/
def x86F64Add (a b : BitVec 64) : BitVec 64 :=
  if f64IsNaN a then x86QuietNaN64 a
  else if f64IsNaN b then x86QuietNaN64 b
  else if (f64IsPosInf a && f64IsNegInf b) || (f64IsNegInf a && f64IsPosInf b) then
    x86DefaultIndef64
  else if f64IsPosInf a || f64IsPosInf b then
    f64PosInf
  else if f64IsNegInf a || f64IsNegInf b then
    f64NegInf
  else
    match float64BitsToRat (UInt64.ofNat a.toNat), float64BitsToRat (UInt64.ofNat b.toNat) with
    | some qa, some qb =>
      let negZ := f64IsZero a && f64IsZero b && f64Sign a && f64Sign b
      ratToFloat64Bits (qa + qb) negZ
    | _, _ => x86DefaultIndef64

/-- x86 `RFLAGS` subset exercised by Go 1.27 SIMD loops and `UCOMISD`. -/
structure X86Flags where
  /-- Zero flag (`ZF`). -/
  zf : Bool := false
  /-- Carry flag (`CF`). -/
  cf : Bool := false
  /-- Sign flag (`SF`). -/
  sf : Bool := false
  /-- Overflow flag (`OF`). -/
  of : Bool := false
  /-- Parity flag (`PF`). -/
  pf : Bool := false
  deriving DecidableEq, Repr, Inhabited

/-- Evaluate an x86-64 conditional jump predicate against `X86Flags`. -/
def evalX86Cond (c : X86Cond) (f : X86Flags) : Bool :=
  match c with
  | .o => f.of
  | .no => !f.of
  | .b => f.cf
  | .ae => !f.cf
  | .e => f.zf
  | .ne => !f.zf
  | .be => f.cf || f.zf
  | .a => !f.cf && !f.zf
  | .s => f.sf
  | .ns => !f.sf
  | .p => f.pf
  | .np => !f.pf
  | .l => f.sf != f.of
  | .ge => f.sf == f.of
  | .le => f.zf || (f.sf != f.of)
  | .g => !f.zf && (f.sf == f.of)

/-- Hardware Intel SDM parity flag (PF): `true` iff the low byte has an even number of set bits. -/
def x86Parity8 (b : BitVec 8) : Bool :=
  !(b.getLsbD 0 ^^ b.getLsbD 1 ^^ b.getLsbD 2 ^^ b.getLsbD 3 ^^
    b.getLsbD 4 ^^ b.getLsbD 5 ^^ b.getLsbD 6 ^^ b.getLsbD 7)

/-- 64-bit x86 subtraction flags (`lhs - rhs`). -/
def x86SubFlags64 (lhs rhs : BitVec 64) : X86Flags :=
  let res := lhs - rhs
  { zf := res == 0#64
    cf := BitVec.ult lhs rhs
    sf := res.msb
    of := (lhs.msb != rhs.msb) && (res.msb != lhs.msb)
    pf := x86Parity8 (res.extractLsb' 0 8) }

/-- 8-bit x86 subtraction flags (`lhs - rhs`). -/
def x86SubFlags8 (lhs rhs : BitVec 8) : X86Flags :=
  let res := lhs - rhs
  { zf := res == 0#8
    cf := BitVec.ult lhs rhs
    sf := res.msb
    of := (lhs.msb != rhs.msb) && (res.msb != lhs.msb)
    pf := x86Parity8 res }

/-- 64-bit x86 `TEST` flags (`lhs &&& rhs`). -/
def x86TestFlags64 (lhs rhs : BitVec 64) : X86Flags :=
  let res := lhs &&& rhs
  { zf := res == 0#64, cf := false, sf := res.msb, of := false, pf := x86Parity8 (res.extractLsb' 0 8) }

/-- 64-bit x86 `ADD` flags (`lhs + rhs`). -/
def x86AddFlags64 (lhs rhs : BitVec 64) : X86Flags :=
  let res := lhs + rhs
  { zf := res == 0#64
    cf := BitVec.ult res lhs
    sf := res.msb
    of := (lhs.msb == rhs.msb) && (res.msb != lhs.msb)
    pf := x86Parity8 (res.extractLsb' 0 8) }

/-- Hardware Intel SDM `UCOMISD lhs, rhs` flags. -/
def x86UcomisdFlags (lhs rhs : BitVec 64) : X86Flags :=
  if f64IsNaN lhs || f64IsNaN rhs then
    { zf := true, pf := true, cf := true, sf := false, of := false }
  else if f64Eq lhs rhs then
    { zf := true, pf := false, cf := false, sf := false, of := false }
  else if f64Lt lhs rhs then
    { zf := false, pf := false, cf := true, sf := false, of := false }
  else
    { zf := false, pf := false, cf := false, sf := false, of := false }

/-- Machine execution status for decoded amd64 vector programs. -/
inductive Amd64ExecStatus where
  | running
  | returned
  | trapMorestack
  | trapPanicBounds
  | callExternal (target : String)
  | trapDecode (off : Nat)
  deriving DecidableEq, Repr, Inhabited

/-- Architectural x86-64 + AVX2 + AVX-512 machine state. -/
structure Amd64VecState where
  /-- General-purpose 64-bit register file (`RAX..R15`). -/
  gpr : BitVec 4 → BitVec 64
  /-- 512-bit SIMD vector register file (`ZMM0..ZMM31`). -/
  zmm : BitVec 5 → Vec512
  /-- 8-bit AVX-512 opmask register file (`k0..k7`). -/
  kmask : BitVec 3 → BitVec 8
  /-- Architectural `RFLAGS` condition-code subset. -/
  flags : X86Flags := {}
  /-- Whether upper YMM/ZMM vector state is currently dirty (cleared by `VZEROUPPER`). -/
  upperDirty : Bool := false
  /-- Latched hazard flag set if a legacy SSE instruction executes while `upperDirty` is true. -/
  sseHazard : Bool := false
  /-- Word-addressed 64-bit memory map. -/
  mem : BitVec 64 → BitVec 64
  /-- Current instruction byte offset within the function's `.text` slice. -/
  pc : Nat := 0
  /-- Current execution status (running, returned, external call, or trap). -/
  status : Amd64ExecStatus := .running

/-- Functional update of general-purpose register `r` to `v`. -/
@[inline] def setGpr (gpr : BitVec 4 → BitVec 64) (r : BitVec 4) (v : BitVec 64) :
    BitVec 4 → BitVec 64 :=
  fun r' => if r' == r then v else gpr r'

/-- Functional update of vector register `r` to `v`. -/
@[inline] def setZmm (zmm : BitVec 5 → Vec512) (r : BitVec 5) (v : Vec512) :
    BitVec 5 → Vec512 :=
  fun r' => if r' == r then v else zmm r'

/-- Functional update of opmask register `k` to `v`. -/
@[inline] def setKmask (kmask : BitVec 3 → BitVec 8) (k : BitVec 3) (v : BitVec 8) :
    BitVec 3 → BitVec 8 :=
  fun k' => if k' == k then v else kmask k'

/-- Functional update storing 64-bit word `v` at address `addr` in memory. -/
@[inline] def write64 (mem : BitVec 64 → BitVec 64) (addr v : BitVec 64) :
    BitVec 64 → BitVec 64 :=
  fun a => if a == addr then v else mem a

/-- Load a 128/256/512-bit vector from word-addressed memory (with optional EVEX `{1toN}` broadcast). -/
def loadVecMem (mem : BitVec 64 → BitVec 64) (w : VecWidth) (addr : BitVec 64)
    (bcast : Bool) : Vec512 :=
  if bcast then
    (Vec512.broadcast (mem addr)).applyWidth w
  else
    match w with
    | .xmm128 =>
      { l0 := mem addr, l1 := mem (addr + 8#64) }
    | .ymm256 =>
      { l0 := mem addr, l1 := mem (addr + 8#64)
        l2 := mem (addr + 16#64), l3 := mem (addr + 24#64) }
    | .zmm512 =>
      { l0 := mem addr, l1 := mem (addr + 8#64)
        l2 := mem (addr + 16#64), l3 := mem (addr + 24#64)
        l4 := mem (addr + 32#64), l5 := mem (addr + 40#64)
        l6 := mem (addr + 48#64), l7 := mem (addr + 56#64) }

/-- Store a 128/256/512-bit vector to word-addressed memory (respecting EVEX write-mask `{k}`). -/
def storeVecMem (mem : BitVec 64 → BitVec 64) (w : VecWidth) (addr : BitVec 64)
    (v : Vec512) (kmask : BitVec 3) (kVal : BitVec 8) : BitVec 64 → BitVec 64 :=
  let wr (m : BitVec 64 → BitVec 64) (i : Nat) (laneVal : BitVec 64) : BitVec 64 → BitVec 64 :=
    if kmask == 0#3 || Vec512.maskBit kVal i then
      write64 m (addr + BitVec.ofNat 64 (i * 8)) laneVal
    else m
  match w with
  | .xmm128 => wr (wr mem 0 v.l0) 1 v.l1
  | .ymm256 => wr (wr (wr (wr mem 0 v.l0) 1 v.l1) 2 v.l2) 3 v.l3
  | .zmm512 =>
    wr (wr (wr (wr (wr (wr (wr (wr mem 0 v.l0) 1 v.l1) 2 v.l2) 3 v.l3) 4 v.l4) 5 v.l5) 6 v.l6) 7 v.l7

/-- Compute effective 64-bit address of a non-RIP memory operand. -/
def evalMemAddr (gpr : BitVec 4 → BitVec 64) : MemAddr → Option (BitVec 64)
  | .ripRel _ => none
  | .baseIdxDisp baseOpt idxOpt disp =>
    let b := match baseOpt with | some r => gpr r | none => 0#64
    let i := match idxOpt with
      | some (r, sc) => gpr r * BitVec.ofNat 64 sc
      | none => 0#64
    let d := BitVec.ofInt 64 disp
    some (b + i + d)

/-- Read a 64-bit word from either RIP-relative `.rodata` or general memory. -/
def readMem64 (rodata : Nat → Option (BitVec 64)) (pc : Nat)
    (gpr : BitVec 4 → BitVec 64) (mem : BitVec 64 → BitVec 64) (addr : MemAddr) :
    Option (BitVec 64) :=
  match addr with
  | .ripRel _ => rodata pc
  | _ => (evalMemAddr gpr addr).map mem

/-- Evaluate a vector `r/m` source operand (register or memory, with EVEX `{1toN}` broadcast). -/
def evalVecRm (s : Amd64VecState) (w : VecWidth) (src : RmOp) (bcast : Bool) : Option Vec512 :=
  match src with
  | .reg r => some (s.zmm r)
  | .mem addr => (evalMemAddr s.gpr addr).map (fun ea => loadVecMem s.mem w ea bcast)

/-- Whether an instruction writes upper YMM/ZMM AVX state (dirtying `upperDirty`). -/
def Amd64VecInsn.dirtiesUpper : Amd64VecInsn → Bool
  | .vbroadcastsd isE w _ _ _
  | .vpbroadcastq isE w _ _ _
  | .vmovdquLoad isE w _ _ _
  | .vmovdquStore isE w _ _ _
  | .vmovdquRR isE w _ _ _
  | .vpand isE w _ _ _ _
  | .vsubpd isE w _ _ _ _
  | .vminpd isE w _ _ _ _ => isE || w == .ymm256 || w == .zmm512
  | .vcmppdVex w _ _ _ _ => w == .ymm256 || w == .zmm512
  | .vcmppdEvex _ _ _ _ _ _ => true
  | .vpblendvb w _ _ _ _ => w == .ymm256 || w == .zmm512
  | .vpblendmq _ _ _ _ _ => true
  | .vmovmskpd w _ _ => w == .ymm256 || w == .zmm512
  | .vpmovm2q _ _ _ => true
  | .vpmovq2m _ _ _ => true
  | _ => false

/-- Whether an instruction is a legacy non-VEX SSE instruction. -/
def Amd64VecInsn.isLegacySSE : Amd64VecInsn → Bool
  | .sseMovupsRR _ _
  | .sseMovupsLoad _ _
  | .sseMovupsStore _ _
  | .sseMovsdLoad _ _
  | .sseMovqFromGpr _ _
  | .sseXorpsRR _ _
  | .ssePorRR _ _
  | .sseMinsdRR _ _
  | .sseUcomisdRR _ _ => true
  | _ => false

private def nextTargetPc (nextOff : Nat) (rel : Int) : Option Nat :=
  let t : Int := (nextOff : Int) + rel
  if t >= 0 then some t.toNat else none

/-- Execute one decoded `Amd64VecInsn` in `Amd64VecState`. -/
def execAmd64VecInsn (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String)
    (s : Amd64VecState) (insn : Amd64VecInsn) (nextOff : Nat) : Amd64VecState :=
  let hazard' := s.sseHazard || (insn.isLegacySSE && s.upperDirty)
  let dirty' :=
    match insn with
    | .vzeroupper => false
    | _ => s.upperDirty || insn.dirtiesUpper
  let base : Amd64VecState := { s with pc := nextOff, upperDirty := dirty', sseHazard := hazard' }
  match insn with
  | .nop => base
  | .pushReg r =>
    let sp' := s.gpr 4#4 - 8#64
    { base with gpr := setGpr s.gpr 4#4 sp', mem := write64 s.mem sp' (s.gpr r) }
  | .popReg r =>
    let sp := s.gpr 4#4
    let v := s.mem sp
    { base with gpr := setGpr (setGpr s.gpr 4#4 (sp + 8#64)) r v }
  | .ret => { base with gpr := setGpr s.gpr 4#4 (s.gpr 4#4 + 8#64), status := .returned }
  | .movRR64 dst src => { base with gpr := setGpr s.gpr dst (s.gpr src) }
  | .movMR64 addr src =>
    match evalMemAddr s.gpr addr with
    | some ea => { base with mem := write64 s.mem ea (s.gpr src) }
    | none => { base with status := .trapDecode s.pc }
  | .movRM64 dst addr =>
    match readMem64 rodata s.pc s.gpr s.mem addr with
    | some v => { base with gpr := setGpr s.gpr dst v }
    | none => { base with status := .trapDecode s.pc }
  | .movImm32 dst imm => { base with gpr := setGpr s.gpr dst (imm.zeroExtend 64) }
  | .movImm64 dst imm => { base with gpr := setGpr s.gpr dst imm }
  | .lea64 dst addr =>
    match evalMemAddr s.gpr addr with
    | some ea => { base with gpr := setGpr s.gpr dst ea }
    | none => { base with status := .trapDecode s.pc }
  | .xorRR32 dst src =>
    let r32 := (s.gpr dst).extractLsb' 0 32 ^^^ (s.gpr src).extractLsb' 0 32
    let r64 := r32.zeroExtend 64
    { base with
      gpr := setGpr s.gpr dst r64
      flags := { zf := r32 == 0#32, cf := false, sf := r32.msb, of := false, pf := x86Parity8 (r32.extractLsb' 0 8) } }
  | .addImm64 dst imm =>
    let lhs := s.gpr dst
    { base with gpr := setGpr s.gpr dst (lhs + imm), flags := x86AddFlags64 lhs imm }
  | .subImm64 dst imm =>
    let lhs := s.gpr dst
    { base with gpr := setGpr s.gpr dst (lhs - imm), flags := x86SubFlags64 lhs imm }
  | .inc64 dst =>
    let lhs := s.gpr dst
    let f := x86AddFlags64 lhs 1#64
    { base with gpr := setGpr s.gpr dst (lhs + 1#64), flags := { f with cf := s.flags.cf } }
  | .testRR64 lhs rhs => { base with flags := x86TestFlags64 (s.gpr lhs) (s.gpr rhs) }
  | .cmpRR64 lhs rhs => { base with flags := x86SubFlags64 (s.gpr lhs) (s.gpr rhs) }
  | .cmpRM64 lhs addr =>
    match readMem64 rodata s.pc s.gpr s.mem addr with
    | some v => { base with flags := x86SubFlags64 (s.gpr lhs) v }
    | none => { base with status := .trapDecode s.pc }
  | .cmpImm64 lhs imm => { base with flags := x86SubFlags64 (s.gpr lhs) imm }
  | .cmpImm8 lhs imm =>
    { base with flags := x86SubFlags8 ((s.gpr lhs).extractLsb' 0 8) imm }
  | .jmpRel rel =>
    match nextTargetPc nextOff rel with
    | some t => { base with pc := t }
    | none => { base with status := .trapDecode s.pc }
  | .jccRel cond rel =>
    if evalX86Cond cond s.flags then
      match nextTargetPc nextOff rel with
      | some t => { base with pc := t }
      | none => { base with status := .trapDecode s.pc }
    else base
  | .callRel _ =>
    match extTargets s.pc with
    | some "runtime.panicBounds" => { base with status := .trapPanicBounds }
    | some "runtime.morestack_noctxt.abi0" => { base with status := .trapMorestack }
    | some tgt => { base with status := .callExternal tgt }
    | none => { base with status := .trapDecode s.pc }
  -- Legacy SSE instructions (operate on low 128 bits, preserve lanes 2..7).
  | .sseMovupsRR dst src =>
    let d5 := dst.zeroExtend 5
    let sv := s.zmm (src.zeroExtend 5)
    let dv := s.zmm d5
    { base with zmm := setZmm s.zmm d5 { dv with l0 := sv.l0, l1 := sv.l1 } }
  | .sseMovupsLoad dst addr =>
    match evalMemAddr s.gpr addr with
    | some ea =>
      let d5 := dst.zeroExtend 5
      let dv := s.zmm d5
      { base with zmm := setZmm s.zmm d5 { dv with l0 := s.mem ea, l1 := s.mem (ea + 8#64) } }
    | none => { base with status := .trapDecode s.pc }
  | .sseMovupsStore addr src =>
    match evalMemAddr s.gpr addr with
    | some ea =>
      let sv := s.zmm (src.zeroExtend 5)
      { base with mem := write64 (write64 s.mem ea sv.l0) (ea + 8#64) sv.l1 }
    | none => { base with status := .trapDecode s.pc }
  | .sseMovsdLoad dst addr =>
    match readMem64 rodata s.pc s.gpr s.mem addr with
    | some v =>
      let d5 := dst.zeroExtend 5
      let dv := s.zmm d5
      { base with zmm := setZmm s.zmm d5 { dv with l0 := v, l1 := 0#64 } }
    | none => { base with status := .trapDecode s.pc }
  | .sseMovqFromGpr dst src =>
    let d5 := dst.zeroExtend 5
    let dv := s.zmm d5
    { base with zmm := setZmm s.zmm d5 { dv with l0 := s.gpr src, l1 := 0#64 } }
  | .sseXorpsRR dst src =>
    let d5 := dst.zeroExtend 5
    let dv := s.zmm d5
    let sv := s.zmm (src.zeroExtend 5)
    { base with zmm := setZmm s.zmm d5 { dv with l0 := dv.l0 ^^^ sv.l0, l1 := dv.l1 ^^^ sv.l1 } }
  | .ssePorRR dst src =>
    let d5 := dst.zeroExtend 5
    let dv := s.zmm d5
    let sv := s.zmm (src.zeroExtend 5)
    { base with zmm := setZmm s.zmm d5 { dv with l0 := dv.l0 ||| sv.l0, l1 := dv.l1 ||| sv.l1 } }
  | .sseMinsdRR dst src =>
    let d5 := dst.zeroExtend 5
    let dv := s.zmm d5
    let sv := s.zmm (src.zeroExtend 5)
    { base with zmm := setZmm s.zmm d5 { dv with l0 := vminpdLane dv.l0 sv.l0 } }
  | .sseUcomisdRR lhs rhs =>
    let lv := s.zmm (lhs.zeroExtend 5)
    let rv := s.zmm (rhs.zeroExtend 5)
    { base with flags := x86UcomisdFlags lv.l0 rv.l0 }
  -- VEX & EVEX vector and opmask instructions.
  | .vzeroupper =>
    { base with zmm := fun r => if r.toNat < 16 then (s.zmm r).zeroHi128 else s.zmm r }
  | .vmovsdLoad dst addr =>
    match readMem64 rodata s.pc s.gpr s.mem addr with
    | some v => { base with zmm := setZmm s.zmm dst { l0 := v } }
    | none => { base with status := .trapDecode s.pc }
  | .vmovsdRR dst src1 src2 =>
    let v1 := s.zmm src1
    let v2 := s.zmm src2
    { base with zmm := setZmm s.zmm dst { l0 := v2.l0, l1 := v1.l1 } }
  | .vmovqFromGpr dst src =>
    { base with zmm := setZmm s.zmm dst { l0 := s.gpr src } }
  | .vbroadcastsd _ w dst src mask =>
    let scalarOpt : Option (BitVec 64) :=
      match src with
      | .reg r => some (s.zmm r).l0
      | .mem addr => readMem64 rodata s.pc s.gpr s.mem addr
    match scalarOpt with
    | some x =>
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) (Vec512.broadcast x)).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vpbroadcastq _ w dst src mask =>
    let scalarOpt : Option (BitVec 64) :=
      match src with
      | .reg r => some (s.zmm r).l0
      | .mem addr => readMem64 rodata s.pc s.gpr s.mem addr
    match scalarOpt with
    | some x =>
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) (Vec512.broadcast x)).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vmovdquLoad _ w dst addr mask =>
    match evalMemAddr s.gpr addr with
    | some ea =>
      let loaded := loadVecMem s.mem w ea false
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) loaded).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vmovdquStore _ w addr src mask =>
    match evalMemAddr s.gpr addr with
    | some ea =>
      { base with mem := storeVecMem s.mem w ea (s.zmm src) mask.kmask (s.kmask mask.kmask) }
    | none => { base with status := .trapDecode s.pc }
  | .vmovdquRR _ w dst src mask =>
    let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) (s.zmm src)).applyWidth w
    { base with zmm := setZmm s.zmm dst res }
  | .vpand _ w dst src1 src2 mask =>
    match evalVecRm s w src2 mask.broadcast with
    | some v2 =>
      let raw := Vec512.zipWith (· &&& ·) (s.zmm src1) v2
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) raw).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vsubpd _ w dst src1 src2 mask =>
    match evalVecRm s w src2 mask.broadcast with
    | some v2 =>
      let raw := Vec512.zipWith x86F64Sub (s.zmm src1) v2
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) raw).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vminpd _ w dst src1 src2 mask =>
    match evalVecRm s w src2 mask.broadcast with
    | some v2 =>
      let raw := Vec512.zipWith vminpdLane (s.zmm src1) v2
      let res := (Vec512.applyEvexMask mask.kmask (s.kmask mask.kmask) mask.zeroing (s.zmm dst) raw).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vcmppdVex w dst src1 src2 imm8 =>
    match evalVecRm s w src2 false with
    | some v2 =>
      let cmpLane (a b : BitVec 64) : BitVec 64 :=
        if x86VcmppdPred imm8 a b then 0xffffffffffffffff#64 else 0#64
      let res := (Vec512.zipWith cmpLane (s.zmm src1) v2).applyWidth w
      { base with zmm := setZmm s.zmm dst res }
    | none => { base with status := .trapDecode s.pc }
  | .vcmppdEvex w dstK src1 src2 imm8 mask =>
    match evalVecRm s w src2 mask.broadcast with
    | some v2 =>
      let v1 := s.zmm src1
      let active (i : Nat) : Bool :=
        (match w with | .xmm128 => i < 2 | .ymm256 => i < 4 | .zmm512 => i < 8) &&
        (mask.kmask == 0#3 || Vec512.maskBit (s.kmask mask.kmask) i)
      let bit (i : Nat) (a b : BitVec 64) : Nat :=
        if active i && x86VcmppdPred imm8 a b then 1 <<< i else 0
      let resK : BitVec 8 := BitVec.ofNat 8
        (bit 0 v1.l0 v2.l0 ||| bit 1 v1.l1 v2.l1 ||| bit 2 v1.l2 v2.l2 ||| bit 3 v1.l3 v2.l3 |||
         bit 4 v1.l4 v2.l4 ||| bit 5 v1.l5 v2.l5 ||| bit 6 v1.l6 v2.l6 ||| bit 7 v1.l7 v2.l7)
      { base with kmask := setKmask s.kmask dstK resK }
    | none => { base with status := .trapDecode s.pc }
  | .vpblendvb w dst src1 src2 maskReg =>
    match evalVecRm s w src2 false with
    | some v2 =>
      let v1 := s.zmm src1
      let vm := s.zmm maskReg
      let raw : Vec512 :=
        { l0 := vpblendvb64 v1.l0 v2.l0 vm.l0, l1 := vpblendvb64 v1.l1 v2.l1 vm.l1
          l2 := vpblendvb64 v1.l2 v2.l2 vm.l2, l3 := vpblendvb64 v1.l3 v2.l3 vm.l3
          l4 := vpblendvb64 v1.l4 v2.l4 vm.l4, l5 := vpblendvb64 v1.l5 v2.l5 vm.l5
          l6 := vpblendvb64 v1.l6 v2.l6 vm.l6, l7 := vpblendvb64 v1.l7 v2.l7 vm.l7 }
      { base with zmm := setZmm s.zmm dst (raw.applyWidth w) }
    | none => { base with status := .trapDecode s.pc }
  | .vpblendmq w dst src1 src2 mask =>
    match evalVecRm s w src2 mask.broadcast with
    | some v2 =>
      let v1 := s.zmm src1
      let kVal := s.kmask mask.kmask
      let pick (i : Nat) (a b : BitVec 64) : BitVec 64 :=
        if mask.kmask == 0#3 || Vec512.maskBit kVal i then b
        else if mask.zeroing then 0#64 else a
      let raw : Vec512 :=
        { l0 := pick 0 v1.l0 v2.l0, l1 := pick 1 v1.l1 v2.l1
          l2 := pick 2 v1.l2 v2.l2, l3 := pick 3 v1.l3 v2.l3
          l4 := pick 4 v1.l4 v2.l4, l5 := pick 5 v1.l5 v2.l5
          l6 := pick 6 v1.l6 v2.l6, l7 := pick 7 v1.l7 v2.l7 }
      { base with zmm := setZmm s.zmm dst (raw.applyWidth w) }
    | none => { base with status := .trapDecode s.pc }
  | .vmovmskpd w dst src =>
    let m := match w with | .xmm128 => (s.zmm src).movmskpd2 | _ => (s.zmm src).movmskpd4
    { base with gpr := setGpr s.gpr dst m }
  | .vpmovm2q w dst srcK =>
    { base with zmm := setZmm s.zmm dst ((Vec512.ofOpmask8 (s.kmask srcK)).applyWidth w) }
  | .vpmovq2m w dstK src =>
    let raw := (s.zmm src).toOpmask8
    let masked := match w with | .xmm128 => raw &&& 0x03#8 | .ymm256 => raw &&& 0x0f#8 | .zmm512 => raw
    { base with kmask := setKmask s.kmask dstK masked }
  | .kmovbToGpr dst srcK =>
    { base with gpr := setGpr s.gpr dst ((s.kmask srcK).zeroExtend 64) }

/-- Single-step operational semantics directly fetching & decoding from raw ELF `.text` bytes. -/
def stepAmd64Vec (bytes : Array UInt8) (rodata : Nat → Option (BitVec 64))
    (extTargets : Nat → Option String) (s : Amd64VecState) : Amd64VecState :=
  if s.status != .running then s
  else
    match decodeAmd64VecInsn bytes s.pc with
    | none => { s with status := .trapDecode s.pc }
    | some (insn, nextOff) => execAmd64VecInsn rodata extTargets s insn nextOff

/-- Fuel-bounded operational execution over raw amd64 ELF bytes. -/
def runAmd64Vec : Nat → Array UInt8 → (Nat → Option (BitVec 64)) → (Nat → Option String) →
    Amd64VecState → Amd64VecState
  | 0, _, _, _, s => s
  | fuel + 1, bytes, rodata, ext, s =>
    if s.status == .running then
      runAmd64Vec fuel bytes rodata ext (stepAmd64Vec bytes rodata ext s)
    else s

theorem runAmd64Vec_add (k1 k2 : Nat) (bytes : Array UInt8)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String) (s : Amd64VecState) :
    runAmd64Vec (k1 + k2) bytes rodata ext s =
      runAmd64Vec k2 bytes rodata ext (runAmd64Vec k1 bytes rodata ext s) := by
  induction k1 generalizing s with
  | zero => simp [runAmd64Vec]
  | succ k ih =>
    rw [Nat.succ_add]
    by_cases hR : (s.status == .running) = true
    · conv => lhs; unfold runAmd64Vec
      conv => rhs; arg 5; unfold runAmd64Vec
      simp only [hR, ↓reduceIte]
      exact ih (stepAmd64Vec bytes rodata ext s)
    · conv => lhs; unfold runAmd64Vec
      conv => rhs; arg 5; unfold runAmd64Vec
      simp only [hR, Bool.false_eq_true, ↓reduceIte]
      cases k2 with
      | zero => rfl
      | succ k2' => simp [runAmd64Vec, hR]

theorem runAmd64Vec_of_not_running (fuel : Nat) (bytes : Array UInt8)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String) (s : Amd64VecState)
    (h : s.status ≠ .running) :
    runAmd64Vec fuel bytes rodata ext s = s := by
  cases fuel with
  | zero => rfl
  | succ k =>
    have hF : (s.status == .running) = false := beq_false_of_ne h
    simp [runAmd64Vec, hF]

theorem stepAmd64Vec_at (bytes : Array UInt8) (rodata : Nat → Option (BitVec 64))
    (ext : Nat → Option String) (s : Amd64VecState) (insn : Amd64VecInsn) (nextOff : Nat)
    (hRun : s.status = .running)
    (hDec : decodeAmd64VecInsn bytes s.pc = some (insn, nextOff)) :
    stepAmd64Vec bytes rodata ext s = execAmd64VecInsn rodata ext s insn nextOff := by
  simp [stepAmd64Vec, hRun, hDec]

/-!
### Part IV: Decode Completeness & Decoded-Machine `VZEROUPPER` / Zero-GC ABI Safety
-/

/-- Decode completeness certificate for an amd64 ELF byte array. -/
def programDecodesCleanly (bytes : Array UInt8) (expectedInsns : Nat) : Bool :=
  match decodeAmd64Program bytes with
  | none => false
  | some insns => insns.size == expectedInsns

private def programDecodesCleanlyChunks (chunks : List (List UInt8)) (expectedInsns : Nat) : Bool :=
  match decodeAmd64ProgramChunks chunks with
  | none => false
  | some insns => insns.size == expectedInsns

private theorem programDecodesCleanly_of_chunks (chunks : List (List UInt8))
    (expectedInsns expectedBytes : Nat)
    (hWf : chunks16WellFormed chunks = true)
    (hDec : programDecodesCleanlyChunks chunks expectedInsns = true)
    (hLen : chunksTotalLen chunks = expectedBytes) :
    programDecodesCleanly ⟨chunks.flatten⟩ expectedInsns = true ∧
      (⟨chunks.flatten⟩ : Array UInt8).size = expectedBytes := by
  refine ⟨?_, ?_⟩
  · unfold programDecodesCleanly
    rw [decodeAmd64ProgramChunks_eq chunks hWf]
    exact hDec
  · simpa [chunksTotalLen_eq_flatten] using hLen

theorem allAbsLE64AVX2_decodes :
    programDecodesCleanly allAbsLE64AVX2Bytes 53 = true ∧ allAbsLE64AVX2Bytes.size = 180 :=
  programDecodesCleanly_of_chunks allAbsLE64AVX2ByteChunks 53 180 (by decide) (by decide) (by decide)

theorem allAbsLE64AVX512_decodes :
    programDecodesCleanly allAbsLE64AVX512Bytes 56 = true ∧ allAbsLE64AVX512Bytes.size = 220 :=
  programDecodesCleanly_of_chunks allAbsLE64AVX512ByteChunks 56 220 (by decide) (by decide) (by decide)

set_option maxRecDepth 4096 in
theorem minNoNaN64AVX2_decodes :
    programDecodesCleanly minNoNaN64AVX2Bytes 81 = true ∧ minNoNaN64AVX2Bytes.size = 284 :=
  programDecodesCleanly_of_chunks minNoNaN64AVX2ByteChunks 81 284 (by decide) (by decide) (by decide)

set_option maxRecDepth 4096 in
theorem minNoNaN64AVX512_decodes :
    programDecodesCleanly minNoNaN64AVX512Bytes 97 = true ∧ minNoNaN64AVX512Bytes.size = 389 :=
  programDecodesCleanly_of_chunks minNoNaN64AVX512ByteChunks 97 389 (by decide) (by decide) (by decide)

set_option maxRecDepth 4096 in
theorem initReducedRow64AVX2_decodes :
    programDecodesCleanly initReducedRow64AVX2Bytes 107 = true ∧ initReducedRow64AVX2Bytes.size = 414 :=
  programDecodesCleanly_of_chunks initReducedRow64AVX2ByteChunks 107 414 (by decide) (by decide) (by decide)

set_option maxRecDepth 4096 in
theorem initReducedRow64AVX512_decodes :
    programDecodesCleanly initReducedRow64AVX512Bytes 136 = true ∧ initReducedRow64AVX512Bytes.size = 587 :=
  programDecodesCleanly_of_chunks initReducedRow64AVX512ByteChunks 136 587 (by decide) (by decide) (by decide)

/-- EVEX `{1to8}` broadcast (`b=1` in EVEX P2 byte) and `{k1}{z}` zero-masking decoder witness:
`62 f1 d5 d9 5c 28` = `vsubpd (%rax){1to8}, %zmm5, %zmm5{%k1}{z}`. -/
theorem evex_broadcast_and_zeromask_decode_witness :
    decodeAmd64VecInsn #[0x62, 0xf1, 0xd5, 0xd9, 0x5c, 0x28] 0 =
      some (.vsubpd true .zmm512 5#5 5#5 (.mem (.baseIdxDisp (some 0#4) none 0))
            { kmask := 1#3, zeroing := true, broadcast := true }, 6) := by
  decide

/-- Structural lookup of the instruction index whose byte offset equals `off`. -/
private def findInsnIdxByOffAux (insns : Array DecodedAmd64Insn) (off : Nat) :
    Nat → Nat → Option Nat
  | 0, _ => none
  | fuel + 1, i =>
    if h : i < insns.size then
      if insns[i].offset == off then some i else findInsnIdxByOffAux insns off fuel (i + 1)
    else none

private def findInsnIdxByOff (insns : Array DecodedAmd64Insn) (off : Nat) : Option Nat :=
  findInsnIdxByOffAux insns off insns.size 0

private theorem findInsnIdxByOff_sound (insns : Array DecodedAmd64Insn) (off idx : Nat)
    (h : findInsnIdxByOff insns off = some idx) :
    ∃ (hLt : idx < insns.size), insns[idx].offset = off := by
  unfold findInsnIdxByOff at h
  suffices ∀ fuel start, findInsnIdxByOffAux insns off fuel start = some idx →
      ∃ (hLt : idx < insns.size), insns[idx].offset = off from this insns.size 0 h
  intro fuel
  induction fuel with
  | zero => intro _ h0; cases h0
  | succ k ih =>
    intro start hGo
    unfold findInsnIdxByOffAux at hGo
    split at hGo
    · next hLt =>
      by_cases hEq : (insns[start].offset == off) = true
      · simp only [hEq, ↓reduceIte] at hGo
        cases hGo
        exact ⟨hLt, beq_iff_eq.mp hEq⟩
      · simp only [hEq, Bool.false_eq_true, ↓reduceIte] at hGo
        exact ih (start + 1) hGo
    · cases hGo

/-- Check that every memory operand of `insn` resolves without `.trapDecode`. -/
private def insnMemResolves (rodata : Nat → Option (BitVec 64)) (off : Nat) : Amd64VecInsn → Bool
  | .movMR64 (.ripRel _) _ | .lea64 _ (.ripRel _)
  | .sseMovupsLoad _ (.ripRel _) | .sseMovupsStore (.ripRel _) _
  | .vmovdquLoad _ _ _ (.ripRel _) _ | .vmovdquStore _ _ (.ripRel _) _ _
  | .vpand _ _ _ _ (.mem (.ripRel _)) _ | .vsubpd _ _ _ _ (.mem (.ripRel _)) _
  | .vminpd _ _ _ _ (.mem (.ripRel _)) _ | .vcmppdVex _ _ _ (.mem (.ripRel _)) _
  | .vcmppdEvex _ _ _ (.mem (.ripRel _)) _ _ | .vpblendvb _ _ _ (.mem (.ripRel _)) _
  | .vpblendmq _ _ _ (.mem (.ripRel _)) _ => false
  | .movRM64 _ (.ripRel _) | .cmpRM64 _ (.ripRel _)
  | .sseMovsdLoad _ (.ripRel _) | .vmovsdLoad _ (.ripRel _)
  | .vbroadcastsd _ _ _ (.mem (.ripRel _)) _ | .vpbroadcastq _ _ _ (.mem (.ripRel _)) _ =>
    (rodata off).isSome
  | _ => true

/-- Allowed Go runtime traps (zero heap allocation / zero GC write barriers). -/
private def isAllowedRuntimeTrap (sym : String) : Bool :=
  sym == "runtime.panicBounds" || sym == "runtime.morestack_noctxt.abi0"

/-- Next `upperDirty` state after executing `insn`. -/
@[inline] private def nextUpperDirty (dirty : Bool) (insn : Amd64VecInsn) : Bool :=
  match insn with
  | .vzeroupper => false
  | _ => dirty || insn.dirtiesUpper

/-- Validate one instruction `insns[idx]` at `(idx, dirty)` and return its abstract successor indices. -/
private def checkInsnStep (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String)
    (idx : Nat) (dirty : Bool) : Option (List (Nat × Bool)) :=
  if h : idx < insns.size then
    if !insnMemResolves rodata insns[idx].offset insns[idx].insn then none
    else if insns[idx].insn.isLegacySSE && dirty then none
    else
      match insns[idx].insn with
      | .ret => if nextUpperDirty dirty insns[idx].insn then none else some []
      | .callRel _ =>
        match extTargets insns[idx].offset with
        | some sym => if isAllowedRuntimeTrap sym then some [] else none
        | none => none
      | .jmpRel rel =>
        match nextTargetPc insns[idx].nextOffset rel with
        | none => none
        | some tgtOff =>
          match findInsnIdxByOff insns tgtOff with
          | none => none
          | some tIdx => some [(tIdx, nextUpperDirty dirty insns[idx].insn)]
      | .jccRel _ rel =>
        match nextTargetPc insns[idx].nextOffset rel with
        | none => none
        | some tgtOff =>
          match findInsnIdxByOff insns tgtOff, findInsnIdxByOff insns insns[idx].nextOffset with
          | some tIdx, some fIdx =>
            some [(tIdx, nextUpperDirty dirty insns[idx].insn),
                  (fIdx, nextUpperDirty dirty insns[idx].insn)]
          | _, _ => none
      | _ =>
        match findInsnIdxByOff insns insns[idx].nextOffset with
        | some fIdx => some [(fIdx, nextUpperDirty dirty insns[idx].insn)]
        | none => none
  else none

private def abiStateKey (idx : Nat) (dirty : Bool) : Nat :=
  idx * 2 + if dirty then 1 else 0

private def pushAbiSuccs (succs : List (Nat × Bool)) (seen : Array Bool)
    (q : List (Nat × Bool)) : Array Bool × List (Nat × Bool) :=
  succs.foldl (fun acc st =>
    let k := abiStateKey st.1 st.2
    if hk : k < acc.1.size then
      if acc.1[k] then acc else (acc.1.set k true hk, st :: acc.2)
    else acc) (seen, q)

private def abiBfs (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String) :
    Nat → Array Bool → List (Nat × Bool) → Option (Array Bool)
  | 0, seen, [] => some seen
  | 0, _, _ :: _ => none
  | _ + 1, seen, [] => some seen
  | fuel + 1, seen, (idx, dirty) :: rest =>
    match checkInsnStep insns rodata extTargets idx dirty with
    | none => none
    | some succs =>
      let (seen', q') := pushAbiSuccs succs seen rest
      abiBfs insns rodata extTargets fuel seen' q'

/-- Executable ABI & control-flow safety checker directly over raw amd64 ELF bytes:
verifies complete decoding, valid internal jump boundaries, zero heap/GC external calls,
zero AVX-SSE transition penalties (`sseHazard == false`), and `upperDirty == false`
at every reachable `RET` (`VZEROUPPER` ABI safety). -/
def checkDecodedAbiSafety (bytes : Array UInt8)
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String) : Bool :=
  match decodeAmd64Program bytes with
  | none => false
  | some insns =>
    if hPos : 0 < insns.size then
      if insns[0].offset != 0 then false
      else
        let n := insns.size
        let seen0 := (Array.replicate (2 * n) false).set 0 true (by simp; omega)
        (abiBfs insns rodata extTargets (4 * n + 2) seen0 [(0, false)]).isSome
    else false

/-! #### Soundness Proof of `checkDecodedAbiSafety` Over `runAmd64Vec` -/

private theorem decodeAmd64ProgramAux_sound (getB : Nat → Option UInt8) (byteLen fuel off : Nat)
    (acc : List DecodedAmd64Insn) (insns : Array DecodedAmd64Insn)
    (hAcc : ∀ d ∈ acc, decodeAmd64VecInsnWith getB d.offset = some (d.insn, d.nextOffset))
    (hEq : decodeAmd64ProgramAux getB byteLen fuel off acc = some insns) :
    ∀ d ∈ insns.toList, decodeAmd64VecInsnWith getB d.offset = some (d.insn, d.nextOffset) := by
  induction fuel generalizing off acc with
  | zero =>
    unfold decodeAmd64ProgramAux at hEq
    split at hEq
    · cases hEq
      intro d hd
      have hMem : d ∈ acc := by simpa using hd
      exact hAcc d hMem
    · cases hEq
  | succ k ih =>
    unfold decodeAmd64ProgramAux at hEq
    split at hEq
    · cases hEq
      intro d hd
      have hMem : d ∈ acc := by simpa using hd
      exact hAcc d hMem
    · split at hEq
      · cases hEq
      · cases hDec : decodeAmd64VecInsnWith getB off with
        | none => simp [hDec] at hEq
        | some pair =>
          rcases pair with ⟨insn, nextOff⟩
          simp only [hDec] at hEq
          split at hEq
          · cases hEq
          · refine ih nextOff ({ offset := off, nextOffset := nextOff, insn := insn } :: acc) ?_ hEq
            intro d hd
            simp only [List.mem_cons] at hd
            rcases hd with rfl | hIn
            · exact hDec
            · exact hAcc d hIn

private theorem decodeAmd64Program_elem_decodes (bytes : Array UInt8) (insns : Array DecodedAmd64Insn)
    (hProg : decodeAmd64Program bytes = some insns) (idx : Nat) (hLt : idx < insns.size) :
    decodeAmd64VecInsn bytes insns[idx].offset = some (insns[idx].insn, insns[idx].nextOffset) := by
  have hAll := decodeAmd64ProgramAux_sound (fun i => bytes[i]?) bytes.size (bytes.size + 1) 0 [] insns
    (fun _ h => by cases h) hProg
  exact hAll insns[idx] (by simp)

private theorem abiStateKey_lt {n idx : Nat} (dirty : Bool) (h : idx < n) :
    abiStateKey idx dirty < 2 * n := by
  unfold abiStateKey; cases dirty <;> simp <;> omega

private theorem abiStateKey_inj {i1 i2 : Nat} {d1 d2 : Bool}
    (h : abiStateKey i1 d1 = abiStateKey i2 d2) : i1 = i2 ∧ d1 = d2 := by
  unfold abiStateKey at h
  cases d1 <;> cases d2 <;> simp at h
  · exact ⟨by omega, rfl⟩
  · omega
  · omega
  · exact ⟨by omega, rfl⟩

private theorem array_bool_getElem?_set (a : Array Bool) (i j : Nat) (v : Bool) (hi : i < a.size) :
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

private theorem checkInsnStep_succ_lt (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (idx : Nat) (dirty : Bool) (succs : List (Nat × Bool))
    (hOk : checkInsnStep insns rodata ext idx dirty = some succs) :
    ∀ i' d', (i', d') ∈ succs → i' < insns.size := by
  unfold checkInsnStep at hOk
  split at hOk
  · split at hOk
    · cases hOk
    · split at hOk
      · cases hOk
      · cases hI : insns[idx].insn <;> simp only [hI] at hOk
        case ret =>
          split at hOk <;> cases hOk
          intro _ _ hm; cases hm
        case jmpRel rel =>
          cases hT : nextTargetPc insns[idx].nextOffset rel with
          | none => simp [hT] at hOk
          | some tgtOff =>
            simp only [hT] at hOk
            cases hF : findInsnIdxByOff insns tgtOff with
            | none => simp [hF] at hOk
            | some tIdx =>
              simp only [hF] at hOk; cases hOk
              intro i' d' hm; simp only [List.mem_singleton, Prod.mk.injEq] at hm
              rcases hm with ⟨rfl, _⟩
              exact (findInsnIdxByOff_sound insns _ _ hF).1
        case jccRel cond rel =>
          cases hT : nextTargetPc insns[idx].nextOffset rel with
          | none => simp [hT] at hOk
          | some tgtOff =>
            simp only [hT] at hOk
            cases hF1 : findInsnIdxByOff insns tgtOff with
            | none => simp [hF1] at hOk
            | some tIdx =>
              cases hF2 : findInsnIdxByOff insns insns[idx].nextOffset with
              | none => simp [hF1, hF2] at hOk
              | some fIdx =>
                simp only [hF1, hF2] at hOk; cases hOk
                intro i' d' hm
                simp only [List.mem_cons, Prod.mk.injEq] at hm
                rcases hm with ⟨rfl, _⟩ | ⟨rfl, _⟩ | hNil
                · exact (findInsnIdxByOff_sound insns _ _ hF1).1
                · exact (findInsnIdxByOff_sound insns _ _ hF2).1
                · cases hNil
        case callRel rel =>
          cases hE : ext insns[idx].offset with
          | none => simp [hE] at hOk
          | some sym =>
            simp only [hE] at hOk
            split at hOk <;> cases hOk
            intro _ _ hm; cases hm
        all_goals
          cases hN : findInsnIdxByOff insns insns[idx].nextOffset with
          | none => simp [hN] at hOk
          | some fIdx =>
            simp only [hN] at hOk; cases hOk
            intro i' d' hm; simp only [List.mem_singleton, Prod.mk.injEq] at hm
            rcases hm with ⟨rfl, _⟩
            exact (findInsnIdxByOff_sound insns _ _ hN).1
  · cases hOk

private theorem pushAbiSuccs_props (n : Nat) (succs : List (Nat × Bool))
    (seen : Array Bool) (q : List (Nat × Bool))
    (hSz : seen.size = 2 * n)
    (hQ : ∀ idx dirty, (idx, dirty) ∈ q → idx < n ∧ seen[abiStateKey idx dirty]? = some true)
    (hSuccs : ∀ i' d', (i', d') ∈ succs → i' < n) :
    let res := pushAbiSuccs succs seen q
    res.1.size = 2 * n ∧
    (∀ idx dirty, (idx, dirty) ∈ res.2 → idx < n ∧ res.1[abiStateKey idx dirty]? = some true) ∧
    (∀ st ∈ q, st ∈ res.2) ∧
    (∀ k : Nat, seen[k]? = some true → res.1[k]? = some true) ∧
    (∀ i' d', (i', d') ∈ succs → res.1[abiStateKey i' d']? = some true) ∧
    (∀ idx dirty, idx < n → res.1[abiStateKey idx dirty]? = some true →
      seen[abiStateKey idx dirty]? = some true ∨ (idx, dirty) ∈ res.2) := by
  induction succs generalizing seen q with
  | nil =>
    dsimp only [pushAbiSuccs]
    refine ⟨hSz, hQ, fun _ h => h, fun _ h => h, ?_, fun _ _ _ h => Or.inl h⟩
    intro _ _ h; cases h
  | cons hd tl ih =>
    simp only [pushAbiSuccs, List.foldl_cons]
    have hHdLt : hd.1 < n := hSuccs hd.1 hd.2 (List.Mem.head tl)
    have hKlt : abiStateKey hd.1 hd.2 < seen.size := by rw [hSz]; exact abiStateKey_lt hd.2 hHdLt
    have hTlSuccs : ∀ i' d', (i', d') ∈ tl → i' < n := fun i' d' h => hSuccs i' d' (List.Mem.tail hd h)
    by_cases hAlready : seen[abiStateKey hd.1 hd.2]
    · simp only [hKlt, ↓reduceDIte, hAlready, ↓reduceIte]
      have res := ih seen q hSz hQ hTlSuccs
      refine ⟨res.1, res.2.1, res.2.2.1, res.2.2.2.1, ?_, res.2.2.2.2.2⟩
      intro i' d' hMem
      cases hMem with
      | head => apply res.2.2.2.1; rw [Array.getElem?_eq_getElem hKlt, hAlready]
      | tail _ hTl => exact res.2.2.2.2.1 i' d' hTl
    · simp only [hKlt, ↓reduceDIte, hAlready, Bool.false_eq_true, ↓reduceIte]
      let seen1 := seen.set (abiStateKey hd.1 hd.2) true hKlt
      let q1 := hd :: q
      have hSz1 : seen1.size = 2 * n := by simp [seen1, hSz]
      have hQ1 : ∀ idx dirty, (idx, dirty) ∈ q1 → idx < n ∧ seen1[abiStateKey idx dirty]? = some true := by
        intro idx dirty h
        dsimp only [q1] at h
        cases h with
        | head => refine ⟨hHdLt, ?_⟩; simp [seen1]
        | tail _ hIn =>
          rcases hQ idx dirty hIn with ⟨hi, hv⟩
          refine ⟨hi, ?_⟩; simp only [seen1, array_bool_getElem?_set]; split <;> simp [hv]
      have res := ih seen1 q1 hSz1 hQ1 hTlSuccs
      refine ⟨res.1, res.2.1, fun st hSt => res.2.2.1 st (List.Mem.tail hd hSt), ?_, ?_, ?_⟩
      · intro k hv; apply res.2.2.2.1; simp only [seen1, array_bool_getElem?_set]; split <;> simp [hv]
      · intro i' d' hMem
        cases hMem with
        | head => apply res.2.2.2.1; simp [seen1]
        | tail _ hTl => exact res.2.2.2.2.1 i' d' hTl
      · intro idx dirty hi hv'
        rcases res.2.2.2.2.2 idx dirty hi hv' with hv1 | hInQ'
        · simp only [seen1, array_bool_getElem?_set] at hv1
          split at hv1
          · next hEqK =>
            have hp := abiStateKey_inj hEqK
            have hPair : (idx, dirty) = hd := Prod.ext hp.1.symm hp.2.symm
            rw [hPair]; exact Or.inr (res.2.2.1 hd (List.Mem.head q))
          · exact Or.inl hv1
        · exact Or.inr hInQ'

private def AbiBfsInv (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (seen : Array Bool) (q : List (Nat × Bool)) : Prop :=
  seen.size = 2 * insns.size ∧
  (∀ idx dirty, (idx, dirty) ∈ q → idx < insns.size ∧ seen[abiStateKey idx dirty]? = some true) ∧
  (0 < insns.size → seen[0]? = some true) ∧
  (∀ idx dirty, idx < insns.size → seen[abiStateKey idx dirty]? = some true →
    (idx, dirty) ∈ q ∨
    ∃ succs, checkInsnStep insns rodata ext idx dirty = some succs ∧
      ∀ i' d', (i', d') ∈ succs → seen[abiStateKey i' d']? = some true)

private theorem abiBfs_inv (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (fuel : Nat) (seen : Array Bool) (q : List (Nat × Bool)) (finalSeen : Array Bool)
    (hInv : AbiBfsInv insns rodata ext seen q)
    (hEq : abiBfs insns rodata ext fuel seen q = some finalSeen) :
    AbiBfsInv insns rodata ext finalSeen [] := by
  induction fuel generalizing seen q with
  | zero =>
    cases q with
    | nil => simp [abiBfs] at hEq; subst hEq; exact hInv
    | cons _ _ => simp [abiBfs] at hEq
  | succ k ih =>
    cases q with
    | nil => simp [abiBfs] at hEq; subst hEq; exact hInv
    | cons st rest =>
      rcases st with ⟨idx, dirty⟩
      simp only [abiBfs] at hEq
      cases hStep : checkInsnStep insns rodata ext idx dirty with
      | none => simp [hStep] at hEq
      | some succs =>
        simp only [hStep] at hEq
        have hQRest : ∀ i d, (i, d) ∈ rest → i < insns.size ∧ seen[abiStateKey i d]? = some true :=
          fun i d h => hInv.2.1 i d (List.Mem.tail (idx, dirty) h)
        have hSLt := checkInsnStep_succ_lt insns rodata ext idx dirty succs hStep
        have hp := pushAbiSuccs_props insns.size succs seen rest hInv.1 hQRest hSLt
        apply ih _ _ ?_ hEq
        refine ⟨hp.1, hp.2.1, fun hPos => hp.2.2.2.1 0 (hInv.2.2.1 hPos), ?_⟩
        intro i d hi hv'
        rcases hp.2.2.2.2.2 i d hi hv' with hv | hInQ'
        · rcases hInv.2.2.2 i d hi hv with hInQ | ⟨sc, hSc, hAll⟩
          · cases hInQ with
            | head => exact Or.inr ⟨succs, hStep, hp.2.2.2.2.1⟩
            | tail _ hInR => exact Or.inl (hp.2.2.1 (i, d) hInR)
          · exact Or.inr ⟨sc, hSc, fun i' d' hM => hp.2.2.2.1 (abiStateKey i' d') (hAll i' d' hM)⟩
        · exact Or.inl hInQ'

private theorem execAmd64VecInsn_abi_step (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (idx : Nat) (hIdx : idx < insns.size) (s : Amd64VecState)
    (hPc : s.pc = insns[idx].offset) (hRun : s.status = .running) (hHaz : s.sseHazard = false)
    (succs : List (Nat × Bool))
    (hCheck : checkInsnStep insns rodata ext idx s.upperDirty = some succs) :
    let s' := execAmd64VecInsn rodata ext s insns[idx].insn insns[idx].nextOffset
    s'.sseHazard = false ∧
    (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧
    (s'.status = .returned → s'.upperDirty = false) ∧
    (s'.status = .running → ∃ i', ∃ (hi' : i' < insns.size),
      s'.pc = insns[i'].offset ∧ (i', s'.upperDirty) ∈ succs) := by
  unfold checkInsnStep at hCheck
  simp only [hIdx, ↓reduceDIte] at hCheck
  split at hCheck
  · cases hCheck
  · next hMemOk =>
    have hMemT : insnMemResolves rodata insns[idx].offset insns[idx].insn = true := by
      simpa using hMemOk
    split at hCheck
    · cases hCheck
    · next hNoSse =>
      have hSseF : (insns[idx].insn.isLegacySSE && s.upperDirty) = false := by
        simpa using hNoSse
      have hHaz' : (s.sseHazard || (insns[idx].insn.isLegacySSE && s.upperDirty)) = false := by
        simp [hHaz, hSseF]
      have hDirtEq : (match insns[idx].insn with
          | .vzeroupper => false | _ => s.upperDirty || insns[idx].insn.dirtiesUpper) =
          nextUpperDirty s.upperDirty insns[idx].insn := rfl
      -- Handle every instruction constructor cleanly.
      cases hI : insns[idx].insn <;> simp only [hI] at hCheck hMemT hSseF hHaz' hDirtEq
      -- Non-branching non-memory instructions (fallthrough to nextOffset).
      case nop | pushReg | popReg | movRR64 | movImm32 | movImm64 | xorRR32 | addImm64
        | subImm64 | inc64 | testRR64 | cmpRR64 | cmpImm64 | cmpImm8 | sseMovupsRR
        | sseMovqFromGpr | sseXorpsRR | ssePorRR | sseMinsdRR | sseUcomisdRR | vzeroupper
        | vmovsdRR | vmovqFromGpr | vmovdquRR | vmovmskpd | vpmovm2q | vpmovq2m | kmovbToGpr =>
        cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
        | none => simp [hF] at hCheck
        | some fIdx =>
          simp only [hF] at hCheck; cases hCheck
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
          refine ⟨by simp [execAmd64VecInsn, hHaz'],
            fun _ => by simp [execAmd64VecInsn, hRun],
            fun _ => by simp [execAmd64VecInsn, hRun],
            fun h => by simp [execAmd64VecInsn, hRun] at h,
            fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, hFOff], by simp [execAmd64VecInsn, nextUpperDirty]⟩⟩
      case ret =>
        split at hCheck
        · cases hCheck
        · next hNd =>
          have hNdF : nextUpperDirty s.upperDirty .ret = false := by simpa using hNd
          cases hCheck
          refine ⟨by simp [execAmd64VecInsn, hHaz'],
            fun _ => by simp [execAmd64VecInsn],
            fun _ => by simp [execAmd64VecInsn],
            fun _ => by simpa [execAmd64VecInsn, nextUpperDirty] using hNdF,
            fun h => by simp [execAmd64VecInsn] at h⟩
      case callRel rel =>
        cases hE : ext insns[idx].offset with
        | none => simp [hE] at hCheck
        | some sym =>
          simp only [hE] at hCheck
          by_cases hAllow : isAllowedRuntimeTrap sym = true
          · simp only [hAllow, ↓reduceIte] at hCheck; cases hCheck
            unfold isAllowedRuntimeTrap at hAllow
            simp only [Bool.or_eq_true, beq_iff_eq] at hAllow
            rcases hAllow with rfl | rfl <;>
              exact ⟨by simp [execAmd64VecInsn, hPc, hE, hHaz'],
                fun _ => by simp [execAmd64VecInsn, hPc, hE],
                fun _ => by simp [execAmd64VecInsn, hPc, hE],
                fun h => by simp [execAmd64VecInsn, hPc, hE] at h,
                fun h => by simp [execAmd64VecInsn, hPc, hE] at h⟩
          · simp [hAllow] at hCheck
      case jmpRel rel =>
        cases hT : nextTargetPc insns[idx].nextOffset rel with
        | none => simp [hT] at hCheck
        | some tgtOff =>
          simp only [hT] at hCheck
          cases hF : findInsnIdxByOff insns tgtOff with
          | none => simp [hF] at hCheck
          | some tIdx =>
            simp only [hF] at hCheck; cases hCheck
            rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hTLt, hTOff⟩
            refine ⟨by simp [execAmd64VecInsn, hT, hHaz'],
              fun _ => by simp [execAmd64VecInsn, hT, hRun],
              fun _ => by simp [execAmd64VecInsn, hT, hRun],
              fun h => by simp [execAmd64VecInsn, hT, hRun] at h,
              fun _ => ⟨tIdx, hTLt, by simp [execAmd64VecInsn, hT, hTOff], by simp [execAmd64VecInsn, hT, nextUpperDirty]⟩⟩
      case jccRel cond rel =>
        cases hT : nextTargetPc insns[idx].nextOffset rel with
        | none => simp [hT] at hCheck
        | some tgtOff =>
          simp only [hT] at hCheck
          cases hF1 : findInsnIdxByOff insns tgtOff with
          | none => simp [hF1] at hCheck
          | some tIdx =>
            cases hF2 : findInsnIdxByOff insns insns[idx].nextOffset with
            | none => simp [hF1, hF2] at hCheck
            | some fIdx =>
              simp only [hF1, hF2] at hCheck; cases hCheck
              rcases findInsnIdxByOff_sound insns _ _ hF1 with ⟨hTLt, hTOff⟩
              rcases findInsnIdxByOff_sound insns _ _ hF2 with ⟨hFLt, hFOff⟩
              by_cases hCond : evalX86Cond cond s.flags = true
              · refine ⟨by simp [execAmd64VecInsn, hCond, hT, hHaz'],
                  fun _ => by simp [execAmd64VecInsn, hCond, hT, hRun],
                  fun _ => by simp [execAmd64VecInsn, hCond, hT, hRun],
                  fun h => by simp [execAmd64VecInsn, hCond, hT, hRun] at h,
                  fun _ => ⟨tIdx, hTLt, by simp [execAmd64VecInsn, hCond, hT, hTOff],
                    by simp [execAmd64VecInsn, hCond, hT, nextUpperDirty]⟩⟩
              · refine ⟨by simp [execAmd64VecInsn, hCond, hHaz'],
                  fun _ => by simp [execAmd64VecInsn, hCond, hRun],
                  fun _ => by simp [execAmd64VecInsn, hCond, hRun],
                  fun h => by simp [execAmd64VecInsn, hCond, hRun] at h,
                  fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, hCond, hFOff],
                    by simp [execAmd64VecInsn, hCond, nextUpperDirty]⟩⟩
      -- Memory non-RIP instructions.
      case movMR64 addr _ | lea64 _ addr | sseMovupsLoad _ addr | sseMovupsStore addr _
        | vmovdquLoad _ _ _ addr _ | vmovdquStore _ _ addr _ _ =>
        cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
        | none => simp [hF] at hCheck
        | some fIdx =>
          simp only [hF] at hCheck; cases hCheck
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
          cases addr with
          | ripRel _ => simp [insnMemResolves] at hMemT
          | baseIdxDisp b i d =>
            refine ⟨by simp [execAmd64VecInsn, evalMemAddr, hHaz'],
              fun _ => by simp [execAmd64VecInsn, evalMemAddr, hRun],
              fun _ => by simp [execAmd64VecInsn, evalMemAddr, hRun],
              fun h => by simp [execAmd64VecInsn, evalMemAddr, hRun] at h,
              fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, evalMemAddr, hFOff],
                by simp [execAmd64VecInsn, evalMemAddr, nextUpperDirty]⟩⟩
      -- Memory instructions supporting RIP-relative rodata.
      case movRM64 _ addr | cmpRM64 _ addr | sseMovsdLoad _ addr | vmovsdLoad _ addr =>
        cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
        | none => simp [hF] at hCheck
        | some fIdx =>
          simp only [hF] at hCheck; cases hCheck
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
          cases addr with
          | ripRel _ =>
            simp only [insnMemResolves] at hMemT
            rcases Option.isSome_iff_exists.mp hMemT with ⟨v, hv⟩
            refine ⟨by simp [execAmd64VecInsn, readMem64, hPc, hv, hHaz'],
              fun _ => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun],
              fun _ => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun],
              fun h => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun] at h,
              fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, readMem64, hPc, hv, hFOff],
                by simp [execAmd64VecInsn, readMem64, hPc, hv, nextUpperDirty]⟩⟩
          | baseIdxDisp b i d =>
            refine ⟨by simp [execAmd64VecInsn, readMem64, evalMemAddr, hHaz'],
              fun _ => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun],
              fun _ => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun],
              fun h => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun] at h,
              fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, readMem64, evalMemAddr, hFOff],
                by simp [execAmd64VecInsn, readMem64, evalMemAddr, nextUpperDirty]⟩⟩
      -- Broadcast instructions (reg or mem/RIP).
      case vbroadcastsd _ _ _ src _ | vpbroadcastq _ _ _ src _ =>
        cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
        | none => simp [hF] at hCheck
        | some fIdx =>
          simp only [hF] at hCheck; cases hCheck
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
          cases src with
          | reg r =>
            refine ⟨by simp [execAmd64VecInsn, hHaz'],
              fun _ => by simp [execAmd64VecInsn, hRun],
              fun _ => by simp [execAmd64VecInsn, hRun],
              fun h => by simp [execAmd64VecInsn, hRun] at h,
              fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, hFOff],
                by simp [execAmd64VecInsn, nextUpperDirty]⟩⟩
          | mem addr =>
            cases addr with
            | ripRel _ =>
              simp only [insnMemResolves] at hMemT
              rcases Option.isSome_iff_exists.mp hMemT with ⟨v, hv⟩
              refine ⟨by simp [execAmd64VecInsn, readMem64, hPc, hv, hHaz'],
                fun _ => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun],
                fun _ => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun],
                fun h => by simp [execAmd64VecInsn, readMem64, hPc, hv, hRun] at h,
                fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, readMem64, hPc, hv, hFOff],
                  by simp [execAmd64VecInsn, readMem64, hPc, hv, nextUpperDirty]⟩⟩
            | baseIdxDisp b i d =>
              refine ⟨by simp [execAmd64VecInsn, readMem64, evalMemAddr, hHaz'],
                fun _ => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun],
                fun _ => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun],
                fun h => by simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun] at h,
                fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, readMem64, evalMemAddr, hFOff],
                  by simp [execAmd64VecInsn, readMem64, evalMemAddr, nextUpperDirty]⟩⟩
      -- Vector RmOp instructions (reg or non-RIP mem).
      case vpand _ _ _ _ src2 _ | vsubpd _ _ _ _ src2 _ | vminpd _ _ _ _ src2 _
        | vcmppdVex _ _ _ src2 _ | vcmppdEvex _ _ _ src2 _ _
        | vpblendvb _ _ _ src2 _ | vpblendmq _ _ _ src2 _ =>
        cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
        | none => simp [hF] at hCheck
        | some fIdx =>
          simp only [hF] at hCheck; cases hCheck
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
          cases src2 with
          | reg r =>
            refine ⟨by simp [execAmd64VecInsn, evalVecRm, hHaz'],
              fun _ => by simp [execAmd64VecInsn, evalVecRm, hRun],
              fun _ => by simp [execAmd64VecInsn, evalVecRm, hRun],
              fun h => by simp [execAmd64VecInsn, evalVecRm, hRun] at h,
              fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, evalVecRm, hFOff],
                by simp [execAmd64VecInsn, evalVecRm, nextUpperDirty]⟩⟩
          | mem addr =>
            cases addr with
            | ripRel _ => simp [insnMemResolves] at hMemT
            | baseIdxDisp b i d =>
              refine ⟨by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, hHaz'],
                fun _ => by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, hRun],
                fun _ => by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, hRun],
                fun h => by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at h,
                fun _ => ⟨fIdx, hFLt, by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, hFOff],
                  by simp [execAmd64VecInsn, evalVecRm, evalMemAddr, nextUpperDirty]⟩⟩

/-- Soundness theorem for `checkDecodedAbiSafety`:
If `checkDecodedAbiSafety bytes rodata extTargets = true`, then for EVERY initial state `s`
at `pc = 0` (`status = .running`, `upperDirty = false`, `sseHazard = false`) and EVERY `fuel`,
executing the raw ELF bytes directly under `runAmd64Vec` guarantees:
1. Zero AVX-SSE transition penalties (`sseHazard = false`),
2. Zero undecodable bytes / mid-instruction jumps (`status ≠ .trapDecode off`),
3. Zero heap allocations / GC / unexpected calls (`status ≠ .callExternal tgt`),
4. `VZEROUPPER` ABI safety (`status = .returned → upperDirty = false`). -/
theorem checkDecodedAbiSafety_sound (bytes : Array UInt8)
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String)
    (hSafe : checkDecodedAbiSafety bytes rodata extTargets = true) :
    ∀ (s : Amd64VecState) (fuel : Nat),
      s.pc = 0 → s.status = .running → s.upperDirty = false → s.sseHazard = false →
      let s' := runAmd64Vec fuel bytes rodata extTargets s
      s'.sseHazard = false ∧
      (∀ off, s'.status ≠ .trapDecode off) ∧
      (∀ tgt, s'.status ≠ .callExternal tgt) ∧
      (s'.status = .returned → s'.upperDirty = false) := by
  unfold checkDecodedAbiSafety at hSafe
  cases hProg : decodeAmd64Program bytes with
  | none => simp [hProg] at hSafe
  | some insns =>
    simp only [hProg] at hSafe
    split at hSafe
    · next hPos =>
      split at hSafe
      · cases hSafe
      · next hOff0 =>
        have hZeroOff : insns[0].offset = 0 := by simpa using hOff0
        rcases Option.isSome_iff_exists.mp hSafe with ⟨finalSeen, hBfs⟩
        have hInv0 : AbiBfsInv insns rodata extTargets
            ((Array.replicate (2 * insns.size) false).set 0 true (by simp; omega)) [(0, false)] := by
          refine ⟨by simp, ?_, ?_, ?_⟩
          · intro idx dirty hm
            simp only [List.mem_singleton, Prod.mk.injEq] at hm
            rcases hm with ⟨rfl, rfl⟩
            exact ⟨hPos, by simp [abiStateKey]⟩
          · intro _; simp
          · intro idx dirty hi hv
            rw [array_bool_getElem?_set] at hv
            split at hv
            · next h0 =>
              have hp := abiStateKey_inj (i1 := 0) (d1 := false) (i2 := idx) (d2 := dirty) (by simpa [abiStateKey] using h0)
              left; simp [hp.1, hp.2]
            · have hkLt : abiStateKey idx dirty < (Array.replicate (2 * insns.size) false).size := by
                simpa using abiStateKey_lt dirty hi
              rw [Array.getElem?_eq_getElem hkLt] at hv
              simp at hv
        have hFinal := abiBfs_inv insns rodata extTargets (4 * insns.size + 2) _ [(0, false)] finalSeen hInv0 hBfs
        suffices hInd : ∀ (fuel : Nat) (s : Amd64VecState),
            s.sseHazard = false →
            (∀ off, s.status ≠ .trapDecode off) →
            (∀ tgt, s.status ≠ .callExternal tgt) →
            (s.status = .returned → s.upperDirty = false) →
            (s.status = .running → ∃ idx, ∃ (_ : idx < insns.size),
              s.pc = insns[idx].offset ∧ finalSeen[abiStateKey idx s.upperDirty]? = some true) →
            let s' := runAmd64Vec fuel bytes rodata extTargets s
            s'.sseHazard = false ∧
            (∀ off, s'.status ≠ .trapDecode off) ∧
            (∀ tgt, s'.status ≠ .callExternal tgt) ∧
            (s'.status = .returned → s'.upperDirty = false) by
          intro s fuel hPc hRun hDirt hHaz
          apply hInd fuel s hHaz (by simp [hRun]) (by simp [hRun]) (by simp [hRun])
          intro _
          exact ⟨0, hPos, by rw [hPc, hZeroOff], by simpa [abiStateKey, hDirt] using hFinal.2.2.1 hPos⟩
        intro fuel
        induction fuel with
        | zero =>
          intro s hHaz hDec hExt hRet _
          exact ⟨hHaz, hDec, hExt, hRet⟩
        | succ k ih =>
          intro s hHaz hDec hExt hRet hReach
          by_cases hRun : s.status = .running
          · have hB : (s.status == .running) = true := beq_iff_eq.mpr hRun
            dsimp only
            rw [runAmd64Vec]
            simp only [hB, ↓reduceIte]
            rcases hReach hRun with ⟨idx, hIdx, hPcEq, hSeen⟩
            have hStepDec := decodeAmd64Program_elem_decodes bytes insns hProg idx hIdx
            rw [← hPcEq] at hStepDec
            rw [stepAmd64Vec_at bytes rodata extTargets s insns[idx].insn insns[idx].nextOffset hRun hStepDec]
            rcases hFinal.2.2.2 idx s.upperDirty hIdx hSeen with hInNil | ⟨succs, hCheck, hSuccSeen⟩
            · cases hInNil
            · have hSt := execAmd64VecInsn_abi_step insns rodata extTargets idx hIdx s hPcEq hRun hHaz succs hCheck
              apply ih _ hSt.1 hSt.2.1 hSt.2.2.1 hSt.2.2.2.1
              intro hRun'
              rcases hSt.2.2.2.2 hRun' with ⟨i', hi', hPc', hMemS⟩
              exact ⟨i', hi', hPc', hSuccSeen i' _ hMemS⟩
          · have hB : (s.status == .running) = false := beq_false_of_ne hRun
            dsimp only
            rw [runAmd64Vec]
            simp only [hB, Bool.false_eq_true, ↓reduceIte]
            exact ⟨hHaz, hDec, hExt, hRet⟩
    · cases hSafe

private def checkDecodedAbiSafetyChunks (chunks : List (List UInt8))
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String) : Bool :=
  match decodeAmd64ProgramChunks chunks with
  | none => false
  | some insns =>
    if hPos : 0 < insns.size then
      if insns[0].offset != 0 then false
      else
        let n := insns.size
        let seen0 := (Array.replicate (2 * n) false).set 0 true (by simp; omega)
        (abiBfs insns rodata extTargets (4 * n + 2) seen0 [(0, false)]).isSome
    else false

private theorem checkDecodedAbiSafety_of_chunks (chunks : List (List UInt8))
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String)
    (hWf : chunks16WellFormed chunks = true)
    (hCert : checkDecodedAbiSafetyChunks chunks rodata extTargets = true) :
    checkDecodedAbiSafety ⟨chunks.flatten⟩ rodata extTargets = true := by
  unfold checkDecodedAbiSafety
  rw [decodeAmd64ProgramChunks_eq chunks hWf]
  exact hCert

set_option maxRecDepth 4096 in
private theorem allAbsLE64AVX2_abi_cert :
    checkDecodedAbiSafety allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets = true :=
  checkDecodedAbiSafety_of_chunks allAbsLE64AVX2ByteChunks _ _ (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem allAbsLE64AVX512_abi_cert :
    checkDecodedAbiSafety allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets = true :=
  checkDecodedAbiSafety_of_chunks allAbsLE64AVX512ByteChunks _ _ (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem minNoNaN64AVX2_abi_cert :
    checkDecodedAbiSafety minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets = true :=
  checkDecodedAbiSafety_of_chunks minNoNaN64AVX2ByteChunks _ _ (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem minNoNaN64AVX512_abi_cert :
    checkDecodedAbiSafety minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets = true :=
  checkDecodedAbiSafety_of_chunks minNoNaN64AVX512ByteChunks _ _ (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_abi_cert :
    checkDecodedAbiSafety initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets = true :=
  checkDecodedAbiSafety_of_chunks initReducedRow64AVX2ByteChunks _ _ (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_abi_cert :
    checkDecodedAbiSafety initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets = true :=
  checkDecodedAbiSafety_of_chunks initReducedRow64AVX512ByteChunks _ _ (by decide) (by decide)

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `allAbsLE64AVX2` ELF bytes. -/
theorem allAbsLE64AVX2_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ allAbsLE64AVX2_abi_cert s fuel hPc hRun hDirt hHaz

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `allAbsLE64AVX512` ELF bytes. -/
theorem allAbsLE64AVX512_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ allAbsLE64AVX512_abi_cert s fuel hPc hRun hDirt hHaz

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `minNoNaN64AVX2` ELF bytes. -/
theorem minNoNaN64AVX2_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ minNoNaN64AVX2_abi_cert s fuel hPc hRun hDirt hHaz

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `minNoNaN64AVX512` ELF bytes. -/
theorem minNoNaN64AVX512_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ minNoNaN64AVX512_abi_cert s fuel hPc hRun hDirt hHaz

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `initReducedRow64AVX2` ELF bytes. -/
theorem initReducedRow64AVX2_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ initReducedRow64AVX2_abi_cert s fuel hPc hRun hDirt hHaz

/-- Machine-checked `VZEROUPPER` ABI & zero-GC safety for decoded `initReducedRow64AVX512` ELF bytes. -/
theorem initReducedRow64AVX512_abi_and_zeroGC_safe (s : Amd64VecState) (fuel : Nat)
    (hPc : s.pc = 0) (hRun : s.status = .running)
    (hDirt : s.upperDirty = false) (hHaz : s.sseHazard = false) :
    let s' := runAmd64Vec fuel initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
    s'.sseHazard = false ∧ (∀ off, s'.status ≠ .trapDecode off) ∧
    (∀ tgt, s'.status ≠ .callExternal tgt) ∧ (s'.status = .returned → s'.upperDirty = false) :=
  checkDecodedAbiSafety_sound _ _ _ initReducedRow64AVX512_abi_cert s fuel hPc hRun hDirt hHaz

/-!
#### Static Frame & Callee/Go-ABI Register Preservation (`RSP`, `RBP`, `R14`, `X15`)
-/

/-- Abstract per-PC frame tracking state for Go amd64 ABI preservation verification. -/
private structure FrameAbiSlot where
  spDelta : BitVec 64
  rbpSaved : Bool
  rdxDelta : Option (BitVec 64)
  deriving DecidableEq, Repr, Inhabited

/-- Returns true iff `insn` never modifies Go's goroutine pointer `R14` (`gpr 14#4`) or
the low 128 bits (`l0`, `l1`) of Go's fixed-zero vector register `X15` (`zmm 15#5`). -/
private def insnPreservesR14AndX15 : Amd64VecInsn → Bool
  | .nop | .pushReg _ | .ret | .movMR64 _ _ | .testRR64 _ _ | .cmpRR64 _ _
  | .cmpRM64 _ _ | .cmpImm64 _ _ | .cmpImm8 _ _ | .jmpRel _ | .jccRel _ _ | .callRel _
  | .sseMovupsStore _ _ | .sseUcomisdRR _ _ | .vzeroupper
  | .vmovdquStore _ _ _ _ _ | .vcmppdEvex _ _ _ _ _ _ | .vpmovq2m _ _ _ => true
  | .popReg dst | .movRR64 dst _ | .movRM64 dst _ | .movImm32 dst _ | .movImm64 dst _
  | .lea64 dst _ | .xorRR32 dst _ | .addImm64 dst _ | .subImm64 dst _ | .inc64 dst
  | .vmovmskpd _ dst _ | .kmovbToGpr dst _ => dst != 14#4
  | .sseMovupsRR dst _ | .sseMovupsLoad dst _ | .sseMovsdLoad dst _ | .sseMovqFromGpr dst _
  | .sseXorpsRR dst _ | .ssePorRR dst _ | .sseMinsdRR dst _ => dst != 15#4
  | .vmovsdLoad dst _ | .vmovsdRR dst _ _ | .vmovqFromGpr dst _
  | .vbroadcastsd _ _ dst _ _ | .vpbroadcastq _ _ dst _ _
  | .vmovdquLoad _ _ dst _ _ | .vmovdquRR _ _ dst _ _
  | .vpand _ _ dst _ _ _ | .vsubpd _ _ dst _ _ _ | .vminpd _ _ dst _ _ _
  | .vcmppdVex _ dst _ _ _ | .vpblendvb _ dst _ _ _ | .vpblendmq _ dst _ _ _
  | .vpmovm2q _ dst _ => dst != 15#5

private theorem bv4_zeroExtend5_ne_15 {r : BitVec 4} (h : r ≠ 15#4) :
    (15#5 : BitVec 5) ≠ r.zeroExtend 5 := by
  revert r h; decide

private theorem execAmd64VecInsn_preserves_R14_X15
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (s : Amd64VecState) (insn : Amd64VecInsn) (nextOff : Nat)
    (hP : insnPreservesR14AndX15 insn = true) :
    let s' := execAmd64VecInsn rodata ext s insn nextOff
    s'.gpr 14#4 = s.gpr 14#4 ∧
    (s'.zmm 15#5).l0 = (s.zmm 15#5).l0 ∧
    (s'.zmm 15#5).l1 = (s.zmm 15#5).l1 := by
  have h4ne14 : (14#4 : BitVec 4) ≠ 4#4 := by decide
  cases insn with
  | nop | ret | testRR64 _ _ | cmpRR64 _ _ | cmpImm64 _ _ | cmpImm8 _ _
  | sseUcomisdRR _ _ | vpmovq2m _ _ _ =>
    simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne14]
  | pushReg r =>
    simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne14]
  | popReg dst =>
    have hNe : (14#4 : BitVec 4) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    simp [execAmd64VecInsn, setGpr, beq_false_of_ne hNe, beq_false_of_ne h4ne14]
  | movRR64 dst _ | movImm32 dst _ | movImm64 dst _ | xorRR32 dst _
  | addImm64 dst _ | subImm64 dst _ | inc64 dst | vmovmskpd _ dst _ | kmovbToGpr dst _ =>
    have hNe : (14#4 : BitVec 4) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    simp [execAmd64VecInsn, setGpr, beq_false_of_ne hNe]
  | movMR64 addr _ | sseMovupsStore addr _ | vmovdquStore _ _ addr _ _ =>
    cases hEa : evalMemAddr s.gpr addr <;> simp [execAmd64VecInsn, hEa]
  | movRM64 dst addr =>
    have hNe : (14#4 : BitVec 4) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;>
      simp [execAmd64VecInsn, hRd, setGpr, beq_false_of_ne hNe]
  | lea64 dst addr =>
    have hNe : (14#4 : BitVec 4) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hEa : evalMemAddr s.gpr addr <;>
      simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne hNe]
  | cmpRM64 _ addr =>
    cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hRd]
  | jmpRel rel =>
    cases hT : nextTargetPc nextOff rel <;> simp [execAmd64VecInsn, hT]
  | jccRel cond rel =>
    cases hC : evalX86Cond cond s.flags
    · simp [execAmd64VecInsn, hC]
    · cases hT : nextTargetPc nextOff rel <;> simp [execAmd64VecInsn, hC, hT]
  | callRel _ =>
    cases hE : ext s.pc with
    | none => simp [execAmd64VecInsn, hE]
    | some sym =>
      simp only [execAmd64VecInsn, hE]
      split <;> simp
  | sseMovupsRR dst _ | sseMovqFromGpr dst _ | sseXorpsRR dst _ | ssePorRR dst _ | sseMinsdRR dst _ =>
    have hNe : (15#5 : BitVec 5) ≠ dst.zeroExtend 5 := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP
      exact bv4_zeroExtend5_ne_15 hP
    simp [execAmd64VecInsn, setZmm, beq_false_of_ne hNe]
  | sseMovupsLoad dst addr =>
    have hNe : (15#5 : BitVec 5) ≠ dst.zeroExtend 5 := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP
      exact bv4_zeroExtend5_ne_15 hP
    cases hEa : evalMemAddr s.gpr addr <;> simp [execAmd64VecInsn, hEa, setZmm, beq_false_of_ne hNe]
  | sseMovsdLoad dst addr =>
    have hNe : (15#5 : BitVec 5) ≠ dst.zeroExtend 5 := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP
      exact bv4_zeroExtend5_ne_15 hP
    cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hRd, setZmm, beq_false_of_ne hNe]
  | vzeroupper =>
    simp [execAmd64VecInsn, Vec512.zeroHi128]
  | vmovsdLoad dst addr =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hRd, setZmm, beq_false_of_ne hNe]
  | vmovsdRR dst _ _ | vmovqFromGpr dst _ | vmovdquRR _ _ dst _ _ | vpmovm2q _ dst _ =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    simp [execAmd64VecInsn, setZmm, beq_false_of_ne hNe]
  | vbroadcastsd _ _ dst src _ | vpbroadcastq _ _ dst src _ =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases src with
    | reg _ => simp [execAmd64VecInsn, setZmm, beq_false_of_ne hNe]
    | mem addr =>
      cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hRd, setZmm, beq_false_of_ne hNe]
  | vmovdquLoad _ _ dst addr _ =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hEa : evalMemAddr s.gpr addr <;> simp [execAmd64VecInsn, hEa, setZmm, beq_false_of_ne hNe]
  | vpand _ w dst _ src2 mask | vsubpd _ w dst _ src2 mask | vminpd _ w dst _ src2 mask =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hRm : evalVecRm s w src2 mask.broadcast <;> simp [execAmd64VecInsn, hRm, setZmm, beq_false_of_ne hNe]
  | vcmppdVex w dst _ src2 _ | vpblendvb w dst _ src2 _ =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hRm : evalVecRm s w src2 false <;> simp [execAmd64VecInsn, hRm, setZmm, beq_false_of_ne hNe]
  | vpblendmq w dst _ src2 mask =>
    have hNe : (15#5 : BitVec 5) ≠ dst := by
      simp only [insnPreservesR14AndX15, bne_iff_ne] at hP; exact Ne.symm hP
    cases hRm : evalVecRm s w src2 mask.broadcast <;> simp [execAmd64VecInsn, hRm, setZmm, beq_false_of_ne hNe]
  | vcmppdEvex w _ _ src2 _ mask =>
    cases hRm : evalVecRm s w src2 mask.broadcast <;> simp [execAmd64VecInsn, hRm]

private def vecLanes : VecWidth → Nat
  | .xmm128 => 2
  | .ymm256 => 4
  | .zmm512 => 8

private def storeSafeFromSavedRbp (slot : FrameAbiSlot) (addr : MemAddr) (nLanes : Nat) : Bool :=
  if !slot.rbpSaved then true
  else
    match addr with
    | .baseIdxDisp (some baseReg) none disp =>
      let baseDelta? : Option (BitVec 64) :=
        if baseReg == 4#4 then some slot.spDelta
        else if baseReg == 2#4 then slot.rdxDelta
        else none
      match baseDelta? with
      | none => false
      | some bd =>
        let baseOff := bd + BitVec.ofInt 64 disp
        (List.range nLanes).all (fun k => baseOff + BitVec.ofNat 64 (k * 8) != 0xfffffffffffffff8#64)
    | _ => false

/-- Abstract step on `(spDelta, rbpSaved, rdxDelta)` for Go ABI frame verification.
When `checkRbp = false`, skips checking `RBP` restoration and memory non-aliasing with `[sp0 - 8]`. -/
private def frameAbiStep (checkRbp : Bool) (slot : FrameAbiSlot) (insn : Amd64VecInsn) :
    Option FrameAbiSlot :=
  if !insnPreservesR14AndX15 insn then none
  else
    let updRdx (dst : BitVec 4) : Option (BitVec 64) :=
      if dst == 2#4 then none else slot.rdxDelta
    match insn with
    | .pushReg r =>
      if r == 5#4 then
        if !slot.rbpSaved && slot.spDelta == 0#64 then
          some { slot with spDelta := slot.spDelta - 8#64, rbpSaved := true }
        else if !checkRbp then
          some { slot with spDelta := slot.spDelta - 8#64 }
        else none
      else if !checkRbp || (!slot.rbpSaved || slot.spDelta - 8#64 != 0xfffffffffffffff8#64) then
        some { slot with spDelta := slot.spDelta - 8#64 }
      else none
    | .popReg r =>
      if r == 4#4 then none
      else if r == 5#4 then
        if slot.rbpSaved && slot.spDelta == 0xfffffffffffffff8#64 then
          some { spDelta := 0#64, rbpSaved := false, rdxDelta := slot.rdxDelta }
        else if !checkRbp then
          some { spDelta := slot.spDelta + 8#64, rbpSaved := false, rdxDelta := slot.rdxDelta }
        else none
      else
        some { slot with spDelta := slot.spDelta + 8#64, rdxDelta := updRdx r }
    | .ret =>
      if slot.spDelta == 0#64 && (!checkRbp || !slot.rbpSaved) then
        some { slot with spDelta := 8#64 }
      else none
    | .addImm64 dst imm =>
      if dst == 4#4 then
        some { slot with spDelta := slot.spDelta + imm }
      else if checkRbp && dst == 5#4 && !slot.rbpSaved then none
      else some { slot with rdxDelta := updRdx dst }
    | .subImm64 dst imm =>
      if dst == 4#4 then
        some { slot with spDelta := slot.spDelta - imm }
      else if checkRbp && dst == 5#4 && !slot.rbpSaved then none
      else some { slot with rdxDelta := updRdx dst }
    | .lea64 dst addr =>
      if dst == 4#4 then none
      else if checkRbp && dst == 5#4 && !slot.rbpSaved then none
      else if dst == 2#4 then
        match addr with
        | .baseIdxDisp (some 4#4) none disp =>
          some { slot with rdxDelta := some (slot.spDelta + BitVec.ofInt 64 disp) }
        | _ => some { slot with rdxDelta := none }
      else some slot
    | .movRR64 dst _ | .movRM64 dst _ | .movImm32 dst _ | .movImm64 dst _
    | .xorRR32 dst _ | .inc64 dst | .vmovmskpd _ dst _ | .kmovbToGpr dst _ =>
      if dst == 4#4 then none
      else if checkRbp && dst == 5#4 && !slot.rbpSaved then none
      else some { slot with rdxDelta := updRdx dst }
    | .movMR64 addr _ =>
      if checkRbp && !storeSafeFromSavedRbp slot addr 1 then none else some slot
    | .sseMovupsStore addr _ =>
      if checkRbp && !storeSafeFromSavedRbp slot addr 2 then none else some slot
    | .vmovdquStore _ w addr _ _ =>
      if checkRbp && !storeSafeFromSavedRbp slot addr (vecLanes w) then none else some slot
    | _ => some slot

private def frameNextIndices (insns : Array DecodedAmd64Insn) (idx : Nat) : Option (List Nat) :=
  if h : idx < insns.size then
    let d := insns[idx]
    match d.insn with
    | .ret | .callRel _ => some []
    | .jmpRel rel =>
      match nextTargetPc d.nextOffset rel >>= findInsnIdxByOff insns with
      | some t => some [t]
      | none => none
    | .jccRel _ rel =>
      match findInsnIdxByOff insns d.nextOffset, nextTargetPc d.nextOffset rel >>= findInsnIdxByOff insns with
      | some f, some t => some [f, t]
      | _, _ => none
    | _ =>
      match findInsnIdxByOff insns d.nextOffset with
      | some f => some [f]
      | none => none
  else none

private def computeFrameTableLoop (checkRbp : Bool) (insns : Array DecodedAmd64Insn) :
    Nat → Array (Option FrameAbiSlot) → List Nat → Option (Array (Option FrameAbiSlot))
  | 0, tbl, [] => some tbl
  | 0, _, _ :: _ => none
  | _ + 1, tbl, [] => some tbl
  | fuel + 1, tbl, idx :: rest =>
    match tbl[idx]? with
    | some (some slot) =>
      if h : idx < insns.size then
        match frameAbiStep checkRbp slot insns[idx].insn, frameNextIndices insns idx with
        | some slot', some succs =>
          let pushRes := succs.foldl (fun (acc : Option (Array (Option FrameAbiSlot) × List Nat)) (nxt : Nat) =>
            match acc with
            | none => none
            | some (t, q) =>
              if hNxt : nxt < t.size then
                match t[nxt] with
                | none => some (t.set nxt (some slot') hNxt, nxt :: q)
                | some prev => if prev == slot' then some (t, q) else none
              else none) (some (tbl, rest))
          match pushRes with
          | none => none
          | some (tbl', q') => computeFrameTableLoop checkRbp insns fuel tbl' q'
        | _, _ => none
      else none
    | _ => none

private def verifyFrameTableGo (checkRbp : Bool) (insns : Array DecodedAmd64Insn)
    (tbl : Array (Option FrameAbiSlot)) : Nat → Nat → Bool
  | 0, _ => true
  | rem + 1, idx =>
    if h : idx < insns.size then
      let okHere :=
        match tbl[idx]? with
        | some (some slot) =>
          match frameAbiStep checkRbp slot insns[idx].insn, frameNextIndices insns idx with
          | some slot', some succs =>
            succs.all (fun nxt => tbl[nxt]? == some (some slot'))
          | _, _ => false
        | _ => true
      okHere && verifyFrameTableGo checkRbp insns tbl rem (idx + 1)
    else true

private theorem verifyFrameTableGo_sound (checkRbp : Bool) (insns : Array DecodedAmd64Insn)
    (tbl : Array (Option FrameAbiSlot)) :
    ∀ (rem start idx : Nat) (slot : FrameAbiSlot),
      verifyFrameTableGo checkRbp insns tbl rem start = true →
      start ≤ idx → idx < start + rem → (hIdx : idx < insns.size) →
      tbl[idx]? = some (some slot) →
      ∃ slot' succs,
        frameAbiStep checkRbp slot insns[idx].insn = some slot' ∧
        frameNextIndices insns idx = some succs ∧
        ∀ nxt ∈ succs, tbl[nxt]? = some (some slot') := by
  intro rem
  induction rem with
  | zero => intro start idx _ _ hLe hLt; omega
  | succ r ih =>
    intro start idx slot hOk hLe hLt hIdx hSlot
    unfold verifyFrameTableGo at hOk
    have hStart : start < insns.size := by omega
    simp only [hStart, ↓reduceDIte, Bool.and_eq_true] at hOk
    by_cases hEq : start = idx
    · subst hEq
      have hHere := hOk.1
      simp only [hSlot] at hHere
      cases hStep : frameAbiStep checkRbp slot insns[start].insn with
      | none => simp [hStep] at hHere
      | some slot' =>
        cases hNext : frameNextIndices insns start with
        | none => simp [hStep, hNext] at hHere
        | some succs =>
          simp only [hStep, hNext, List.all_eq_true, beq_iff_eq] at hHere
          exact ⟨slot', succs, rfl, rfl, hHere⟩
    · exact ih (start + 1) idx slot hOk.2 (by omega) (by omega) hIdx hSlot

private def verifyFrameTable (checkRbp : Bool) (insns : Array DecodedAmd64Insn)
    (tbl : Array (Option FrameAbiSlot)) : Bool :=
  if hPos : 0 < insns.size then
    tbl.size == insns.size &&
    insns[0].offset == 0 &&
    tbl[0]? == some (some { spDelta := 0#64, rbpSaved := false, rdxDelta := none }) &&
    verifyFrameTableGo checkRbp insns tbl insns.size 0
  else false

private def checkDecodedFrameAbi (checkRbp : Bool) (bytes : Array UInt8) : Bool :=
  match decodeAmd64Program bytes with
  | none => false
  | some insns =>
    if hPos : 0 < insns.size then
      let tbl0 := (Array.replicate insns.size none).set 0
        (some { spDelta := 0#64, rbpSaved := false, rdxDelta := none }) (by simp [hPos])
      match computeFrameTableLoop checkRbp insns (4 * insns.size + 2) tbl0 [0] with
      | none => false
      | some tbl => verifyFrameTable checkRbp insns tbl
    else false

private def checkDecodedFrameAbiChunks (checkRbp : Bool) (chunks : List (List UInt8)) : Bool :=
  match decodeAmd64ProgramChunks chunks with
  | none => false
  | some insns =>
    if hPos : 0 < insns.size then
      let tbl0 := (Array.replicate insns.size none).set 0
        (some { spDelta := 0#64, rbpSaved := false, rdxDelta := none }) (by simp [hPos])
      match computeFrameTableLoop checkRbp insns (4 * insns.size + 2) tbl0 [0] with
      | none => false
      | some tbl => verifyFrameTable checkRbp insns tbl
    else false

private theorem checkDecodedFrameAbi_of_chunks (checkRbp : Bool) (chunks : List (List UInt8))
    (hWf : chunks16WellFormed chunks = true)
    (hCert : checkDecodedFrameAbiChunks checkRbp chunks = true) :
    checkDecodedFrameAbi checkRbp ⟨chunks.flatten⟩ = true := by
  unfold checkDecodedFrameAbi
  rw [decodeAmd64ProgramChunks_eq chunks hWf]
  exact hCert

private def FrameSlotHolds (checkRbp : Bool) (s0 s : Amd64VecState) (slot : FrameAbiSlot) : Prop :=
  s.gpr 4#4 = s0.gpr 4#4 + slot.spDelta ∧
  (∀ bd, slot.rdxDelta = some bd → s.gpr 2#4 = s0.gpr 4#4 + bd) ∧
  (checkRbp = true →
    (slot.rbpSaved = false → s.gpr 5#4 = s0.gpr 5#4) ∧
    (slot.rbpSaved = true → s.mem (s0.gpr 4#4 - 8#64) = s0.gpr 5#4))

private theorem storeSafeFromSavedRbp_ea_ne {s0 s : Amd64VecState} {slot : FrameAbiSlot}
    {addr : MemAddr} {nLanes : Nat} {ea : BitVec 64}
    (hHolds : FrameSlotHolds true s0 s slot)
    (hSaved : slot.rbpSaved = true)
    (hSafe : storeSafeFromSavedRbp slot addr nLanes = true)
    (hEa : evalMemAddr s.gpr addr = some ea) :
    ∀ k < nLanes, ea + BitVec.ofNat 64 (k * 8) ≠ s0.gpr 4#4 - 8#64 := by
  unfold storeSafeFromSavedRbp at hSafe
  simp only [hSaved, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hSafe
  cases addr with
  | ripRel _ => simp at hSafe
  | baseIdxDisp baseOpt idxOpt disp =>
    cases baseOpt with
    | none => simp at hSafe
    | some baseReg =>
      cases idxOpt with
      | some _ => simp at hSafe
      | none =>
        dsimp only at hSafe
        have hBd : ∃ bd, s.gpr baseReg = s0.gpr 4#4 + bd ∧
            (List.range nLanes).all (fun k => bd + BitVec.ofInt 64 disp + BitVec.ofNat 64 (k * 8) != 0xfffffffffffffff8#64) = true := by
          by_cases h4 : baseReg = 4#4
          · subst h4
            simp only [beq_self_eq_true, ↓reduceIte] at hSafe
            exact ⟨slot.spDelta, hHolds.1, hSafe⟩
          · simp only [beq_false_of_ne h4, Bool.false_eq_true, ↓reduceIte] at hSafe
            by_cases h2 : baseReg = 2#4
            · subst h2
              simp only [beq_self_eq_true, ↓reduceIte] at hSafe
              cases hRdx : slot.rdxDelta with
              | none => simp [hRdx] at hSafe
              | some bd =>
                simp only [hRdx] at hSafe
                exact ⟨bd, hHolds.2.1 bd hRdx, hSafe⟩
            · simp [beq_false_of_ne h2] at hSafe
        rcases hBd with ⟨bd, hRegEq, hAll⟩
        simp only [evalMemAddr, Option.some.injEq] at hEa
        rw [hRegEq] at hEa
        simp only [List.all_eq_true, List.mem_range, bne_iff_ne] at hAll
        intro k hk
        have hNe := hAll k hk
        rw [← hEa]
        bv_omega

private theorem write64_storeVecMem_preserves_ne {m : BitVec 64 → BitVec 64} {w : VecWidth}
    {ea target : BitVec 64} {v : Vec512} {kmask : BitVec 3} {kVal : BitVec 8}
    (hNe : ∀ k < vecLanes w, ea + BitVec.ofNat 64 (k * 8) ≠ target) :
    storeVecMem m w ea v kmask kVal target = m target := by
  have hWr : ∀ (m0 : BitVec 64 → BitVec 64) (i : Nat) (val : BitVec 64),
      ea + BitVec.ofNat 64 (i * 8) ≠ target →
      (if kmask == 0#3 || Vec512.maskBit kVal i then write64 m0 (ea + BitVec.ofNat 64 (i * 8)) val else m0) target = m0 target := by
    intro m0 i val hN
    split
    · simp only [write64, beq_false_of_ne (Ne.symm hN), Bool.false_eq_true, ↓reduceIte]
    · rfl
  cases w with
  | xmm128 =>
    unfold storeVecMem; dsimp only
    rw [hWr _ 1 _ (hNe 1 (by decide)), hWr _ 0 _ (hNe 0 (by decide))]
  | ymm256 =>
    unfold storeVecMem; dsimp only
    rw [hWr _ 3 _ (hNe 3 (by decide)), hWr _ 2 _ (hNe 2 (by decide)),
        hWr _ 1 _ (hNe 1 (by decide)), hWr _ 0 _ (hNe 0 (by decide))]
  | zmm512 =>
    unfold storeVecMem; dsimp only
    rw [hWr _ 7 _ (hNe 7 (by decide)), hWr _ 6 _ (hNe 6 (by decide)),
        hWr _ 5 _ (hNe 5 (by decide)), hWr _ 4 _ (hNe 4 (by decide)),
        hWr _ 3 _ (hNe 3 (by decide)), hWr _ 2 _ (hNe 2 (by decide)),
        hWr _ 1 _ (hNe 1 (by decide)), hWr _ 0 _ (hNe 0 (by decide))]

private theorem execAmd64VecInsn_frameAbi_step
    (checkRbp : Bool) (insns : Array DecodedAmd64Insn)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (idx : Nat) (hIdx : idx < insns.size) (s0 s : Amd64VecState)
    (slot slot' : FrameAbiSlot) (succs : List Nat)
    (hRun : s.status = .running)
    (hHolds : FrameSlotHolds checkRbp s0 s slot)
    (hStep : frameAbiStep checkRbp slot insns[idx].insn = some slot')
    (hSuccs : frameNextIndices insns idx = some succs) :
    let s' := execAmd64VecInsn rodata ext s insns[idx].insn insns[idx].nextOffset
    s'.gpr 14#4 = s.gpr 14#4 ∧
    (s'.zmm 15#5).l0 = (s.zmm 15#5).l0 ∧
    (s'.zmm 15#5).l1 = (s.zmm 15#5).l1 ∧
    (s'.status = .returned →
      s'.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
      (checkRbp = true → s'.gpr 5#4 = s0.gpr 5#4)) ∧
    (s'.status = .running →
      FrameSlotHolds checkRbp s0 s' slot' ∧
      ∃ nxt ∈ succs, ∃ (hNxt : nxt < insns.size), s'.pc = insns[nxt].offset) := by
  have hP : insnPreservesR14AndX15 insns[idx].insn = true := by
    unfold frameAbiStep at hStep
    cases h : insnPreservesR14AndX15 insns[idx].insn
    · simp [h] at hStep
    · rfl
  have hR14X15 := execAmd64VecInsn_preserves_R14_X15 rodata ext s insns[idx].insn insns[idx].nextOffset hP
  refine ⟨hR14X15.1, hR14X15.2.1, hR14X15.2.2, ?_, ?_⟩
  · -- Returned case: only `.ret` sets `status = .returned`.
    intro hRet
    cases hI : insns[idx].insn with
    | ret =>
      rw [hI] at hStep
      unfold frameAbiStep at hStep
      dsimp only [insnPreservesR14AndX15] at hStep
      have hCond : (slot.spDelta == 0#64 && (!checkRbp || !slot.rbpSaved)) = true := by
        cases hc : (slot.spDelta == 0#64 && (!checkRbp || !slot.rbpSaved))
        · simp [hc] at hStep
        · rfl
      simp only [Bool.and_eq_true, beq_iff_eq, Bool.or_eq_true, Bool.not_eq_true'] at hCond
      have hSp : s.gpr 4#4 = s0.gpr 4#4 := by rw [hHolds.1, hCond.1]; bv_omega
      have h4ne5 : (5#4 : BitVec 4) ≠ 4#4 := by decide
      refine ⟨?_, ?_⟩
      · simp [execAmd64VecInsn, setGpr, hSp]
      · intro hChk
        have hNotSaved : slot.rbpSaved = false := by rcases hCond.2 with hF | hF <;> simp_all
        simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne5, (hHolds.2.2 hChk).1 hNotSaved]
    | callRel _ =>
      cases hE : ext s.pc with
      | none => simp [execAmd64VecInsn, hI, hE] at hRet
      | some sym =>
        simp only [execAmd64VecInsn, hI, hE] at hRet
        split at hRet <;> simp at hRet
    | jmpRel rel =>
      cases hT : nextTargetPc insns[idx].nextOffset rel <;> simp [execAmd64VecInsn, hI, hT, hRun] at hRet
    | jccRel cond rel =>
      cases hC : evalX86Cond cond s.flags
      · simp [execAmd64VecInsn, hI, hC, hRun] at hRet
      · cases hT : nextTargetPc insns[idx].nextOffset rel <;> simp [execAmd64VecInsn, hI, hC, hT, hRun] at hRet
    | movMR64 addr _ | lea64 _ addr | sseMovupsLoad _ addr | sseMovupsStore addr _
    | vmovdquLoad _ _ _ addr _ | vmovdquStore _ _ addr _ _ =>
      cases hEa : evalMemAddr s.gpr addr <;> simp [execAmd64VecInsn, hI, hEa, hRun] at hRet
    | movRM64 _ addr | cmpRM64 _ addr | sseMovsdLoad _ addr | vmovsdLoad _ addr =>
      cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hI, hRd, hRun] at hRet
    | vbroadcastsd _ _ _ src _ | vpbroadcastq _ _ _ src _ =>
      cases src with
      | reg _ => simp [execAmd64VecInsn, hI, hRun] at hRet
      | mem addr => cases hRd : readMem64 rodata s.pc s.gpr s.mem addr <;> simp [execAmd64VecInsn, hI, hRd, hRun] at hRet
    | vpand _ w _ _ src2 mask | vsubpd _ w _ _ src2 mask | vminpd _ w _ _ src2 mask
    | vcmppdEvex w _ _ src2 _ mask | vpblendmq w _ _ src2 mask =>
      cases hRm : evalVecRm s w src2 mask.broadcast <;> simp [execAmd64VecInsn, hI, hRm, hRun] at hRet
    | vcmppdVex w _ _ src2 _ | vpblendvb w _ _ src2 _ =>
      cases hRm : evalVecRm s w src2 false <;> simp [execAmd64VecInsn, hI, hRm, hRun] at hRet
    | nop | pushReg _ | popReg _ | movRR64 _ _ | movImm32 _ _ | movImm64 _ _
    | xorRR32 _ _ | addImm64 _ _ | subImm64 _ _ | inc64 _ | testRR64 _ _
    | cmpRR64 _ _ | cmpImm64 _ _ | cmpImm8 _ _ | sseMovupsRR _ _ | sseMovqFromGpr _ _
    | sseXorpsRR _ _ | ssePorRR _ _ | sseMinsdRR _ _ | sseUcomisdRR _ _
    | vzeroupper | vmovsdRR _ _ _ | vmovqFromGpr _ _ | vmovdquRR _ _ _ _ _
    | vmovmskpd _ _ _ | vpmovm2q _ _ _ | vpmovq2m _ _ _ | kmovbToGpr _ _ =>
      simp [execAmd64VecInsn, hI, hRun] at hRet
  · -- Running case: show `FrameSlotHolds` and successor PC in `succs`.
    intro hRun'
    have hFall : ∀ {f : Nat}, findInsnIdxByOff insns insns[idx].nextOffset = some f →
        succs = [f] →
        ∃ nxt ∈ succs, ∃ (hNxt : nxt < insns.size), insns[idx].nextOffset = insns[nxt].offset := by
      intro f hF hEq
      subst hEq
      rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hLt, hOff⟩
      exact ⟨f, List.Mem.head [], hLt, hOff.symm⟩
    unfold frameNextIndices at hSuccs
    simp only [hIdx, ↓reduceDIte] at hSuccs
    unfold frameAbiStep at hStep
    simp only [hP, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hStep
    cases hI : insns[idx].insn <;> simp only [hI] at hStep hSuccs hRun' ⊢
    case ret => simp [execAmd64VecInsn] at hRun'
    case callRel _ =>
      cases hE : ext s.pc with
      | none => simp [execAmd64VecInsn, hE] at hRun'
      | some sym =>
        simp only [execAmd64VecInsn, hE] at hRun'
        split at hRun' <;> simp at hRun'
    case jmpRel rel =>
      cases hStep
      cases hT : nextTargetPc insns[idx].nextOffset rel with
      | none => simp [hT] at hSuccs
      | some tgtOff =>
        simp only [hT, Option.bind_eq_bind, Option.bind_some] at hSuccs
        cases hF : findInsnIdxByOff insns tgtOff with
        | none => simp [hF] at hSuccs
        | some t =>
          simp only [hF, Option.some.injEq] at hSuccs
          rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hLt, hOff⟩
          refine ⟨?_, t, by rw [← hSuccs]; exact List.Mem.head [], hLt, by simp [execAmd64VecInsn, hT, hOff]⟩
          simpa [execAmd64VecInsn, hT, FrameSlotHolds] using hHolds
    case jccRel cond rel =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        cases hT : nextTargetPc insns[idx].nextOffset rel with
        | none => simp [hF, hT] at hSuccs
        | some tgtOff =>
          simp only [hF, hT, Option.bind_eq_bind, Option.bind_some] at hSuccs
          cases hFt : findInsnIdxByOff insns tgtOff with
          | none => simp [hFt] at hSuccs
          | some t =>
            simp only [hFt, Option.some.injEq] at hSuccs
            rcases findInsnIdxByOff_sound insns _ _ hF with ⟨hFLt, hFOff⟩
            rcases findInsnIdxByOff_sound insns _ _ hFt with ⟨hTLt, hTOff⟩
            cases hC : evalX86Cond cond s.flags
            · refine ⟨by simpa [execAmd64VecInsn, hC, FrameSlotHolds] using hHolds,
                f, by rw [← hSuccs]; exact List.Mem.head _, hFLt, by simp [execAmd64VecInsn, hC, hFOff]⟩
            · refine ⟨by simpa [execAmd64VecInsn, hC, hT, FrameSlotHolds] using hHolds,
                t, by rw [← hSuccs]; exact List.Mem.tail _ (List.Mem.head _), hTLt,
                by simp [execAmd64VecInsn, hC, hT, hTOff]⟩
    case pushReg r =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        have h4ne2 : (2#4 : BitVec 4) ≠ 4#4 := by decide
        have h4ne5 : (5#4 : BitVec 4) ≠ 4#4 := by decide
        refine ⟨?_, hFall hF hSuccs.symm⟩
        by_cases hr5 : r = 5#4
        · subst hr5
          simp only [beq_self_eq_true, ↓reduceIte] at hStep
          by_cases hSave : (!slot.rbpSaved && slot.spDelta == 0#64) = true
          · simp only [hSave, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            simp only [Bool.and_eq_true, Bool.not_eq_true', beq_iff_eq] at hSave
            refine ⟨?_, ?_, ?_⟩
            · simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]; bv_omega
            · intro bd hBd; simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne2, Bool.false_eq_true, ↓reduceIte]
              exact hHolds.2.1 bd hBd
            · intro hChk
              refine ⟨fun h => Bool.noConfusion h, fun _ => ?_⟩
              have hSp0 : s.gpr 4#4 = s0.gpr 4#4 := by rw [hHolds.1, hSave.2]; bv_omega
              simp only [execAmd64VecInsn, write64, hSp0, beq_self_eq_true, ↓reduceIte]
              exact (hHolds.2.2 hChk).1 hSave.1
          · simp only [hSave, Bool.false_eq_true, ↓reduceIte] at hStep
            cases checkRbp with
            | true => simp at hStep
            | false =>
              simp only [Bool.not_false, ↓reduceIte, Option.some.injEq] at hStep
              subst hStep
              refine ⟨?_, ?_, fun h => Bool.noConfusion h⟩
              · simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]; bv_omega
              · intro bd hBd; simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne2, Bool.false_eq_true, ↓reduceIte]
                exact hHolds.2.1 bd hBd
        · simp only [beq_false_of_ne hr5, Bool.false_eq_true, ↓reduceIte] at hStep
          have hOk : (!checkRbp || (!slot.rbpSaved || slot.spDelta - 8#64 != 0xfffffffffffffff8#64)) = true := by
            cases hc : (!checkRbp || (!slot.rbpSaved || slot.spDelta - 8#64 != 0xfffffffffffffff8#64))
            · simp [hc] at hStep
            · rfl
          simp only [hOk, ↓reduceIte, Option.some.injEq] at hStep
          subst hStep
          refine ⟨?_, ?_, ?_⟩
          · simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]; bv_omega
          · intro bd hBd; simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne2, Bool.false_eq_true, ↓reduceIte]
            exact hHolds.2.1 bd hBd
          · intro hChk
            simp only [hChk, Bool.not_true, Bool.false_or, Bool.or_eq_true, Bool.not_eq_true', bne_iff_ne] at hOk
            refine ⟨fun hNS => by simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne5, (hHolds.2.2 hChk).1 hNS],
              fun hS => ?_⟩
            have hNe : slot.spDelta - 8#64 ≠ 0xfffffffffffffff8#64 := by rcases hOk with hF | hN <;> simp_all
            have hAddrNe : s0.gpr 4#4 - 8#64 ≠ s.gpr 4#4 - 8#64 := by rw [hHolds.1]; bv_omega
            simp only [execAmd64VecInsn, write64, beq_false_of_ne hAddrNe, Bool.false_eq_true, ↓reduceIte]
            exact (hHolds.2.2 hChk).2 hS
    case popReg r =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        by_cases hr4 : r = 4#4
        · subst hr4; simp at hStep
        · simp only [beq_false_of_ne hr4, Bool.false_eq_true, ↓reduceIte] at hStep
          have h4neR : (4#4 : BitVec 4) ≠ r := Ne.symm hr4
          have h4ne2 : (2#4 : BitVec 4) ≠ 4#4 := by decide
          refine ⟨?_, hFall hF hSuccs.symm⟩
          by_cases hr5 : r = 5#4
          · subst hr5
            simp only [beq_self_eq_true, ↓reduceIte] at hStep
            by_cases hRest : (slot.rbpSaved && slot.spDelta == 0xfffffffffffffff8#64) = true
            · simp only [hRest, ↓reduceIte, Option.some.injEq] at hStep
              subst hStep
              simp only [Bool.and_eq_true, beq_iff_eq] at hRest
              have h5ne2 : (2#4 : BitVec 4) ≠ 5#4 := by decide
              refine ⟨?_, ?_, ?_⟩
              · simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4neR, beq_self_eq_true, Bool.false_eq_true,
                  ↓reduceIte, hHolds.1, hRest.2]; bv_omega
              · intro bd hBd
                simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h5ne2, beq_false_of_ne h4ne2,
                  Bool.false_eq_true, ↓reduceIte]
                exact hHolds.2.1 bd hBd
              · intro hChk
                refine ⟨fun _ => ?_, fun h => Bool.noConfusion h⟩
                have hSpNeg8 : s.gpr 4#4 = s0.gpr 4#4 - 8#64 := by rw [hHolds.1, hRest.2]; bv_omega
                simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hSpNeg8]
                exact (hHolds.2.2 hChk).2 hRest.1
            · simp only [hRest, Bool.false_eq_true, ↓reduceIte] at hStep
              cases checkRbp with
              | true => simp at hStep
              | false =>
                simp only [Bool.not_false, ↓reduceIte, Option.some.injEq] at hStep
                subst hStep
                have h5ne2 : (2#4 : BitVec 4) ≠ 5#4 := by decide
                refine ⟨?_, ?_, fun h => Bool.noConfusion h⟩
                · simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4neR, beq_self_eq_true,
                    Bool.false_eq_true, ↓reduceIte, hHolds.1]; bv_omega
                · intro bd hBd
                  simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h5ne2, beq_false_of_ne h4ne2,
                    Bool.false_eq_true, ↓reduceIte]
                  exact hHolds.2.1 bd hBd
          · simp only [beq_false_of_ne hr5, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            have h5neR : (5#4 : BitVec 4) ≠ r := Ne.symm hr5
            have h4ne5 : (5#4 : BitVec 4) ≠ 4#4 := by decide
            refine ⟨?_, ?_, ?_⟩
            · simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4neR, beq_self_eq_true,
                Bool.false_eq_true, ↓reduceIte, hHolds.1]; bv_omega
            · intro bd hBd
              by_cases hr2 : r = 2#4
              · subst hr2; simp at hBd
              · simp only [beq_false_of_ne hr2, Bool.false_eq_true, ↓reduceIte] at hBd
                simp only [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hr2), beq_false_of_ne h4ne2,
                  Bool.false_eq_true, ↓reduceIte]
                exact hHolds.2.1 bd hBd
            · intro hChk
              refine ⟨fun hNS => by
                simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h5neR, beq_false_of_ne h4ne5,
                  Bool.false_eq_true, ↓reduceIte]
                exact (hHolds.2.2 hChk).1 hNS,
                fun hS => (hHolds.2.2 hChk).2 hS⟩
    case addImm64 dst imm =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        refine ⟨?_, hFall hF hSuccs.symm⟩
        by_cases hd4 : dst = 4#4
        · subst hd4
          simp only [beq_self_eq_true, ↓reduceIte, Option.some.injEq] at hStep
          subst hStep
          have h4ne2 : (2#4 : BitVec 4) ≠ 4#4 := by decide
          have h4ne5 : (5#4 : BitVec 4) ≠ 4#4 := by decide
          refine ⟨?_, ?_, ?_⟩
          · simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]; bv_omega
          · intro bd hBd; simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne2, Bool.false_eq_true, ↓reduceIte]
            exact hHolds.2.1 bd hBd
          · intro hChk
            exact ⟨fun hNS => by simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne5, (hHolds.2.2 hChk).1 hNS],
                   fun hS => (hHolds.2.2 hChk).2 hS⟩
        · simp only [beq_false_of_ne hd4, Bool.false_eq_true, ↓reduceIte] at hStep
          by_cases hd5 : (checkRbp && dst == 5#4 && !slot.rbpSaved) = true
          · simp [hd5] at hStep
          · simp only [hd5, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨?_, ?_, ?_⟩
            · simp only [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd4), Bool.false_eq_true, ↓reduceIte]
              exact hHolds.1
            · intro bd hBd
              by_cases hd2 : dst = 2#4
              · subst hd2; simp at hBd
              · simp only [beq_false_of_ne hd2, Bool.false_eq_true, ↓reduceIte] at hBd
                simp only [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd2), Bool.false_eq_true, ↓reduceIte]
                exact hHolds.2.1 bd hBd
            · intro hChk
              refine ⟨fun hNS => ?_, fun hS => (hHolds.2.2 hChk).2 hS⟩
              have h5neD : (5#4 : BitVec 4) ≠ dst := by
                intro hEq; subst hEq; dsimp only at hNS; simp [hChk, hNS] at hd5
              simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h5neD, Bool.false_eq_true, ↓reduceIte]
              exact (hHolds.2.2 hChk).1 hNS
    case subImm64 dst imm =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        refine ⟨?_, hFall hF hSuccs.symm⟩
        by_cases hd4 : dst = 4#4
        · subst hd4
          simp only [beq_self_eq_true, ↓reduceIte, Option.some.injEq] at hStep
          subst hStep
          have h4ne2 : (2#4 : BitVec 4) ≠ 4#4 := by decide
          have h4ne5 : (5#4 : BitVec 4) ≠ 4#4 := by decide
          refine ⟨?_, ?_, ?_⟩
          · simp only [execAmd64VecInsn, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]; bv_omega
          · intro bd hBd; simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne2, Bool.false_eq_true, ↓reduceIte]
            exact hHolds.2.1 bd hBd
          · intro hChk
            exact ⟨fun hNS => by simp [execAmd64VecInsn, setGpr, beq_false_of_ne h4ne5, (hHolds.2.2 hChk).1 hNS],
                   fun hS => (hHolds.2.2 hChk).2 hS⟩
        · simp only [beq_false_of_ne hd4, Bool.false_eq_true, ↓reduceIte] at hStep
          by_cases hd5 : (checkRbp && dst == 5#4 && !slot.rbpSaved) = true
          · simp [hd5] at hStep
          · simp only [hd5, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨?_, ?_, ?_⟩
            · simp only [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd4), Bool.false_eq_true, ↓reduceIte]
              exact hHolds.1
            · intro bd hBd
              by_cases hd2 : dst = 2#4
              · subst hd2; simp at hBd
              · simp only [beq_false_of_ne hd2, Bool.false_eq_true, ↓reduceIte] at hBd
                simp only [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd2), Bool.false_eq_true, ↓reduceIte]
                exact hHolds.2.1 bd hBd
            · intro hChk
              refine ⟨fun hNS => ?_, fun hS => (hHolds.2.2 hChk).2 hS⟩
              have h5neD : (5#4 : BitVec 4) ≠ dst := by
                intro hEq; subst hEq; dsimp only at hNS; simp [hChk, hNS] at hd5
              simp only [execAmd64VecInsn, setGpr, beq_false_of_ne h5neD, Bool.false_eq_true, ↓reduceIte]
              exact (hHolds.2.2 hChk).1 hNS
    case lea64 dst addr =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hEa : evalMemAddr s.gpr addr with
        | none => simp [execAmd64VecInsn, hEa] at hRun'
        | some ea =>
          by_cases hd4 : dst = 4#4
          · subst hd4; simp at hStep
          · simp only [beq_false_of_ne hd4, Bool.false_eq_true, ↓reduceIte] at hStep
            by_cases hd5 : (checkRbp && dst == 5#4 && !slot.rbpSaved) = true
            · simp [hd5] at hStep
            · simp only [hd5, Bool.false_eq_true, ↓reduceIte] at hStep
              refine ⟨?_, by simpa [execAmd64VecInsn, hEa] using hFall hF hSuccs.symm⟩
              by_cases hd2 : dst = 2#4
              · subst hd2
                simp only [beq_self_eq_true, ↓reduceIte] at hStep
                have h5ne2 : (5#4 : BitVec 4) ≠ 2#4 := by decide
                cases addr with
                | baseIdxDisp bOpt iOpt disp =>
                  cases bOpt with
                  | some bReg =>
                    by_cases hb4 : bReg = 4#4
                    · subst hb4
                      cases iOpt with
                      | none =>
                        simp only [Option.some.injEq] at hStep
                        subst hStep
                        simp only [evalMemAddr, Option.some.injEq] at hEa
                        refine ⟨by simp [execAmd64VecInsn, evalMemAddr, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1],
                          ?_, ?_⟩
                        · intro bd hBd
                          simp only [Option.some.injEq] at hBd; subst hBd
                          simp only [execAmd64VecInsn, evalMemAddr, setGpr, beq_self_eq_true, ↓reduceIte, hHolds.1]
                          bv_omega
                        · intro hChk
                          exact ⟨fun hNS => by simp [execAmd64VecInsn, evalMemAddr, setGpr, beq_false_of_ne h5ne2, (hHolds.2.2 hChk).1 hNS],
                                 fun hS => by simp [execAmd64VecInsn, evalMemAddr, (hHolds.2.2 hChk).2 hS]⟩
                      | some _ =>
                        simp only [Option.some.injEq] at hStep; subst hStep
                        refine ⟨by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1],
                          fun _ h => by simp at h, fun hChk =>
                          ⟨fun hNS => by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne h5ne2, (hHolds.2.2 hChk).1 hNS],
                           fun hS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).2 hS]⟩⟩
                    · have hStep' : some { slot with rdxDelta := none } = some slot' := by
                        revert hStep; split <;> simp_all
                      cases hStep'
                      refine ⟨by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1],
                        fun _ h => by simp at h, fun hChk =>
                        ⟨fun hNS => by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne h5ne2, (hHolds.2.2 hChk).1 hNS],
                         fun hS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).2 hS]⟩⟩
                  | none =>
                    simp only [Option.some.injEq] at hStep; subst hStep
                    refine ⟨by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1],
                      fun _ h => by simp at h, fun hChk =>
                      ⟨fun hNS => by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne h5ne2, (hHolds.2.2 hChk).1 hNS],
                       fun hS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).2 hS]⟩⟩
                | ripRel _ => simp [evalMemAddr] at hEa
              · simp only [beq_false_of_ne hd2, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
                subst hStep
                refine ⟨by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1],
                  fun bd hBd => by simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne (Ne.symm hd2), hHolds.2.1 bd hBd],
                  fun hChk => ⟨fun hNS => by
                    have h5neD : (5#4 : BitVec 4) ≠ dst := by intro hEq; subst hEq; simp [hChk, hNS] at hd5
                    simp [execAmd64VecInsn, hEa, setGpr, beq_false_of_ne h5neD, (hHolds.2.2 hChk).1 hNS],
                    fun hS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).2 hS]⟩⟩
    case movRR64 dst _ | movImm32 dst _ | movImm64 dst _ | xorRR32 dst _
      | inc64 dst | vmovmskpd _ dst _ | kmovbToGpr dst _ =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        by_cases hd4 : dst = 4#4
        · subst hd4; simp at hStep
        · simp only [beq_false_of_ne hd4, Bool.false_eq_true, ↓reduceIte] at hStep
          by_cases hd5 : (checkRbp && dst == 5#4 && !slot.rbpSaved) = true
          · simp [hd5] at hStep
          · simp only [hd5, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨⟨by simp [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1], ?_, ?_⟩,
              hFall hF hSuccs.symm⟩
            · intro bd hBd
              by_cases hd2 : dst = 2#4
              · subst hd2; simp at hBd
              · simp only [beq_false_of_ne hd2, Bool.false_eq_true, ↓reduceIte] at hBd
                simp [execAmd64VecInsn, setGpr, beq_false_of_ne (Ne.symm hd2), hHolds.2.1 bd hBd]
            · intro hChk
              refine ⟨fun hNS => ?_, fun hS => (hHolds.2.2 hChk).2 hS⟩
              have h5neD : (5#4 : BitVec 4) ≠ dst := by intro hEq; subst hEq; dsimp only at hNS; simp [hChk, hNS] at hd5
              simp [execAmd64VecInsn, setGpr, beq_false_of_ne h5neD, (hHolds.2.2 hChk).1 hNS]
    case movRM64 dst addr =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hRd : readMem64 rodata s.pc s.gpr s.mem addr with
        | none => simp [execAmd64VecInsn, hRd] at hRun'
        | some v =>
          by_cases hd4 : dst = 4#4
          · subst hd4; simp at hStep
          · simp only [beq_false_of_ne hd4, Bool.false_eq_true, ↓reduceIte] at hStep
            by_cases hd5 : (checkRbp && dst == 5#4 && !slot.rbpSaved) = true
            · simp [hd5] at hStep
            · simp only [hd5, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
              subst hStep
              refine ⟨⟨by simp [execAmd64VecInsn, hRd, setGpr, beq_false_of_ne (Ne.symm hd4), hHolds.1], ?_, ?_⟩,
                by simpa [execAmd64VecInsn, hRd] using hFall hF hSuccs.symm⟩
              · intro bd hBd
                by_cases hd2 : dst = 2#4
                · subst hd2; simp at hBd
                · simp only [beq_false_of_ne hd2, Bool.false_eq_true, ↓reduceIte] at hBd
                  simp [execAmd64VecInsn, hRd, setGpr, beq_false_of_ne (Ne.symm hd2), hHolds.2.1 bd hBd]
              · intro hChk
                refine ⟨fun hNS => ?_, fun hS => by simp [execAmd64VecInsn, hRd, (hHolds.2.2 hChk).2 hS]⟩
                have h5neD : (5#4 : BitVec 4) ≠ dst := by intro hEq; subst hEq; dsimp only at hNS; simp [hChk, hNS] at hd5
                simp [execAmd64VecInsn, hRd, setGpr, beq_false_of_ne h5neD, (hHolds.2.2 hChk).1 hNS]
    case movMR64 addr src =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hEa : evalMemAddr s.gpr addr with
        | none => simp [execAmd64VecInsn, hEa] at hRun'
        | some ea =>
          by_cases hBad : (checkRbp && !storeSafeFromSavedRbp slot addr 1) = true
          · simp [hBad] at hStep
          · simp only [hBad, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨⟨by simp [execAmd64VecInsn, hEa, hHolds.1],
              fun bd hBd => by simp [execAmd64VecInsn, hEa, hHolds.2.1 bd hBd], ?_⟩,
              by simpa [execAmd64VecInsn, hEa] using hFall hF hSuccs.symm⟩
            intro hChk
            refine ⟨fun hNS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).1 hNS], fun hS => ?_⟩
            have hSafe : storeSafeFromSavedRbp slot addr 1 = true := by
              cases hs : storeSafeFromSavedRbp slot addr 1
              · simp [hChk, hs] at hBad
              · rfl
            have hNe0 := storeSafeFromSavedRbp_ea_ne (hChk ▸ hHolds) hS hSafe hEa 0 (by omega)
            have hAddrNe : s0.gpr 4#4 - 8#64 ≠ ea := by
              have h0 : ea + 0#64 = ea := BitVec.add_zero ea
              simpa [h0] using Ne.symm hNe0
            simp only [execAmd64VecInsn, hEa, write64, beq_false_of_ne hAddrNe, Bool.false_eq_true, ↓reduceIte]
            exact (hHolds.2.2 hChk).2 hS
    case sseMovupsStore addr src =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hEa : evalMemAddr s.gpr addr with
        | none => simp [execAmd64VecInsn, hEa] at hRun'
        | some ea =>
          by_cases hBad : (checkRbp && !storeSafeFromSavedRbp slot addr 2) = true
          · simp [hBad] at hStep
          · simp only [hBad, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨⟨by simp [execAmd64VecInsn, hEa, hHolds.1],
              fun bd hBd => by simp [execAmd64VecInsn, hEa, hHolds.2.1 bd hBd], ?_⟩,
              by simpa [execAmd64VecInsn, hEa] using hFall hF hSuccs.symm⟩
            intro hChk
            refine ⟨fun hNS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).1 hNS], fun hS => ?_⟩
            have hSafe : storeSafeFromSavedRbp slot addr 2 = true := by
              cases hs : storeSafeFromSavedRbp slot addr 2
              · simp [hChk, hs] at hBad
              · rfl
            have hNe0 := storeSafeFromSavedRbp_ea_ne (hChk ▸ hHolds) hS hSafe hEa 0 (by omega)
            have hNe1 := storeSafeFromSavedRbp_ea_ne (hChk ▸ hHolds) hS hSafe hEa 1 (by omega)
            have hA0 : s0.gpr 4#4 - 8#64 ≠ ea := by simpa [BitVec.add_zero] using Ne.symm hNe0
            have hA1 : s0.gpr 4#4 - 8#64 ≠ ea + 8#64 := Ne.symm hNe1
            simp only [execAmd64VecInsn, hEa, write64, beq_false_of_ne hA1, beq_false_of_ne hA0,
              Bool.false_eq_true, ↓reduceIte]
            exact (hHolds.2.2 hChk).2 hS
    case vmovdquStore _ w addr src mask =>
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hEa : evalMemAddr s.gpr addr with
        | none => simp [execAmd64VecInsn, hEa] at hRun'
        | some ea =>
          by_cases hBad : (checkRbp && !storeSafeFromSavedRbp slot addr (vecLanes w)) = true
          · simp [hBad] at hStep
          · simp only [hBad, Bool.false_eq_true, ↓reduceIte, Option.some.injEq] at hStep
            subst hStep
            refine ⟨⟨by simp [execAmd64VecInsn, hEa, hHolds.1],
              fun bd hBd => by simp [execAmd64VecInsn, hEa, hHolds.2.1 bd hBd], ?_⟩,
              by simpa [execAmd64VecInsn, hEa] using hFall hF hSuccs.symm⟩
            intro hChk
            refine ⟨fun hNS => by simp [execAmd64VecInsn, hEa, (hHolds.2.2 hChk).1 hNS], fun hS => ?_⟩
            have hSafe : storeSafeFromSavedRbp slot addr (vecLanes w) = true := by
              cases hs : storeSafeFromSavedRbp slot addr (vecLanes w)
              · simp [hChk, hs] at hBad
              · rfl
            have hNeAll := storeSafeFromSavedRbp_ea_ne (hChk ▸ hHolds) hS hSafe hEa
            simp only [execAmd64VecInsn, hEa]
            rw [write64_storeVecMem_preserves_ne hNeAll]
            exact (hHolds.2.2 hChk).2 hS
    case nop | testRR64 _ _ | cmpRR64 _ _ | cmpImm64 _ _ | cmpImm8 _ _
      | sseMovupsRR _ _ | sseMovqFromGpr _ _ | sseXorpsRR _ _ | ssePorRR _ _
      | sseMinsdRR _ _ | sseUcomisdRR _ _ | vzeroupper | vmovsdRR _ _ _
      | vmovqFromGpr _ _ | vmovdquRR _ _ _ _ _ | vpmovm2q _ _ _ | vpmovq2m _ _ _ =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        exact ⟨by simpa [execAmd64VecInsn, FrameSlotHolds] using hHolds, hFall hF hSuccs.symm⟩
    case cmpRM64 _ addr | sseMovsdLoad _ addr | vmovsdLoad _ addr =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hRd : readMem64 rodata s.pc s.gpr s.mem addr with
        | none => simp [execAmd64VecInsn, hRd] at hRun'
        | some v =>
          exact ⟨by simpa [execAmd64VecInsn, hRd, FrameSlotHolds] using hHolds,
            by simpa [execAmd64VecInsn, hRd] using hFall hF hSuccs.symm⟩
    case sseMovupsLoad _ addr | vmovdquLoad _ _ _ addr _ =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hEa : evalMemAddr s.gpr addr with
        | none => simp [execAmd64VecInsn, hEa] at hRun'
        | some ea =>
          exact ⟨by simpa [execAmd64VecInsn, hEa, FrameSlotHolds] using hHolds,
            by simpa [execAmd64VecInsn, hEa] using hFall hF hSuccs.symm⟩
    case vbroadcastsd _ _ _ src _ | vpbroadcastq _ _ _ src _ =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases src with
        | reg _ =>
          exact ⟨by simpa [execAmd64VecInsn, FrameSlotHolds] using hHolds, hFall hF hSuccs.symm⟩
        | mem addr =>
          cases hRd : readMem64 rodata s.pc s.gpr s.mem addr with
          | none => simp [execAmd64VecInsn, hRd] at hRun'
          | some v =>
            exact ⟨by simpa [execAmd64VecInsn, hRd, FrameSlotHolds] using hHolds,
              by simpa [execAmd64VecInsn, hRd] using hFall hF hSuccs.symm⟩
    case vpand _ w _ _ src2 mask | vsubpd _ w _ _ src2 mask | vminpd _ w _ _ src2 mask
      | vcmppdEvex w _ _ src2 _ mask | vpblendmq w _ _ src2 mask =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hRm : evalVecRm s w src2 mask.broadcast with
        | none => simp [execAmd64VecInsn, hRm] at hRun'
        | some v2 =>
          exact ⟨by simpa [execAmd64VecInsn, hRm, FrameSlotHolds] using hHolds,
            by simpa [execAmd64VecInsn, hRm] using hFall hF hSuccs.symm⟩
    case vcmppdVex w _ _ src2 _ | vpblendvb w _ _ src2 _ =>
      cases hStep
      cases hF : findInsnIdxByOff insns insns[idx].nextOffset with
      | none => simp [hF] at hSuccs
      | some f =>
        simp only [hF, Option.some.injEq] at hSuccs
        cases hRm : evalVecRm s w src2 false with
        | none => simp [execAmd64VecInsn, hRm] at hRun'
        | some v2 =>
          exact ⟨by simpa [execAmd64VecInsn, hRm, FrameSlotHolds] using hHolds,
            by simpa [execAmd64VecInsn, hRm] using hFall hF hSuccs.symm⟩

/-- Soundness theorem for static Go amd64 ABI register/frame preservation (`RSP`, `RBP`, `R14`, `X15`). -/
theorem checkDecodedFrameAbi_sound (checkRbp : Bool) (bytes : Array UInt8)
    (rodata : Nat → Option (BitVec 64)) (extTargets : Nat → Option String)
    (hCert : checkDecodedFrameAbi checkRbp bytes = true)
    (s0 : Amd64VecState) (fuel : Nat) (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel bytes rodata extTargets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned →
      sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
      (checkRbp = true → sf.gpr 5#4 = s0.gpr 5#4)) := by
  unfold checkDecodedFrameAbi at hCert
  cases hProg : decodeAmd64Program bytes with
  | none => simp [hProg] at hCert
  | some insns =>
    simp only [hProg] at hCert
    split at hCert
    · next hPos =>
      cases hComp : computeFrameTableLoop checkRbp insns (4 * insns.size + 2)
          ((Array.replicate insns.size none).set 0
            (some { spDelta := 0#64, rbpSaved := false, rdxDelta := none }) (by simp [hPos])) [0] with
      | none => simp [hComp] at hCert
      | some tbl =>
        simp only [hComp] at hCert
        unfold verifyFrameTable at hCert
        simp only [hPos, ↓reduceDIte, Bool.and_eq_true, beq_iff_eq] at hCert
        rcases hCert with ⟨⟨⟨_, hOff0⟩, hTbl0⟩, hGo⟩
        suffices hLoop : ∀ (k : Nat) (s : Amd64VecState),
            s.gpr 14#4 = s0.gpr 14#4 →
            (s.zmm 15#5).l0 = (s0.zmm 15#5).l0 →
            (s.zmm 15#5).l1 = (s0.zmm 15#5).l1 →
            (s.status = .returned →
              s.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
              (checkRbp = true → s.gpr 5#4 = s0.gpr 5#4)) →
            (s.status = .running →
              ∃ idx, ∃ (_ : idx < insns.size), ∃ slot,
                s.pc = insns[idx].offset ∧ tbl[idx]? = some (some slot) ∧
                FrameSlotHolds checkRbp s0 s slot) →
            let sf := runAmd64Vec k bytes rodata extTargets s
            sf.gpr 14#4 = s0.gpr 14#4 ∧
            (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
            (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
            (sf.status = .returned →
              sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
              (checkRbp = true → sf.gpr 5#4 = s0.gpr 5#4)) by
          apply hLoop fuel s0 rfl rfl rfl (fun h => by rw [hRun] at h; cases h)
          intro _
          refine ⟨0, hPos, { spDelta := 0#64, rbpSaved := false, rdxDelta := none },
            by rw [hPc, hOff0], hTbl0, ?_⟩
          refine ⟨by bv_omega, fun _ h => by simp at h,
            fun _ => ⟨fun _ => rfl, fun h => Bool.noConfusion h⟩⟩
        intro k
        induction k with
        | zero =>
          intro s h14 hX0 hX1 hRet _
          exact ⟨h14, hX0, hX1, hRet⟩
        | succ k ih =>
          intro s h14 hX0 hX1 hRet hReach
          by_cases hR : s.status = .running
          · have hB : (s.status == .running) = true := beq_iff_eq.mpr hR
            dsimp only
            rw [runAmd64Vec]
            simp only [hB, ↓reduceIte]
            rcases hReach hR with ⟨idx, hIdx, slot, hPcEq, hSlotEq, hHolds⟩
            have hStepDec := decodeAmd64Program_elem_decodes bytes insns hProg idx hIdx
            rw [← hPcEq] at hStepDec
            rw [stepAmd64Vec_at bytes rodata extTargets s insns[idx].insn insns[idx].nextOffset hR hStepDec]
            rcases verifyFrameTableGo_sound checkRbp insns tbl insns.size 0 idx slot hGo (Nat.zero_le idx) (by omega) hIdx hSlotEq with
              ⟨slot', succs, hFStep, hFSuccs, hAllSuccs⟩
            have hSt := execAmd64VecInsn_frameAbi_step checkRbp insns rodata extTargets idx hIdx
              s0 s slot slot' succs hR hHolds hFStep hFSuccs
            apply ih _ (hSt.1.trans h14) (hSt.2.1.trans hX0) (hSt.2.2.1.trans hX1) hSt.2.2.2.1
            intro hRun'
            rcases hSt.2.2.2.2 hRun' with ⟨hHolds', nxt, hMemNxt, hNxtLt, hPcNxt⟩
            exact ⟨nxt, hNxtLt, slot', hPcNxt, hAllSuccs nxt hMemNxt, hHolds'⟩
          · have hB : (s.status == .running) = false := beq_false_of_ne hR
            dsimp only
            rw [runAmd64Vec]
            simp only [hB, Bool.false_eq_true, ↓reduceIte]
            exact ⟨h14, hX0, hX1, hRet⟩
    · simp at hCert

set_option maxRecDepth 4096 in
private theorem allAbsLE64AVX2_frame_cert :
    checkDecodedFrameAbi true allAbsLE64AVX2Bytes = true :=
  checkDecodedFrameAbi_of_chunks true allAbsLE64AVX2ByteChunks (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem allAbsLE64AVX512_frame_cert :
    checkDecodedFrameAbi true allAbsLE64AVX512Bytes = true :=
  checkDecodedFrameAbi_of_chunks true allAbsLE64AVX512ByteChunks (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem minNoNaN64AVX2_frame_cert :
    checkDecodedFrameAbi true minNoNaN64AVX2Bytes = true :=
  checkDecodedFrameAbi_of_chunks true minNoNaN64AVX2ByteChunks (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem minNoNaN64AVX512_frame_cert :
    checkDecodedFrameAbi true minNoNaN64AVX512Bytes = true :=
  checkDecodedFrameAbi_of_chunks true minNoNaN64AVX512ByteChunks (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_frame_cert :
    checkDecodedFrameAbi false initReducedRow64AVX2Bytes = true :=
  checkDecodedFrameAbi_of_chunks false initReducedRow64AVX2ByteChunks (by decide) (by decide)

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_frame_cert :
    checkDecodedFrameAbi false initReducedRow64AVX512Bytes = true :=
  checkDecodedFrameAbi_of_chunks false initReducedRow64AVX512ByteChunks (by decide) (by decide)

/-- Universal Go ABI register preservation (`RSP`, `RBP`, `R14`, `X15`) for decoded `allAbsLE64AVX2`. -/
theorem allAbsLE64AVX2_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧ sf.gpr 5#4 = s0.gpr 5#4) := by
  have h := checkDecodedFrameAbi_sound true allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets allAbsLE64AVX2_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => ⟨(h.2.2.2 hR).1, (h.2.2.2 hR).2 rfl⟩⟩

/-- Universal Go ABI register preservation (`RSP`, `RBP`, `R14`, `X15`) for decoded `allAbsLE64AVX512`. -/
theorem allAbsLE64AVX512_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧ sf.gpr 5#4 = s0.gpr 5#4) := by
  have h := checkDecodedFrameAbi_sound true allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets allAbsLE64AVX512_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => ⟨(h.2.2.2 hR).1, (h.2.2.2 hR).2 rfl⟩⟩

/-- Universal Go ABI register preservation (`RSP`, `RBP`, `R14`, `X15`) for decoded `minNoNaN64AVX2`. -/
theorem minNoNaN64AVX2_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧ sf.gpr 5#4 = s0.gpr 5#4) := by
  have h := checkDecodedFrameAbi_sound true minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets minNoNaN64AVX2_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => ⟨(h.2.2.2 hR).1, (h.2.2.2 hR).2 rfl⟩⟩

/-- Universal Go ABI register preservation (`RSP`, `RBP`, `R14`, `X15`) for decoded `minNoNaN64AVX512`. -/
theorem minNoNaN64AVX512_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧ sf.gpr 5#4 = s0.gpr 5#4) := by
  have h := checkDecodedFrameAbi_sound true minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets minNoNaN64AVX512_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => ⟨(h.2.2.2 hR).1, (h.2.2.2 hR).2 rfl⟩⟩

/-- Universal Go ABI register preservation (`RSP`, `R14`, `X15`) for decoded `initReducedRow64AVX2`. -/
theorem initReducedRow64AVX2_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64) := by
  have h := checkDecodedFrameAbi_sound false initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets initReducedRow64AVX2_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => (h.2.2.2 hR).1⟩

/-- Universal Go ABI register preservation (`RSP`, `R14`, `X15`) for decoded `initReducedRow64AVX512`. -/
theorem initReducedRow64AVX512_abi_reg_preserved (s0 : Amd64VecState) (fuel : Nat)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) :
    let sf := runAmd64Vec fuel initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (sf.status = .returned → sf.gpr 4#4 = s0.gpr 4#4 + 8#64) := by
  have h := checkDecodedFrameAbi_sound false initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets initReducedRow64AVX512_frame_cert s0 fuel hPc hRun
  exact ⟨h.1, h.2.1, h.2.2.1, fun hR => (h.2.2.2 hR).1⟩

/-- Adversarial negative control: stripping `VZEROUPPER` (`C5 F8 77`) at offset `0x9d` from
`allAbsLE64AVX2` (replacing with 3 `NOP` bytes `90 90 90`) is immediately rejected. -/
private def replaceByteInChunks (chunks : List (List UInt8)) (pos : Nat) (val : UInt8) :
    List (List UInt8) :=
  chunks.set (pos / 16) ((chunks.getD (pos / 16) []).set (pos % 16) val)

/-- Mutated `allAbsLE64AVX2` byte chunks with `VZEROUPPER` (`0x9d..0x9f`) overwritten by three `NOP` (`0x90`) bytes. -/
def allAbsLE64AVX2StrippedVzuChunks : List (List UInt8) :=
  replaceByteInChunks (replaceByteInChunks (replaceByteInChunks allAbsLE64AVX2ByteChunks 0x9d 0x90) 0x9e 0x90) 0x9f 0x90

/-- Flattened byte array for the `VZEROUPPER`-stripped `allAbsLE64AVX2` negative-control fixture. -/
def allAbsLE64AVX2StrippedVzuBytes : Array UInt8 :=
  ⟨allAbsLE64AVX2StrippedVzuChunks.flatten⟩

set_option maxRecDepth 4096 in
theorem stripped_vzeroupper_rejected :
    checkDecodedAbiSafety allAbsLE64AVX2StrippedVzuBytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets = false := by
  unfold checkDecodedAbiSafety allAbsLE64AVX2StrippedVzuBytes
  rw [decodeAmd64ProgramChunks_eq allAbsLE64AVX2StrippedVzuChunks (by decide)]
  decide

/-!
### Part V: IEEE-754 SIMD Algebra, x86 Flag Lemmas & `Track5D` Bridge
-/

/-- Slice element address `base + j * 8` in 64-bit address arithmetic. -/
@[inline] def elemAddr (base : BitVec 64) (j : Nat) : BitVec 64 :=
  base + BitVec.ofNat 64 j * 8#64

/-- 64-bit all-1s/all-0s lane mask from a Boolean (`VCMPPD` / `VPMOVM2Q`). -/
@[inline] def maskLane64 (b : Bool) : BitVec 64 :=
  if b then 0xffffffffffffffff#64 else 0#64

/-- 4-lane YMM Boolean mask vector (`allOK0`/`allOK1` in AVX2). -/
@[inline] def maskVec4 (b0 b1 b2 b3 : Bool) : Vec512 :=
  { l0 := maskLane64 b0, l1 := maskLane64 b1, l2 := maskLane64 b2, l3 := maskLane64 b3 }

/-- 8-lane ZMM Boolean mask vector (`allOK0`/`allOK1` in AVX-512). -/
@[inline] def maskVec8 (b0 b1 b2 b3 b4 b5 b6 b7 : Bool) : Vec512 :=
  { l0 := maskLane64 b0, l1 := maskLane64 b1, l2 := maskLane64 b2, l3 := maskLane64 b3
    l4 := maskLane64 b4, l5 := maskLane64 b5, l6 := maskLane64 b6, l7 := maskLane64 b7 }

theorem setGpr_eq (g : BitVec 4 → BitVec 64) (r : BitVec 4) (v : BitVec 64) :
    setGpr g r v r = v := by simp [setGpr]

theorem setGpr_ne (g : BitVec 4 → BitVec 64) {r r' : BitVec 4} (v : BitVec 64) (h : r' ≠ r) :
    setGpr g r v r' = g r' := by simp [setGpr, beq_false_of_ne h]

theorem setZmm_eq (z : BitVec 5 → Vec512) (r : BitVec 5) (v : Vec512) :
    setZmm z r v r = v := by simp [setZmm]

theorem setZmm_ne (z : BitVec 5 → Vec512) {r r' : BitVec 5} (v : Vec512) (h : r' ≠ r) :
    setZmm z r v r' = z r' := by simp [setZmm, beq_false_of_ne h]

theorem setKmask_eq (k : BitVec 3 → BitVec 8) (r : BitVec 3) (v : BitVec 8) :
    setKmask k r v r = v := by simp [setKmask]

theorem setKmask_ne (k : BitVec 3 → BitVec 8) {r r' : BitVec 3} (v : BitVec 8) (h : r' ≠ r) :
    setKmask k r v r' = k r' := by simp [setKmask, beq_false_of_ne h]

theorem write64_eq (m : BitVec 64 → BitVec 64) (a v : BitVec 64) :
    write64 m a v a = v := by simp [write64]

theorem write64_ne (m : BitVec 64 → BitVec 64) {a b : BitVec 64} (v : BitVec 64) (h : b ≠ a) :
    write64 m a v b = m b := by simp [write64, beq_false_of_ne h]

theorem bitVec_ofNat_add (a b : Nat) :
    BitVec.ofNat 64 a + BitVec.ofNat 64 b = BitVec.ofNat 64 (a + b) := by
  apply BitVec.eq_of_toNat_eq
  simp [BitVec.toNat_add, Nat.add_mod]

theorem elemAddr_add (base : BitVec 64) (j k : Nat) :
    elemAddr base j + BitVec.ofNat 64 (k * 8) = elemAddr base (j + k) := by
  unfold elemAddr
  have hMul : BitVec.ofNat 64 (k * 8) = BitVec.ofNat 64 k * 8#64 := by
    apply BitVec.eq_of_toNat_eq
    simp [BitVec.toNat_mul, Nat.mul_mod]
  have hAdd : BitVec.ofNat 64 (j + k) = BitVec.ofNat 64 j + BitVec.ofNat 64 k :=
    (bitVec_ofNat_add j k).symm
  rw [hMul, hAdd, BitVec.add_mul, BitVec.add_assoc]

theorem elemAddr_plus_0 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 0#64 = elemAddr base j := by simp

theorem elemAddr_plus_8 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 8#64 = elemAddr base (j + 1) := elemAddr_add base j 1

theorem elemAddr_plus_16 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 16#64 = elemAddr base (j + 2) := elemAddr_add base j 2

theorem elemAddr_plus_24 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 24#64 = elemAddr base (j + 3) := elemAddr_add base j 3

theorem elemAddr_plus_32 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 32#64 = elemAddr base (j + 4) := elemAddr_add base j 4

theorem elemAddr_plus_40 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 40#64 = elemAddr base (j + 5) := elemAddr_add base j 5

theorem elemAddr_plus_48 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 48#64 = elemAddr base (j + 6) := elemAddr_add base j 6

theorem elemAddr_plus_56 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 56#64 = elemAddr base (j + 7) := elemAddr_add base j 7

theorem elemAddr_plus_64 (base : BitVec 64) (j : Nat) :
    elemAddr base j + 64#64 = elemAddr base (j + 8) := elemAddr_add base j 8

/-! #### x86-64 Flag & Branch Evaluation Lemmas -/

private theorem x86SubFlags64_ge_bv (a b : BitVec 64) (ha : a.msb = false) (hb : b.msb = false) :
    evalX86Cond .ge (x86SubFlags64 a b) = BitVec.ule b a := by
  unfold evalX86Cond x86SubFlags64
  simp only [ha, hb, bne_self_eq_false, Bool.false_and, beq_false]
  rw [BitVec.msb_eq_decide] at ha hb
  rw [BitVec.msb_eq_decide, BitVec.ule_eq_decide]
  simp only [decide_eq_false_iff_not, BitVec.toNat_sub] at ha hb ⊢
  apply Bool.eq_iff_iff.mpr
  simp only [Bool.not_eq_true', decide_eq_false_iff_not, decide_eq_true_eq]
  omega

private theorem x86SubFlags64_le_bv (a b : BitVec 64) (ha : a.msb = false) (hb : b.msb = false) :
    evalX86Cond .le (x86SubFlags64 a b) = BitVec.ule a b := by
  unfold evalX86Cond x86SubFlags64
  have hBneFalse : ∀ x : Bool, (x != false) = x := fun x => by cases x <;> rfl
  simp only [ha, hb, bne_self_eq_false, Bool.false_and, hBneFalse]
  rw [BitVec.msb_eq_decide] at ha hb
  rw [BitVec.msb_eq_decide, BitVec.ule_eq_decide]
  simp only [decide_eq_false_iff_not] at ha hb
  apply Bool.eq_iff_iff.mpr
  simp only [Bool.or_eq_true, beq_iff_eq, decide_eq_true_eq]
  constructor
  · rintro (hEq | hMsb)
    · have hNat := congrArg BitVec.toNat hEq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat] at hNat
      omega
    · simp only [BitVec.toNat_sub] at hMsb
      omega
  · intro hLe
    by_cases hEq : a.toNat = b.toNat
    · left
      apply BitVec.eq_of_toNat_eq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat]
      omega
    · right
      simp only [BitVec.toNat_sub]
      omega

private theorem x86SubFlags64_g_bv (a b : BitVec 64) (ha : a.msb = false) (hb : b.msb = false) :
    evalX86Cond .g (x86SubFlags64 a b) = BitVec.ult b a := by
  unfold evalX86Cond x86SubFlags64
  simp only [ha, hb, bne_self_eq_false, Bool.false_and, beq_false]
  rw [BitVec.msb_eq_decide] at ha hb
  rw [BitVec.msb_eq_decide, BitVec.ult_eq_decide]
  simp only [decide_eq_false_iff_not] at ha hb
  apply Bool.eq_iff_iff.mpr
  simp only [Bool.and_eq_true, Bool.not_eq_true', beq_eq_false_iff_ne, decide_eq_false_iff_not,
    decide_eq_true_eq]
  constructor
  · rintro ⟨hNe, hNotMsb⟩
    simp only [BitVec.toNat_sub] at hNotMsb
    have hNeNat : a.toNat ≠ b.toNat := by
      intro hEq
      apply hNe
      apply BitVec.eq_of_toNat_eq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat]
      omega
    omega
  · intro hLt
    refine ⟨?_, ?_⟩
    · intro hEq
      have hNat := congrArg BitVec.toNat hEq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat] at hNat
      omega
    · simp only [BitVec.toNat_sub]
      omega

private theorem bitVec_ofNat_msb_false {n : Nat} (h : n < 2 ^ 63) :
    (BitVec.ofNat 64 n).msb = false := by
  rw [BitVec.msb_eq_decide]
  simp only [BitVec.toNat_ofNat, decide_eq_false_iff_not]
  have hMod : n % 2 ^ 64 = n := Nat.mod_eq_of_lt (by omega)
  omega

theorem evalX86Cond_ge_sub64 {a b : Nat} (ha : a < 2 ^ 63) (hb : b < 2 ^ 63) :
    evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 a) (BitVec.ofNat 64 b)) = decide (b ≤ a) := by
  rw [x86SubFlags64_ge_bv _ _ (bitVec_ofNat_msb_false ha) (bitVec_ofNat_msb_false hb),
      BitVec.ule_eq_decide]
  simp only [BitVec.toNat_ofNat]
  rw [Nat.mod_eq_of_lt (by omega), Nat.mod_eq_of_lt (by omega)]

theorem evalX86Cond_le_sub64 {a b : Nat} (ha : a < 2 ^ 63) (hb : b < 2 ^ 63) :
    evalX86Cond .le (x86SubFlags64 (BitVec.ofNat 64 a) (BitVec.ofNat 64 b)) = decide (a ≤ b) := by
  rw [x86SubFlags64_le_bv _ _ (bitVec_ofNat_msb_false ha) (bitVec_ofNat_msb_false hb),
      BitVec.ule_eq_decide]
  simp only [BitVec.toNat_ofNat]
  rw [Nat.mod_eq_of_lt (by omega), Nat.mod_eq_of_lt (by omega)]

theorem evalX86Cond_g_sub64 {a b : Nat} (ha : a < 2 ^ 63) (hb : b < 2 ^ 63) :
    evalX86Cond .g (x86SubFlags64 (BitVec.ofNat 64 a) (BitVec.ofNat 64 b)) = decide (b < a) := by
  rw [x86SubFlags64_g_bv _ _ (bitVec_ofNat_msb_false ha) (bitVec_ofNat_msb_false hb),
      BitVec.ult_eq_decide]
  simp only [BitVec.toNat_ofNat]
  rw [Nat.mod_eq_of_lt (by omega), Nat.mod_eq_of_lt (by omega)]

theorem evalX86Cond_be_test64_self {n : Nat} (hPos : 0 < n) (hLt : n < 2 ^ 64) :
    evalX86Cond .be (x86TestFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = false := by
  unfold evalX86Cond x86TestFlags64
  simp only [BitVec.and_self, Bool.false_or]
  apply beq_false_of_ne
  intro hEq
  have hNat := congrArg BitVec.toNat hEq
  simp only [BitVec.toNat_ofNat, Nat.mod_eq_of_lt hLt] at hNat
  omega

theorem evalX86Cond_be_sub64_unsigned (a b : BitVec 64) :
    evalX86Cond .be (x86SubFlags64 a b) = BitVec.ule a b := by
  unfold evalX86Cond x86SubFlags64
  rw [BitVec.ult_eq_decide, BitVec.ule_eq_decide]
  apply Bool.eq_iff_iff.mpr
  simp only [Bool.or_eq_true, decide_eq_true_eq, beq_iff_eq]
  constructor
  · rintro (hLt | hEq)
    · omega
    · have hNat := congrArg BitVec.toNat hEq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat] at hNat
      omega
  · intro hLe
    by_cases hLt : a.toNat < b.toNat
    · exact Or.inl hLt
    · right
      apply BitVec.eq_of_toNat_eq
      simp only [BitVec.toNat_sub, BitVec.toNat_ofNat]
      omega

theorem evalX86Cond_e_sub8 (a b : BitVec 8) :
    evalX86Cond .e (x86SubFlags8 a b) = (a == b) := by
  unfold evalX86Cond x86SubFlags8
  apply Bool.eq_iff_iff.mpr
  simp only [beq_iff_eq]
  constructor
  · intro h
    apply BitVec.eq_of_toNat_eq
    have hNat := congrArg BitVec.toNat h
    simp only [BitVec.toNat_sub, BitVec.toNat_ofNat] at hNat
    omega
  · intro h; subst h; simp

/-! #### IEEE-754 SIMD Lane Algebra & `Track5D` Bridge Theorems -/

private theorem f64IsZero_false_of_isNaN {x : BitVec 64} (h : f64IsNaN x = true) :
    f64IsZero x = false := by
  cases hz : f64IsZero x
  · rfl
  · rcases (f64IsZero_iff x).mp hz with rfl | rfl <;> simp [f64IsNaN] at h

private theorem canonZero64_notNaN {x : BitVec 64} (h : f64NotNaN x = true) :
    f64NotNaN (canonZero64 x) = true := by
  unfold canonZero64
  split
  · decide
  · exact h

private theorem canonZero64_idem (x : BitVec 64) :
    canonZero64 (canonZero64 x) = canonZero64 x := by
  unfold canonZero64
  cases hz : f64IsZero x
  · simp [hz]
  · simp [f64PosZero, f64IsZero]

private theorem f64NumKey_canonZero64 (x : BitVec 64) :
    f64NumKey (canonZero64 x) = f64NumKey x := by
  unfold f64NumKey
  rw [canonZero64_idem]

private theorem canonZero64_eq_of_numKey_eq {a b : BitVec 64}
    (h : f64NumKey a = f64NumKey b) : canonZero64 a = canonZero64 b := by
  have hB : (f64NumKey a == f64NumKey b) = true := beq_iff_eq.mpr h
  rw [f64NumKey_eq_iff] at hB
  simp only [Bool.or_eq_true, beq_iff_eq, Bool.and_eq_true] at hB
  rcases hB with rfl | ⟨ha, hb⟩
  · rfl
  · simp [canonZero64, ha, hb]

theorem f64Eq_self_eq_notNaN (x : BitVec 64) : f64Eq x x = !f64IsNaN x := by
  unfold f64Eq
  cases f64IsNaN x <;> simp

theorem maskLane64_and (a b : Bool) :
    maskLane64 a &&& maskLane64 b = maskLane64 (a && b) := by
  cases a <;> cases b <;> rfl

theorem maskLane64_msb (b : Bool) : (maskLane64 b).msb = b := by
  cases b <;> decide

theorem movmskpd4_maskVec4_eq_0xf (b0 b1 b2 b3 : Bool) :
    ((maskVec4 b0 b1 b2 b3).movmskpd4.extractLsb' 0 8 == 0x0f#8) = (b0 && b1 && b2 && b3) := by
  cases b0 <;> cases b1 <;> cases b2 <;> cases b3 <;> decide

theorem toOpmask8_maskVec8_eq_0xff (b0 b1 b2 b3 b4 b5 b6 b7 : Bool) :
    (((maskVec8 b0 b1 b2 b3 b4 b5 b6 b7).toOpmask8.zeroExtend 64).extractLsb' 0 8 == 0xff#8) =
      (b0 && b1 && b2 && b3 && b4 && b5 && b6 && b7) := by
  cases b0 <;> cases b1 <;> cases b2 <;> cases b3 <;>
    cases b4 <;> cases b5 <;> cases b6 <;> cases b7 <;> decide

private theorem bitVec_0xff_getLsbD {k : Nat} (hk : k < 8) :
    (0xff#64 : BitVec 64).getLsbD k = true := by
  match k with
  | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 => decide
  | _ + 8 => omega

private theorem bitVec_getLsbD_byteSlice (x : BitVec 64) (sh i : Nat) (hi : i < 64) :
    ((((x >>> sh) &&& 0xff#64) <<< sh).getLsbD i) =
      (decide (sh ≤ i ∧ i < sh + 8) && x.getLsbD i) := by
  simp only [BitVec.getLsbD_shiftLeft, BitVec.getLsbD_and, BitVec.getLsbD_ushiftRight, hi,
    decide_true, Bool.true_and]
  by_cases hGe : sh ≤ i
  · have hNotLt : ¬(i < sh) := by omega
    simp only [hNotLt, decide_false, Bool.not_false, Bool.true_and, hGe, true_and]
    by_cases hLt8 : i - sh < 8
    · have hLt : i < sh + 8 := by omega
      have hMask : (0xff#64 : BitVec 64).getLsbD (i - sh) = true := bitVec_0xff_getLsbD hLt8
      have hAdd : sh + (i - sh) = i := by omega
      simp [hLt, hMask, hAdd]
    · have hNotLt8 : ¬(i < sh + 8) := by omega
      have hMask : (0xff#64 : BitVec 64).getLsbD (i - sh) = false := by
        rw [BitVec.getLsbD]
        simp only [BitVec.toNat_ofNat]
        have hMod : 0xff % 2 ^ 64 = 255 := by decide
        rw [hMod]
        exact Nat.testBit_lt_two_pow (by
          have hPow : 2 ^ 8 ≤ 2 ^ (i - sh) := Nat.pow_le_pow_right (by omega) (by omega)
          omega)
      simp [hNotLt8, hMask]
  · have hLt : i < sh := by omega
    simp [hLt, hGe]

private theorem reassemble_bytes64 (x : BitVec 64) :
    (((x >>> 0) &&& 0xff#64) <<< 0 ||| ((x >>> 8) &&& 0xff#64) <<< 8 |||
     ((x >>> 16) &&& 0xff#64) <<< 16 ||| ((x >>> 24) &&& 0xff#64) <<< 24 |||
     ((x >>> 32) &&& 0xff#64) <<< 32 ||| ((x >>> 40) &&& 0xff#64) <<< 40 |||
     ((x >>> 48) &&& 0xff#64) <<< 48 ||| ((x >>> 56) &&& 0xff#64) <<< 56) = x := by
  apply BitVec.eq_of_getLsbD_eq
  intro i hi
  simp only [BitVec.getLsbD_or, bitVec_getLsbD_byteSlice x _ i hi]
  by_cases h0 : i < 8
  · simp [show ¬(8 ≤ i) by omega, show ¬(16 ≤ i) by omega, show ¬(24 ≤ i) by omega,
          show ¬(32 ≤ i) by omega, show ¬(40 ≤ i) by omega, show ¬(48 ≤ i) by omega,
          show ¬(56 ≤ i) by omega, h0]
  · by_cases h1 : i < 16
    · simp [h0, h1, show 8 ≤ i by omega, show ¬(16 ≤ i) by omega, show ¬(24 ≤ i) by omega,
            show ¬(32 ≤ i) by omega, show ¬(40 ≤ i) by omega, show ¬(48 ≤ i) by omega,
            show ¬(56 ≤ i) by omega]
    · by_cases h2 : i < 24
      · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show 16 ≤ i by omega, h2,
              show ¬(24 ≤ i) by omega, show ¬(32 ≤ i) by omega, show ¬(40 ≤ i) by omega,
              show ¬(48 ≤ i) by omega, show ¬(56 ≤ i) by omega]
      · by_cases h3 : i < 32
        · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show ¬(i < 24) by omega,
                show 24 ≤ i by omega, h3, show ¬(32 ≤ i) by omega, show ¬(40 ≤ i) by omega,
                show ¬(48 ≤ i) by omega, show ¬(56 ≤ i) by omega]
        · by_cases h4 : i < 40
          · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show ¬(i < 24) by omega,
                  show ¬(i < 32) by omega, show 32 ≤ i by omega, h4, show ¬(40 ≤ i) by omega,
                  show ¬(48 ≤ i) by omega, show ¬(56 ≤ i) by omega]
          · by_cases h5 : i < 48
            · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show ¬(i < 24) by omega,
                    show ¬(i < 32) by omega, show ¬(i < 40) by omega, show 40 ≤ i by omega, h5,
                    show ¬(48 ≤ i) by omega, show ¬(56 ≤ i) by omega]
            · by_cases h6 : i < 56
              · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show ¬(i < 24) by omega,
                      show ¬(i < 32) by omega, show ¬(i < 40) by omega, show ¬(i < 48) by omega,
                      show 48 ≤ i by omega, h6, show ¬(56 ≤ i) by omega]
              · simp [show ¬(i < 8) by omega, show ¬(i < 16) by omega, show ¬(i < 24) by omega,
                      show ¬(i < 32) by omega, show ¬(i < 40) by omega, show ¬(i < 48) by omega,
                      show ¬(i < 56) by omega, show 56 ≤ i by omega, hi]

/-- `VPBLENDVB` lane identity when mask lane comes from `VCMPPD` (all-1s or all-0s). -/
theorem vpblendvb64_maskLane (src1 src2 : BitVec 64) (cond : Bool) :
    vpblendvb64 src1 src2 (maskLane64 cond) = if cond then src2 else src1 := by
  cases cond <;> unfold vpblendvb64 maskLane64 <;> dsimp only
  · exact reassemble_bytes64 src1
  · exact reassemble_bytes64 src2

/-- Hardware `UCOMISD` + `JA` matches strict `f64Lt v res` (skipping NaN and equal values). -/
theorem evalX86Cond_a_ucomisd (res v : BitVec 64) :
    evalX86Cond .a (x86UcomisdFlags res v) = f64Lt v res := by
  unfold evalX86Cond x86UcomisdFlags
  by_cases hNaN : (f64IsNaN res || f64IsNaN v) = true
  · simp only [hNaN, ↓reduceIte]
    have hNaNRev : (f64IsNaN v || f64IsNaN res) = true := by rw [Bool.or_comm]; exact hNaN
    unfold f64Lt
    simp [hNaNRev]
  · simp only [hNaN, Bool.false_eq_true, ↓reduceIte]
    simp only [Bool.or_eq_true, not_or, Bool.not_eq_true] at hNaN
    have hResOk : f64NotNaN res = true := by simp [f64NotNaN, hNaN.1]
    have hVOk : f64NotNaN v = true := by simp [f64NotNaN, hNaN.2]
    rw [f64Eq_iff_eq_numKey res v hResOk hVOk,
        f64Lt_iff_ult_numKey res v hResOk hVOk,
        f64Lt_iff_ult_numKey v res hVOk hResOk]
    by_cases hEq : (f64NumKey res == f64NumKey v) = true
    · simp only [hEq, ↓reduceIte, BitVec.ult_eq_decide]
      have hK : f64NumKey res = f64NumKey v := beq_iff_eq.mp hEq
      simp [hK]
    · simp only [hEq, Bool.false_eq_true, ↓reduceIte]
      have hNeNat : (f64NumKey res).toNat ≠ (f64NumKey v).toNat := by
        intro hN; apply hEq; exact beq_iff_eq.mpr (BitVec.eq_of_toNat_eq hN)
      by_cases hLt : BitVec.ult (f64NumKey res) (f64NumKey v) = true
      · have hLtN : (f64NumKey res).toNat < (f64NumKey v).toNat := by
          simpa [BitVec.ult_eq_decide] using hLt
        simp only [hLt, ↓reduceIte, BitVec.ult_eq_decide]
        simp [show ¬((f64NumKey v).toNat < (f64NumKey res).toNat) by omega]
      · have hNotLtN : ¬((f64NumKey res).toNat < (f64NumKey v).toNat) := by
          simpa [BitVec.ult_eq_decide] using hLt
        simp only [hLt, Bool.false_eq_true, ↓reduceIte, BitVec.ult_eq_decide]
        simp [show (f64NumKey v).toNat < (f64NumKey res).toNat by omega]

/-- Go 1.27's signed-zero canonicalization (`XORPS; UCOMISD; JNE; JP; XORPS`) equals `Track5D.canonZero64`. -/
theorem x86_ucomisd_zero_canon_eq_canonZero64 (res : BitVec 64) :
    (if evalX86Cond .ne (x86UcomisdFlags res 0#64) || evalX86Cond .p (x86UcomisdFlags res 0#64)
     then res else 0#64) = canonZero64 res := by
  unfold evalX86Cond x86UcomisdFlags canonZero64 f64PosZero
  by_cases hNaN : f64IsNaN res = true
  · have hNotZ : f64IsZero res = false := f64IsZero_false_of_isNaN hNaN
    simp [hNaN, hNotZ]
  · simp only [Bool.not_eq_true] at hNaN
    have hZ0 : f64IsNaN 0#64 = false := by decide
    have hZZero : f64IsZero 0#64 = true := by decide
    simp only [hNaN, hZ0, Bool.or_self, Bool.false_eq_true, ↓reduceIte]
    cases hz : f64IsZero res
    · have hNotEq : f64Eq res 0#64 = false := by
        unfold f64Eq
        have hNe0 : res ≠ 0#64 := by
          intro hEq; subst hEq; simp [f64IsZero] at hz
        simp [hNaN, hZ0, hz, beq_false_of_ne hNe0]
      simp only [hNotEq, Bool.false_eq_true, ↓reduceIte]
      by_cases hLt : f64Lt res 0#64 = true <;> simp [hLt]
    · have hEq : f64Eq res 0#64 = true := by
        unfold f64Eq; simp [hNaN, hZ0, hz, hZZero]
      simp [hEq]

/-- Go 1.27's branchless x86 `min(a, b)` lowering (`MINSD b, a; MINSD a, t; POR`) equals
IEEE-754 `Track5A.f64Min a b` (preserving `-0.0 < +0.0`) on all non-NaN binary64 inputs. -/
theorem minsd_por_eq_f64Min (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    (vminpdLane (vminpdLane a b) a ||| vminpdLane a b) = f64Min a b := by
  have haN : f64IsNaN a = false := by simpa [f64NotNaN] using ha
  have hbN : f64IsNaN b = false := by simpa [f64NotNaN] using hb
  have hLt := f64Lt_eq_f64Cmp_n a b
  unfold f64Min vminpdLane
  simp only [haN, hbN, Bool.or_self, Bool.false_eq_true, ↓reduceIte, ← hLt]
  by_cases hBothZ : (f64IsZero a && f64IsZero b) = true
  · simp only [Bool.and_eq_true] at hBothZ
    rcases (f64IsZero_iff a).mp hBothZ.1 with rfl | rfl <;>
    rcases (f64IsZero_iff b).mp hBothZ.2 with rfl | rfl <;> decide
  · simp only [hBothZ, Bool.false_eq_true, ↓reduceIte]
    by_cases hab : f64Lt a b = true
    · have haa : f64Lt a a = false := by
        rw [f64Lt_iff_ult_numKey a a ha ha, BitVec.ult_eq_decide]; simp
      simp [hab, haa]
    · simp only [hab, Bool.false_eq_true, ↓reduceIte]
      by_cases hba : f64Lt b a = true
      · simp [hba]
      · simp only [hba, Bool.false_eq_true, ↓reduceIte]
        rw [f64Lt_iff_ult_numKey a b ha hb, BitVec.ult_eq_decide] at hab
        rw [f64Lt_iff_ult_numKey b a hb ha, BitVec.ult_eq_decide] at hba
        have hK : f64NumKey a = f64NumKey b :=
          BitVec.eq_of_toNat_eq (by simp only [decide_eq_true_eq] at hab hba; omega)
        have hEq : (a == b || f64IsZero a && f64IsZero b) = true := by
          rw [← f64NumKey_eq_iff a b]; exact beq_iff_eq.mpr hK
        simp only [hBothZ, Bool.or_false, beq_iff_eq] at hEq
        subst hEq
        simp

/-- Direct bridge to `Track5D`: `VMINPD` exhibits x86 signed-zero asymmetry, yet canonicalized
`canonZero64 (vminpdLane a b)` is commutative, associative, and equal to `canonZero64 (f64Min a b)`. -/
theorem vminpd_track5d_canon_bridge (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    vminpdLane f64PosZero f64NegZero = f64NegZero ∧
    vminpdLane f64NegZero f64PosZero = f64PosZero ∧
    canonZero64 (vminpdLane a b) = canonZero64 (f64Min a b) ∧
    canonZero64 (vminpdLane a b) = canonZero64 (vminpdLane b a) ∧
    f64NotNaN (vminpdLane a b) = true := by
  refine ⟨vminpdLane_signed_zero_asymmetry.1, vminpdLane_signed_zero_asymmetry.2, ?_,
    (vminpdLane_numKey_assoc_comm a b a ha hb ha).2.1, ?_⟩
  · have haN : f64IsNaN a = false := by simpa [f64NotNaN] using ha
    have hbN : f64IsNaN b = false := by simpa [f64NotNaN] using hb
    have hLt := f64Lt_eq_f64Cmp_n a b
    unfold f64Min vminpdLane
    simp only [haN, hbN, Bool.or_self, Bool.false_eq_true, ↓reduceIte, ← hLt]
    by_cases hBothZ : (f64IsZero a && f64IsZero b) = true
    · simp only [Bool.and_eq_true] at hBothZ
      rcases (f64IsZero_iff a).mp hBothZ.1 with rfl | rfl <;>
      rcases (f64IsZero_iff b).mp hBothZ.2 with rfl | rfl <;> decide
    · simp [hBothZ]
  · unfold vminpdLane
    split <;> assumption

/-- Chunk-accelerated single-step lemma for symbolic execution over raw ELF `.text` bytes. -/
private theorem runAmd64Vec_step_chunk (k : Nat) (chunks : List (List UInt8))
    (hWf : chunks16WellFormed chunks = true)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (s : Amd64VecState) (insn : Amd64VecInsn) (nextOff : Nat)
    (hRun : s.status = .running)
    (hDec : decodeAmd64VecInsnWith (chunkGet16 chunks) s.pc = some (insn, nextOff)) :
    runAmd64Vec (k + 1) ⟨chunks.flatten⟩ rodata ext s =
      runAmd64Vec k ⟨chunks.flatten⟩ rodata ext (execAmd64VecInsn rodata ext s insn nextOff) := by
  have hB : (s.status == .running) = true := beq_iff_eq.mpr hRun
  have hFun : (fun i => (⟨chunks.flatten⟩ : Array UInt8)[i]?) = chunkGet16 chunks := by
    funext i; exact (chunkGet16_eq_flatten chunks hWf i).symm
  have hDecArr : decodeAmd64VecInsn ⟨chunks.flatten⟩ s.pc = some (insn, nextOff) := by
    unfold decodeAmd64VecInsn; rw [hFun]; exact hDec
  rw [runAmd64Vec]; simp only [hB, ↓reduceIte]
  rw [stepAmd64Vec_at ⟨chunks.flatten⟩ rodata ext s insn nextOff hRun hDecArr]

/-- Whether an instruction is a guaranteed fall-through preserving `status = .running`
and advancing `pc := nextOff` (given `rodata` resolves any RIP-relative constant). -/
private def Amd64VecInsn.isSafeFallthrough (rodata : Nat → Option (BitVec 64)) (off : Nat) :
    Amd64VecInsn → Bool
  | .ret | .jmpRel _ | .jccRel _ _ | .callRel _ => false
  | .movMR64 (.ripRel _) _ | .lea64 _ (.ripRel _)
  | .sseMovupsLoad _ (.ripRel _) | .sseMovupsStore (.ripRel _) _
  | .vmovdquLoad _ _ _ (.ripRel _) _ | .vmovdquStore _ _ (.ripRel _) _ _
  | .vpand _ _ _ _ (.mem (.ripRel _)) _ | .vsubpd _ _ _ _ (.mem (.ripRel _)) _
  | .vminpd _ _ _ _ (.mem (.ripRel _)) _ | .vcmppdVex _ _ _ (.mem (.ripRel _)) _
  | .vcmppdEvex _ _ _ (.mem (.ripRel _)) _ _ | .vpblendvb _ _ _ (.mem (.ripRel _)) _
  | .vpblendmq _ _ _ (.mem (.ripRel _)) _ => false
  | .movRM64 _ (.ripRel _) | .cmpRM64 _ (.ripRel _)
  | .sseMovsdLoad _ (.ripRel _) | .vmovsdLoad _ (.ripRel _)
  | .vbroadcastsd _ _ _ (.mem (.ripRel _)) _ | .vpbroadcastq _ _ _ (.mem (.ripRel _)) _ =>
      (rodata off).isSome
  | _ => true

private theorem execAmd64VecInsn_safeFallthrough (rodata : Nat → Option (BitVec 64))
    (ext : Nat → Option String) (s : Amd64VecState) (insn : Amd64VecInsn) (nextOff : Nat)
    (hRun : s.status = .running)
    (hSafe : insn.isSafeFallthrough rodata s.pc = true) :
    (execAmd64VecInsn rodata ext s insn nextOff).status = .running ∧
    (execAmd64VecInsn rodata ext s insn nextOff).pc = nextOff := by
  cases insn with
  | ret | jmpRel _ | jccRel _ _ | callRel _ => simp [Amd64VecInsn.isSafeFallthrough] at hSafe
  | movMR64 addr _ =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | lea64 _ addr =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | sseMovupsLoad _ addr =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | sseMovupsStore addr _ =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | vmovdquLoad _ _ _ addr _ =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | vmovdquStore _ _ addr _ _ =>
    cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalMemAddr, hRun] at hSafe ⊢
  | movRM64 _ addr =>
    cases addr with
    | ripRel _ =>
      simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
      rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
      simp [execAmd64VecInsn, readMem64, hv, hRun]
    | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | cmpRM64 _ addr =>
    cases addr with
    | ripRel _ =>
      simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
      rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
      simp [execAmd64VecInsn, readMem64, hv, hRun]
    | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | sseMovsdLoad _ addr =>
    cases addr with
    | ripRel _ =>
      simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
      rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
      simp [execAmd64VecInsn, readMem64, hv, hRun]
    | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | vmovsdLoad _ addr =>
    cases addr with
    | ripRel _ =>
      simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
      rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
      simp [execAmd64VecInsn, readMem64, hv, hRun]
    | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | vbroadcastsd _ _ _ src _ =>
    cases src with
    | reg _ => simp [execAmd64VecInsn, hRun]
    | mem addr =>
      cases addr with
      | ripRel _ =>
        simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
        rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
        simp [execAmd64VecInsn, readMem64, hv, hRun]
      | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | vpbroadcastq _ _ _ src _ =>
    cases src with
    | reg _ => simp [execAmd64VecInsn, hRun]
    | mem addr =>
      cases addr with
      | ripRel _ =>
        simp only [Amd64VecInsn.isSafeFallthrough] at hSafe
        rcases Option.isSome_iff_exists.mp hSafe with ⟨v, hv⟩
        simp [execAmd64VecInsn, readMem64, hv, hRun]
      | baseIdxDisp _ _ _ => simp [execAmd64VecInsn, readMem64, evalMemAddr, hRun]
  | vpand _ _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vsubpd _ _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vminpd _ _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vcmppdVex _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vcmppdEvex _ _ _ src2 _ _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vpblendvb _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | vpblendmq _ _ _ src2 _ =>
    cases src2 with
    | reg _ => simp [execAmd64VecInsn, evalVecRm, hRun]
    | mem addr => cases addr <;> simp [Amd64VecInsn.isSafeFallthrough, execAmd64VecInsn, evalVecRm, evalMemAddr, hRun] at hSafe ⊢
  | _ => simp [execAmd64VecInsn, hRun]

/-- Execute a concrete decoded instruction list sequentially. -/
private def execDecodedSeq (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (s : Amd64VecState) : List (Amd64VecInsn × Nat) → Amd64VecState
  | [] => s
  | (insn, nextOff) :: rest =>
      execDecodedSeq rodata ext (execAmd64VecInsn rodata ext s insn nextOff) rest

/-- Verify that `seq` is a valid straight-line decoded block starting at `pc0`. -/
private def checkBlockSeq (chunks : List (List UInt8)) (rodata : Nat → Option (BitVec 64))
    (pc0 : Nat) : List (Amd64VecInsn × Nat) → Bool
  | [] => true
  | [(insn, nextOff)] =>
      decodeAmd64VecInsnWith (chunkGet16 chunks) pc0 == some (insn, nextOff)
  | (insn, nextOff) :: rest =>
      (decodeAmd64VecInsnWith (chunkGet16 chunks) pc0 == some (insn, nextOff)) &&
      insn.isSafeFallthrough rodata pc0 &&
      checkBlockSeq chunks rodata nextOff rest

private theorem runAmd64Vec_block_seq (k : Nat) (chunks : List (List UInt8))
    (hWf : chunks16WellFormed chunks = true)
    (rodata : Nat → Option (BitVec 64)) (ext : Nat → Option String)
    (s : Amd64VecState) (pc0 : Nat) (seq : List (Amd64VecInsn × Nat))
    (hRun : s.status = .running) (hPc : s.pc = pc0)
    (hChk : checkBlockSeq chunks rodata pc0 seq = true) :
    runAmd64Vec (k + seq.length) ⟨chunks.flatten⟩ rodata ext s =
      runAmd64Vec k ⟨chunks.flatten⟩ rodata ext (execDecodedSeq rodata ext s seq) := by
  induction seq generalizing s pc0 with
  | nil => simp [execDecodedSeq]
  | cons hd tl ih =>
    rcases hd with ⟨insn, nextOff⟩
    cases tl with
    | nil =>
      simp only [checkBlockSeq, beq_iff_eq] at hChk
      simp only [List.length_singleton, execDecodedSeq]
      exact runAmd64Vec_step_chunk k chunks hWf rodata ext s insn nextOff hRun (hPc ▸ hChk)
    | cons hd2 tl2 =>
      simp only [checkBlockSeq, Bool.and_eq_true, beq_iff_eq] at hChk
      rcases hChk with ⟨⟨hDec, hSafe⟩, hRest⟩
      have hStep := runAmd64Vec_step_chunk (k + (hd2 :: tl2).length) chunks hWf rodata ext s insn nextOff hRun (hPc ▸ hDec)
      have hNext := execAmd64VecInsn_safeFallthrough rodata ext s insn nextOff hRun (hPc ▸ hSafe)
      have hLen : k + ((insn, nextOff) :: hd2 :: tl2).length = (k + (hd2 :: tl2).length) + 1 := by
        simp [Nat.add_assoc]
      rw [hLen, hStep]
      exact ih (execAmd64VecInsn rodata ext s insn nextOff) nextOff hNext.1 hNext.2 hRest

/-!
### Part VI.A: Inductive Functional Equivalence — `allAbsLE64AVX2` & `allAbsLE64AVX512`
-/

/-- Clean sequential IEEE-754 specification of `AllAbsLE64` over the first `m` elements of `x`. -/
def specAllAbsLE (x : Nat → BitVec 64) (limit : BitVec 64) : Nat → Bool
  | 0 => !f64IsNaN limit
  | k + 1 => specAllAbsLE x limit k && f64Le (f64Abs (x k)) limit

theorem specAllAbsLE_iff (x : Nat → BitVec 64) (limit : BitVec 64) (m : Nat) :
    specAllAbsLE x limit m = true ↔
      (!f64IsNaN limit = true ∧ ∀ j < m, f64Le (f64Abs (x j)) limit = true) := by
  induction m with
  | zero => simp [specAllAbsLE]
  | succ k ih =>
    simp only [specAllAbsLE, Bool.and_eq_true, ih]
    constructor
    · rintro ⟨⟨hL, hAll⟩, hk⟩
      refine ⟨hL, fun j hj => ?_⟩
      by_cases hEq : j = k
      · subst hEq; exact hk
      · exact hAll j (by omega)
    · rintro ⟨hL, hAll⟩
      exact ⟨⟨hL, fun j hj => hAll j (by omega)⟩, hAll k (by omega)⟩

private theorem allAbsLE64AVX2_chunks_wf : chunks16WellFormed allAbsLE64AVX2ByteChunks = true := by
  decide

private theorem allAbsLE64AVX512_chunks_wf : chunks16WellFormed allAbsLE64AVX512ByteChunks = true := by
  decide

/-- Loop invariant at `pc = 0x68` (8-wide header) and `pc = 0x87` (4-wide header) of `allAbsLE64AVX2`. -/
private structure AllAbsLE2Inv (base : BitVec 64) (x : Nat → BitVec 64) (limit : BitVec 64)
    (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = base
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  zmm0Eq : s.zmm 0#5 = (Vec512.broadcast limit).zeroHi256
  zmm1Eq : s.zmm 1#5 = (Vec512.broadcast 0x7fffffffffffffff#64).zeroHi256
  memX : ∀ j < n, s.mem (elemAddr base j) = x j
  masks : ∃ a0 a1 a2 a3 b0 b1 b2 b3 : Bool,
    s.zmm 3#5 = maskVec4 a0 a1 a2 a3 ∧
    s.zmm 2#5 = maskVec4 b0 b1 b2 b3 ∧
    (a0 && a1 && a2 && a3 && b0 && b1 && b2 && b3) = specAllAbsLE x limit i

private def allAbsLE2BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 1),
    (.movRR64 5#4 4#4, 4),
    (.movMR64 (.baseIdxDisp (some 4#4) none 16) 0#4, 9),
    (.testRR64 3#4 3#4, 12),
    (.jccRel .be 156, 18) ]

private def allAbsLE2BlkPro1 : List (Amd64VecInsn × Nat) :=
  [ (.vmovsdRR 0#5 15#5 0#5, 22),
    (.vbroadcastsd false .ymm256 0#5 (.reg 0#5) {}, 27),
    (.movImm64 1#4 0x7fffffffffffffff#64, 37),
    (.vmovqFromGpr 1#5 1#4, 42),
    (.nop, 43), (.nop, 44), (.nop, 45),
    (.vpbroadcastq false .ymm256 1#5 (.reg 1#5) {}, 50),
    (.vcmppdVex .ymm256 2#5 0#5 (.reg 0#5) 0, 55),
    (.xorRR32 1#4 1#4, 57),
    (.vmovdquRR false .ymm256 3#5 2#5 {}, 61),
    (.jmpRel 41, 63) ]

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkPro0_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 0 allAbsLE2BlkPro0 = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkPro1_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 18 allAbsLE2BlkPro1 = true := by
  decide

private theorem allAbsLE64AVX2_prologue (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running)
    (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64) :
    let s1 := runAmd64Vec 17 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
    s1.pc = 0x68 ∧ AllAbsLE2Inv base x limit n 0 s1 := by
  have hBe : evalX86Cond .be (x86TestFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = false :=
    evalX86Cond_be_test64_self hPos (by omega)
  have hRsp : s0.gpr 4#4 - 8#64 + 16#64 = s0.gpr 4#4 + 8#64 := by bv_omega
  have hJmp : nextTargetPc 63 41 = some 0x68 := by decide
  dsimp only [allAbsLE64AVX2Bytes]
  have hStep0 := runAmd64Vec_block_seq 12 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0 0 allAbsLE2BlkPro0 hRun hPc allAbsLE2BlkPro0_ok
  have hLen0 : 12 + allAbsLE2BlkPro0.length = 17 := rfl
  rw [← hLen0, hStep0]
  let sMid := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0 allAbsLE2BlkPro0
  change (runAmd64Vec 12 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid).pc = 0x68 ∧
    AllAbsLE2Inv base x limit n 0
      (runAmd64Vec 12 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hRun]
  have hMidPc : sMid.pc = 18 := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hRbx, hBe, hHaz]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRax, hRbx, hBe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidZ0 : (sMid.zmm 0#5).l0 = limit := by
    simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hLim]
  have hMidMem : ∀ j < n, sMid.mem (elemAddr base j) = x j := by
    intro j hj
    have hd := hDisj j hj
    have hMem0 : sMid.mem = write64 (write64 s0.mem (s0.gpr 4#4 - 8#64) (s0.gpr 5#4)) (s0.gpr 4#4 + 8#64) base := by
      simp [sMid, allAbsLE2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
        hRax, hRbx, hBe, hRsp]
    rw [hMem0, write64_ne _ _ hd.2, write64_ne _ _ hd.1]
    exact hX j hj
  have hStep1 := runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid 18 allAbsLE2BlkPro1 hMidRun hMidPc allAbsLE2BlkPro1_ok
  have hLen1 : 0 + allAbsLE2BlkPro1.length = 12 := rfl
  rw [← hLen1, hStep1]
  simp only [runAmd64Vec]
  refine ⟨by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp], ?_⟩
  refine ⟨by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, hMidRun],
    by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, Amd64VecInsn.dirtiesUpper],
    by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr, hMidRax],
    by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr],
    by simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr, hMidRbx],
    ?_, ?_, ?_, ?_⟩
  · simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp,
      setZmm, Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256, hMidZ0]
  · simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp,
      setZmm, setGpr, Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256]
  · intro j hj
    have hMemPro1 : (execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid allAbsLE2BlkPro1).mem = sMid.mem := by
      simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp]
    rw [hMemPro1]
    exact hMidMem j hj
  · let ok := !f64IsNaN limit
    refine ⟨ok, ok, ok, ok, ok, ok, ok, ok, ?_, ?_, ?_⟩
    · simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm,
        Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256,
        Vec512.zipWith, x86VcmppdPred, f64Eq_self_eq_notNaN, maskVec4, maskLane64, ok, hMidZ0]
    · simp [allAbsLE2BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm,
        Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256,
        Vec512.zipWith, x86VcmppdPred, f64Eq_self_eq_notNaN, maskVec4, maskLane64, ok, hMidZ0]
    · cases h : f64IsNaN limit <;> simp [ok, specAllAbsLE, h]

private theorem f64Abs_and_comm (x : BitVec 64) :
    0x7fffffffffffffff#64 &&& x = f64Abs x := by
  unfold f64Abs; exact BitVec.and_comm _ _

private theorem ite_mask_and_prop (p q : Bool) :
    ((if p = true then 0xffffffffffffffff#64 else 0#64) &&&
     (if q = true then 0xffffffffffffffff#64 else 0#64)) =
    if (p = true ∧ q = true) then 0xffffffffffffffff#64 else 0#64 := by
  cases p <;> cases q <;> rfl

private theorem applyEvexMask_zero (kVal : BitVec 8) (z : Bool) (oldV newV : Vec512) :
    Vec512.applyEvexMask 0#3 kVal z oldV newV = newV := rfl

private theorem specAllAbsLE_add4 (x : Nat → BitVec 64) (limit : BitVec 64) (i : Nat) :
    specAllAbsLE x limit (i + 4) =
      (specAllAbsLE x limit i &&
        f64Le (f64Abs (x i)) limit &&
        f64Le (f64Abs (x (i + 1))) limit &&
        f64Le (f64Abs (x (i + 2))) limit &&
        f64Le (f64Abs (x (i + 3))) limit) := by
  rfl

private theorem specAllAbsLE_add8 (x : Nat → BitVec 64) (limit : BitVec 64) (i : Nat) :
    specAllAbsLE x limit (i + 8) =
      (specAllAbsLE x limit i &&
        f64Le (f64Abs (x i)) limit &&
        f64Le (f64Abs (x (i + 1))) limit &&
        f64Le (f64Abs (x (i + 2))) limit &&
        f64Le (f64Abs (x (i + 3))) limit &&
        f64Le (f64Abs (x (i + 4))) limit &&
        f64Le (f64Abs (x (i + 5))) limit &&
        f64Le (f64Abs (x (i + 6))) limit &&
        f64Le (f64Abs (x (i + 7))) limit) := by
  rfl

private theorem specAllAbsLE_add16 (x : Nat → BitVec 64) (limit : BitVec 64) (i : Nat) :
    specAllAbsLE x limit (i + 16) =
      (specAllAbsLE x limit (i + 8) &&
        f64Le (f64Abs (x (i + 8))) limit &&
        f64Le (f64Abs (x (i + 9))) limit &&
        f64Le (f64Abs (x (i + 10))) limit &&
        f64Le (f64Abs (x (i + 11))) limit &&
        f64Le (f64Abs (x (i + 12))) limit &&
        f64Le (f64Abs (x (i + 13))) limit &&
        f64Le (f64Abs (x (i + 14))) limit &&
        f64Le (f64Abs (x (i + 15))) limit) := by
  rfl

private def allAbsLE2BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 108),
    (.cmpRR64 3#4 2#4, 111),
    (.jccRel .ge (-50), 113) ]

private def allAbsLE2BlkLoop8Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 67),
    (.vpand false .ymm256 4#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 71),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 75),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 32), 79),
    (.vpand false .ymm256 5#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 83),
    (.vcmppdVex .ymm256 4#5 4#5 (.reg 0#5) 2, 88),
    (.vcmppdVex .ymm256 5#5 5#5 (.reg 0#5) 2, 93),
    (.vpand false .ymm256 3#5 4#5 (.reg 3#5) {}, 97),
    (.vpand false .ymm256 2#5 5#5 (.reg 2#5) {}, 101),
    (.movRR64 1#4 2#4, 104) ]

private def allAbsLE2BlkLoop8Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 20, 115) ]

private def allAbsLE2BlkLoop4Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 4), 139),
    (.cmpRR64 3#4 2#4, 142),
    (.jccRel .ge (-29), 144) ]

private def allAbsLE2BlkLoop4Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 119),
    (.vpand false .ymm256 4#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 123),
    (.vcmppdVex .ymm256 4#5 4#5 (.reg 0#5) 2, 128),
    (.vpand false .ymm256 3#5 3#5 (.reg 4#5) {}, 132),
    (.movRR64 1#4 2#4, 135) ]

private def allAbsLE2BlkEpi0 : List (Amd64VecInsn × Nat) :=
  [ (.vpand false .ymm256 0#5 3#5 (.reg 2#5) {}, 148),
    (.vmovmskpd .ymm256 1#4 0#5, 152),
    (.cmpImm8 1#4 0x0f#8, 155),
    (.jccRel .e 7, 157) ]

private def allAbsLE2BlkRetFalse : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 160),
    (.xorRR32 0#4 0#4, 162),
    (.popReg 5#4, 163),
    (.ret, 164) ]

private def allAbsLE2BlkRetTrue : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 167),
    (.movImm32 0#4 1#32, 172),
    (.popReg 5#4, 173),
    (.ret, 174) ]

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkLoop8Hdr_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 104 allAbsLE2BlkLoop8Hdr = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkLoop8Body_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 63 allAbsLE2BlkLoop8Body = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkLoop8Exit_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 113 allAbsLE2BlkLoop8Exit = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkLoop4Hdr_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 135 allAbsLE2BlkLoop4Hdr = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkLoop4Body_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 115 allAbsLE2BlkLoop4Body = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkEpi0_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 144 allAbsLE2BlkEpi0 = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkRetFalse_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 157 allAbsLE2BlkRetFalse = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE2BlkRetTrue_ok :
    checkBlockSeq allAbsLE64AVX2ByteChunks allAbsLE64AVX2Rodata 164 allAbsLE2BlkRetTrue = true := by
  decide

private theorem allAbsLE64AVX2_loop8_step (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x68) (hInv : AllAbsLE2Inv base x limit n i s)
    (hStep : i + 8 ≤ n) (hN : n + 16 < 2 ^ 63) :
    let s' := runAmd64Vec 13 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
    s'.pc = 0x68 ∧ AllAbsLE2Inv base x limit n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 113 (-50) = some 63 := by decide
  dsimp only [allAbsLE64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 10 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s 104 allAbsLE2BlkLoop8Hdr hInv.statusRun hPc allAbsLE2BlkLoop8Hdr_ok
  have hL0 : 10 + allAbsLE2BlkLoop8Hdr.length = 13 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s allAbsLE2BlkLoop8Hdr
  change (runAmd64Vec 10 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid).pc = 0x68 ∧
    AllAbsLE2Inv base x limit n (i + 8)
      (runAmd64Vec 10 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 63 := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid 63 allAbsLE2BlkLoop8Body hMidRun hMidPc allAbsLE2BlkLoop8Body_ok
  have hL1 : 0 + allAbsLE2BlkLoop8Body.length = 10 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.masks with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ3, hZ2, hProd⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hBaseAddr32 : elemAddr base i + 32#64 = elemAddr base (i + 4) := elemAddr_plus_32 base i
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega)
  have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega)
  have hX7 := hInv.memX (i + 7) (by omega)
  refine ⟨by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm0Eq],
    by simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨f64Le (f64Abs (x i)) limit && a0,
          f64Le (f64Abs (x (i + 1))) limit && a1,
          f64Le (f64Abs (x (i + 2))) limit && a2,
          f64Le (f64Abs (x (i + 3))) limit && a3,
          f64Le (f64Abs (x (i + 4))) limit && b0,
          f64Le (f64Abs (x (i + 5))) limit && b1,
          f64Le (f64Abs (x (i + 6))) limit && b2,
          f64Le (f64Abs (x (i + 7))) limit && b3, ?_, ?_, ?_⟩
  · simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm0Eq, hInv.zmm1Eq,
      hZ3, hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX0, hX1, hX2, hX3, Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm, maskVec4,
      maskLane64, ite_mask_and_prop]
  · simp [allAbsLE2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm0Eq, hInv.zmm1Eq,
      hZ2, hBaseAddr, hBaseAddr32, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX4, hX5, hX6, hX7, Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm, maskVec4,
      maskLane64, ite_mask_and_prop]
  · rw [specAllAbsLE_add8, ← hProd]
    ac_rfl

private theorem allAbsLE64AVX2_loop8_iter (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n : Nat) (hN : n + 16 < 2 ^ 63) :
    ∀ q i s, i + 8 * q ≤ n → s.pc = 0x68 → AllAbsLE2Inv base x limit n i s →
      let s' := runAmd64Vec (13 * q) allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
      s'.pc = 0x68 ∧ AllAbsLE2Inv base x limit n (i + 8 * q) s' := by
  intro q
  induction q with
  | zero =>
    intro i s _ hPc hInv
    simpa [runAmd64Vec] using And.intro hPc hInv
  | succ q ih =>
    intro i s hLe hPc hInv
    have hFuel : 13 * (q + 1) = 13 + 13 * q := by omega
    dsimp only
    rw [hFuel, runAmd64Vec_add]
    have h1 := allAbsLE64AVX2_loop8_step base x limit n i s hPc hInv (by omega) hN
    have h2 := ih (i + 8) _ (by omega) h1.1 h1.2
    have hIdx : i + 8 + 8 * q = i + 8 * (q + 1) := by omega
    simpa [hIdx] using h2

private theorem allAbsLE64AVX2_loop8_exit (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x68) (hInv : AllAbsLE2Inv base x limit n i s)
    (hLt : n < i + 8) (hN : n + 16 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
    s'.pc = 0x87 ∧ AllAbsLE2Inv base x limit n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJmp : nextTargetPc 115 20 = some 0x87 := by decide
  dsimp only [allAbsLE64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 1 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s 104 allAbsLE2BlkLoop8Hdr hInv.statusRun hPc allAbsLE2BlkLoop8Hdr_ok
  have hL0 : 1 + allAbsLE2BlkLoop8Hdr.length = 4 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s allAbsLE2BlkLoop8Hdr
  change (runAmd64Vec 1 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid).pc = 0x87 ∧
    AllAbsLE2Inv base x limit n i
      (runAmd64Vec 1 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 113 := by
    simp [sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid 113 allAbsLE2BlkLoop8Exit hMidRun hMidPc allAbsLE2BlkLoop8Exit_ok
  have hL1 : 0 + allAbsLE2BlkLoop8Exit.length = 1 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [allAbsLE2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp], ?_⟩
  refine ⟨by simp [allAbsLE2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMidRun],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.upperDirtyT],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.noHazard],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.zmm0Eq],
    by simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq,
         execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.memX j hj],
    by simpa [allAbsLE2BlkLoop8Exit, sMid, allAbsLE2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp] using hInv.masks⟩

private theorem allAbsLE64AVX2_loop4_step (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x87) (hInv : AllAbsLE2Inv base x limit n i s)
    (hStep : i + 4 ≤ n) (hN : n + 16 < 2 ^ 63) :
    let s' := runAmd64Vec 8 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
    s'.pc = 0x87 ∧ AllAbsLE2Inv base x limit n (i + 4) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  have hJcc : nextTargetPc 144 (-29) = some 115 := by decide
  dsimp only [allAbsLE64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 5 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s 135 allAbsLE2BlkLoop4Hdr hInv.statusRun hPc allAbsLE2BlkLoop4Hdr_ok
  have hL0 : 5 + allAbsLE2BlkLoop4Hdr.length = 8 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s allAbsLE2BlkLoop4Hdr
  change (runAmd64Vec 5 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid).pc = 0x87 ∧
    AllAbsLE2Inv base x limit n (i + 4)
      (runAmd64Vec 5 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 115 := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 4) := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid 115 allAbsLE2BlkLoop4Body hMidRun hMidPc allAbsLE2BlkLoop4Body_ok
  have hL1 : 0 + allAbsLE2BlkLoop4Body.length = 5 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.masks with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ3, hZ2, hProd⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  refine ⟨by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm0Eq],
    by simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨a0 && f64Le (f64Abs (x i)) limit,
          a1 && f64Le (f64Abs (x (i + 1))) limit,
          a2 && f64Le (f64Abs (x (i + 2))) limit,
          a3 && f64Le (f64Abs (x (i + 3))) limit,
          b0, b1, b2, b3, ?_, ?_, ?_⟩
  · simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm0Eq, hInv.zmm1Eq,
      hZ3, hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX0, hX1, hX2, hX3, Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm, maskVec4,
      maskLane64, ite_mask_and_prop]
  · simp [allAbsLE2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, hMidZmm, hZ2]
  · rw [specAllAbsLE_add4, ← hProd]
    ac_rfl

private theorem allAbsLE64AVX2_loop4_exit_and_ret (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x87) (hInv : AllAbsLE2Inv base x limit n i s)
    (hLt : n < i + 4) (hN : n + 16 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 11 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s
    s'.status = .returned ∧
    s'.upperDirty = false ∧
    s'.sseHazard = false ∧
    s'.gpr 0#4 = (if specAllAbsLE x limit i then 1#64 else 0#64) := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  dsimp only [allAbsLE64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 8 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s 135 allAbsLE2BlkLoop4Hdr hInv.statusRun hPc allAbsLE2BlkLoop4Hdr_ok
  have hL0 : 8 + allAbsLE2BlkLoop4Hdr.length = 11 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s allAbsLE2BlkLoop4Hdr
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 144 := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.noHazard]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  change (runAmd64Vec 8 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid).status = .returned ∧ _
  have hS1 := runAmd64Vec_block_seq 4 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
    allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid 144 allAbsLE2BlkEpi0 hMidRun hMidPc allAbsLE2BlkEpi0_ok
  have hL1 : 4 + allAbsLE2BlkEpi0.length = 8 := rfl
  rw [← hL1, hS1]
  let sEpi := execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sMid allAbsLE2BlkEpi0
  rcases hInv.masks with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ3, hZ2, hProd⟩
  have hComb : ((maskVec4 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3)).movmskpd4.extractLsb' 0 8 == 0x0f#8) =
      specAllAbsLE x limit i := by
    rw [movmskpd4_maskVec4_eq_0xf, ← hProd]
    ac_rfl
  have hCmp : evalX86Cond .e (x86SubFlags8 ((maskVec4 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3)).movmskpd4.extractLsb' 0 8) 0x0f#8) =
      specAllAbsLE x limit i := by
    rw [evalX86Cond_e_sub8, hComb]
  have hJcc7 : nextTargetPc 157 7 = some 164 := by decide
  have hMaskComb : (Vec512.applyWidth .ymm256
      (Vec512.applyEvexMask 0#3 (sMid.kmask 0#3) false (s.zmm 0#5)
        (Vec512.zipWith (· &&& ·) (maskVec4 a0 a1 a2 a3) (maskVec4 b0 b1 b2 b3)))) =
      maskVec4 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3) := by
    simp [Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.zipWith,
      maskVec4, maskLane64_and]
  have hEpiRun : sEpi.status = .running := by
    cases hSpec : specAllAbsLE x limit i <;>
      simp [sEpi, allAbsLE2BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7, hMidRun]
  have hEpiHaz : sEpi.sseHazard = false := by
    cases hSpec : specAllAbsLE x limit i <;>
      simp [sEpi, allAbsLE2BlkEpi0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
        evalVecRm, setZmm, setGpr, hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7, hMidHaz]
  change (runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi).status = .returned ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi).upperDirty = false ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi).sseHazard = false ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi).gpr 0#4 =
      (if specAllAbsLE x limit i then 1#64 else 0#64)
  cases hSpec : specAllAbsLE x limit i with
  | false =>
    have hEpiPc : sEpi.pc = 157 := by
      simp [sEpi, allAbsLE2BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec]
    have hS2 : runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi =
        runAmd64Vec 0 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets
          (execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi allAbsLE2BlkRetFalse) :=
      runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
        allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi 157 allAbsLE2BlkRetFalse hEpiRun hEpiPc allAbsLE2BlkRetFalse_ok
    rw [hS2]
    simp [runAmd64Vec, allAbsLE2BlkRetFalse, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setGpr, hEpiHaz]
  | true =>
    have hEpiPc : sEpi.pc = 164 := by
      simp [sEpi, allAbsLE2BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7]
    have hS2 : runAmd64Vec 4 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi =
        runAmd64Vec 0 ⟨allAbsLE64AVX2ByteChunks.flatten⟩ allAbsLE64AVX2Rodata allAbsLE64AVX2Targets
          (execDecodedSeq allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi allAbsLE2BlkRetTrue) :=
      runAmd64Vec_block_seq 0 allAbsLE64AVX2ByteChunks allAbsLE64AVX2_chunks_wf
        allAbsLE64AVX2Rodata allAbsLE64AVX2Targets sEpi 164 allAbsLE2BlkRetTrue hEpiRun hEpiPc allAbsLE2BlkRetTrue_ok
    rw [hS2]
    simp [runAmd64Vec, allAbsLE2BlkRetTrue, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setGpr, hEpiHaz]

/-- Exact step count executed by `allAbsLE64AVX2` on a slice of length `n = 4q + r`. -/
def allAbsLE64AVX2Steps (n : Nat) : Nat :=
  17 + 13 * (n / 8) + 4 + (if 8 * (n / 8) + 4 ≤ n then 8 else 0) + 11

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`allAbsLE64AVX2`)**:
For arbitrary slice length `n = 4q + r` (`0 < n`, `n + 16 < 2^63`, including zero-trip cases
`n < 4` and `n < 8`), executing the raw compiled `GOARCH=amd64 GOEXPERIMENT=simd` VEX 256-bit
ELF `.text` bytes of `internal/simd.allAbsLE64AVX2`:
1. Never hits `runtime.panicBounds`, `morestack`, or any external runtime/GC call;
2. Terminates with `status = .returned`, `upperDirty = false` (`VZEROUPPER` ABI safety), and
   `sseHazard = false`;
3. Returns bit-exact `RAX = if specAllAbsLE x limit (4 * (n / 4)) then 1 else 0`;
4. Preserves Go amd64 ABI registers: `RSP = s0.gpr 4#4 + 8#64`, `RBP = s0.gpr 5#4`,
   `R14 = s0.gpr 14#4`, and `X15` low 128 bits (`l0`, `l1`). -/
theorem allAbsLE64AVX2_functional_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running)
    (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64)
    (hFuel : allAbsLE64AVX2Steps n ≤ fuel) :
    let sf := runAmd64Vec fuel allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    sf.gpr 0#4 = (if specAllAbsLE x limit (4 * (n / 4)) then 1#64 else 0#64) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  have hPro := allAbsLE64AVX2_prologue base x limit n s0 hPc hRun hHaz hPos hN hRax hRbx hLim hX hDisj
  have hL8 := allAbsLE64AVX2_loop8_iter base x limit n hN (n / 8) 0 _ (by omega) hPro.1 hPro.2
  have hEx8 := allAbsLE64AVX2_loop8_exit base x limit n (8 * (n / 8)) _
    hL8.1 (by simpa using hL8.2) (by omega) hN (by omega)
  have hExact :
      let se := runAmd64Vec (allAbsLE64AVX2Steps n) allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      se.gpr 0#4 = (if specAllAbsLE x limit (4 * (n / 4)) then 1#64 else 0#64) := by
    unfold allAbsLE64AVX2Steps
    by_cases hRem : 8 * (n / 8) + 4 ≤ n
    · have h4Eq : 8 * (n / 8) + 4 = 4 * (n / 4) := by omega
      simp only [hRem, ↓reduceIte]
      rw [runAmd64Vec_add (17 + 13 * (n / 8) + 4 + 8) 11,
          runAmd64Vec_add (17 + 13 * (n / 8) + 4) 8,
          runAmd64Vec_add (17 + 13 * (n / 8)) 4,
          runAmd64Vec_add 17 (13 * (n / 8))]
      have hL4 := allAbsLE64AVX2_loop4_step base x limit n (8 * (n / 8)) _ hEx8.1 hEx8.2 hRem hN
      have hRet := allAbsLE64AVX2_loop4_exit_and_ret base x limit n (8 * (n / 8) + 4) _
        hL4.1 hL4.2 (by omega) hN (by omega)
      simpa [h4Eq] using hRet
    · have h4Eq : 8 * (n / 8) = 4 * (n / 4) := by omega
      simp only [hRem, ↓reduceIte, Nat.add_zero]
      rw [runAmd64Vec_add (17 + 13 * (n / 8) + 4) 11,
          runAmd64Vec_add (17 + 13 * (n / 8)) 4,
          runAmd64Vec_add 17 (13 * (n / 8))]
      have hRet := allAbsLE64AVX2_loop4_exit_and_ret base x limit n (8 * (n / 8)) _
        hEx8.1 hEx8.2 (by omega) hN (by omega)
      simpa [h4Eq] using hRet
  have hSplit : fuel = allAbsLE64AVX2Steps n + (fuel - allAbsLE64AVX2Steps n) := by omega
  have hAbi := allAbsLE64AVX2_abi_reg_preserved s0 fuel hPc hRun
  have hCore :
      let sf := runAmd64Vec fuel allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      sf.gpr 0#4 = (if specAllAbsLE x limit (4 * (n / 4)) then 1#64 else 0#64) := by
    dsimp only
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hExact.1])]
    exact hExact
  have hSpRbp := hAbi.2.2.2 hCore.1
  exact ⟨hCore.1, hCore.2.1, hCore.2.2.1, hCore.2.2.2, hSpRbp.1, hSpRbp.2, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

/-! #### AVX-512 (`allAbsLE64AVX512`) Inductive Functional Equivalence -/

private def packOpmask8 (c0 c1 c2 c3 c4 c5 c6 c7 : Bool) : BitVec 8 :=
  BitVec.ofNat 8
    ((if c0 then 1 <<< 0 else 0) ||| (if c1 then 1 <<< 1 else 0) |||
     (if c2 then 1 <<< 2 else 0) ||| (if c3 then 1 <<< 3 else 0) |||
     (if c4 then 1 <<< 4 else 0) ||| (if c5 then 1 <<< 5 else 0) |||
     (if c6 then 1 <<< 6 else 0) ||| (if c7 then 1 <<< 7 else 0))

private theorem maskBit_packOpmask8 (c0 c1 c2 c3 c4 c5 c6 c7 : Bool) :
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 0 = c0 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 1 = c1 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 2 = c2 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 3 = c3 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 4 = c4 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 5 = c5 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 6 = c6 ∧
    Vec512.maskBit (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) 7 = c7 := by
  cases c0 <;> cases c1 <;> cases c2 <;> cases c3 <;>
    cases c4 <;> cases c5 <;> cases c6 <;> cases c7 <;> decide

private theorem ofOpmask8_packOpmask8 (c0 c1 c2 c3 c4 c5 c6 c7 : Bool) :
    Vec512.ofOpmask8 (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) =
      maskVec8 c0 c1 c2 c3 c4 c5 c6 c7 := by
  have hb := maskBit_packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7
  unfold Vec512.ofOpmask8 maskVec8 maskLane64
  simp [hb.1, hb.2.1, hb.2.2.1, hb.2.2.2.1, hb.2.2.2.2.1, hb.2.2.2.2.2.1, hb.2.2.2.2.2.2.1, hb.2.2.2.2.2.2.2]

private theorem maskLane64_if_zero (a c : Bool) :
    (if c then maskLane64 a else 0#64) = maskLane64 (a && c) := by
  cases a <;> cases c <;> rfl

private theorem applyEvexMask_zeroing_maskVec8 (k : BitVec 3) (hk : (k == 0#3) = false)
    (a0 a1 a2 a3 a4 a5 a6 a7 c0 c1 c2 c3 c4 c5 c6 c7 : Bool) :
    Vec512.applyEvexMask k (packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7) true
      (maskVec8 a0 a1 a2 a3 a4 a5 a6 a7) (maskVec8 a0 a1 a2 a3 a4 a5 a6 a7) =
    maskVec8 (a0 && c0) (a1 && c1) (a2 && c2) (a3 && c3)
             (a4 && c4) (a5 && c5) (a6 && c6) (a7 && c7) := by
  have hb := maskBit_packOpmask8 c0 c1 c2 c3 c4 c5 c6 c7
  unfold Vec512.applyEvexMask maskVec8
  simp [hk, hb.1, hb.2.1, hb.2.2.1, hb.2.2.2.1, hb.2.2.2.2.1, hb.2.2.2.2.2.1, hb.2.2.2.2.2.2.1, hb.2.2.2.2.2.2.2,
    maskLane64_if_zero]

/-- Loop invariant at `pc = 0x80` (16-wide header) and `pc = 0xa5` (8-wide header) of `allAbsLE64AVX512`. -/
private structure AllAbsLE512Inv (base : BitVec 64) (x : Nat → BitVec 64) (limit : BitVec 64)
    (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = base
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  zmm0Eq : s.zmm 0#5 = Vec512.broadcast limit
  zmm1Eq : s.zmm 1#5 = Vec512.broadcast 0x7fffffffffffffff#64
  memX : ∀ j < n, s.mem (elemAddr base j) = x j
  masks : ∃ a0 a1 a2 a3 a4 a5 a6 a7 b0 b1 b2 b3 b4 b5 b6 b7 : Bool,
    s.zmm 3#5 = maskVec8 a0 a1 a2 a3 a4 a5 a6 a7 ∧
    s.zmm 2#5 = maskVec8 b0 b1 b2 b3 b4 b5 b6 b7 ∧
    (a0 && a1 && a2 && a3 && a4 && a5 && a6 && a7 &&
     b0 && b1 && b2 && b3 && b4 && b5 && b6 && b7) = specAllAbsLE x limit i

private def allAbsLE512BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 1),
    (.movRR64 5#4 4#4, 4),
    (.movMR64 (.baseIdxDisp (some 4#4) none 16) 0#4, 9),
    (.testRR64 3#4 3#4, 12),
    (.jccRel .be 196, 18) ]

private def allAbsLE512BlkPro1 : List (Amd64VecInsn × Nat) :=
  [ (.vmovsdRR 0#5 15#5 0#5, 22),
    (.vbroadcastsd true .zmm512 0#5 (.reg 0#5) {}, 28),
    (.movImm64 1#4 0x7fffffffffffffff#64, 38),
    (.vmovqFromGpr 1#5 1#4, 43),
    (.vcmppdEvex .zmm512 1#3 0#5 (.reg 0#5) 0 {}, 50),
    (.nop, 51), (.nop, 52), (.nop, 53),
    (.vpbroadcastq true .zmm512 1#5 (.reg 1#5) {}, 59),
    (.vpmovm2q .zmm512 2#5 1#3, 65),
    (.xorRR32 1#4 1#4, 67),
    (.vmovdquRR true .zmm512 3#5 2#5 {}, 73),
    (.jmpRel 53, 75) ]

private def allAbsLE512BlkLoop16Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 16), 132),
    (.cmpRR64 3#4 2#4, 135),
    (.jccRel .ge (-62), 137) ]

private def allAbsLE512BlkLoop16Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 79),
    (.vpand true .zmm512 4#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 85),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 89),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 64), 93),
    (.vpand true .zmm512 5#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 99),
    (.vcmppdEvex .zmm512 1#3 4#5 (.reg 0#5) 2 {}, 106),
    (.vcmppdEvex .zmm512 2#3 5#5 (.reg 0#5) 2 {}, 113),
    (.vmovdquRR true .zmm512 3#5 3#5 { kmask := 1#3, zeroing := true, broadcast := false }, 119),
    (.vmovdquRR true .zmm512 2#5 2#5 { kmask := 2#3, zeroing := true, broadcast := false }, 125),
    (.movRR64 1#4 2#4, 128) ]

private def allAbsLE512BlkLoop16Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 26, 139) ]

private def allAbsLE512BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 169),
    (.cmpRR64 3#4 2#4, 172),
    (.jccRel .ge (-35), 174) ]

private def allAbsLE512BlkLoop8Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 143),
    (.vpand true .zmm512 4#5 1#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 149),
    (.vcmppdEvex .zmm512 1#3 4#5 (.reg 0#5) 2 {}, 156),
    (.vmovdquRR true .zmm512 3#5 3#5 { kmask := 1#3, zeroing := true, broadcast := false }, 162),
    (.movRR64 1#4 2#4, 165) ]

private def allAbsLE512BlkEpi0 : List (Amd64VecInsn × Nat) :=
  [ (.vpand true .zmm512 0#5 3#5 (.reg 2#5) {}, 180),
    (.vpmovq2m .zmm512 1#3 0#5, 186),
    (.kmovbToGpr 1#4 1#3, 190),
    (.nop, 192),
    (.cmpImm8 1#4 0xff#8, 195),
    (.jccRel .e 7, 197) ]

private def allAbsLE512BlkRetFalse : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 200),
    (.xorRR32 0#4 0#4, 202),
    (.popReg 5#4, 203),
    (.ret, 204) ]

private def allAbsLE512BlkRetTrue : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 207),
    (.movImm32 0#4 1#32, 212),
    (.popReg 5#4, 213),
    (.ret, 214) ]

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkPro0_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 0 allAbsLE512BlkPro0 = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkPro1_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 18 allAbsLE512BlkPro1 = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkLoop16Hdr_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 128 allAbsLE512BlkLoop16Hdr = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkLoop16Body_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 75 allAbsLE512BlkLoop16Body = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkLoop16Exit_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 137 allAbsLE512BlkLoop16Exit = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkLoop8Hdr_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 165 allAbsLE512BlkLoop8Hdr = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkLoop8Body_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 139 allAbsLE512BlkLoop8Body = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkEpi0_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 174 allAbsLE512BlkEpi0 = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkRetFalse_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 197 allAbsLE512BlkRetFalse = true := by
  decide

set_option maxRecDepth 2048 in
private theorem allAbsLE512BlkRetTrue_ok :
    checkBlockSeq allAbsLE64AVX512ByteChunks allAbsLE64AVX512Rodata 204 allAbsLE512BlkRetTrue = true := by
  decide

private theorem allAbsLE64AVX512_prologue (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running)
    (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64) :
    let s1 := runAmd64Vec 18 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
    s1.pc = 0x80 ∧ AllAbsLE512Inv base x limit n 0 s1 := by
  have hBe : evalX86Cond .be (x86TestFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = false :=
    evalX86Cond_be_test64_self hPos (by omega)
  have hRsp : s0.gpr 4#4 - 8#64 + 16#64 = s0.gpr 4#4 + 8#64 := by bv_omega
  have hJmp : nextTargetPc 75 53 = some 0x80 := by decide
  dsimp only [allAbsLE64AVX512Bytes]
  have hStep0 := runAmd64Vec_block_seq 13 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0 0 allAbsLE512BlkPro0 hRun hPc allAbsLE512BlkPro0_ok
  have hLen0 : 13 + allAbsLE512BlkPro0.length = 18 := rfl
  rw [← hLen0, hStep0]
  let sMid := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0 allAbsLE512BlkPro0
  change (runAmd64Vec 13 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid).pc = 0x80 ∧
    AllAbsLE512Inv base x limit n 0
      (runAmd64Vec 13 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hRun]
  have hMidPc : sMid.pc = 18 := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hRbx, hBe, hHaz]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRax, hRbx, hBe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidZ0 : (sMid.zmm 0#5).l0 = limit := by
    simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hLim]
  have hMidMem : ∀ j < n, sMid.mem (elemAddr base j) = x j := by
    intro j hj
    have hd := hDisj j hj
    have hMem0 : sMid.mem = write64 (write64 s0.mem (s0.gpr 4#4 - 8#64) (s0.gpr 5#4)) (s0.gpr 4#4 + 8#64) base := by
      simp [sMid, allAbsLE512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
        hRax, hRbx, hBe, hRsp]
    rw [hMem0, write64_ne _ _ hd.2, write64_ne _ _ hd.1]
    exact hX j hj
  have hStep1 := runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid 18 allAbsLE512BlkPro1 hMidRun hMidPc allAbsLE512BlkPro1_ok
  have hLen1 : 0 + allAbsLE512BlkPro1.length = 13 := rfl
  rw [← hLen1, hStep1]
  simp only [runAmd64Vec]
  refine ⟨by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp], ?_⟩
  refine ⟨by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, hMidRun],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, Amd64VecInsn.dirtiesUpper],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr, hMidRax],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setGpr, hMidRbx],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm,
         Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, hMidZ0],
    by simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm, setGpr,
         Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast],
    by intro j hj
       have hMemPro1 : (execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid allAbsLE512BlkPro1).mem = sMid.mem := by
         simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp]
       rw [hMemPro1]; exact hMidMem j hj,
    ?_⟩
  let ok := !f64IsNaN limit
  refine ⟨ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ok, ?_, ?_, ?_⟩
  · have hEq : Vec512.ofOpmask8 (packOpmask8 ok ok ok ok ok ok ok ok) =
        maskVec8 ok ok ok ok ok ok ok ok := ofOpmask8_packOpmask8 ok ok ok ok ok ok ok ok
    simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm, setKmask,
      Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, x86VcmppdPred, f64Eq_self_eq_notNaN,
      hMidZ0]
    simpa [packOpmask8, ok] using hEq
  · have hEq : Vec512.ofOpmask8 (packOpmask8 ok ok ok ok ok ok ok ok) =
        maskVec8 ok ok ok ok ok ok ok ok := ofOpmask8_packOpmask8 ok ok ok ok ok ok ok ok
    simp [allAbsLE512BlkPro1, execDecodedSeq, execAmd64VecInsn, evalVecRm, hJmp, setZmm, setKmask,
      Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, x86VcmppdPred, f64Eq_self_eq_notNaN,
      hMidZ0]
    simpa [packOpmask8, ok] using hEq
  · cases h : f64IsNaN limit <;> simp [ok, specAllAbsLE, h]

private theorem allAbsLE64AVX512_loop16_step (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x80) (hInv : AllAbsLE512Inv base x limit n i s)
    (hStep : i + 16 ≤ n) (hN : n + 32 < 2 ^ 63) :
    let s' := runAmd64Vec 13 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
    s'.pc = 0x80 ∧ AllAbsLE512Inv base x limit n (i + 16) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJcc : nextTargetPc 137 (-62) = some 75 := by decide
  dsimp only [allAbsLE64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 10 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s 128 allAbsLE512BlkLoop16Hdr hInv.statusRun hPc allAbsLE512BlkLoop16Hdr_ok
  have hL0 : 10 + allAbsLE512BlkLoop16Hdr.length = 13 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s allAbsLE512BlkLoop16Hdr
  change (runAmd64Vec 10 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid).pc = 0x80 ∧
    AllAbsLE512Inv base x limit n (i + 16)
      (runAmd64Vec 10 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 75 := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 16) := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid 75 allAbsLE512BlkLoop16Body hMidRun hMidPc allAbsLE512BlkLoop16Body_ok
  have hL1 : 0 + allAbsLE512BlkLoop16Body.length = 10 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.masks with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ3, hZ2, hProd⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hBaseAddr64 : elemAddr base i + 64#64 = elemAddr base (i + 8) := elemAddr_plus_64 base i
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega)
  have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega)
  have hX7 := hInv.memX (i + 7) (by omega)
  have hX8 := hInv.memX (i + 8) (by omega)
  have hX9 := hInv.memX (i + 9) (by omega)
  have hX10 := hInv.memX (i + 10) (by omega)
  have hX11 := hInv.memX (i + 11) (by omega)
  have hX12 := hInv.memX (i + 12) (by omega)
  have hX13 := hInv.memX (i + 13) (by omega)
  have hX14 := hInv.memX (i + 14) (by omega)
  have hX15 := hInv.memX (i + 15) (by omega)
  refine ⟨by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm0Eq],
    by simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨a0 && f64Le (f64Abs (x i)) limit,
          a1 && f64Le (f64Abs (x (i + 1))) limit,
          a2 && f64Le (f64Abs (x (i + 2))) limit,
          a3 && f64Le (f64Abs (x (i + 3))) limit,
          a4 && f64Le (f64Abs (x (i + 4))) limit,
          a5 && f64Le (f64Abs (x (i + 5))) limit,
          a6 && f64Le (f64Abs (x (i + 6))) limit,
          a7 && f64Le (f64Abs (x (i + 7))) limit,
          b0 && f64Le (f64Abs (x (i + 8))) limit,
          b1 && f64Le (f64Abs (x (i + 9))) limit,
          b2 && f64Le (f64Abs (x (i + 10))) limit,
          b3 && f64Le (f64Abs (x (i + 11))) limit,
          b4 && f64Le (f64Abs (x (i + 12))) limit,
          b5 && f64Le (f64Abs (x (i + 13))) limit,
          b6 && f64Le (f64Abs (x (i + 14))) limit,
          b7 && f64Le (f64Abs (x (i + 15))) limit, ?_, ?_, ?_⟩
  · have hK1 := applyEvexMask_zeroing_maskVec8 1#3 (by decide) a0 a1 a2 a3 a4 a5 a6 a7
      (f64Le (f64Abs (x i)) limit) (f64Le (f64Abs (x (i + 1))) limit)
      (f64Le (f64Abs (x (i + 2))) limit) (f64Le (f64Abs (x (i + 3))) limit)
      (f64Le (f64Abs (x (i + 4))) limit) (f64Le (f64Abs (x (i + 5))) limit)
      (f64Le (f64Abs (x (i + 6))) limit) (f64Le (f64Abs (x (i + 7))) limit)
    simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem,
      hInv.zmm0Eq, hInv.zmm1Eq, hZ3, hBaseAddr,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX0, hX1, hX2, hX3, hX4, hX5, hX6, hX7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm]
    simpa [packOpmask8] using hK1
  · have hK2 := applyEvexMask_zeroing_maskVec8 2#3 (by decide) b0 b1 b2 b3 b4 b5 b6 b7
      (f64Le (f64Abs (x (i + 8))) limit) (f64Le (f64Abs (x (i + 9))) limit)
      (f64Le (f64Abs (x (i + 10))) limit) (f64Le (f64Abs (x (i + 11))) limit)
      (f64Le (f64Abs (x (i + 12))) limit) (f64Le (f64Abs (x (i + 13))) limit)
      (f64Le (f64Abs (x (i + 14))) limit) (f64Le (f64Abs (x (i + 15))) limit)
    simp [allAbsLE512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem,
      hInv.zmm0Eq, hInv.zmm1Eq, hZ2, hBaseAddr, hBaseAddr64,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX8, hX9, hX10, hX11, hX12, hX13, hX14, hX15,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm]
    simpa [packOpmask8] using hK2
  · rw [specAllAbsLE_add16, specAllAbsLE_add8, ← hProd]
    ac_rfl

private theorem allAbsLE64AVX512_loop16_iter (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n : Nat) (hN : n + 32 < 2 ^ 63) :
    ∀ q i s, i + 16 * q ≤ n → s.pc = 0x80 → AllAbsLE512Inv base x limit n i s →
      let s' := runAmd64Vec (13 * q) allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
      s'.pc = 0x80 ∧ AllAbsLE512Inv base x limit n (i + 16 * q) s' := by
  intro q
  induction q with
  | zero =>
    intro i s _ hPc hInv
    simpa [runAmd64Vec] using And.intro hPc hInv
  | succ q ih =>
    intro i s hLe hPc hInv
    have hFuel : 13 * (q + 1) = 13 + 13 * q := by omega
    dsimp only
    rw [hFuel, runAmd64Vec_add]
    have h1 := allAbsLE64AVX512_loop16_step base x limit n i s hPc hInv (by omega) hN
    have h2 := ih (i + 16) _ (by omega) h1.1 h1.2
    have hIdx : i + 16 + 16 * q = i + 16 * (q + 1) := by omega
    simpa [hIdx] using h2

private theorem allAbsLE64AVX512_loop16_exit (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0x80) (hInv : AllAbsLE512Inv base x limit n i s)
    (hLt : n < i + 16) (hN : n + 32 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
    s'.pc = 0xa5 ∧ AllAbsLE512Inv base x limit n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJmp : nextTargetPc 139 26 = some 0xa5 := by decide
  dsimp only [allAbsLE64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 1 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s 128 allAbsLE512BlkLoop16Hdr hInv.statusRun hPc allAbsLE512BlkLoop16Hdr_ok
  have hL0 : 1 + allAbsLE512BlkLoop16Hdr.length = 4 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s allAbsLE512BlkLoop16Hdr
  change (runAmd64Vec 1 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid).pc = 0xa5 ∧
    AllAbsLE512Inv base x limit n i
      (runAmd64Vec 1 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 137 := by
    simp [sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid 137 allAbsLE512BlkLoop16Exit hMidRun hMidPc allAbsLE512BlkLoop16Exit_ok
  have hL1 : 0 + allAbsLE512BlkLoop16Exit.length = 1 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [allAbsLE512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp], ?_⟩
  refine ⟨by simp [allAbsLE512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMidRun],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.upperDirtyT],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.noHazard],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.zmm0Eq],
    by simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq,
         execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.memX j hj],
    by simpa [allAbsLE512BlkLoop16Exit, sMid, allAbsLE512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp] using hInv.masks⟩

private theorem allAbsLE64AVX512_loop8_step (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0xa5) (hInv : AllAbsLE512Inv base x limit n i s)
    (hStep : i + 8 ≤ n) (hN : n + 32 < 2 ^ 63) :
    let s' := runAmd64Vec 8 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
    s'.pc = 0xa5 ∧ AllAbsLE512Inv base x limit n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 174 (-35) = some 139 := by decide
  dsimp only [allAbsLE64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 5 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s 165 allAbsLE512BlkLoop8Hdr hInv.statusRun hPc allAbsLE512BlkLoop8Hdr_ok
  have hL0 : 5 + allAbsLE512BlkLoop8Hdr.length = 8 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s allAbsLE512BlkLoop8Hdr
  change (runAmd64Vec 5 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid).pc = 0xa5 ∧
    AllAbsLE512Inv base x limit n (i + 8)
      (runAmd64Vec 5 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 139 := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid 139 allAbsLE512BlkLoop8Body hMidRun hMidPc allAbsLE512BlkLoop8Body_ok
  have hL1 : 0 + allAbsLE512BlkLoop8Body.length = 5 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.masks with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ3, hZ2, hProd⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega)
  have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega)
  have hX7 := hInv.memX (i + 7) (by omega)
  refine ⟨by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm0Eq],
    by simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨a0 && f64Le (f64Abs (x i)) limit,
          a1 && f64Le (f64Abs (x (i + 1))) limit,
          a2 && f64Le (f64Abs (x (i + 2))) limit,
          a3 && f64Le (f64Abs (x (i + 3))) limit,
          a4 && f64Le (f64Abs (x (i + 4))) limit,
          a5 && f64Le (f64Abs (x (i + 5))) limit,
          a6 && f64Le (f64Abs (x (i + 6))) limit,
          a7 && f64Le (f64Abs (x (i + 7))) limit,
          b0, b1, b2, b3, b4, b5, b6, b7, ?_, ?_, ?_⟩
  · have hK1 := applyEvexMask_zeroing_maskVec8 1#3 (by decide) a0 a1 a2 a3 a4 a5 a6 a7
      (f64Le (f64Abs (x i)) limit) (f64Le (f64Abs (x (i + 1))) limit)
      (f64Le (f64Abs (x (i + 2))) limit) (f64Le (f64Abs (x (i + 3))) limit)
      (f64Le (f64Abs (x (i + 4))) limit) (f64Le (f64Abs (x (i + 5))) limit)
      (f64Le (f64Abs (x (i + 6))) limit) (f64Le (f64Abs (x (i + 7))) limit)
    simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem,
      hInv.zmm0Eq, hInv.zmm1Eq, hZ3, hBaseAddr,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX0, hX1, hX2, hX3, hX4, hX5, hX6, hX7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred, f64Abs_and_comm]
    simpa [packOpmask8] using hK1
  · simp [allAbsLE512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, hMidZmm, hZ2]
  · rw [specAllAbsLE_add8, ← hProd]
    ac_rfl

private theorem allAbsLE64AVX512_loop8_exit_and_ret (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 0xa5) (hInv : AllAbsLE512Inv base x limit n i s)
    (hLt : n < i + 8) (hN : n + 32 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 13 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s
    s'.status = .returned ∧
    s'.upperDirty = false ∧
    s'.sseHazard = false ∧
    s'.gpr 0#4 = (if specAllAbsLE x limit i then 1#64 else 0#64) := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  dsimp only [allAbsLE64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 10 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s 165 allAbsLE512BlkLoop8Hdr hInv.statusRun hPc allAbsLE512BlkLoop8Hdr_ok
  have hL0 : 10 + allAbsLE512BlkLoop8Hdr.length = 13 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s allAbsLE512BlkLoop8Hdr
  have hMidRun : sMid.status = .running := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 174 := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.noHazard]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, allAbsLE512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  change (runAmd64Vec 10 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid).status = .returned ∧ _
  have hS1 := runAmd64Vec_block_seq 4 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
    allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid 174 allAbsLE512BlkEpi0 hMidRun hMidPc allAbsLE512BlkEpi0_ok
  have hL1 : 4 + allAbsLE512BlkEpi0.length = 10 := rfl
  rw [← hL1, hS1]
  let sEpi := execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sMid allAbsLE512BlkEpi0
  rcases hInv.masks with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ3, hZ2, hProd⟩
  have hComb : (((maskVec8 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3)
        (a4 && b4) (a5 && b5) (a6 && b6) (a7 && b7)).toOpmask8.zeroExtend 64).extractLsb' 0 8 == 0xff#8) =
      specAllAbsLE x limit i := by
    rw [toOpmask8_maskVec8_eq_0xff, ← hProd]
    ac_rfl
  have hCmp : evalX86Cond .e (x86SubFlags8
      (((maskVec8 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3)
        (a4 && b4) (a5 && b5) (a6 && b6) (a7 && b7)).toOpmask8.zeroExtend 64).extractLsb' 0 8) 0xff#8) =
      specAllAbsLE x limit i := by
    rw [evalX86Cond_e_sub8, hComb]
  have hJcc7 : nextTargetPc 197 7 = some 204 := by decide
  have hMaskComb : (Vec512.applyWidth .zmm512
      (Vec512.applyEvexMask 0#3 (sMid.kmask 0#3) false (s.zmm 0#5)
        (Vec512.zipWith (· &&& ·) (maskVec8 a0 a1 a2 a3 a4 a5 a6 a7) (maskVec8 b0 b1 b2 b3 b4 b5 b6 b7)))) =
      maskVec8 (a0 && b0) (a1 && b1) (a2 && b2) (a3 && b3)
               (a4 && b4) (a5 && b5) (a6 && b6) (a7 && b7) := by
    simp [Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zipWith, maskVec8, maskLane64_and]
  have hEpiRun : sEpi.status = .running := by
    cases hSpec : specAllAbsLE x limit i <;>
      simp [sEpi, allAbsLE512BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7, hMidRun]
  have hEpiHaz : sEpi.sseHazard = false := by
    cases hSpec : specAllAbsLE x limit i <;>
      simp [sEpi, allAbsLE512BlkEpi0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
        evalVecRm, setZmm, setKmask, setGpr, hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7, hMidHaz]
  change (runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi).status = .returned ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi).upperDirty = false ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi).sseHazard = false ∧
    (runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi).gpr 0#4 =
      (if specAllAbsLE x limit i then 1#64 else 0#64)
  cases hSpec : specAllAbsLE x limit i with
  | false =>
    have hEpiPc : sEpi.pc = 197 := by
      simp [sEpi, allAbsLE512BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec]
    have hS2 : runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi =
        runAmd64Vec 0 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets
          (execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi allAbsLE512BlkRetFalse) :=
      runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
        allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi 197 allAbsLE512BlkRetFalse hEpiRun hEpiPc allAbsLE512BlkRetFalse_ok
    rw [hS2]
    simp [runAmd64Vec, allAbsLE512BlkRetFalse, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setGpr, hEpiHaz]
  | true =>
    have hEpiPc : sEpi.pc = 204 := by
      simp [sEpi, allAbsLE512BlkEpi0, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask, setGpr,
        hMidZmm, hZ3, hZ2, hMaskComb, hCmp, hSpec, hJcc7]
    have hS2 : runAmd64Vec 4 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi =
        runAmd64Vec 0 ⟨allAbsLE64AVX512ByteChunks.flatten⟩ allAbsLE64AVX512Rodata allAbsLE64AVX512Targets
          (execDecodedSeq allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi allAbsLE512BlkRetTrue) :=
      runAmd64Vec_block_seq 0 allAbsLE64AVX512ByteChunks allAbsLE64AVX512_chunks_wf
        allAbsLE64AVX512Rodata allAbsLE64AVX512Targets sEpi 204 allAbsLE512BlkRetTrue hEpiRun hEpiPc allAbsLE512BlkRetTrue_ok
    rw [hS2]
    simp [runAmd64Vec, allAbsLE512BlkRetTrue, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setGpr, hEpiHaz]

/-- Exact step count executed by `allAbsLE64AVX512` on a slice of length `n = 8q + r`. -/
def allAbsLE64AVX512Steps (n : Nat) : Nat :=
  18 + 13 * (n / 16) + 4 + (if 16 * (n / 16) + 8 ≤ n then 8 else 0) + 13

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`allAbsLE64AVX512`)**:
For arbitrary slice length `n = 8q + r` (`0 < n`, `n + 32 < 2^63`, including zero-trip cases
`n < 8` and `n < 16`), executing the raw compiled `GOARCH=amd64 GOEXPERIMENT=simd` EVEX 512-bit
ELF `.text` bytes of `internal/simd.allAbsLE64AVX512` (exercising EVEX opmask comparisons
`VCMPPD -> k1/k2`, zero-masking `VMOVDQU64 {k1}{z}/{k2}{z}`, `VPMOVM2Q`, `VPMOVQ2M`, and `KMOVB`):
1. Never hits `runtime.panicBounds`, `morestack`, or any external runtime/GC call;
2. Terminates with `status = .returned`, `upperDirty = false` (`VZEROUPPER` ABI safety), and
   `sseHazard = false`;
3. Returns bit-exact `RAX = if specAllAbsLE x limit (8 * (n / 8)) then 1 else 0`;
4. Preserves Go amd64 ABI registers: `RSP = s0.gpr 4#4 + 8#64`, `RBP = s0.gpr 5#4`,
   `R14 = s0.gpr 14#4`, and `X15` low 128 bits (`l0`, `l1`). -/
theorem allAbsLE64AVX512_functional_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running)
    (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64)
    (hFuel : allAbsLE64AVX512Steps n ≤ fuel) :
    let sf := runAmd64Vec fuel allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    sf.gpr 0#4 = (if specAllAbsLE x limit (8 * (n / 8)) then 1#64 else 0#64) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  have hPro := allAbsLE64AVX512_prologue base x limit n s0 hPc hRun hHaz hPos hN hRax hRbx hLim hX hDisj
  have hL16 := allAbsLE64AVX512_loop16_iter base x limit n hN (n / 16) 0 _ (by omega) hPro.1 hPro.2
  have hEx16 := allAbsLE64AVX512_loop16_exit base x limit n (16 * (n / 16)) _
    hL16.1 (by simpa using hL16.2) (by omega) hN (by omega)
  have hExact :
      let se := runAmd64Vec (allAbsLE64AVX512Steps n) allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      se.gpr 0#4 = (if specAllAbsLE x limit (8 * (n / 8)) then 1#64 else 0#64) := by
    unfold allAbsLE64AVX512Steps
    by_cases hRem : 16 * (n / 16) + 8 ≤ n
    · have h8Eq : 16 * (n / 16) + 8 = 8 * (n / 8) := by omega
      simp only [hRem, ↓reduceIte]
      rw [runAmd64Vec_add (18 + 13 * (n / 16) + 4 + 8) 13,
          runAmd64Vec_add (18 + 13 * (n / 16) + 4) 8,
          runAmd64Vec_add (18 + 13 * (n / 16)) 4,
          runAmd64Vec_add 18 (13 * (n / 16))]
      have hL8 := allAbsLE64AVX512_loop8_step base x limit n (16 * (n / 16)) _ hEx16.1 hEx16.2 hRem hN
      have hRet := allAbsLE64AVX512_loop8_exit_and_ret base x limit n (16 * (n / 16) + 8) _
        hL8.1 hL8.2 (by omega) hN (by omega)
      simpa [h8Eq] using hRet
    · have h8Eq : 16 * (n / 16) = 8 * (n / 8) := by omega
      simp only [hRem, ↓reduceIte, Nat.add_zero]
      rw [runAmd64Vec_add (18 + 13 * (n / 16) + 4) 13,
          runAmd64Vec_add (18 + 13 * (n / 16)) 4,
          runAmd64Vec_add 18 (13 * (n / 16))]
      have hRet := allAbsLE64AVX512_loop8_exit_and_ret base x limit n (16 * (n / 16)) _
        hEx16.1 hEx16.2 (by omega) hN (by omega)
      simpa [h8Eq] using hRet
  have hSplit : fuel = allAbsLE64AVX512Steps n + (fuel - allAbsLE64AVX512Steps n) := by omega
  have hAbi := allAbsLE64AVX512_abi_reg_preserved s0 fuel hPc hRun
  have hCore :
      let sf := runAmd64Vec fuel allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      sf.gpr 0#4 = (if specAllAbsLE x limit (8 * (n / 8)) then 1#64 else 0#64) := by
    dsimp only
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hExact.1])]
    exact hExact
  have hSpRbp := hAbi.2.2.2 hCore.1
  exact ⟨hCore.1, hCore.2.1, hCore.2.2.1, hCore.2.2.2, hSpRbp.1, hSpRbp.2, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

/-- Cross-ISA bit-exact equivalence between decoded AVX2 (`allAbsLE64AVX2`) and decoded
AVX-512 (`allAbsLE64AVX512`) on all 8-aligned non-empty slices (`n = 8q > 0`). -/
theorem allAbsLE64_avx2_eq_avx512_mult8 (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (q fuel2 fuel512 : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hq : 0 < q) (hN : 8 * q + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 (8 * q))
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < 8 * q, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < 8 * q, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64)
    (hFuel2 : allAbsLE64AVX2Steps (8 * q) ≤ fuel2)
    (hFuel512 : allAbsLE64AVX512Steps (8 * q) ≤ fuel512) :
    (runAmd64Vec fuel2 allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0).gpr 0#4 =
    (runAmd64Vec fuel512 allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0).gpr 0#4 := by
  have h2 := allAbsLE64AVX2_functional_equiv base x limit (8 * q) fuel2 s0
    hPc hRun hHaz (by omega) (by omega) hRax hRbx hLim hX hDisj hFuel2
  have h512 := allAbsLE64AVX512_functional_equiv base x limit (8 * q) fuel512 s0
    hPc hRun hHaz (by omega) hN hRax hRbx hLim hX hDisj hFuel512
  have hEq4 : 4 * (8 * q / 4) = 8 * q := by omega
  have hEq8 : 8 * (8 * q / 8) = 8 * q := by omega
  rw [h2.2.2.2.1, h512.2.2.2.1, hEq4, hEq8]

/-!
### Part VI.B: Inductive Functional Equivalence — `minNoNaN64AVX2` & `minNoNaN64AVX512`
-/

/-- Replace NaN lanes with `+Inf` (matching `vInf.IfElse(vd.IsNaN(), vd)` via `VCMPUNORDPD`+`VPBLENDVB`/`VPBLENDMQ`). -/
@[inline] def sanitizeNaNInf (v : BitVec 64) : BitVec 64 :=
  if f64IsNaN v then f64PosInf else v

/-- Single sequential non-NaN minimum update (`if v < acc then v else acc`). -/
@[inline] def minStepNoNaN (acc v : BitVec 64) : BitVec 64 :=
  if f64Lt v acc then v else acc

/-- Raw sequential non-NaN minimum over `x[0..m)` before signed-zero canonicalization. -/
def specMinNoNaNRaw (x : Nat → BitVec 64) : Nat → BitVec 64
  | 0 => f64PosInf
  | k + 1 => minStepNoNaN (specMinNoNaNRaw x k) (x k)

/-- Clean sequential IEEE-754 specification of `MinNoNaN64` (`MinNoNaN64Scalar`) over `x[0..n)`. -/
def specMinNoNaN64 (x : Nat → BitVec 64) (n : Nat) : BitVec 64 :=
  canonZero64 (specMinNoNaNRaw x n)

/-- Canonical total binary64 minimum (mapping NaN → `+Inf`, `-0.0` → `+0.0`); unconditionally
associative and commutative on all `BitVec 64`. -/
def cmin64 (a b : BitVec 64) : BitVec 64 :=
  canonZero64 (vminpdLane (sanitizeNaNInf a) (sanitizeNaNInf b))

theorem f64PosInf_notNaN : f64NotNaN f64PosInf = true := by decide

theorem sanitizeNaNInf_notNaN (v : BitVec 64) : f64NotNaN (sanitizeNaNInf v) = true := by
  unfold sanitizeNaNInf
  cases h : f64IsNaN v
  · simp [f64NotNaN, h]
  · simp [f64PosInf_notNaN]

private theorem sanitizeNaNInf_of_notNaN {v : BitVec 64} (h : f64NotNaN v = true) :
    sanitizeNaNInf v = v := by
  have hF : f64IsNaN v = false := by simpa [f64NotNaN] using h
  simp [sanitizeNaNInf, hF]

theorem vminpdLane_notNaN {a b : BitVec 64} (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64NotNaN (vminpdLane a b) = true := by
  unfold vminpdLane
  split <;> assumption

theorem minStepNoNaN_notNaN {a : BitVec 64} (v : BitVec 64) (ha : f64NotNaN a = true) :
    f64NotNaN (minStepNoNaN a v) = true := by
  unfold minStepNoNaN
  split
  · next hLt =>
    cases hNaN : f64IsNaN v
    · simp [f64NotNaN, hNaN]
    · unfold f64Lt at hLt; simp [hNaN] at hLt
  · exact ha

private theorem f64NumKey_vminpdLane {a b : BitVec 64}
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64NumKey (vminpdLane a b) =
      if BitVec.ult (f64NumKey a) (f64NumKey b) then f64NumKey a else f64NumKey b := by
  unfold vminpdLane
  rw [f64Lt_iff_ult_numKey a b ha hb]
  split <;> rfl

private theorem f64NumKey_cmin64 (a b : BitVec 64) :
    f64NumKey (cmin64 a b) =
      if BitVec.ult (f64NumKey (sanitizeNaNInf a)) (f64NumKey (sanitizeNaNInf b))
      then f64NumKey (sanitizeNaNInf a)
      else f64NumKey (sanitizeNaNInf b) := by
  unfold cmin64
  rw [f64NumKey_canonZero64]
  exact f64NumKey_vminpdLane (sanitizeNaNInf_notNaN a) (sanitizeNaNInf_notNaN b)

theorem cmin64_notNaN (a b : BitVec 64) : f64NotNaN (cmin64 a b) = true := by
  unfold cmin64
  exact canonZero64_notNaN (vminpdLane_notNaN (sanitizeNaNInf_notNaN a) (sanitizeNaNInf_notNaN b))

theorem cmin64_comm (a b : BitVec 64) : cmin64 a b = cmin64 b a := by
  unfold cmin64
  exact (vminpdLane_numKey_assoc_comm (sanitizeNaNInf a) (sanitizeNaNInf b) (sanitizeNaNInf a)
    (sanitizeNaNInf_notNaN a) (sanitizeNaNInf_notNaN b) (sanitizeNaNInf_notNaN a)).2.1

theorem cmin64_assoc (a b c : BitVec 64) : cmin64 (cmin64 a b) c = cmin64 a (cmin64 b c) := by
  have hL : cmin64 (cmin64 a b) c = canonZero64 (cmin64 (cmin64 a b) c) := by
    unfold cmin64; rw [canonZero64_idem]
  have hR : cmin64 a (cmin64 b c) = canonZero64 (cmin64 a (cmin64 b c)) := by
    unfold cmin64; rw [canonZero64_idem]
  rw [hL, hR]
  apply canonZero64_eq_of_numKey_eq
  rw [f64NumKey_cmin64, f64NumKey_cmin64,
      sanitizeNaNInf_of_notNaN (cmin64_notNaN a b),
      sanitizeNaNInf_of_notNaN (cmin64_notNaN b c),
      f64NumKey_cmin64, f64NumKey_cmin64]
  apply BitVec.eq_of_toNat_eq
  simp only [BitVec.ult_eq_decide]
  by_cases hab : (f64NumKey (sanitizeNaNInf a)).toNat < (f64NumKey (sanitizeNaNInf b)).toNat <;>
    by_cases hbc : (f64NumKey (sanitizeNaNInf b)).toNat < (f64NumKey (sanitizeNaNInf c)).toNat <;>
    by_cases hac : (f64NumKey (sanitizeNaNInf a)).toNat < (f64NumKey (sanitizeNaNInf c)).toNat <;>
    simp [hab, hbc, hac] <;> omega

instance : Std.Commutative cmin64 := ⟨cmin64_comm⟩
instance : Std.Associative cmin64 := ⟨cmin64_assoc⟩

theorem cmin64_vminpdLane_eq {a b : BitVec 64} (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    canonZero64 (vminpdLane a b) = cmin64 a b := by
  unfold cmin64
  rw [sanitizeNaNInf_of_notNaN ha, sanitizeNaNInf_of_notNaN hb]

theorem cmin64_vminpdLane_left {a b : BitVec 64} (c : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    cmin64 (vminpdLane a b) c = cmin64 (cmin64 a b) c := by
  have hL : cmin64 (vminpdLane a b) c = canonZero64 (cmin64 (vminpdLane a b) c) := by
    unfold cmin64; rw [canonZero64_idem]
  have hR : cmin64 (cmin64 a b) c = canonZero64 (cmin64 (cmin64 a b) c) := by
    unfold cmin64; rw [canonZero64_idem]
  rw [hL, hR]
  apply canonZero64_eq_of_numKey_eq
  rw [f64NumKey_cmin64, f64NumKey_cmin64,
      sanitizeNaNInf_of_notNaN (vminpdLane_notNaN ha hb),
      sanitizeNaNInf_of_notNaN (cmin64_notNaN a b),
      f64NumKey_vminpdLane ha hb, f64NumKey_cmin64,
      sanitizeNaNInf_of_notNaN ha, sanitizeNaNInf_of_notNaN hb]

theorem cmin64_vminpdLane_right (a : BitVec 64) {b c : BitVec 64}
    (hb : f64NotNaN b = true) (hc : f64NotNaN c = true) :
    cmin64 a (vminpdLane b c) = cmin64 a (cmin64 b c) := by
  rw [cmin64_comm a (vminpdLane b c), cmin64_vminpdLane_left a hb hc, cmin64_comm]

theorem cmin64_sanitize_left (a b : BitVec 64) : cmin64 (sanitizeNaNInf a) b = cmin64 a b := by
  unfold cmin64
  rw [sanitizeNaNInf_of_notNaN (sanitizeNaNInf_notNaN a)]

theorem cmin64_sanitize_right (a b : BitVec 64) : cmin64 a (sanitizeNaNInf b) = cmin64 a b := by
  rw [cmin64_comm, cmin64_sanitize_left, cmin64_comm]

theorem cmin64_canonZero_left (a b : BitVec 64) : cmin64 (canonZero64 a) b = cmin64 a b := by
  have hL : cmin64 (canonZero64 a) b = canonZero64 (cmin64 (canonZero64 a) b) := by
    unfold cmin64; rw [canonZero64_idem]
  have hR : cmin64 a b = canonZero64 (cmin64 a b) := by
    unfold cmin64; rw [canonZero64_idem]
  rw [hL, hR]
  apply canonZero64_eq_of_numKey_eq
  rw [f64NumKey_cmin64, f64NumKey_cmin64]
  have hK : f64NumKey (sanitizeNaNInf (canonZero64 a)) = f64NumKey (sanitizeNaNInf a) := by
    cases hNaN : f64IsNaN a
    · have ha : f64NotNaN a = true := by simp [f64NotNaN, hNaN]
      rw [sanitizeNaNInf_of_notNaN (canonZero64_notNaN ha), sanitizeNaNInf_of_notNaN ha,
          f64NumKey_canonZero64]
    · have hNotZ : f64IsZero a = false := f64IsZero_false_of_isNaN hNaN
      simp [canonZero64, hNotZ]
  rw [hK]

theorem cmin64_canonZero_right (a b : BitVec 64) : cmin64 a (canonZero64 b) = cmin64 a b := by
  rw [cmin64_comm, cmin64_canonZero_left, cmin64_comm]

theorem cmin64_posInf_left (a : BitVec 64) : cmin64 f64PosInf a = canonZero64 (sanitizeNaNInf a) := by
  unfold cmin64
  rw [sanitizeNaNInf_of_notNaN f64PosInf_notNaN]
  have hLt : f64Lt f64PosInf (sanitizeNaNInf a) = false :=
    (f64PosInf_top_of_notNaN (sanitizeNaNInf a) (sanitizeNaNInf_notNaN a)).2.2.1
  simp [vminpdLane, hLt]

theorem cmin64_posInf_posInf : cmin64 f64PosInf f64PosInf = f64PosInf := by decide

theorem cmin64_minStepNoNaN {a : BitVec 64} (v : BitVec 64) (ha : f64NotNaN a = true) :
    canonZero64 (minStepNoNaN a v) = cmin64 a v := by
  cases hNaN : f64IsNaN v
  · have hv : f64NotNaN v = true := by simp [f64NotNaN, hNaN]
    have hEq : minStepNoNaN a v = vminpdLane v a := rfl
    rw [hEq, cmin64_vminpdLane_eq hv ha, cmin64_comm]
  · have hLtF : f64Lt v a = false := by unfold f64Lt; simp [hNaN]
    unfold minStepNoNaN
    simp only [hLtF, Bool.false_eq_true, ↓reduceIte]
    rw [cmin64_comm]
    have hSan : sanitizeNaNInf v = f64PosInf := by simp [sanitizeNaNInf, hNaN]
    rw [← cmin64_sanitize_left, hSan, cmin64_posInf_left, sanitizeNaNInf_of_notNaN ha]

theorem cmin64_f64Min {a b : BitVec 64} (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    canonZero64 (f64Min a b) = cmin64 a b := by
  have hB := (vminpd_track5d_canon_bridge a b ha hb).2.2.1
  rw [← hB, cmin64_vminpdLane_eq ha hb]

theorem specMinNoNaNRaw_notNaN (x : Nat → BitVec 64) (i : Nat) :
    f64NotNaN (specMinNoNaNRaw x i) = true := by
  induction i with
  | zero => exact f64PosInf_notNaN
  | succ k ih => exact minStepNoNaN_notNaN (x k) ih

theorem specMinNoNaN64_zero (x : Nat → BitVec 64) : specMinNoNaN64 x 0 = f64PosInf := by
  unfold specMinNoNaN64 specMinNoNaNRaw; decide

theorem specMinNoNaN64_succ (x : Nat → BitVec 64) (i : Nat) :
    specMinNoNaN64 x (i + 1) = cmin64 (specMinNoNaN64 x i) (x i) := by
  unfold specMinNoNaN64
  rw [specMinNoNaNRaw, cmin64_minStepNoNaN (x i) (specMinNoNaNRaw_notNaN x i), cmin64_canonZero_left]

private theorem specMinNoNaN64_add4 (x : Nat → BitVec 64) (i : Nat) :
    specMinNoNaN64 x (i + 4) =
      cmin64 (cmin64 (cmin64 (cmin64 (specMinNoNaN64 x i) (x i)) (x (i + 1))) (x (i + 2))) (x (i + 3)) := by
  simp only [specMinNoNaN64_succ]

private theorem specMinNoNaN64_add8 (x : Nat → BitVec 64) (i : Nat) :
    specMinNoNaN64 x (i + 8) =
      cmin64 (cmin64 (cmin64 (cmin64 (specMinNoNaN64 x (i + 4)) (x (i + 4))) (x (i + 5))) (x (i + 6))) (x (i + 7)) := by
  simp only [specMinNoNaN64_succ]

private theorem specMinNoNaN64_add16 (x : Nat → BitVec 64) (i : Nat) :
    specMinNoNaN64 x (i + 16) =
      cmin64 (cmin64 (cmin64 (cmin64
        (cmin64 (cmin64 (cmin64 (cmin64 (specMinNoNaN64 x (i + 8)) (x (i + 8))) (x (i + 9))) (x (i + 10))) (x (i + 11)))
        (x (i + 12))) (x (i + 13))) (x (i + 14))) (x (i + 15)) := by
  simp only [specMinNoNaN64_succ]

private theorem cmin64_combine4_eq {a0 a1 a2 a3 b0 b1 b2 b3 : BitVec 64}
    (ha0 : f64NotNaN a0 = true) (ha1 : f64NotNaN a1 = true)
    (ha2 : f64NotNaN a2 = true) (ha3 : f64NotNaN a3 = true)
    (hb0 : f64NotNaN b0 = true) (hb1 : f64NotNaN b1 = true)
    (hb2 : f64NotNaN b2 = true) (hb3 : f64NotNaN b3 = true) :
    cmin64 (cmin64 (vminpdLane a0 b0) (vminpdLane a1 b1))
           (cmin64 (vminpdLane a2 b2) (vminpdLane a3 b3)) =
    cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3))
           (cmin64 (cmin64 b0 b1) (cmin64 b2 b3)) := by
  rw [cmin64_vminpdLane_left _ ha0 hb0, cmin64_vminpdLane_right _ ha1 hb1,
      cmin64_vminpdLane_left _ ha2 hb2, cmin64_vminpdLane_right _ ha3 hb3]
  ac_rfl

private theorem cmin64_combine8_eq
    {a0 a1 a2 a3 a4 a5 a6 a7 b0 b1 b2 b3 b4 b5 b6 b7 : BitVec 64}
    (ha0 : f64NotNaN a0 = true) (ha1 : f64NotNaN a1 = true)
    (ha2 : f64NotNaN a2 = true) (ha3 : f64NotNaN a3 = true)
    (ha4 : f64NotNaN a4 = true) (ha5 : f64NotNaN a5 = true)
    (ha6 : f64NotNaN a6 = true) (ha7 : f64NotNaN a7 = true)
    (hb0 : f64NotNaN b0 = true) (hb1 : f64NotNaN b1 = true)
    (hb2 : f64NotNaN b2 = true) (hb3 : f64NotNaN b3 = true)
    (hb4 : f64NotNaN b4 = true) (hb5 : f64NotNaN b5 = true)
    (hb6 : f64NotNaN b6 = true) (hb7 : f64NotNaN b7 = true) :
    cmin64
      (cmin64 (cmin64 (vminpdLane a0 b0) (vminpdLane a1 b1))
              (cmin64 (vminpdLane a2 b2) (vminpdLane a3 b3)))
      (cmin64 (cmin64 (vminpdLane a4 b4) (vminpdLane a5 b5))
              (cmin64 (vminpdLane a6 b6) (vminpdLane a7 b7))) =
    cmin64
      (cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3))
              (cmin64 (cmin64 a4 a5) (cmin64 a6 a7)))
      (cmin64 (cmin64 (cmin64 b0 b1) (cmin64 b2 b3))
              (cmin64 (cmin64 b4 b5) (cmin64 b6 b7))) := by
  rw [cmin64_vminpdLane_left _ ha0 hb0, cmin64_vminpdLane_right _ ha1 hb1,
      cmin64_vminpdLane_left _ ha2 hb2, cmin64_vminpdLane_right _ ha3 hb3,
      cmin64_vminpdLane_left _ ha4 hb4, cmin64_vminpdLane_right _ ha5 hb5,
      cmin64_vminpdLane_left _ ha6 hb6, cmin64_vminpdLane_right _ ha7 hb7]
  ac_rfl

private theorem vpblendvb64_isNaN_inf (v : BitVec 64) :
    vpblendvb64 v f64PosInf (if f64IsNaN v = true then 0xffffffffffffffff#64 else 0#64) =
      sanitizeNaNInf v := by
  have hBlend := vpblendvb64_maskLane v f64PosInf (f64IsNaN v)
  simpa [maskLane64, sanitizeNaNInf] using hBlend

private theorem cmin64_minStep_fold {a : BitVec 64} (v : BitVec 64) (ha : f64NotNaN a = true) :
    canonZero64 (minStepNoNaN a v) = cmin64 (canonZero64 a) v := by
  rw [cmin64_minStepNoNaN v ha, cmin64_canonZero_left]

private theorem maskBit_or8_bv (c0 c1 c2 c3 c4 c5 c6 c7 : Bool) :
    let kv := BitVec.ofNat 8 (if c0 then 1 else 0) |||
      BitVec.ofNat 8 (if c1 then 2 else 0) |||
      BitVec.ofNat 8 (if c2 then 4 else 0) |||
      BitVec.ofNat 8 (if c3 then 8 else 0) |||
      BitVec.ofNat 8 (if c4 then 16 else 0) |||
      BitVec.ofNat 8 (if c5 then 32 else 0) |||
      BitVec.ofNat 8 (if c6 then 64 else 0) |||
      BitVec.ofNat 8 (if c7 then 128 else 0)
    Vec512.maskBit kv 0 = c0 ∧ Vec512.maskBit kv 1 = c1 ∧
    Vec512.maskBit kv 2 = c2 ∧ Vec512.maskBit kv 3 = c3 ∧
    Vec512.maskBit kv 4 = c4 ∧ Vec512.maskBit kv 5 = c5 ∧
    Vec512.maskBit kv 6 = c6 ∧ Vec512.maskBit kv 7 = c7 := by
  cases c0 <;> cases c1 <;> cases c2 <;> cases c3 <;>
    cases c4 <;> cases c5 <;> cases c6 <;> cases c7 <;> decide

private theorem minNoNaN64AVX2_chunks_wf : chunks16WellFormed minNoNaN64AVX2ByteChunks = true := by
  decide

private theorem minNoNaN64AVX512_chunks_wf : chunks16WellFormed minNoNaN64AVX512ByteChunks = true := by
  decide

/-- Vector loop invariant at `pc = 101` (8-wide header) and `pc = 138` (4-wide header) of `minNoNaN64AVX2`. -/
private structure MinNoNaN2Inv (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = base
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  rspEq : s.gpr 4#4 = spF
  zmm1Eq : s.zmm 1#5 = (Vec512.broadcast f64PosInf).zeroHi256
  memX : ∀ j < n, s.mem (elemAddr base j) = x j
  accs : ∃ a0 a1 a2 a3 b0 b1 b2 b3 : BitVec 64,
    s.zmm 0#5 = { l0 := a0, l1 := a1, l2 := a2, l3 := a3 } ∧
    s.zmm 2#5 = { l0 := b0, l1 := b1, l2 := b2, l3 := b3 } ∧
    f64NotNaN a0 = true ∧ f64NotNaN a1 = true ∧ f64NotNaN a2 = true ∧ f64NotNaN a3 = true ∧
    f64NotNaN b0 = true ∧ f64NotNaN b1 = true ∧ f64NotNaN b2 = true ∧ f64NotNaN b3 = true ∧
    cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3)) (cmin64 (cmin64 b0 b1) (cmin64 b2 b3)) =
      specMinNoNaN64 x i

private def min2BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 1),
    (.movRR64 5#4 4#4, 4),
    (.subImm64 4#4 0x40#64, 8),
    (.movMR64 (.baseIdxDisp (some 4#4) none 80) 0#4, 13),
    (.testRR64 3#4 3#4, 16),
    (.jccRel .be 256, 22) ]

private def min2BlkPro1 : List (Amd64VecInsn × Nat) :=
  [ (.nop, 23),
    (.vmovsdLoad 0#5 (.ripRel 323425), 31),
    (.vbroadcastsd false .ymm256 0#5 (.reg 0#5) {}, 36),
    (.xorRR32 1#4 1#4, 38),
    (.vmovdquRR false .ymm256 1#5 0#5 {}, 42),
    (.vmovdquRR false .ymm256 2#5 1#5 {}, 46),
    (.jmpRel 53, 48) ]

private def min2BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 105),
    (.cmpRR64 3#4 2#4, 108),
    (.jccRel .ge (-62), 110) ]

private def min2BlkLoop8Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 52),
    (.vmovdquLoad false .ymm256 3#5 (.baseIdxDisp (some 6#4) none 0) {}, 56),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 60),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 32), 64),
    (.vmovdquLoad false .ymm256 4#5 (.baseIdxDisp (some 6#4) none 0) {}, 68),
    (.vcmppdVex .ymm256 5#5 3#5 (.reg 3#5) 3, 73),
    (.vcmppdVex .ymm256 6#5 4#5 (.reg 4#5) 3, 78),
    (.vpblendvb .ymm256 3#5 3#5 (.reg 1#5) 5#5, 84),
    (.vpblendvb .ymm256 4#5 4#5 (.reg 1#5) 6#5, 90),
    (.vminpd false .ymm256 0#5 3#5 (.reg 0#5) {}, 94),
    (.vminpd false .ymm256 2#5 4#5 (.reg 2#5) {}, 98),
    (.movRR64 1#4 2#4, 101) ]

private def min2BlkLoop8Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 26, 112) ]

private def min2BlkLoop4Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 4), 142),
    (.cmpRR64 3#4 2#4, 145),
    (.jccRel .ge (-35), 147) ]

private def min2BlkLoop4Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 116),
    (.vmovdquLoad false .ymm256 3#5 (.baseIdxDisp (some 6#4) none 0) {}, 120),
    (.vcmppdVex .ymm256 4#5 3#5 (.reg 3#5) 3, 125),
    (.vpblendvb .ymm256 3#5 3#5 (.reg 1#5) 4#5, 131),
    (.vminpd false .ymm256 0#5 0#5 (.reg 3#5) {}, 135),
    (.movRR64 1#4 2#4, 138) ]

private def min2BlkSpillA : List (Amd64VecInsn × Nat) :=
  [ (.vminpd false .ymm256 0#5 0#5 (.reg 2#5) {}, 151),
    (.vmovdquStore false .ymm256 (.baseIdxDisp (some 4#4) none 0) 0#5 {}, 156),
    (.vzeroupper, 159),
    (.nop, 160), (.nop, 161),
    (.lea64 2#4 (.baseIdxDisp (some 4#4) none 32), 166),
    (.lea64 6#4 (.baseIdxDisp (some 4#4) none 0), 170) ]

private def min2BlkSpillB : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 0), 174),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 0) 14#4, 178),
    (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 16), 183),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 16) 14#4, 188),
    (.xorRR32 2#4 2#4, 190),
    (.sseMovsdLoad 0#4 (.ripRel 323258), 198),
    (.jmpRel 6, 200) ]

private def min2BlkHorizHdr : List (Amd64VecInsn × Nat) :=
  [ (.cmpImm64 2#4 4#64, 210),
    (.jccRel .ge 25, 212) ]

private def min2BlkHorizCmp : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) (some (2#4, 8)) 32), 218),
    (.sseUcomisdRR 0#4 1#4, 222),
    (.nop, 224),
    (.jccRel .a (-26), 226) ]

private def min2BlkHorizKeep : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 0#4, 229),
    (.jmpRel (-31), 231) ]

private def min2BlkHorizInc : List (Amd64VecInsn × Nat) :=
  [ (.inc64 2#4, 203),
    (.sseMovupsRR 0#4 1#4, 206) ]

private def min2BlkTailHdr : List (Amd64VecInsn × Nat) :=
  [ (.cmpRR64 3#4 1#4, 240),
    (.jccRel .le 16, 242) ]

private def min2BlkTailCmp : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 247),
    (.sseUcomisdRR 0#4 1#4, 251),
    (.jccRel .a (-22), 253) ]

private def min2BlkTailKeep : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 0#4, 256),
    (.jmpRel (-27), 258) ]

private def min2BlkTailInc : List (Amd64VecInsn × Nat) :=
  [ (.inc64 1#4, 234),
    (.sseMovupsRR 0#4 1#4, 237) ]

private def min2BlkCanon0 : List (Amd64VecInsn × Nat) :=
  [ (.sseXorpsRR 1#4 1#4, 261),
    (.sseUcomisdRR 0#4 1#4, 265),
    (.jccRel .ne 5, 267) ]

private def min2BlkCanonJp : List (Amd64VecInsn × Nat) :=
  [ (.jccRel .p 3, 269) ]

private def min2BlkCanonZero : List (Amd64VecInsn × Nat) :=
  [ (.sseXorpsRR 0#4 0#4, 272) ]

private def min2BlkRet : List (Amd64VecInsn × Nat) :=
  [ (.addImm64 4#4 0x40#64, 276),
    (.popReg 5#4, 277),
    (.ret, 278) ]

set_option maxRecDepth 2048 in
private theorem min2BlkPro0_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 0 min2BlkPro0 = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkPro1_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 22 min2BlkPro1 = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkLoop8Hdr_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 101 min2BlkLoop8Hdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkLoop8Body_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 48 min2BlkLoop8Body = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkLoop8Exit_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 110 min2BlkLoop8Exit = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkLoop4Hdr_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 138 min2BlkLoop4Hdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkLoop4Body_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 112 min2BlkLoop4Body = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkSpillA_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 147 min2BlkSpillA = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkSpillB_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 170 min2BlkSpillB = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkHorizHdr_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 206 min2BlkHorizHdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkHorizCmp_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 212 min2BlkHorizCmp = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkHorizKeep_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 226 min2BlkHorizKeep = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkHorizInc_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 200 min2BlkHorizInc = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkTailHdr_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 237 min2BlkTailHdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkTailCmp_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 242 min2BlkTailCmp = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkTailKeep_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 253 min2BlkTailKeep = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkTailInc_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 231 min2BlkTailInc = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkCanon0_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 258 min2BlkCanon0 = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkCanonJp_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 267 min2BlkCanonJp = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkCanonZero_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 269 min2BlkCanonZero = true := by decide
set_option maxRecDepth 2048 in
private theorem min2BlkRet_ok : checkBlockSeq minNoNaN64AVX2ByteChunks minNoNaN64AVX2Rodata 272 min2BlkRet = true := by decide

private theorem minNoNaN64AVX2_prologue (base : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64) :
    let s1 := runAmd64Vec 13 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
    s1.pc = 101 ∧ MinNoNaN2Inv base (s0.gpr 4#4 - 72#64) x n 0 s1 := by
  have hBe : evalX86Cond .be (x86TestFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = false :=
    evalX86Cond_be_test64_self hPos (by omega)
  have hSpF : s0.gpr 4#4 - 8#64 - 0x40#64 = s0.gpr 4#4 - 72#64 := by bv_omega
  have hSp80 : s0.gpr 4#4 - 8#64 - 0x40#64 + 80#64 = s0.gpr 4#4 + 8#64 := by bv_omega
  have hJmp : nextTargetPc 48 53 = some 101 := by decide
  have hRo23 : minNoNaN64AVX2Rodata 23 = some f64PosInf := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 7 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0 0 min2BlkPro0 hRun hPc min2BlkPro0_ok
  have hL0 : 7 + min2BlkPro0.length = 13 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0 min2BlkPro0
  change (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).pc = 101 ∧
    MinNoNaN2Inv base (s0.gpr 4#4 - 72#64) x n 0
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hRun]
  have hMidPc : sMid.pc = 22 := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hRbx, hBe, hHaz]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRax, hRbx, hBe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe]
  have hMidRsp : sMid.gpr 4#4 = s0.gpr 4#4 - 72#64 := by
    simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx, hBe, hSpF]
  have hMidMem : ∀ j < n, sMid.mem (elemAddr base j) = x j := by
    intro j hj
    have hd := hDisj j hj
    have hMem0 : sMid.mem = write64 (write64 s0.mem (s0.gpr 4#4 - 8#64) (s0.gpr 5#4)) (s0.gpr 4#4 + 8#64) base := by
      simp [sMid, min2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
        hRax, hRbx, hBe, hSp80]
    rw [hMem0, write64_ne _ _ hd.2, write64_ne _ _ hd.1]
    exact hX j hj
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid 22 min2BlkPro1 hMidRun hMidPc min2BlkPro1_ok
  have hL1 : 0 + min2BlkPro1.length = 7 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp], ?_⟩
  refine ⟨by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, hMidRun],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, Amd64VecInsn.dirtiesUpper],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setGpr, hMidRax],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setGpr],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setGpr, hMidRbx],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setGpr, hMidRsp],
    by simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setZmm,
         applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256],
    by intro j hj
       have hM : (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid min2BlkPro1).mem = sMid.mem := by
         simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp]
       rw [hM]; exact hMidMem j hj,
    ?_⟩
  refine ⟨f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
    ?_, ?_, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
    f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, ?_⟩
  · simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setZmm,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256]
  · simp [min2BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo23, hJmp, setZmm,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zeroHi256]
  · rw [specMinNoNaN64_zero]; simp [cmin64_posInf_posInf]

private theorem minNoNaN64AVX2_loop8_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 101) (hInv : MinNoNaN2Inv base spF x n i s)
    (hStep : i + 8 ≤ n) (hN : n + 16 < 2 ^ 63) :
    let s' := runAmd64Vec 15 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.pc = 101 ∧ MinNoNaN2Inv base spF x n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 110 (-62) = some 48 := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 12 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 101 min2BlkLoop8Hdr hInv.statusRun hPc min2BlkLoop8Hdr_ok
  have hL0 : 12 + min2BlkLoop8Hdr.length = 15 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkLoop8Hdr
  change (runAmd64Vec 12 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).pc = 101 ∧
    MinNoNaN2Inv base spF x n (i + 8)
      (runAmd64Vec 12 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 48 := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid 48 min2BlkLoop8Body hMidRun hMidPc min2BlkLoop8Body_ok
  have hL1 : 0 + min2BlkLoop8Body.length = 12 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.accs with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ0, hZ2,
    ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hBaseAddr32 : elemAddr base i + 32#64 = elemAddr base (i + 4) := elemAddr_plus_32 base i
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega)
  have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega)
  have hX7 := hInv.memX (i + 7) (by omega)
  refine ⟨by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp],
    by simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨vminpdLane (sanitizeNaNInf (x i)) a0,
          vminpdLane (sanitizeNaNInf (x (i + 1))) a1,
          vminpdLane (sanitizeNaNInf (x (i + 2))) a2,
          vminpdLane (sanitizeNaNInf (x (i + 3))) a3,
          vminpdLane (sanitizeNaNInf (x (i + 4))) b0,
          vminpdLane (sanitizeNaNInf (x (i + 5))) b1,
          vminpdLane (sanitizeNaNInf (x (i + 6))) b2,
          vminpdLane (sanitizeNaNInf (x (i + 7))) b3,
    ?_, ?_,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha0,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha1,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha2,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha3,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb0,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb1,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb2,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb3, ?_⟩
  · simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ0,
      hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX0, hX1, hX2, hX3, applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, vpblendvb64_isNaN_inf]
  · simp [min2BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ2,
      hBaseAddr, hBaseAddr32, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX4, hX5, hX6, hX7, applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, vpblendvb64_isNaN_inf]
  · rw [cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha0,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha1,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha2,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha3,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb0,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb1,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb2,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb3]
    simp only [cmin64_sanitize_left]
    rw [specMinNoNaN64_add8, specMinNoNaN64_add4, ← hCmin]
    ac_rfl

private theorem minNoNaN64AVX2_loop8_iter (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (hN : n + 16 < 2 ^ 63) :
    ∀ q i s, i + 8 * q ≤ n → s.pc = 101 → MinNoNaN2Inv base spF x n i s →
      let s' := runAmd64Vec (15 * q) minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
      s'.pc = 101 ∧ MinNoNaN2Inv base spF x n (i + 8 * q) s' := by
  intro q
  induction q with
  | zero =>
    intro i s _ hPc hInv
    simpa [runAmd64Vec] using And.intro hPc hInv
  | succ q ih =>
    intro i s hLe hPc hInv
    have hFuel : 15 * (q + 1) = 15 + 15 * q := by omega
    dsimp only
    rw [hFuel, runAmd64Vec_add]
    have h1 := minNoNaN64AVX2_loop8_step base spF x n i s hPc hInv (by omega) hN
    have h2 := ih (i + 8) _ (by omega) h1.1 h1.2
    have hIdx : i + 8 + 8 * q = i + 8 * (q + 1) := by omega
    simpa [hIdx] using h2

private theorem minNoNaN64AVX2_loop8_exit (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 101) (hInv : MinNoNaN2Inv base spF x n i s)
    (hLt : n < i + 8) (hN : n + 16 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.pc = 138 ∧ MinNoNaN2Inv base spF x n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJmp : nextTargetPc 112 26 = some 138 := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 1 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 101 min2BlkLoop8Hdr hInv.statusRun hPc min2BlkLoop8Hdr_ok
  have hL0 : 1 + min2BlkLoop8Hdr.length = 4 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkLoop8Hdr
  change (runAmd64Vec 1 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).pc = 138 ∧
    MinNoNaN2Inv base spF x n i
      (runAmd64Vec 1 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 110 := by
    simp [sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid 110 min2BlkLoop8Exit hMidRun hMidPc min2BlkLoop8Exit_ok
  have hL1 : 0 + min2BlkLoop8Exit.length = 1 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [min2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp], ?_⟩
  refine ⟨by simp [min2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMidRun],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.upperDirtyT],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.noHazard],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
    by simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.zmm1Eq],
    by intro j hj; simp [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.memX j hj],
    by simpa [min2BlkLoop8Exit, sMid, min2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp] using hInv.accs⟩

private theorem minNoNaN64AVX2_loop4_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 138) (hInv : MinNoNaN2Inv base spF x n i s)
    (hStep : i + 4 ≤ n) (hN : n + 16 < 2 ^ 63) :
    let s' := runAmd64Vec 9 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.pc = 138 ∧ MinNoNaN2Inv base spF x n (i + 4) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  have hJcc : nextTargetPc 147 (-35) = some 112 := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 6 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 138 min2BlkLoop4Hdr hInv.statusRun hPc min2BlkLoop4Hdr_ok
  have hL0 : 6 + min2BlkLoop4Hdr.length = 9 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkLoop4Hdr
  change (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).pc = 138 ∧
    MinNoNaN2Inv base spF x n (i + 4)
      (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 112 := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 4) := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid 112 min2BlkLoop4Body hMidRun hMidPc min2BlkLoop4Body_ok
  have hL1 : 0 + min2BlkLoop4Body.length = 6 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.accs with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ0, hZ2,
    ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hX0 := hInv.memX i (by omega)
  have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega)
  have hX3 := hInv.memX (i + 3) (by omega)
  refine ⟨by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp],
    by simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  -- Note Go 1.27 flips `VMINPD` operand order here (`vminpdLane a_k (sanitizeNaNInf x[i+k])`).
  refine ⟨vminpdLane a0 (sanitizeNaNInf (x i)),
          vminpdLane a1 (sanitizeNaNInf (x (i + 1))),
          vminpdLane a2 (sanitizeNaNInf (x (i + 2))),
          vminpdLane a3 (sanitizeNaNInf (x (i + 3))),
          b0, b1, b2, b3,
    ?_, ?_,
    vminpdLane_notNaN ha0 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha1 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha2 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha3 (sanitizeNaNInf_notNaN _),
    hb0, hb1, hb2, hb3, ?_⟩
  · simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ0,
      hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hX0, hX1, hX2, hX3, applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast,
      Vec512.zeroHi256, Vec512.zipWith, x86VcmppdPred, vpblendvb64_isNaN_inf]
  · simp [min2BlkLoop4Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, hMidZmm, hZ2]
  · rw [cmin64_vminpdLane_left _ ha0 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha1 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_left _ ha2 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha3 (sanitizeNaNInf_notNaN _)]
    simp only [cmin64_sanitize_right]
    rw [specMinNoNaN64_add4, ← hCmin]
    ac_rfl

/-- Scalar post-`VZEROUPPER` state invariant for the 4-lane horizontal & tail loops of `minNoNaN64AVX2`. -/
private structure MinNoNaN2ScalarInv (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (acc : BitVec 64) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyF : s.upperDirty = false
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = base
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  rspEq : s.gpr 4#4 = spF
  xmm0Eq : (s.zmm 0#5).l0 = acc
  accNotNaN : f64NotNaN acc = true
  memX : ∀ j < n, s.mem (elemAddr base j) = x j

private theorem stackSlot_ne_offset (spF : BitVec 64) (k1 k2 : Nat)
    (hNe : k1 ≠ k2) (hk1 : k1 < 16) (hk2 : k2 < 16) :
    spF + BitVec.ofNat 64 (k1 * 8) ≠ spF + BitVec.ofNat 64 (k2 * 8) := by
  intro hEq
  have hDiff : BitVec.ofNat 64 (k1 * 8) = BitVec.ofNat 64 (k2 * 8) := by
    bv_omega
  have hNat := congrArg BitVec.toNat hDiff
  simp only [BitVec.toNat_ofNat] at hNat
  rw [Nat.mod_eq_of_lt (by omega), Nat.mod_eq_of_lt (by omega)] at hNat
  omega

private theorem minNoNaN64AVX2_spill_and_vzu (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 138) (hInv : MinNoNaN2Inv base spF x n i s)
    (hLt : n < i + 4) (hN : n + 16 < 2 ^ 63) (hi : i ≤ n)
    (hDisjStack : ∀ j < n, ∀ k < 8, elemAddr base j ≠ spF + BitVec.ofNat 64 (k * 8)) :
    let s' := runAmd64Vec 17 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.pc = 206 ∧ s'.gpr 2#4 = 0#64 ∧
    MinNoNaN2ScalarInv base spF x n i f64PosInf s' ∧
    ∃ l0 l1 l2 l3 : BitVec 64,
      s'.mem (spF + 32#64) = l0 ∧ s'.mem (spF + 40#64) = l1 ∧
      s'.mem (spF + 48#64) = l2 ∧ s'.mem (spF + 56#64) = l3 ∧
      f64NotNaN l0 = true ∧ f64NotNaN l1 = true ∧ f64NotNaN l2 = true ∧ f64NotNaN l3 = true ∧
      cmin64 (cmin64 l0 l1) (cmin64 l2 l3) = specMinNoNaN64 x i := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  have hRo190 : minNoNaN64AVX2Rodata 190 = some f64PosInf := by decide
  have hJmp : nextTargetPc 200 6 = some 206 := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 14 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 138 min2BlkLoop4Hdr hInv.statusRun hPc min2BlkLoop4Hdr_ok
  have hL0 : 14 + min2BlkLoop4Hdr.length = 17 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkLoop4Hdr
  have hMidRun : sMid.status = .running := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 147 := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  change (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).pc = 206 ∧
    (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).gpr 2#4 = 0#64 ∧
    MinNoNaN2ScalarInv base spF x n i f64PosInf
      (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid) ∧
    ∃ l0 l1 l2 l3 : BitVec 64,
      (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).mem (spF + 32#64) = l0 ∧
      (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).mem (spF + 40#64) = l1 ∧
      (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).mem (spF + 48#64) = l2 ∧
      (runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid).mem (spF + 56#64) = l3 ∧
      f64NotNaN l0 = true ∧ f64NotNaN l1 = true ∧ f64NotNaN l2 = true ∧ f64NotNaN l3 = true ∧
      cmin64 (cmin64 l0 l1) (cmin64 l2 l3) = specMinNoNaN64 x i
  have hSA : runAmd64Vec 14 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid =
      runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
        (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid min2BlkSpillA) :=
    runAmd64Vec_block_seq 7 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid 147 min2BlkSpillA hMidRun hMidPc min2BlkSpillA_ok
  rw [hSA]
  let sA := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sMid min2BlkSpillA
  have hARun : sA.status = .running := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun]
  have hAPc : sA.pc = 170 := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = false := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, Amd64VecInsn.dirtiesUpper]
  have hAHaz : sA.sseHazard = false := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      Amd64VecInsn.isLegacySSE, hMidHaz]
  have hARax : sA.gpr 0#4 = base := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax]
  have hARcx : sA.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRcx]
  have hARdx : sA.gpr 2#4 = spF + 32#64 := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx]
  have hARsp : sA.gpr 4#4 = spF := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  have hARsi : sA.gpr 6#4 = spF := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  rcases hInv.accs with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ0, hZ2,
    ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
  have hSlotNe : ∀ k1 k2 : Nat, k1 ≠ k2 → k1 < 8 → k2 < 8 →
      spF + BitVec.ofNat 64 (k1 * 8) ≠ spF + BitVec.ofNat 64 (k2 * 8) :=
    fun k1 k2 hNe hk1 hk2 => stackSlot_ne_offset spF k1 k2 hNe (by omega) (by omega)
  let l0 := vminpdLane a0 b0
  let l1 := vminpdLane a1 b1
  let l2 := vminpdLane a2 b2
  let l3 := vminpdLane a3 b3
  have hAMemEq : sA.mem =
      write64 (write64 (write64 (write64 s.mem (spF + 0#64) l0) (spF + 8#64) l1) (spF + 16#64) l2) (spF + 24#64) l3 := by
    simp [sA, min2BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, storeVecMem,
      setZmm, hMidRsp, hMidZmm, hMidMem, hZ0, hZ2, applyEvexMask_zero, Vec512.applyWidth,
      Vec512.zeroHi256, Vec512.zipWith, l0, l1, l2, l3]
  have hAM0 : sA.mem spF = l0 := by
    suffices sA.mem (spF + 0#64) = l0 by simpa using this
    rw [hAMemEq, write64_ne _ _ (hSlotNe 0 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 2 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 1 (by decide) (by omega) (by omega)), write64_eq]
  have hAM1 : sA.mem (spF + 8#64) = l1 := by
    rw [hAMemEq, write64_ne _ _ (hSlotNe 1 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 2 (by decide) (by omega) (by omega)), write64_eq]
  have hAM2 : sA.mem (spF + 16#64) = l2 := by
    rw [hAMemEq, write64_ne _ _ (hSlotNe 2 3 (by decide) (by omega) (by omega)), write64_eq]
  have hAM3 : sA.mem (spF + 24#64) = l3 := by
    rw [hAMemEq, write64_eq]
  change (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).pc = 206 ∧
    (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).gpr 2#4 = 0#64 ∧
    MinNoNaN2ScalarInv base spF x n i f64PosInf
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA) ∧
    ∃ l0 l1 l2 l3 : BitVec 64,
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).mem (spF + 32#64) = l0 ∧
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).mem (spF + 40#64) = l1 ∧
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).mem (spF + 48#64) = l2 ∧
      (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA).mem (spF + 56#64) = l3 ∧
      f64NotNaN l0 = true ∧ f64NotNaN l1 = true ∧ f64NotNaN l2 = true ∧ f64NotNaN l3 = true ∧
      cmin64 (cmin64 l0 l1) (cmin64 l2 l3) = specMinNoNaN64 x i
  have hSB : runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA =
      runAmd64Vec 0 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
        (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA min2BlkSpillB) :=
    runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA 170 min2BlkSpillB hARun hAPc min2BlkSpillB_ok
  rw [hSB]
  simp only [runAmd64Vec]
  have hA32_8 : spF + 32#64 + 8#64 = spF + 40#64 := by bv_omega
  have hA16_8 : spF + 16#64 + 8#64 = spF + 24#64 := by bv_omega
  have hA32_16 : spF + 32#64 + 16#64 = spF + 48#64 := by bv_omega
  have hA48_8 : spF + 48#64 + 8#64 = spF + 56#64 := by bv_omega
  have h24 : spF + 16#64 ≠ spF + 32#64 := hSlotNe 2 4 (by decide) (by omega) (by omega)
  have h25 : spF + 16#64 ≠ spF + 40#64 := hSlotNe 2 5 (by decide) (by omega) (by omega)
  have h34 : spF + 24#64 ≠ spF + 32#64 := hSlotNe 3 4 (by decide) (by omega) (by omega)
  have h35 : spF + 24#64 ≠ spF + 40#64 := hSlotNe 3 5 (by decide) (by omega) (by omega)
  have hMemFinal :
      (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sA min2BlkSpillB).mem =
        write64 (write64 (write64 (write64 sA.mem
          (spF + 32#64) l0) (spF + 40#64) l1) (spF + 48#64) l2) (spF + 56#64) l3 := by
    simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setZmm,
      hARdx, hARsi, hRo190, hJmp, hA32_8, hA16_8, hA32_16, hA48_8, hAM0, hAM1,
      write64_ne _ _ h25, write64_ne _ _ h24, hAM2,
      write64_ne _ _ h35, write64_ne _ _ h34, hAM3]
  refine ⟨by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, hRo190, hJmp],
    by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo190, hJmp],
    ⟨by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, hRo190, hJmp, hARun],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64,
          Amd64VecInsn.dirtiesUpper, hRo190, hJmp, hADirt],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64,
          Amd64VecInsn.isLegacySSE, Amd64VecInsn.dirtiesUpper, hRo190, hJmp, hADirt, hAHaz],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo190, hJmp, hARax],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo190, hJmp, hARcx],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo190, hJmp, hARbx],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo190, hJmp, hARsp],
     by simp [min2BlkSpillB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setZmm, hRo190, hJmp],
     f64PosInf_notNaN,
     by intro j hj
        rw [hMemFinal, hAMemEq]
        rw [write64_ne _ _ (hDisjStack j hj 7 (by omega)),
            write64_ne _ _ (hDisjStack j hj 6 (by omega)),
            write64_ne _ _ (hDisjStack j hj 5 (by omega)),
            write64_ne _ _ (hDisjStack j hj 4 (by omega)),
            write64_ne _ _ (hDisjStack j hj 3 (by omega)),
            write64_ne _ _ (hDisjStack j hj 2 (by omega)),
            write64_ne _ _ (hDisjStack j hj 1 (by omega)),
            write64_ne _ _ (hDisjStack j hj 0 (by omega))]
        exact hInv.memX j hj⟩,
    l0, l1, l2, l3, ?_, ?_, ?_, ?_,
    vminpdLane_notNaN ha0 hb0, vminpdLane_notNaN ha1 hb1,
    vminpdLane_notNaN ha2 hb2, vminpdLane_notNaN ha3 hb3, ?_⟩
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 4 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 5 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 5 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 5 6 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 6 7 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal, write64_eq]
  · dsimp only [l0, l1, l2, l3]
    rw [cmin64_combine4_eq ha0 ha1 ha2 ha3 hb0 hb1 hb2 hb3, hCmin]

private theorem minNoNaN64AVX2_horiz_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i k : Nat) (acc lk : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 206) (hk : k < 4) (hRdx : s.gpr 2#4 = BitVec.ofNat 64 k)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s)
    (hSlot : s.mem (spF + BitVec.ofNat 64 (k * 8) + 32#64) = lk) :
    ∃ steps ≤ 10,
      let s' := runAmd64Vec steps minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
      s'.pc = 206 ∧ s'.gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧ s'.mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 k) 4#64) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd1 : BitVec.ofNat 64 k + 1#64 = BitVec.ofNat 64 (k + 1) := bitVec_ofNat_add k 1
  have hSlotAddr : spF + BitVec.ofNat 64 k * 8#64 + 32#64 = spF + BitVec.ofNat 64 (k * 8) + 32#64 := by
    have hMul : BitVec.ofNat 64 k * 8#64 = BitVec.ofNat 64 (k * 8) := by
      apply BitVec.eq_of_toNat_eq; simp [BitVec.toNat_mul, Nat.mul_mod]
    rw [hMul]
  have hJa : evalX86Cond .a (x86UcomisdFlags acc lk) = f64Lt lk acc := evalX86Cond_a_ucomisd acc lk
  have hJcc200 : nextTargetPc 226 (-26) = some 200 := by decide
  have hJmp200 : nextTargetPc 231 (-31) = some 200 := by decide
  by_cases hLt : f64Lt lk acc = true
  · refine ⟨8, by omega, ?_⟩
    dsimp only [minNoNaN64AVX2Bytes]
    have hS0 := runAmd64Vec_block_seq 6 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 206 min2BlkHorizHdr hInv.statusRun hPc min2BlkHorizHdr_ok
    have hL0 : 6 + min2BlkHorizHdr.length = 8 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkHorizHdr
    have h1Run : s1.status = .running := by
      simp [s1, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hInv.statusRun]
    have h1Pc : s1.pc = 212 := by
      simp [s1, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe]
    change (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).pc = 206 ∧
      (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1)
    have hS1 := runAmd64Vec_block_seq 2 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 212 min2BlkHorizCmp h1Run h1Pc min2BlkHorizCmp_ok
    have hL1 : 2 + min2BlkHorizCmp.length = 6 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 min2BlkHorizCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLt, hJcc200, hInv.statusRun]
    have h2Pc : s2.pc = 200 := by
      simp [s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLt, hJcc200]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).pc = 206 ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2)
    have hS2 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 200 min2BlkHorizInc h2Run h2Pc min2BlkHorizInc_ok
    have hL2 : 0 + min2BlkHorizInc.length = 2 := rfl
    rw [← hL2, hS2]
    simp only [runAmd64Vec]
    have hMem2 : s2.mem = s.mem := by
      simp [s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLt, hJcc200]
    refine ⟨by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn],
      by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
           readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
           hJa, hLt, hJcc200, hAdd1],
      by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem2],
      ⟨by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, h2Run],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200, hInv.upperDirtyF],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, readMem64, evalMemAddr, setZmm,
            hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLt, hJcc200, hInv.upperDirtyF, hInv.noHazard],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.raxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rcxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200],
       by simp [min2BlkHorizInc, s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc200, minStepNoNaN],
       minStepNoNaN_notNaN lk hInv.accNotNaN,
       by intro j hj; simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem2, hInv.memX j hj]⟩⟩
  · have hLtF : f64Lt lk acc = false := Bool.eq_false_iff.mpr hLt
    refine ⟨10, by omega, ?_⟩
    dsimp only [minNoNaN64AVX2Bytes]
    have hS0 := runAmd64Vec_block_seq 8 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 206 min2BlkHorizHdr hInv.statusRun hPc min2BlkHorizHdr_ok
    have hL0 : 8 + min2BlkHorizHdr.length = 10 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkHorizHdr
    have h1Run : s1.status = .running := by
      simp [s1, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hInv.statusRun]
    have h1Pc : s1.pc = 212 := by
      simp [s1, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe]
    change (runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).pc = 206 ∧
      (runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1)
    have hS1 := runAmd64Vec_block_seq 4 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 212 min2BlkHorizCmp h1Run h1Pc min2BlkHorizCmp_ok
    have hL1 : 4 + min2BlkHorizCmp.length = 8 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 min2BlkHorizCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLtF, hInv.statusRun]
    have h2Pc : s2.pc = 226 := by
      simp [s2, s1, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLtF]
    change (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).pc = 206 ∧
      (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2)
    have hS2 : runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 =
        runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
          (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 min2BlkHorizKeep) :=
      runAmd64Vec_block_seq 2 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
        minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 226 min2BlkHorizKeep h2Run h2Pc min2BlkHorizKeep_ok
    rw [hS2]
    let s3 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 min2BlkHorizKeep
    have h3Run : s3.status = .running := by
      simp [s3, min2BlkHorizKeep, execDecodedSeq, execAmd64VecInsn, hJmp200, h2Run]
    have h3Pc : s3.pc = 200 := by
      simp [s3, min2BlkHorizKeep, execDecodedSeq, execAmd64VecInsn, hJmp200]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3).pc = 206 ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3)
    have hS3 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 200 min2BlkHorizInc h3Run h3Pc min2BlkHorizInc_ok
    have hL3 : 0 + min2BlkHorizInc.length = 2 := rfl
    rw [← hL3, hS3]
    simp only [runAmd64Vec]
    have hMem3 : s3.mem = s.mem := by
      simp [s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr, execDecodedSeq,
        execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
        hSlotAddr, hSlot, hJa, hLtF, hJmp200]
    refine ⟨by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn],
      by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
           execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
           hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200, hAdd1],
      by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem3],
      ⟨by simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, h3Run],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm,
            hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200, hInv.upperDirtyF],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
            readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
            hJa, hLtF, hJmp200, hInv.upperDirtyF, hInv.noHazard],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.raxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rcxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200],
       by simp [min2BlkHorizInc, s3, s2, s1, min2BlkHorizKeep, min2BlkHorizCmp, min2BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hRdx, hGe,
            hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp200, minStepNoNaN],
       minStepNoNaN_notNaN lk hInv.accNotNaN,
       by intro j hj; simp [min2BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem3, hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX2_horiz_exit (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 206) (hRdx : s.gpr 2#4 = 4#64)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s) :
    let s' := runAmd64Vec 2 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
    s'.pc = 237 ∧ MinNoNaN2ScalarInv base spF x n i acc s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 4#64 4#64) = true := by decide
  have hJcc : nextTargetPc 212 25 = some 237 := by decide
  dsimp only [minNoNaN64AVX2Bytes]
  have hS0 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
    minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 206 min2BlkHorizHdr hInv.statusRun hPc min2BlkHorizHdr_ok
  have hL0 : 0 + min2BlkHorizHdr.length = 2 := rfl
  rw [← hL0, hS0]
  simp only [runAmd64Vec]
  refine ⟨by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc],
    ⟨by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.statusRun],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
          hRdx, hGe, hJcc, hInv.upperDirtyF],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
          hRdx, hGe, hJcc, hInv.noHazard],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.raxEq],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rcxEq],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rbxEq],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rspEq],
     by simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.xmm0Eq],
     hInv.accNotNaN,
     by intro j hj; simp [min2BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX2_tail_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 237) (hi : i < n) (hN : n + 16 < 2 ^ 63)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s) :
    ∃ steps ≤ 9,
      let s' := runAmd64Vec steps minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
      s'.pc = 237 ∧ MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i)) s' := by
  have hLe : evalX86Cond .le (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 i)) = false := by
    rw [evalX86Cond_le_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd1 : BitVec.ofNat 64 i + 1#64 = BitVec.ofNat 64 (i + 1) := bitVec_ofNat_add i 1
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hXi := hInv.memX i hi
  have hJa : evalX86Cond .a (x86UcomisdFlags acc (x i)) = f64Lt (x i) acc := evalX86Cond_a_ucomisd acc (x i)
  have hJcc231 : nextTargetPc 253 (-22) = some 231 := by decide
  have hJmp231 : nextTargetPc 258 (-27) = some 231 := by decide
  by_cases hLt : f64Lt (x i) acc = true
  · refine ⟨7, by omega, ?_⟩
    dsimp only [minNoNaN64AVX2Bytes]
    have hS0 := runAmd64Vec_block_seq 5 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 237 min2BlkTailHdr hInv.statusRun hPc min2BlkTailHdr_ok
    have hL0 : 5 + min2BlkTailHdr.length = 7 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkTailHdr
    have h1Run : s1.status = .running := by
      simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hInv.statusRun]
    have h1Pc : s1.pc = 242 := by
      simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe]
    change (runAmd64Vec 5 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).pc = 237 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 5 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1)
    have hS1 := runAmd64Vec_block_seq 2 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 242 min2BlkTailCmp h1Run h1Pc min2BlkTailCmp_ok
    have hL1 : 2 + min2BlkTailCmp.length = 5 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 min2BlkTailCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLt, hJcc231, hInv.statusRun]
    have h2Pc : s2.pc = 231 := by
      simp [s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLt, hJcc231]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).pc = 237 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2)
    have hS2 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 231 min2BlkTailInc h2Run h2Pc min2BlkTailInc_ok
    have hL2 : 0 + min2BlkTailInc.length = 2 := rfl
    rw [← hL2, hS2]
    simp only [runAmd64Vec]
    refine ⟨by simp [min2BlkTailInc, execDecodedSeq, execAmd64VecInsn],
      ⟨by simp [min2BlkTailInc, execDecodedSeq, execAmd64VecInsn, h2Run],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc231, hInv.upperDirtyF],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc231,
            hInv.upperDirtyF, hInv.noHazard],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc231],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc231, hAdd1],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc231],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.rspEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc231],
       by simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc231, minStepNoNaN],
       minStepNoNaN_notNaN (x i) hInv.accNotNaN,
       by intro j hj; simp [min2BlkTailInc, s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq,
            execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc231, hInv.memX j hj]⟩⟩
  · have hLtF : f64Lt (x i) acc = false := Bool.eq_false_iff.mpr hLt
    refine ⟨9, by omega, ?_⟩
    dsimp only [minNoNaN64AVX2Bytes]
    have hS0 := runAmd64Vec_block_seq 7 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 237 min2BlkTailHdr hInv.statusRun hPc min2BlkTailHdr_ok
    have hL0 : 7 + min2BlkTailHdr.length = 9 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkTailHdr
    have h1Run : s1.status = .running := by
      simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hInv.statusRun]
    have h1Pc : s1.pc = 242 := by
      simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe]
    change (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1).pc = 237 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1)
    have hS1 := runAmd64Vec_block_seq 4 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 242 min2BlkTailCmp h1Run h1Pc min2BlkTailCmp_ok
    have hL1 : 4 + min2BlkTailCmp.length = 7 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 min2BlkTailCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLtF, hInv.statusRun]
    have h2Pc : s2.pc = 253 := by
      simp [s2, s1, min2BlkTailCmp, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLtF]
    change (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2).pc = 237 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2)
    have hS2 := runAmd64Vec_block_seq 2 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 253 min2BlkTailKeep h2Run h2Pc min2BlkTailKeep_ok
    have hL2 : 2 + min2BlkTailKeep.length = 4 := rfl
    rw [← hL2, hS2]
    let s3 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 min2BlkTailKeep
    have h3Run : s3.status = .running := by
      simp [s3, min2BlkTailKeep, execDecodedSeq, execAmd64VecInsn, hJmp231, h2Run]
    have h3Pc : s3.pc = 231 := by
      simp [s3, min2BlkTailKeep, execDecodedSeq, execAmd64VecInsn, hJmp231]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3).pc = 237 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 2 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3)
    have hS3 := runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
      minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 231 min2BlkTailInc h3Run h3Pc min2BlkTailInc_ok
    have hL3 : 0 + min2BlkTailInc.length = 2 := rfl
    rw [← hL3, hS3]
    simp only [runAmd64Vec]
    refine ⟨by simp [min2BlkTailInc, execDecodedSeq, execAmd64VecInsn],
      ⟨by simp [min2BlkTailInc, execDecodedSeq, execAmd64VecInsn, h3Run],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231,
            hInv.upperDirtyF],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
            readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231, hInv.upperDirtyF, hInv.noHazard],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231, hAdd1],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231],
       by simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231, minStepNoNaN],
       minStepNoNaN_notNaN (x i) hInv.accNotNaN,
       by intro j hj; simp [min2BlkTailInc, s3, s2, s1, min2BlkTailKeep, min2BlkTailCmp, min2BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp231,
            hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX2_tail_iter (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (hN : n + 16 < 2 ^ 63) :
    ∀ rem i acc s, i + rem = n → s.pc = 237 →
      MinNoNaN2ScalarInv base spF x n i acc s →
      canonZero64 acc = specMinNoNaN64 x i →
      ∃ steps ≤ 9 * rem,
        let s' := runAmd64Vec steps minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
        s'.pc = 237 ∧ ∃ acc',
          MinNoNaN2ScalarInv base spF x n n acc' s' ∧ canonZero64 acc' = specMinNoNaN64 x n := by
  intro rem
  induction rem with
  | zero =>
    intro i acc s hEq hPc hInv hCanon
    have hi : i = n := by omega
    subst hi
    exact ⟨0, by omega, by simpa [runAmd64Vec] using hPc, acc, by simpa [runAmd64Vec] using hInv, hCanon⟩
  | succ r ih =>
    intro i acc s hEq hPc hInv hCanon
    have h1 := minNoNaN64AVX2_tail_step base spF x n i acc s hPc (by omega) hN hInv
    rcases h1 with ⟨st1, hSt1Le, hPc1, hInv1⟩
    have hCanon1 : canonZero64 (minStepNoNaN acc (x i)) = specMinNoNaN64 x (i + 1) := by
      rw [cmin64_minStepNoNaN (x i) hInv.accNotNaN, ← cmin64_canonZero_left, hCanon, specMinNoNaN64_succ]
    have h2 := ih (i + 1) (minStepNoNaN acc (x i)) _ (by omega) hPc1 hInv1 hCanon1
    rcases h2 with ⟨st2, hSt2Le, hPc2, acc', hInv2, hCanon2⟩
    refine ⟨st1 + st2, by omega, ?_, acc', ?_, hCanon2⟩
    · rw [runAmd64Vec_add]; exact hPc2
    · rw [runAmd64Vec_add]; exact hInv2

private theorem minNoNaN64AVX2_canon_and_ret (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 237) (hN : n + 16 < 2 ^ 63)
    (hInv : MinNoNaN2ScalarInv base spF x n n acc s) :
    ∃ steps ≤ 10,
      let s' := runAmd64Vec steps minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s
      s'.status = .returned ∧ s'.upperDirty = false ∧ s'.sseHazard = false ∧
      (s'.zmm 0#5).l0 = canonZero64 acc := by
  have hLe : evalX86Cond .le (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = true := by
    rw [evalX86Cond_le_sub64 (by omega) (by omega)]; exact decide_eq_true (Nat.le_refl n)
  have hJle : nextTargetPc 242 16 = some 258 := by decide
  have hJne : nextTargetPc 267 5 = some 272 := by decide
  have hJp : nextTargetPc 269 3 = some 272 := by decide
  have hXorSelf : ∀ v : BitVec 64, v ^^^ v = 0#64 := fun _ => BitVec.xor_self
  have hCanon := x86_ucomisd_zero_canon_eq_canonZero64 acc
  -- Run `tailHdr` (2 steps -> pc 258) then `canon0` (3 steps -> pc 267 or 272).
  let s1 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s min2BlkTailHdr
  have h1Run : s1.status = .running := by
    simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.statusRun]
  have h1Pc : s1.pc = 258 := by
    simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle]
  have h1Dirt : s1.upperDirty = false := by
    simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
      hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.upperDirtyF]
  have h1Haz : s1.sseHazard = false := by
    simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.noHazard]
  have h1Z0 : (s1.zmm 0#5).l0 = acc := by
    simp [s1, min2BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.xmm0Eq]
  let s2 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 min2BlkCanon0
  by_cases hNe : evalX86Cond .ne (x86UcomisdFlags acc 0#64) = true
  · refine ⟨8, by omega, ?_⟩
    dsimp only [minNoNaN64AVX2Bytes]
    have hS0 : runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s =
        runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 :=
      runAmd64Vec_block_seq 6 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
        minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 237 min2BlkTailHdr hInv.statusRun hPc min2BlkTailHdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 6 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 =
        runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 :=
      runAmd64Vec_block_seq 3 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
        minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 258 min2BlkCanon0 h1Run h1Pc min2BlkCanon0_ok
    rw [hS1]
    have h2Run : s2.status = .running := by
      simp [s2, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNe, hJne, h1Run]
    have h2Pc : s2.pc = 272 := by
      simp [s2, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNe, hJne]
    have hS2 : runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 =
        runAmd64Vec 0 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
          (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 min2BlkRet) :=
      runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
        minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 272 min2BlkRet h2Run h2Pc min2BlkRet_ok
    rw [hS2]
    have hAccEq : acc = canonZero64 acc := by simpa [hNe] using hCanon
    rw [← hAccEq]
    simp [runAmd64Vec, min2BlkRet, s2, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setZmm, h1Z0, hXorSelf, hNe, hJne,
      h1Dirt, h1Haz]
  · have hNeF : evalX86Cond .ne (x86UcomisdFlags acc 0#64) = false := Bool.eq_false_iff.mpr hNe
    have h2Run : s2.status = .running := by
      simp [s2, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNeF, h1Run]
    have h2Pc : s2.pc = 267 := by
      simp [s2, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNeF]
    let s3 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 min2BlkCanonJp
    by_cases hP : evalX86Cond .p (x86UcomisdFlags acc 0#64) = true
    · refine ⟨9, by omega, ?_⟩
      dsimp only [minNoNaN64AVX2Bytes]
      have hS0 : runAmd64Vec 9 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s =
          runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 :=
        runAmd64Vec_block_seq 7 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 237 min2BlkTailHdr hInv.statusRun hPc min2BlkTailHdr_ok
      rw [hS0]
      have hS1 : runAmd64Vec 7 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 =
          runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 :=
        runAmd64Vec_block_seq 4 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 258 min2BlkCanon0 h1Run h1Pc min2BlkCanon0_ok
      rw [hS1]
      have hS2 : runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 =
          runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 :=
        runAmd64Vec_block_seq 3 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 267 min2BlkCanonJp h2Run h2Pc min2BlkCanonJp_ok
      rw [hS2]
      have h3Run : s3.status = .running := by
        simp [s3, s2, min2BlkCanonJp, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hP, hJp, h1Run]
      have h3Pc : s3.pc = 272 := by
        simp [s3, s2, min2BlkCanonJp, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hP, hJp]
      have hS3 : runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 =
          runAmd64Vec 0 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
            (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 min2BlkRet) :=
        runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 272 min2BlkRet h3Run h3Pc min2BlkRet_ok
      rw [hS3]
      have hAccEq : acc = canonZero64 acc := by simpa [hNeF, hP] using hCanon
      rw [← hAccEq]
      simp [runAmd64Vec, min2BlkRet, s3, s2, min2BlkCanonJp, min2BlkCanon0, execDecodedSeq,
        execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setZmm,
        h1Z0, hXorSelf, hNeF, hP, hJp, h1Dirt, h1Haz]
    · have hPF : evalX86Cond .p (x86UcomisdFlags acc 0#64) = false := Bool.eq_false_iff.mpr hP
      refine ⟨10, by omega, ?_⟩
      dsimp only [minNoNaN64AVX2Bytes]
      have hS0 : runAmd64Vec 10 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s =
          runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 :=
        runAmd64Vec_block_seq 8 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s 237 min2BlkTailHdr hInv.statusRun hPc min2BlkTailHdr_ok
      rw [hS0]
      have hS1 : runAmd64Vec 8 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 =
          runAmd64Vec 5 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 :=
        runAmd64Vec_block_seq 5 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s1 258 min2BlkCanon0 h1Run h1Pc min2BlkCanon0_ok
      rw [hS1]
      have hS2 : runAmd64Vec 5 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 =
          runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 :=
        runAmd64Vec_block_seq 4 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s2 267 min2BlkCanonJp h2Run h2Pc min2BlkCanonJp_ok
      rw [hS2]
      have h3Run : s3.status = .running := by
        simp [s3, s2, min2BlkCanonJp, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hPF, h1Run]
      have h3Pc : s3.pc = 269 := by
        simp [s3, s2, min2BlkCanonJp, min2BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hPF]
      let s4 := execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 min2BlkCanonZero
      have hS3 : runAmd64Vec 4 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 =
          runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s4 :=
        runAmd64Vec_block_seq 3 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s3 269 min2BlkCanonZero h3Run h3Pc min2BlkCanonZero_ok
      rw [hS3]
      have h4Run : s4.status = .running := by
        simp [s4, min2BlkCanonZero, execDecodedSeq, execAmd64VecInsn, h3Run]
      have h4Pc : s4.pc = 272 := by
        simp [s4, min2BlkCanonZero, execDecodedSeq, execAmd64VecInsn]
      have hS4 : runAmd64Vec 3 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s4 =
          runAmd64Vec 0 ⟨minNoNaN64AVX2ByteChunks.flatten⟩ minNoNaN64AVX2Rodata minNoNaN64AVX2Targets
            (execDecodedSeq minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s4 min2BlkRet) :=
        runAmd64Vec_block_seq 0 minNoNaN64AVX2ByteChunks minNoNaN64AVX2_chunks_wf
          minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s4 272 min2BlkRet h4Run h4Pc min2BlkRet_ok
      rw [hS4]
      have hZeroEq : 0#64 = canonZero64 acc := by simpa [hNeF, hPF] using hCanon
      rw [← hZeroEq]
      simp [runAmd64Vec, min2BlkRet, s4, s3, s2, min2BlkCanonZero, min2BlkCanonJp, min2BlkCanon0,
        execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
        setZmm, h1Z0, hXorSelf, hNeF, hPF, h1Dirt, h1Haz]

/-- Generous upper bound on step count for `minNoNaN64AVX2` on arbitrary slice length `n = 4q + r`. -/
def minNoNaN64AVX2FuelBound (n : Nat) : Nat :=
  13 + 15 * (n / 8) + 4 + 9 + 17 + 42 + 9 * (n % 4) + 10

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`minNoNaN64AVX2`)**:
For arbitrary slice length `n = 4q + r` (`0 < n`, `n + 16 < 2^63`, covering 2x-unrolled 8-wide
AVX2 loop, 4-wide vector tail with reversed `VMINPD` operand order, 4-lane horizontal SSE loop,
arbitrary scalar tail `r ∈ {0, 1, 2, 3}`, zero-trip vector cases `n < 4` / `n < 8`, signed-zero
asymmetry, subnormals, `±Inf`, and `NaN` replacement):
1. Never hits `runtime.panicBounds`, `morestack`, or any external runtime/GC call;
2. Terminates with `status = .returned`, `upperDirty = false` (`VZEROUPPER` ABI safety before
   every legacy SSE instruction and before `RET`), and `sseHazard = false`;
3. Returns bit-exact `XMM0.l0 = specMinNoNaN64 x n` (`Track5D.canonZero64` of sequential min);
4. Preserves Go amd64 ABI registers: `RSP = s0.gpr 4#4 + 8#64`, `RBP = s0.gpr 5#4`,
   `R14 = s0.gpr 14#4`, and `X15` low 128 bits (`l0`, `l1`). -/
theorem minNoNaN64AVX2_functional_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 8, elemAddr base j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8))
    (hFuel : minNoNaN64AVX2FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    (sf.zmm 0#5).l0 = specMinNoNaN64 x n ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  let spF := s0.gpr 4#4 - 72#64
  have hPro := minNoNaN64AVX2_prologue base x n s0 hPc hRun hHaz hPos hN hRax hRbx hX
    (fun j hj => ⟨(hDisj j hj).1, (hDisj j hj).2.1⟩)
  have hL8 := minNoNaN64AVX2_loop8_iter base spF x n hN (n / 8) 0 _ (by omega) hPro.1 hPro.2
  have hEx8 := minNoNaN64AVX2_loop8_exit base spF x n (8 * (n / 8)) _
    hL8.1 (by simpa using hL8.2) (by omega) hN (by omega)
  -- Helper completing from pc=138 at `i = 4 * (n / 4)` with `n < i + 4` to final return.
  have finishFrom138 : ∀ sVec, sVec.pc = 138 →
      MinNoNaN2Inv base spF x n (4 * (n / 4)) sVec →
      ∃ stRest ≤ 17 + 42 + 9 * (n % 4) + 10,
        let sEnd := runAmd64Vec stRest minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sVec
        sEnd.status = .returned ∧ sEnd.upperDirty = false ∧ sEnd.sseHazard = false ∧
        (sEnd.zmm 0#5).l0 = specMinNoNaN64 x n := by
    intro sVec hVecPc hVecInv
    have hSpill := minNoNaN64AVX2_spill_and_vzu base spF x n (4 * (n / 4)) sVec
      hVecPc hVecInv (by omega) hN (by omega) (fun j hj => (hDisj j hj).2.2)
    rcases hSpill with ⟨hPcSp, hRdxSp, hSc0, l0, l1, l2, l3,
      hM0, hM1, hM2, hM3, hl0, hl1, hl2, hl3, hLanesEq⟩
    let sSp := runAmd64Vec 17 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets sVec
    have hAddr0 : spF + BitVec.ofNat 64 (0 * 8) + 32#64 = spF + 32#64 := by bv_omega
    have hAddr1 : spF + BitVec.ofNat 64 (1 * 8) + 32#64 = spF + 40#64 := by bv_omega
    have hAddr2 : spF + BitVec.ofNat 64 (2 * 8) + 32#64 = spF + 48#64 := by bv_omega
    have hAddr3 : spF + BitVec.ofNat 64 (3 * 8) + 32#64 = spF + 56#64 := by bv_omega
    have hH0 := minNoNaN64AVX2_horiz_step base spF x n (4 * (n / 4)) 0 f64PosInf l0 sSp
      hPcSp (by omega) hRdxSp hSc0 (by rw [hAddr0]; exact hM0)
    rcases hH0 with ⟨st0, hSt0, hPc0, hRdx0, hMem0, hInv0⟩
    have hH1 := minNoNaN64AVX2_horiz_step base spF x n (4 * (n / 4)) 1 _ l1 _
      hPc0 (by omega) hRdx0 hInv0 (by rw [hMem0, hAddr1]; exact hM1)
    rcases hH1 with ⟨st1, hSt1, hPc1, hRdx1, hMem1, hInv1⟩
    have hH2 := minNoNaN64AVX2_horiz_step base spF x n (4 * (n / 4)) 2 _ l2 _
      hPc1 (by omega) hRdx1 hInv1 (by rw [hMem1, hMem0, hAddr2]; exact hM2)
    rcases hH2 with ⟨st2, hSt2, hPc2, hRdx2, hMem2, hInv2⟩
    have hH3 := minNoNaN64AVX2_horiz_step base spF x n (4 * (n / 4)) 3 _ l3 _
      hPc2 (by omega) hRdx2 hInv2 (by rw [hMem2, hMem1, hMem0, hAddr3]; exact hM3)
    rcases hH3 with ⟨st3, hSt3, hPc3, hRdx3, _, hInv3⟩
    have hExH := minNoNaN64AVX2_horiz_exit base spF x n (4 * (n / 4)) _ _ hPc3 hRdx3 hInv3
    have hAccCanon : canonZero64 (minStepNoNaN (minStepNoNaN (minStepNoNaN (minStepNoNaN f64PosInf l0) l1) l2) l3) =
        specMinNoNaN64 x (4 * (n / 4)) := by
      have ha0 : f64NotNaN (minStepNoNaN f64PosInf l0) = true := minStepNoNaN_notNaN l0 f64PosInf_notNaN
      have ha1 : f64NotNaN (minStepNoNaN (minStepNoNaN f64PosInf l0) l1) = true := minStepNoNaN_notNaN l1 ha0
      have ha2 : f64NotNaN (minStepNoNaN (minStepNoNaN (minStepNoNaN f64PosInf l0) l1) l2) = true := minStepNoNaN_notNaN l2 ha1
      rw [cmin64_minStep_fold l3 ha2, cmin64_minStep_fold l2 ha1,
          cmin64_minStep_fold l1 ha0, cmin64_minStep_fold l0 f64PosInf_notNaN,
          cmin64_canonZero_left, cmin64_posInf_left, cmin64_canonZero_left, cmin64_sanitize_left]
      rw [← hLanesEq]
      ac_rfl
    have hTail := minNoNaN64AVX2_tail_iter base spF x n hN (n % 4) (4 * (n / 4)) _ _
      (by omega) hExH.1 hExH.2 hAccCanon
    rcases hTail with ⟨stT, hStT, hPcT, acc', hInvT, hCanonT⟩
    have hRet := minNoNaN64AVX2_canon_and_ret base spF x n acc' _ hPcT hN hInvT
    rcases hRet with ⟨stR, hStR, hStatR, hDirtR, hHazR, hZ0R⟩
    refine ⟨17 + st0 + st1 + st2 + st3 + 2 + stT + stR, by omega, ?_⟩
    dsimp only
    rw [runAmd64Vec_add (17 + st0 + st1 + st2 + st3 + 2 + stT) stR,
        runAmd64Vec_add (17 + st0 + st1 + st2 + st3 + 2) stT,
        runAmd64Vec_add (17 + st0 + st1 + st2 + st3) 2,
        runAmd64Vec_add (17 + st0 + st1 + st2) st3,
        runAmd64Vec_add (17 + st0 + st1) st2,
        runAmd64Vec_add (17 + st0) st1,
        runAmd64Vec_add 17 st0]
    exact ⟨hStatR, hDirtR, hHazR, hZ0R.trans hCanonT⟩
  have hExist : ∃ stTotal ≤ minNoNaN64AVX2FuelBound n,
      let se := runAmd64Vec stTotal minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      (se.zmm 0#5).l0 = specMinNoNaN64 x n := by
    unfold minNoNaN64AVX2FuelBound
    by_cases hRem : 8 * (n / 8) + 4 ≤ n
    · have h4Eq : 8 * (n / 8) + 4 = 4 * (n / 4) := by omega
      have hL4 := minNoNaN64AVX2_loop4_step base spF x n (8 * (n / 8)) _ hEx8.1 hEx8.2 hRem hN
      rcases finishFrom138 _ hL4.1 (by simpa [h4Eq] using hL4.2) with ⟨stRest, hStRest, hFin⟩
      refine ⟨13 + 15 * (n / 8) + 4 + 9 + stRest, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (13 + 15 * (n / 8) + 4 + 9) stRest,
          runAmd64Vec_add (13 + 15 * (n / 8) + 4) 9,
          runAmd64Vec_add (13 + 15 * (n / 8)) 4,
          runAmd64Vec_add 13 (15 * (n / 8))]
      exact hFin
    · have h4Eq : 8 * (n / 8) = 4 * (n / 4) := by omega
      rcases finishFrom138 _ hEx8.1 (by simpa [h4Eq] using hEx8.2) with ⟨stRest, hStRest, hFin⟩
      refine ⟨13 + 15 * (n / 8) + 4 + stRest, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (13 + 15 * (n / 8) + 4) stRest,
          runAmd64Vec_add (13 + 15 * (n / 8)) 4,
          runAmd64Vec_add 13 (15 * (n / 8))]
      exact hFin
  rcases hExist with ⟨stTotal, hStLe, hExact⟩
  have hSplit : fuel = stTotal + (fuel - stTotal) := by omega
  have hAbi := minNoNaN64AVX2_abi_reg_preserved s0 fuel hPc hRun
  have hCore :
      let sf := runAmd64Vec fuel minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      (sf.zmm 0#5).l0 = specMinNoNaN64 x n := by
    dsimp only
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hExact.1])]
    exact hExact
  have hSpRbp := hAbi.2.2.2 hCore.1
  exact ⟨hCore.1, hCore.2.1, hCore.2.2.1, hCore.2.2.2, hSpRbp.1, hSpRbp.2, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

/-! #### AVX-512 (`minNoNaN64AVX512`) Inductive Functional Equivalence -/

/-- Vector loop invariant at `pc = 137` (16-wide header) and `pc = 180` (8-wide header) of `minNoNaN64AVX512`. -/
private structure MinNoNaN512Inv (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = base
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  rspEq : s.gpr 4#4 = spF
  zmm1Eq : s.zmm 1#5 = Vec512.broadcast f64PosInf
  memX : ∀ j < n, s.mem (elemAddr base j) = x j
  accs : ∃ a0 a1 a2 a3 a4 a5 a6 a7 b0 b1 b2 b3 b4 b5 b6 b7 : BitVec 64,
    s.zmm 0#5 = { l0 := a0, l1 := a1, l2 := a2, l3 := a3, l4 := a4, l5 := a5, l6 := a6, l7 := a7 } ∧
    s.zmm 2#5 = { l0 := b0, l1 := b1, l2 := b2, l3 := b3, l4 := b4, l5 := b5, l6 := b6, l7 := b7 } ∧
    f64NotNaN a0 = true ∧ f64NotNaN a1 = true ∧ f64NotNaN a2 = true ∧ f64NotNaN a3 = true ∧
    f64NotNaN a4 = true ∧ f64NotNaN a5 = true ∧ f64NotNaN a6 = true ∧ f64NotNaN a7 = true ∧
    f64NotNaN b0 = true ∧ f64NotNaN b1 = true ∧ f64NotNaN b2 = true ∧ f64NotNaN b3 = true ∧
    f64NotNaN b4 = true ∧ f64NotNaN b5 = true ∧ f64NotNaN b6 = true ∧ f64NotNaN b7 = true ∧
    cmin64
      (cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3)) (cmin64 (cmin64 a4 a5) (cmin64 a6 a7)))
      (cmin64 (cmin64 (cmin64 b0 b1) (cmin64 b2 b3)) (cmin64 (cmin64 b4 b5) (cmin64 b6 b7))) =
      specMinNoNaN64 x i

private def min512BlkProGuard : List (Amd64VecInsn × Nat) :=
  [ (.lea64 12#4 (.baseIdxDisp (some 4#4) none (-8)), 5),
    (.cmpRM64 12#4 (.baseIdxDisp (some 14#4) none 16), 9),
    (.jccRel .be 333, 15) ]

private def min512BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 16),
    (.movRR64 5#4 4#4, 19),
    (.addImm64 4#4 0xffffffffffffff80#64, 23),
    (.movMR64 (.baseIdxDisp (some 4#4) none 144) 0#4, 31),
    (.nop, 32),
    (.testRR64 3#4 3#4, 35),
    (.jccRel .be 301, 41) ]

private def min512BlkPro1 : List (Amd64VecInsn × Nat) :=
  [ (.nop, 42),
    (.vmovsdLoad 0#5 (.ripRel 296846), 50),
    (.vbroadcastsd true .zmm512 0#5 (.reg 0#5) {}, 56),
    (.xorRR32 1#4 1#4, 58),
    (.vmovdquRR true .zmm512 1#5 0#5 {}, 64),
    (.vmovdquRR true .zmm512 2#5 1#5 {}, 70),
    (.jmpRel 65, 72) ]

private def min512BlkLoop16Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 16), 141),
    (.cmpRR64 3#4 2#4, 144),
    (.jccRel .ge (-74), 146) ]

private def min512BlkLoop16Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 76),
    (.vmovdquLoad true .zmm512 3#5 (.baseIdxDisp (some 6#4) none 0) {}, 82),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 86),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 64), 90),
    (.vmovdquLoad true .zmm512 4#5 (.baseIdxDisp (some 6#4) none 0) {}, 96),
    (.vcmppdEvex .zmm512 1#3 3#5 (.reg 3#5) 3 {}, 103),
    (.vcmppdEvex .zmm512 2#3 4#5 (.reg 4#5) 3 {}, 110),
    (.vpblendmq .zmm512 3#5 3#5 (.reg 1#5) { kmask := 1#3, zeroing := false, broadcast := false }, 116),
    (.vpblendmq .zmm512 4#5 4#5 (.reg 1#5) { kmask := 2#3, zeroing := false, broadcast := false }, 122),
    (.vminpd true .zmm512 0#5 3#5 (.reg 0#5) {}, 128),
    (.vminpd true .zmm512 2#5 4#5 (.reg 2#5) {}, 134),
    (.movRR64 1#4 2#4, 137) ]

private def min512BlkLoop16Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 32, 148) ]

private def min512BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 184),
    (.cmpRR64 3#4 2#4, 187),
    (.jccRel .ge (-41), 189) ]

private def min512BlkLoop8Body : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 152),
    (.vmovdquLoad true .zmm512 3#5 (.baseIdxDisp (some 6#4) none 0) {}, 158),
    (.vcmppdEvex .zmm512 1#3 3#5 (.reg 3#5) 3 {}, 165),
    (.vpblendmq .zmm512 3#5 3#5 (.reg 1#5) { kmask := 1#3, zeroing := false, broadcast := false }, 171),
    (.vminpd true .zmm512 0#5 0#5 (.reg 3#5) {}, 177),
    (.movRR64 1#4 2#4, 180) ]

private def min512BlkSpillA : List (Amd64VecInsn × Nat) :=
  [ (.vminpd true .zmm512 0#5 0#5 (.reg 2#5) {}, 195),
    (.vmovdquStore true .zmm512 (.baseIdxDisp (some 4#4) none 0) 0#5 {}, 202),
    (.vzeroupper, 205),
    (.nop, 206), (.nop, 207),
    (.lea64 2#4 (.baseIdxDisp (some 4#4) none 64), 212),
    (.lea64 6#4 (.baseIdxDisp (some 4#4) none 0), 216) ]

private def min512BlkSpillB1 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 0), 220),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 0) 14#4, 224),
    (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 16), 229),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 16) 14#4, 234) ]

private def min512BlkSpillB2 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 32), 239),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 32) 14#4, 244),
    (.sseMovupsLoad 14#4 (.baseIdxDisp (some 6#4) none 48), 249),
    (.sseMovupsStore (.baseIdxDisp (some 2#4) none 48) 14#4, 254),
    (.xorRR32 2#4 2#4, 256),
    (.sseMovsdLoad 0#4 (.ripRel 296632), 264),
    (.jmpRel 6, 266) ]

private def min512BlkHorizHdr : List (Amd64VecInsn × Nat) :=
  [ (.cmpImm64 2#4 8#64, 276),
    (.jccRel .ge 23, 278) ]

private def min512BlkHorizCmp : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) (some (2#4, 8)) 64), 284),
    (.sseUcomisdRR 0#4 1#4, 288),
    (.jccRel .a (-24), 290) ]

private def min512BlkHorizKeep : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 0#4, 293),
    (.jmpRel (-29), 295) ]

private def min512BlkHorizInc : List (Amd64VecInsn × Nat) :=
  [ (.inc64 2#4, 269),
    (.sseMovupsRR 0#4 1#4, 272) ]

private def min512BlkTailHdr : List (Amd64VecInsn × Nat) :=
  [ (.cmpRR64 3#4 1#4, 304),
    (.jccRel .le 16, 306) ]

private def min512BlkTailCmp : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 311),
    (.sseUcomisdRR 0#4 1#4, 315),
    (.jccRel .a (-22), 317) ]

private def min512BlkTailKeep : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 0#4, 320),
    (.jmpRel (-27), 322) ]

private def min512BlkTailInc : List (Amd64VecInsn × Nat) :=
  [ (.inc64 1#4, 298),
    (.sseMovupsRR 0#4 1#4, 301) ]

private def min512BlkCanon0 : List (Amd64VecInsn × Nat) :=
  [ (.sseXorpsRR 1#4 1#4, 325),
    (.sseUcomisdRR 0#4 1#4, 329),
    (.jccRel .ne 5, 331) ]

private def min512BlkCanonJp : List (Amd64VecInsn × Nat) :=
  [ (.jccRel .p 3, 333) ]

private def min512BlkCanonZero : List (Amd64VecInsn × Nat) :=
  [ (.sseXorpsRR 0#4 0#4, 336) ]

private def min512BlkRet : List (Amd64VecInsn × Nat) :=
  [ (.subImm64 4#4 0xffffffffffffff80#64, 340),
    (.popReg 5#4, 341),
    (.ret, 342) ]

set_option maxRecDepth 2048 in
private theorem min512BlkProGuard_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 0 min512BlkProGuard = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkPro0_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 15 min512BlkPro0 = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkPro1_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 41 min512BlkPro1 = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkLoop16Hdr_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 137 min512BlkLoop16Hdr = true := by decide
set_option maxRecDepth 4096 in
private theorem min512BlkLoop16Body_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 72 min512BlkLoop16Body = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkLoop16Exit_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 146 min512BlkLoop16Exit = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkLoop8Hdr_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 180 min512BlkLoop8Hdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkLoop8Body_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 148 min512BlkLoop8Body = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkSpillA_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 189 min512BlkSpillA = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkSpillB1_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 216 min512BlkSpillB1 = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkSpillB2_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 234 min512BlkSpillB2 = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkHorizHdr_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 272 min512BlkHorizHdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkHorizCmp_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 278 min512BlkHorizCmp = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkHorizKeep_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 290 min512BlkHorizKeep = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkHorizInc_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 266 min512BlkHorizInc = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkTailHdr_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 301 min512BlkTailHdr = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkTailCmp_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 306 min512BlkTailCmp = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkTailKeep_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 317 min512BlkTailKeep = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkTailInc_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 295 min512BlkTailInc = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkCanon0_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 322 min512BlkCanon0 = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkCanonJp_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 331 min512BlkCanonJp = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkCanonZero_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 333 min512BlkCanonZero = true := by decide
set_option maxRecDepth 2048 in
private theorem min512BlkRet_ok : checkBlockSeq minNoNaN64AVX512ByteChunks minNoNaN64AVX512Rodata 336 min512BlkRet = true := by decide

private theorem bitVec_ule_false_of_ult_rev {a b : BitVec 64} (h : BitVec.ult b a = true) :
    BitVec.ule a b = false := by
  simp only [BitVec.ult_eq_decide, decide_eq_true_eq] at h
  simp only [BitVec.ule_eq_decide, decide_eq_false_iff_not]
  omega

private theorem minNoNaN64AVX512_prologue_guard (base : BitVec 64) (n : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hGuard : BitVec.ult (s0.mem (s0.gpr 14#4 + 16#64)) (s0.gpr 4#4 - 8#64) = true)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n) :
    let sG := runAmd64Vec 3 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
    sG.status = .running ∧ sG.pc = 15 ∧ sG.sseHazard = false ∧
    sG.gpr 0#4 = base ∧ sG.gpr 3#4 = BitVec.ofNat 64 n ∧ sG.gpr 4#4 = s0.gpr 4#4 ∧ sG.mem = s0.mem := by
  have hAddNeg8 : s0.gpr 4#4 + BitVec.ofInt 64 (-8) = s0.gpr 4#4 - 8#64 := by
    rw [show (-8 : Int) = -(8 : Nat) from rfl, BitVec.ofInt_neg, BitVec.ofInt_natCast, ← BitVec.sub_eq_add_neg]
  have hGuardBe : evalX86Cond .be (x86SubFlags64 (s0.gpr 4#4 - 8#64) (s0.mem (s0.gpr 14#4 + 16#64))) = false := by
    rw [evalX86Cond_be_sub64_unsigned]
    exact bitVec_ule_false_of_ult_rev hGuard
  dsimp only [minNoNaN64AVX512Bytes]
  have hSG : runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0 =
      execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0 min512BlkProGuard :=
    runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0 0 min512BlkProGuard hRun hPc min512BlkProGuard_ok
  have hState : execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0 min512BlkProGuard =
      { s0 with
        gpr := setGpr s0.gpr 12#4 (s0.gpr 4#4 - 8#64)
        flags := x86SubFlags64 (s0.gpr 4#4 - 8#64) (s0.mem (s0.gpr 14#4 + 16#64))
        pc := 15 } := by
    simp [min512BlkProGuard, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr,
      -BitVec.reduceOfInt, hAddNeg8, hGuardBe, Amd64VecInsn.isLegacySSE, Amd64VecInsn.dirtiesUpper]
  rw [hSG, hState]
  exact ⟨hRun, rfl, hHaz, by simp [setGpr, hRax], by simp [setGpr, hRbx], by simp [setGpr], rfl⟩

private theorem minNoNaN64AVX512_prologue (base : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hGuard : BitVec.ult (s0.mem (s0.gpr 14#4 + 16#64)) (s0.gpr 4#4 - 8#64) = true)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64) :
    let s1 := runAmd64Vec 17 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
    s1.pc = 137 ∧ MinNoNaN512Inv base (s0.gpr 4#4 - 136#64) x n 0 s1 := by
  have hG := minNoNaN64AVX512_prologue_guard base n s0 hPc hRun hHaz hGuard hRax hRbx
  dsimp only [minNoNaN64AVX512Bytes] at hG
  rcases hG with ⟨hGRun, hGPc, hGHaz, hGRax, hGRbx, hGRsp, hGMem⟩
  have hBe : evalX86Cond .be (x86TestFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = false :=
    evalX86Cond_be_test64_self hPos (by omega)
  have hSpF : s0.gpr 4#4 - 8#64 + 0xffffffffffffff80#64 = s0.gpr 4#4 - 136#64 := by bv_omega
  have hSp144 : s0.gpr 4#4 - 8#64 + 0xffffffffffffff80#64 + 144#64 = s0.gpr 4#4 + 8#64 := by bv_omega
  have hJmp : nextTargetPc 72 65 = some 137 := by decide
  have hRo42 : minNoNaN64AVX512Rodata 42 = some f64PosInf := rfl
  have hSplit : (17 : Nat) = 3 + 14 := rfl
  dsimp only [minNoNaN64AVX512Bytes]
  rw [hSplit, runAmd64Vec_add 3 14]
  generalize hsG : runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0 = sG at hGRun hGPc hGHaz hGRax hGRbx hGRsp hGMem ⊢
  let sMid := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sG min512BlkPro0
  have hS0 : runAmd64Vec 14 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sG =
      runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid :=
    runAmd64Vec_block_seq 7 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sG 15 min512BlkPro0 hGRun hGPc min512BlkPro0_ok
  rw [hS0]
  have hMidRun : sMid.status = .running := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hGRbx, hBe, hGRun]
  have hMidPc : sMid.pc = 41 := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hGRbx, hBe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hGRbx, hBe, hGHaz]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hGRax, hGRbx, hBe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hGRbx, hBe]
  have hMidRsp : sMid.gpr 4#4 = s0.gpr 4#4 - 136#64 := by
    simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hGRsp, hGRbx, hBe, hSpF]
  have hMidMem : ∀ j < n, sMid.mem (elemAddr base j) = x j := by
    intro j hj
    have hd := hDisj j hj
    have hMem0 : sMid.mem = write64 (write64 s0.mem (s0.gpr 4#4 - 8#64) (sG.gpr 5#4)) (s0.gpr 4#4 + 8#64) base := by
      simp [sMid, min512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
        hGRax, hGRbx, hGRsp, hGMem, hBe, hSp144]
    rw [hMem0, write64_ne _ _ hd.2, write64_ne _ _ hd.1]
    exact hX j hj
  have hS1 : runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid =
      runAmd64Vec 0 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
        (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid min512BlkPro1) :=
    runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid 41 min512BlkPro1 hMidRun hMidPc min512BlkPro1_ok
  rw [hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp], ?_⟩
  refine ⟨by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, hMidRun],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, Amd64VecInsn.dirtiesUpper],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setGpr, hMidRax],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setGpr],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setGpr, hMidRbx],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setGpr, hMidRsp],
    by simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setZmm,
         applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast],
    by intro j hj
       have hM : (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid min512BlkPro1).mem = sMid.mem := by
         simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp]
       rw [hM]; exact hMidMem j hj,
    ?_⟩
  refine ⟨f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
    f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
    ?_, ?_,
    f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
    f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
    f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
    f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, ?_⟩
  · simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setZmm,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast]
  · simp [min512BlkPro1, execDecodedSeq, execAmd64VecInsn, readMem64, hRo42, hJmp, setZmm,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast]
  · rw [specMinNoNaN64_zero]; simp [cmin64_posInf_posInf]

private theorem minNoNaN64AVX512_loop16_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 137) (hInv : MinNoNaN512Inv base spF x n i s)
    (hStep : i + 16 ≤ n) (hN : n + 32 < 2 ^ 63) :
    let s' := runAmd64Vec 15 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.pc = 137 ∧ MinNoNaN512Inv base spF x n (i + 16) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJcc : nextTargetPc 146 (-74) = some 72 := by decide
  dsimp only [minNoNaN64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 12 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 137 min512BlkLoop16Hdr hInv.statusRun hPc min512BlkLoop16Hdr_ok
  have hL0 : 12 + min512BlkLoop16Hdr.length = 15 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkLoop16Hdr
  change (runAmd64Vec 12 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).pc = 137 ∧
    MinNoNaN512Inv base spF x n (i + 16)
      (runAmd64Vec 12 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 72 := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 16) := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid 72 min512BlkLoop16Body hMidRun hMidPc min512BlkLoop16Body_ok
  have hL1 : 0 + min512BlkLoop16Body.length = 12 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.accs with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ0, hZ2,
    ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hBaseAddr64 : elemAddr base i + 64#64 = elemAddr base (i + 8) := elemAddr_plus_64 base i
  have hX0 := hInv.memX i (by omega); have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega); have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega); have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega); have hX7 := hInv.memX (i + 7) (by omega)
  have hX8 := hInv.memX (i + 8) (by omega); have hX9 := hInv.memX (i + 9) (by omega)
  have hX10 := hInv.memX (i + 10) (by omega); have hX11 := hInv.memX (i + 11) (by omega)
  have hX12 := hInv.memX (i + 12) (by omega); have hX13 := hInv.memX (i + 13) (by omega)
  have hX14 := hInv.memX (i + 14) (by omega); have hX15 := hInv.memX (i + 15) (by omega)
  refine ⟨by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp],
    by simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  refine ⟨vminpdLane (sanitizeNaNInf (x i)) a0,
          vminpdLane (sanitizeNaNInf (x (i + 1))) a1,
          vminpdLane (sanitizeNaNInf (x (i + 2))) a2,
          vminpdLane (sanitizeNaNInf (x (i + 3))) a3,
          vminpdLane (sanitizeNaNInf (x (i + 4))) a4,
          vminpdLane (sanitizeNaNInf (x (i + 5))) a5,
          vminpdLane (sanitizeNaNInf (x (i + 6))) a6,
          vminpdLane (sanitizeNaNInf (x (i + 7))) a7,
          vminpdLane (sanitizeNaNInf (x (i + 8))) b0,
          vminpdLane (sanitizeNaNInf (x (i + 9))) b1,
          vminpdLane (sanitizeNaNInf (x (i + 10))) b2,
          vminpdLane (sanitizeNaNInf (x (i + 11))) b3,
          vminpdLane (sanitizeNaNInf (x (i + 12))) b4,
          vminpdLane (sanitizeNaNInf (x (i + 13))) b5,
          vminpdLane (sanitizeNaNInf (x (i + 14))) b6,
          vminpdLane (sanitizeNaNInf (x (i + 15))) b7,
    ?_, ?_,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha0, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha1,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha2, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha3,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha4, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha5,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha6, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) ha7,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb0, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb1,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb2, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb3,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb4, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb5,
    vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb6, vminpdLane_notNaN (sanitizeNaNInf_notNaN _) hb7, ?_⟩
  · have hMask := maskBit_or8_bv
      (f64IsNaN (x i)) (f64IsNaN (x (i + 1))) (f64IsNaN (x (i + 2))) (f64IsNaN (x (i + 3)))
      (f64IsNaN (x (i + 4))) (f64IsNaN (x (i + 5))) (f64IsNaN (x (i + 6))) (f64IsNaN (x (i + 7)))
    simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ0,
      hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX0, hX1, hX2, hX3, hX4, hX5, hX6, hX7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred,
      hMask.1, hMask.2.1, hMask.2.2.1, hMask.2.2.2.1, hMask.2.2.2.2.1, hMask.2.2.2.2.2.1,
      hMask.2.2.2.2.2.2.1, hMask.2.2.2.2.2.2.2, sanitizeNaNInf]
  · have hMask := maskBit_or8_bv
      (f64IsNaN (x (i + 8))) (f64IsNaN (x (i + 9))) (f64IsNaN (x (i + 10))) (f64IsNaN (x (i + 11)))
      (f64IsNaN (x (i + 12))) (f64IsNaN (x (i + 13))) (f64IsNaN (x (i + 14))) (f64IsNaN (x (i + 15)))
    simp [min512BlkLoop16Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ2,
      hBaseAddr, hBaseAddr64, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX8, hX9, hX10, hX11, hX12, hX13, hX14, hX15,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred,
      hMask.1, hMask.2.1, hMask.2.2.1, hMask.2.2.2.1, hMask.2.2.2.2.1, hMask.2.2.2.2.2.1,
      hMask.2.2.2.2.2.2.1, hMask.2.2.2.2.2.2.2, sanitizeNaNInf]
  · rw [cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha0,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha1,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha2,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha3,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha4,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha5,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) ha6,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) ha7,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb0,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb1,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb2,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb3,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb4,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb5,
        cmin64_vminpdLane_left _ (sanitizeNaNInf_notNaN _) hb6,
        cmin64_vminpdLane_right _ (sanitizeNaNInf_notNaN _) hb7]
    simp only [cmin64_sanitize_left]
    rw [specMinNoNaN64_add16, specMinNoNaN64_add8, specMinNoNaN64_add4, ← hCmin]
    ac_rfl

private theorem minNoNaN64AVX512_loop16_iter (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (hN : n + 32 < 2 ^ 63) :
    ∀ q i s, i + 16 * q ≤ n → s.pc = 137 → MinNoNaN512Inv base spF x n i s →
      let s' := runAmd64Vec (15 * q) minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
      s'.pc = 137 ∧ MinNoNaN512Inv base spF x n (i + 16 * q) s' := by
  intro q
  induction q with
  | zero =>
    intro i s _ hPc hInv
    simpa [runAmd64Vec] using And.intro hPc hInv
  | succ q ih =>
    intro i s hLe hPc hInv
    have hFuel : 15 * (q + 1) = 15 + 15 * q := by omega
    dsimp only
    rw [hFuel, runAmd64Vec_add]
    have h1 := minNoNaN64AVX512_loop16_step base spF x n i s hPc hInv (by omega) hN
    have h2 := ih (i + 16) _ (by omega) h1.1 h1.2
    have hIdx : i + 16 + 16 * q = i + 16 * (q + 1) := by omega
    simpa [hIdx] using h2

private theorem minNoNaN64AVX512_loop16_exit (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 137) (hInv : MinNoNaN512Inv base spF x n i s)
    (hLt : n < i + 16) (hN : n + 32 < 2 ^ 63) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.pc = 180 ∧ MinNoNaN512Inv base spF x n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJmp : nextTargetPc 148 32 = some 180 := by decide
  dsimp only [minNoNaN64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 1 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 137 min512BlkLoop16Hdr hInv.statusRun hPc min512BlkLoop16Hdr_ok
  have hL0 : 1 + min512BlkLoop16Hdr.length = 4 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkLoop16Hdr
  change (runAmd64Vec 1 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).pc = 180 ∧
    MinNoNaN512Inv base spF x n i
      (runAmd64Vec 1 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 146 := by
    simp [sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid 146 min512BlkLoop16Exit hMidRun hMidPc min512BlkLoop16Exit_ok
  have hL1 : 0 + min512BlkLoop16Exit.length = 1 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [min512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp], ?_⟩
  refine ⟨by simp [min512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMidRun],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.upperDirtyT],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.noHazard],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
    by simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.zmm1Eq],
    by intro j hj; simp [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.memX j hj],
    by simpa [min512BlkLoop16Exit, sMid, min512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn,
         evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp] using hInv.accs⟩

private theorem minNoNaN64AVX512_loop8_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 180) (hInv : MinNoNaN512Inv base spF x n i s)
    (hStep : i + 8 ≤ n) (hN : n + 32 < 2 ^ 63) :
    let s' := runAmd64Vec 9 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.pc = 180 ∧ MinNoNaN512Inv base spF x n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 189 (-41) = some 148 := by decide
  dsimp only [minNoNaN64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 6 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 180 min512BlkLoop8Hdr hInv.statusRun hPc min512BlkLoop8Hdr_ok
  have hL0 : 6 + min512BlkLoop8Hdr.length = 9 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkLoop8Hdr
  change (runAmd64Vec 6 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).pc = 180 ∧
    MinNoNaN512Inv base spF x n (i + 8)
      (runAmd64Vec 6 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid)
  have hMidRun : sMid.status = .running := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hMidPc : sMid.pc = 148 := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidDirt : sMid.upperDirty = true := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRdx : sMid.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hS1 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid 148 min512BlkLoop8Body hMidRun hMidPc min512BlkLoop8Body_ok
  have hL1 : 0 + min512BlkLoop8Body.length = 6 := rfl
  rw [← hL1, hS1]
  simp only [runAmd64Vec]
  rcases hInv.accs with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ0, hZ2,
    ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hX0 := hInv.memX i (by omega); have hX1 := hInv.memX (i + 1) (by omega)
  have hX2 := hInv.memX (i + 2) (by omega); have hX3 := hInv.memX (i + 3) (by omega)
  have hX4 := hInv.memX (i + 4) (by omega); have hX5 := hInv.memX (i + 5) (by omega)
  have hX6 := hInv.memX (i + 6) (by omega); have hX7 := hInv.memX (i + 7) (by omega)
  refine ⟨by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr], ?_⟩
  refine ⟨by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidDirt],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         Amd64VecInsn.isLegacySSE, hMidHaz],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRdx],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp],
    by simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         setZmm, hMidZmm, hInv.zmm1Eq],
    by intro j hj; simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
         hMidMem, hInv.memX j hj],
    ?_⟩
  -- Go 1.27 reverses `VMINPD` operand order in the 8-wide tail (`vminpdLane a_k (sanitizeNaNInf x[i+k])`).
  refine ⟨vminpdLane a0 (sanitizeNaNInf (x i)),
          vminpdLane a1 (sanitizeNaNInf (x (i + 1))),
          vminpdLane a2 (sanitizeNaNInf (x (i + 2))),
          vminpdLane a3 (sanitizeNaNInf (x (i + 3))),
          vminpdLane a4 (sanitizeNaNInf (x (i + 4))),
          vminpdLane a5 (sanitizeNaNInf (x (i + 5))),
          vminpdLane a6 (sanitizeNaNInf (x (i + 6))),
          vminpdLane a7 (sanitizeNaNInf (x (i + 7))),
          b0, b1, b2, b3, b4, b5, b6, b7,
    ?_, ?_,
    vminpdLane_notNaN ha0 (sanitizeNaNInf_notNaN _), vminpdLane_notNaN ha1 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha2 (sanitizeNaNInf_notNaN _), vminpdLane_notNaN ha3 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha4 (sanitizeNaNInf_notNaN _), vminpdLane_notNaN ha5 (sanitizeNaNInf_notNaN _),
    vminpdLane_notNaN ha6 (sanitizeNaNInf_notNaN _), vminpdLane_notNaN ha7 (sanitizeNaNInf_notNaN _),
    hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, ?_⟩
  · have hMask := maskBit_or8_bv
      (f64IsNaN (x i)) (f64IsNaN (x (i + 1))) (f64IsNaN (x (i + 2))) (f64IsNaN (x (i + 3)))
      (f64IsNaN (x (i + 4))) (f64IsNaN (x (i + 5))) (f64IsNaN (x (i + 6))) (f64IsNaN (x (i + 7)))
    simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      loadVecMem, setGpr, setZmm, setKmask, hMidRax, hMidRcx, hMidZmm, hMidMem, hInv.zmm1Eq, hZ0,
      hBaseAddr, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56,
      hX0, hX1, hX2, hX3, hX4, hX5, hX6, hX7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, x86VcmppdPred,
      hMask.1, hMask.2.1, hMask.2.2.1, hMask.2.2.2.1, hMask.2.2.2.2.1, hMask.2.2.2.2.2.1,
      hMask.2.2.2.2.2.2.1, hMask.2.2.2.2.2.2.2, sanitizeNaNInf]
  · simp [min512BlkLoop8Body, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, hMidZmm, hZ2]
  · rw [cmin64_vminpdLane_left _ ha0 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha1 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_left _ ha2 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha3 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_left _ ha4 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha5 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_left _ ha6 (sanitizeNaNInf_notNaN _),
        cmin64_vminpdLane_right _ ha7 (sanitizeNaNInf_notNaN _)]
    simp only [cmin64_sanitize_right]
    rw [specMinNoNaN64_add8, specMinNoNaN64_add4, ← hCmin]
    ac_rfl

private theorem minNoNaN64AVX512_spill_and_vzu (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 180) (hInv : MinNoNaN512Inv base spF x n i s)
    (hLt : n < i + 8) (hN : n + 32 < 2 ^ 63) (hi : i ≤ n)
    (hDisjStack : ∀ j < n, ∀ k < 16, elemAddr base j ≠ spF + BitVec.ofNat 64 (k * 8)) :
    let s' := runAmd64Vec 21 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.pc = 272 ∧ s'.gpr 2#4 = 0#64 ∧
    MinNoNaN2ScalarInv base spF x n i f64PosInf s' ∧
    ∃ l0 l1 l2 l3 l4 l5 l6 l7 : BitVec 64,
      s'.mem (spF + 64#64) = l0 ∧ s'.mem (spF + 72#64) = l1 ∧
      s'.mem (spF + 80#64) = l2 ∧ s'.mem (spF + 88#64) = l3 ∧
      s'.mem (spF + 96#64) = l4 ∧ s'.mem (spF + 104#64) = l5 ∧
      s'.mem (spF + 112#64) = l6 ∧ s'.mem (spF + 120#64) = l7 ∧
      f64NotNaN l0 = true ∧ f64NotNaN l1 = true ∧ f64NotNaN l2 = true ∧ f64NotNaN l3 = true ∧
      f64NotNaN l4 = true ∧ f64NotNaN l5 = true ∧ f64NotNaN l6 = true ∧ f64NotNaN l7 = true ∧
      cmin64 (cmin64 (cmin64 l0 l1) (cmin64 l2 l3)) (cmin64 (cmin64 l4 l5) (cmin64 l6 l7)) =
        specMinNoNaN64 x i := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hRo256 : minNoNaN64AVX512Rodata 256 = some f64PosInf := by decide
  have hJmp : nextTargetPc 266 6 = some 272 := by decide
  dsimp only [minNoNaN64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 18 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 180 min512BlkLoop8Hdr hInv.statusRun hPc min512BlkLoop8Hdr_ok
  have hL0 : 18 + min512BlkLoop8Hdr.length = 21 := rfl
  rw [← hL0, hS0]
  let sMid := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkLoop8Hdr
  have hMidRun : sMid.status = .running := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hMidPc : sMid.pc = 189 := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidHaz : sMid.sseHazard = false := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.noHazard]
  have hMidRax : sMid.gpr 0#4 = base := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidRcx : sMid.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidRbx : sMid.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidRsp : sMid.gpr 4#4 = spF := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidZmm : sMid.zmm = s.zmm := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hMidMem : sMid.mem = s.mem := by
    simp [sMid, min512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  change (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).pc = 272 ∧
    (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).gpr 2#4 = 0#64 ∧
    MinNoNaN2ScalarInv base spF x n i f64PosInf
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid) ∧
    ∃ l0 l1 l2 l3 l4 l5 l6 l7 : BitVec 64,
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 64#64) = l0 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 72#64) = l1 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 80#64) = l2 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 88#64) = l3 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 96#64) = l4 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 104#64) = l5 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 112#64) = l6 ∧
      (runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid).mem (spF + 120#64) = l7 ∧
      f64NotNaN l0 = true ∧ f64NotNaN l1 = true ∧ f64NotNaN l2 = true ∧ f64NotNaN l3 = true ∧
      f64NotNaN l4 = true ∧ f64NotNaN l5 = true ∧ f64NotNaN l6 = true ∧ f64NotNaN l7 = true ∧
      cmin64 (cmin64 (cmin64 l0 l1) (cmin64 l2 l3)) (cmin64 (cmin64 l4 l5) (cmin64 l6 l7)) =
        specMinNoNaN64 x i
  have hSA : runAmd64Vec 18 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid =
      runAmd64Vec 11 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
        (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid min512BlkSpillA) :=
    runAmd64Vec_block_seq 11 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid 189 min512BlkSpillA hMidRun hMidPc min512BlkSpillA_ok
  rw [hSA]
  let sA := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sMid min512BlkSpillA
  have hARun : sA.status = .running := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hMidRun]
  have hAPc : sA.pc = 216 := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = false := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, Amd64VecInsn.dirtiesUpper]
  have hAHaz : sA.sseHazard = false := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      Amd64VecInsn.isLegacySSE, hMidHaz]
  have hARax : sA.gpr 0#4 = base := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRax]
  have hARcx : sA.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRcx]
  have hARdx : sA.gpr 2#4 = spF + 64#64 := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRbx]
  have hARsp : sA.gpr 4#4 = spF := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  have hARsi : sA.gpr 6#4 = spF := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hMidRsp]
  rcases hInv.accs with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7, hZ0, hZ2,
    ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
  have hSlotNe : ∀ k1 k2 : Nat, k1 ≠ k2 → k1 < 16 → k2 < 16 →
      spF + BitVec.ofNat 64 (k1 * 8) ≠ spF + BitVec.ofNat 64 (k2 * 8) :=
    fun k1 k2 hNe hk1 hk2 => stackSlot_ne_offset spF k1 k2 hNe hk1 hk2
  let l0 := vminpdLane a0 b0; let l1 := vminpdLane a1 b1
  let l2 := vminpdLane a2 b2; let l3 := vminpdLane a3 b3
  let l4 := vminpdLane a4 b4; let l5 := vminpdLane a5 b5
  let l6 := vminpdLane a6 b6; let l7 := vminpdLane a7 b7
  have hAMemEq : sA.mem =
      write64 (write64 (write64 (write64 (write64 (write64 (write64 (write64 s.mem
        (spF + 0#64) l0) (spF + 8#64) l1) (spF + 16#64) l2) (spF + 24#64) l3)
        (spF + 32#64) l4) (spF + 40#64) l5) (spF + 48#64) l6) (spF + 56#64) l7 := by
    simp [sA, min512BlkSpillA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, storeVecMem,
      setZmm, hMidRsp, hMidZmm, hMidMem, hZ0, hZ2, applyEvexMask_zero, Vec512.applyWidth,
      Vec512.zipWith, l0, l1, l2, l3, l4, l5, l6, l7]
  have hAM0 : sA.mem spF = l0 := by
    suffices sA.mem (spF + 0#64) = l0 by simpa using this
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 0 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 2 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 0 1 (by decide) (by omega) (by omega)), write64_eq]
  have hAM1 : sA.mem (spF + 8#64) = l1 := by
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 1 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 1 2 (by decide) (by omega) (by omega)), write64_eq]
  have hAM2 : sA.mem (spF + 16#64) = l2 := by
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 2 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 2 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 2 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 2 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 2 3 (by decide) (by omega) (by omega)), write64_eq]
  have hAM3 : sA.mem (spF + 24#64) = l3 := by
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 3 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 3 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 3 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 3 4 (by decide) (by omega) (by omega)), write64_eq]
  have hAM4 : sA.mem (spF + 32#64) = l4 := by
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 4 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 5 (by decide) (by omega) (by omega)), write64_eq]
  have hAM5 : sA.mem (spF + 40#64) = l5 := by
    rw [hAMemEq,
        write64_ne _ _ (hSlotNe 5 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 5 6 (by decide) (by omega) (by omega)), write64_eq]
  have hAM6 : sA.mem (spF + 48#64) = l6 := by
    rw [hAMemEq, write64_ne _ _ (hSlotNe 6 7 (by decide) (by omega) (by omega)), write64_eq]
  have hAM7 : sA.mem (spF + 56#64) = l7 := by rw [hAMemEq, write64_eq]
  -- Step `SpillB1` (copies lanes 0..3 -> spF+64..spF+88).
  let sB1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sA min512BlkSpillB1
  have hSB1 : runAmd64Vec 11 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sA =
      runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sB1 :=
    runAmd64Vec_block_seq 7 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sA 216 min512BlkSpillB1 hARun hAPc min512BlkSpillB1_ok
  rw [hSB1]
  have hB1Run : sB1.status = .running := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hARun]
  have hB1Pc : sB1.pc = 234 := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1Dirt : sB1.upperDirty = false := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, Amd64VecInsn.dirtiesUpper, hADirt]
  have hB1Haz : sB1.sseHazard = false := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, hADirt, hAHaz]
  have hB1Gpr : sB1.gpr = sA.gpr := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have h64_8 : spF + 64#64 + 8#64 = spF + 72#64 := by bv_omega
  have h64_16 : spF + 64#64 + 16#64 = spF + 80#64 := by bv_omega
  have h80_8 : spF + 80#64 + 8#64 = spF + 88#64 := by bv_omega
  have h16_8 : spF + 16#64 + 8#64 = spF + 24#64 := by bv_omega
  have hB1MemEq : sB1.mem =
      write64 (write64 (write64 (write64 sA.mem
        (spF + 64#64) l0) (spF + 72#64) l1) (spF + 80#64) l2) (spF + 88#64) l3 := by
    simp [sB1, min512BlkSpillB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setZmm,
      hARdx, hARsi, h64_8, h64_16, h80_8, h16_8, hAM0, hAM1,
      write64_ne _ _ (hSlotNe 2 9 (by decide) (by omega) (by omega)),
      write64_ne _ _ (hSlotNe 2 8 (by decide) (by omega) (by omega)), hAM2,
      write64_ne _ _ (hSlotNe 3 9 (by decide) (by omega) (by omega)),
      write64_ne _ _ (hSlotNe 3 8 (by decide) (by omega) (by omega)), hAM3]
  have hB1M4 : sB1.mem (spF + 32#64) = l4 := by
    rw [hB1MemEq,
        write64_ne _ _ (hSlotNe 4 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 10 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 9 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 4 8 (by decide) (by omega) (by omega)), hAM4]
  have hB1M5 : sB1.mem (spF + 40#64) = l5 := by
    rw [hB1MemEq,
        write64_ne _ _ (hSlotNe 5 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 5 10 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 5 9 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 5 8 (by decide) (by omega) (by omega)), hAM5]
  have hB1M6 : sB1.mem (spF + 48#64) = l6 := by
    rw [hB1MemEq,
        write64_ne _ _ (hSlotNe 6 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 6 10 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 6 9 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 6 8 (by decide) (by omega) (by omega)), hAM6]
  have hB1M7 : sB1.mem (spF + 56#64) = l7 := by
    rw [hB1MemEq,
        write64_ne _ _ (hSlotNe 7 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 7 10 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 7 9 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 7 8 (by decide) (by omega) (by omega)), hAM7]
  -- Step `SpillB2` (copies lanes 4..7 -> spF+96..spF+120 and sets up horiz loop).
  have hSB2 : runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sB1 =
      runAmd64Vec 0 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
        (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sB1 min512BlkSpillB2) :=
    runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sB1 234 min512BlkSpillB2 hB1Run hB1Pc min512BlkSpillB2_ok
  rw [hSB2]
  simp only [runAmd64Vec]
  have h64_32 : spF + 64#64 + 32#64 = spF + 96#64 := by bv_omega
  have h96_8 : spF + 96#64 + 8#64 = spF + 104#64 := by bv_omega
  have h64_48 : spF + 64#64 + 48#64 = spF + 112#64 := by bv_omega
  have h112_8 : spF + 112#64 + 8#64 = spF + 120#64 := by bv_omega
  have h32_8 : spF + 32#64 + 8#64 = spF + 40#64 := by bv_omega
  have h48_8 : spF + 48#64 + 8#64 = spF + 56#64 := by bv_omega
  have hMemFinal :
      (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sB1 min512BlkSpillB2).mem =
        write64 (write64 (write64 (write64 sB1.mem
          (spF + 96#64) l4) (spF + 104#64) l5) (spF + 112#64) l6) (spF + 120#64) l7 := by
    simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setZmm,
      hB1Gpr, hARdx, hARsi, hRo256, hJmp, h64_32, h96_8, h64_48, h112_8, h32_8, h48_8,
      hB1M4, hB1M5,
      write64_ne _ _ (hSlotNe 6 13 (by decide) (by omega) (by omega)),
      write64_ne _ _ (hSlotNe 6 12 (by decide) (by omega) (by omega)), hB1M6,
      write64_ne _ _ (hSlotNe 7 13 (by decide) (by omega) (by omega)),
      write64_ne _ _ (hSlotNe 7 12 (by decide) (by omega) (by omega)), hB1M7]
  refine ⟨by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, hRo256, hJmp],
    by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo256, hJmp],
    ⟨by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, hRo256, hJmp, hB1Run],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64,
          Amd64VecInsn.dirtiesUpper, hRo256, hJmp, hB1Dirt],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64,
          Amd64VecInsn.isLegacySSE, Amd64VecInsn.dirtiesUpper, hRo256, hJmp, hB1Dirt, hB1Haz],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo256, hJmp, hB1Gpr, hARax],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo256, hJmp, hB1Gpr, hARcx],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo256, hJmp, hB1Gpr, hARbx],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setGpr, hRo256, hJmp, hB1Gpr, hARsp],
     by simp [min512BlkSpillB2, execDecodedSeq, execAmd64VecInsn, evalMemAddr, readMem64, setZmm, hRo256, hJmp],
     f64PosInf_notNaN,
     by intro j hj
        rw [hMemFinal, hB1MemEq, hAMemEq]
        rw [write64_ne _ _ (hDisjStack j hj 15 (by omega)), write64_ne _ _ (hDisjStack j hj 14 (by omega)),
            write64_ne _ _ (hDisjStack j hj 13 (by omega)), write64_ne _ _ (hDisjStack j hj 12 (by omega)),
            write64_ne _ _ (hDisjStack j hj 11 (by omega)), write64_ne _ _ (hDisjStack j hj 10 (by omega)),
            write64_ne _ _ (hDisjStack j hj 9 (by omega)), write64_ne _ _ (hDisjStack j hj 8 (by omega)),
            write64_ne _ _ (hDisjStack j hj 7 (by omega)), write64_ne _ _ (hDisjStack j hj 6 (by omega)),
            write64_ne _ _ (hDisjStack j hj 5 (by omega)), write64_ne _ _ (hDisjStack j hj 4 (by omega)),
            write64_ne _ _ (hDisjStack j hj 3 (by omega)), write64_ne _ _ (hDisjStack j hj 2 (by omega)),
            write64_ne _ _ (hDisjStack j hj 1 (by omega)), write64_ne _ _ (hDisjStack j hj 0 (by omega))]
        exact hInv.memX j hj⟩,
    l0, l1, l2, l3, l4, l5, l6, l7, ?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_,
    vminpdLane_notNaN ha0 hb0, vminpdLane_notNaN ha1 hb1, vminpdLane_notNaN ha2 hb2, vminpdLane_notNaN ha3 hb3,
    vminpdLane_notNaN ha4 hb4, vminpdLane_notNaN ha5 hb5, vminpdLane_notNaN ha6 hb6, vminpdLane_notNaN ha7 hb7, ?_⟩
  · rw [hMemFinal, hB1MemEq,
        write64_ne _ _ (hSlotNe 8 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 14 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 13 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 12 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 10 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 8 9 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal, hB1MemEq,
        write64_ne _ _ (hSlotNe 9 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 9 14 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 9 13 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 9 12 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 9 11 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 9 10 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal, hB1MemEq,
        write64_ne _ _ (hSlotNe 10 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 10 14 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 10 13 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 10 12 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 10 11 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal, hB1MemEq,
        write64_ne _ _ (hSlotNe 11 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 11 14 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 11 13 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 11 12 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 12 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 12 14 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 12 13 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 13 15 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hSlotNe 13 14 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal,
        write64_ne _ _ (hSlotNe 14 15 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hMemFinal, write64_eq]
  · dsimp only [l0, l1, l2, l3, l4, l5, l6, l7]
    rw [cmin64_combine8_eq ha0 ha1 ha2 ha3 ha4 ha5 ha6 ha7 hb0 hb1 hb2 hb3 hb4 hb5 hb6 hb7, hCmin]

private theorem minNoNaN64AVX512_horiz_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i k : Nat) (acc lk : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 272) (hk : k < 8) (hRdx : s.gpr 2#4 = BitVec.ofNat 64 k)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s)
    (hSlot : s.mem (spF + BitVec.ofNat 64 (k * 8) + 64#64) = lk) :
    ∃ steps ≤ 9,
      let s' := runAmd64Vec steps minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
      s'.pc = 272 ∧ s'.gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧ s'.mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 k) 8#64) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd1 : BitVec.ofNat 64 k + 1#64 = BitVec.ofNat 64 (k + 1) := bitVec_ofNat_add k 1
  have hSlotAddr : spF + BitVec.ofNat 64 k * 8#64 + 64#64 = spF + BitVec.ofNat 64 (k * 8) + 64#64 := by
    have hMul : BitVec.ofNat 64 k * 8#64 = BitVec.ofNat 64 (k * 8) := by
      apply BitVec.eq_of_toNat_eq; simp [BitVec.toNat_mul, Nat.mul_mod]
    rw [hMul]
  have hJa : evalX86Cond .a (x86UcomisdFlags acc lk) = f64Lt lk acc := evalX86Cond_a_ucomisd acc lk
  have hJcc266 : nextTargetPc 290 (-24) = some 266 := by decide
  have hJmp266 : nextTargetPc 295 (-29) = some 266 := by decide
  by_cases hLt : f64Lt lk acc = true
  · refine ⟨7, by omega, ?_⟩
    dsimp only [minNoNaN64AVX512Bytes]
    have hS0 := runAmd64Vec_block_seq 5 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 272 min512BlkHorizHdr hInv.statusRun hPc min512BlkHorizHdr_ok
    have hL0 : 5 + min512BlkHorizHdr.length = 7 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkHorizHdr
    have h1Run : s1.status = .running := by
      simp [s1, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hInv.statusRun]
    have h1Pc : s1.pc = 278 := by
      simp [s1, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe]
    change (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).pc = 272 ∧
      (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1)
    have hS1 := runAmd64Vec_block_seq 2 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 278 min512BlkHorizCmp h1Run h1Pc min512BlkHorizCmp_ok
    have hL1 : 2 + min512BlkHorizCmp.length = 5 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 min512BlkHorizCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLt, hJcc266, hInv.statusRun]
    have h2Pc : s2.pc = 266 := by
      simp [s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLt, hJcc266]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).pc = 272 ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2)
    have hS2 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 266 min512BlkHorizInc h2Run h2Pc min512BlkHorizInc_ok
    have hL2 : 0 + min512BlkHorizInc.length = 2 := rfl
    rw [← hL2, hS2]
    simp only [runAmd64Vec]
    have hMem2 : s2.mem = s.mem := by
      simp [s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLt, hJcc266]
    refine ⟨by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn],
      by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
           readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
           hJa, hLt, hJcc266, hAdd1],
      by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem2],
      ⟨by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, h2Run],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266, hInv.upperDirtyF],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, readMem64, evalMemAddr, setZmm,
            hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLt, hJcc266, hInv.upperDirtyF, hInv.noHazard],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.raxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rcxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266],
       by simp [min512BlkHorizInc, s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
            hSlotAddr, hSlot, hJa, hLt, hJcc266, minStepNoNaN],
       minStepNoNaN_notNaN lk hInv.accNotNaN,
       by intro j hj; simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem2, hInv.memX j hj]⟩⟩
  · have hLtF : f64Lt lk acc = false := Bool.eq_false_iff.mpr hLt
    refine ⟨9, by omega, ?_⟩
    dsimp only [minNoNaN64AVX512Bytes]
    have hS0 := runAmd64Vec_block_seq 7 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 272 min512BlkHorizHdr hInv.statusRun hPc min512BlkHorizHdr_ok
    have hL0 : 7 + min512BlkHorizHdr.length = 9 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkHorizHdr
    have h1Run : s1.status = .running := by
      simp [s1, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hInv.statusRun]
    have h1Pc : s1.pc = 278 := by
      simp [s1, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe]
    change (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).pc = 272 ∧
      (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1)
    have hS1 := runAmd64Vec_block_seq 4 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 278 min512BlkHorizCmp h1Run h1Pc min512BlkHorizCmp_ok
    have hL1 : 4 + min512BlkHorizCmp.length = 7 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 min512BlkHorizCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLtF, hInv.statusRun]
    have h2Pc : s2.pc = 290 := by
      simp [s2, s1, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
        hJa, hLtF]
    change (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).pc = 272 ∧
      (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2)
    have hS2 : runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 =
        runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
          (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 min512BlkHorizKeep) :=
      runAmd64Vec_block_seq 2 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
        minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 290 min512BlkHorizKeep h2Run h2Pc min512BlkHorizKeep_ok
    rw [hS2]
    let s3 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 min512BlkHorizKeep
    have h3Run : s3.status = .running := by
      simp [s3, min512BlkHorizKeep, execDecodedSeq, execAmd64VecInsn, hJmp266, h2Run]
    have h3Pc : s3.pc = 266 := by
      simp [s3, min512BlkHorizKeep, execDecodedSeq, execAmd64VecInsn, hJmp266]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3).pc = 272 ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3).gpr 2#4 = BitVec.ofNat 64 (k + 1) ∧
      (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3).mem = s.mem ∧
      MinNoNaN2ScalarInv base spF x n i (minStepNoNaN acc lk)
        (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3)
    have hS3 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 266 min512BlkHorizInc h3Run h3Pc min512BlkHorizInc_ok
    have hL3 : 0 + min512BlkHorizInc.length = 2 := rfl
    rw [← hL3, hS3]
    simp only [runAmd64Vec]
    have hMem3 : s3.mem = s.mem := by
      simp [s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr, execDecodedSeq,
        execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq,
        hSlotAddr, hSlot, hJa, hLtF, hJmp266]
    refine ⟨by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn],
      by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
           execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
           hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266, hAdd1],
      by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem3],
      ⟨by simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, h3Run],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm,
            hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266, hInv.upperDirtyF],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
            readMem64, evalMemAddr, setZmm, hRdx, hGe, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot,
            hJa, hLtF, hJmp266, hInv.upperDirtyF, hInv.noHazard],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.raxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rcxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm, hRdx, hGe,
            hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266],
       by simp [min512BlkHorizInc, s3, s2, s1, min512BlkHorizKeep, min512BlkHorizCmp, min512BlkHorizHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hRdx, hGe,
            hInv.rspEq, hInv.xmm0Eq, hSlotAddr, hSlot, hJa, hLtF, hJmp266, minStepNoNaN],
       minStepNoNaN_notNaN lk hInv.accNotNaN,
       by intro j hj; simp [min512BlkHorizInc, execDecodedSeq, execAmd64VecInsn, hMem3, hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX512_horiz_exit (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 272) (hRdx : s.gpr 2#4 = 8#64)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s) :
    let s' := runAmd64Vec 2 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
    s'.pc = 301 ∧ MinNoNaN2ScalarInv base spF x n i acc s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 8#64 8#64) = true := by decide
  have hJcc : nextTargetPc 278 23 = some 301 := by decide
  dsimp only [minNoNaN64AVX512Bytes]
  have hS0 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
    minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 272 min512BlkHorizHdr hInv.statusRun hPc min512BlkHorizHdr_ok
  have hL0 : 0 + min512BlkHorizHdr.length = 2 := rfl
  rw [← hL0, hS0]
  simp only [runAmd64Vec]
  refine ⟨by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc],
    ⟨by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.statusRun],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
          hRdx, hGe, hJcc, hInv.upperDirtyF],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
          hRdx, hGe, hJcc, hInv.noHazard],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.raxEq],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rcxEq],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rbxEq],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.rspEq],
     by simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.xmm0Eq],
     hInv.accNotNaN,
     by intro j hj; simp [min512BlkHorizHdr, execDecodedSeq, execAmd64VecInsn, hRdx, hGe, hJcc, hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX512_tail_step (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n i : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 301) (hi : i < n) (hN : n + 32 < 2 ^ 63)
    (hInv : MinNoNaN2ScalarInv base spF x n i acc s) :
    ∃ steps ≤ 9,
      let s' := runAmd64Vec steps minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
      s'.pc = 301 ∧ MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i)) s' := by
  have hLe : evalX86Cond .le (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 i)) = false := by
    rw [evalX86Cond_le_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd1 : BitVec.ofNat 64 i + 1#64 = BitVec.ofNat 64 (i + 1) := bitVec_ofNat_add i 1
  have hBaseAddr : base + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr base i := by
    unfold elemAddr; simp
  have hXi := hInv.memX i hi
  have hJa : evalX86Cond .a (x86UcomisdFlags acc (x i)) = f64Lt (x i) acc := evalX86Cond_a_ucomisd acc (x i)
  have hJcc295 : nextTargetPc 317 (-22) = some 295 := by decide
  have hJmp295 : nextTargetPc 322 (-27) = some 295 := by decide
  by_cases hLt : f64Lt (x i) acc = true
  · refine ⟨7, by omega, ?_⟩
    dsimp only [minNoNaN64AVX512Bytes]
    have hS0 := runAmd64Vec_block_seq 5 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 301 min512BlkTailHdr hInv.statusRun hPc min512BlkTailHdr_ok
    have hL0 : 5 + min512BlkTailHdr.length = 7 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkTailHdr
    have h1Run : s1.status = .running := by
      simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hInv.statusRun]
    have h1Pc : s1.pc = 306 := by
      simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe]
    change (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).pc = 301 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1)
    have hS1 := runAmd64Vec_block_seq 2 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 306 min512BlkTailCmp h1Run h1Pc min512BlkTailCmp_ok
    have hL1 : 2 + min512BlkTailCmp.length = 5 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 min512BlkTailCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLt, hJcc295, hInv.statusRun]
    have h2Pc : s2.pc = 295 := by
      simp [s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLt, hJcc295]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).pc = 301 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2)
    have hS2 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 295 min512BlkTailInc h2Run h2Pc min512BlkTailInc_ok
    have hL2 : 0 + min512BlkTailInc.length = 2 := rfl
    rw [← hL2, hS2]
    simp only [runAmd64Vec]
    refine ⟨by simp [min512BlkTailInc, execDecodedSeq, execAmd64VecInsn],
      ⟨by simp [min512BlkTailInc, execDecodedSeq, execAmd64VecInsn, h2Run],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc295, hInv.upperDirtyF],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc295,
            hInv.upperDirtyF, hInv.noHazard],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc295],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc295, hAdd1],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc295],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setGpr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.rspEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc295],
       by simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
            readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLt, hJcc295, minStepNoNaN],
       minStepNoNaN_notNaN (x i) hInv.accNotNaN,
       by intro j hj; simp [min512BlkTailInc, s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq,
            execAmd64VecInsn, readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq,
            hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLt, hJcc295, hInv.memX j hj]⟩⟩
  · have hLtF : f64Lt (x i) acc = false := Bool.eq_false_iff.mpr hLt
    refine ⟨9, by omega, ?_⟩
    dsimp only [minNoNaN64AVX512Bytes]
    have hS0 := runAmd64Vec_block_seq 7 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 301 min512BlkTailHdr hInv.statusRun hPc min512BlkTailHdr_ok
    have hL0 : 7 + min512BlkTailHdr.length = 9 := rfl
    rw [← hL0, hS0]
    let s1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkTailHdr
    have h1Run : s1.status = .running := by
      simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hInv.statusRun]
    have h1Pc : s1.pc = 306 := by
      simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe]
    change (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1).pc = 301 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1)
    have hS1 := runAmd64Vec_block_seq 4 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 306 min512BlkTailCmp h1Run h1Pc min512BlkTailCmp_ok
    have hL1 : 4 + min512BlkTailCmp.length = 7 := rfl
    rw [← hL1, hS1]
    let s2 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 min512BlkTailCmp
    have h2Run : s2.status = .running := by
      simp [s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLtF, hInv.statusRun]
    have h2Pc : s2.pc = 317 := by
      simp [s2, s1, min512BlkTailCmp, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn,
        readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
        hLe, hBaseAddr, hXi, hJa, hLtF]
    change (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2).pc = 301 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2)
    have hS2 := runAmd64Vec_block_seq 2 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 317 min512BlkTailKeep h2Run h2Pc min512BlkTailKeep_ok
    have hL2 : 2 + min512BlkTailKeep.length = 4 := rfl
    rw [← hL2, hS2]
    let s3 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 min512BlkTailKeep
    have h3Run : s3.status = .running := by
      simp [s3, min512BlkTailKeep, execDecodedSeq, execAmd64VecInsn, hJmp295, h2Run]
    have h3Pc : s3.pc = 295 := by
      simp [s3, min512BlkTailKeep, execDecodedSeq, execAmd64VecInsn, hJmp295]
    change (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3).pc = 301 ∧
      MinNoNaN2ScalarInv base spF x n (i + 1) (minStepNoNaN acc (x i))
        (runAmd64Vec 2 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3)
    have hS3 := runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
      minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 295 min512BlkTailInc h3Run h3Pc min512BlkTailInc_ok
    have hL3 : 0 + min512BlkTailInc.length = 2 := rfl
    rw [← hL3, hS3]
    simp only [runAmd64Vec]
    refine ⟨by simp [min512BlkTailInc, execDecodedSeq, execAmd64VecInsn],
      ⟨by simp [min512BlkTailInc, execDecodedSeq, execAmd64VecInsn, h3Run],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295,
            hInv.upperDirtyF],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
            readMem64, evalMemAddr, setZmm, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq,
            hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295, hInv.upperDirtyF, hInv.noHazard],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295, hAdd1],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setGpr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.rspEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295],
       by simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295, minStepNoNaN],
       minStepNoNaN_notNaN (x i) hInv.accNotNaN,
       by intro j hj; simp [min512BlkTailInc, s3, s2, s1, min512BlkTailKeep, min512BlkTailCmp, min512BlkTailHdr,
            execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
            hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hInv.xmm0Eq, hLe, hBaseAddr, hXi, hJa, hLtF, hJmp295,
            hInv.memX j hj]⟩⟩

private theorem minNoNaN64AVX512_tail_iter (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (hN : n + 32 < 2 ^ 63) :
    ∀ rem i acc s, i + rem = n → s.pc = 301 →
      MinNoNaN2ScalarInv base spF x n i acc s →
      canonZero64 acc = specMinNoNaN64 x i →
      ∃ steps ≤ 9 * rem,
        let s' := runAmd64Vec steps minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
        s'.pc = 301 ∧ ∃ acc',
          MinNoNaN2ScalarInv base spF x n n acc' s' ∧ canonZero64 acc' = specMinNoNaN64 x n := by
  intro rem
  induction rem with
  | zero =>
    intro i acc s hEq hPc hInv hCanon
    have hi : i = n := by omega
    subst hi
    exact ⟨0, by omega, by simpa [runAmd64Vec] using hPc, acc, by simpa [runAmd64Vec] using hInv, hCanon⟩
  | succ r ih =>
    intro i acc s hEq hPc hInv hCanon
    have h1 := minNoNaN64AVX512_tail_step base spF x n i acc s hPc (by omega) hN hInv
    rcases h1 with ⟨st1, hSt1Le, hPc1, hInv1⟩
    have hCanon1 : canonZero64 (minStepNoNaN acc (x i)) = specMinNoNaN64 x (i + 1) := by
      rw [cmin64_minStepNoNaN (x i) hInv.accNotNaN, ← cmin64_canonZero_left, hCanon, specMinNoNaN64_succ]
    have h2 := ih (i + 1) (minStepNoNaN acc (x i)) _ (by omega) hPc1 hInv1 hCanon1
    rcases h2 with ⟨st2, hSt2Le, hPc2, acc', hInv2, hCanon2⟩
    refine ⟨st1 + st2, by omega, ?_, acc', ?_, hCanon2⟩
    · rw [runAmd64Vec_add]; exact hPc2
    · rw [runAmd64Vec_add]; exact hInv2

private theorem minNoNaN64AVX512_canon_and_ret (base spF : BitVec 64) (x : Nat → BitVec 64)
    (n : Nat) (acc : BitVec 64) (s : Amd64VecState)
    (hPc : s.pc = 301) (hN : n + 32 < 2 ^ 63)
    (hInv : MinNoNaN2ScalarInv base spF x n n acc s) :
    ∃ steps ≤ 10,
      let s' := runAmd64Vec steps minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s
      s'.status = .returned ∧ s'.upperDirty = false ∧ s'.sseHazard = false ∧
      (s'.zmm 0#5).l0 = canonZero64 acc := by
  have hLe : evalX86Cond .le (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 n)) = true := by
    rw [evalX86Cond_le_sub64 (by omega) (by omega)]; exact decide_eq_true (Nat.le_refl n)
  have hJle : nextTargetPc 306 16 = some 322 := by decide
  have hJne : nextTargetPc 331 5 = some 336 := by decide
  have hJp : nextTargetPc 333 3 = some 336 := by decide
  have hXorSelf : ∀ v : BitVec 64, v ^^^ v = 0#64 := fun _ => BitVec.xor_self
  have hCanon := x86_ucomisd_zero_canon_eq_canonZero64 acc
  let s1 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s min512BlkTailHdr
  have h1Run : s1.status = .running := by
    simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.statusRun]
  have h1Pc : s1.pc = 322 := by
    simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle]
  have h1Dirt : s1.upperDirty = false := by
    simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
      hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.upperDirtyF]
  have h1Haz : s1.sseHazard = false := by
    simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.noHazard]
  have h1Z0 : (s1.zmm 0#5).l0 = acc := by
    simp [s1, min512BlkTailHdr, execDecodedSeq, execAmd64VecInsn, hInv.rbxEq, hInv.rcxEq, hLe, hJle, hInv.xmm0Eq]
  let s2 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 min512BlkCanon0
  by_cases hNe : evalX86Cond .ne (x86UcomisdFlags acc 0#64) = true
  · refine ⟨8, by omega, ?_⟩
    dsimp only [minNoNaN64AVX512Bytes]
    have hS0 : runAmd64Vec 8 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s =
        runAmd64Vec 6 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 :=
      runAmd64Vec_block_seq 6 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
        minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 301 min512BlkTailHdr hInv.statusRun hPc min512BlkTailHdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 6 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 =
        runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 :=
      runAmd64Vec_block_seq 3 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
        minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 322 min512BlkCanon0 h1Run h1Pc min512BlkCanon0_ok
    rw [hS1]
    have h2Run : s2.status = .running := by
      simp [s2, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNe, hJne, h1Run]
    have h2Pc : s2.pc = 336 := by
      simp [s2, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNe, hJne]
    have hS2 : runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 =
        runAmd64Vec 0 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
          (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 min512BlkRet) :=
      runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
        minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 336 min512BlkRet h2Run h2Pc min512BlkRet_ok
    rw [hS2]
    have hAccEq : acc = canonZero64 acc := by simpa [hNe] using hCanon
    rw [← hAccEq]
    simp [runAmd64Vec, min512BlkRet, s2, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn,
      Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setZmm, h1Z0, hXorSelf, hNe, hJne,
      h1Dirt, h1Haz]
  · have hNeF : evalX86Cond .ne (x86UcomisdFlags acc 0#64) = false := Bool.eq_false_iff.mpr hNe
    have h2Run : s2.status = .running := by
      simp [s2, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNeF, h1Run]
    have h2Pc : s2.pc = 331 := by
      simp [s2, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm, h1Z0, hXorSelf, hNeF]
    let s3 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 min512BlkCanonJp
    by_cases hP : evalX86Cond .p (x86UcomisdFlags acc 0#64) = true
    · refine ⟨9, by omega, ?_⟩
      dsimp only [minNoNaN64AVX512Bytes]
      have hS0 : runAmd64Vec 9 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s =
          runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 :=
        runAmd64Vec_block_seq 7 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 301 min512BlkTailHdr hInv.statusRun hPc min512BlkTailHdr_ok
      rw [hS0]
      have hS1 : runAmd64Vec 7 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 =
          runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 :=
        runAmd64Vec_block_seq 4 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 322 min512BlkCanon0 h1Run h1Pc min512BlkCanon0_ok
      rw [hS1]
      have hS2 : runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 =
          runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 :=
        runAmd64Vec_block_seq 3 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 331 min512BlkCanonJp h2Run h2Pc min512BlkCanonJp_ok
      rw [hS2]
      have h3Run : s3.status = .running := by
        simp [s3, s2, min512BlkCanonJp, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hP, hJp, h1Run]
      have h3Pc : s3.pc = 336 := by
        simp [s3, s2, min512BlkCanonJp, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hP, hJp]
      have hS3 : runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 =
          runAmd64Vec 0 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
            (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 min512BlkRet) :=
        runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 336 min512BlkRet h3Run h3Pc min512BlkRet_ok
      rw [hS3]
      have hAccEq : acc = canonZero64 acc := by simpa [hNeF, hP] using hCanon
      rw [← hAccEq]
      simp [runAmd64Vec, min512BlkRet, s3, s2, min512BlkCanonJp, min512BlkCanon0, execDecodedSeq,
        execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, setZmm,
        h1Z0, hXorSelf, hNeF, hP, hJp, h1Dirt, h1Haz]
    · have hPF : evalX86Cond .p (x86UcomisdFlags acc 0#64) = false := Bool.eq_false_iff.mpr hP
      refine ⟨10, by omega, ?_⟩
      dsimp only [minNoNaN64AVX512Bytes]
      have hS0 : runAmd64Vec 10 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s =
          runAmd64Vec 8 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 :=
        runAmd64Vec_block_seq 8 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s 301 min512BlkTailHdr hInv.statusRun hPc min512BlkTailHdr_ok
      rw [hS0]
      have hS1 : runAmd64Vec 8 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 =
          runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 :=
        runAmd64Vec_block_seq 5 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s1 322 min512BlkCanon0 h1Run h1Pc min512BlkCanon0_ok
      rw [hS1]
      have hS2 : runAmd64Vec 5 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 =
          runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 :=
        runAmd64Vec_block_seq 4 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s2 331 min512BlkCanonJp h2Run h2Pc min512BlkCanonJp_ok
      rw [hS2]
      have h3Run : s3.status = .running := by
        simp [s3, s2, min512BlkCanonJp, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hPF, h1Run]
      have h3Pc : s3.pc = 333 := by
        simp [s3, s2, min512BlkCanonJp, min512BlkCanon0, execDecodedSeq, execAmd64VecInsn, setZmm,
          h1Z0, hXorSelf, hNeF, hPF]
      let s4 := execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 min512BlkCanonZero
      have hS3 : runAmd64Vec 4 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 =
          runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s4 :=
        runAmd64Vec_block_seq 3 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s3 333 min512BlkCanonZero h3Run h3Pc min512BlkCanonZero_ok
      rw [hS3]
      have h4Run : s4.status = .running := by
        simp [s4, min512BlkCanonZero, execDecodedSeq, execAmd64VecInsn, h3Run]
      have h4Pc : s4.pc = 336 := by
        simp [s4, min512BlkCanonZero, execDecodedSeq, execAmd64VecInsn]
      have hS4 : runAmd64Vec 3 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s4 =
          runAmd64Vec 0 ⟨minNoNaN64AVX512ByteChunks.flatten⟩ minNoNaN64AVX512Rodata minNoNaN64AVX512Targets
            (execDecodedSeq minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s4 min512BlkRet) :=
        runAmd64Vec_block_seq 0 minNoNaN64AVX512ByteChunks minNoNaN64AVX512_chunks_wf
          minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s4 336 min512BlkRet h4Run h4Pc min512BlkRet_ok
      rw [hS4]
      have hZeroEq : 0#64 = canonZero64 acc := by simpa [hNeF, hPF] using hCanon
      rw [← hZeroEq]
      simp [runAmd64Vec, min512BlkRet, s4, s3, s2, min512BlkCanonZero, min512BlkCanonJp, min512BlkCanon0,
        execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE,
        setZmm, h1Z0, hXorSelf, hNeF, hPF, h1Dirt, h1Haz]

/-- Generous upper bound on step count for `minNoNaN64AVX512` on arbitrary slice length `n = 8q + r`. -/
def minNoNaN64AVX512FuelBound (n : Nat) : Nat :=
  17 + 15 * (n / 16) + 4 + 9 + 21 + 74 + 9 * (n % 8) + 10

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`minNoNaN64AVX512`)**:
For arbitrary slice length `n = 8q + r` (`0 < n`, `n + 32 < 2^63`), assuming sufficient goroutine
stack headroom (`hGuard : g.stackguard0 < rsp - 8`, covering 2x-unrolled 16-wide EVEX 512-bit
loop with `VCMPUNORDPD -> k` and `VPBLENDMQ {k}`, 8-wide EVEX vector tail with reversed `VMINPD`
operand order, 8-lane horizontal SSE loop, arbitrary scalar tail `r ∈ {0..7}`, zero-trip vector
cases `n < 8` / `n < 16`, signed-zero asymmetry, subnormals, `±Inf`, and `NaN` replacement):
1. Under the stack-headroom hypothesis `hGuard`, proves the `runtime.morestack_noctxt.abi0`
   stack-split branch is not taken and `runtime.panicBounds` is unreachable;
2. Terminates with `status = .returned`, `upperDirty = false` (`VZEROUPPER` ABI safety before
   every legacy SSE instruction and before `RET`), and `sseHazard = false`;
3. Returns bit-exact `XMM0.l0 = specMinNoNaN64 x n` (`Track5D.canonZero64` of sequential min);
4. Preserves Go amd64 ABI registers: `RSP = s0.gpr 4#4 + 8#64`, `RBP = s0.gpr 5#4`,
   `R14 = s0.gpr 14#4`, and `X15` low 128 bits (`l0`, `l1`). -/
theorem minNoNaN64AVX512_functional_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hGuard : BitVec.ult (s0.mem (s0.gpr 14#4 + 16#64)) (s0.gpr 4#4 - 8#64) = true)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 16, elemAddr base j ≠ s0.gpr 4#4 - 136#64 + BitVec.ofNat 64 (k * 8))
    (hFuel : minNoNaN64AVX512FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    (sf.zmm 0#5).l0 = specMinNoNaN64 x n ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  let spF := s0.gpr 4#4 - 136#64
  have hPro := minNoNaN64AVX512_prologue base x n s0 hPc hRun hHaz hGuard hPos hN hRax hRbx hX
    (fun j hj => ⟨(hDisj j hj).1, (hDisj j hj).2.1⟩)
  have hL16 := minNoNaN64AVX512_loop16_iter base spF x n hN (n / 16) 0 _ (by omega) hPro.1 hPro.2
  have hEx16 := minNoNaN64AVX512_loop16_exit base spF x n (16 * (n / 16)) _
    hL16.1 (by simpa using hL16.2) (by omega) hN (by omega)
  have finishFrom180 : ∀ sVec, sVec.pc = 180 →
      MinNoNaN512Inv base spF x n (8 * (n / 8)) sVec →
      ∃ stRest ≤ 21 + 74 + 9 * (n % 8) + 10,
        let sEnd := runAmd64Vec stRest minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sVec
        sEnd.status = .returned ∧ sEnd.upperDirty = false ∧ sEnd.sseHazard = false ∧
        (sEnd.zmm 0#5).l0 = specMinNoNaN64 x n := by
    intro sVec hVecPc hVecInv
    have hSpill := minNoNaN64AVX512_spill_and_vzu base spF x n (8 * (n / 8)) sVec
      hVecPc hVecInv (by omega) hN (by omega) (fun j hj => (hDisj j hj).2.2)
    rcases hSpill with ⟨hPcSp, hRdxSp, hSc0, l0, l1, l2, l3, l4, l5, l6, l7,
      hM0, hM1, hM2, hM3, hM4, hM5, hM6, hM7,
      hl0, hl1, hl2, hl3, hl4, hl5, hl6, hl7, hLanesEq⟩
    let sSp := runAmd64Vec 21 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets sVec
    have hA0 : spF + BitVec.ofNat 64 (0 * 8) + 64#64 = spF + 64#64 := by bv_omega
    have hA1 : spF + BitVec.ofNat 64 (1 * 8) + 64#64 = spF + 72#64 := by bv_omega
    have hA2 : spF + BitVec.ofNat 64 (2 * 8) + 64#64 = spF + 80#64 := by bv_omega
    have hA3 : spF + BitVec.ofNat 64 (3 * 8) + 64#64 = spF + 88#64 := by bv_omega
    have hA4 : spF + BitVec.ofNat 64 (4 * 8) + 64#64 = spF + 96#64 := by bv_omega
    have hA5 : spF + BitVec.ofNat 64 (5 * 8) + 64#64 = spF + 104#64 := by bv_omega
    have hA6 : spF + BitVec.ofNat 64 (6 * 8) + 64#64 = spF + 112#64 := by bv_omega
    have hA7 : spF + BitVec.ofNat 64 (7 * 8) + 64#64 = spF + 120#64 := by bv_omega
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 0 f64PosInf l0 sSp
      hPcSp (by omega) hRdxSp hSc0 (by rw [hA0]; exact hM0) with ⟨st0, hSt0, hPc0, hRdx0, hMem0, hInv0⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 1 _ l1 _
      hPc0 (by omega) hRdx0 hInv0 (by rw [hMem0, hA1]; exact hM1) with ⟨st1, hSt1, hPc1, hRdx1, hMem1, hInv1⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 2 _ l2 _
      hPc1 (by omega) hRdx1 hInv1 (by rw [hMem1, hMem0, hA2]; exact hM2) with ⟨st2, hSt2, hPc2, hRdx2, hMem2, hInv2⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 3 _ l3 _
      hPc2 (by omega) hRdx2 hInv2 (by rw [hMem2, hMem1, hMem0, hA3]; exact hM3) with ⟨st3, hSt3, hPc3, hRdx3, hMem3, hInv3⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 4 _ l4 _
      hPc3 (by omega) hRdx3 hInv3 (by rw [hMem3, hMem2, hMem1, hMem0, hA4]; exact hM4) with ⟨st4, hSt4, hPc4, hRdx4, hMem4, hInv4⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 5 _ l5 _
      hPc4 (by omega) hRdx4 hInv4 (by rw [hMem4, hMem3, hMem2, hMem1, hMem0, hA5]; exact hM5) with ⟨st5, hSt5, hPc5, hRdx5, hMem5, hInv5⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 6 _ l6 _
      hPc5 (by omega) hRdx5 hInv5 (by rw [hMem5, hMem4, hMem3, hMem2, hMem1, hMem0, hA6]; exact hM6) with ⟨st6, hSt6, hPc6, hRdx6, hMem6, hInv6⟩
    rcases minNoNaN64AVX512_horiz_step base spF x n (8 * (n / 8)) 7 _ l7 _
      hPc6 (by omega) hRdx6 hInv6 (by rw [hMem6, hMem5, hMem4, hMem3, hMem2, hMem1, hMem0, hA7]; exact hM7) with ⟨st7, hSt7, hPc7, hRdx7, _, hInv7⟩
    have hExH := minNoNaN64AVX512_horiz_exit base spF x n (8 * (n / 8)) _ _ hPc7 hRdx7 hInv7
    have hAccCanon : canonZero64
        (minStepNoNaN (minStepNoNaN (minStepNoNaN (minStepNoNaN
          (minStepNoNaN (minStepNoNaN (minStepNoNaN (minStepNoNaN f64PosInf l0) l1) l2) l3) l4) l5) l6) l7) =
        specMinNoNaN64 x (8 * (n / 8)) := by
      have ha0 : f64NotNaN (minStepNoNaN f64PosInf l0) = true := minStepNoNaN_notNaN l0 f64PosInf_notNaN
      have ha1 := minStepNoNaN_notNaN l1 ha0
      have ha2 := minStepNoNaN_notNaN l2 ha1
      have ha3 := minStepNoNaN_notNaN l3 ha2
      have ha4 := minStepNoNaN_notNaN l4 ha3
      have ha5 := minStepNoNaN_notNaN l5 ha4
      have ha6 := minStepNoNaN_notNaN l6 ha5
      rw [cmin64_minStep_fold l7 ha6, cmin64_minStep_fold l6 ha5,
          cmin64_minStep_fold l5 ha4, cmin64_minStep_fold l4 ha3,
          cmin64_minStep_fold l3 ha2, cmin64_minStep_fold l2 ha1,
          cmin64_minStep_fold l1 ha0, cmin64_minStep_fold l0 f64PosInf_notNaN,
          cmin64_canonZero_left, cmin64_posInf_left, cmin64_canonZero_left, cmin64_sanitize_left]
      rw [← hLanesEq]
      ac_rfl
    have hTail := minNoNaN64AVX512_tail_iter base spF x n hN (n % 8) (8 * (n / 8)) _ _
      (by omega) hExH.1 hExH.2 hAccCanon
    rcases hTail with ⟨stT, hStT, hPcT, acc', hInvT, hCanonT⟩
    have hRet := minNoNaN64AVX512_canon_and_ret base spF x n acc' _ hPcT hN hInvT
    rcases hRet with ⟨stR, hStR, hStatR, hDirtR, hHazR, hZ0R⟩
    refine ⟨21 + st0 + st1 + st2 + st3 + st4 + st5 + st6 + st7 + 2 + stT + stR, by omega, ?_⟩
    dsimp only
    rw [runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4 + st5 + st6 + st7 + 2 + stT) stR,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4 + st5 + st6 + st7 + 2) stT,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4 + st5 + st6 + st7) 2,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4 + st5 + st6) st7,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4 + st5) st6,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3 + st4) st5,
        runAmd64Vec_add (21 + st0 + st1 + st2 + st3) st4,
        runAmd64Vec_add (21 + st0 + st1 + st2) st3,
        runAmd64Vec_add (21 + st0 + st1) st2,
        runAmd64Vec_add (21 + st0) st1,
        runAmd64Vec_add 21 st0]
    exact ⟨hStatR, hDirtR, hHazR, hZ0R.trans hCanonT⟩
  have hExist : ∃ stTotal ≤ minNoNaN64AVX512FuelBound n,
      let se := runAmd64Vec stTotal minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      (se.zmm 0#5).l0 = specMinNoNaN64 x n := by
    unfold minNoNaN64AVX512FuelBound
    by_cases hRem : 16 * (n / 16) + 8 ≤ n
    · have h8Eq : 16 * (n / 16) + 8 = 8 * (n / 8) := by omega
      have hL8 := minNoNaN64AVX512_loop8_step base spF x n (16 * (n / 16)) _ hEx16.1 hEx16.2 hRem hN
      rcases finishFrom180 _ hL8.1 (by simpa [h8Eq] using hL8.2) with ⟨stRest, hStRest, hFin⟩
      refine ⟨17 + 15 * (n / 16) + 4 + 9 + stRest, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (17 + 15 * (n / 16) + 4 + 9) stRest,
          runAmd64Vec_add (17 + 15 * (n / 16) + 4) 9,
          runAmd64Vec_add (17 + 15 * (n / 16)) 4,
          runAmd64Vec_add 17 (15 * (n / 16))]
      exact hFin
    · have h8Eq : 16 * (n / 16) = 8 * (n / 8) := by omega
      rcases finishFrom180 _ hEx16.1 (by simpa [h8Eq] using hEx16.2) with ⟨stRest, hStRest, hFin⟩
      refine ⟨17 + 15 * (n / 16) + 4 + stRest, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (17 + 15 * (n / 16) + 4) stRest,
          runAmd64Vec_add (17 + 15 * (n / 16)) 4,
          runAmd64Vec_add 17 (15 * (n / 16))]
      exact hFin
  rcases hExist with ⟨stTotal, hStLe, hExact⟩
  have hSplit : fuel = stTotal + (fuel - stTotal) := by omega
  have hAbi := minNoNaN64AVX512_abi_reg_preserved s0 fuel hPc hRun
  have hCore :
      let sf := runAmd64Vec fuel minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      (sf.zmm 0#5).l0 = specMinNoNaN64 x n := by
    dsimp only
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hExact.1])]
    exact hExact
  have hSpRbp := hAbi.2.2.2 hCore.1
  exact ⟨hCore.1, hCore.2.1, hCore.2.2.1, hCore.2.2.2, hSpRbp.1, hSpRbp.2, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

/-- **Cross-ISA Bit-Exact Equivalence (`minNoNaN64AVX2` = `minNoNaN64AVX512` = `specMinNoNaN64`)**:
For EVERY non-empty slice length `n` (`0 < n`, `n + 32 < 2^63`, arbitrary `n = 4q + r` and
`n = 8q' + r'`) and EVERY IEEE-754 binary64 input array (including `-0.0`/`+0.0` at arbitrary
positions across the `VMINPD` operand-order flip, subnormals, `±Inf`, and `NaN`s), the decoded
AVX2 VEX machine bytes and the decoded AVX-512 EVEX machine bytes return bit-identical `XMM0.l0`. -/
theorem minNoNaN64_avx2_eq_avx512 (base : BitVec 64) (x : Nat → BitVec 64)
    (n fuel2 fuel512 : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hGuard : BitVec.ult (s0.mem (s0.gpr 14#4 + 16#64)) (s0.gpr 4#4 - 8#64) = true)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 63)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hDisj2 : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 8, elemAddr base j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8))
    (hDisj512 : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 16, elemAddr base j ≠ s0.gpr 4#4 - 136#64 + BitVec.ofNat 64 (k * 8))
    (hFuel2 : minNoNaN64AVX2FuelBound n ≤ fuel2)
    (hFuel512 : minNoNaN64AVX512FuelBound n ≤ fuel512) :
    ((runAmd64Vec fuel2 minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0).zmm 0#5).l0 =
    ((runAmd64Vec fuel512 minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0).zmm 0#5).l0 := by
  have h2 := minNoNaN64AVX2_functional_equiv base x n fuel2 s0
    hPc hRun hHaz hPos (by omega) hRax hRbx hX hDisj2 hFuel2
  have h512 := minNoNaN64AVX512_functional_equiv base x n fuel512 s0
    hPc hRun hHaz hGuard hPos hN hRax hRbx hX hDisj512 hFuel512
  rw [h2.2.2.2.1, h512.2.2.2.1]

/-!
### Part VI.C: Full Decoded Functional Equivalence for `initReducedRow64AVX2` and `initReducedRow64AVX512`
-/

/-- Intel SDM x86-64 IEEE-754 binary64 reduced cost `(costRow[j] - v[j]) - ui` matching hardware `VSUBPD`. -/
def specReducedElem (c v : Nat → BitVec 64) (ui : BitVec 64) (j : Nat) : BitVec 64 :=
  x86F64Sub (x86F64Sub (c j) (v j)) ui

/-- Sequential NaN-free check over the reduced-cost prefix `0 .. m - 1`. -/
def specAllReducedNotNaN (c v : Nat → BitVec 64) (ui : BitVec 64) : Nat → Bool
  | 0 => true
  | k + 1 => specAllReducedNotNaN c v ui k && f64NotNaN (specReducedElem c v ui k)

/-- Sequential canonicalized minimum over the reduced-cost prefix `0 .. m - 1`. -/
def specInitReducedMin (c v : Nat → BitVec 64) (ui : BitVec 64) : Nat → BitVec 64
  | 0 => f64PosInf
  | k + 1 => cmin64 (specInitReducedMin c v ui k) (specReducedElem c v ui k)

private theorem bv_add_left_cancel_64 {b x y : BitVec 64} (h : b + x = b + y) : x = y := by
  bv_omega

private theorem elemAddr_inj {base : BitVec 64} {j1 j2 : Nat}
    (h1 : j1 < 2 ^ 61) (h2 : j2 < 2 ^ 61) (hNe : j1 ≠ j2) :
    elemAddr base j1 ≠ elemAddr base j2 := by
  intro hEq
  have hMul := bv_add_left_cancel_64 hEq
  have hNat := congrArg BitVec.toNat hMul
  simp only [BitVec.toNat_mul, BitVec.toNat_ofNat] at hNat
  have h1' : j1 % 2 ^ 64 = j1 := Nat.mod_eq_of_lt (by omega)
  have h2' : j2 % 2 ^ 64 = j2 := Nat.mod_eq_of_lt (by omega)
  have h8 : 8 % 2 ^ 64 = 8 := by decide
  rw [h1', h2', h8, Nat.mod_eq_of_lt (by omega), Nat.mod_eq_of_lt (by omega)] at hNat
  omega

private theorem minsd_por_comm_eq_f64Min (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    (vminpdLane a b ||| vminpdLane (vminpdLane a b) a) = f64Min a b := by
  rw [BitVec.or_comm]
  exact minsd_por_eq_f64Min a b ha hb

private theorem f64Min_notNaN {a b : BitVec 64}
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64NotNaN (f64Min a b) = true := by
  have haN : f64IsNaN a = false := by simpa [f64NotNaN] using ha
  have hbN : f64IsNaN b = false := by simpa [f64NotNaN] using hb
  unfold f64Min
  simp only [haN, hbN, Bool.or_self, Bool.false_eq_true, ↓reduceIte]
  split
  · split <;> assumption
  · split <;> assumption

private theorem cmin64_f64Min_tree4 {l0 l1 l2 l3 : BitVec 64}
    (h0 : f64NotNaN l0 = true) (h1 : f64NotNaN l1 = true)
    (h2 : f64NotNaN l2 = true) (h3 : f64NotNaN l3 = true) :
    f64NotNaN (f64Min (f64Min l0 l1) (f64Min l2 l3)) = true ∧
    canonZero64 (f64Min (f64Min l0 l1) (f64Min l2 l3)) =
      cmin64 (cmin64 l0 l1) (cmin64 l2 l3) := by
  have h01 := f64Min_notNaN h0 h1
  have h23 := f64Min_notNaN h2 h3
  refine ⟨f64Min_notNaN h01 h23, ?_⟩
  rw [cmin64_f64Min h01 h23, ← cmin64_canonZero_left, ← cmin64_canonZero_right,
      cmin64_f64Min h0 h1, cmin64_f64Min h2 h3]

private theorem cmin64_f64Min_tree8 {l0 l1 l2 l3 l4 l5 l6 l7 : BitVec 64}
    (h0 : f64NotNaN l0 = true) (h1 : f64NotNaN l1 = true)
    (h2 : f64NotNaN l2 = true) (h3 : f64NotNaN l3 = true)
    (h4 : f64NotNaN l4 = true) (h5 : f64NotNaN l5 = true)
    (h6 : f64NotNaN l6 = true) (h7 : f64NotNaN l7 = true) :
    let t03 := f64Min (f64Min l0 l1) (f64Min l2 l3)
    let t47 := f64Min (f64Min l4 l5) (f64Min l6 l7)
    f64NotNaN (f64Min t03 t47) = true ∧
    canonZero64 (f64Min t03 t47) =
      cmin64 (cmin64 (cmin64 l0 l1) (cmin64 l2 l3))
             (cmin64 (cmin64 l4 l5) (cmin64 l6 l7)) := by
  have h03 := cmin64_f64Min_tree4 h0 h1 h2 h3
  have h47 := cmin64_f64Min_tree4 h4 h5 h6 h7
  refine ⟨f64Min_notNaN h03.1 h47.1, ?_⟩
  rw [cmin64_f64Min h03.1 h47.1, ← cmin64_canonZero_left, ← cmin64_canonZero_right,
      h03.2, h47.2]

private theorem storeVecMem_ymm256_at (m : BitVec 64 → BitVec 64) (base : BitVec 64)
    (v : Vec512) (kVal : BitVec 8) :
    let m' := storeVecMem m .ymm256 base v 0#3 kVal
    m' base = v.l0 ∧
    m' (base + 8#64) = v.l1 ∧
    m' (base + 16#64) = v.l2 ∧
    m' (base + 24#64) = v.l3 ∧
    ∀ addr, addr ≠ base → addr ≠ base + 8#64 → addr ≠ base + 16#64 → addr ≠ base + 24#64 →
      m' addr = m addr := by
  have hS := fun k1 k2 (hne : k1 ≠ k2) (h1 : k1 < 4) (h2 : k2 < 4) =>
    stackSlot_ne_offset base k1 k2 hne (by omega) (by omega)
  have h0 : base + 0#64 = base := BitVec.add_zero base
  have hEq : storeVecMem m .ymm256 base v 0#3 kVal =
      write64 (write64 (write64 (write64 m base v.l0) (base + 8#64) v.l1)
        (base + 16#64) v.l2) (base + 24#64) v.l3 := by
    simp [storeVecMem, h0]
  have h01 : base ≠ base + 8#64 := by bv_omega
  have h02 : base ≠ base + 16#64 := by bv_omega
  have h03 : base ≠ base + 24#64 := by bv_omega
  have h12 : base + 8#64 ≠ base + 16#64 := hS 1 2 (by decide) (by omega) (by omega)
  have h13 : base + 8#64 ≠ base + 24#64 := hS 1 3 (by decide) (by omega) (by omega)
  have h23 : base + 16#64 ≠ base + 24#64 := hS 2 3 (by decide) (by omega) (by omega)
  dsimp only
  rw [hEq]
  refine ⟨by rw [write64_ne _ _ h03, write64_ne _ _ h02, write64_ne _ _ h01, write64_eq],
    by rw [write64_ne _ _ h13, write64_ne _ _ h12, write64_eq],
    by rw [write64_ne _ _ h23, write64_eq],
    by rw [write64_eq],
    by intro addr ha0 ha1 ha2 ha3
       rw [write64_ne _ _ ha3, write64_ne _ _ ha2, write64_ne _ _ ha1, write64_ne _ _ ha0]⟩

private theorem storeVecMem_zmm512_at (m : BitVec 64 → BitVec 64) (base : BitVec 64)
    (v : Vec512) (kVal : BitVec 8) :
    let m' := storeVecMem m .zmm512 base v 0#3 kVal
    m' base = v.l0 ∧
    m' (base + 8#64) = v.l1 ∧
    m' (base + 16#64) = v.l2 ∧
    m' (base + 24#64) = v.l3 ∧
    m' (base + 32#64) = v.l4 ∧
    m' (base + 40#64) = v.l5 ∧
    m' (base + 48#64) = v.l6 ∧
    m' (base + 56#64) = v.l7 ∧
    ∀ addr, (∀ k < 8, addr ≠ base + BitVec.ofNat 64 (k * 8)) → m' addr = m addr := by
  have hS := fun k1 k2 (hne : k1 ≠ k2) (h1 : k1 < 8) (h2 : k2 < 8) =>
    stackSlot_ne_offset base k1 k2 hne (by omega) (by omega)
  have h0 : base + 0#64 = base := BitVec.add_zero base
  have hEq : storeVecMem m .zmm512 base v 0#3 kVal =
      write64 (write64 (write64 (write64 (write64 (write64 (write64 (write64 m
        (base + 0#64) v.l0) (base + 8#64) v.l1) (base + 16#64) v.l2) (base + 24#64) v.l3)
        (base + 32#64) v.l4) (base + 40#64) v.l5) (base + 48#64) v.l6) (base + 56#64) v.l7 := by
    simp [storeVecMem]
  dsimp only
  refine ⟨?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_, ?_⟩
  · suffices storeVecMem m .zmm512 base v 0#3 kVal (base + 0#64) = v.l0 by simpa [h0] using this
    rw [hEq, write64_ne _ _ (hS 0 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 2 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 0 1 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 1 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 1 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 1 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 1 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 1 3 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 1 2 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 2 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 2 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 2 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 2 4 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 2 3 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 3 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 3 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 3 5 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 3 4 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 4 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 4 6 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 4 5 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 5 7 (by decide) (by omega) (by omega)),
        write64_ne _ _ (hS 5 6 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_ne _ _ (hS 6 7 (by decide) (by omega) (by omega)), write64_eq]
  · rw [hEq, write64_eq]
  · intro addr hNe
    rw [hEq,
        write64_ne _ _ (hNe 7 (by omega)), write64_ne _ _ (hNe 6 (by omega)),
        write64_ne _ _ (hNe 5 (by omega)), write64_ne _ _ (hNe 4 (by omega)),
        write64_ne _ _ (hNe 3 (by omega)), write64_ne _ _ (hNe 2 (by omega)),
        write64_ne _ _ (hNe 1 (by omega)), write64_ne _ _ (hNe 0 (by omega))]

/-- Destination slice state invariant: `[0 .. i)` holds bit-exact reduced costs, `[i .. n)` holds `dst0`. -/
private def DstMemState (dstBase : BitVec 64) (c v dst0 : Nat → BitVec 64) (ui : BitVec 64)
    (n i : Nat) (mem : BitVec 64 → BitVec 64) : Prop :=
  (∀ j < i, mem (elemAddr dstBase j) = specReducedElem c v ui j) ∧
  (∀ j, i ≤ j → j < n → mem (elemAddr dstBase j) = dst0 j)

private theorem dstMemState_store4 {dstBase : BitVec 64} {c v dst0 : Nat → BitVec 64}
    {ui : BitVec 64} {n i : Nat} {m : BitVec 64 → BitVec 64} {vec : Vec512} {kVal : BitVec 8}
    (hDst : DstMemState dstBase c v dst0 ui n i m) (hStep : i + 4 ≤ n) (hN : n < 2 ^ 61)
    (hl0 : vec.l0 = specReducedElem c v ui i)
    (hl1 : vec.l1 = specReducedElem c v ui (i + 1))
    (hl2 : vec.l2 = specReducedElem c v ui (i + 2))
    (hl3 : vec.l3 = specReducedElem c v ui (i + 3)) :
    DstMemState dstBase c v dst0 ui n (i + 4) (storeVecMem m .ymm256 (elemAddr dstBase i) vec 0#3 kVal) := by
  have hAt := storeVecMem_ymm256_at m (elemAddr dstBase i) vec kVal
  rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt
  constructor
  · intro j hj
    by_cases h0 : j = i
    · subst h0; exact hAt.1.trans hl0
    · by_cases h1 : j = i + 1
      · subst h1; exact hAt.2.1.trans hl1
      · by_cases h2 : j = i + 2
        · subst h2; exact hAt.2.2.1.trans hl2
        · by_cases h3 : j = i + 3
          · subst h3; exact hAt.2.2.2.1.trans hl3
          · rw [hAt.2.2.2.2 _ (elemAddr_inj (by omega) (by omega) h0)
                (elemAddr_inj (by omega) (by omega) h1)
                (elemAddr_inj (by omega) (by omega) h2)
                (elemAddr_inj (by omega) (by omega) h3)]
            exact hDst.1 j (by omega)
  · intro j hge hj
    rw [hAt.2.2.2.2 _ (elemAddr_inj (by omega) (by omega) (by omega))
        (elemAddr_inj (by omega) (by omega) (by omega))
        (elemAddr_inj (by omega) (by omega) (by omega))
        (elemAddr_inj (by omega) (by omega) (by omega))]
    exact hDst.2 j (by omega) hj

private theorem dstMemState_store8 {dstBase : BitVec 64} {c v dst0 : Nat → BitVec 64}
    {ui : BitVec 64} {n i : Nat} {m : BitVec 64 → BitVec 64} {vec : Vec512} {kVal : BitVec 8}
    (hDst : DstMemState dstBase c v dst0 ui n i m) (hStep : i + 8 ≤ n) (hN : n < 2 ^ 61)
    (hl0 : vec.l0 = specReducedElem c v ui i)
    (hl1 : vec.l1 = specReducedElem c v ui (i + 1))
    (hl2 : vec.l2 = specReducedElem c v ui (i + 2))
    (hl3 : vec.l3 = specReducedElem c v ui (i + 3))
    (hl4 : vec.l4 = specReducedElem c v ui (i + 4))
    (hl5 : vec.l5 = specReducedElem c v ui (i + 5))
    (hl6 : vec.l6 = specReducedElem c v ui (i + 6))
    (hl7 : vec.l7 = specReducedElem c v ui (i + 7)) :
    DstMemState dstBase c v dst0 ui n (i + 8) (storeVecMem m .zmm512 (elemAddr dstBase i) vec 0#3 kVal) := by
  have hAt := storeVecMem_zmm512_at m (elemAddr dstBase i) vec kVal
  rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      elemAddr_plus_32, elemAddr_plus_40, elemAddr_plus_48, elemAddr_plus_56] at hAt
  have hOther : ∀ j < n, (j < i ∨ i + 8 ≤ j) →
      storeVecMem m .zmm512 (elemAddr dstBase i) vec 0#3 kVal (elemAddr dstBase j) = m (elemAddr dstBase j) := by
    intro j hj hOut
    apply hAt.2.2.2.2.2.2.2.2
    intro k hk
    rw [elemAddr_add]
    exact elemAddr_inj (by omega) (by omega) (by omega)
  constructor
  · intro j hj
    by_cases h0 : j = i
    · subst h0; exact hAt.1.trans hl0
    · by_cases h1 : j = i + 1
      · subst h1; exact hAt.2.1.trans hl1
      · by_cases h2 : j = i + 2
        · subst h2; exact hAt.2.2.1.trans hl2
        · by_cases h3 : j = i + 3
          · subst h3; exact hAt.2.2.2.1.trans hl3
          · by_cases h4 : j = i + 4
            · subst h4; exact hAt.2.2.2.2.1.trans hl4
            · by_cases h5 : j = i + 5
              · subst h5; exact hAt.2.2.2.2.2.1.trans hl5
              · by_cases h6 : j = i + 6
                · subst h6; exact hAt.2.2.2.2.2.2.1.trans hl6
                · by_cases h7 : j = i + 7
                  · subst h7; exact hAt.2.2.2.2.2.2.2.1.trans hl7
                  · rw [hOther j (by omega) (Or.inl (by omega))]; exact hDst.1 j (by omega)
  · intro j hge hj
    rw [hOther j hj (Or.inr hge)]
    exact hDst.2 j (by omega) hj

private theorem initReducedRow64AVX2_chunks_wf :
    chunks16WellFormed initReducedRow64AVX2ByteChunks = true := by decide

private theorem initReducedRow64AVX512_chunks_wf :
    chunks16WellFormed initReducedRow64AVX512ByteChunks = true := by decide

private theorem specAllReducedNotNaN_add4 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specAllReducedNotNaN c v ui (i + 4) =
      (specAllReducedNotNaN c v ui i &&
       f64NotNaN (specReducedElem c v ui i) && f64NotNaN (specReducedElem c v ui (i + 1)) &&
       f64NotNaN (specReducedElem c v ui (i + 2)) && f64NotNaN (specReducedElem c v ui (i + 3))) := by
  simp only [specAllReducedNotNaN]

private theorem specAllReducedNotNaN_add8 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specAllReducedNotNaN c v ui (i + 8) =
      (specAllReducedNotNaN c v ui (i + 4) &&
       f64NotNaN (specReducedElem c v ui (i + 4)) && f64NotNaN (specReducedElem c v ui (i + 5)) &&
       f64NotNaN (specReducedElem c v ui (i + 6)) && f64NotNaN (specReducedElem c v ui (i + 7))) := by
  simp only [specAllReducedNotNaN]

private theorem specInitReducedMin_add4 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specInitReducedMin c v ui (i + 4) =
      cmin64 (cmin64 (cmin64 (cmin64 (specInitReducedMin c v ui i)
        (specReducedElem c v ui i)) (specReducedElem c v ui (i + 1)))
        (specReducedElem c v ui (i + 2))) (specReducedElem c v ui (i + 3)) := by
  simp only [specInitReducedMin]

private theorem specInitReducedMin_add8 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specInitReducedMin c v ui (i + 8) =
      cmin64 (cmin64 (cmin64 (cmin64 (specInitReducedMin c v ui (i + 4))
        (specReducedElem c v ui (i + 4))) (specReducedElem c v ui (i + 5)))
        (specReducedElem c v ui (i + 6))) (specReducedElem c v ui (i + 7)) := by
  simp only [specInitReducedMin]

private theorem vcmppd0_self_ymm256 (x0 x1 x2 x3 : BitVec 64) :
    (Vec512.zipWith (fun a b => if x86VcmppdPred 0 a b then 0xffffffffffffffff#64 else 0#64)
      { l0 := x0, l1 := x1, l2 := x2, l3 := x3 }
      { l0 := x0, l1 := x1, l2 := x2, l3 := x3 }).zeroHi256 =
      maskVec4 (f64NotNaN x0) (f64NotNaN x1) (f64NotNaN x2) (f64NotNaN x3) := by
  simp [Vec512.zipWith, Vec512.zeroHi256, x86VcmppdPred, f64Eq_self_eq_notNaN, f64NotNaN, maskVec4, maskLane64]

private theorem vpand_maskVec4 (m0 m1 m2 m3 p0 p1 p2 p3 : Bool) :
    (Vec512.zipWith (· &&& ·) (maskVec4 m0 m1 m2 m3) (maskVec4 p0 p1 p2 p3)).zeroHi256 =
      maskVec4 (m0 && p0) (m1 && p1) (m2 && p2) (m3 && p3) := by
  simp [Vec512.zipWith, Vec512.zeroHi256, maskVec4, maskLane64_and]

private theorem vpand_maskVec8 (m0 m1 m2 m3 m4 m5 m6 m7 p0 p1 p2 p3 p4 p5 p6 p7 : Bool) :
    Vec512.zipWith (· &&& ·) (maskVec8 m0 m1 m2 m3 m4 m5 m6 m7) (maskVec8 p0 p1 p2 p3 p4 p5 p6 p7) =
      maskVec8 (m0 && p0) (m1 && p1) (m2 && p2) (m3 && p3)
               (m4 && p4) (m5 && p5) (m6 && p6) (m7 && p7) := by
  simp [Vec512.zipWith, maskVec8, maskLane64_and]

/-- Loop invariant at `pc = 200` (8-wide header) and `pc = 257` (4-wide header) of `initReducedRow64AVX2`. -/
private structure InitReduced2Inv (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = dstBase
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  rspEq : s.gpr 4#4 = spF
  rdiEq : s.gpr 7#4 = costBase
  r9Eq : s.gpr 9#4 = vBase
  zmm0Eq : s.zmm 0#5 = (Vec512.broadcast ui).zeroHi256
  memCost : ∀ j < n, s.mem (elemAddr costBase j) = c j
  memV : ∀ j < n, s.mem (elemAddr vBase j) = v j
  memDst : DstMemState dstBase c v dst0 ui n i s.mem
  savedRbp : s.mem (spF + 32#64) = rbp0
  ordMasks : ∃ m0 m1 m2 m3 p0 p1 p2 p3 : Bool,
    s.zmm 2#5 = maskVec4 m0 m1 m2 m3 ∧
    s.zmm 3#5 = maskVec4 p0 p1 p2 p3 ∧
    ((m0 && p0) && (m1 && p1) && (m2 && p2) && (m3 && p3)) = specAllReducedNotNaN c v ui i
  minAccs : specAllReducedNotNaN c v ui i = true →
    ∃ a0 a1 a2 a3 b0 b1 b2 b3 : BitVec 64,
      s.zmm 1#5 = { l0 := a0, l1 := a1, l2 := a2, l3 := a3 } ∧
      s.zmm 4#5 = { l0 := b0, l1 := b1, l2 := b2, l3 := b3 } ∧
      f64NotNaN a0 = true ∧ f64NotNaN a1 = true ∧ f64NotNaN a2 = true ∧ f64NotNaN a3 = true ∧
      f64NotNaN b0 = true ∧ f64NotNaN b1 = true ∧ f64NotNaN b2 = true ∧ f64NotNaN b3 = true ∧
      cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3)) (cmin64 (cmin64 b0 b1) (cmin64 b2 b3)) =
        specInitReducedMin c v ui i

private def init2BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 1),
    (.movRR64 5#4 4#4, 4),
    (.subImm64 4#4 0x20#64, 8),
    (.movMR64 (.baseIdxDisp (some 4#4) none 48) 0#4, 13),
    (.movMR64 (.baseIdxDisp (some 4#4) none 72) 7#4, 18),
    (.movMR64 (.baseIdxDisp (some 4#4) none 96) 9#4, 23),
    (.nop, 32) ]

private def init2BlkPro1a : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 3#4 3#4, 35),
    (.jccRel .be 367, 41) ]

private def init2BlkPro1b : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 6#4 6#4, 44),
    (.jccRel .be 353, 50) ]

private def init2BlkPro1c : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 10#4 10#4, 53),
    (.jccRel .be 339, 59) ]

private def init2BlkPro2 : List (Amd64VecInsn × Nat) :=
  [ (.vmovsdRR 0#5 15#5 0#5, 63),
    (.vmovsdLoad 1#5 (.ripRel 281113), 71),
    (.vbroadcastsd false .ymm256 1#5 (.reg 1#5) {}, 76),
    (.nop, 77), (.nop, 78), (.nop, 79),
    (.vbroadcastsd false .ymm256 0#5 (.reg 0#5) {}, 84),
    (.vcmppdVex .ymm256 2#5 1#5 (.reg 1#5) 0, 89),
    (.xorRR32 1#4 1#4, 91),
    (.vmovdquRR false .ymm256 3#5 2#5 {}, 95),
    (.vmovdquRR false .ymm256 4#5 1#5 {}, 99),
    (.jmpRel 99, 101) ]

private def init2BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 204),
    (.cmpRR64 3#4 2#4, 207),
    (.jccRel .ge (-108), 209) ]

private def init2BlkLoop8Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 46, 211) ]

private def init2BlkLoop8BodyA : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 105),
    (.lea64 8#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 109),
    (.lea64 10#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 113),
    (.vmovdquLoad false .ymm256 5#5 (.baseIdxDisp (some 6#4) none 0) {}, 117),
    (.vsubpd false .ymm256 5#5 5#5 (.mem (.baseIdxDisp (some 8#4) none 0)) {}, 122),
    (.vsubpd false .ymm256 5#5 5#5 (.reg 0#5) {}, 126),
    (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 130),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 32), 134),
    (.vmovdquLoad false .ymm256 6#5 (.baseIdxDisp (some 6#4) none 0) {}, 138),
    (.lea64 6#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 142),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 32), 146),
    (.vsubpd false .ymm256 6#5 6#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 150) ]

private def init2BlkLoop8BodyB1 : List (Amd64VecInsn × Nat) :=
  [ (.vmovdquStore false .ymm256 (.baseIdxDisp (some 10#4) none 0) 5#5 {}, 155) ]

private def init2BlkLoop8BodyB2 : List (Amd64VecInsn × Nat) :=
  [ (.vsubpd false .ymm256 6#5 6#5 (.reg 0#5) {}, 159),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 163),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 32), 167),
    (.vmovdquStore false .ymm256 (.baseIdxDisp (some 6#4) none 0) 6#5 {}, 171) ]

private def init2BlkLoop8BodyC : List (Amd64VecInsn × Nat) :=
  [ (.vcmppdVex .ymm256 7#5 5#5 (.reg 5#5) 0, 176),
    (.vcmppdVex .ymm256 8#5 6#5 (.reg 6#5) 0, 181),
    (.vpand false .ymm256 2#5 7#5 (.reg 2#5) {}, 185),
    (.vpand false .ymm256 3#5 8#5 (.reg 3#5) {}, 189),
    (.vminpd false .ymm256 1#5 5#5 (.reg 1#5) {}, 193),
    (.vminpd false .ymm256 4#5 6#5 (.reg 4#5) {}, 197),
    (.movRR64 1#4 2#4, 200) ]

private def init2BlkLoop4Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 4), 261),
    (.cmpRR64 3#4 2#4, 264),
    (.jccRel .ge (-55), 266) ]

private def init2BlkLoop4BodyA : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 215),
    (.lea64 8#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 219),
    (.lea64 10#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 223),
    (.vmovdquLoad false .ymm256 5#5 (.baseIdxDisp (some 6#4) none 0) {}, 227),
    (.vsubpd false .ymm256 5#5 5#5 (.mem (.baseIdxDisp (some 8#4) none 0)) {}, 232),
    (.vsubpd false .ymm256 5#5 5#5 (.reg 0#5) {}, 236) ]

private def init2BlkLoop4BodyB : List (Amd64VecInsn × Nat) :=
  [ (.vmovdquStore false .ymm256 (.baseIdxDisp (some 10#4) none 0) 5#5 {}, 241) ]

private def init2BlkLoop4BodyC : List (Amd64VecInsn × Nat) :=
  [ (.vcmppdVex .ymm256 6#5 5#5 (.reg 5#5) 0, 246),
    (.vpand false .ymm256 2#5 2#5 (.reg 6#5) {}, 250),
    (.vminpd false .ymm256 1#5 1#5 (.reg 5#5) {}, 254),
    (.movRR64 1#4 2#4, 257) ]

private def init2BlkEpiCmp : List (Amd64VecInsn × Nat) :=
  [ (.vminpd false .ymm256 0#5 1#5 (.reg 4#5) {}, 270),
    (.vmovdquStore false .ymm256 (.baseIdxDisp (some 4#4) none 0) 0#5 {}, 275),
    (.vpand false .ymm256 0#5 2#5 (.reg 3#5) {}, 279),
    (.vmovmskpd .ymm256 0#4 0#5, 283),
    (.cmpImm8 0#4 0x0f#8, 285),
    (.jccRel .e 25, 287) ]

private def init2BlkRetNaN : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 290),
    (.movImm64 0#4 0x7ff8000000000001#64, 300),
    (.sseMovqFromGpr 0#4 0#4, 305),
    (.nop, 306),
    (.addImm64 4#4 0x20#64, 310),
    (.popReg 5#4, 311),
    (.ret, 312) ]

private def init2BlkHoriz01 : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 315),
    (.sseMovsdLoad 0#4 (.baseIdxDisp (some 4#4) none 0), 320),
    (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 8), 326),
    (.sseMovupsRR 2#4 0#4, 329),
    (.sseMinsdRR 2#4 1#4, 333),
    (.sseMovupsRR 1#4 2#4, 336),
    (.sseMinsdRR 2#4 0#4, 340),
    (.ssePorRR 2#4 1#4, 344) ]

private def init2BlkHoriz23 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 0#4 (.baseIdxDisp (some 4#4) none 16), 350),
    (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 24), 356),
    (.sseMovupsRR 3#4 0#4, 359),
    (.sseMinsdRR 0#4 1#4, 363),
    (.sseMovupsRR 1#4 0#4, 366),
    (.sseMinsdRR 0#4 3#4, 370),
    (.ssePorRR 0#4 1#4, 374) ]

private def init2BlkHorizFin : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 2#4, 377),
    (.sseMinsdRR 2#4 0#4, 381),
    (.sseMovupsRR 0#4 2#4, 384),
    (.sseMinsdRR 0#4 1#4, 388),
    (.ssePorRR 0#4 2#4, 392),
    (.addImm64 4#4 0x20#64, 396),
    (.popReg 5#4, 397),
    (.ret, 398) ]

set_option maxRecDepth 4096 in
private theorem init2BlkPro0_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 0 init2BlkPro0 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkPro1a_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 32 init2BlkPro1a = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkPro1b_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 41 init2BlkPro1b = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkPro1c_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 50 init2BlkPro1c = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkPro2_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 59 init2BlkPro2 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8Hdr_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 200 init2BlkLoop8Hdr = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8Exit_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 209 init2BlkLoop8Exit = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8BodyA_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 101 init2BlkLoop8BodyA = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8BodyB1_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 150 init2BlkLoop8BodyB1 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8BodyB2_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 155 init2BlkLoop8BodyB2 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop8BodyC_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 171 init2BlkLoop8BodyC = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop4Hdr_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 257 init2BlkLoop4Hdr = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop4BodyA_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 211 init2BlkLoop4BodyA = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop4BodyB_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 236 init2BlkLoop4BodyB = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkLoop4BodyC_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 241 init2BlkLoop4BodyC = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkEpiCmp_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 266 init2BlkEpiCmp = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkRetNaN_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 287 init2BlkRetNaN = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkHoriz01_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 312 init2BlkHoriz01 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkHoriz23_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 344 init2BlkHoriz23 = true := by decide
set_option maxRecDepth 4096 in
private theorem init2BlkHorizFin_ok : checkBlockSeq initReducedRow64AVX2ByteChunks initReducedRow64AVX2Rodata 374 init2BlkHorizFin = true := by decide

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_prologue (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenCost lenV : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hPosC : 0 < lenCost) (hPosV : 0 < lenV)
    (hN : n + 16 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV : lenV < 2 ^ 64)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64) :
    let s1 := runAmd64Vec 25 initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
    s1.pc = 200 ∧ InitReduced2Inv dstBase costBase vBase (s0.gpr 4#4 - 40#64) (s0.gpr 5#4) c v dst0 ui n 0 s1 := by
  let sp0 := s0.gpr 4#4
  let spF := sp0 - 40#64
  have hSub40 : sp0 - 8#64 - 0x20#64 = spF := by dsimp only [spF]; bv_omega
  have hSp32 : spF + 32#64 = sp0 - 8#64 := by dsimp only [spF]; bv_omega
  have hSp48 : spF + 48#64 = sp0 + 8#64 := by dsimp only [spF]; bv_omega
  have hSp72 : spF + 72#64 = sp0 + 32#64 := by dsimp only [spF]; bv_omega
  have hSp96 : spF + 96#64 = sp0 + 56#64 := by dsimp only [spF]; bv_omega
  have hRbpNe8 : sp0 - 8#64 ≠ sp0 + 8#64 := by bv_omega
  have hRbpNe32 : sp0 - 8#64 ≠ sp0 + 32#64 := by bv_omega
  have hRbpNe56 : sp0 - 8#64 ≠ sp0 + 56#64 := by bv_omega
  have hBeN := evalX86Cond_be_test64_self hPos (by omega)
  have hBeC := evalX86Cond_be_test64_self hPosC hLenC
  have hBeV := evalX86Cond_be_test64_self hPosV hLenV
  have hRo63 : initReducedRow64AVX2Rodata 63 = some f64PosInf := rfl
  have hJmp : nextTargetPc 101 99 = some 200 := by decide
  have hEqInf : f64Eq f64PosInf f64PosInf = true := by decide
  dsimp only [initReducedRow64AVX2Bytes]
  let sA := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0 init2BlkPro0
  have hS0 : runAmd64Vec 25 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0 =
      runAmd64Vec 18 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA :=
    runAmd64Vec_block_seq 18 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0 0 init2BlkPro0 hRun hPc init2BlkPro0_ok
  rw [hS0]
  have hARun : sA.status = .running := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hRun]
  have hAPc : sA.pc = 32 := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hAHaz : sA.sseHazard = false := by
    simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, Amd64VecInsn.isLegacySSE, hHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRax]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx]
  have hARsp : sA.gpr 4#4 = spF := by
    simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, sp0, hSub40]
  have hARsi : sA.gpr 6#4 = BitVec.ofNat 64 lenCost := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRsi]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hR9]
  have hAR10 : sA.gpr 10#4 = BitVec.ofNat 64 lenV := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hR10]
  have hAZ0 : (sA.zmm 0#5).l0 = ui := by simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hUi]
  have hAMemEq : sA.mem =
      write64 (write64 (write64 (write64 s0.mem (sp0 - 8#64) (s0.gpr 5#4))
        (sp0 + 8#64) dstBase) (sp0 + 32#64) costBase) (sp0 + 56#64) vBase := by
    simp [sA, init2BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hRax, hRdi, hR9, sp0, hSub40, hSp48, hSp72, hSp96]
  let sB1 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA init2BlkPro1a
  have hS1a : runAmd64Vec 18 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA =
      runAmd64Vec 16 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 :=
    runAmd64Vec_block_seq 16 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA 32 init2BlkPro1a hARun hAPc init2BlkPro1a_ok
  rw [hS1a]
  have hB1Run : sB1.status = .running := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN, hARun]
  have hB1Pc : sB1.pc = 41 := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Haz : sB1.sseHazard = false := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hARbx, hBeN, hAHaz]
  have hB1Gpr : sB1.gpr = sA.gpr := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Zmm : sB1.zmm = sA.zmm := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Mem : sB1.mem = sA.mem := by simp [sB1, init2BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  let sB2 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 init2BlkPro1b
  have hS1b : runAmd64Vec 16 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 =
      runAmd64Vec 14 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 :=
    runAmd64Vec_block_seq 14 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 41 init2BlkPro1b hB1Run hB1Pc init2BlkPro1b_ok
  rw [hS1b]
  have hB2Run : sB2.status = .running := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Run]
  have hB2Pc : sB2.pc = 50 := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC]
  have hB2Haz : sB2.sseHazard = false := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hB1Gpr, hARsi, hBeC, hB1Haz]
  have hB2Gpr : sB2.gpr = sA.gpr := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC]
  have hB2Zmm : sB2.zmm = sA.zmm := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Zmm]
  have hB2Mem : sB2.mem = sA.mem := by simp [sB2, init2BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Mem]
  let sB := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 init2BlkPro1c
  have hS1c : runAmd64Vec 14 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 =
      runAmd64Vec 12 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB :=
    runAmd64Vec_block_seq 12 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 50 init2BlkPro1c hB2Run hB2Pc init2BlkPro1c_ok
  rw [hS1c]
  have hBRun : sB.status = .running := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Run]
  have hBPc : sB.pc = 59 := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV]
  have hBHaz : sB.sseHazard = false := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hB2Gpr, hAR10, hBeV, hB2Haz]
  have hBGpr : sB.gpr = sA.gpr := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV]
  have hBZmm : sB.zmm = sA.zmm := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Zmm]
  have hBMem : sB.mem = sA.mem := by simp [sB, init2BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Mem]
  have hS2 : runAmd64Vec 12 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB =
      runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
        (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB init2BlkPro2) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB 59 init2BlkPro2 hBRun hBPc init2BlkPro2_ok
  rw [hS2]
  simp only [runAmd64Vec]
  have hMemFinal :
      (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB init2BlkPro2).mem = sA.mem := by
    simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp, hBMem]
  have hPres : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      sA.mem (elemAddr base j) = s0.mem (elemAddr base j) := by
    intro j hj base hIn
    rcases hDisjPro j hj base hIn with ⟨hd0, hd1, hd2, hd3⟩
    rw [hAMemEq, write64_ne _ _ hd3, write64_ne _ _ hd2, write64_ne _ _ hd1, write64_ne _ _ hd0]
  refine ⟨by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp],
    ⟨by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp, hBRun],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, Amd64VecInsn.dirtiesUpper, hRo63, hJmp],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, Amd64VecInsn.isLegacySSE, hRo63, hJmp, hBHaz],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARax],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARbx],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARsp, spF, sp0],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARdi],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hAR9],
     by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
          Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, hRo63, hJmp, hBZmm, hAZ0],
     by intro j hj; rw [hMemFinal, hPres j hj costBase (by simp)]; exact hMemC j hj,
     by intro j hj; rw [hMemFinal, hPres j hj vBase (by simp)]; exact hMemV j hj,
     ⟨fun _ h => by omega, fun j _ hj => by rw [hMemFinal, hPres j hj dstBase (by simp)]; exact hMemD j hj⟩,
     by rw [hMemFinal, hSp32, hAMemEq, write64_ne _ _ hRbpNe56, write64_ne _ _ hRbpNe32,
          write64_ne _ _ hRbpNe8, write64_eq],
     ⟨true, true, true, true, true, true, true, true,
      by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, Vec512.zipWith,
           x86VcmppdPred, hEqInf, hRo63, hJmp, maskVec4, maskLane64],
      by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, Vec512.zipWith,
           x86VcmppdPred, hEqInf, hRo63, hJmp, maskVec4, maskLane64],
      rfl⟩,
     fun _ => ⟨f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
      by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, hRo63, hJmp],
      by simp [init2BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, hRo63, hJmp],
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      by simp [specInitReducedMin, cmin64_posInf_posInf]⟩⟩⟩

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_loop8_step (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 200) (hInv : InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hStep : i + 8 ≤ n) (hN : n + 16 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 32#64 ≠ elemAddr dstBase j) :
    let s' := runAmd64Vec 27 initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
    s'.pc = 200 ∧ InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 209 (-108) = some 101 := by decide
  dsimp only [initReducedRow64AVX2Bytes]
  -- Step 1: Loop8Hdr (3 steps: 200 -> 101).
  let sH := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s init2BlkLoop8Hdr
  have hS0 : runAmd64Vec 27 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s =
      runAmd64Vec 24 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH :=
    runAmd64Vec_block_seq 24 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s 200 init2BlkLoop8Hdr hInv.statusRun hPc init2BlkLoop8Hdr_ok
  rw [hS0]
  have hHRun : sH.status = .running := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hHPc : sH.pc = 101 := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHDirt : sH.upperDirty = true := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hHHaz : sH.sseHazard = false := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
      evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hHRax : sH.gpr 0#4 = dstBase := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRcx : sH.gpr 1#4 = BitVec.ofNat 64 i := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRdx : sH.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRbx : sH.gpr 3#4 = BitVec.ofNat 64 n := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRsp : sH.gpr 4#4 = spF := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRdi : sH.gpr 7#4 = costBase := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHR9 : sH.gpr 9#4 = vBase := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHZmm : sH.zmm = s.zmm := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHMem : sH.mem = s.mem := by
    simp [sH, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  -- Step 2: Loop8BodyA (12 steps: 101 -> 150, 0 writes, computes YMM5=vd0, YMM6=c1-v1).
  let sA := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH init2BlkLoop8BodyA
  have hSA : runAmd64Vec 24 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH =
      runAmd64Vec 12 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA :=
    runAmd64Vec_block_seq 12 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH 101 init2BlkLoop8BodyA hHRun hHPc init2BlkLoop8BodyA_ok
  rw [hSA]
  have hCBase : costBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr costBase i := by unfold elemAddr; simp
  have hVBase : vBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr vBase i := by unfold elemAddr; simp
  have hDBase : dstBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr dstBase i := by unfold elemAddr; simp
  have hC0 := hInv.memCost i (by omega); have hC1 := hInv.memCost (i + 1) (by omega)
  have hC2 := hInv.memCost (i + 2) (by omega); have hC3 := hInv.memCost (i + 3) (by omega)
  have hC4 := hInv.memCost (i + 4) (by omega); have hC5 := hInv.memCost (i + 5) (by omega)
  have hC6 := hInv.memCost (i + 6) (by omega); have hC7 := hInv.memCost (i + 7) (by omega)
  have hV0 := hInv.memV i (by omega); have hV1 := hInv.memV (i + 1) (by omega)
  have hV2 := hInv.memV (i + 2) (by omega); have hV3 := hInv.memV (i + 3) (by omega)
  have hV4 := hInv.memV (i + 4) (by omega); have hV5 := hInv.memV (i + 5) (by omega)
  have hV6 := hInv.memV (i + 6) (by omega); have hV7 := hInv.memV (i + 7) (by omega)
  have hARun : sA.status = .running := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHRun]
  have hAPc : sA.pc = 150 := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = true := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHDirt]
  have hAHaz : sA.sseHazard = false := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hHHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax]
  have hARcx : sA.gpr 1#4 = BitVec.ofNat 64 i := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRcx]
  have hARdx : sA.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdx]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRbx]
  have hARsp : sA.gpr 4#4 = spF := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRsp]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHR9]
  have hAR10 : sA.gpr 10#4 = elemAddr dstBase i := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax, hHRcx, hDBase]
  have hAMem : sA.mem = s.mem := by simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHMem]
  have hAZ0 : sA.zmm 0#5 = (Vec512.broadcast ui).zeroHi256 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm, hInv.zmm0Eq]
  have hAZ1 : sA.zmm 1#5 = s.zmm 1#5 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ2 : sA.zmm 2#5 = s.zmm 2#5 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ3 : sA.zmm 3#5 = s.zmm 3#5 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ4 : sA.zmm 4#5 = s.zmm 4#5 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  let vd0 : Vec512 :=
    { l0 := specReducedElem c v ui i, l1 := specReducedElem c v ui (i + 1)
      l2 := specReducedElem c v ui (i + 2), l3 := specReducedElem c v ui (i + 3) }
  let vd1 : Vec512 :=
    { l0 := specReducedElem c v ui (i + 4), l1 := specReducedElem c v ui (i + 5)
      l2 := specReducedElem c v ui (i + 6), l3 := specReducedElem c v ui (i + 7) }
  have hAZ5 : sA.zmm 5#5 = vd0 := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHZmm, hHMem, hInv.zmm0Eq, hCBase, hVBase,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24, hC0, hC1, hC2, hC3, hV0, hV1, hV2, hV3,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, Vec512.zipWith,
      vd0, specReducedElem]
  have hAZ6 : sA.zmm 6#5 =
      { l0 := x86F64Sub (c (i + 4)) (v (i + 4)), l1 := x86F64Sub (c (i + 5)) (v (i + 5))
        l2 := x86F64Sub (c (i + 6)) (v (i + 6)), l3 := x86F64Sub (c (i + 7)) (v (i + 7)) } := by
    simp [sA, init2BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHMem, hCBase, hVBase,
      elemAddr_plus_32, elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24,
      hC4, hC5, hC6, hC7, hV4, hV5, hV6, hV7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.zipWith]
  -- Step 3: Loop8BodyB1 (1 step: 150 -> 155, store vd0 to dst[i..i+3]).
  let sB1 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA init2BlkLoop8BodyB1
  have hSB1 : runAmd64Vec 12 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA =
      runAmd64Vec 11 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 :=
    runAmd64Vec_block_seq 11 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA 150 init2BlkLoop8BodyB1 hARun hAPc init2BlkLoop8BodyB1_ok
  rw [hSB1]
  have hB1Run : sB1.status = .running := by simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hARun]
  have hB1Pc : sB1.pc = 155 := by simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1Dirt : sB1.upperDirty = true := by simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hADirt]
  have hB1Haz : sB1.sseHazard = false := by
    simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, hAHaz]
  have hB1Gpr : sB1.gpr = sA.gpr := by simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1Zmm : sB1.zmm = sA.zmm := by simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1MemEq : sB1.mem = storeVecMem s.mem .ymm256 (elemAddr dstBase i) vd0 0#3 (sA.kmask 0#3) := by
    simp [sB1, init2BlkLoop8BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hAR10, hAZ5, hAMem]
  have hB1Dst : DstMemState dstBase c v dst0 ui n (i + 4) sB1.mem := by
    rw [hB1MemEq]
    exact dstMemState_store4 hInv.memDst (by omega) (by omega) rfl rfl rfl rfl
  -- Step 4: Loop8BodyB2 (4 steps: 155 -> 171, vd1 -= ui and store vd1 to dst[i+4..i+7]).
  let sB2 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 init2BlkLoop8BodyB2
  have hSB2 : runAmd64Vec 11 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 =
      runAmd64Vec 7 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 :=
    runAmd64Vec_block_seq 7 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB1 155 init2BlkLoop8BodyB2 hB1Run hB1Pc init2BlkLoop8BodyB2_ok
  rw [hSB2]
  have hB2Run : sB2.status = .running := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hB1Run]
  have hB2Pc : sB2.pc = 171 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hB2Dirt : sB2.upperDirty = true := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hB1Dirt]
  have hB2Haz : sB2.sseHazard = false := by
    simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hB1Haz]
  have hB2Rax : sB2.gpr 0#4 = dstBase := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARax]
  have hB2Rdx : sB2.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARdx]
  have hB2Rbx : sB2.gpr 3#4 = BitVec.ofNat 64 n := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARbx]
  have hB2Rsp : sB2.gpr 4#4 = spF := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARsp]
  have hB2Rdi : sB2.gpr 7#4 = costBase := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARdi]
  have hB2R9 : sB2.gpr 9#4 = vBase := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hAR9]
  have hB2Z0 : sB2.zmm 0#5 = (Vec512.broadcast ui).zeroHi256 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ0]
  have hB2Z1 : sB2.zmm 1#5 = s.zmm 1#5 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ1]
  have hB2Z2 : sB2.zmm 2#5 = s.zmm 2#5 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ2]
  have hB2Z3 : sB2.zmm 3#5 = s.zmm 3#5 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ3]
  have hB2Z4 : sB2.zmm 4#5 = s.zmm 4#5 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ4]
  have hB2Z5 : sB2.zmm 5#5 = vd0 := by simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ5]
  have hB2Z6 : sB2.zmm 6#5 = vd1 := by
    simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm,
      hB1Zmm, hAZ0, hAZ6, applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast,
      Vec512.zipWith, vd1, specReducedElem]
  have hB2MemEq : sB2.mem = storeVecMem sB1.mem .ymm256 (elemAddr dstBase (i + 4)) vd1 0#3 (sB1.kmask 0#3) := by
    simp [sB2, init2BlkLoop8BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, setZmm,
      hB1Gpr, hB1Zmm, hARax, hARcx, hAZ0, hAZ6, hDBase, elemAddr_plus_32,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, Vec512.zipWith,
      vd1, specReducedElem]
  have hB2Dst : DstMemState dstBase c v dst0 ui n (i + 8) sB2.mem := by
    rw [hB2MemEq]
    exact dstMemState_store4 hB1Dst (by omega) (by omega) rfl rfl rfl rfl
  have hB2MemCost : ∀ j < n, sB2.mem (elemAddr costBase j) = c j := by
    intro j hj
    have hAt1 := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_ymm256_at sB1.mem (elemAddr dstBase (i + 4)) vd1 (sB1.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt1 hAt2
    rw [hB2MemEq, hAt2.2.2.2.2 _ (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega))
        (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2 _ (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega))
        (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega))]
    exact hInv.memCost j hj
  have hB2MemV : ∀ j < n, sB2.mem (elemAddr vBase j) = v j := by
    intro j hj
    have hAt1 := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_ymm256_at sB1.mem (elemAddr dstBase (i + 4)) vd1 (sB1.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt1 hAt2
    rw [hB2MemEq, hAt2.2.2.2.2 _ (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega))
        (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2 _ (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega))
        (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega))]
    exact hInv.memV j hj
  have hB2Rbp : sB2.mem (spF + 32#64) = rbp0 := by
    have hAt1 := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_ymm256_at sB1.mem (elemAddr dstBase (i + 4)) vd1 (sB1.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt1 hAt2
    rw [hB2MemEq, hAt2.2.2.2.2 _ (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega))
        (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2 _ (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega))
        (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega))]
    exact hInv.savedRbp
  -- Step 5: Loop8BodyC (7 steps: 171 -> 200, update ord0/ord1/vmin0/vmin1/rcx).
  have hSC : runAmd64Vec 7 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 =
      runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
        (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 init2BlkLoop8BodyC) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 171 init2BlkLoop8BodyC hB2Run hB2Pc init2BlkLoop8BodyC_ok
  rw [hSC]
  simp only [runAmd64Vec]
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, p0, p1, p2, p3, hZ2M, hZ3M, hOrdEq⟩
  have hCMem : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB2 init2BlkLoop8BodyC).mem = sB2.mem := by
    simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm]
  refine ⟨by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm],
    ⟨by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hB2Run],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hB2Dirt],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, hB2Haz],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rax],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rdx],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rbx],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rsp],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rdi],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2R9],
     by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hB2Z0],
     by intro j hj; rw [hCMem]; exact hB2MemCost j hj,
     by intro j hj; rw [hCMem]; exact hB2MemV j hj,
     by rw [hCMem]; exact hB2Dst,
     by rw [hCMem]; exact hB2Rbp,
     ⟨f64NotNaN (specReducedElem c v ui i) && m0,
      f64NotNaN (specReducedElem c v ui (i + 1)) && m1,
      f64NotNaN (specReducedElem c v ui (i + 2)) && m2,
      f64NotNaN (specReducedElem c v ui (i + 3)) && m3,
      f64NotNaN (specReducedElem c v ui (i + 4)) && p0,
      f64NotNaN (specReducedElem c v ui (i + 5)) && p1,
      f64NotNaN (specReducedElem c v ui (i + 6)) && p2,
      f64NotNaN (specReducedElem c v ui (i + 7)) && p3,
      by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
           hB2Z2, hB2Z5, hZ2M, vd0, applyEvexMask_zero, Vec512.applyWidth, vcmppd0_self_ymm256, vpand_maskVec4],
      by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
           hB2Z3, hB2Z6, hZ3M, vd1, applyEvexMask_zero, Vec512.applyWidth, vcmppd0_self_ymm256, vpand_maskVec4],
      by rw [specAllReducedNotNaN_add8, specAllReducedNotNaN_add4, ← hOrdEq]
         haveI : Std.Associative (· && ·) := ⟨Bool.and_assoc⟩
         haveI : Std.Commutative (· && ·) := ⟨Bool.and_comm⟩
         ac_rfl⟩,
     ?_⟩⟩
  intro hAll8
  rw [specAllReducedNotNaN_add8, specAllReducedNotNaN_add4] at hAll8
  simp only [Bool.and_eq_true] at hAll8
  rcases hAll8 with ⟨⟨⟨⟨⟨⟨⟨⟨hAllI, hr0⟩, hr1⟩, hr2⟩, hr3⟩, hr4⟩, hr5⟩, hr6⟩, hr7⟩
  rcases hInv.minAccs hAllI with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ1, hZ4,
    ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
  refine ⟨vminpdLane (specReducedElem c v ui i) a0,
    vminpdLane (specReducedElem c v ui (i + 1)) a1,
    vminpdLane (specReducedElem c v ui (i + 2)) a2,
    vminpdLane (specReducedElem c v ui (i + 3)) a3,
    vminpdLane (specReducedElem c v ui (i + 4)) b0,
    vminpdLane (specReducedElem c v ui (i + 5)) b1,
    vminpdLane (specReducedElem c v ui (i + 6)) b2,
    vminpdLane (specReducedElem c v ui (i + 7)) b3,
    by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hB2Z1, hB2Z5, hZ1, vd0, applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.zipWith],
    by simp [init2BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hB2Z4, hB2Z6, hZ4, vd1, applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.zipWith],
    vminpdLane_notNaN hr0 ha0, vminpdLane_notNaN hr1 ha1,
    vminpdLane_notNaN hr2 ha2, vminpdLane_notNaN hr3 ha3,
    vminpdLane_notNaN hr4 hb0, vminpdLane_notNaN hr5 hb1,
    vminpdLane_notNaN hr6 hb2, vminpdLane_notNaN hr7 hb3, ?_⟩
  rw [cmin64_combine4_eq hr0 hr1 hr2 hr3 ha0 ha1 ha2 ha3,
      cmin64_combine4_eq hr4 hr5 hr6 hr7 hb0 hb1 hb2 hb3,
      specInitReducedMin_add8, specInitReducedMin_add4, ← hCmin]
  ac_rfl

private theorem initReducedRow64AVX2_loop8_iter (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n : Nat) (hN : n + 16 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 32#64 ≠ elemAddr dstBase j) :
    ∀ k i s, i + 8 * k ≤ n → s.pc = 200 →
      InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s →
      let s' := runAmd64Vec (27 * k) initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
      s'.pc = 200 ∧ InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 8 * k) s' := by
  intro k
  induction k with
  | zero => intro i s _ hPc hInv; simpa [runAmd64Vec] using ⟨hPc, hInv⟩
  | succ m ih =>
    intro i s hLe hPc hInv
    have hStep := initReducedRow64AVX2_loop8_step dstBase costBase vBase spF rbp0 c v dst0 ui n i s
      hPc hInv (by omega) hN hDisjCD hDisjVD hDisjRbp
    have hRest := ih (i + 8) _ (by omega) hStep.1 hStep.2
    have hAdd : 27 * (m + 1) = 27 + 27 * m := by omega
    have hIdx : i + 8 + 8 * m = i + 8 * (m + 1) := by omega
    dsimp only
    rw [hAdd, runAmd64Vec_add, ← hIdx]
    exact hRest

private theorem initReducedRow64AVX2_loop8_exit (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 200) (hInv : InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hLt : n < i + 8) (hN : n + 16 < 2 ^ 61) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
    s'.pc = 257 ∧ InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJmp : nextTargetPc 211 46 = some 257 := by decide
  dsimp only [initReducedRow64AVX2Bytes]
  let sMid := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s init2BlkLoop8Hdr
  have hS0 : runAmd64Vec 4 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s =
      runAmd64Vec 1 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid :=
    runAmd64Vec_block_seq 1 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s 200 init2BlkLoop8Hdr hInv.statusRun hPc init2BlkLoop8Hdr_ok
  rw [hS0]
  have hMRun : sMid.status = .running := by
    simp [sMid, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hMPc : sMid.pc = 209 := by
    simp [sMid, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hS1 : runAmd64Vec 1 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid =
      runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
        (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid init2BlkLoop8Exit) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid 209 init2BlkLoop8Exit hMRun hMPc init2BlkLoop8Exit_ok
  rw [hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [init2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp],
    ⟨by simp [init2BlkLoop8Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMRun],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.upperDirtyT],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
          evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.noHazard],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp],
     by simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.zmm0Eq],
     by intro j hj; simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.memCost j hj],
     by intro j hj; simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp, hInv.memV j hj],
     by have hM : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid init2BlkLoop8Exit).mem = s.mem := by
          simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp]
        rw [hM]; exact hInv.memDst,
     by have hM : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid init2BlkLoop8Exit).mem = s.mem := by
          simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp]
        rw [hM]; exact hInv.savedRbp,
     by have hZ : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid init2BlkLoop8Exit).zmm = s.zmm := by
          simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp]
        rw [hZ]; exact hInv.ordMasks,
     by have hZ : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sMid init2BlkLoop8Exit).zmm = s.zmm := by
          simp [sMid, init2BlkLoop8Exit, init2BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJmp]
        rw [hZ]; exact hInv.minAccs⟩⟩

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_loop4_step (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 257) (hInv : InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hStep : i + 4 ≤ n) (hN : n + 16 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 32#64 ≠ elemAddr dstBase j) :
    let s' := runAmd64Vec 14 initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
    s'.pc = 257 ∧ InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 4) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  have hJcc : nextTargetPc 266 (-55) = some 211 := by decide
  dsimp only [initReducedRow64AVX2Bytes]
  let sH := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s init2BlkLoop4Hdr
  have hS0 : runAmd64Vec 14 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s =
      runAmd64Vec 11 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH :=
    runAmd64Vec_block_seq 11 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s 257 init2BlkLoop4Hdr hInv.statusRun hPc init2BlkLoop4Hdr_ok
  rw [hS0]
  have hHRun : sH.status = .running := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.statusRun]
  have hHPc : sH.pc = 211 := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHDirt : sH.upperDirty = true := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.upperDirtyT]
  have hHHaz : sH.sseHazard = false := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc, hInv.noHazard]
  have hHRax : sH.gpr 0#4 = dstBase := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHRcx : sH.gpr 1#4 = BitVec.ofNat 64 i := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHRdx : sH.gpr 2#4 = BitVec.ofNat 64 (i + 4) := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHRbx : sH.gpr 3#4 = BitVec.ofNat 64 n := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHRsp : sH.gpr 4#4 = spF := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHRdi : sH.gpr 7#4 = costBase := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHR9 : sH.gpr 9#4 = vBase := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHZmm : sH.zmm = s.zmm := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  have hHMem : sH.mem = s.mem := by simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hJcc]
  let sA := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH init2BlkLoop4BodyA
  have hSA : runAmd64Vec 11 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH =
      runAmd64Vec 5 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA :=
    runAmd64Vec_block_seq 5 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH 211 init2BlkLoop4BodyA hHRun hHPc init2BlkLoop4BodyA_ok
  rw [hSA]
  have hCBase : costBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr costBase i := by unfold elemAddr; simp
  have hVBase : vBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr vBase i := by unfold elemAddr; simp
  have hDBase : dstBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr dstBase i := by unfold elemAddr; simp
  have hC0 := hInv.memCost i (by omega); have hC1 := hInv.memCost (i + 1) (by omega)
  have hC2 := hInv.memCost (i + 2) (by omega); have hC3 := hInv.memCost (i + 3) (by omega)
  have hV0 := hInv.memV i (by omega); have hV1 := hInv.memV (i + 1) (by omega)
  have hV2 := hInv.memV (i + 2) (by omega); have hV3 := hInv.memV (i + 3) (by omega)
  let vd0 : Vec512 :=
    { l0 := specReducedElem c v ui i, l1 := specReducedElem c v ui (i + 1)
      l2 := specReducedElem c v ui (i + 2), l3 := specReducedElem c v ui (i + 3) }
  have hARun : sA.status = .running := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHRun]
  have hAPc : sA.pc = 236 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = true := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHDirt]
  have hAHaz : sA.sseHazard = false := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hHHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax]
  have hARdx : sA.gpr 2#4 = BitVec.ofNat 64 (i + 4) := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdx]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRbx]
  have hARsp : sA.gpr 4#4 = spF := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRsp]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHR9]
  have hAR10 : sA.gpr 10#4 = elemAddr dstBase i := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax, hHRcx, hDBase]
  have hAMem : sA.mem = s.mem := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHMem]
  have hAZ0 : sA.zmm 0#5 = (Vec512.broadcast ui).zeroHi256 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm, hInv.zmm0Eq]
  have hAZ1 : sA.zmm 1#5 = s.zmm 1#5 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ2 : sA.zmm 2#5 = s.zmm 2#5 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ3 : sA.zmm 3#5 = s.zmm 3#5 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ4 : sA.zmm 4#5 = s.zmm 4#5 := by simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ5 : sA.zmm 5#5 = vd0 := by
    simp [sA, init2BlkLoop4BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHZmm, hHMem, hInv.zmm0Eq, hCBase, hVBase,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24, hC0, hC1, hC2, hC3, hV0, hV1, hV2, hV3,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.broadcast, Vec512.zipWith,
      vd0, specReducedElem]
  let sB := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA init2BlkLoop4BodyB
  have hSB : runAmd64Vec 5 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA =
      runAmd64Vec 4 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB :=
    runAmd64Vec_block_seq 4 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sA 236 init2BlkLoop4BodyB hARun hAPc init2BlkLoop4BodyB_ok
  rw [hSB]
  have hBRun : sB.status = .running := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hARun]
  have hBPc : sB.pc = 241 := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBDirt : sB.upperDirty = true := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hADirt]
  have hBHaz : sB.sseHazard = false := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, hAHaz]
  have hBGpr : sB.gpr = sA.gpr := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBZmm : sB.zmm = sA.zmm := by simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBMemEq : sB.mem = storeVecMem s.mem .ymm256 (elemAddr dstBase i) vd0 0#3 (sA.kmask 0#3) := by
    simp [sB, init2BlkLoop4BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hAR10, hAZ5, hAMem]
  have hBDst : DstMemState dstBase c v dst0 ui n (i + 4) sB.mem := by
    rw [hBMemEq]; exact dstMemState_store4 hInv.memDst (by omega) (by omega) rfl rfl rfl rfl
  have hBMemCost : ∀ j < n, sB.mem (elemAddr costBase j) = c j := by
    intro j hj
    have hAt := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt
    rw [hBMemEq, hAt.2.2.2.2 _ (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega))
        (hDisjCD j hj _ (by omega)) (hDisjCD j hj _ (by omega))]
    exact hInv.memCost j hj
  have hBMemV : ∀ j < n, sB.mem (elemAddr vBase j) = v j := by
    intro j hj
    have hAt := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt
    rw [hBMemEq, hAt.2.2.2.2 _ (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega))
        (hDisjVD j hj _ (by omega)) (hDisjVD j hj _ (by omega))]
    exact hInv.memV j hj
  have hBRbp : sB.mem (spF + 32#64) = rbp0 := by
    have hAt := storeVecMem_ymm256_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24] at hAt
    rw [hBMemEq, hAt.2.2.2.2 _ (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega))
        (hDisjRbp _ (by omega)) (hDisjRbp _ (by omega))]
    exact hInv.savedRbp
  have hSC : runAmd64Vec 4 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB =
      runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
        (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB init2BlkLoop4BodyC) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
      initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB 241 init2BlkLoop4BodyC hBRun hBPc init2BlkLoop4BodyC_ok
  rw [hSC]
  simp only [runAmd64Vec]
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, p0, p1, p2, p3, hZ2M, hZ3M, hOrdEq⟩
  have hCMem : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sB init2BlkLoop4BodyC).mem = sB.mem := by
    simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm]
  refine ⟨by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm],
    ⟨by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hBRun],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hBDirt],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, hBHaz],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARax],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARdx],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARbx],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARsp],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARdi],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hAR9],
     by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ0],
     by intro j hj; rw [hCMem]; exact hBMemCost j hj,
     by intro j hj; rw [hCMem]; exact hBMemV j hj,
     by rw [hCMem]; exact hBDst,
     by rw [hCMem]; exact hBRbp,
     ⟨m0 && f64NotNaN (specReducedElem c v ui i),
      m1 && f64NotNaN (specReducedElem c v ui (i + 1)),
      m2 && f64NotNaN (specReducedElem c v ui (i + 2)),
      m3 && f64NotNaN (specReducedElem c v ui (i + 3)),
      p0, p1, p2, p3,
      by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
           hBZmm, hAZ2, hAZ5, hZ2M, vd0, applyEvexMask_zero, Vec512.applyWidth, vcmppd0_self_ymm256, vpand_maskVec4],
      by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ3, hZ3M],
      by rw [specAllReducedNotNaN_add4, ← hOrdEq]
         haveI : Std.Associative (· && ·) := ⟨Bool.and_assoc⟩
         haveI : Std.Commutative (· && ·) := ⟨Bool.and_comm⟩
         ac_rfl⟩,
     ?_⟩⟩
  intro hAll4
  rw [specAllReducedNotNaN_add4] at hAll4
  simp only [Bool.and_eq_true] at hAll4
  rcases hAll4 with ⟨⟨⟨⟨hAllI, hr0⟩, hr1⟩, hr2⟩, hr3⟩
  rcases hInv.minAccs hAllI with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ1, hZ4,
    ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
  refine ⟨vminpdLane a0 (specReducedElem c v ui i),
    vminpdLane a1 (specReducedElem c v ui (i + 1)),
    vminpdLane a2 (specReducedElem c v ui (i + 2)),
    vminpdLane a3 (specReducedElem c v ui (i + 3)),
    b0, b1, b2, b3,
    by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hBZmm, hAZ1, hAZ5, hZ1, vd0, applyEvexMask_zero, Vec512.applyWidth, Vec512.zeroHi256, Vec512.zipWith],
    by simp [init2BlkLoop4BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ4, hZ4],
    vminpdLane_notNaN ha0 hr0, vminpdLane_notNaN ha1 hr1,
    vminpdLane_notNaN ha2 hr2, vminpdLane_notNaN ha3 hr3,
    hb0, hb1, hb2, hb3, ?_⟩
  rw [cmin64_combine4_eq ha0 ha1 ha2 ha3 hr0 hr1 hr2 hr3,
      specInitReducedMin_add4, ← hCmin]
  ac_rfl

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX2_epilogue (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 257) (hInv : InitReduced2Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hLt : n < i + 4) (hN : n + 16 < 2 ^ 61) (hi : i ≤ n)
    (hDisjSpill : ∀ j < n, ∀ k < 4, elemAddr dstBase j ≠ spF + BitVec.ofNat 64 (k * 8)) :
    ∃ st ≤ 32,
      let sf := runAmd64Vec st initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      DstMemState dstBase c v dst0 ui n i sf.mem ∧
      (specAllReducedNotNaN c v ui i = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
      (specAllReducedNotNaN c v ui i = true →
        f64NotNaN (sf.zmm 0#5).l0 = true ∧ canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui i) ∧
      sf.gpr 5#4 = rbp0 := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 4))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd4 : BitVec.ofNat 64 i + 4#64 = BitVec.ofNat 64 (i + 4) := bitVec_ofNat_add i 4
  have hJcc312 : nextTargetPc 287 25 = some 312 := by decide
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, p0, p1, p2, p3, hZ2M, hZ3M, hOrdEq⟩
  have hMskEq : evalX86Cond .e
      (x86SubFlags8 ((maskVec4 (m0 && p0) (m1 && p1) (m2 && p2) (m3 && p3)).movmskpd4.extractLsb' 0 8) 0x0f#8) =
      specAllReducedNotNaN c v ui i := by
    rw [evalX86Cond_e_sub8, movmskpd4_maskVec4_eq_0xf, hOrdEq]
  let sH := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s init2BlkLoop4Hdr
  have hHRun : sH.status = .running := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.statusRun]
  have hHPc : sH.pc = 266 := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hHDirt : sH.upperDirty = true := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.upperDirtyT]
  have hHHaz : sH.sseHazard = false := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe, hInv.noHazard]
  have hHRsp : sH.gpr 4#4 = spF := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hHZmm : sH.zmm = s.zmm := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  have hHMem : sH.mem = s.mem := by
    simp [sH, init2BlkLoop4Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd4, hGe]
  let vMin : Vec512 := (Vec512.zipWith vminpdLane (s.zmm 1#5) (s.zmm 4#5)).zeroHi256
  let sE := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH init2BlkEpiCmp
  have hEMemEq : sE.mem = storeVecMem s.mem .ymm256 spF vMin 0#3 (sH.kmask 0#3) := by
    simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, setGpr, hHRsp, hHZmm, hHMem, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
      vpand_maskVec4, hMskEq, hJcc312, vMin]
    cases specAllReducedNotNaN c v ui i <;> rfl
  have hEAt := storeVecMem_ymm256_at s.mem spF vMin (sH.kmask 0#3)
  have hSp0 : spF + 0#64 = spF := BitVec.add_zero spF
  have hEDst : DstMemState dstBase c v dst0 ui n i sE.mem := by
    have hPres : ∀ j < n, sE.mem (elemAddr dstBase j) = s.mem (elemAddr dstBase j) := by
      intro j hj
      rw [hEMemEq]
      exact hEAt.2.2.2.2 _ (by simpa [hSp0] using hDisjSpill j hj 0 (by omega))
        (hDisjSpill j hj 1 (by omega)) (hDisjSpill j hj 2 (by omega)) (hDisjSpill j hj 3 (by omega))
    exact ⟨fun j hj => (hPres j (by omega)).trans (hInv.memDst.1 j hj),
           fun j hge hj => (hPres j hj).trans (hInv.memDst.2 j hge hj)⟩
  have hERbp : sE.mem (spF + 32#64) = rbp0 := by
    rw [hEMemEq]
    have h0 : spF + 32#64 ≠ spF := by bv_omega
    have h1 : spF + 32#64 ≠ spF + 8#64 := by bv_omega
    have h2 : spF + 32#64 ≠ spF + 16#64 := by bv_omega
    have h3 : spF + 32#64 ≠ spF + 24#64 := by bv_omega
    exact (hEAt.2.2.2.2 _ h0 h1 h2 h3).trans hInv.savedRbp
  cases hOrd : specAllReducedNotNaN c v ui i
  · -- NaN branch: 3 + 6 + 7 = 16 steps.
    refine ⟨16, by omega, ?_⟩
    dsimp only [initReducedRow64AVX2Bytes]
    have hS0 : runAmd64Vec 16 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s =
        runAmd64Vec 13 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH :=
      runAmd64Vec_block_seq 13 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s 257 init2BlkLoop4Hdr hInv.statusRun hPc init2BlkLoop4Hdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 13 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH =
        runAmd64Vec 7 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE :=
      runAmd64Vec_block_seq 7 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH 266 init2BlkEpiCmp hHRun hHPc init2BlkEpiCmp_ok
    rw [hS1]
    have hERun : sE.status = .running := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hHRun]
    have hEPc : sE.pc = 287 := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd]
    have hEHaz : sE.sseHazard = false := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hHHaz]
    have hERsp : sE.gpr 4#4 = spF := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd]
    have hS2 : runAmd64Vec 7 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE =
        runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
          (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE init2BlkRetNaN) :=
      runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE 287 init2BlkRetNaN hERun hEPc init2BlkRetNaN_ok
    rw [hS2]
    simp only [runAmd64Vec]
    have hRetMem : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE init2BlkRetNaN).mem = sE.mem := by
      simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn]
    refine ⟨by simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn],
      by simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper],
      by simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, Amd64VecInsn.dirtiesUpper, hEHaz],
      by rw [hRetMem]; exact hEDst,
      fun _ => by simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn, setGpr, setZmm],
      fun h => Bool.noConfusion h,
      by simp [init2BlkRetNaN, execDecodedSeq, execAmd64VecInsn, setGpr, hERsp, hERbp]⟩
  · -- Ordered branch: 3 + 6 + 8 + 7 + 8 = 32 steps.
    refine ⟨32, by omega, ?_⟩
    dsimp only [initReducedRow64AVX2Bytes]
    have hS0 : runAmd64Vec 32 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s =
        runAmd64Vec 29 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH :=
      runAmd64Vec_block_seq 29 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s 257 init2BlkLoop4Hdr hInv.statusRun hPc init2BlkLoop4Hdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 29 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH =
        runAmd64Vec 23 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE :=
      runAmd64Vec_block_seq 23 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sH 266 init2BlkEpiCmp hHRun hHPc init2BlkEpiCmp_ok
    rw [hS1]
    have hERun : sE.status = .running := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hJcc312, hHRun]
    have hEPc : sE.pc = 312 := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hJcc312]
    have hEHaz : sE.sseHazard = false := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hJcc312, hHHaz]
    have hERsp : sE.gpr 4#4 = spF := by
      simp [sE, init2BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec4, hMskEq, hOrd, hJcc312]
    rcases hInv.minAccs hOrd with ⟨a0, a1, a2, a3, b0, b1, b2, b3, hZ1, hZ4,
      ha0, ha1, ha2, ha3, hb0, hb1, hb2, hb3, hCmin⟩
    let l0 := vminpdLane a0 b0
    let l1 := vminpdLane a1 b1
    let l2 := vminpdLane a2 b2
    let l3 := vminpdLane a3 b3
    have hl0 : f64NotNaN l0 = true := vminpdLane_notNaN ha0 hb0
    have hl1 : f64NotNaN l1 = true := vminpdLane_notNaN ha1 hb1
    have hl2 : f64NotNaN l2 = true := vminpdLane_notNaN ha2 hb2
    have hl3 : f64NotNaN l3 = true := vminpdLane_notNaN ha3 hb3
    have hvMinEq : vMin = { l0 := l0, l1 := l1, l2 := l2, l3 := l3 } := by
      simp [vMin, hZ1, hZ4, Vec512.zipWith, Vec512.zeroHi256, l0, l1, l2, l3]
    have hM0 : sE.mem spF = l0 := by rw [hEMemEq, hEAt.1, hvMinEq]
    have hM1 : sE.mem (spF + 8#64) = l1 := by rw [hEMemEq, hEAt.2.1, hvMinEq]
    have hM2 : sE.mem (spF + 16#64) = l2 := by rw [hEMemEq, hEAt.2.2.1, hvMinEq]
    have hM3 : sE.mem (spF + 24#64) = l3 := by rw [hEMemEq, hEAt.2.2.2.1, hvMinEq]
    -- Horiz01 (8 steps: 312 -> 344).
    let s1 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE init2BlkHoriz01
    have hS2 : runAmd64Vec 23 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE =
        runAmd64Vec 15 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s1 :=
      runAmd64Vec_block_seq 15 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets sE 312 init2BlkHoriz01 hERun hEPc init2BlkHoriz01_ok
    rw [hS2]
    have h1Run : s1.status = .running := by simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, hERun]
    have h1Pc : s1.pc = 344 := by simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h1Dirt : s1.upperDirty = false := by
      simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper]
    have h1Haz : s1.sseHazard = false := by
      simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr,
        Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, hEHaz]
    have h1Rsp : s1.gpr 4#4 = spF := by simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, hERsp]
    have h1Mem : s1.mem = sE.mem := by simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h1Z2 : (s1.zmm 2#5).l0 = f64Min l0 l1 := by
      simp [s1, init2BlkHoriz01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        hERsp, hM0, hM1, minsd_por_eq_f64Min l0 l1 hl0 hl1]
    -- Horiz23 (7 steps: 344 -> 374).
    let s2 := execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s1 init2BlkHoriz23
    have hS3 : runAmd64Vec 15 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s1 =
        runAmd64Vec 8 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 :=
      runAmd64Vec_block_seq 8 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s1 344 init2BlkHoriz23 h1Run h1Pc init2BlkHoriz23_ok
    rw [hS3]
    have h2Run : s2.status = .running := by simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Run]
    have h2Pc : s2.pc = 374 := by simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h2Dirt : s2.upperDirty = false := by
      simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, h1Dirt]
    have h2Haz : s2.sseHazard = false := by
      simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr,
        Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h1Dirt, h1Haz]
    have h2Rsp : s2.gpr 4#4 = spF := by simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Rsp]
    have h2Mem : s2.mem = sE.mem := by simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Mem]
    have h2Z2 : (s2.zmm 2#5).l0 = f64Min l0 l1 := by
      simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, h1Z2]
    have h2Z0 : (s2.zmm 0#5).l0 = f64Min l2 l3 := by
      simp [s2, init2BlkHoriz23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        h1Rsp, h1Mem, hM2, hM3, minsd_por_eq_f64Min l2 l3 hl2 hl3]
    -- HorizFin (8 steps: 374 -> ret).
    have hS4 : runAmd64Vec 8 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 =
        runAmd64Vec 0 ⟨initReducedRow64AVX2ByteChunks.flatten⟩ initReducedRow64AVX2Rodata initReducedRow64AVX2Targets
          (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 init2BlkHorizFin) :=
      runAmd64Vec_block_seq 0 initReducedRow64AVX2ByteChunks initReducedRow64AVX2_chunks_wf
        initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 374 init2BlkHorizFin h2Run h2Pc init2BlkHorizFin_ok
    rw [hS4]
    simp only [runAmd64Vec]
    have hFinMem : (execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 init2BlkHorizFin).mem = sE.mem := by
      simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn, h2Mem]
    have hTree := cmin64_f64Min_tree4 hl0 hl1 hl2 hl3
    have h01 := f64Min_notNaN hl0 hl1
    have h23 := f64Min_notNaN hl2 hl3
    have hZ0Fin : ((execDecodedSeq initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s2 init2BlkHorizFin).zmm 0#5).l0 =
        f64Min (f64Min l0 l1) (f64Min l2 l3) := by
      simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn, setZmm, h2Z2, h2Z0]
      exact minsd_por_eq_f64Min _ _ h01 h23
    refine ⟨by simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn],
      by simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, h2Dirt],
      by simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
           Amd64VecInsn.isLegacySSE, h2Dirt, h2Haz],
      by rw [hFinMem]; exact hEDst,
      fun h => Bool.noConfusion h,
      fun _ => ?_,
      by simp [init2BlkHorizFin, execDecodedSeq, execAmd64VecInsn, setGpr, h2Rsp, h2Mem, hERbp]⟩
    rw [hZ0Fin]
    refine ⟨hTree.1, ?_⟩
    rw [hTree.2]
    dsimp only [l0, l1, l2, l3]
    rw [cmin64_combine4_eq ha0 ha1 ha2 ha3 hb0 hb1 hb2 hb3, hCmin]

/-- Fuel bound for executing decoded `initReducedRow64AVX2` on any slice length `n`. -/
def initReducedRow64AVX2FuelBound (n : Nat) : Nat :=
  27 * (n / 8) + 75

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`initReducedRow64AVX2`)**:
For arbitrary non-empty 8-byte-aligned slices (`n = 4q + r`, `0 < n`, `n ≤ lenDst`, `n ≤ lenCost`,
`n ≤ lenV`, `n + 16 < 2^61`) and non-aliasing `dst`/`costRow`/`v`/stack memory, running the raw ELF
`.text` bytes of `internal/simd.initReducedRow64AVX2`:
1. Terminates cleanly at `RET` with `upperDirty = false` and `sseHazard = false` (all 3
   `runtime.panicBounds` guards are provably unreachable);
2. Writes bit-exact x86-64 IEEE-754 `dst[j] = x86F64Sub (x86F64Sub costRow[j] v[j]) ui` for all
   `j < 4 * (n / 4)` (matching Intel SDM `VSUBPD` NaN quieting and negative indefinite NaN
   `0xfff8000000000000#64`) while leaving tail lanes `4 * (n / 4) ≤ j < n` unmodified;
3. Returns exact quiet-NaN sentinel `0x7ff8000000000001#64` iff any reduced element in `0..4*(n/4)`
   is NaN, and otherwise returns a non-NaN minimum whose `canonZero64` equals `specInitReducedMin`;
4. Preserves Go ABI registers: `RSP = RSP0 + 8`, `RBP = RBP0`, `R14 = R14_0`, and `XMM15 = XMM15_0`. -/
theorem initReducedRow64AVX2_functional_equiv (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenDst lenCost lenV fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hLenDst : n ≤ lenDst) (hLenCost : n ≤ lenCost) (hLenV : n ≤ lenV)
    (hN : n + 16 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV64 : lenV < 2 ^ 64)
    (hAlign : dstBase.toNat % 8 = 0 ∧ costBase.toNat % 8 = 0 ∧ vBase.toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64)
    (hDisjSpill : ∀ j < n, ∀ k < 4, elemAddr dstBase j ≠ s0.gpr 4#4 - 40#64 + BitVec.ofNat 64 (k * 8))
    (hFuel : initReducedRow64AVX2FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
    sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
    (∀ j < 4 * (n / 4), sf.mem (elemAddr dstBase j) = specReducedElem c v ui j) ∧
    (∀ j, 4 * (n / 4) ≤ j → j < n → sf.mem (elemAddr dstBase j) = dst0 j) ∧
    (specAllReducedNotNaN c v ui (4 * (n / 4)) = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
    (specAllReducedNotNaN c v ui (4 * (n / 4)) = true →
      f64NotNaN (sf.zmm 0#5).l0 = true ∧
      canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui (4 * (n / 4))) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  have _ := And.intro hLenDst hAlign
  let spF := s0.gpr 4#4 - 40#64
  let rbp0 := s0.gpr 5#4
  have hDisjRbp : ∀ j < n, spF + 32#64 ≠ elemAddr dstBase j := by
    intro j hj
    have hEq : spF + 32#64 = s0.gpr 4#4 - 8#64 := by dsimp only [spF]; bv_omega
    rw [hEq]
    exact (hDisjPro j hj dstBase (by simp)).1.symm
  have hPro := initReducedRow64AVX2_prologue dstBase costBase vBase c v dst0 ui n lenCost lenV s0
    hPc hRun hHaz hPos (by omega) (by omega) hN hLenC hLenV64 hRax hRbx hRdi hRsi hR9 hR10 hUi hMemC hMemV hMemD hDisjPro
  have hL8 := initReducedRow64AVX2_loop8_iter dstBase costBase vBase spF rbp0 c v dst0 ui n hN hDisjCD hDisjVD hDisjRbp
    (n / 8) 0 _ (by omega) hPro.1 hPro.2
  have hEx8 := initReducedRow64AVX2_loop8_exit dstBase costBase vBase spF rbp0 c v dst0 ui n (8 * (n / 8)) _
    hL8.1 (by simpa using hL8.2) (by omega) hN (by omega)
  have hExist : ∃ stTotal ≤ initReducedRow64AVX2FuelBound n,
      let se := runAmd64Vec stTotal initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      DstMemState dstBase c v dst0 ui n (4 * (n / 4)) se.mem ∧
      (specAllReducedNotNaN c v ui (4 * (n / 4)) = false → (se.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
      (specAllReducedNotNaN c v ui (4 * (n / 4)) = true →
        f64NotNaN (se.zmm 0#5).l0 = true ∧
        canonZero64 (se.zmm 0#5).l0 = specInitReducedMin c v ui (4 * (n / 4))) ∧
      se.gpr 5#4 = rbp0 := by
    unfold initReducedRow64AVX2FuelBound
    by_cases hRem : 8 * (n / 8) + 4 ≤ n
    · have h4Eq : 8 * (n / 8) + 4 = 4 * (n / 4) := by omega
      have hL4 := initReducedRow64AVX2_loop4_step dstBase costBase vBase spF rbp0 c v dst0 ui n (8 * (n / 8)) _
        hEx8.1 hEx8.2 hRem hN hDisjCD hDisjVD hDisjRbp
      rcases initReducedRow64AVX2_epilogue dstBase costBase vBase spF rbp0 c v dst0 ui n (4 * (n / 4)) _
        hL4.1 (by simpa [h4Eq] using hL4.2) (by omega) hN (by omega) hDisjSpill with ⟨stE, hStE, hEpi⟩
      refine ⟨25 + 27 * (n / 8) + 4 + 14 + stE, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (25 + 27 * (n / 8) + 4 + 14) stE,
          runAmd64Vec_add (25 + 27 * (n / 8) + 4) 14,
          runAmd64Vec_add (25 + 27 * (n / 8)) 4,
          runAmd64Vec_add 25 (27 * (n / 8))]
      exact hEpi
    · have h4Eq : 8 * (n / 8) = 4 * (n / 4) := by omega
      rcases initReducedRow64AVX2_epilogue dstBase costBase vBase spF rbp0 c v dst0 ui n (4 * (n / 4)) _
        hEx8.1 (by simpa [h4Eq] using hEx8.2) (by omega) hN (by omega) hDisjSpill with ⟨stE, hStE, hEpi⟩
      refine ⟨25 + 27 * (n / 8) + 4 + stE, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (25 + 27 * (n / 8) + 4) stE,
          runAmd64Vec_add (25 + 27 * (n / 8)) 4,
          runAmd64Vec_add 25 (27 * (n / 8))]
      exact hEpi
  have hAbi := initReducedRow64AVX2_abi_reg_preserved s0 fuel hPc hRun
  rcases hExist with ⟨stTotal, hStLe, hStat, hDirt, hHazF, hDst, hNaN, hOrd, hRbp⟩
  have hSplit : fuel = stTotal + (fuel - stTotal) := by omega
  have hSfStat : (runAmd64Vec fuel initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0).status = .returned := by
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])]; exact hStat
  have hRsp := hAbi.2.2.2 hSfStat
  dsimp only
  rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])]
  rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])] at hAbi hRsp
  exact ⟨hStat, hDirt, hHazF, hDst.1, hDst.2, hNaN, hOrd, hRsp, hRbp, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

private theorem specAllReducedNotNaN_add16 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specAllReducedNotNaN c v ui (i + 16) =
      (specAllReducedNotNaN c v ui (i + 8) &&
       f64NotNaN (specReducedElem c v ui (i + 8)) && f64NotNaN (specReducedElem c v ui (i + 9)) &&
       f64NotNaN (specReducedElem c v ui (i + 10)) && f64NotNaN (specReducedElem c v ui (i + 11)) &&
       f64NotNaN (specReducedElem c v ui (i + 12)) && f64NotNaN (specReducedElem c v ui (i + 13)) &&
       f64NotNaN (specReducedElem c v ui (i + 14)) && f64NotNaN (specReducedElem c v ui (i + 15))) := by
  simp only [specAllReducedNotNaN]

private theorem specInitReducedMin_add16 (c v : Nat → BitVec 64) (ui : BitVec 64) (i : Nat) :
    specInitReducedMin c v ui (i + 16) =
      cmin64 (cmin64 (cmin64 (cmin64
        (cmin64 (cmin64 (cmin64 (cmin64 (specInitReducedMin c v ui (i + 8))
          (specReducedElem c v ui (i + 8))) (specReducedElem c v ui (i + 9)))
          (specReducedElem c v ui (i + 10))) (specReducedElem c v ui (i + 11)))
        (specReducedElem c v ui (i + 12))) (specReducedElem c v ui (i + 13)))
        (specReducedElem c v ui (i + 14))) (specReducedElem c v ui (i + 15)) := by
  simp only [specInitReducedMin]

/-- Loop invariant at `pc = 240` (16-wide header) and `pc = 313` (8-wide header) of `initReducedRow64AVX512`. -/
private structure InitReduced512Inv (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState) : Prop where
  statusRun : s.status = .running
  upperDirtyT : s.upperDirty = true
  noHazard : s.sseHazard = false
  raxEq : s.gpr 0#4 = dstBase
  rcxEq : s.gpr 1#4 = BitVec.ofNat 64 i
  rbxEq : s.gpr 3#4 = BitVec.ofNat 64 n
  rspEq : s.gpr 4#4 = spF
  rdiEq : s.gpr 7#4 = costBase
  r9Eq : s.gpr 9#4 = vBase
  zmm0Eq : s.zmm 0#5 = Vec512.broadcast ui
  memCost : ∀ j < n, s.mem (elemAddr costBase j) = c j
  memV : ∀ j < n, s.mem (elemAddr vBase j) = v j
  memDst : DstMemState dstBase c v dst0 ui n i s.mem
  savedRbp : s.mem (spF + 64#64) = rbp0
  ordMasks : ∃ m0 m1 m2 m3 m4 m5 m6 m7 p0 p1 p2 p3 p4 p5 p6 p7 : Bool,
    s.zmm 2#5 = maskVec8 m0 m1 m2 m3 m4 m5 m6 m7 ∧
    s.zmm 3#5 = maskVec8 p0 p1 p2 p3 p4 p5 p6 p7 ∧
    ((m0 && p0) && (m1 && p1) && (m2 && p2) && (m3 && p3) &&
     (m4 && p4) && (m5 && p5) && (m6 && p6) && (m7 && p7)) = specAllReducedNotNaN c v ui i
  minAccs : specAllReducedNotNaN c v ui i = true →
    ∃ a0 a1 a2 a3 a4 a5 a6 a7 b0 b1 b2 b3 b4 b5 b6 b7 : BitVec 64,
      s.zmm 1#5 = { l0 := a0, l1 := a1, l2 := a2, l3 := a3, l4 := a4, l5 := a5, l6 := a6, l7 := a7 } ∧
      s.zmm 4#5 = { l0 := b0, l1 := b1, l2 := b2, l3 := b3, l4 := b4, l5 := b5, l6 := b6, l7 := b7 } ∧
      f64NotNaN a0 = true ∧ f64NotNaN a1 = true ∧ f64NotNaN a2 = true ∧ f64NotNaN a3 = true ∧
      f64NotNaN a4 = true ∧ f64NotNaN a5 = true ∧ f64NotNaN a6 = true ∧ f64NotNaN a7 = true ∧
      f64NotNaN b0 = true ∧ f64NotNaN b1 = true ∧ f64NotNaN b2 = true ∧ f64NotNaN b3 = true ∧
      f64NotNaN b4 = true ∧ f64NotNaN b5 = true ∧ f64NotNaN b6 = true ∧ f64NotNaN b7 = true ∧
      cmin64 (cmin64 (cmin64 (cmin64 a0 a1) (cmin64 a2 a3)) (cmin64 (cmin64 a4 a5) (cmin64 a6 a7)))
             (cmin64 (cmin64 (cmin64 b0 b1) (cmin64 b2 b3)) (cmin64 (cmin64 b4 b5) (cmin64 b6 b7))) =
        specInitReducedMin c v ui i

private def init512BlkPro0 : List (Amd64VecInsn × Nat) :=
  [ (.pushReg 5#4, 1),
    (.movRR64 5#4 4#4, 4),
    (.subImm64 4#4 0x40#64, 8),
    (.movMR64 (.baseIdxDisp (some 4#4) none 80) 0#4, 13),
    (.movMR64 (.baseIdxDisp (some 4#4) none 104) 7#4, 18),
    (.movMR64 (.baseIdxDisp (some 4#4) none 128) 9#4, 26),
    (.nop, 32) ]

private def init512BlkPro1a : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 3#4 3#4, 35),
    (.jccRel .be 540, 41) ]

private def init512BlkPro1b : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 6#4 6#4, 44),
    (.jccRel .be 523, 50) ]

private def init512BlkPro1c : List (Amd64VecInsn × Nat) :=
  [ (.testRR64 10#4 10#4, 53),
    (.jccRel .be 509, 59) ]

private def init512BlkPro2 : List (Amd64VecInsn × Nat) :=
  [ (.vmovsdRR 0#5 15#5 0#5, 63),
    (.vmovsdLoad 1#5 (.ripRel 270617), 71),
    (.vbroadcastsd true .zmm512 1#5 (.reg 1#5) {}, 77),
    (.vcmppdEvex .zmm512 1#3 1#5 (.reg 1#5) 0 {}, 84),
    (.nop, 85), (.nop, 86), (.nop, 87),
    (.vbroadcastsd true .zmm512 0#5 (.reg 0#5) {}, 93),
    (.vpmovm2q .zmm512 2#5 1#3, 99),
    (.xorRR32 1#4 1#4, 101),
    (.vmovdquRR true .zmm512 3#5 2#5 {}, 107),
    (.vmovdquRR true .zmm512 4#5 1#5 {}, 113),
    (.jmpRel 125, 115) ]

private def init512BlkLoop16Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 16), 244),
    (.cmpRR64 3#4 2#4, 247),
    (.jccRel .ge (-138), 253) ]

private def init512BlkLoop16Exit : List (Amd64VecInsn × Nat) :=
  [ (.jmpRel 58, 255) ]

private def init512BlkLoop16BodyA : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 119),
    (.lea64 8#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 123),
    (.lea64 10#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 127),
    (.vmovdquLoad true .zmm512 5#5 (.baseIdxDisp (some 6#4) none 0) {}, 133),
    (.vsubpd true .zmm512 5#5 5#5 (.mem (.baseIdxDisp (some 8#4) none 0)) {}, 139),
    (.vsubpd true .zmm512 5#5 5#5 (.reg 0#5) {}, 145),
    (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 149),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 64), 153),
    (.vmovdquLoad true .zmm512 6#5 (.baseIdxDisp (some 6#4) none 0) {}, 159),
    (.lea64 6#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 163),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 64), 167),
    (.vsubpd true .zmm512 6#5 6#5 (.mem (.baseIdxDisp (some 6#4) none 0)) {}, 173) ]

private def init512BlkLoop16BodyB1 : List (Amd64VecInsn × Nat) :=
  [ (.vmovdquStore true .zmm512 (.baseIdxDisp (some 10#4) none 0) 5#5 {}, 179) ]

private def init512BlkLoop16BodyB2 : List (Amd64VecInsn × Nat) :=
  [ (.vsubpd true .zmm512 6#5 6#5 (.reg 0#5) {}, 185),
    (.lea64 6#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 189),
    (.lea64 6#4 (.baseIdxDisp (some 6#4) none 64), 193),
    (.vmovdquStore true .zmm512 (.baseIdxDisp (some 6#4) none 0) 6#5 {}, 199) ]

private def init512BlkLoop16BodyC : List (Amd64VecInsn × Nat) :=
  [ (.vcmppdEvex .zmm512 1#3 5#5 (.reg 5#5) 0 {}, 206),
    (.vcmppdEvex .zmm512 2#3 6#5 (.reg 6#5) 0 {}, 213),
    (.vmovdquRR true .zmm512 2#5 2#5 { kmask := 1#3, zeroing := true, broadcast := false }, 219),
    (.vmovdquRR true .zmm512 3#5 3#5 { kmask := 2#3, zeroing := true, broadcast := false }, 225),
    (.vminpd true .zmm512 1#5 5#5 (.reg 1#5) {}, 231),
    (.vminpd true .zmm512 4#5 6#5 (.reg 4#5) {}, 237),
    (.movRR64 1#4 2#4, 240) ]

private def init512BlkLoop8Hdr : List (Amd64VecInsn × Nat) :=
  [ (.lea64 2#4 (.baseIdxDisp (some 1#4) none 8), 317),
    (.nop, 320),
    (.cmpRR64 3#4 2#4, 323),
    (.jccRel .ge (-70), 325) ]

private def init512BlkLoop8BodyA : List (Amd64VecInsn × Nat) :=
  [ (.lea64 6#4 (.baseIdxDisp (some 7#4) (some (1#4, 8)) 0), 259),
    (.lea64 8#4 (.baseIdxDisp (some 9#4) (some (1#4, 8)) 0), 263),
    (.lea64 10#4 (.baseIdxDisp (some 0#4) (some (1#4, 8)) 0), 267),
    (.vmovdquLoad true .zmm512 5#5 (.baseIdxDisp (some 6#4) none 0) {}, 273),
    (.vsubpd true .zmm512 5#5 5#5 (.mem (.baseIdxDisp (some 8#4) none 0)) {}, 279),
    (.vsubpd true .zmm512 5#5 5#5 (.reg 0#5) {}, 285) ]

private def init512BlkLoop8BodyB : List (Amd64VecInsn × Nat) :=
  [ (.vmovdquStore true .zmm512 (.baseIdxDisp (some 10#4) none 0) 5#5 {}, 291) ]

private def init512BlkLoop8BodyC : List (Amd64VecInsn × Nat) :=
  [ (.vcmppdEvex .zmm512 1#3 5#5 (.reg 5#5) 0 {}, 298),
    (.vmovdquRR true .zmm512 2#5 2#5 { kmask := 1#3, zeroing := true, broadcast := false }, 304),
    (.vminpd true .zmm512 1#5 1#5 (.reg 5#5) {}, 310),
    (.movRR64 1#4 2#4, 313) ]

private def init512BlkEpiCmp : List (Amd64VecInsn × Nat) :=
  [ (.vminpd true .zmm512 0#5 1#5 (.reg 4#5) {}, 331),
    (.vmovdquStore true .zmm512 (.baseIdxDisp (some 4#4) none 0) 0#5 {}, 338),
    (.vpand true .zmm512 0#5 2#5 (.reg 3#5) {}, 344),
    (.vpmovq2m .zmm512 1#3 0#5, 350),
    (.kmovbToGpr 0#4 1#3, 354),
    (.cmpImm8 0#4 0xff#8, 356),
    (.jccRel .e 25, 358) ]

private def init512BlkRetNaN : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 361),
    (.movImm64 0#4 0x7ff8000000000001#64, 371),
    (.sseMovqFromGpr 0#4 0#4, 376),
    (.nop, 377),
    (.addImm64 4#4 0x40#64, 381),
    (.popReg 5#4, 382),
    (.ret, 383) ]

private def init512BlkH01 : List (Amd64VecInsn × Nat) :=
  [ (.vzeroupper, 386),
    (.sseMovsdLoad 0#4 (.baseIdxDisp (some 4#4) none 0), 391),
    (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 8), 397),
    (.sseMovupsRR 2#4 0#4, 400),
    (.sseMinsdRR 2#4 1#4, 404),
    (.sseMovupsRR 1#4 2#4, 407),
    (.sseMinsdRR 2#4 0#4, 411),
    (.ssePorRR 2#4 1#4, 415) ]

private def init512BlkH23 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 0#4 (.baseIdxDisp (some 4#4) none 16), 421),
    (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 24), 427),
    (.sseMovupsRR 3#4 0#4, 430),
    (.sseMinsdRR 0#4 1#4, 434),
    (.sseMovupsRR 1#4 0#4, 437),
    (.sseMinsdRR 0#4 3#4, 441),
    (.ssePorRR 1#4 0#4, 445) ]

private def init512BlkH03 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 0#4 2#4, 448),
    (.sseMinsdRR 0#4 1#4, 452),
    (.sseMovupsRR 1#4 0#4, 455),
    (.sseMinsdRR 1#4 2#4, 459),
    (.ssePorRR 0#4 1#4, 463) ]

private def init512BlkH45 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 32), 469),
    (.sseMovsdLoad 2#4 (.baseIdxDisp (some 4#4) none 40), 475),
    (.sseMovupsRR 3#4 1#4, 478),
    (.sseMinsdRR 1#4 2#4, 482),
    (.sseMovupsRR 2#4 1#4, 485),
    (.sseMinsdRR 1#4 3#4, 489),
    (.ssePorRR 2#4 1#4, 493) ]

private def init512BlkH67 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovsdLoad 1#4 (.baseIdxDisp (some 4#4) none 48), 499),
    (.sseMovsdLoad 3#4 (.baseIdxDisp (some 4#4) none 56), 505),
    (.sseMovupsRR 4#4 1#4, 508),
    (.sseMinsdRR 1#4 3#4, 512),
    (.sseMovupsRR 3#4 1#4, 515),
    (.sseMinsdRR 1#4 4#4, 519),
    (.ssePorRR 1#4 3#4, 523) ]

private def init512BlkH47 : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 3#4 2#4, 526),
    (.sseMinsdRR 2#4 1#4, 530),
    (.sseMovupsRR 1#4 2#4, 533),
    (.sseMinsdRR 2#4 3#4, 537),
    (.ssePorRR 2#4 1#4, 541) ]

private def init512BlkHFin : List (Amd64VecInsn × Nat) :=
  [ (.sseMovupsRR 1#4 0#4, 544),
    (.sseMinsdRR 1#4 2#4, 548),
    (.sseMovupsRR 2#4 1#4, 551),
    (.sseMinsdRR 1#4 0#4, 555),
    (.ssePorRR 1#4 2#4, 559),
    (.sseMovupsRR 0#4 1#4, 562),
    (.addImm64 4#4 0x40#64, 566),
    (.popReg 5#4, 567),
    (.ret, 568) ]

set_option maxRecDepth 4096 in
private theorem init512BlkPro0_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 0 init512BlkPro0 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkPro1a_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 32 init512BlkPro1a = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkPro1b_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 41 init512BlkPro1b = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkPro1c_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 50 init512BlkPro1c = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkPro2_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 59 init512BlkPro2 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16Hdr_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 240 init512BlkLoop16Hdr = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16Exit_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 253 init512BlkLoop16Exit = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16BodyA_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 115 init512BlkLoop16BodyA = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16BodyB1_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 173 init512BlkLoop16BodyB1 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16BodyB2_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 179 init512BlkLoop16BodyB2 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop16BodyC_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 199 init512BlkLoop16BodyC = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop8Hdr_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 313 init512BlkLoop8Hdr = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop8BodyA_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 255 init512BlkLoop8BodyA = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop8BodyB_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 285 init512BlkLoop8BodyB = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkLoop8BodyC_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 291 init512BlkLoop8BodyC = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkEpiCmp_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 325 init512BlkEpiCmp = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkRetNaN_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 358 init512BlkRetNaN = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH01_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 383 init512BlkH01 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH23_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 415 init512BlkH23 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH03_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 445 init512BlkH03 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH45_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 463 init512BlkH45 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH67_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 493 init512BlkH67 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkH47_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 523 init512BlkH47 = true := by decide
set_option maxRecDepth 4096 in
private theorem init512BlkHFin_ok : checkBlockSeq initReducedRow64AVX512ByteChunks initReducedRow64AVX512Rodata 541 init512BlkHFin = true := by decide

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_prologue (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenCost lenV : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hPosC : 0 < lenCost) (hPosV : 0 < lenV)
    (hN : n + 32 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV : lenV < 2 ^ 64)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64) :
    let s1 := runAmd64Vec 26 initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
    s1.pc = 240 ∧ InitReduced512Inv dstBase costBase vBase (s0.gpr 4#4 - 72#64) (s0.gpr 5#4) c v dst0 ui n 0 s1 := by
  let sp0 := s0.gpr 4#4
  let spF := sp0 - 72#64
  have hSub72 : sp0 - 8#64 - 0x40#64 = spF := by dsimp only [spF]; bv_omega
  have hSp64 : spF + 64#64 = sp0 - 8#64 := by dsimp only [spF]; bv_omega
  have hSp80 : spF + 80#64 = sp0 + 8#64 := by dsimp only [spF]; bv_omega
  have hSp104 : spF + 104#64 = sp0 + 32#64 := by dsimp only [spF]; bv_omega
  have hSp128 : spF + 128#64 = sp0 + 56#64 := by dsimp only [spF]; bv_omega
  have hRbpNe8 : sp0 - 8#64 ≠ sp0 + 8#64 := by bv_omega
  have hRbpNe32 : sp0 - 8#64 ≠ sp0 + 32#64 := by bv_omega
  have hRbpNe56 : sp0 - 8#64 ≠ sp0 + 56#64 := by bv_omega
  have hBeN := evalX86Cond_be_test64_self hPos (by omega)
  have hBeC := evalX86Cond_be_test64_self hPosC hLenC
  have hBeV := evalX86Cond_be_test64_self hPosV hLenV
  have hRo63 : initReducedRow64AVX512Rodata 63 = some f64PosInf := rfl
  have hJmp : nextTargetPc 115 125 = some 240 := by decide
  have hEqInf : f64Eq f64PosInf f64PosInf = true := by decide
  have hOfMaskTrue : Vec512.ofOpmask8 0xff#8 = maskVec8 true true true true true true true true := by decide
  dsimp only [initReducedRow64AVX512Bytes]
  let sA := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0 init512BlkPro0
  have hS0 : runAmd64Vec 26 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0 =
      runAmd64Vec 19 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA :=
    runAmd64Vec_block_seq 19 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0 0 init512BlkPro0 hRun hPc init512BlkPro0_ok
  rw [hS0]
  have hARun : sA.status = .running := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hRun]
  have hAPc : sA.pc = 32 := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hAHaz : sA.sseHazard = false := by
    simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, Amd64VecInsn.isLegacySSE, hHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRax]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRbx]
  have hARsp : sA.gpr 4#4 = spF := by
    simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, sp0, hSub72]
  have hARsi : sA.gpr 6#4 = BitVec.ofNat 64 lenCost := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRsi]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hR9]
  have hAR10 : sA.gpr 10#4 = BitVec.ofNat 64 lenV := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hR10]
  have hAZ0 : (sA.zmm 0#5).l0 = ui := by simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hUi]
  have hAMemEq : sA.mem =
      write64 (write64 (write64 (write64 s0.mem (sp0 - 8#64) (s0.gpr 5#4))
        (sp0 + 8#64) dstBase) (sp0 + 32#64) costBase) (sp0 + 56#64) vBase := by
    simp [sA, init512BlkPro0, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hRax, hRdi, hR9, sp0, hSub72, hSp80, hSp104, hSp128]
  let sB1 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA init512BlkPro1a
  have hS1a : runAmd64Vec 19 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA =
      runAmd64Vec 17 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 :=
    runAmd64Vec_block_seq 17 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA 32 init512BlkPro1a hARun hAPc init512BlkPro1a_ok
  rw [hS1a]
  have hB1Run : sB1.status = .running := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN, hARun]
  have hB1Pc : sB1.pc = 41 := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Haz : sB1.sseHazard = false := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hARbx, hBeN, hAHaz]
  have hB1Gpr : sB1.gpr = sA.gpr := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Zmm : sB1.zmm = sA.zmm := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  have hB1Mem : sB1.mem = sA.mem := by simp [sB1, init512BlkPro1a, execDecodedSeq, execAmd64VecInsn, hARbx, hBeN]
  let sB2 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 init512BlkPro1b
  have hS1b : runAmd64Vec 17 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 =
      runAmd64Vec 15 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 :=
    runAmd64Vec_block_seq 15 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 41 init512BlkPro1b hB1Run hB1Pc init512BlkPro1b_ok
  rw [hS1b]
  have hB2Run : sB2.status = .running := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Run]
  have hB2Pc : sB2.pc = 50 := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC]
  have hB2Haz : sB2.sseHazard = false := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hB1Gpr, hARsi, hBeC, hB1Haz]
  have hB2Gpr : sB2.gpr = sA.gpr := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC]
  have hB2Zmm : sB2.zmm = sA.zmm := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Zmm]
  have hB2Mem : sB2.mem = sA.mem := by simp [sB2, init512BlkPro1b, execDecodedSeq, execAmd64VecInsn, hB1Gpr, hARsi, hBeC, hB1Mem]
  let sB := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 init512BlkPro1c
  have hS1c : runAmd64Vec 15 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 =
      runAmd64Vec 13 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB :=
    runAmd64Vec_block_seq 13 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 50 init512BlkPro1c hB2Run hB2Pc init512BlkPro1c_ok
  rw [hS1c]
  have hBRun : sB.status = .running := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Run]
  have hBPc : sB.pc = 59 := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV]
  have hBHaz : sB.sseHazard = false := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, hB2Gpr, hAR10, hBeV, hB2Haz]
  have hBGpr : sB.gpr = sA.gpr := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV]
  have hBZmm : sB.zmm = sA.zmm := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Zmm]
  have hBMem : sB.mem = sA.mem := by simp [sB, init512BlkPro1c, execDecodedSeq, execAmd64VecInsn, hB2Gpr, hAR10, hBeV, hB2Mem]
  have hS2 : runAmd64Vec 13 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB =
      runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
        (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB init512BlkPro2) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB 59 init512BlkPro2 hBRun hBPc init512BlkPro2_ok
  rw [hS2]
  simp only [runAmd64Vec]
  have hMemFinal :
      (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB init512BlkPro2).mem = sA.mem := by
    simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp, hBMem]
  have hPres : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      sA.mem (elemAddr base j) = s0.mem (elemAddr base j) := by
    intro j hj base hIn
    rcases hDisjPro j hj base hIn with ⟨hd0, hd1, hd2, hd3⟩
    rw [hAMemEq, write64_ne _ _ hd3, write64_ne _ _ hd2, write64_ne _ _ hd1, write64_ne _ _ hd0]
  refine ⟨by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp],
    ⟨by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, hRo63, hJmp, hBRun],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, Amd64VecInsn.dirtiesUpper, hRo63, hJmp],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, Amd64VecInsn.isLegacySSE, hRo63, hJmp, hBHaz],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARax],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARbx],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARsp, spF, sp0],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hARdi],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setGpr, hRo63, hJmp, hBGpr, hAR9],
     by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
          Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, hRo63, hJmp, hBZmm, hAZ0],
     by intro j hj; rw [hMemFinal, hPres j hj costBase (by simp)]; exact hMemC j hj,
     by intro j hj; rw [hMemFinal, hPres j hj vBase (by simp)]; exact hMemV j hj,
     ⟨fun _ h => by omega, fun j _ hj => by rw [hMemFinal, hPres j hj dstBase (by simp)]; exact hMemD j hj⟩,
     by rw [hMemFinal, hSp64, hAMemEq, write64_ne _ _ hRbpNe56, write64_ne _ _ hRbpNe32,
          write64_ne _ _ hRbpNe8, write64_eq],
     ⟨true, true, true, true, true, true, true, true, true, true, true, true, true, true, true, true,
      by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm, setKmask,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, x86VcmppdPred, hEqInf, hRo63, hJmp, hOfMaskTrue],
      by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm, setKmask,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, x86VcmppdPred, hEqInf, hRo63, hJmp, hOfMaskTrue],
      rfl⟩,
     fun _ => ⟨f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
      f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf, f64PosInf,
      by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, hRo63, hJmp],
      by simp [init512BlkPro2, execDecodedSeq, execAmd64VecInsn, readMem64, evalVecRm, setZmm,
           Vec512.applyEvexMask, Vec512.applyWidth, Vec512.broadcast, hRo63, hJmp],
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN, f64PosInf_notNaN,
      by simp [specInitReducedMin, cmin64_posInf_posInf]⟩⟩⟩

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_loop16_step (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 240) (hInv : InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hStep : i + 16 ≤ n) (hN : n + 32 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 64#64 ≠ elemAddr dstBase j) :
    let s' := runAmd64Vec 27 initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
    s'.pc = 240 ∧ InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 16) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJcc : nextTargetPc 253 (-138) = some 115 := by decide
  dsimp only [initReducedRow64AVX512Bytes]
  -- Step 1: Loop16Hdr (3 steps: 240 -> 115).
  let sH := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s init512BlkLoop16Hdr
  have hS0 : runAmd64Vec 27 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s =
      runAmd64Vec 24 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH :=
    runAmd64Vec_block_seq 24 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s 240 init512BlkLoop16Hdr hInv.statusRun hPc init512BlkLoop16Hdr_ok
  rw [hS0]
  have hHRun : sH.status = .running := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.statusRun]
  have hHPc : sH.pc = 115 := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHDirt : sH.upperDirty = true := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.upperDirtyT]
  have hHHaz : sH.sseHazard = false := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc, hInv.noHazard]
  have hHRax : sH.gpr 0#4 = dstBase := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHRcx : sH.gpr 1#4 = BitVec.ofNat 64 i := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHRdx : sH.gpr 2#4 = BitVec.ofNat 64 (i + 16) := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHRbx : sH.gpr 3#4 = BitVec.ofNat 64 n := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHRsp : sH.gpr 4#4 = spF := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHRdi : sH.gpr 7#4 = costBase := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHR9 : sH.gpr 9#4 = vBase := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHZmm : sH.zmm = s.zmm := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  have hHMem : sH.mem = s.mem := by simp [sH, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJcc]
  -- Step 2: Loop16BodyA (12 steps: 115 -> 173, 0 writes).
  let sA := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH init512BlkLoop16BodyA
  have hSA : runAmd64Vec 24 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH =
      runAmd64Vec 12 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA :=
    runAmd64Vec_block_seq 12 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH 115 init512BlkLoop16BodyA hHRun hHPc init512BlkLoop16BodyA_ok
  rw [hSA]
  have hCBase : costBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr costBase i := by unfold elemAddr; simp
  have hVBase : vBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr vBase i := by unfold elemAddr; simp
  have hDBase : dstBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr dstBase i := by unfold elemAddr; simp
  have hC0 := hInv.memCost i (by omega); have hC1 := hInv.memCost (i + 1) (by omega)
  have hC2 := hInv.memCost (i + 2) (by omega); have hC3 := hInv.memCost (i + 3) (by omega)
  have hC4 := hInv.memCost (i + 4) (by omega); have hC5 := hInv.memCost (i + 5) (by omega)
  have hC6 := hInv.memCost (i + 6) (by omega); have hC7 := hInv.memCost (i + 7) (by omega)
  have hC8 := hInv.memCost (i + 8) (by omega); have hC9 := hInv.memCost (i + 9) (by omega)
  have hC10 := hInv.memCost (i + 10) (by omega); have hC11 := hInv.memCost (i + 11) (by omega)
  have hC12 := hInv.memCost (i + 12) (by omega); have hC13 := hInv.memCost (i + 13) (by omega)
  have hC14 := hInv.memCost (i + 14) (by omega); have hC15 := hInv.memCost (i + 15) (by omega)
  have hV0 := hInv.memV i (by omega); have hV1 := hInv.memV (i + 1) (by omega)
  have hV2 := hInv.memV (i + 2) (by omega); have hV3 := hInv.memV (i + 3) (by omega)
  have hV4 := hInv.memV (i + 4) (by omega); have hV5 := hInv.memV (i + 5) (by omega)
  have hV6 := hInv.memV (i + 6) (by omega); have hV7 := hInv.memV (i + 7) (by omega)
  have hV8 := hInv.memV (i + 8) (by omega); have hV9 := hInv.memV (i + 9) (by omega)
  have hV10 := hInv.memV (i + 10) (by omega); have hV11 := hInv.memV (i + 11) (by omega)
  have hV12 := hInv.memV (i + 12) (by omega); have hV13 := hInv.memV (i + 13) (by omega)
  have hV14 := hInv.memV (i + 14) (by omega); have hV15 := hInv.memV (i + 15) (by omega)
  let vd0 : Vec512 :=
    { l0 := specReducedElem c v ui i, l1 := specReducedElem c v ui (i + 1)
      l2 := specReducedElem c v ui (i + 2), l3 := specReducedElem c v ui (i + 3)
      l4 := specReducedElem c v ui (i + 4), l5 := specReducedElem c v ui (i + 5)
      l6 := specReducedElem c v ui (i + 6), l7 := specReducedElem c v ui (i + 7) }
  let vd1 : Vec512 :=
    { l0 := specReducedElem c v ui (i + 8), l1 := specReducedElem c v ui (i + 9)
      l2 := specReducedElem c v ui (i + 10), l3 := specReducedElem c v ui (i + 11)
      l4 := specReducedElem c v ui (i + 12), l5 := specReducedElem c v ui (i + 13)
      l6 := specReducedElem c v ui (i + 14), l7 := specReducedElem c v ui (i + 15) }
  have hARun : sA.status = .running := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHRun]
  have hAPc : sA.pc = 173 := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = true := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHDirt]
  have hAHaz : sA.sseHazard = false := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hHHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax]
  have hARcx : sA.gpr 1#4 = BitVec.ofNat 64 i := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRcx]
  have hARdx : sA.gpr 2#4 = BitVec.ofNat 64 (i + 16) := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdx]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRbx]
  have hARsp : sA.gpr 4#4 = spF := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRsp]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHR9]
  have hAR10 : sA.gpr 10#4 = elemAddr dstBase i := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax, hHRcx, hDBase]
  have hAMem : sA.mem = s.mem := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHMem]
  have hAZ0 : sA.zmm 0#5 = Vec512.broadcast ui := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm, hInv.zmm0Eq]
  have hAZ1 : sA.zmm 1#5 = s.zmm 1#5 := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ2 : sA.zmm 2#5 = s.zmm 2#5 := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ3 : sA.zmm 3#5 = s.zmm 3#5 := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ4 : sA.zmm 4#5 = s.zmm 4#5 := by simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ5 : sA.zmm 5#5 = vd0 := by
    simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHZmm, hHMem, hInv.zmm0Eq, hCBase, hVBase,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24, elemAddr_plus_32, elemAddr_plus_40,
      elemAddr_plus_48, elemAddr_plus_56,
      hC0, hC1, hC2, hC3, hC4, hC5, hC6, hC7, hV0, hV1, hV2, hV3, hV4, hV5, hV6, hV7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, vd0, specReducedElem]
  have hAZ6 : sA.zmm 6#5 =
      { l0 := x86F64Sub (c (i + 8)) (v (i + 8)), l1 := x86F64Sub (c (i + 9)) (v (i + 9))
        l2 := x86F64Sub (c (i + 10)) (v (i + 10)), l3 := x86F64Sub (c (i + 11)) (v (i + 11))
        l4 := x86F64Sub (c (i + 12)) (v (i + 12)), l5 := x86F64Sub (c (i + 13)) (v (i + 13))
        l6 := x86F64Sub (c (i + 14)) (v (i + 14)), l7 := x86F64Sub (c (i + 15)) (v (i + 15)) } := by
    simp [sA, init512BlkLoop16BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHMem, hCBase, hVBase, elemAddr_plus_64,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24, elemAddr_plus_32, elemAddr_plus_40,
      elemAddr_plus_48, elemAddr_plus_56,
      hC8, hC9, hC10, hC11, hC12, hC13, hC14, hC15, hV8, hV9, hV10, hV11, hV12, hV13, hV14, hV15,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.zipWith]
  -- Step 3: Loop16BodyB1 (1 step: 173 -> 179, store vd0 to dst[i..i+7]).
  let sB1 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA init512BlkLoop16BodyB1
  have hSB1 : runAmd64Vec 12 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA =
      runAmd64Vec 11 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 :=
    runAmd64Vec_block_seq 11 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA 173 init512BlkLoop16BodyB1 hARun hAPc init512BlkLoop16BodyB1_ok
  rw [hSB1]
  have hB1Run : sB1.status = .running := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hARun]
  have hB1Pc : sB1.pc = 179 := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1Dirt : sB1.upperDirty = true := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hADirt]
  have hB1Haz : sB1.sseHazard = false := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, hAHaz]
  have hB1Gpr : sB1.gpr = sA.gpr := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1Zmm : sB1.zmm = sA.zmm := by simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hB1MemEq : sB1.mem = storeVecMem s.mem .zmm512 (elemAddr dstBase i) vd0 0#3 (sA.kmask 0#3) := by
    simp [sB1, init512BlkLoop16BodyB1, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hAR10, hAZ5, hAMem]
  have hB1Dst : DstMemState dstBase c v dst0 ui n (i + 8) sB1.mem := by
    rw [hB1MemEq]; exact dstMemState_store8 hInv.memDst (by omega) (by omega) rfl rfl rfl rfl rfl rfl rfl rfl
  -- Step 4: Loop16BodyB2 (4 steps: 179 -> 199, vd1 -= ui and store vd1 to dst[i+8..i+15]).
  let sB2 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 init512BlkLoop16BodyB2
  have hSB2 : runAmd64Vec 11 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 =
      runAmd64Vec 7 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 :=
    runAmd64Vec_block_seq 7 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB1 179 init512BlkLoop16BodyB2 hB1Run hB1Pc init512BlkLoop16BodyB2_ok
  rw [hSB2]
  have hB2Run : sB2.status = .running := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hB1Run]
  have hB2Pc : sB2.pc = 199 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hB2Dirt : sB2.upperDirty = true := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hB1Dirt]
  have hB2Haz : sB2.sseHazard = false := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hB1Haz]
  have hB2Rax : sB2.gpr 0#4 = dstBase := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARax]
  have hB2Rdx : sB2.gpr 2#4 = BitVec.ofNat 64 (i + 16) := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARdx]
  have hB2Rbx : sB2.gpr 3#4 = BitVec.ofNat 64 n := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARbx]
  have hB2Rsp : sB2.gpr 4#4 = spF := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARsp]
  have hB2Rdi : sB2.gpr 7#4 = costBase := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hARdi]
  have hB2R9 : sB2.gpr 9#4 = vBase := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hB1Gpr, hAR9]
  have hB2Z0 : sB2.zmm 0#5 = Vec512.broadcast ui := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ0]
  have hB2Z1 : sB2.zmm 1#5 = s.zmm 1#5 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ1]
  have hB2Z2 : sB2.zmm 2#5 = s.zmm 2#5 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ2]
  have hB2Z3 : sB2.zmm 3#5 = s.zmm 3#5 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ3]
  have hB2Z4 : sB2.zmm 4#5 = s.zmm 4#5 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ4]
  have hB2Z5 : sB2.zmm 5#5 = vd0 := by simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hB1Zmm, hAZ5]
  have hB2Z6 : sB2.zmm 6#5 = vd1 := by
    simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm,
      hB1Zmm, hAZ0, hAZ6, applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, vd1, specReducedElem]
  have hB2MemEq : sB2.mem = storeVecMem sB1.mem .zmm512 (elemAddr dstBase (i + 8)) vd1 0#3 (sB1.kmask 0#3) := by
    simp [sB2, init512BlkLoop16BodyB2, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, setZmm,
      hB1Gpr, hB1Zmm, hARax, hARcx, hAZ0, hAZ6, hDBase, elemAddr_plus_64,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, vd1, specReducedElem]
  have hB2Dst : DstMemState dstBase c v dst0 ui n (i + 16) sB2.mem := by
    rw [hB2MemEq]; exact dstMemState_store8 hB1Dst (by omega) (by omega) rfl rfl rfl rfl rfl rfl rfl rfl
  have hB2MemCost : ∀ j < n, sB2.mem (elemAddr costBase j) = c j := by
    intro j hj
    have hAt1 := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_zmm512_at sB1.mem (elemAddr dstBase (i + 8)) vd1 (sB1.kmask 0#3)
    rw [hB2MemEq, hAt2.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjCD j hj _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjCD j hj _ (by omega))]
    exact hInv.memCost j hj
  have hB2MemV : ∀ j < n, sB2.mem (elemAddr vBase j) = v j := by
    intro j hj
    have hAt1 := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_zmm512_at sB1.mem (elemAddr dstBase (i + 8)) vd1 (sB1.kmask 0#3)
    rw [hB2MemEq, hAt2.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjVD j hj _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjVD j hj _ (by omega))]
    exact hInv.memV j hj
  have hB2Rbp : sB2.mem (spF + 64#64) = rbp0 := by
    have hAt1 := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    have hAt2 := storeVecMem_zmm512_at sB1.mem (elemAddr dstBase (i + 8)) vd1 (sB1.kmask 0#3)
    rw [hB2MemEq, hAt2.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjRbp _ (by omega)),
        hB1MemEq, hAt1.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjRbp _ (by omega))]
    exact hInv.savedRbp
  -- Step 5: Loop16BodyC (7 steps: 199 -> 240, EVEX vcmppd + zero-masked vmovdqu64 + vminpd).
  have hSC : runAmd64Vec 7 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 =
      runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
        (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 init512BlkLoop16BodyC) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 199 init512BlkLoop16BodyC hB2Run hB2Pc init512BlkLoop16BodyC_ok
  rw [hSC]
  simp only [runAmd64Vec]
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, m4, m5, m6, m7, p0, p1, p2, p3, p4, p5, p6, p7, hZ2M, hZ3M, hOrdEq⟩
  have hCMem : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB2 init512BlkLoop16BodyC).mem = sB2.mem := by
    simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm]
  refine ⟨by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm],
    ⟨by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hB2Run],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hB2Dirt],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, hB2Haz],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rax],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rdx],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rbx],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rsp],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2Rdi],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hB2R9],
     by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hB2Z0],
     by intro j hj; rw [hCMem]; exact hB2MemCost j hj,
     by intro j hj; rw [hCMem]; exact hB2MemV j hj,
     by rw [hCMem]; exact hB2Dst,
     by rw [hCMem]; exact hB2Rbp,
     ⟨m0 && f64NotNaN (specReducedElem c v ui i),
      m1 && f64NotNaN (specReducedElem c v ui (i + 1)),
      m2 && f64NotNaN (specReducedElem c v ui (i + 2)),
      m3 && f64NotNaN (specReducedElem c v ui (i + 3)),
      m4 && f64NotNaN (specReducedElem c v ui (i + 4)),
      m5 && f64NotNaN (specReducedElem c v ui (i + 5)),
      m6 && f64NotNaN (specReducedElem c v ui (i + 6)),
      m7 && f64NotNaN (specReducedElem c v ui (i + 7)),
      p0 && f64NotNaN (specReducedElem c v ui (i + 8)),
      p1 && f64NotNaN (specReducedElem c v ui (i + 9)),
      p2 && f64NotNaN (specReducedElem c v ui (i + 10)),
      p3 && f64NotNaN (specReducedElem c v ui (i + 11)),
      p4 && f64NotNaN (specReducedElem c v ui (i + 12)),
      p5 && f64NotNaN (specReducedElem c v ui (i + 13)),
      p6 && f64NotNaN (specReducedElem c v ui (i + 14)),
      p7 && f64NotNaN (specReducedElem c v ui (i + 15)),
      by have hK1 := applyEvexMask_zeroing_maskVec8 1#3 (by decide) m0 m1 m2 m3 m4 m5 m6 m7
           (f64NotNaN (specReducedElem c v ui i)) (f64NotNaN (specReducedElem c v ui (i + 1)))
           (f64NotNaN (specReducedElem c v ui (i + 2))) (f64NotNaN (specReducedElem c v ui (i + 3)))
           (f64NotNaN (specReducedElem c v ui (i + 4))) (f64NotNaN (specReducedElem c v ui (i + 5)))
           (f64NotNaN (specReducedElem c v ui (i + 6))) (f64NotNaN (specReducedElem c v ui (i + 7)))
         simp only [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask,
           hB2Z2, hB2Z5, hZ2M, vd0, Vec512.applyWidth, x86VcmppdPred, f64Eq_self_eq_notNaN]
         simpa [packOpmask8, f64NotNaN] using hK1,
      by have hK2 := applyEvexMask_zeroing_maskVec8 2#3 (by decide) p0 p1 p2 p3 p4 p5 p6 p7
           (f64NotNaN (specReducedElem c v ui (i + 8))) (f64NotNaN (specReducedElem c v ui (i + 9)))
           (f64NotNaN (specReducedElem c v ui (i + 10))) (f64NotNaN (specReducedElem c v ui (i + 11)))
           (f64NotNaN (specReducedElem c v ui (i + 12))) (f64NotNaN (specReducedElem c v ui (i + 13)))
           (f64NotNaN (specReducedElem c v ui (i + 14))) (f64NotNaN (specReducedElem c v ui (i + 15)))
         simp only [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask,
           hB2Z3, hB2Z6, hZ3M, vd1, Vec512.applyWidth, x86VcmppdPred, f64Eq_self_eq_notNaN]
         simpa [packOpmask8, f64NotNaN] using hK2,
      by rw [specAllReducedNotNaN_add16, specAllReducedNotNaN_add8, specAllReducedNotNaN_add4, ← hOrdEq]
         haveI : Std.Associative (· && ·) := ⟨Bool.and_assoc⟩
         haveI : Std.Commutative (· && ·) := ⟨Bool.and_comm⟩
         ac_rfl⟩,
     ?_⟩⟩
  intro hAll16
  rw [specAllReducedNotNaN_add16, specAllReducedNotNaN_add8, specAllReducedNotNaN_add4] at hAll16
  simp only [Bool.and_eq_true] at hAll16
  rcases hAll16 with ⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨⟨hAllI, hr0⟩, hr1⟩, hr2⟩, hr3⟩, hr4⟩, hr5⟩, hr6⟩, hr7⟩,
    hr8⟩, hr9⟩, hr10⟩, hr11⟩, hr12⟩, hr13⟩, hr14⟩, hr15⟩
  rcases hInv.minAccs hAllI with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7,
    hZ1, hZ4, ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
  refine ⟨vminpdLane (specReducedElem c v ui i) a0, vminpdLane (specReducedElem c v ui (i + 1)) a1,
    vminpdLane (specReducedElem c v ui (i + 2)) a2, vminpdLane (specReducedElem c v ui (i + 3)) a3,
    vminpdLane (specReducedElem c v ui (i + 4)) a4, vminpdLane (specReducedElem c v ui (i + 5)) a5,
    vminpdLane (specReducedElem c v ui (i + 6)) a6, vminpdLane (specReducedElem c v ui (i + 7)) a7,
    vminpdLane (specReducedElem c v ui (i + 8)) b0, vminpdLane (specReducedElem c v ui (i + 9)) b1,
    vminpdLane (specReducedElem c v ui (i + 10)) b2, vminpdLane (specReducedElem c v ui (i + 11)) b3,
    vminpdLane (specReducedElem c v ui (i + 12)) b4, vminpdLane (specReducedElem c v ui (i + 13)) b5,
    vminpdLane (specReducedElem c v ui (i + 14)) b6, vminpdLane (specReducedElem c v ui (i + 15)) b7,
    by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hB2Z1, hB2Z5, hZ1, vd0, applyEvexMask_zero, Vec512.applyWidth, Vec512.zipWith],
    by simp [init512BlkLoop16BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hB2Z4, hB2Z6, hZ4, vd1, applyEvexMask_zero, Vec512.applyWidth, Vec512.zipWith],
    vminpdLane_notNaN hr0 ha0, vminpdLane_notNaN hr1 ha1, vminpdLane_notNaN hr2 ha2, vminpdLane_notNaN hr3 ha3,
    vminpdLane_notNaN hr4 ha4, vminpdLane_notNaN hr5 ha5, vminpdLane_notNaN hr6 ha6, vminpdLane_notNaN hr7 ha7,
    vminpdLane_notNaN hr8 hb0, vminpdLane_notNaN hr9 hb1, vminpdLane_notNaN hr10 hb2, vminpdLane_notNaN hr11 hb3,
    vminpdLane_notNaN hr12 hb4, vminpdLane_notNaN hr13 hb5, vminpdLane_notNaN hr14 hb6, vminpdLane_notNaN hr15 hb7, ?_⟩
  rw [cmin64_combine8_eq hr0 hr1 hr2 hr3 hr4 hr5 hr6 hr7 ha0 ha1 ha2 ha3 ha4 ha5 ha6 ha7,
      cmin64_combine8_eq hr8 hr9 hr10 hr11 hr12 hr13 hr14 hr15 hb0 hb1 hb2 hb3 hb4 hb5 hb6 hb7,
      specInitReducedMin_add16, specInitReducedMin_add8, specInitReducedMin_add4, ← hCmin]
  ac_rfl

private theorem initReducedRow64AVX512_loop16_iter (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n : Nat) (hN : n + 32 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 64#64 ≠ elemAddr dstBase j) :
    ∀ k i s, i + 16 * k ≤ n → s.pc = 240 →
      InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s →
      let s' := runAmd64Vec (27 * k) initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
      s'.pc = 240 ∧ InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 16 * k) s' := by
  intro k
  induction k with
  | zero => intro i s _ hPc hInv; simpa [runAmd64Vec] using ⟨hPc, hInv⟩
  | succ m ih =>
    intro i s hLe hPc hInv
    have hStep := initReducedRow64AVX512_loop16_step dstBase costBase vBase spF rbp0 c v dst0 ui n i s
      hPc hInv (by omega) hN hDisjCD hDisjVD hDisjRbp
    have hRest := ih (i + 16) _ (by omega) hStep.1 hStep.2
    have hAdd : 27 * (m + 1) = 27 + 27 * m := by omega
    have hIdx : i + 16 + 16 * m = i + 16 * (m + 1) := by omega
    dsimp only
    rw [hAdd, runAmd64Vec_add, ← hIdx]
    exact hRest

private theorem initReducedRow64AVX512_loop16_exit (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 240) (hInv : InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hLt : n < i + 16) (hN : n + 32 < 2 ^ 61) (hi : i ≤ n) :
    let s' := runAmd64Vec 4 initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
    s'.pc = 313 ∧ InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 16))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd16 : BitVec.ofNat 64 i + 16#64 = BitVec.ofNat 64 (i + 16) := bitVec_ofNat_add i 16
  have hJmp : nextTargetPc 255 58 = some 313 := by decide
  dsimp only [initReducedRow64AVX512Bytes]
  let sMid := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s init512BlkLoop16Hdr
  have hS0 : runAmd64Vec 4 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s =
      runAmd64Vec 1 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid :=
    runAmd64Vec_block_seq 1 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s 240 init512BlkLoop16Hdr hInv.statusRun hPc init512BlkLoop16Hdr_ok
  rw [hS0]
  have hMRun : sMid.status = .running := by
    simp [sMid, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
      hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hInv.statusRun]
  have hMPc : sMid.pc = 253 := by
    simp [sMid, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe]
  have hS1 : runAmd64Vec 1 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid =
      runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
        (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid init512BlkLoop16Exit) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid 253 init512BlkLoop16Exit hMRun hMPc init512BlkLoop16Exit_ok
  rw [hS1]
  simp only [runAmd64Vec]
  refine ⟨by simp [init512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp],
    ⟨by simp [init512BlkLoop16Exit, execDecodedSeq, execAmd64VecInsn, hJmp, hMRun],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.upperDirtyT],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE,
          evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.noHazard],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp],
     by simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.zmm0Eq],
     by intro j hj; simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.memCost j hj],
     by intro j hj; simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
          hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp, hInv.memV j hj],
     by have hM : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid init512BlkLoop16Exit).mem = s.mem := by
          simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp]
        rw [hM]; exact hInv.memDst,
     by have hM : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid init512BlkLoop16Exit).mem = s.mem := by
          simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp]
        rw [hM]; exact hInv.savedRbp,
     by have hZ : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid init512BlkLoop16Exit).zmm = s.zmm := by
          simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp]
        rw [hZ]; exact hInv.ordMasks,
     by have hZ : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sMid init512BlkLoop16Exit).zmm = s.zmm := by
          simp [sMid, init512BlkLoop16Exit, init512BlkLoop16Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr,
            hInv.rcxEq, hInv.rbxEq, hAdd16, hGe, hJmp]
        rw [hZ]; exact hInv.minAccs⟩⟩

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_loop8_step (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 313) (hInv : InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hStep : i + 8 ≤ n) (hN : n + 32 < 2 ^ 61)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjRbp : ∀ j < n, spF + 64#64 ≠ elemAddr dstBase j) :
    let s' := runAmd64Vec 15 initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
    s'.pc = 313 ∧ InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n (i + 8) s' := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = true := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_true hStep
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc : nextTargetPc 325 (-70) = some 255 := by decide
  dsimp only [initReducedRow64AVX512Bytes]
  let sH := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s init512BlkLoop8Hdr
  have hS0 : runAmd64Vec 15 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s =
      runAmd64Vec 11 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH :=
    runAmd64Vec_block_seq 11 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s 313 init512BlkLoop8Hdr hInv.statusRun hPc init512BlkLoop8Hdr_ok
  rw [hS0]
  have hHRun : sH.status = .running := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.statusRun]
  have hHPc : sH.pc = 255 := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHDirt : sH.upperDirty = true := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.upperDirtyT]
  have hHHaz : sH.sseHazard = false := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc, hInv.noHazard]
  have hHRax : sH.gpr 0#4 = dstBase := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.raxEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRcx : sH.gpr 1#4 = BitVec.ofNat 64 i := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRdx : sH.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRbx : sH.gpr 3#4 = BitVec.ofNat 64 n := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRsp : sH.gpr 4#4 = spF := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHRdi : sH.gpr 7#4 = costBase := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rdiEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHR9 : sH.gpr 9#4 = vBase := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.r9Eq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHZmm : sH.zmm = s.zmm := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  have hHMem : sH.mem = s.mem := by simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hJcc]
  let sA := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH init512BlkLoop8BodyA
  have hSA : runAmd64Vec 11 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH =
      runAmd64Vec 5 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA :=
    runAmd64Vec_block_seq 5 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH 255 init512BlkLoop8BodyA hHRun hHPc init512BlkLoop8BodyA_ok
  rw [hSA]
  have hCBase : costBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr costBase i := by unfold elemAddr; simp
  have hVBase : vBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr vBase i := by unfold elemAddr; simp
  have hDBase : dstBase + BitVec.ofNat 64 i * 8#64 + 0#64 = elemAddr dstBase i := by unfold elemAddr; simp
  have hC0 := hInv.memCost i (by omega); have hC1 := hInv.memCost (i + 1) (by omega)
  have hC2 := hInv.memCost (i + 2) (by omega); have hC3 := hInv.memCost (i + 3) (by omega)
  have hC4 := hInv.memCost (i + 4) (by omega); have hC5 := hInv.memCost (i + 5) (by omega)
  have hC6 := hInv.memCost (i + 6) (by omega); have hC7 := hInv.memCost (i + 7) (by omega)
  have hV0 := hInv.memV i (by omega); have hV1 := hInv.memV (i + 1) (by omega)
  have hV2 := hInv.memV (i + 2) (by omega); have hV3 := hInv.memV (i + 3) (by omega)
  have hV4 := hInv.memV (i + 4) (by omega); have hV5 := hInv.memV (i + 5) (by omega)
  have hV6 := hInv.memV (i + 6) (by omega); have hV7 := hInv.memV (i + 7) (by omega)
  let vd0 : Vec512 :=
    { l0 := specReducedElem c v ui i, l1 := specReducedElem c v ui (i + 1)
      l2 := specReducedElem c v ui (i + 2), l3 := specReducedElem c v ui (i + 3)
      l4 := specReducedElem c v ui (i + 4), l5 := specReducedElem c v ui (i + 5)
      l6 := specReducedElem c v ui (i + 6), l7 := specReducedElem c v ui (i + 7) }
  have hARun : sA.status = .running := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHRun]
  have hAPc : sA.pc = 285 := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr]
  have hADirt : sA.upperDirty = true := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHDirt]
  have hAHaz : sA.sseHazard = false := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr, hHHaz]
  have hARax : sA.gpr 0#4 = dstBase := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax]
  have hARdx : sA.gpr 2#4 = BitVec.ofNat 64 (i + 8) := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdx]
  have hARbx : sA.gpr 3#4 = BitVec.ofNat 64 n := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRbx]
  have hARsp : sA.gpr 4#4 = spF := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRsp]
  have hARdi : sA.gpr 7#4 = costBase := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRdi]
  have hAR9 : sA.gpr 9#4 = vBase := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHR9]
  have hAR10 : sA.gpr 10#4 = elemAddr dstBase i := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setGpr, hHRax, hHRcx, hDBase]
  have hAMem : sA.mem = s.mem := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, hHMem]
  have hAZ0 : sA.zmm 0#5 = Vec512.broadcast ui := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm, hInv.zmm0Eq]
  have hAZ1 : sA.zmm 1#5 = s.zmm 1#5 := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ2 : sA.zmm 2#5 = s.zmm 2#5 := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ3 : sA.zmm 3#5 = s.zmm 3#5 := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ4 : sA.zmm 4#5 = s.zmm 4#5 := by simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, setZmm, hHZmm]
  have hAZ5 : sA.zmm 5#5 = vd0 := by
    simp [sA, init512BlkLoop8BodyA, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr, loadVecMem,
      setGpr, setZmm, hHRcx, hHRdi, hHR9, hHZmm, hHMem, hInv.zmm0Eq, hCBase, hVBase,
      elemAddr_plus_8, elemAddr_plus_16, elemAddr_plus_24, elemAddr_plus_32, elemAddr_plus_40,
      elemAddr_plus_48, elemAddr_plus_56,
      hC0, hC1, hC2, hC3, hC4, hC5, hC6, hC7, hV0, hV1, hV2, hV3, hV4, hV5, hV6, hV7,
      applyEvexMask_zero, Vec512.applyWidth, Vec512.broadcast, Vec512.zipWith, vd0, specReducedElem]
  let sB := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA init512BlkLoop8BodyB
  have hSB : runAmd64Vec 5 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA =
      runAmd64Vec 4 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB :=
    runAmd64Vec_block_seq 4 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sA 285 init512BlkLoop8BodyB hARun hAPc init512BlkLoop8BodyB_ok
  rw [hSB]
  have hBRun : sB.status = .running := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hARun]
  have hBPc : sB.pc = 291 := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBDirt : sB.upperDirty = true := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hADirt]
  have hBHaz : sB.sseHazard = false := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, hAHaz]
  have hBGpr : sB.gpr = sA.gpr := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBZmm : sB.zmm = sA.zmm := by simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr]
  have hBMemEq : sB.mem = storeVecMem s.mem .zmm512 (elemAddr dstBase i) vd0 0#3 (sA.kmask 0#3) := by
    simp [sB, init512BlkLoop8BodyB, execDecodedSeq, execAmd64VecInsn, evalMemAddr, hAR10, hAZ5, hAMem]
  have hBDst : DstMemState dstBase c v dst0 ui n (i + 8) sB.mem := by
    rw [hBMemEq]; exact dstMemState_store8 hInv.memDst (by omega) (by omega) rfl rfl rfl rfl rfl rfl rfl rfl
  have hBMemCost : ∀ j < n, sB.mem (elemAddr costBase j) = c j := by
    intro j hj
    have hAt := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [hBMemEq, hAt.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjCD j hj _ (by omega))]
    exact hInv.memCost j hj
  have hBMemV : ∀ j < n, sB.mem (elemAddr vBase j) = v j := by
    intro j hj
    have hAt := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [hBMemEq, hAt.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjVD j hj _ (by omega))]
    exact hInv.memV j hj
  have hBRbp : sB.mem (spF + 64#64) = rbp0 := by
    have hAt := storeVecMem_zmm512_at s.mem (elemAddr dstBase i) vd0 (sA.kmask 0#3)
    rw [hBMemEq, hAt.2.2.2.2.2.2.2.2 _ (fun k hk => by rw [elemAddr_add]; exact hDisjRbp _ (by omega))]
    exact hInv.savedRbp
  have hSC : runAmd64Vec 4 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB =
      runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
        (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB init512BlkLoop8BodyC) :=
    runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
      initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB 291 init512BlkLoop8BodyC hBRun hBPc init512BlkLoop8BodyC_ok
  rw [hSC]
  simp only [runAmd64Vec]
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, m4, m5, m6, m7, p0, p1, p2, p3, p4, p5, p6, p7, hZ2M, hZ3M, hOrdEq⟩
  have hCMem : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sB init512BlkLoop8BodyC).mem = sB.mem := by
    simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm]
  refine ⟨by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm],
    ⟨by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hBRun],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, hBDirt],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, hBHaz],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARax],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARdx],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARbx],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARsp],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hARdi],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setGpr, hBGpr, hAR9],
     by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ0],
     by intro j hj; rw [hCMem]; exact hBMemCost j hj,
     by intro j hj; rw [hCMem]; exact hBMemV j hj,
     by rw [hCMem]; exact hBDst,
     by rw [hCMem]; exact hBRbp,
     ⟨m0 && f64NotNaN (specReducedElem c v ui i),
      m1 && f64NotNaN (specReducedElem c v ui (i + 1)),
      m2 && f64NotNaN (specReducedElem c v ui (i + 2)),
      m3 && f64NotNaN (specReducedElem c v ui (i + 3)),
      m4 && f64NotNaN (specReducedElem c v ui (i + 4)),
      m5 && f64NotNaN (specReducedElem c v ui (i + 5)),
      m6 && f64NotNaN (specReducedElem c v ui (i + 6)),
      m7 && f64NotNaN (specReducedElem c v ui (i + 7)),
      p0, p1, p2, p3, p4, p5, p6, p7,
      by have hK1 := applyEvexMask_zeroing_maskVec8 1#3 (by decide) m0 m1 m2 m3 m4 m5 m6 m7
           (f64NotNaN (specReducedElem c v ui i)) (f64NotNaN (specReducedElem c v ui (i + 1)))
           (f64NotNaN (specReducedElem c v ui (i + 2))) (f64NotNaN (specReducedElem c v ui (i + 3)))
           (f64NotNaN (specReducedElem c v ui (i + 4))) (f64NotNaN (specReducedElem c v ui (i + 5)))
           (f64NotNaN (specReducedElem c v ui (i + 6))) (f64NotNaN (specReducedElem c v ui (i + 7)))
         simp only [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, setKmask,
           hBZmm, hAZ2, hAZ5, hZ2M, vd0, Vec512.applyWidth, x86VcmppdPred, f64Eq_self_eq_notNaN]
         simpa [packOpmask8, f64NotNaN] using hK1,
      by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ3, hZ3M],
      by rw [specAllReducedNotNaN_add8, specAllReducedNotNaN_add4, ← hOrdEq]
         haveI : Std.Associative (· && ·) := ⟨Bool.and_assoc⟩
         haveI : Std.Commutative (· && ·) := ⟨Bool.and_comm⟩
         ac_rfl⟩,
     ?_⟩⟩
  intro hAll8
  rw [specAllReducedNotNaN_add8, specAllReducedNotNaN_add4] at hAll8
  simp only [Bool.and_eq_true] at hAll8
  rcases hAll8 with ⟨⟨⟨⟨⟨⟨⟨⟨hAllI, hr0⟩, hr1⟩, hr2⟩, hr3⟩, hr4⟩, hr5⟩, hr6⟩, hr7⟩
  rcases hInv.minAccs hAllI with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7,
    hZ1, hZ4, ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
  refine ⟨vminpdLane a0 (specReducedElem c v ui i), vminpdLane a1 (specReducedElem c v ui (i + 1)),
    vminpdLane a2 (specReducedElem c v ui (i + 2)), vminpdLane a3 (specReducedElem c v ui (i + 3)),
    vminpdLane a4 (specReducedElem c v ui (i + 4)), vminpdLane a5 (specReducedElem c v ui (i + 5)),
    vminpdLane a6 (specReducedElem c v ui (i + 6)), vminpdLane a7 (specReducedElem c v ui (i + 7)),
    b0, b1, b2, b3, b4, b5, b6, b7,
    by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm,
         hBZmm, hAZ1, hAZ5, hZ1, vd0, applyEvexMask_zero, Vec512.applyWidth, Vec512.zipWith],
    by simp [init512BlkLoop8BodyC, execDecodedSeq, execAmd64VecInsn, evalVecRm, setZmm, hBZmm, hAZ4, hZ4],
    vminpdLane_notNaN ha0 hr0, vminpdLane_notNaN ha1 hr1, vminpdLane_notNaN ha2 hr2, vminpdLane_notNaN ha3 hr3,
    vminpdLane_notNaN ha4 hr4, vminpdLane_notNaN ha5 hr5, vminpdLane_notNaN ha6 hr6, vminpdLane_notNaN ha7 hr7,
    hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, ?_⟩
  rw [cmin64_combine8_eq ha0 ha1 ha2 ha3 ha4 ha5 ha6 ha7 hr0 hr1 hr2 hr3 hr4 hr5 hr6 hr7,
      specInitReducedMin_add8, specInitReducedMin_add4, ← hCmin]
  ac_rfl

set_option maxRecDepth 4096 in
private theorem initReducedRow64AVX512_epilogue (dstBase costBase vBase spF rbp0 : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n i : Nat) (s : Amd64VecState)
    (hPc : s.pc = 313) (hInv : InitReduced512Inv dstBase costBase vBase spF rbp0 c v dst0 ui n i s)
    (hLt : n < i + 8) (hN : n + 32 < 2 ^ 61) (hi : i ≤ n)
    (hDisjSpill : ∀ j < n, ∀ k < 8, elemAddr dstBase j ≠ spF + BitVec.ofNat 64 (k * 8)) :
    ∃ st ≤ 59,
      let sf := runAmd64Vec st initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s
      sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
      DstMemState dstBase c v dst0 ui n i sf.mem ∧
      (specAllReducedNotNaN c v ui i = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
      (specAllReducedNotNaN c v ui i = true →
        f64NotNaN (sf.zmm 0#5).l0 = true ∧ canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui i) ∧
      sf.gpr 5#4 = rbp0 := by
  have hGe : evalX86Cond .ge (x86SubFlags64 (BitVec.ofNat 64 n) (BitVec.ofNat 64 (i + 8))) = false := by
    rw [evalX86Cond_ge_sub64 (by omega) (by omega)]; exact decide_eq_false (by omega)
  have hAdd8 : BitVec.ofNat 64 i + 8#64 = BitVec.ofNat 64 (i + 8) := bitVec_ofNat_add i 8
  have hJcc383 : nextTargetPc 358 25 = some 383 := by decide
  rcases hInv.ordMasks with ⟨m0, m1, m2, m3, m4, m5, m6, m7, p0, p1, p2, p3, p4, p5, p6, p7, hZ2M, hZ3M, hOrdEq⟩
  have hMskEq : evalX86Cond .e
      (x86SubFlags8
        ((((maskVec8 (m0 && p0) (m1 && p1) (m2 && p2) (m3 && p3)
                     (m4 && p4) (m5 && p5) (m6 && p6) (m7 && p7)).toOpmask8).zeroExtend 64).extractLsb' 0 8)
        0xff#8) = specAllReducedNotNaN c v ui i := by
    rw [evalX86Cond_e_sub8, toOpmask8_maskVec8_eq_0xff, hOrdEq]
  let sH := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s init512BlkLoop8Hdr
  have hHRun : sH.status = .running := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.statusRun]
  have hHPc : sH.pc = 325 := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hHHaz : sH.sseHazard = false := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe, hInv.noHazard]
  have hHRsp : sH.gpr 4#4 = spF := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rspEq, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hHZmm : sH.zmm = s.zmm := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  have hHMem : sH.mem = s.mem := by
    simp [sH, init512BlkLoop8Hdr, execDecodedSeq, execAmd64VecInsn, evalMemAddr, setGpr, hInv.rcxEq, hInv.rbxEq, hAdd8, hGe]
  let vMin : Vec512 := Vec512.zipWith vminpdLane (s.zmm 1#5) (s.zmm 4#5)
  let sE := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH init512BlkEpiCmp
  have hEMemEq : sE.mem = storeVecMem s.mem .zmm512 spF vMin 0#3 (sH.kmask 0#3) := by
    simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
      setZmm, setGpr, setKmask, hHRsp, hHZmm, hHMem, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
      vpand_maskVec8, hMskEq, hJcc383, vMin]
    cases specAllReducedNotNaN c v ui i <;> rfl
  have hEAt := storeVecMem_zmm512_at s.mem spF vMin (sH.kmask 0#3)
  have hEDst : DstMemState dstBase c v dst0 ui n i sE.mem := by
    have hPres : ∀ j < n, sE.mem (elemAddr dstBase j) = s.mem (elemAddr dstBase j) := by
      intro j hj
      rw [hEMemEq]
      exact hEAt.2.2.2.2.2.2.2.2 _ (fun k hk => hDisjSpill j hj k hk)
    exact ⟨fun j hj => (hPres j (by omega)).trans (hInv.memDst.1 j hj),
           fun j hge hj => (hPres j hj).trans (hInv.memDst.2 j hge hj)⟩
  have hERbp : sE.mem (spF + 64#64) = rbp0 := by
    rw [hEMemEq]
    refine (hEAt.2.2.2.2.2.2.2.2 _ ?_).trans hInv.savedRbp
    intro k hk
    have hk8 : (BitVec.ofNat 64 (k * 8)).toNat = k * 8 := Nat.mod_eq_of_lt (by omega)
    bv_omega
  cases hOrd : specAllReducedNotNaN c v ui i
  · -- NaN branch: 4 + 7 + 7 = 18 steps.
    refine ⟨18, by omega, ?_⟩
    dsimp only [initReducedRow64AVX512Bytes]
    have hS0 : runAmd64Vec 18 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s =
        runAmd64Vec 14 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH :=
      runAmd64Vec_block_seq 14 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s 313 init512BlkLoop8Hdr hInv.statusRun hPc init512BlkLoop8Hdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 14 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH =
        runAmd64Vec 7 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE :=
      runAmd64Vec_block_seq 7 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH 325 init512BlkEpiCmp hHRun hHPc init512BlkEpiCmp_ok
    rw [hS1]
    have hERun : sE.status = .running := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hHRun]
    have hEPc : sE.pc = 358 := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd]
    have hEHaz : sE.sseHazard = false := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hHHaz]
    have hERsp : sE.gpr 4#4 = spF := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd]
    have hS2 : runAmd64Vec 7 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE =
        runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
          (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE init512BlkRetNaN) :=
      runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE 358 init512BlkRetNaN hERun hEPc init512BlkRetNaN_ok
    rw [hS2]
    simp only [runAmd64Vec]
    have hRetMem : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE init512BlkRetNaN).mem = sE.mem := by
      simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn]
    refine ⟨by simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn],
      by simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper],
      by simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, Amd64VecInsn.dirtiesUpper, hEHaz],
      by rw [hRetMem]; exact hEDst,
      fun _ => by simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn, setGpr, setZmm],
      fun h => Bool.noConfusion h,
      by simp [init512BlkRetNaN, execDecodedSeq, execAmd64VecInsn, setGpr, hERsp, hERbp]⟩
  · -- Ordered branch: 4 + 7 + 8 + 7 + 5 + 7 + 7 + 5 + 9 = 59 steps.
    refine ⟨59, by omega, ?_⟩
    dsimp only [initReducedRow64AVX512Bytes]
    have hS0 : runAmd64Vec 59 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s =
        runAmd64Vec 55 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH :=
      runAmd64Vec_block_seq 55 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s 313 init512BlkLoop8Hdr hInv.statusRun hPc init512BlkLoop8Hdr_ok
    rw [hS0]
    have hS1 : runAmd64Vec 55 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH =
        runAmd64Vec 48 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE :=
      runAmd64Vec_block_seq 48 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sH 325 init512BlkEpiCmp hHRun hHPc init512BlkEpiCmp_ok
    rw [hS1]
    have hERun : sE.status = .running := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hJcc383, hHRun]
    have hEPc : sE.pc = 383 := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hJcc383]
    have hEHaz : sE.sseHazard = false := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.isLegacySSE, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hJcc383, hHHaz]
    have hERsp : sE.gpr 4#4 = spF := by
      simp [sE, init512BlkEpiCmp, execDecodedSeq, execAmd64VecInsn, evalVecRm, evalMemAddr,
        setZmm, setGpr, setKmask, hHRsp, hHZmm, hZ2M, hZ3M, applyEvexMask_zero, Vec512.applyWidth,
        vpand_maskVec8, hMskEq, hOrd, hJcc383]
    rcases hInv.minAccs hOrd with ⟨a0, a1, a2, a3, a4, a5, a6, a7, b0, b1, b2, b3, b4, b5, b6, b7,
      hZ1, hZ4, ha0, ha1, ha2, ha3, ha4, ha5, ha6, ha7, hb0, hb1, hb2, hb3, hb4, hb5, hb6, hb7, hCmin⟩
    let l0 := vminpdLane a0 b0; let l1 := vminpdLane a1 b1
    let l2 := vminpdLane a2 b2; let l3 := vminpdLane a3 b3
    let l4 := vminpdLane a4 b4; let l5 := vminpdLane a5 b5
    let l6 := vminpdLane a6 b6; let l7 := vminpdLane a7 b7
    have hl0 := vminpdLane_notNaN ha0 hb0; have hl1 := vminpdLane_notNaN ha1 hb1
    have hl2 := vminpdLane_notNaN ha2 hb2; have hl3 := vminpdLane_notNaN ha3 hb3
    have hl4 := vminpdLane_notNaN ha4 hb4; have hl5 := vminpdLane_notNaN ha5 hb5
    have hl6 := vminpdLane_notNaN ha6 hb6; have hl7 := vminpdLane_notNaN ha7 hb7
    have hvMinEq : vMin = { l0 := l0, l1 := l1, l2 := l2, l3 := l3, l4 := l4, l5 := l5, l6 := l6, l7 := l7 } := by
      simp [vMin, hZ1, hZ4, Vec512.zipWith, l0, l1, l2, l3, l4, l5, l6, l7]
    have hM0 : sE.mem spF = l0 := by rw [hEMemEq, hEAt.1, hvMinEq]
    have hM1 : sE.mem (spF + 8#64) = l1 := by rw [hEMemEq, hEAt.2.1, hvMinEq]
    have hM2 : sE.mem (spF + 16#64) = l2 := by rw [hEMemEq, hEAt.2.2.1, hvMinEq]
    have hM3 : sE.mem (spF + 24#64) = l3 := by rw [hEMemEq, hEAt.2.2.2.1, hvMinEq]
    have hM4 : sE.mem (spF + 32#64) = l4 := by rw [hEMemEq, hEAt.2.2.2.2.1, hvMinEq]
    have hM5 : sE.mem (spF + 40#64) = l5 := by rw [hEMemEq, hEAt.2.2.2.2.2.1, hvMinEq]
    have hM6 : sE.mem (spF + 48#64) = l6 := by rw [hEMemEq, hEAt.2.2.2.2.2.2.1, hvMinEq]
    have hM7 : sE.mem (spF + 56#64) = l7 := by rw [hEMemEq, hEAt.2.2.2.2.2.2.2.1, hvMinEq]
    have h01 := f64Min_notNaN hl0 hl1
    have h23 := f64Min_notNaN hl2 hl3
    have h03 := f64Min_notNaN h01 h23
    have h45 := f64Min_notNaN hl4 hl5
    have h67 := f64Min_notNaN hl6 hl7
    have h47 := f64Min_notNaN h45 h67
    -- H01 (8 steps: 383 -> 415): xmm2 = f64Min l0 l1.
    let s1 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE init512BlkH01
    have hS2 : runAmd64Vec 48 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE =
        runAmd64Vec 40 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s1 :=
      runAmd64Vec_block_seq 40 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets sE 383 init512BlkH01 hERun hEPc init512BlkH01_ok
    rw [hS2]
    have h1Run : s1.status = .running := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, hERun]
    have h1Pc : s1.pc = 415 := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h1Dirt : s1.upperDirty = false := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper]
    have h1Haz : s1.sseHazard = false := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, hEHaz]
    have h1Rsp : s1.gpr 4#4 = spF := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, hERsp]
    have h1Mem : s1.mem = sE.mem := by simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h1Z2 : (s1.zmm 2#5).l0 = f64Min l0 l1 := by
      simp [s1, init512BlkH01, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        hERsp, hM0, hM1, minsd_por_eq_f64Min l0 l1 hl0 hl1]
    -- H23 (7 steps: 415 -> 445): xmm1 = f64Min l2 l3.
    let s2 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s1 init512BlkH23
    have hS3 : runAmd64Vec 40 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s1 =
        runAmd64Vec 33 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s2 :=
      runAmd64Vec_block_seq 33 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s1 415 init512BlkH23 h1Run h1Pc init512BlkH23_ok
    rw [hS3]
    have h2Run : s2.status = .running := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Run]
    have h2Pc : s2.pc = 445 := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h2Dirt : s2.upperDirty = false := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, h1Dirt]
    have h2Haz : s2.sseHazard = false := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h1Dirt, h1Haz]
    have h2Rsp : s2.gpr 4#4 = spF := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Rsp]
    have h2Mem : s2.mem = sE.mem := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h1Mem]
    have h2Z2 : (s2.zmm 2#5).l0 = f64Min l0 l1 := by simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, h1Z2]
    have h2Z1 : (s2.zmm 1#5).l0 = f64Min l2 l3 := by
      simp [s2, init512BlkH23, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        h1Rsp, h1Mem, hM2, hM3, minsd_por_comm_eq_f64Min l2 l3 hl2 hl3]
    -- H03 (5 steps: 445 -> 463): xmm0 = f64Min (f64Min l0 l1) (f64Min l2 l3).
    let s3 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s2 init512BlkH03
    have hS4 : runAmd64Vec 33 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s2 =
        runAmd64Vec 28 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s3 :=
      runAmd64Vec_block_seq 28 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s2 445 init512BlkH03 h2Run h2Pc init512BlkH03_ok
    rw [hS4]
    have h3Run : s3.status = .running := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, h2Run]
    have h3Pc : s3.pc = 463 := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn]
    have h3Dirt : s3.upperDirty = false := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, h2Dirt]
    have h3Haz : s3.sseHazard = false := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h2Dirt, h2Haz]
    have h3Rsp : s3.gpr 4#4 = spF := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, h2Rsp]
    have h3Mem : s3.mem = sE.mem := by simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, h2Mem]
    have h3Z0 : (s3.zmm 0#5).l0 = f64Min (f64Min l0 l1) (f64Min l2 l3) := by
      simp [s3, init512BlkH03, execDecodedSeq, execAmd64VecInsn, setZmm, h2Z2, h2Z1]
      exact minsd_por_comm_eq_f64Min _ _ h01 h23
    -- H45 (7 steps: 463 -> 493): xmm2 = f64Min l4 l5.
    let s4 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s3 init512BlkH45
    have hS5 : runAmd64Vec 28 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s3 =
        runAmd64Vec 21 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s4 :=
      runAmd64Vec_block_seq 21 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s3 463 init512BlkH45 h3Run h3Pc init512BlkH45_ok
    rw [hS5]
    have h4Run : s4.status = .running := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h3Run]
    have h4Pc : s4.pc = 493 := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h4Dirt : s4.upperDirty = false := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, h3Dirt]
    have h4Haz : s4.sseHazard = false := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h3Dirt, h3Haz]
    have h4Rsp : s4.gpr 4#4 = spF := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h3Rsp]
    have h4Mem : s4.mem = sE.mem := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h3Mem]
    have h4Z0 : (s4.zmm 0#5).l0 = f64Min (f64Min l0 l1) (f64Min l2 l3) := by simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, h3Z0]
    have h4Z2 : (s4.zmm 2#5).l0 = f64Min l4 l5 := by
      simp [s4, init512BlkH45, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        h3Rsp, h3Mem, hM4, hM5, minsd_por_comm_eq_f64Min l4 l5 hl4 hl5]
    -- H67 (7 steps: 493 -> 523): xmm1 = f64Min l6 l7.
    let s5 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s4 init512BlkH67
    have hS6 : runAmd64Vec 21 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s4 =
        runAmd64Vec 14 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s5 :=
      runAmd64Vec_block_seq 14 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s4 493 init512BlkH67 h4Run h4Pc init512BlkH67_ok
    rw [hS6]
    have h5Run : s5.status = .running := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h4Run]
    have h5Pc : s5.pc = 523 := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr]
    have h5Dirt : s5.upperDirty = false := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, h4Dirt]
    have h5Haz : s5.sseHazard = false := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h4Dirt, h4Haz]
    have h5Rsp : s5.gpr 4#4 = spF := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h4Rsp]
    have h5Mem : s5.mem = sE.mem := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, h4Mem]
    have h5Z0 : (s5.zmm 0#5).l0 = f64Min (f64Min l0 l1) (f64Min l2 l3) := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, h4Z0]
    have h5Z2 : (s5.zmm 2#5).l0 = f64Min l4 l5 := by simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm, h4Z2]
    have h5Z1 : (s5.zmm 1#5).l0 = f64Min l6 l7 := by
      simp [s5, init512BlkH67, execDecodedSeq, execAmd64VecInsn, readMem64, evalMemAddr, setZmm,
        h4Rsp, h4Mem, hM6, hM7, minsd_por_eq_f64Min l6 l7 hl6 hl7]
    -- H47 (5 steps: 523 -> 541): xmm2 = f64Min (f64Min l4 l5) (f64Min l6 l7).
    let s6 := execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s5 init512BlkH47
    have hS7 : runAmd64Vec 14 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s5 =
        runAmd64Vec 9 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 :=
      runAmd64Vec_block_seq 9 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s5 523 init512BlkH47 h5Run h5Pc init512BlkH47_ok
    rw [hS7]
    have h6Run : s6.status = .running := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, h5Run]
    have h6Pc : s6.pc = 541 := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn]
    have h6Dirt : s6.upperDirty = false := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, h5Dirt]
    have h6Haz : s6.sseHazard = false := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, Amd64VecInsn.isLegacySSE, h5Dirt, h5Haz]
    have h6Rsp : s6.gpr 4#4 = spF := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, h5Rsp]
    have h6Mem : s6.mem = sE.mem := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, h5Mem]
    have h6Z0 : (s6.zmm 0#5).l0 = f64Min (f64Min l0 l1) (f64Min l2 l3) := by simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, setZmm, h5Z0]
    have h6Z2 : (s6.zmm 2#5).l0 = f64Min (f64Min l4 l5) (f64Min l6 l7) := by
      simp [s6, init512BlkH47, execDecodedSeq, execAmd64VecInsn, setZmm, h5Z2, h5Z1]
      exact minsd_por_eq_f64Min _ _ h45 h67
    -- HFin (9 steps: 541 -> ret): xmm0 = f64Min t03 t47.
    have hS8 : runAmd64Vec 9 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 =
        runAmd64Vec 0 ⟨initReducedRow64AVX512ByteChunks.flatten⟩ initReducedRow64AVX512Rodata initReducedRow64AVX512Targets
          (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 init512BlkHFin) :=
      runAmd64Vec_block_seq 0 initReducedRow64AVX512ByteChunks initReducedRow64AVX512_chunks_wf
        initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 541 init512BlkHFin h6Run h6Pc init512BlkHFin_ok
    rw [hS8]
    simp only [runAmd64Vec]
    have hFinMem : (execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 init512BlkHFin).mem = sE.mem := by
      simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn, h6Mem]
    have hTree := cmin64_f64Min_tree8 hl0 hl1 hl2 hl3 hl4 hl5 hl6 hl7
    have hZ0Fin : ((execDecodedSeq initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s6 init512BlkHFin).zmm 0#5).l0 =
        f64Min (f64Min (f64Min l0 l1) (f64Min l2 l3)) (f64Min (f64Min l4 l5) (f64Min l6 l7)) := by
      simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn, setZmm, h6Z0, h6Z2]
      exact minsd_por_eq_f64Min _ _ h03 h47
    refine ⟨by simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn],
      by simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper, h6Dirt],
      by simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn, Amd64VecInsn.dirtiesUpper,
           Amd64VecInsn.isLegacySSE, h6Dirt, h6Haz],
      by rw [hFinMem]; exact hEDst,
      fun h => Bool.noConfusion h,
      fun _ => ?_,
      by simp [init512BlkHFin, execDecodedSeq, execAmd64VecInsn, setGpr, h6Rsp, h6Mem, hERbp]⟩
    rw [hZ0Fin]
    refine ⟨hTree.1, ?_⟩
    rw [hTree.2]
    exact (cmin64_combine8_eq ha0 ha1 ha2 ha3 ha4 ha5 ha6 ha7 hb0 hb1 hb2 hb3 hb4 hb5 hb6 hb7).trans hCmin

/-- Fuel bound for executing decoded `initReducedRow64AVX512` on any slice length `n`. -/
def initReducedRow64AVX512FuelBound (n : Nat) : Nat :=
  27 * (n / 16) + 104

/-- **Top-Level Decoded Machine-Code Functional Equivalence (`initReducedRow64AVX512`)**:
For arbitrary non-empty 8-byte-aligned slices (`n = 8q + r`, `0 < n`, `n ≤ lenDst`, `n ≤ lenCost`,
`n ≤ lenV`, `n + 32 < 2^61`) and non-aliasing `dst`/`costRow`/`v`/stack memory, running the raw
512-bit EVEX ELF `.text` bytes of `internal/simd.initReducedRow64AVX512`:
1. Terminates cleanly at `RET` with `upperDirty = false` and `sseHazard = false` (all 3
   `runtime.panicBounds` guards are provably unreachable);
2. Writes bit-exact x86-64 IEEE-754 `dst[j] = x86F64Sub (x86F64Sub costRow[j] v[j]) ui` for all
   `j < 8 * (n / 8)` (matching Intel SDM `VSUBPD` NaN quieting and negative indefinite NaN
   `0xfff8000000000000#64`) while leaving tail lanes `8 * (n / 8) ≤ j < n` unmodified;
3. Returns exact quiet-NaN sentinel `0x7ff8000000000001#64` iff any reduced element in `0..8*(n/8)`
   is NaN (verified directly through EVEX `VCMPPD k`, zero-masked `VMOVDQU64 {k}{z}`, `VPMOVQ2M`,
   and `KMOVB`), and otherwise returns a non-NaN minimum whose `canonZero64` equals `specInitReducedMin`;
4. Preserves Go ABI registers: `RSP = RSP0 + 8`, `RBP = RBP0`, `R14 = R14_0`, and `XMM15 = XMM15_0`. -/
theorem initReducedRow64AVX512_functional_equiv (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenDst lenCost lenV fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hLenDst : n ≤ lenDst) (hLenCost : n ≤ lenCost) (hLenV : n ≤ lenV)
    (hN : n + 32 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV64 : lenV < 2 ^ 64)
    (hAlign : dstBase.toNat % 8 = 0 ∧ costBase.toNat % 8 = 0 ∧ vBase.toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64)
    (hDisjSpill : ∀ j < n, ∀ k < 8, elemAddr dstBase j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8))
    (hFuel : initReducedRow64AVX512FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
    sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
    (∀ j < 8 * (n / 8), sf.mem (elemAddr dstBase j) = specReducedElem c v ui j) ∧
    (∀ j, 8 * (n / 8) ≤ j → j < n → sf.mem (elemAddr dstBase j) = dst0 j) ∧
    (specAllReducedNotNaN c v ui (8 * (n / 8)) = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
    (specAllReducedNotNaN c v ui (8 * (n / 8)) = true →
      f64NotNaN (sf.zmm 0#5).l0 = true ∧
      canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui (8 * (n / 8))) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 := by
  have _ := And.intro hLenDst hAlign
  let spF := s0.gpr 4#4 - 72#64
  let rbp0 := s0.gpr 5#4
  have hDisjRbp : ∀ j < n, spF + 64#64 ≠ elemAddr dstBase j := by
    intro j hj
    have hEq : spF + 64#64 = s0.gpr 4#4 - 8#64 := by dsimp only [spF]; bv_omega
    rw [hEq]
    exact (hDisjPro j hj dstBase (by simp)).1.symm
  have hPro := initReducedRow64AVX512_prologue dstBase costBase vBase c v dst0 ui n lenCost lenV s0
    hPc hRun hHaz hPos (by omega) (by omega) hN hLenC hLenV64 hRax hRbx hRdi hRsi hR9 hR10 hUi hMemC hMemV hMemD hDisjPro
  have hL16 := initReducedRow64AVX512_loop16_iter dstBase costBase vBase spF rbp0 c v dst0 ui n hN hDisjCD hDisjVD hDisjRbp
    (n / 16) 0 _ (by omega) hPro.1 hPro.2
  have hEx16 := initReducedRow64AVX512_loop16_exit dstBase costBase vBase spF rbp0 c v dst0 ui n (16 * (n / 16)) _
    hL16.1 (by simpa using hL16.2) (by omega) hN (by omega)
  have hExist : ∃ stTotal ≤ initReducedRow64AVX512FuelBound n,
      let se := runAmd64Vec stTotal initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
      se.status = .returned ∧ se.upperDirty = false ∧ se.sseHazard = false ∧
      DstMemState dstBase c v dst0 ui n (8 * (n / 8)) se.mem ∧
      (specAllReducedNotNaN c v ui (8 * (n / 8)) = false → (se.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
      (specAllReducedNotNaN c v ui (8 * (n / 8)) = true →
        f64NotNaN (se.zmm 0#5).l0 = true ∧
        canonZero64 (se.zmm 0#5).l0 = specInitReducedMin c v ui (8 * (n / 8))) ∧
      se.gpr 5#4 = rbp0 := by
    unfold initReducedRow64AVX512FuelBound
    by_cases hRem : 16 * (n / 16) + 8 ≤ n
    · have h8Eq : 16 * (n / 16) + 8 = 8 * (n / 8) := by omega
      have hL8 := initReducedRow64AVX512_loop8_step dstBase costBase vBase spF rbp0 c v dst0 ui n (16 * (n / 16)) _
        hEx16.1 hEx16.2 hRem hN hDisjCD hDisjVD hDisjRbp
      rcases initReducedRow64AVX512_epilogue dstBase costBase vBase spF rbp0 c v dst0 ui n (8 * (n / 8)) _
        hL8.1 (by simpa [h8Eq] using hL8.2) (by omega) hN (by omega) hDisjSpill with ⟨stE, hStE, hEpi⟩
      refine ⟨26 + 27 * (n / 16) + 4 + 15 + stE, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (26 + 27 * (n / 16) + 4 + 15) stE,
          runAmd64Vec_add (26 + 27 * (n / 16) + 4) 15,
          runAmd64Vec_add (26 + 27 * (n / 16)) 4,
          runAmd64Vec_add 26 (27 * (n / 16))]
      exact hEpi
    · have h8Eq : 16 * (n / 16) = 8 * (n / 8) := by omega
      rcases initReducedRow64AVX512_epilogue dstBase costBase vBase spF rbp0 c v dst0 ui n (8 * (n / 8)) _
        hEx16.1 (by simpa [h8Eq] using hEx16.2) (by omega) hN (by omega) hDisjSpill with ⟨stE, hStE, hEpi⟩
      refine ⟨26 + 27 * (n / 16) + 4 + stE, by omega, ?_⟩
      dsimp only
      rw [runAmd64Vec_add (26 + 27 * (n / 16) + 4) stE,
          runAmd64Vec_add (26 + 27 * (n / 16)) 4,
          runAmd64Vec_add 26 (27 * (n / 16))]
      exact hEpi
  have hAbi := initReducedRow64AVX512_abi_reg_preserved s0 fuel hPc hRun
  rcases hExist with ⟨stTotal, hStLe, hStat, hDirt, hHazF, hDst, hNaN, hOrd, hRbp⟩
  have hSplit : fuel = stTotal + (fuel - stTotal) := by omega
  have hSfStat : (runAmd64Vec fuel initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0).status = .returned := by
    rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])]; exact hStat
  have hRsp := hAbi.2.2.2 hSfStat
  dsimp only
  rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])]
  rw [hSplit, runAmd64Vec_add, runAmd64Vec_of_not_running _ _ _ _ _ (by simp [hStat])] at hAbi hRsp
  exact ⟨hStat, hDirt, hHazF, hDst.1, hDst.2, hNaN, hOrd, hRsp, hRbp, hAbi.1, hAbi.2.1, hAbi.2.2.1⟩

/-- **Cross-ISA Equivalence (`initReducedRow64AVX2` = `initReducedRow64AVX512` on every multiple of 8)**:
For any positive multiple-of-8 8-byte-aligned slice length `n = 8 * q > 0`, running the decoded
256-bit VEX AVX2 and 512-bit EVEX AVX-512 ELF bytes writes bit-identical x86-64 IEEE-754
reduced-cost slices `dst[0..n)`, agrees bit-for-bit on the quiet-NaN sentinel
`0x7ff8000000000001#64` whenever any reduced cost is NaN, and returns canonicalized (`canonZero64`)
equal minima on all NaN-free rows. -/
theorem initReducedRow64_avx2_eq_avx512_mult8 (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (q lenDst lenCost lenV fuel2 fuel512 : Nat)
    (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPosQ : 0 < q) (hLenDst : 8 * q ≤ lenDst) (hLenCost : 8 * q ≤ lenCost) (hLenV : 8 * q ≤ lenV)
    (hN : 8 * q + 32 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV64 : lenV < 2 ^ 64)
    (hAlign : dstBase.toNat % 8 = 0 ∧ costBase.toNat % 8 = 0 ∧ vBase.toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 (8 * q))
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < 8 * q, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < 8 * q, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < 8 * q, s0.mem (elemAddr dstBase j) = dst0 j)
    (hDisjCD : ∀ j1 < 8 * q, ∀ j2 < 8 * q, elemAddr costBase j1 ≠ elemAddr dstBase j2)
    (hDisjVD : ∀ j1 < 8 * q, ∀ j2 < 8 * q, elemAddr vBase j1 ≠ elemAddr dstBase j2)
    (hDisjPro : ∀ j < 8 * q, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64)
    (hDisjSpill2 : ∀ j < 8 * q, ∀ k < 4, elemAddr dstBase j ≠ s0.gpr 4#4 - 40#64 + BitVec.ofNat 64 (k * 8))
    (hDisjSpill512 : ∀ j < 8 * q, ∀ k < 8, elemAddr dstBase j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8))
    (hFuel2 : initReducedRow64AVX2FuelBound (8 * q) ≤ fuel2)
    (hFuel512 : initReducedRow64AVX512FuelBound (8 * q) ≤ fuel512) :
    let sf2 := runAmd64Vec fuel2 initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
    let sf512 := runAmd64Vec fuel512 initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
    (∀ j < 8 * q, sf2.mem (elemAddr dstBase j) = sf512.mem (elemAddr dstBase j)) ∧
    (specAllReducedNotNaN c v ui (8 * q) = false → (sf2.zmm 0#5).l0 = (sf512.zmm 0#5).l0) ∧
    (specAllReducedNotNaN c v ui (8 * q) = true →
      canonZero64 (sf2.zmm 0#5).l0 = canonZero64 (sf512.zmm 0#5).l0) := by
  have h2 := initReducedRow64AVX2_functional_equiv dstBase costBase vBase c v dst0 ui (8 * q) lenDst lenCost lenV fuel2 s0
    hPc hRun hHaz (by omega) hLenDst hLenCost hLenV (by omega) hLenC hLenV64 hAlign hRax hRbx hRdi hRsi hR9 hR10 hUi
    hMemC hMemV hMemD hDisjCD hDisjVD hDisjPro hDisjSpill2 hFuel2
  have h512 := initReducedRow64AVX512_functional_equiv dstBase costBase vBase c v dst0 ui (8 * q) lenDst lenCost lenV fuel512 s0
    hPc hRun hHaz (by omega) hLenDst hLenCost hLenV hN hLenC hLenV64 hAlign hRax hRbx hRdi hRsi hR9 hR10 hUi
    hMemC hMemV hMemD hDisjCD hDisjVD hDisjPro hDisjSpill512 hFuel512
  have hDiv4 : 4 * (8 * q / 4) = 8 * q := by omega
  have hDiv8 : 8 * (8 * q / 8) = 8 * q := by omega
  rw [hDiv4] at h2
  rw [hDiv8] at h512
  refine ⟨fun j hj => (h2.2.2.2.1 j hj).trans (h512.2.2.2.1 j hj).symm,
    fun hF => (h2.2.2.2.2.2.1 hF).trans (h512.2.2.2.2.2.1 hF).symm,
    fun hT => (h2.2.2.2.2.2.2.1 hT).2.trans (h512.2.2.2.2.2.2.1 hT).2.symm⟩

/-! ### REX.B `0x90` Regression & Explicit 8-Byte Alignment / Byte-Range Non-Overlap Corollaries -/

/-- Regression witness: `REX.B` + `0x90` (`0x41 0x90` = `XCHG R8D, EAX`, `0x49 0x90` = `XCHG R8, RAX`)
is rejected (`none`) rather than decoded as `.nop`, while plain `0x90` and `0x66 0x90` remain `.nop`. -/
theorem rexB_0x90_not_nop :
    decodeAmd64VecInsn #[0x41, 0x90] 0 = none ∧
    decodeAmd64VecInsn #[0x49, 0x90] 0 = none ∧
    decodeAmd64VecInsn #[0x90] 0 = some (.nop, 1) ∧
    decodeAmd64VecInsn #[0x66, 0x90] 0 = some (.nop, 2) := by
  decide

/-- Two 64-bit base addresses `a` and `b` occupy disjoint 8-byte physical byte ranges
(`[a, a+8) ∩ [b, b+8) = ∅` modulo `2^64`). -/
def byteRangeDisjoint64 (a b : BitVec 64) : Prop :=
  ∀ o1 < 8, ∀ o2 < 8, a + BitVec.ofNat 64 o1 ≠ b + BitVec.ofNat 64 o2

/-- Any two distinct 8-byte-aligned 64-bit addresses occupy disjoint 8-byte physical byte ranges. -/
theorem byteRangeDisjoint64_of_aligned_ne {a b : BitVec 64}
    (ha : a.toNat % 8 = 0) (hb : b.toNat % 8 = 0) (hne : a ≠ b) :
    byteRangeDisjoint64 a b := by
  intro o1 ho1 o2 ho2 hEq
  have hMod : (a + BitVec.ofNat 64 o1).toNat % 8 = (b + BitVec.ofNat 64 o2).toNat % 8 :=
    congrArg (fun x => x.toNat % 8) hEq
  simp only [BitVec.toNat_add, BitVec.toNat_ofNat] at hMod
  have hoEq : o1 = o2 := by omega
  subst hoEq
  apply hne
  have hSub := congrArg (· - BitVec.ofNat 64 o1) hEq
  simpa using hSub

/-- Physical 8-byte byte-range disjointness implies 64-bit word base address inequality. -/
theorem ne_of_byteRangeDisjoint64 {a b : BitVec 64}
    (h : byteRangeDisjoint64 a b) : a ≠ b := by
  have h0 := h 0 (by omega) 0 (by omega)
  simpa using h0

/-- `elemAddr` preserves 8-byte alignment. -/
theorem elemAddr_aligned {base : BitVec 64} (hb : base.toNat % 8 = 0) (j : Nat) :
    (elemAddr base j).toNat % 8 = 0 := by
  unfold elemAddr
  simp only [BitVec.toNat_add, BitVec.toNat_mul, BitVec.toNat_ofNat]
  omega

/-- Distinct indices within an 8-byte-aligned slice of length `< 2^61` occupy disjoint 8-byte byte ranges. -/
theorem elemAddr_byteRangeDisjoint64_self {base : BitVec 64} (hb : base.toNat % 8 = 0)
    {n j1 j2 : Nat} (hN : n < 2 ^ 61) (hj1 : j1 < n) (hj2 : j2 < n) (hne : j1 ≠ j2) :
    byteRangeDisjoint64 (elemAddr base j1) (elemAddr base j2) := by
  apply byteRangeDisjoint64_of_aligned_ne (elemAddr_aligned hb j1) (elemAddr_aligned hb j2)
  intro hEq
  unfold elemAddr at hEq
  have hSub : BitVec.ofNat 64 j1 * 8#64 = BitVec.ofNat 64 j2 * 8#64 := by
    have h1 := congrArg (fun x => x - base) hEq
    simpa [BitVec.add_comm] using h1
  have hNat := congrArg BitVec.toNat hSub
  simp only [BitVec.toNat_mul, BitVec.toNat_ofNat] at hNat
  omega

/-- `RSP + 8` preserves 8-byte stack alignment. -/
theorem rsp_add8_aligned {rsp : BitVec 64} (h : rsp.toNat % 8 = 0) :
    (rsp + 8#64).toNat % 8 = 0 := by
  simp only [BitVec.toNat_add, BitVec.toNat_ofNat]
  omega

/-- 8-byte aligned and byte-range non-overlapping corollary for `allAbsLE64AVX2`. -/
theorem allAbsLE64AVX2_aligned_byteDisjoint_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 61)
    (hAlign : base.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hByteDisj : ∀ j < n,
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64))
    (hFuel : allAbsLE64AVX2Steps n ≤ fuel) :
    let sf := runAmd64Vec fuel allAbsLE64AVX2Bytes allAbsLE64AVX2Rodata allAbsLE64AVX2Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    sf.gpr 0#4 = (if specAllAbsLE x limit (4 * (n / 4)) then 1#64 else 0#64) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j < n, (elemAddr base j).toNat % 8 = 0) ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr base j1) (elemAddr base j2)) := by
  have hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64 :=
    fun j hj => ⟨ne_of_byteRangeDisjoint64 (hByteDisj j hj).1, ne_of_byteRangeDisjoint64 (hByteDisj j hj).2⟩
  have hEq := allAbsLE64AVX2_functional_equiv base x limit n fuel s0
    hPc hRun hHaz hPos (by omega) hRax hRbx hLim hX hDisj hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2,
    fun j _ => elemAddr_aligned hAlign.1 j,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2

/-- 8-byte aligned and byte-range non-overlapping corollary for `allAbsLE64AVX512`. -/
theorem allAbsLE64AVX512_aligned_byteDisjoint_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (limit : BitVec 64) (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 61)
    (hAlign : base.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hLim : (s0.zmm 0#5).l0 = limit)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hByteDisj : ∀ j < n,
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64))
    (hFuel : allAbsLE64AVX512Steps n ≤ fuel) :
    let sf := runAmd64Vec fuel allAbsLE64AVX512Bytes allAbsLE64AVX512Rodata allAbsLE64AVX512Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    sf.gpr 0#4 = (if specAllAbsLE x limit (8 * (n / 8)) then 1#64 else 0#64) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j < n, (elemAddr base j).toNat % 8 = 0) ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr base j1) (elemAddr base j2)) := by
  have hDisj : ∀ j < n, elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧ elemAddr base j ≠ s0.gpr 4#4 + 8#64 :=
    fun j hj => ⟨ne_of_byteRangeDisjoint64 (hByteDisj j hj).1, ne_of_byteRangeDisjoint64 (hByteDisj j hj).2⟩
  have hEq := allAbsLE64AVX512_functional_equiv base x limit n fuel s0
    hPc hRun hHaz hPos (by omega) hRax hRbx hLim hX hDisj hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2,
    fun j _ => elemAddr_aligned hAlign.1 j,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2

/-- 8-byte aligned and byte-range non-overlapping corollary for `minNoNaN64AVX2`. -/
theorem minNoNaN64AVX2_aligned_byteDisjoint_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hN : n + 16 < 2 ^ 61)
    (hAlign : base.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hByteDisj : ∀ j < n,
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64) ∧
      ∀ k < 8, byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8)))
    (hFuel : minNoNaN64AVX2FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel minNoNaN64AVX2Bytes minNoNaN64AVX2Rodata minNoNaN64AVX2Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    (sf.zmm 0#5).l0 = specMinNoNaN64 x n ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j < n, (elemAddr base j).toNat % 8 = 0) ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr base j1) (elemAddr base j2)) := by
  have hDisj : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 8, elemAddr base j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8) :=
    fun j hj => ⟨ne_of_byteRangeDisjoint64 (hByteDisj j hj).1,
      ne_of_byteRangeDisjoint64 (hByteDisj j hj).2.1,
      fun k hk => ne_of_byteRangeDisjoint64 ((hByteDisj j hj).2.2 k hk)⟩
  have hEq := minNoNaN64AVX2_functional_equiv base x n fuel s0
    hPc hRun hHaz hPos (by omega) hRax hRbx hX hDisj hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2,
    fun j _ => elemAddr_aligned hAlign.1 j,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2

/-- 8-byte aligned and byte-range non-overlapping corollary for `minNoNaN64AVX512`. -/
theorem minNoNaN64AVX512_aligned_byteDisjoint_equiv (base : BitVec 64) (x : Nat → BitVec 64)
    (n fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hGuard : BitVec.ult (s0.mem (s0.gpr 14#4 + 16#64)) (s0.gpr 4#4 - 8#64) = true)
    (hPos : 0 < n) (hN : n + 32 < 2 ^ 61)
    (hAlign : base.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = base) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hX : ∀ j < n, s0.mem (elemAddr base j) = x j)
    (hByteDisj : ∀ j < n,
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64) ∧
      ∀ k < 16, byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 136#64 + BitVec.ofNat 64 (k * 8)))
    (hFuel : minNoNaN64AVX512FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel minNoNaN64AVX512Bytes minNoNaN64AVX512Rodata minNoNaN64AVX512Targets s0
    sf.status = .returned ∧
    sf.upperDirty = false ∧
    sf.sseHazard = false ∧
    (sf.zmm 0#5).l0 = specMinNoNaN64 x n ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j < n, (elemAddr base j).toNat % 8 = 0) ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr base j1) (elemAddr base j2)) := by
  have hDisj : ∀ j < n,
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      ∀ k < 16, elemAddr base j ≠ s0.gpr 4#4 - 136#64 + BitVec.ofNat 64 (k * 8) :=
    fun j hj => ⟨ne_of_byteRangeDisjoint64 (hByteDisj j hj).1,
      ne_of_byteRangeDisjoint64 (hByteDisj j hj).2.1,
      fun k hk => ne_of_byteRangeDisjoint64 ((hByteDisj j hj).2.2 k hk)⟩
  have hEq := minNoNaN64AVX512_functional_equiv base x n fuel s0
    hPc hRun hHaz hGuard hPos (by omega) hRax hRbx hX hDisj hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2,
    fun j _ => elemAddr_aligned hAlign.1 j,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2

/-- 8-byte aligned (including `RSP`) and byte-range non-overlapping corollary for `initReducedRow64AVX2`. -/
theorem initReducedRow64AVX2_aligned_byteDisjoint_equiv (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenDst lenCost lenV fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hLenDst : n ≤ lenDst) (hLenCost : n ≤ lenCost) (hLenV : n ≤ lenV)
    (hN : n + 16 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV64 : lenV < 2 ^ 64)
    (hAlign : dstBase.toNat % 8 = 0 ∧ costBase.toNat % 8 = 0 ∧ vBase.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hByteDisjCD : ∀ j1 < n, ∀ j2 < n, byteRangeDisjoint64 (elemAddr costBase j1) (elemAddr dstBase j2))
    (hByteDisjVD : ∀ j1 < n, ∀ j2 < n, byteRangeDisjoint64 (elemAddr vBase j1) (elemAddr dstBase j2))
    (hByteDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 32#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 56#64))
    (hByteDisjSpill : ∀ j < n, ∀ k < 4,
      byteRangeDisjoint64 (elemAddr dstBase j) (s0.gpr 4#4 - 40#64 + BitVec.ofNat 64 (k * 8)))
    (hFuel : initReducedRow64AVX2FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel initReducedRow64AVX2Bytes initReducedRow64AVX2Rodata initReducedRow64AVX2Targets s0
    sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
    (∀ j < 4 * (n / 4), sf.mem (elemAddr dstBase j) = specReducedElem c v ui j) ∧
    (∀ j, 4 * (n / 4) ≤ j → j < n → sf.mem (elemAddr dstBase j) = dst0 j) ∧
    (specAllReducedNotNaN c v ui (4 * (n / 4)) = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
    (specAllReducedNotNaN c v ui (4 * (n / 4)) = true →
      f64NotNaN (sf.zmm 0#5).l0 = true ∧
      canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui (4 * (n / 4))) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr dstBase j1) (elemAddr dstBase j2)) := by
  have hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2 :=
    fun j1 hj1 j2 hj2 => ne_of_byteRangeDisjoint64 (hByteDisjCD j1 hj1 j2 hj2)
  have hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2 :=
    fun j1 hj1 j2 hj2 => ne_of_byteRangeDisjoint64 (hByteDisjVD j1 hj1 j2 hj2)
  have hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64 :=
    fun j hj b hb => ⟨ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.2.1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.2.2⟩
  have hDisjSpill : ∀ j < n, ∀ k < 4, elemAddr dstBase j ≠ s0.gpr 4#4 - 40#64 + BitVec.ofNat 64 (k * 8) :=
    fun j hj k hk => ne_of_byteRangeDisjoint64 (hByteDisjSpill j hj k hk)
  have hEq := initReducedRow64AVX2_functional_equiv dstBase costBase vBase c v dst0 ui n lenDst lenCost lenV fuel s0
    hPc hRun hHaz hPos hLenDst hLenCost hLenV hN hLenC hLenV64
    ⟨hAlign.1, hAlign.2.1, hAlign.2.2.1⟩ hRax hRbx hRdi hRsi hR9 hR10 hUi
    hMemC hMemV hMemD hDisjCD hDisjVD hDisjPro hDisjSpill hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1,
    hEq.2.2.2.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.2.2,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2.2.2

/-- 8-byte aligned (including `RSP`) and byte-range non-overlapping corollary for `initReducedRow64AVX512`. -/
theorem initReducedRow64AVX512_aligned_byteDisjoint_equiv (dstBase costBase vBase : BitVec 64)
    (c v dst0 : Nat → BitVec 64) (ui : BitVec 64) (n lenDst lenCost lenV fuel : Nat) (s0 : Amd64VecState)
    (hPc : s0.pc = 0) (hRun : s0.status = .running) (hHaz : s0.sseHazard = false)
    (hPos : 0 < n) (hLenDst : n ≤ lenDst) (hLenCost : n ≤ lenCost) (hLenV : n ≤ lenV)
    (hN : n + 32 < 2 ^ 61) (hLenC : lenCost < 2 ^ 64) (hLenV64 : lenV < 2 ^ 64)
    (hAlign : dstBase.toNat % 8 = 0 ∧ costBase.toNat % 8 = 0 ∧ vBase.toNat % 8 = 0 ∧ (s0.gpr 4#4).toNat % 8 = 0)
    (hRax : s0.gpr 0#4 = dstBase) (hRbx : s0.gpr 3#4 = BitVec.ofNat 64 n)
    (hRdi : s0.gpr 7#4 = costBase) (hRsi : s0.gpr 6#4 = BitVec.ofNat 64 lenCost)
    (hR9 : s0.gpr 9#4 = vBase) (hR10 : s0.gpr 10#4 = BitVec.ofNat 64 lenV)
    (hUi : (s0.zmm 0#5).l0 = ui)
    (hMemC : ∀ j < n, s0.mem (elemAddr costBase j) = c j)
    (hMemV : ∀ j < n, s0.mem (elemAddr vBase j) = v j)
    (hMemD : ∀ j < n, s0.mem (elemAddr dstBase j) = dst0 j)
    (hByteDisjCD : ∀ j1 < n, ∀ j2 < n, byteRangeDisjoint64 (elemAddr costBase j1) (elemAddr dstBase j2))
    (hByteDisjVD : ∀ j1 < n, ∀ j2 < n, byteRangeDisjoint64 (elemAddr vBase j1) (elemAddr dstBase j2))
    (hByteDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 - 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 8#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 32#64) ∧
      byteRangeDisjoint64 (elemAddr base j) (s0.gpr 4#4 + 56#64))
    (hByteDisjSpill : ∀ j < n, ∀ k < 8,
      byteRangeDisjoint64 (elemAddr dstBase j) (s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8)))
    (hFuel : initReducedRow64AVX512FuelBound n ≤ fuel) :
    let sf := runAmd64Vec fuel initReducedRow64AVX512Bytes initReducedRow64AVX512Rodata initReducedRow64AVX512Targets s0
    sf.status = .returned ∧ sf.upperDirty = false ∧ sf.sseHazard = false ∧
    (∀ j < 8 * (n / 8), sf.mem (elemAddr dstBase j) = specReducedElem c v ui j) ∧
    (∀ j, 8 * (n / 8) ≤ j → j < n → sf.mem (elemAddr dstBase j) = dst0 j) ∧
    (specAllReducedNotNaN c v ui (8 * (n / 8)) = false → (sf.zmm 0#5).l0 = 0x7ff8000000000001#64) ∧
    (specAllReducedNotNaN c v ui (8 * (n / 8)) = true →
      f64NotNaN (sf.zmm 0#5).l0 = true ∧
      canonZero64 (sf.zmm 0#5).l0 = specInitReducedMin c v ui (8 * (n / 8))) ∧
    sf.gpr 4#4 = s0.gpr 4#4 + 8#64 ∧
    (sf.gpr 4#4).toNat % 8 = 0 ∧
    sf.gpr 5#4 = s0.gpr 5#4 ∧
    sf.gpr 14#4 = s0.gpr 14#4 ∧
    (sf.zmm 15#5).l0 = (s0.zmm 15#5).l0 ∧
    (sf.zmm 15#5).l1 = (s0.zmm 15#5).l1 ∧
    (∀ j1 < n, ∀ j2 < n, j1 ≠ j2 → byteRangeDisjoint64 (elemAddr dstBase j1) (elemAddr dstBase j2)) := by
  have hDisjCD : ∀ j1 < n, ∀ j2 < n, elemAddr costBase j1 ≠ elemAddr dstBase j2 :=
    fun j1 hj1 j2 hj2 => ne_of_byteRangeDisjoint64 (hByteDisjCD j1 hj1 j2 hj2)
  have hDisjVD : ∀ j1 < n, ∀ j2 < n, elemAddr vBase j1 ≠ elemAddr dstBase j2 :=
    fun j1 hj1 j2 hj2 => ne_of_byteRangeDisjoint64 (hByteDisjVD j1 hj1 j2 hj2)
  have hDisjPro : ∀ j < n, ∀ base ∈ ([dstBase, costBase, vBase] : List (BitVec 64)),
      elemAddr base j ≠ s0.gpr 4#4 - 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 8#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 32#64 ∧
      elemAddr base j ≠ s0.gpr 4#4 + 56#64 :=
    fun j hj b hb => ⟨ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.2.1,
      ne_of_byteRangeDisjoint64 (hByteDisjPro j hj b hb).2.2.2⟩
  have hDisjSpill : ∀ j < n, ∀ k < 8, elemAddr dstBase j ≠ s0.gpr 4#4 - 72#64 + BitVec.ofNat 64 (k * 8) :=
    fun j hj k hk => ne_of_byteRangeDisjoint64 (hByteDisjSpill j hj k hk)
  have hEq := initReducedRow64AVX512_functional_equiv dstBase costBase vBase c v dst0 ui n lenDst lenCost lenV fuel s0
    hPc hRun hHaz hPos hLenDst hLenCost hLenV hN hLenC hLenV64
    ⟨hAlign.1, hAlign.2.1, hAlign.2.2.1⟩ hRax hRbx hRdi hRsi hR9 hR10 hUi
    hMemC hMemV hMemD hDisjCD hDisjVD hDisjPro hDisjSpill hFuel
  refine ⟨hEq.1, hEq.2.1, hEq.2.2.1, hEq.2.2.2.1, hEq.2.2.2.2.1, hEq.2.2.2.2.2.1, hEq.2.2.2.2.2.2.1,
    hEq.2.2.2.2.2.2.2.1, ?_,
    hEq.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.2.1, hEq.2.2.2.2.2.2.2.2.2.2.2,
    fun j1 hj1 j2 hj2 hne => elemAddr_byteRangeDisjoint64_self hAlign.1 (by omega) hj1 hj2 hne⟩
  rw [hEq.2.2.2.2.2.2.2.1]
  exact rsp_add8_aligned hAlign.2.2.2

end GoblinFormal.Track5E
