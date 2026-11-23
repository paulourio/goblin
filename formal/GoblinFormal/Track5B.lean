/-
Track5B: Tier 5B Direct Binary Verification of Zero-Allocation Go 1.27 Solver Kernels
(`hasInfeasibleIntegerEqRow`, `augmentRowScalar`, `InitReducedRow64`,
`refillQueueIndirectScalar`, `relaxRowIndirectScalar`).
-/

import GoblinFormal.Track5A

namespace GoblinFormal.Track5B

open GoblinFormal
open GoblinFormal.Track5A

/-!
### Stock Go 1.27 `GOARCH=arm64` ELF `.text` Instruction Words (Chunked) & Relocation Targets
-/

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (154 insns) for `hasInfeasibleIntegerEqRow` in 16-word chunks. -/
def hasInfeasibleIntegerEqRowChunks : List (List UInt32) := [
  [0xf9400b90, 0xeb3063ff, 0x54001269, 0xf81f0ffe, 0xf81f83fd, 0xd10023fd, 0x39492401, 0x36000081, 0xf9411c01, 0xaa1f03e2, 0x14000006, 0xaa1f03e0, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x91000442],
  [0xeb01005f, 0x54000daa, 0xa9511003, 0xeb04005f, 0x54000fe2, 0x38a26863, 0xd3401c63, 0x34000083, 0x90000cdb, 0xfd453b61, 0x17fffff5, 0xa9571003, 0xeb04005f, 0x54000ea2, 0x91000445, 0xf8627866],
  [0xeb05009f, 0x54000e09, 0xf8657863, 0xeb06007f, 0x54000161, 0xa9529003, 0xeb04005f, 0x54000d22, 0xfc627860, 0x1e60c000, 0x90000cdb, 0xfd453b61, 0x1e602020, 0x54fffc45, 0x1400004c, 0xaa1f03e4],
  [0x14000002, 0x910004c6, 0xeb0300df, 0x5400042a, 0xa9589c05, 0xeb0700df, 0x54000b22, 0xf86678a5, 0xa9482007, 0xeb0800bf, 0x54000a82, 0x386568e5, 0x340002a5, 0xa95a1c05, 0xeb0700df, 0x540009c2],
  [0xfc6678a0, 0x1e664001, 0x1e612000, 0x54000161, 0x1e60c020, 0x90000cdb, 0xfd447b61, 0x1e602020, 0x540005e4, 0x9e780005, 0xf10000bf, 0x54fffccd, 0xd503201f, 0x1400003e, 0x90000cdb, 0xfd447b61],
  [0x14000027, 0x90000cdb, 0xfd447b61, 0x14000024, 0xf100009f, 0x5400040d, 0xa9529403, 0xeb05005f, 0x540005c2, 0xfc627860, 0x1e60c001, 0x90000cdb, 0xfd46a362, 0x1e622020, 0x54000288, 0x90000cdb],
  [0xfd454361, 0x1e613803, 0x1e64c063, 0x9e780063, 0x1e612800, 0x1e654000, 0x9e780005, 0x9ac40c66, 0x9ac40c7b, 0x9b048f63, 0xf100007f, 0x9a86d4c3, 0x9b047c63, 0xeb0300bf, 0x5400028b, 0x90000cdb],
  [0xfd447b61, 0x14000006, 0x90000cdb, 0xfd447b61, 0x14000003, 0x90000cdb, 0xfd447b61, 0x90000cdb, 0xfd453b61, 0x17ffff96, 0xb24003e0, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xaa1f03e0, 0xf85f83fd],
  [0xf84107fe, 0xd65f03c0, 0xb24003e0, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x97fa65c6, 0x9ac50c9b, 0x9b059367, 0xaa0503e4, 0xaa0703e5, 0xb5ffff85, 0x17ffffa5, 0x97fa65bf, 0x97fa65be, 0x97fa65bd],
  [0x97fa65bc, 0x97fa65bb, 0x97fa65ba, 0x97fa65b9, 0xd503201f, 0xf90007e0, 0xaa1e03e3, 0x97fa5d7d, 0xf94007e0, 0x17ffff67]
]
def hasInfeasibleIntegerEqRowWords : Array UInt32 := ⟨hasInfeasibleIntegerEqRowChunks.flatten⟩
def hasInfeasibleIntegerEqRowTargets (idx : Nat) : Option String :=
  match idx with
  | 134 | 141 | 142 | 143 | 144 | 145 | 146 | 147 => some "runtime.panicBounds"
  | 151 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (335 insns) for `augmentRowScalar` in 16-word chunks. -/
def augmentRowScalarChunks : List (List UInt32) := [
  [0xf9400b90, 0xd101c3f1, 0xeb10023f, 0x54002729, 0xf8110ffe, 0xf81f83fd, 0xd10023fd, 0xf900b7e2, 0xf900c3e5, 0xf900cfe8, 0xf900dbeb, 0xf100003f, 0x5400088d, 0xeb01013f, 0x5400084b, 0xeb01019f],
  [0x5400080b, 0xf94083e9, 0xeb09003f, 0x540007ac, 0xf9409be9, 0xeb09003f, 0x5400074c, 0xf940a7e9, 0xeb09003f, 0x540006ec, 0x9b007c29, 0xeb09007f, 0x540023c3, 0xcb09008c, 0xcb04012f, 0x8a8ffd29],
  [0x8b090c49, 0xeb0c003f, 0x540022e8, 0xeb0000df, 0x54002289, 0xf900afe0, 0xf900b3e1, 0xf900e7ee, 0xf900c7e6, 0xf900e3ed, 0xf900dbeb, 0xf900d7ea, 0xf900cfe8, 0xf900c3e5, 0xf900bfe4, 0xf900bbe3],
  [0xf900b7e2, 0xfc6078a0, 0xd503201f, 0xaa0b03e0, 0xaa0d03e2, 0xaa0903e3, 0xaa0103e4, 0xaa0c03e5, 0xaa0803e6, 0xaa0103e7, 0xaa0a03e8, 0x97ffcf81, 0xd503201f, 0xb0000d5b, 0xfd46cf61, 0x1e612000],
  [0x54000145, 0xaa1f03f0, 0xf940b3ef, 0xf940dbe6, 0xf940a3f1, 0xf940aff3, 0xf9407fe9, 0xf94097ec, 0xaa1f03e0, 0x1400000d, 0xb000265b, 0x9132837b, 0xa9400760, 0xf85f83fd, 0xf84f07fe, 0xd65f03c0],
  [0xaa1f03e0, 0xaa1f03e1, 0xf85f83fd, 0xf84f07fe, 0xd65f03c0, 0x91000610, 0xeb1001ff, 0x5400044d, 0xfc7078c2, 0xf8307a30, 0xf8307933, 0x1e602040, 0x54ffff28, 0xf8707994, 0xb100069f, 0x54000160],
  [0xeb0001ff, 0x54001ac9, 0xf8607a34, 0xf8307a34, 0xf8207a30, 0xfc6078c3, 0xfc3078c3, 0xfc2078c2, 0x91000400, 0x17ffffec, 0xf940c3e2, 0xfc737840, 0x1e622800, 0xfc337840, 0xf9408fe2, 0xeb02027f],
  [0x540018a2, 0xf9408be2, 0xf8337850, 0xf8307993, 0xaa1f03e0, 0xaa1f03e1, 0xf85f83fd, 0xf84f07fe, 0xd65f03c0, 0xaa1f03e1, 0x14000009, 0xf940dbe6, 0xf9407fe9, 0xf94097ec, 0xf940b3ef, 0xf940a3f1],
  [0xf940aff3, 0xf9406fe1, 0xfd406be0, 0xeb00003f, 0x540002e1, 0xf9006fe1, 0xaa0f03e1, 0xf940e3e2, 0xaa0c03e3, 0xaa0103e4, 0xf9409fe5, 0xaa0103e7, 0xf940abe8, 0xaa0103e9, 0xaa0003ea, 0xaa0603e0],
  [0xaa1103e6, 0x97fffe77, 0xb5000d02, 0xb6f80a61, 0xf9406fe1, 0xf940dbe6, 0xf9407fe9, 0xf94097ec, 0xf940b3ef, 0xf940a3f1, 0xf940aff3, 0xeb0101ff, 0x54001309, 0xfd006be0, 0xf8617a30, 0x91000434],
  [0xf9006ff4, 0xf940e7e1, 0xb4000261, 0xf2401a9f, 0x54000221, 0xf90073f0, 0xf90067e0, 0xaa1f03e0, 0x97f8d1e0, 0x37000680, 0xf94067e0, 0xf940e7e1, 0xf940dbe6, 0xf9407fe9, 0xf94097ec, 0xf940b3ef],
  [0xf94073f0, 0xf940a3f1, 0xf940aff3, 0xf9406ff4, 0xfd406be0, 0xeb1001ff, 0x54000f89, 0xf8707990, 0x9b107df5, 0xf940bbf6, 0xeb1502df, 0x54000e83, 0xf940bff7, 0xcb1502e2, 0xcb1702b8, 0x8a98feb5],
  [0xf940b7f8, 0x8b150f15, 0xeb0201ff, 0x54000d68, 0xf940c7f9, 0xeb10033f, 0x54000ca9, 0xf940c3f3, 0xfc707a62, 0xa900bff1, 0xf940abf1, 0xa901c3f1, 0xf90017e0, 0xaa1503e0, 0xaa0f03e1, 0xf940cfe3],
  [0xaa0103e4, 0xf940d7e5, 0xaa0103e7, 0xf940e3e8, 0xaa0103ea, 0xf94087eb, 0xaa0103ed, 0xf9409fee, 0x1e604001, 0x1e604040, 0x97fffdbe, 0xb7fff401, 0x14000007, 0xb000265b, 0x9132c37b, 0xa9400760],
  [0xf85f83fd, 0xf84f07fe, 0xd65f03c0, 0xf9406fe0, 0xfd406be0, 0x14000002, 0xf9406fe0, 0xf940afe2, 0xf940c3e3, 0xfc627861, 0x1e602821, 0xfc227861, 0xf940abe4, 0xeb00009f, 0x54000783, 0xf940e3e4],
  [0xeb00009f, 0x54000703, 0xaa1f03e4, 0xf940a3e5, 0xf940dbe6, 0xf940b3e7, 0xf940cfe8, 0xf94097e9, 0xf940c7ea, 0x1400000a, 0xaa0203e0, 0xaa0303e1, 0xf85f83fd, 0xf84f07fe, 0xd65f03c0, 0xfc6b7862],
  [0x1e613841, 0xfc2b7861, 0x91000484, 0xeb00009f, 0x540001aa, 0xf86478ab, 0xfc6478c1, 0x1e603821, 0xeb0b00ff, 0x540003e9, 0xfc6b7902, 0x1e612842, 0xfc2b7902, 0xf86b792b, 0xeb0b015f, 0x54fffe08],
  [0x14000017, 0xf9407fe3, 0xf9408fe4, 0xf9408be5, 0x14000002, 0xaa0803e1, 0xeb0100ff, 0x540001e9, 0xf8617866, 0xf8217926, 0xeb06009f, 0x54000149, 0xf86678a8, 0xf82678a1, 0xeb06005f, 0x54fffec1],
  [0xaa1f03e0, 0xaa1f03e1, 0xf85f83fd, 0xf84f07fe, 0xd65f03c0, 0x97faa88b, 0x97faa88a, 0x97faa889, 0x97faa888, 0x97faa887, 0x97faa886, 0xaa1003e0, 0xaa1903e1, 0x97faa883, 0x97faa882, 0xaa1503e0],
  [0xaa1603e1, 0x97faa87f, 0xaa1003e0, 0x97faa87d, 0x97faa87c, 0xaa1303e0, 0x97faa87a, 0x97faa879, 0x97faa878, 0x97faa877, 0x97faa876, 0xd503201f, 0xa90687e0, 0xa9078fe2, 0xa90897e4, 0xa9099fe6],
  [0xa90aa7e8, 0xa90bafea, 0xa90cb7ec, 0xf9006fee, 0xaa1e03e3, 0x97faa033, 0xa94687e0, 0xa9478fe2, 0xa94897e4, 0xa9499fe6, 0xa94aa7e8, 0xa94bafea, 0xa94cb7ec, 0xf9406fee, 0x17fffeb2]
]
def augmentRowScalarWords : Array UInt32 := ⟨augmentRowScalarChunks.flatten⟩
def augmentRowScalarTargets (idx : Nat) : Option String :=
  match idx with
  | 59 => some "internal/simd.InitReducedRow64"
  | 145 => some "assign.refillQueueIndirectScalar"
  | 168 => some "runtime.selectnbrecv"
  | 218 => some "assign.relaxRowIndirectScalar"
  | 293 | 294 | 295 | 296 | 297 | 298 | 301 | 302 | 305 | 307 | 308 | 310 | 311 | 312 | 313 | 314 => some "runtime.panicBounds"
  | 325 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (102 insns) for `InitReducedRow64` in 16-word chunks. -/
def initReducedRow64Chunks : List (List UInt32) := [
  [0xf9400b90, 0xeb3063ff, 0x54000aa9, 0xf81f0ffe, 0xf81f83fd, 0xd10023fd, 0xf9000fe0, 0xf9001be3, 0xf90027e6, 0xeb0400ff, 0x9a84b0e2, 0xeb02003f, 0x9a81c041, 0xb40001c1, 0xd503201f, 0xd1001022],
  [0xd503201f, 0xaa1f03e4, 0xb0000dbb, 0xfd46cf61, 0xb0000dbb, 0xfd46cf62, 0xb0000dbb, 0xfd46cf63, 0xb0000dbb, 0xfd46cf64, 0x14000023, 0xd503201f, 0xb0000dbb, 0xfd46cf60, 0xf85f83fd, 0xf84107fe],
  [0xd65f03c0, 0xfc647865, 0xfc6478c6, 0x1e6638a5, 0x1e6038a5, 0x91000485, 0xfc657866, 0xfc6578c7, 0x1e6738c6, 0x1e6038c6, 0x91000887, 0xfc677867, 0xfc6778c8, 0x1e6838e7, 0x1e6038e7, 0x91000c88],
  [0xfc687868, 0xfc6878c9, 0xfc247805, 0xfc257806, 0xfc277807, 0x1e693908, 0x1e603908, 0xfc287808, 0x1e655884, 0x1e665863, 0x1e675842, 0x1e685821, 0x91001084, 0xeb02009f, 0x54fffc6d, 0x1e635883],
  [0x1e615841, 0x1e615861, 0x14000008, 0xfc647862, 0xfc6478c3, 0x1e633842, 0x1e603842, 0xfc247802, 0x1e625821, 0x91000484, 0xeb01009f, 0x5400006a, 0x54fffee3, 0x14000008, 0x1e602028, 0x54000041],
  [0x9e6703e1, 0x1e604020, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x97fad99f, 0xd503201f, 0xa90087e0, 0xa9018fe2, 0xa90297e4, 0xa9039fe6, 0xf90027e8, 0xfd002be0, 0xaa1e03e3, 0x97fad15e, 0xa94087e0],
  [0xa9418fe2, 0xa94297e4, 0xa9439fe6, 0xf94027e8, 0xfd402be0, 0x17ffff9b]
]
def initReducedRow64Words : Array UInt32 := ⟨initReducedRow64Chunks.flatten⟩
def initReducedRow64Targets (idx : Nat) : Option String :=
  match idx with
  | 85 => some "runtime.panicBounds"
  | 94 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (248 insns) for `refillQueueIndirectScalar` in 16-word chunks. -/
def refillQueueIndirectScalarChunks : List (List UInt32) := [
  [0xf9400b90, 0xeb3063ff, 0x54001ce9, 0xf81f0ffe, 0xf81f83fd, 0xd10023fd, 0xf9000fe0, 0xf9001be3, 0xf90027e6, 0xeb09015f, 0x54000242, 0xeb0a003f, 0x54001b69, 0xfc6a7800, 0xeb09005f, 0x54001ae3],
  [0x91000545, 0xcb0a0048, 0xcb0a012b, 0xd1000508, 0xcb0803ec, 0x8a8cfcac, 0x8b0c0c0c, 0xd100056b, 0xaa1f03ed, 0xaa0503e2, 0xaa0a03ee, 0x14000014, 0xb000265b, 0x9132837b, 0xa9400f62, 0x9e6703e0],
  [0xaa0a03e0, 0x92800001, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xd1001108, 0xcb0803ef, 0x937ffdef, 0x927b01ef, 0x8b0f018c, 0xd100116b, 0x9100114f, 0x1e604080, 0xaa0503ea, 0xaa0f03e5, 0xf100117f],
  [0x5400086b, 0x6d400981, 0x6d411183, 0x1e602020, 0x540001a9, 0x1e602040, 0x54000068, 0x1e602020, 0x14000008, 0x1e602060, 0x54000068, 0x1e602020, 0x14000004, 0x1e602080, 0x54000608, 0x1e602020],
  [0x54000168, 0x54000065, 0xaa0503ea, 0x14000005, 0x1e604001, 0xaa0a03ed, 0xaa0503ea, 0xaa0d03e5, 0x9a9f57ef, 0xd24001ed, 0x14000005, 0x1e604001, 0xaa0a03ef, 0xaa0503ea, 0xaa0f03e5, 0x1e612040],
  [0x54000108, 0x54000065, 0x91000545, 0x14000002, 0x1e604022, 0x9a9f57ef, 0xd24001ed, 0x14000002, 0x1e604022, 0x1e622060, 0x54000108, 0x54000065, 0x91000945, 0x14000002, 0x1e604043, 0x9a9f57ef],
  [0xd24001ed, 0x14000002, 0x1e604043, 0x1e632080, 0x54000108, 0x54000065, 0x91000d45, 0x14000002, 0x1e604064, 0x9a9f57ef, 0xd24001ed, 0x17ffffba, 0x1e604064, 0x17ffffb8, 0x1e604004, 0xaa0a03ef],
  [0xaa0503ea, 0xaa0f03e5, 0x17ffffb3, 0xaa1f03e8, 0x14000003, 0x91000508, 0x1e604020, 0xeb0b011f, 0x540001aa, 0xfc687981, 0x1e602020, 0x54000108, 0x54000065, 0x8b0800aa, 0x14000002, 0x1e604001],
  [0x9a9f57ef, 0xd24001ed, 0x17fffff3, 0x1e604001, 0x17fffff1, 0xb0000d5b, 0xfd46cb61, 0x1e602020, 0x54000444, 0x370005ed, 0xeb0a00ff, 0x54000569, 0xf86a78c5, 0xeb05009f, 0x540004e9, 0xf8657864],
  [0xb100049f, 0x54000240, 0xeb0e00ff, 0x54000429, 0xf86e78c4, 0xf82a78c4, 0xf82e78c5, 0xfc6e7801, 0xeb0a003f, 0x54000349, 0xfc2a7801, 0xfc2e7800, 0xaa0203e0, 0x92800001, 0xaa1f03e2, 0xaa1f03e3],
  [0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xaa0e03e0, 0xaa0503e1, 0xaa1f03e2, 0xaa1f03e3, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xb000265b, 0x9132837b, 0xa9400f62, 0x9e6703e0, 0xaa0e03e0, 0x92800001],
  [0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x97faa9f5, 0x97faa9f4, 0x97faa9f3, 0x97faa9f2, 0x9100054a, 0xeb0a013f, 0x5400042d, 0xeb0a003f, 0x54000549, 0xfc6a7801, 0x1e602020, 0x54ffff28, 0xeb0a00ff],
  [0x54000489, 0xf86a78c5, 0xeb05009f, 0x54000409, 0xf8657868, 0xb100051f, 0x540001a0, 0xeb0e00ff, 0x54000349, 0xf86e78c8, 0xf82a78c8, 0xf82e78c5, 0xeb0e003f, 0x54000289, 0xfc6e7802, 0xfc2a7802],
  [0xfc2e7801, 0x910005ce, 0x17ffffe5, 0xaa0e03e0, 0xaa0503e1, 0xaa1f03e2, 0xaa1f03e3, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xaa0e03e0, 0x92800001, 0xaa1f03e2, 0xaa1f03e3, 0xf85f83fd, 0xf84107fe],
  [0xd65f03c0, 0x97faa9c7, 0x97faa9c6, 0x97faa9c5, 0x97faa9c4, 0x97faa9c3, 0x97faa9c2, 0x97faa9c1, 0xd503201f, 0xa90087e0, 0xa9018fe2, 0xa90297e4, 0xa9039fe6, 0xa904a7e8, 0xf9002fea, 0xaa1e03e3],
  [0x97faa180, 0xa94087e0, 0xa9418fe2, 0xa94297e4, 0xa9439fe6, 0xa944a7e8, 0xf9402fea, 0x17ffff09]
]
def refillQueueIndirectScalarWords : Array UInt32 := ⟨refillQueueIndirectScalarChunks.flatten⟩
def refillQueueIndirectScalarTargets (idx : Nat) : Option String :=
  match idx with
  | 179 | 180 | 181 | 182 | 225 | 226 | 227 | 228 | 229 | 230 | 231 => some "runtime.panicBounds"
  | 240 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (111 insns) for `relaxRowIndirectScalar` in 16-word chunks. -/
def relaxRowIndirectScalarChunks : List (List UInt32) := [
  [0xf9400b90, 0xeb3063ff, 0x54000b09, 0xf81f0ffe, 0xf81f83fd, 0xd10023fd, 0xf90023e0, 0xf9002fe3, 0xf9003be6, 0xf90047e9, 0xf90053ec, 0xf10001ff, 0x540002ad, 0xf9401fe1, 0xb6f80041, 0xaa1f03e1],
  [0xeb0f005f, 0x540008e3, 0xeb0f00bf, 0x54000883, 0xeb0f011f, 0x54000823, 0xeb0f017f, 0x540007c3, 0xeb0f01df, 0x54000763, 0xf94017e2, 0xeb0201ff, 0x540006e8, 0xf9400fe2, 0xf9401be4, 0xaa0103e5],
  [0x14000007, 0xf9401fe0, 0x92800001, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0x91000421, 0xeb0101ff, 0x540004ad, 0xf8617847, 0xb7ffff87, 0xeb0701ff, 0x54ffff4d, 0xfc677802, 0xfc677863, 0x1e633842],
  [0x1e603842, 0x1e622822, 0xfc6178c3, 0x1e632040, 0x54fffe45, 0xf8277924, 0x1e612040, 0x540001e8, 0xf8677988, 0xb100051f, 0x540001c0, 0xeb0501ff, 0x54fffd4d, 0x540002a9, 0xf8657848, 0xf8217848],
  [0xf8257847, 0xfc6578c2, 0xfc2178c2, 0xfc2578c1, 0x910004a5, 0x17ffffe1, 0xfc2178c2, 0x17ffffdf, 0xaa0503e0, 0xaa0703e1, 0xf85f83fd, 0xf84107fe, 0xd65f03c0, 0xaa0503e0, 0x92800001, 0xf85f83fd],
  [0xf84107fe, 0xd65f03c0, 0x97faaac6, 0x97faaac5, 0x97faaac4, 0x97faaac3, 0x97faaac2, 0x97faaac1, 0x97faaac0, 0xd503201f, 0xa90307e0, 0xa9040fe2, 0xa90517e4, 0xa9061fe6, 0xa90727e8, 0xa9082fea],
  [0xa90937ec, 0xa90a3fee, 0x6d0b07e0, 0xaa1e03e3, 0x97faa27c, 0xa94307e0, 0xa9440fe2, 0xa94517e4, 0xa9461fe6, 0xa94727e8, 0xa9482fea, 0xa94937ec, 0xa94a3fee, 0x6d4b07e0, 0x17ffff92]
]
def relaxRowIndirectScalarWords : Array UInt32 := ⟨relaxRowIndirectScalarChunks.flatten⟩
def relaxRowIndirectScalarTargets (idx : Nat) : Option String :=
  match idx with
  | 82 | 83 | 84 | 85 | 86 | 87 | 88 => some "runtime.panicBounds"
  | 100 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-!
### Chunked Decoding, CFG Analyzers, and Soundness Proofs
-/

/-- Indexed list quantifier avoiding deep `Array.map` reductions in kernel proofs. -/
def everyWithIdx {α : Type} (base : Nat) (xs : List α) (p : Nat → α → Bool) : Bool :=
  match xs with
  | [] => true
  | x :: rest => p base x && everyWithIdx (base + 1) rest p

/-- Chunked indexed list quantifier whose kernel stack depth is `O(numChunks + chunkLen)`. -/
def everyWithIdxChunks {α : Type} (base : Nat) (xss : List (List α)) (p : Nat → α → Bool) : Bool :=
  match xss with
  | [] => true
  | c :: cs => everyWithIdx base c p && everyWithIdxChunks (base + c.length) cs p

theorem everyWithIdx_append {α : Type} (base : Nat) (xs ys : List α) (p : Nat → α → Bool) :
    everyWithIdx base (xs ++ ys) p =
      (everyWithIdx base xs p && everyWithIdx (base + xs.length) ys p) := by
  induction xs generalizing base with
  | nil => simp [everyWithIdx]
  | cons x xs ih =>
    simp only [List.cons_append, everyWithIdx, ih, List.length_cons, Bool.and_assoc]
    have hEq : base + 1 + xs.length = base + (xs.length + 1) := by omega
    rw [hEq]

theorem everyWithIdx_flatten {α : Type} (base : Nat) (xss : List (List α)) (p : Nat → α → Bool) :
    everyWithIdx base xss.flatten p = everyWithIdxChunks base xss p := by
  induction xss generalizing base with
  | nil => rfl
  | cons xs xss ih =>
    simp only [List.flatten_cons, everyWithIdx_append, everyWithIdxChunks, ih]

theorem everyWithIdx_map {α β : Type} (f : α → β) (base : Nat) (xs : List α) (p : Nat → β → Bool) :
    everyWithIdx base (xs.map f) p = everyWithIdx base xs (fun i a => p i (f a)) := by
  induction xs generalizing base with
  | nil => rfl
  | cons x xs ih =>
    simp only [List.map_cons, everyWithIdx, ih]

theorem everyWithIdx_getElem? {α : Type} (base : Nat) (xs : List α) (p : Nat → α → Bool)
    (hAll : everyWithIdx base xs p = true) :
    ∀ k a, xs[k]? = some a → p (base + k) a = true := by
  induction xs generalizing base with
  | nil =>
    intro k a hk
    simp at hk
  | cons x xs ih =>
    simp only [everyWithIdx, Bool.and_eq_true] at hAll
    intro k a hk
    cases k with
    | zero =>
      simp only [List.getElem?_cons_zero, Option.some.injEq] at hk
      subst hk
      simpa using hAll.1
    | succ k' =>
      simp only [List.getElem?_cons_succ] at hk
      have hRec := ih (base + 1) hAll.2 k' a hk
      have hEq : base + 1 + k' = base + (k' + 1) := by omega
      rw [hEq] at hRec
      exact hRec

theorem everyWithIdx_getElem?_zero {α : Type} (xs : List α) (p : Nat → α → Bool)
    (hAll : everyWithIdx 0 xs p = true) :
    ∀ k a, xs[k]? = some a → p k a = true := by
  intro k a hk
  simpa using everyWithIdx_getElem? 0 xs p hAll k a hk

def allChunksBounded16 {α : Type} : List (List α) → Bool
  | [] => true
  | [last] => last.length <= 16
  | c :: cs => c.length == 16 && allChunksBounded16 cs

theorem chunks16_len_of_all {α : Type} (chunks : List (List α))
    (hOk : allChunksBounded16 chunks = true) :
    ∀ i c, chunks[i]? = some c → (i + 1 < chunks.length → c.length = 16) ∧ c.length ≤ 16 := by
  induction chunks with
  | nil => intro i c hc; simp at hc
  | cons c cs ih =>
    intro i c' hc'
    cases cs with
    | nil =>
      simp only [allChunksBounded16, decide_eq_true_eq] at hOk
      cases i with
      | zero =>
        simp only [List.getElem?_cons_zero, Option.some.injEq] at hc'
        subst hc'
        exact ⟨fun h => by simp at h, hOk⟩
      | succ i' => simp at hc'
    | cons d ds =>
      simp only [allChunksBounded16, Bool.and_eq_true, beq_iff_eq] at hOk
      cases i with
      | zero =>
        simp only [List.getElem?_cons_zero, Option.some.injEq] at hc'
        subst hc'
        exact ⟨fun _ => hOk.1, by omega⟩
      | succ i' =>
        simp only [List.getElem?_cons_succ] at hc'
        have hRec := ih hOk.2 i' c' hc'
        exact ⟨fun h => hRec.1 (by simp only [List.length_cons] at h ⊢; omega), hRec.2⟩

def chunksSumLen {α : Type} : List (List α) → Nat
  | [] => 0
  | c :: cs => c.length + chunksSumLen cs

theorem chunksSumLen_eq_flatten_length {α : Type} (xss : List (List α)) :
    xss.flatten.length = chunksSumLen xss := by
  induction xss with
  | nil => rfl
  | cons xs xss ih => simp [chunksSumLen, ih]

/-- O(numChunks + cSz) word lookup in a chunked ELF `.text` image. -/
def chunkWord (chunks : List (List UInt32)) (cSz idx : Nat) : Option UInt32 :=
  match chunks[idx / cSz]? with
  | some c => c[idx % cSz]?
  | none => none

private theorem list_getElem?_append {α : Type} (xs ys : List α) (i : Nat) :
    (xs ++ ys)[i]? = if i < xs.length then xs[i]? else ys[i - xs.length]? := by
  by_cases hi : i < xs.length
  · simp [hi, List.getElem?_append_left]
  · have hGe : xs.length ≤ i := Nat.le_of_not_lt hi
    simp [hi, List.getElem?_append_right hGe]

theorem flatten_getElem?_chunk {α : Type} (xss : List (List α)) (cSz idx : Nat)
    (hc : 0 < cSz)
    (hLen : ∀ i c, xss[i]? = some c → (i + 1 < xss.length → c.length = cSz) ∧ c.length ≤ cSz) :
    xss.flatten[idx]? = match xss[idx / cSz]? with | some c => c[idx % cSz]? | none => none := by
  induction xss generalizing idx with
  | nil => simp
  | cons c cs ih =>
    have hcDrop : ∀ i c', cs[i]? = some c' → (i + 1 < cs.length → c'.length = cSz) ∧ c'.length ≤ cSz := by
      intro i c' hc'
      have hPar := hLen (i + 1) c' (by simpa using hc')
      refine ⟨fun h1 => hPar.1 (by simp only [List.length_cons]; omega), hPar.2⟩
    have hcInfo := hLen 0 c rfl
    simp only [List.flatten_cons, list_getElem?_append]
    cases cs with
    | nil =>
      simp only [List.flatten_nil, List.getElem?_nil]
      by_cases hLt : idx < c.length
      · have hLtSz : idx < cSz := Nat.lt_of_lt_of_le hLt hcInfo.2
        rw [Nat.div_eq_of_lt hLtSz, Nat.mod_eq_of_lt hLtSz]
        simp [hLt]
      · have hGe : c.length ≤ idx := Nat.le_of_not_lt hLt
        have hLeft : (if idx < c.length then c[idx]? else none) = none := by simp [hLt]
        rw [hLeft]
        by_cases hLtSz : idx < cSz
        · rw [Nat.div_eq_of_lt hLtSz, Nat.mod_eq_of_lt hLtSz]
          simp [List.getElem?_eq_none hGe]
        · have hGeSz : cSz ≤ idx := Nat.le_of_not_lt hLtSz
          have hDivPos : 1 ≤ idx / cSz := Nat.div_pos hGeSz hc
          have hNon : ([c] : List (List α))[idx / cSz]? = none := by
            apply List.getElem?_eq_none
            simp; omega
          rw [hNon]
    | cons d ds =>
      have hcEq : c.length = cSz := hcInfo.1 (by simp)
      by_cases hLt : idx < cSz
      · rw [Nat.div_eq_of_lt hLt, Nat.mod_eq_of_lt hLt]
        simp [hcEq, hLt]
      · have hGe : cSz ≤ idx := Nat.le_of_not_lt hLt
        simp only [hcEq, hLt, ↓reduceIte]
        rw [ih (idx - cSz) hcDrop]
        have hDivSub : idx / cSz = (idx - cSz) / cSz + 1 := by
          have := Nat.div_eq_sub_div hc hGe
          omega
        have hModSub : idx % cSz = (idx - cSz) % cSz := Nat.mod_eq_sub_mod hGe
        rw [hDivSub, hModSub]
        simp only [List.getElem?_cons_succ]

private theorem decodeArm64Prog_size_eq (xs : List UInt32) :
    (decodeArm64Prog ⟨xs⟩).size = xs.length := by
  unfold decodeArm64Prog
  change (xs.map decodeArm64Word).length = xs.length
  exact List.length_map decodeArm64Word

private theorem decodeChunks_size (chunks : List (List UInt32)) (sz : Nat)
    (h : chunksSumLen chunks = sz) :
    (decodeArm64Prog ⟨chunks.flatten⟩).size = sz := by
  rw [decodeArm64Prog_size_eq, chunksSumLen_eq_flatten_length, h]

theorem decodeProg_getElem?_chunk (chunks : List (List UInt32)) (idx : Nat)
    (hLen : ∀ i c, chunks[i]? = some c → (i + 1 < chunks.length → c.length = 16) ∧ c.length ≤ 16) :
    (decodeArm64Prog ⟨chunks.flatten⟩)[idx]? = (chunkWord chunks 16 idx).map decodeArm64Word := by
  have h1 : (decodeArm64Prog ⟨chunks.flatten⟩)[idx]? = (chunks.flatten.map decodeArm64Word)[idx]? := by
    simp [decodeArm64Prog]
  rw [h1, List.getElem?_map, flatten_getElem?_chunk chunks 16 idx (by decide) hLen]
  unfold chunkWord
  cases chunks[idx / 16]? <;> rfl

/-- Fast chunk-backed ARM64 step matching `stepArm64 (decodeArm64Prog ⟨chunks.flatten⟩)`. -/
def stepChunked (chunks : List (List UInt32)) (cSz progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State) : Arm64State :=
  if s.status != .running then s
  else if s.pcIdx < progLen then
    match chunkWord chunks cSz s.pcIdx with
    | some w => execArm64Insn progLen ext (decodeArm64Word w) s
    | none => { s with status := .trapIllegal }
  else { s with status := .trapIllegal }

def runChunked (fuel : Nat) (chunks : List (List UInt32)) (cSz progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State) : Arm64State :=
  match fuel with
  | 0 => s
  | n + 1 => runChunked n chunks cSz progLen ext (stepChunked chunks cSz progLen ext s)

theorem runChunked_add (n m : Nat) (chunks : List (List UInt32)) (cSz progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State) :
    runChunked (n + m) chunks cSz progLen ext s =
      runChunked m chunks cSz progLen ext (runChunked n chunks cSz progLen ext s) := by
  induction n generalizing s with
  | zero => simp [runChunked]
  | succ k ih =>
    have hEq : k + 1 + m = k + m + 1 := by omega
    rw [hEq, runChunked, ih, runChunked]

private theorem runChunked_of_not_running (n : Nat) (chunks : List (List UInt32)) (cSz progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State) (hStop : s.status ≠ .running) :
    runChunked n chunks cSz progLen ext s = s := by
  induction n with
  | zero => rfl
  | succ k ih =>
    have hCond : (s.status != .running) = true := by simp [hStop]
    have hStep : stepChunked chunks cSz progLen ext s = s := by simp [stepChunked, hCond]
    simp [runChunked, hStep, ih]

theorem stepChunked_eq_stepArm64 (chunks : List (List UInt32)) (progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State)
    (hLen : ∀ i c, chunks[i]? = some c → (i + 1 < chunks.length → c.length = 16) ∧ c.length ≤ 16)
    (hSize : chunks.flatten.length = progLen) :
    stepArm64 (decodeArm64Prog ⟨chunks.flatten⟩) ext s =
      stepChunked chunks 16 progLen ext s := by
  have hProgSz : (decodeArm64Prog ⟨chunks.flatten⟩).size = progLen := by
    rw [decodeArm64Prog_size_eq]; exact hSize
  by_cases hRun : s.status = .running
  · by_cases hLt : s.pcIdx < progLen
    · have hLtArr : s.pcIdx < (decodeArm64Prog ⟨chunks.flatten⟩).size := hProgSz ▸ hLt
      have hGet : (decodeArm64Prog ⟨chunks.flatten⟩)[s.pcIdx]? =
          (chunkWord chunks 16 s.pcIdx).map decodeArm64Word :=
        decodeProg_getElem?_chunk chunks s.pcIdx hLen
      cases hCW : chunkWord chunks 16 s.pcIdx with
      | none =>
        rw [hCW] at hGet
        have hEq := (Array.getElem?_eq_getElem hLtArr).symm.trans hGet
        simp at hEq
      | some w =>
        rw [hCW] at hGet
        have hFetch : (decodeArm64Prog ⟨chunks.flatten⟩)[s.pcIdx]? = some (decodeArm64Word w) := hGet
        rw [stepArm64_insn _ ext s s.pcIdx progLen (decodeArm64Word w) hRun rfl hProgSz hFetch]
        have hNotStop : (s.status != .running) = false := by simp [hRun]
        simp [stepChunked, hNotStop, hLt, hCW]
    · unfold stepArm64 stepChunked
      have hNotStop : (s.status != .running) = false := by simp [hRun]
      have hNotArr : ¬ s.pcIdx < (decodeArm64Prog ⟨chunks.flatten⟩).size := by omega
      simp [hNotStop, hLt, hNotArr]
  · unfold stepArm64 stepChunked
    have hStop : (s.status != .running) = true := by simp [hRun]
    simp [hStop]

theorem runChunked_eq_runArm64 (fuel : Nat) (chunks : List (List UInt32)) (progLen : Nat)
    (ext : Nat → Option String) (s : Arm64State)
    (hLen : ∀ i c, chunks[i]? = some c → (i + 1 < chunks.length → c.length = 16) ∧ c.length ≤ 16)
    (hSize : chunks.flatten.length = progLen) :
    runArm64 fuel (decodeArm64Prog ⟨chunks.flatten⟩) ext s =
      runChunked fuel chunks 16 progLen ext s := by
  induction fuel generalizing s with
  | zero => rfl
  | succ k ih =>
    by_cases hRun : s.status = .running
    · have hNotStop : (s.status != .running) = false := by simp [hRun]
      simp only [runArm64, runChunked, hNotStop, Bool.false_eq_true, ↓reduceIte]
      rw [stepChunked_eq_stepArm64 chunks progLen ext s hLen hSize]
      exact ih _
    · have hStop : (s.status != .running) = true := by simp [hRun]
      simp only [runArm64, hStop, ↓reduceIte]
      exact (runChunked_of_not_running (k + 1) chunks 16 progLen ext s hRun).symm

/-! ### Non-Branch Instruction State Preservation Helpers -/

private theorem status_writeGpr_any (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).status = s.status := by
  unfold writeGpr; split <;> rfl

private theorem pcIdx_writeGpr_any (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeGpr s r v).pcIdx = s.pcIdx := by
  unfold writeGpr; split <;> rfl

private theorem status_writeFp_any (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeFp s r v).status = s.status := rfl

private theorem pcIdx_writeFp_any (s : Arm64State) (r : BitVec 5) (v : BitVec 64) :
    (writeFp s r v).pcIdx = s.pcIdx := rfl

/-- All supported linear (non-branch, non-ret, non-unknown) ARM64 instructions preserve `status` and increment `pcIdx`. -/
private theorem execArm64Insn_linear (progLen : Nat) (ext : Nat → Option String)
    (insn : Arm64Insn) (s : Arm64State)
    (hLin : match insn with
      | .ret _ | .b _ | .bl _ | .bCond _ _ | .cbz _ _ _ _ | .tbz _ _ _ _ | .unknown _ => false
      | .fpUnaryD op _ _ => isSupportedFpUnary op
      | .fpBinaryD op _ _ _ => isSupportedFpBinary op
      | _ => true) :
    (execArm64Insn progLen ext insn s).status = s.status ∧
      (execArm64Insn progLen ext insn s).pcIdx = s.pcIdx + 1 := by
  cases insn with
  | ret _ | b _ | bl _ | bCond _ _ | cbz _ _ _ _ | tbz _ _ _ _ | unknown _ =>
    simp at hLin
  | nop => exact ⟨rfl, rfl⟩
  | adrp _ _ | movWide _ _ _ _ _ | bitfield _ _ _ _ _ _ _
  | condSel _ _ _ _ _ _ | mul3 _ _ _ _ _ =>
    unfold execArm64Insn
    simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]
    exact ⟨trivial, trivial⟩
  | div is64 _ _ _ _ =>
    cases is64
    · unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte, advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩
    · unfold execArm64Insn; dsimp only; simp only [↓reduceIte, advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩
  | addSubImm _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split
    · simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩
    · simp only [advancePc, status_writeGprSP, pcIdx_writeGprSP]; exact ⟨trivial, trivial⟩
  | addSubReg _ _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> (simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩)
  | addSubExt _ _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split
    · simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩
    · simp only [advancePc, status_writeGprSP, pcIdx_writeGprSP]; exact ⟨trivial, trivial⟩
  | logicalImm _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split
    · simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩
    · simp only [advancePc, status_writeGprSP, pcIdx_writeGprSP]; exact ⟨trivial, trivial⟩
  | logicalReg _ _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> (simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any]; exact ⟨trivial, trivial⟩)
  | ldrStrPair _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> split <;> try split
    all_goals simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any, status_writeGprSP, pcIdx_writeGprSP, status_writeFp_any, pcIdx_writeFp_any]
    all_goals exact ⟨trivial, trivial⟩
  | ldrStrUImm _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> try split <;> try split
    all_goals simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any, status_writeFp_any, pcIdx_writeFp_any]
    all_goals exact ⟨trivial, trivial⟩
  | ldrStrImm _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> try split <;> try split <;> try split
    all_goals simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any, status_writeGprSP, pcIdx_writeGprSP, status_writeFp_any, pcIdx_writeFp_any]
    all_goals exact ⟨trivial, trivial⟩
  | ldrStrReg _ _ _ _ _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> try split <;> try split
    all_goals simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any, status_writeFp_any, pcIdx_writeFp_any]
    all_goals exact ⟨trivial, trivial⟩
  | fcmpD _ _ _ => exact ⟨rfl, rfl⟩
  | fpUnaryD op _ _ =>
    dsimp only at hLin
    unfold isSupportedFpUnary at hLin
    split at hLin
    all_goals first | exact ⟨rfl, rfl⟩ | exact absurd hLin (by decide)
  | fpBinaryD op _ _ _ =>
    dsimp only at hLin
    unfold isSupportedFpBinary at hLin
    split at hLin
    all_goals first | exact ⟨rfl, rfl⟩ | exact absurd hLin (by decide)
  | fpIntConv _ _ _ _ _ =>
    unfold execArm64Insn
    dsimp only
    split <;> try split <;> try split
    all_goals simp only [advancePc, status_writeGpr_any, pcIdx_writeGpr_any, status_writeFp_any, pcIdx_writeFp_any]
    all_goals exact ⟨trivial, trivial⟩

/-! ### Tier 5B Executable + Proven ELF CFG Analyzers -/

/-- Structural `List Char` prefix predicate (100% kernel-reducible under `decide`). -/
def strHasPrefix (s p : String) : Bool :=
  s.toList.take p.length == p.toList

/-- Strict fail-closed allow-list of the 18 audited non-allocating external targets permitted by
`checkZeroGC` across all verified Go 1.27 ARM64 kernels (matching `ALLOWED_ZEROGC_SYMS` in
`scripts/extract_elf_track5.py`). Any symbol outside this list is rejected.

Why stack-growth and channel-poll runtime symbols appear here:
- `"runtime.morestack_noctxt.abi0"`: Go 1.27 `arm64` emits a stack-check prologue calling
  `runtime.morestack_noctxt.abi0` in every non-frameless function. `checkZeroGC` permits the
  prologue branch target while `checkStackGuard` / `checkStackGuardWith_sound` independently
  proves `.trapMorestack` unreachable whenever caller stack headroom holds
  (`sp - headroom > gStackGuard0`).
- `"runtime.selectnbrecv"`: appears solely in `augmentRowScalar`'s cold cancellation-poll path
  (`case <-ctx.Done():`); `augmentRowScalar_done_nil_no_selectnbrecv` proves the fast path
  (`done == nil`) bypasses it unconditionally. -/
def allowedZeroGCSyms : List String := [
  "runtime.panicBounds",
  "runtime.panicdivide",
  -- Stack-growth prologue branch target; ruled unreachable by `checkStackGuardWith_sound`.
  "runtime.morestack_noctxt.abi0",
  "runtime.memclrNoHeapPointers",
  -- Cold context-cancel poll in `augmentRowScalar`; bypassed when `done == nil`.
  "runtime.selectnbrecv",
  "math.Float64bits",
  "math.IsInf",
  "math.Nextafter",
  "internal/simd.InitReducedRow64",
  "internal/simd.ScanNearTightMatrix64Scalar",
  "internal/simd.ScanNearTightRow64Scalar",
  "assign.twoSumAssign",
  "assign.exactReducedCost",
  "assign.refillQueueIndirectScalar",
  "assign.relaxRowIndirectScalar",
  "assign.scanCancelRowsFallback",
  "assign.buildCancelEdgesFallback",
  "assign.projectCancelViolFallback"
]

/-- Return `true` iff `sym` is on the audited non-allocating allow-list. -/
def isAllowedZeroGCSym (sym : String) : Bool :=
  allowedZeroGCSyms.contains sym

/-- Fail-closed ZeroGC predicate: every external symbol outside `allowedZeroGCSyms` is forbidden. -/
def isForbiddenAllocSym (sym : String) : Bool :=
  !isAllowedZeroGCSym sym

/-- Return `true` iff `insn` is a decoded, modeled instruction (`RET` must target `X30`, and
scalar FP opcodes must be implemented in `execArm64Insn`). -/
def insnDecodedValid : Arm64Insn → Bool
  | .unknown _ => false
  | .ret rn => rn == 30#5
  | .fpUnaryD op _ _ => isSupportedFpUnary op
  | .fpBinaryD op _ _ _ => isSupportedFpBinary op
  | _ => true

def isOutOfBoundsRel (progSize idx : Nat) (imm : Int) : Bool :=
  let t : Int := (idx : Int) + imm
  !(0 <= t && t.toNat < progSize)

def zeroGcInsnOk (progSize : Nat) (ext : Nat → Option String) (idx : Nat) (insn : Arm64Insn) : Bool :=
  let extClean :=
    match ext idx with
    | some sym => !isForbiddenAllocSym sym
    | none => true
  let noUnknown := insnDecodedValid insn
  let extJumpResolved :=
    match insn with
    | .b imm26 | .bl imm26 => if isOutOfBoundsRel progSize idx imm26 then (ext idx).isSome else true
    | .bCond _ imm19 | .cbz _ _ _ imm19 => if isOutOfBoundsRel progSize idx imm19 then (ext idx).isSome else true
    | .tbz _ _ _ imm14 => if isOutOfBoundsRel progSize idx imm14 then (ext idx).isSome else true
    | _ => true
  extClean && noUnknown && extJumpResolved

/-- Executable Zero-GC / Zero-Allocation verifier over a decoded ARM64 program and external relocation targets. -/
def checkZeroGC (prog : Array Arm64Insn) (ext : Nat → Option String) : Except String Unit :=
  if everyWithIdx 0 prog.toList (zeroGcInsnOk prog.size ext) then
    .ok ()
  else
    .error "ZeroGC violation: non-allowlisted external target, illegal/unknown opcode, or unresolved external branch"

private theorem jumpRel_status_callExternal (progLen : Nat) (ext : Nat → Option String)
    (isCall : Bool) (s : Arm64State) (imm : Int) (tgt : String)
    (hResolved : isOutOfBoundsRel progLen s.pcIdx imm = true → (ext s.pcIdx).isSome = true)
    (hExtClean : match ext s.pcIdx with | some sym => !isForbiddenAllocSym sym = true | none => true)
    (hCall : (jumpRel progLen ext isCall s imm).status = .callExternal tgt)
    (hRun : s.status = .running) :
    isForbiddenAllocSym tgt = false := by
  unfold jumpRel at hCall
  by_cases hIn : (0 ≤ (s.pcIdx : Int) + imm && ((s.pcIdx : Int) + imm).toNat < progLen) = true
  · simp only [hIn, ↓reduceIte] at hCall
    have hEq : (if isCall = true then writeGpr s 30#5 (s.baseAddr + BitVec.ofNat 64 (4 * (s.pcIdx + 1))) else s).status = s.status := by
      cases isCall <;> simp only [↓reduceIte, Bool.false_eq_true, status_writeGpr_any]
    rw [hEq, hRun] at hCall
    cases hCall
  · have hOut : isOutOfBoundsRel progLen s.pcIdx imm = true := by
      simp [isOutOfBoundsRel, hIn]
    have hSome := hResolved hOut
    simp only [hIn, Bool.false_eq_true, ↓reduceIte] at hCall
    cases hExt : ext s.pcIdx with
    | none => rw [hExt] at hSome; simp at hSome
    | some sym =>
      rw [hExt] at hCall hExtClean
      dsimp only at hCall hExtClean
      by_cases hM : (sym == "runtime.morestack_noctxt.abi0" || sym == "runtime.morestack") = true
      · simp [hM] at hCall
      · by_cases hPB : (sym == "runtime.panicBounds") = true
        · simp [hM, hPB] at hCall
        · by_cases hPD : (sym == "runtime.panicdivide") = true
          · simp [hM, hPB, hPD] at hCall
          · simp [hM, hPB, hPD] at hCall
            cases hCall
            simpa using hExtClean

/-- Soundness of `checkZeroGC`: zero single-step calls into forbidden allocation or GC barrier symbols. -/
theorem checkZeroGC_sound (prog : Array Arm64Insn) (ext : Nat → Option String)
    (hOk : checkZeroGC prog ext = .ok ()) (s : Arm64State) (tgt : String)
    (hRun : s.status = .running)
    (hCall : (stepArm64 prog ext s).status = .callExternal tgt) :
    isForbiddenAllocSym tgt = false := by
  unfold checkZeroGC at hOk
  split at hOk
  · rename_i hAll
    have hAt := everyWithIdx_getElem?_zero prog.toList (zeroGcInsnOk prog.size ext) hAll
    unfold stepArm64 at hCall
    have hNotStop : (s.status != .running) = false := by simp [hRun]
    simp only [hNotStop, Bool.false_eq_true, ↓reduceIte] at hCall
    by_cases hLt : s.pcIdx < prog.size
    · simp only [hLt, ↓reduceDIte] at hCall
      have hGet : prog.toList[s.pcIdx]? = some prog[s.pcIdx] :=
        Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLt)
      have hOkInsn := hAt s.pcIdx prog[s.pcIdx] hGet
      unfold zeroGcInsnOk at hOkInsn
      simp only [Bool.and_eq_true] at hOkInsn
      rcases hOkInsn with ⟨⟨hClean, hNoUnk⟩, hRes⟩
      have hClean' : match ext s.pcIdx with | some sym => !isForbiddenAllocSym sym = true | none => true := by
        cases hE : ext s.pcIdx <;> simp [hE] at hClean ⊢; exact hClean
      cases hInsn : prog[s.pcIdx] with
      | unknown w =>
        rw [hInsn] at hNoUnk
        simp [insnDecodedValid] at hNoUnk
      | ret rn =>
        rw [hInsn] at hCall
        dsimp only [execArm64Insn] at hCall
        split at hCall <;> cases hCall
      | b imm26 =>
        rw [hInsn] at hCall hRes
        exact jumpRel_status_callExternal prog.size ext false s imm26 tgt
          (fun h => by simpa [h] using hRes) hClean' hCall hRun
      | bl imm26 =>
        rw [hInsn] at hCall hRes
        exact jumpRel_status_callExternal prog.size ext true s imm26 tgt
          (fun h => by simpa [h] using hRes) hClean' hCall hRun
      | bCond cond imm19 =>
        rw [hInsn] at hCall hRes
        dsimp only [execArm64Insn] at hCall
        split at hCall
        · exact jumpRel_status_callExternal prog.size ext false s imm19 tgt
            (fun h => by simpa [h] using hRes) hClean' hCall hRun
        · simp only [advancePc, hRun] at hCall
          cases hCall
      | cbz is64 isNz rt imm19 =>
        rw [hInsn] at hCall hRes
        dsimp only [execArm64Insn] at hCall
        cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] at hCall <;> split at hCall
        · exact jumpRel_status_callExternal prog.size ext false s imm19 tgt
            (fun h => by simpa [h] using hRes) hClean' hCall hRun
        · simp only [advancePc, hRun] at hCall; cases hCall
        · exact jumpRel_status_callExternal prog.size ext false s imm19 tgt
            (fun h => by simpa [h] using hRes) hClean' hCall hRun
        · simp only [advancePc, hRun] at hCall; cases hCall
      | tbz isNz bit rt imm14 =>
        rw [hInsn] at hCall hRes
        dsimp only [execArm64Insn] at hCall
        cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] at hCall <;> split at hCall
        · exact jumpRel_status_callExternal prog.size ext false s imm14 tgt
            (fun h => by simpa [h] using hRes) hClean' hCall hRun
        · simp only [advancePc, hRun] at hCall; cases hCall
        · exact jumpRel_status_callExternal prog.size ext false s imm14 tgt
            (fun h => by simpa [h] using hRes) hClean' hCall hRun
        · simp only [advancePc, hRun] at hCall; cases hCall
      | nop | adrp _ _ | movWide _ _ _ _ _ | addSubImm _ _ _ _ _ _ | addSubReg _ _ _ _ _ _ _ _
      | addSubExt _ _ _ _ _ _ _ _ | logicalImm _ _ _ _ _ | logicalReg _ _ _ _ _ _ _ _ | bitfield _ _ _ _ _ _ _
      | condSel _ _ _ _ _ _ | div _ _ _ _ _ | mul3 _ _ _ _ _ | ldrStrPair _ _ _ _ _ _ _
      | ldrStrUImm _ _ _ _ _ _ | ldrStrImm _ _ _ _ _ _ _ | ldrStrReg _ _ _ _ _ _ _ _ _
      | fcmpD _ _ _ | fpIntConv _ _ _ _ _ =>
        have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simp [hInsn])
        rw [hLin.1, hRun] at hCall
        cases hCall
      | fpUnaryD _ _ _ | fpBinaryD _ _ _ _ =>
        have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simpa [insnDecodedValid, hInsn] using hNoUnk)
        rw [hLin.1, hRun] at hCall
        cases hCall
    · simp [hLt] at hCall
  · cases hOk

/-- Multi-step Zero-GC soundness: running any fuel never stops at a forbidden GC/allocation call. -/
theorem checkZeroGC_run_sound (prog : Array Arm64Insn) (ext : Nat → Option String)
    (hOk : checkZeroGC prog ext = .ok ()) :
    ∀ (fuel : Nat) (s : Arm64State) (tgt : String),
      s.status = .running →
      (runArm64 fuel prog ext s).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false := by
  intro fuel
  induction fuel with
  | zero =>
    intro s tgt hRun hCall
    simp [runArm64, hRun] at hCall
  | succ k ih =>
    intro s tgt hRun hCall
    rw [runArm64_succ_running k prog ext s hRun] at hCall
    let s1 := stepArm64 prog ext s
    by_cases hRun1 : s1.status = .running
    · exact ih s1 tgt hRun1 hCall
    · have hStop : runArm64 k prog ext s1 = s1 := runArm64_of_not_running k prog ext s1 hRun1
      rw [hStop] at hCall
      exact checkZeroGC_sound prog ext hOk s tgt hRun hCall

/-!
### Stack-Headroom Safety & Panic-Bounds Guard Reachability
-/

/-- Verified Go 1.27 ARM64 stack-check prologue shape. -/
inductive StackPrologueKind where
  | smallFrame (trapStart safeLimit : Nat)
  | largeFrame (subImm trapStart safeLimit : Nat)
  deriving DecidableEq, Repr

def StackPrologueKind.guardIdx : StackPrologueKind → Nat
  | .smallFrame _ _ => 2
  | .largeFrame _ _ _ => 3

def StackPrologueKind.headroomBytes : StackPrologueKind → Nat
  | .smallFrame _ _ => 32
  | .largeFrame subImm _ _ => subImm

def StackPrologueKind.safeLimit : StackPrologueKind → Nat
  | .smallFrame _ s | .largeFrame _ _ s => s

def isMorestackSym (sym : String) : Bool :=
  sym == "runtime.morestack_noctxt.abi0" || sym == "runtime.morestack"

def insnSuccessors (progSize : Nat) (ext : Nat → Option String) (idx : Nat) : Arm64Insn → List Nat
  | .ret _ | .unknown _ => []
  | .b imm26 =>
    let t := (idx : Int) + imm26
    if 0 <= t && t.toNat < progSize then [t.toNat] else []
  | .bl imm26 =>
    let t := (idx : Int) + imm26
    let callSucc := if 0 <= t && t.toNat < progSize then [t.toNat] else []
    let noreturn :=
      match ext idx with
      | some sym => sym == "runtime.panicBounds" || sym == "runtime.panicdivide"
      | none => false
    if noreturn then callSucc else callSucc ++ (if idx + 1 < progSize then [idx + 1] else [])
  | .bCond _ imm19 | .cbz _ _ _ imm19 =>
    let t := (idx : Int) + imm19
    let br := if 0 <= t && t.toNat < progSize then [t.toNat] else []
    let fall := if idx + 1 < progSize then [idx + 1] else []
    br ++ fall
  | .tbz _ _ _ imm14 =>
    let t := (idx : Int) + imm14
    let br := if 0 <= t && t.toNat < progSize then [t.toNat] else []
    let fall := if idx + 1 < progSize then [idx + 1] else []
    br ++ fall
  | _ => if idx + 1 < progSize then [idx + 1] else []

/-- Return `true` iff `insn` modifies `SP` (`r == 31#5`). -/
def modifiesSP : Arm64Insn → Bool
  | .addSubImm _ _ false rd _ _ => rd == 31#5
  | .addSubExt _ _ false _ _ rd _ _ => rd == 31#5
  | .logicalImm _ opc rd _ _ => opc != 3 && rd == 31#5
  | .ldrStrPair _ _ .pre _ _ _ rn => rn == 31#5
  | .ldrStrPair _ _ .post _ _ _ rn => rn == 31#5
  | .ldrStrImm _ _ _ .pre _ _ rn => rn == 31#5
  | .ldrStrImm _ _ _ .post _ _ rn => rn == 31#5
  | _ => false

/-- End index of the verified Go 1.27 frame-allocation prologue (`guardIdx + 1` for standard
pre-indexed LR spill, `guardIdx + 3` for >256-byte `SUB X20, SP; STP; MOV SP, X20`, else `guardIdx`). -/
def prologueEndIdx (getInsn : Nat → Option Arm64Insn) (guardIdx : Nat) : Nat :=
  match getInsn (guardIdx + 3) with
  | some (.addSubImm true false false 31#5 20#5 0#64) => guardIdx + 3
  | _ =>
    match getInsn (guardIdx + 1) with
    | some (.ldrStrImm false false 8 .pre _ 30#5 31#5) => guardIdx + 1
    | _ => guardIdx

private theorem prologueEndIdx_ge (getInsn : Nat → Option Arm64Insn) (guardIdx : Nat) :
    guardIdx ≤ prologueEndIdx getInsn guardIdx := by
  unfold prologueEndIdx
  split
  · omega
  · split <;> omega

/-- Check whether an SP-modifying instruction at `idx` is a verified Go 1.27 prologue frame
allocation or epilogue frame deallocation immediately preceding `RET X30`, bounded by `maxFrameBytes`. -/
def isAllowedPrologueOrEpilogueSP (getInsn : Nat → Option Arm64Insn)
    (guardIdx maxFrameBytes idx : Nat) (insn : Arm64Insn) : Bool :=
  match insn with
  | .ldrStrImm false false 8 .pre imm9 30#5 31#5 =>
    idx == guardIdx + 1 && imm9 < 0 && (-imm9).toNat <= maxFrameBytes
  | .addSubImm true false false 31#5 20#5 0#64 =>
    idx == guardIdx + 3 &&
    (match getInsn (guardIdx + 1), getInsn (guardIdx + 2) with
     | some (.addSubImm true true false 20#5 31#5 frameImm),
       some (.ldrStrPair false false .offset _ 29#5 30#5 20#5) =>
       0 < frameImm.toNat && frameImm.toNat <= maxFrameBytes
     | _, _ => false)
  | .ldrStrImm true false 8 .post imm9 30#5 31#5 =>
    0 < imm9 && imm9.toNat <= maxFrameBytes &&
    getInsn (idx + 1) == some (.ret 30#5)
  | .addSubImm true false false 31#5 31#5 imm =>
    0 < imm.toNat && imm.toNat <= maxFrameBytes &&
    getInsn (idx + 1) == some (.ret 30#5)
  | _ => false

def bodyInsnStackSafe (progSize guardIdx safeLimit maxFrameBytes : Nat)
    (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String)
    (idx : Nat) (insn : Arm64Insn) : Bool :=
  if idx <= guardIdx || safeLimit <= idx then true
  else
    let noUnk := insnDecodedValid insn
    let noMore := match ext idx with | some sym => !isMorestackSym sym | none => true
    let noOutMore :=
      match insn with
      | .b imm26 | .bl imm26 =>
        if isOutOfBoundsRel progSize idx imm26 then
          match ext idx with | some sym => !isMorestackSym sym | none => true
        else true
      | .bCond _ imm19 | .cbz _ _ _ imm19 =>
        if isOutOfBoundsRel progSize idx imm19 then
          match ext idx with | some sym => !isMorestackSym sym | none => true
        else true
      | .tbz _ _ _ imm14 =>
        if isOutOfBoundsRel progSize idx imm14 then
          match ext idx with | some sym => !isMorestackSym sym | none => true
        else true
      | _ => true
    let proEnd := prologueEndIdx getInsn guardIdx
    let minSucc := if idx > proEnd then proEnd else guardIdx
    let succsSafe :=
      (insnSuccessors progSize ext idx insn).all (fun j => minSucc < j && j < safeLimit)
    let fallSafe :=
      if idx + 1 == safeLimit then
        match insn with
        | .ret _ | .b _ | .unknown _ => true
        | .bl _ =>
          match ext idx with
          | some sym => sym == "runtime.panicBounds" || sym == "runtime.panicdivide"
          | none => false
        | _ => false
      else true
    let spSafe :=
      !modifiesSP insn || isAllowedPrologueOrEpilogueSP getInsn guardIdx maxFrameBytes idx insn
    noUnk && noMore && noOutMore && succsSafe && fallSafe && spSafe

private def trapStubReachesMorestack (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (progSize trapStart : Nat) : Bool :=
  let rec go (fuel idx : Nat) : Bool :=
    match fuel with
    | 0 => false
    | f + 1 =>
      if idx >= progSize then false
      else
        match getInsn idx with
        | some (.bl _) =>
          match ext idx with | some sym => isMorestackSym sym | none => false
        | some (.b imm26) =>
          let t := (idx : Int) + imm26
          if 0 <= t && t.toNat < progSize then go f t.toNat
          else match ext idx with | some sym => isMorestackSym sym | none => false
        | some .nop | some (.ldrStrUImm _ _ _ _ _ _) | some (.ldrStrImm _ _ _ _ _ _ _)
        | some (.ldrStrPair _ _ _ _ _ _ _) | some (.logicalReg _ _ _ _ _ _ _ _) =>
          go f (idx + 1)
        | _ => false
  go 24 trapStart

private def computeSafeLimit (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String)
    (trapStart : Nat) : Nat :=
  if trapStart >= 2 then
    match getInsn (trapStart - 1), getInsn (trapStart - 2) with
    | some .nop, some (.bl _) =>
      match ext (trapStart - 2) with
      | some sym =>
        if sym == "runtime.panicBounds" || sym == "runtime.panicdivide" then trapStart - 1
        else trapStart
      | none => trapStart
    | _, _ => trapStart
  else trapStart

def checkStackGuardWith (progSize : Nat) (getInsn : Nat → Option Arm64Insn)
    (insns : List Arm64Insn) (ext : Nat → Option String) : Except String StackPrologueKind :=
  if !(getInsn 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) then
    .error "missing Go stackguard load at [0]"
  else if getInsn 1 == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5) then
    match getInsn 2 with
    | some (.bCond .ls imm19) =>
      if (2 : Int) + imm19 < 4 || ((2 : Int) + imm19).toNat >= progSize then
        .error s!"invalid small-frame stack-guard branch target {(2 : Int) + imm19}"
      else
        if !trapStubReachesMorestack getInsn ext progSize ((2 : Int) + imm19).toNat then
          .error s!"stack-guard target @{((2 : Int) + imm19).toNat} does not invoke runtime.morestack"
        else
          let safeLim := computeSafeLimit getInsn ext ((2 : Int) + imm19).toNat
          if safeLim <= 3 then
            .error "empty guarded function body"
          else if !everyWithIdx 0 insns (bodyInsnStackSafe progSize 2 safeLim 128 getInsn ext) then
            .error "body instruction escapes stack-guarded region, violates SP discipline, or calls morestack"
          else
            .ok (.smallFrame ((2 : Int) + imm19).toNat safeLim)
    | _ => .error "expected B.ls after CMP R16, RSP at [2]"
  else
    match getInsn 1 with
    | some (.addSubImm true true false rd rn subImm) =>
      if !(rd == 17#5 && rn == 31#5) then
        .error "unrecognized ARM64 Go stack-check prologue sequence"
      else if !(getInsn 2 == some (.addSubReg true true true .lsl 0 31#5 17#5 16#5)) then
        .error "expected CMP R16, R17 at [2]"
      else
        match getInsn 3 with
        | some (.bCond .ls imm19) =>
          if subImm.toNat == 0 || subImm.toNat >= 4096 then
            .error "unexpected large-frame stack adjustment immediate"
          else if (3 : Int) + imm19 < 5 || ((3 : Int) + imm19).toNat >= progSize then
            .error s!"invalid large-frame stack-guard branch target {(3 : Int) + imm19}"
          else
            let trapStart := ((3 : Int) + imm19).toNat
            if !trapStubReachesMorestack getInsn ext progSize trapStart then
              .error s!"large-frame stack-guard target @{trapStart} does not invoke runtime.morestack"
            else
              let safeLim := computeSafeLimit getInsn ext trapStart
              if safeLim <= 4 then
                .error "empty guarded large-frame function body"
              else if !everyWithIdx 0 insns (bodyInsnStackSafe progSize 3 safeLim (subImm.toNat + 128) getInsn ext) then
                .error "large-frame body escapes stack-guarded region, violates SP discipline, or calls morestack"
              else
                .ok (.largeFrame subImm.toNat trapStart safeLim)
        | _ => .error "expected B.ls after CMP R16, R17 at [3]"
    | _ => .error "unrecognized ARM64 Go stack-check prologue sequence"

def checkStackGuard (prog : Array Arm64Insn) (ext : Nat → Option String) :
    Except String StackPrologueKind :=
  checkStackGuardWith prog.size (fun i => prog[i]?) prog.toList ext

private theorem step_addSubReg_cmp64 (prog : Array Arm64Insn) (ext : Nat → Option String)
    (s : Arm64State) (k n : Nat) (rn rm : BitVec 5)
    (hRun : s.status = .running) (hPc : s.pcIdx = k) (hSize : prog.size = n)
    (hFetch : prog[k]? = some (.addSubReg true true true .lsl 0 31#5 rn rm)) :
    stepArm64 prog ext s =
      advancePc (writeGpr { s with nzcv := subFlags64 (readGpr s rn) (readGpr s rm) } 31#5
        (readGpr s rn - readGpr s rm)) := by
  rw [stepArm64_insn prog ext s k n (.addSubReg true true true .lsl 0 31#5 rn rm) hRun hPc hSize hFetch]
  have hSh : (readGpr s rm) <<< (0 % 64) = readGpr s rm := by simp
  dsimp only [execArm64Insn]
  simp only [mask32IfNeeded, shiftVal, subFlags, ↓reduceIte, hSh]

private theorem jumpRel_body_safe (progLen guardIdx safeLimit : Nat) (ext : Nat → Option String)
    (isCall : Bool) (s : Arm64State) (imm : Int)
    (hRun : s.status = .running)
    (hNoOutMore : isOutOfBoundsRel progLen s.pcIdx imm = true →
      (match ext s.pcIdx with | some sym => !isMorestackSym sym | none => true) = true)
    (hTargetSafe : 0 ≤ (s.pcIdx : Int) + imm → ((s.pcIdx : Int) + imm).toNat < progLen →
      guardIdx < ((s.pcIdx : Int) + imm).toNat ∧ ((s.pcIdx : Int) + imm).toNat < safeLimit) :
    let s' := jumpRel progLen ext isCall s imm
    s'.status ≠ .trapMorestack ∧ (s'.status = .running → guardIdx < s'.pcIdx ∧ s'.pcIdx < safeLimit) := by
  unfold jumpRel
  dsimp only
  by_cases hIn : (0 ≤ (s.pcIdx : Int) + imm && ((s.pcIdx : Int) + imm).toNat < progLen) = true
  · simp only [hIn, ↓reduceIte]
    have hPos : 0 ≤ (s.pcIdx : Int) + imm := by simp at hIn; exact hIn.1
    have hLt : ((s.pcIdx : Int) + imm).toNat < progLen := by simp at hIn; exact hIn.2
    have hSafe := hTargetSafe hPos hLt
    have hSt : (if isCall = true then writeGpr s 30#5 (s.baseAddr + BitVec.ofNat 64 (4 * (s.pcIdx + 1))) else s).status = s.status := by
      cases isCall <;> simp only [↓reduceIte, Bool.false_eq_true, status_writeGpr_any]
    rw [hSt, hRun]
    exact ⟨(fun h => by cases h), fun _ => hSafe⟩
  · have hOut : isOutOfBoundsRel progLen s.pcIdx imm = true := by simp [isOutOfBoundsRel, hIn]
    have hNoM := hNoOutMore hOut
    simp only [hIn, Bool.false_eq_true, ↓reduceIte]
    cases hExt : ext s.pcIdx with
    | none =>
      exact ⟨(fun h => by cases h), fun h => by cases h⟩
    | some sym =>
      rw [hExt] at hNoM
      dsimp only at hNoM
      have hNotM : (sym == "runtime.morestack_noctxt.abi0" || sym == "runtime.morestack") = false := by
        simpa [isMorestackSym] using hNoM
      dsimp only
      rw [hNotM]
      simp only [Bool.false_eq_true, ↓reduceIte]
      split
      · exact ⟨(fun h => by cases h), fun h => by cases h⟩
      · split
        · exact ⟨(fun h => by cases h), fun h => by cases h⟩
        · exact ⟨(fun h => by cases h), fun h => by cases h⟩

private theorem bodyInsnStackSafe_step (prog : Array Arm64Insn) (ext : Nat → Option String)
    (guardIdx safeLimit maxFrameBytes : Nat) (getInsn : Nat → Option Arm64Insn)
    (hSafeLim : safeLimit ≤ prog.size)
    (hAll : everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size guardIdx safeLimit maxFrameBytes getInsn ext) = true)
    (s : Arm64State) (hRun : s.status = .running)
    (hLo : guardIdx < s.pcIdx) (hHi : s.pcIdx < safeLimit) :
    let s' := stepArm64 prog ext s
    s'.status ≠ .trapMorestack ∧
      (s'.status = .running → guardIdx < s'.pcIdx ∧ s'.pcIdx < safeLimit) := by
  have hAt := everyWithIdx_getElem?_zero prog.toList (bodyInsnStackSafe prog.size guardIdx safeLimit maxFrameBytes getInsn ext) hAll
  have hLtProg : s.pcIdx < prog.size := Nat.lt_of_lt_of_le hHi hSafeLim
  have hGet : prog.toList[s.pcIdx]? = some prog[s.pcIdx] :=
    Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLtProg)
  have hB := hAt s.pcIdx prog[s.pcIdx] hGet
  have hNotSkip : ¬ (s.pcIdx ≤ guardIdx || safeLimit ≤ s.pcIdx) = true := by
    simp [Nat.not_le.mpr hLo, Nat.not_le.mpr hHi]
  unfold bodyInsnStackSafe at hB
  simp only [hNotSkip, Bool.false_eq_true, ↓reduceIte, Bool.and_eq_true] at hB
  rcases hB with ⟨⟨⟨⟨⟨hNoUnk, _⟩, hNoOutMore⟩, hSucc⟩, hFall⟩, _⟩
  have hMinGe : guardIdx ≤ (if s.pcIdx > prologueEndIdx getInsn guardIdx then prologueEndIdx getInsn guardIdx else guardIdx) := by
    have hGe := prologueEndIdx_ge getInsn guardIdx
    split <;> omega
  have hSuccRelax : ∀ j, j ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] →
      guardIdx < j ∧ j < safeLimit := by
    intro j hj
    have hP := List.all_eq_true.mp hSucc j hj
    simp only [Bool.and_eq_true, decide_eq_true_eq] at hP
    exact ⟨Nat.lt_of_le_of_lt hMinGe hP.1, hP.2⟩
  have hAdvSafe : s.pcIdx + 1 ≠ safeLimit → guardIdx < s.pcIdx + 1 ∧ s.pcIdx + 1 < safeLimit := by
    intro hNe; omega
  unfold stepArm64
  have hNotStop : (s.status != .running) = false := by simp [hRun]
  simp only [hNotStop, Bool.false_eq_true, ↓reduceIte, hLtProg, ↓reduceDIte]
  cases hInsn : prog[s.pcIdx] with
  | unknown w =>
    rw [hInsn] at hNoUnk
    simp [insnDecodedValid] at hNoUnk
  | ret rn =>
    dsimp only [execArm64Insn]
    split <;> exact ⟨(fun h => by cases h), fun h => by cases h⟩
  | b imm26 =>
    rw [hInsn] at hNoOutMore
    dsimp only [execArm64Insn]
    apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm26 hRun
      (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
    intro hPos hLt
    have hMem : ((s.pcIdx : Int) + imm26).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
      rw [hInsn]; simp [insnSuccessors, hPos, hLt]
    exact hSuccRelax _ hMem
  | bl imm26 =>
    rw [hInsn] at hNoOutMore
    dsimp only [execArm64Insn]
    apply jumpRel_body_safe prog.size guardIdx safeLimit ext true s imm26 hRun
      (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
    intro hPos hLt
    have hMem : ((s.pcIdx : Int) + imm26).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
      rw [hInsn]
      unfold insnSuccessors
      simp only [hPos, hLt, decide_true, Bool.and_self, ↓reduceIte]
      split <;> try split <;> simp
    exact hSuccRelax _ hMem
  | bCond cond imm19 =>
    rw [hInsn] at hNoOutMore hFall
    dsimp only [execArm64Insn]
    split
    · apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm19 hRun
        (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
      intro hPos hLt
      have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
        rw [hInsn]; simp [insnSuccessors, hPos, hLt]
      exact hSuccRelax _ hMem
    · simp only [advancePc, hRun]
      have hNe : s.pcIdx + 1 ≠ safeLimit := by
        intro hEq; rw [hEq] at hFall; simp at hFall
      exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
  | cbz is64 isNz rt imm19 =>
    rw [hInsn] at hNoOutMore hFall
    dsimp only [execArm64Insn]
    cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
    · apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm19 hRun
        (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
      intro hPos hLt
      have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
        rw [hInsn]; simp [insnSuccessors, hPos, hLt]
      exact hSuccRelax _ hMem
    · simp only [advancePc, hRun]
      have hNe : s.pcIdx + 1 ≠ safeLimit := by
        intro hEq; rw [hEq] at hFall; simp at hFall
      exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
    · apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm19 hRun
        (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
      intro hPos hLt
      have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
        rw [hInsn]; simp [insnSuccessors, hPos, hLt]
      exact hSuccRelax _ hMem
    · simp only [advancePc, hRun]
      have hNe : s.pcIdx + 1 ≠ safeLimit := by
        intro hEq; rw [hEq] at hFall; simp at hFall
      exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
  | tbz isNz bit rt imm14 =>
    rw [hInsn] at hNoOutMore hFall
    dsimp only [execArm64Insn]
    cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
    · apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm14 hRun
        (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
      intro hPos hLt
      have hMem : ((s.pcIdx : Int) + imm14).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
        rw [hInsn]; simp [insnSuccessors, hPos, hLt]
      exact hSuccRelax _ hMem
    · simp only [advancePc, hRun]
      have hNe : s.pcIdx + 1 ≠ safeLimit := by
        intro hEq; rw [hEq] at hFall; simp at hFall
      exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
    · apply jumpRel_body_safe prog.size guardIdx safeLimit ext false s imm14 hRun
        (fun h => by simp only [h, ↓reduceIte] at hNoOutMore; exact hNoOutMore)
      intro hPos hLt
      have hMem : ((s.pcIdx : Int) + imm14).toNat ∈ insnSuccessors prog.size ext s.pcIdx prog[s.pcIdx] := by
        rw [hInsn]; simp [insnSuccessors, hPos, hLt]
      exact hSuccRelax _ hMem
    · simp only [advancePc, hRun]
      have hNe : s.pcIdx + 1 ≠ safeLimit := by
        intro hEq; rw [hEq] at hFall; simp at hFall
      exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
  | nop | adrp _ _ | movWide _ _ _ _ _ | addSubImm _ _ _ _ _ _ | addSubReg _ _ _ _ _ _ _ _
  | addSubExt _ _ _ _ _ _ _ _ | logicalImm _ _ _ _ _ | logicalReg _ _ _ _ _ _ _ _ | bitfield _ _ _ _ _ _ _
  | condSel _ _ _ _ _ _ | div _ _ _ _ _ | mul3 _ _ _ _ _ | ldrStrPair _ _ _ _ _ _ _
  | ldrStrUImm _ _ _ _ _ _ | ldrStrImm _ _ _ _ _ _ _ | ldrStrReg _ _ _ _ _ _ _ _ _
  | fcmpD _ _ _ | fpIntConv _ _ _ _ _ =>
    rw [hInsn] at hFall
    have hNe : s.pcIdx + 1 ≠ safeLimit := by
      intro hEq; rw [hEq] at hFall; simp at hFall
    have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simp [hInsn])
    rw [hInsn] at hLin
    rw [hLin.1, hLin.2, hRun]
    exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩
  | fpUnaryD _ _ _ | fpBinaryD _ _ _ _ =>
    rw [hInsn] at hFall
    have hNe : s.pcIdx + 1 ≠ safeLimit := by
      intro hEq; rw [hEq] at hFall; simp at hFall
    have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simpa [insnDecodedValid, hInsn] using hNoUnk)
    rw [hInsn] at hLin
    rw [hLin.1, hLin.2, hRun]
    exact ⟨(fun h => by cases h), fun _ => hAdvSafe hNe⟩

private theorem bodyInsnStackSafe_run (prog : Array Arm64Insn) (ext : Nat → Option String)
    (guardIdx safeLimit maxFrameBytes : Nat) (getInsn : Nat → Option Arm64Insn)
    (hSafeLim : safeLimit ≤ prog.size)
    (hAll : everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size guardIdx safeLimit maxFrameBytes getInsn ext) = true) :
    ∀ (fuel : Nat) (s : Arm64State),
      s.status = .running → guardIdx < s.pcIdx → s.pcIdx < safeLimit →
      (runArm64 fuel prog ext s).status ≠ .trapMorestack := by
  intro fuel
  induction fuel with
  | zero =>
    intro s hRun _ _
    simp [runArm64, hRun]
  | succ k ih =>
    intro s hRun hLo hHi
    rw [runArm64_succ_running k prog ext s hRun]
    have hStep := bodyInsnStackSafe_step prog ext guardIdx safeLimit maxFrameBytes getInsn hSafeLim hAll s hRun hLo hHi
    let s1 := stepArm64 prog ext s
    by_cases hRun1 : s1.status = .running
    · have hBounds := hStep.2 hRun1
      exact ih s1 hRun1 hBounds.1 hBounds.2
    · rw [runArm64_of_not_running k prog ext s1 hRun1]
      exact hStep.1

private theorem stackGuard_sub_ls_false (sp sg subImm : BitVec 64)
    (hGuard : BitVec.ult sg (sp - subImm) = true) :
    evalCond .ls (subFlags64 (sp - subImm) sg) = false := by
  rw [evalCond_ls_subFlags64, hGuard]; rfl

private theorem computeSafeLimit_le (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String)
    (trapStart : Nat) : computeSafeLimit getInsn ext trapStart ≤ trapStart := by
  unfold computeSafeLimit
  split
  · split
    · split
      · split <;> omega
      · omega
    · omega
  · omega

/-- Full all-fuel stack-guard soundness theorem for `checkStackGuardWith`:
verifies Go stack-guard prologue, bounded single-entry prologue/epilogue SP discipline,
and proves `runtime.morestack` is unreachable for all fuels when entry headroom holds. -/
theorem checkStackGuardWith_sound (prog : Array Arm64Insn) (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (kind : StackPrologueKind)
    (hGet : ∀ i, getInsn i = prog[i]?)
    (hOk : checkStackGuardWith prog.size getInsn prog.toList ext = .ok kind)
    (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0)
    (hSp : BitVec.ult (BitVec.ofNat 64 (kind.headroomBytes - 1)) s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - BitVec.ofNat 64 kind.headroomBytes) = true) :
    ∀ fuel : Nat, (runArm64 fuel prog ext s).status ≠ .trapMorestack := by
  unfold checkStackGuardWith at hOk
  have hGetFun : getInsn = (fun i => prog[i]?) := funext hGet
  rw [hGetFun] at hOk
  by_cases h0 : (prog[0]? == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true
  · have hFetch0 : prog[0]? = some (.ldrStrUImm true false 8 16 16#5 28#5) := eq_of_beq h0
    simp only [h0, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hOk
    by_cases h1 : (prog[1]? == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5)) = true
    · have hFetch1 : prog[1]? = some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5) := eq_of_beq h1
      simp only [h1, ↓reduceIte] at hOk
      cases h2 : prog[2]? with
      | none => rw [h2] at hOk; simp at hOk
      | some insn2 =>
        rw [h2] at hOk
        cases insn2 with
        | bCond cond imm19 =>
          cases cond
          case ls =>
            dsimp only at hOk
            by_cases hRange : ((2 : Int) + imm19 < 4 || ((2 : Int) + imm19).toNat >= prog.size) = true
            · simp only [hRange, ↓reduceIte] at hOk; cases hOk
            · simp only [hRange, Bool.false_eq_true, ↓reduceIte] at hOk
              by_cases hTrap : (!trapStubReachesMorestack (fun i => prog[i]?) ext prog.size ((2 : Int) + imm19).toNat) = true
              · simp only [hTrap, ↓reduceIte] at hOk; cases hOk
              · simp only [hTrap, Bool.false_eq_true, ↓reduceIte] at hOk
                by_cases hSafeGt : computeSafeLimit (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat ≤ 3
                · simp only [hSafeGt, ↓reduceIte] at hOk; cases hOk
                · simp only [hSafeGt, ↓reduceIte] at hOk
                  by_cases hAllNeg : (!everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size 2 (computeSafeLimit (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat) 128 (fun i => prog[i]?) ext)) = true
                  · simp only [hAllNeg, ↓reduceIte] at hOk; cases hOk
                  · simp only [hAllNeg, Bool.false_eq_true, ↓reduceIte, Except.ok.injEq] at hOk
                    have hAll : everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size 2 (computeSafeLimit (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat) 128 (fun i => prog[i]?) ext) = true := by
                      simpa [Bool.not_eq_true'] using hAllNeg
                    have hSafeLimLt : computeSafeLimit (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat < prog.size := by
                      have hT : ((2 : Int) + imm19).toNat < prog.size := by
                        have hNR := hRange
                        simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at hNR
                        omega
                      have hLe := computeSafeLimit_le (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat
                      omega
                    subst hOk
                    have hSafeLimGt3 : 3 < computeSafeLimit (fun i => prog[i]?) ext ((2 : Int) + imm19).toNat := by
                      omega
                    let s1 := advancePc (writeGpr s 16#5 s.gStackGuard0)
                    have hStep0 : stepArm64 prog ext s = s1 := by
                      have hs := step_ldrStrUImm_load64 prog ext s 0 prog.size 16 16#5 28#5 hRun hPc rfl hFetch0
                      have hAddr : readGprSP s 28#5 + BitVec.ofNat 64 16 = s.regs 28#5 + 16#64 := rfl
                      rw [hs, hAddr, hG]
                    have hRun1 : s1.status = .running := by simp [s1, status_advancePc, status_writeGpr, hRun]
                    have hPc1 : s1.pcIdx = 1 := by simp [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
                    have hR16 : readGpr s1 16#5 = s.gStackGuard0 := by
                      rw [show s1 = advancePc (writeGpr s 16#5 s.gStackGuard0) from rfl,
                        readGpr_advancePc, readGpr_writeGpr_eq _ _ _ (by decide)]
                    let s2 := advancePc (writeGpr { s1 with nzcv := subFlags64 s.sp s.gStackGuard0 } 31#5 (s.sp - s.gStackGuard0))
                    have hStep1 : stepArm64 prog ext s1 = s2 := by
                      have hs := step_addSubExt_cmp64 prog ext s1 1 prog.size 31#5 16#5 hRun1 hPc1 rfl hFetch1
                      have hSp1 : readGprSP s1 31#5 = s.sp := rfl
                      have hCmp : (readGpr s1 16#5) <<< (0 % 5) = s.gStackGuard0 := by rw [hR16]; simp
                      rw [hs, hSp1, hCmp]
                    have hRun2 : s2.status = .running := by simp [s2, status_advancePc, status_writeGpr, hRun1]
                    have hPc2 : s2.pcIdx = 2 := by simp [s2, pcIdx_advancePc, pcIdx_writeGpr, hPc1]
                    have hCondFalse : evalCond .ls s2.nzcv = false := by
                      have hNz : s2.nzcv = subFlags64 s.sp s.gStackGuard0 := rfl
                      rw [hNz, evalCond_ls_subFlags64, stackGuard_32_ok s.sp s.gStackGuard0 hSp hGuard]
                      rfl
                    let s3 := advancePc s2
                    have hStep2 : stepArm64 prog ext s2 = s3 :=
                      step_bCond_false prog ext s2 2 prog.size .ls imm19 hRun2 hPc2 rfl h2 hCondFalse
                    have hRun3 : s3.status = .running := by simp [s3, status_advancePc, hRun2]
                    have hPc3 : s3.pcIdx = 3 := by simp [s3, pcIdx_advancePc, hPc2]
                    intro fuel
                    cases fuel with
                    | zero => simp [runArm64, hRun]
                    | succ f1 =>
                      rw [runArm64_succ_running f1 prog ext s hRun, hStep0]
                      cases f1 with
                      | zero => simp [runArm64, hRun1]
                      | succ f2 =>
                        rw [runArm64_succ_running f2 prog ext s1 hRun1, hStep1]
                        cases f2 with
                        | zero => simp [runArm64, hRun2]
                        | succ f3 =>
                          rw [runArm64_succ_running f3 prog ext s2 hRun2, hStep2]
                          exact bodyInsnStackSafe_run prog ext 2 _ 128 (fun i => prog[i]?) (Nat.le_of_lt hSafeLimLt) hAll f3 s3 hRun3 (by omega) (by rw [hPc3]; omega)
          all_goals (dsimp only at hOk; cases hOk)
        | _ => dsimp only at hOk; cases hOk
    · simp only [h1, Bool.false_eq_true, ↓reduceIte] at hOk
      cases h1f : prog[1]? with
      | none => rw [h1f] at hOk; simp at hOk
      | some insn1 =>
        rw [h1f] at hOk
        cases insn1 with
        | addSubImm is64 isSub setFlags rd rn subImm =>
          cases is64 <;> cases isSub <;> cases setFlags <;> try (dsimp only at hOk; cases hOk)
          dsimp only at hOk
          by_cases hRds : (rd == 17#5 && rn == 31#5) = true
          · simp only [hRds, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hOk
            rcases Bool.and_eq_true_iff.mp hRds with ⟨hRd, hRn⟩
            have hRdEq : rd = 17#5 := eq_of_beq hRd
            have hRnEq : rn = 31#5 := eq_of_beq hRn
            subst hRdEq; subst hRnEq
            by_cases h2 : (prog[2]? == some (.addSubReg true true true .lsl 0 31#5 17#5 16#5)) = true
            · have hFetch2 : prog[2]? = some (.addSubReg true true true .lsl 0 31#5 17#5 16#5) := eq_of_beq h2
              simp only [h2, Bool.not_true, Bool.false_eq_true, ↓reduceIte] at hOk
              cases h3 : prog[3]? with
              | none => rw [h3] at hOk; simp at hOk
              | some insn3 =>
                rw [h3] at hOk
                cases insn3 with
                | bCond cond imm19 =>
                  cases cond
                  case ls =>
                    dsimp only at hOk
                    by_cases hImm : (subImm.toNat == 0 || subImm.toNat >= 4096) = true
                    · simp only [hImm, ↓reduceIte] at hOk; cases hOk
                    · simp only [hImm, Bool.false_eq_true, ↓reduceIte] at hOk
                      by_cases hRange : ((3 : Int) + imm19 < 5 || ((3 : Int) + imm19).toNat >= prog.size) = true
                      · simp only [hRange, ↓reduceIte] at hOk; cases hOk
                      · simp only [hRange, Bool.false_eq_true, ↓reduceIte] at hOk
                        by_cases hTrap : (!trapStubReachesMorestack (fun i => prog[i]?) ext prog.size ((3 : Int) + imm19).toNat) = true
                        · simp only [hTrap, ↓reduceIte] at hOk; cases hOk
                        · simp only [hTrap, Bool.false_eq_true, ↓reduceIte] at hOk
                          by_cases hSafeGt : computeSafeLimit (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat ≤ 4
                          · simp only [hSafeGt, ↓reduceIte] at hOk; cases hOk
                          · simp only [hSafeGt, ↓reduceIte] at hOk
                            by_cases hAllNeg : (!everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size 3 (computeSafeLimit (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat) (subImm.toNat + 128) (fun i => prog[i]?) ext)) = true
                            · simp only [hAllNeg, ↓reduceIte] at hOk; cases hOk
                            · simp only [hAllNeg, Bool.false_eq_true, ↓reduceIte, Except.ok.injEq] at hOk
                              have hAll : everyWithIdx 0 prog.toList (bodyInsnStackSafe prog.size 3 (computeSafeLimit (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat) (subImm.toNat + 128) (fun i => prog[i]?) ext) = true := by
                                simpa [Bool.not_eq_true'] using hAllNeg
                              have hSafeLimLt : computeSafeLimit (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat < prog.size := by
                                have hT : ((3 : Int) + imm19).toNat < prog.size := by
                                  have hNR := hRange
                                  simp only [Bool.or_eq_true, decide_eq_true_eq, not_or] at hNR
                                  omega
                                have hLe := computeSafeLimit_le (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat
                                omega
                              subst hOk
                              have hSafeLimGt4 : 4 < computeSafeLimit (fun i => prog[i]?) ext ((3 : Int) + imm19).toNat := by
                                omega
                              let s1 := advancePc (writeGpr s 16#5 s.gStackGuard0)
                              have hStep0 : stepArm64 prog ext s = s1 := by
                                have hs := step_ldrStrUImm_load64 prog ext s 0 prog.size 16 16#5 28#5 hRun hPc rfl hFetch0
                                have hAddr : readGprSP s 28#5 + BitVec.ofNat 64 16 = s.regs 28#5 + 16#64 := rfl
                                rw [hs, hAddr, hG]
                              have hRun1 : s1.status = .running := by simp [s1, status_advancePc, status_writeGpr, hRun]
                              have hPc1 : s1.pcIdx = 1 := by simp [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
                              have hR16_1 : readGpr s1 16#5 = s.gStackGuard0 := by
                                rw [show s1 = advancePc (writeGpr s 16#5 s.gStackGuard0) from rfl,
                                  readGpr_advancePc, readGpr_writeGpr_eq _ _ _ (by decide)]
                              have hSp1 : readGprSP s1 31#5 = s.sp := rfl
                              let s2 := advancePc (writeGprSP s1 17#5 (s.sp - subImm))
                              have hStep1 : stepArm64 prog ext s1 = s2 := by
                                have hs := step_addSubImm_sub64 prog ext s1 1 prog.size 17#5 31#5 subImm hRun1 hPc1 rfl h1f
                                rw [hs, hSp1]
                              have hRun2 : s2.status = .running := by simp [s2, status_advancePc, status_writeGprSP, hRun1]
                              have hPc2 : s2.pcIdx = 2 := by simp [s2, pcIdx_advancePc, pcIdx_writeGprSP, hPc1]
                              have hR16_2 : readGpr s2 16#5 = s.gStackGuard0 := by
                                rw [show s2 = advancePc (writeGprSP s1 17#5 _) from rfl,
                                  readGpr_advancePc, readGpr_writeGprSP_ne _ _ _ _ (by decide) (by decide), hR16_1]
                              have hR17_2 : readGpr s2 17#5 = s.sp - subImm := by
                                simp [s2, readGpr, writeGprSP, advancePc]
                              let s3 := advancePc (writeGpr { s2 with nzcv := subFlags64 (s.sp - subImm) s.gStackGuard0 } 31#5 ((s.sp - subImm) - s.gStackGuard0))
                              have hStep2 : stepArm64 prog ext s2 = s3 := by
                                have hs := step_addSubReg_cmp64 prog ext s2 2 prog.size 17#5 16#5 hRun2 hPc2 rfl hFetch2
                                rw [hs, hR17_2, hR16_2]
                              have hRun3 : s3.status = .running := by simp [s3, status_advancePc, status_writeGpr, hRun2]
                              have hPc3 : s3.pcIdx = 3 := by simp [s3, pcIdx_advancePc, pcIdx_writeGpr, hPc2]
                              have hCondFalse : evalCond .ls s3.nzcv = false := by
                                have hNz : s3.nzcv = subFlags64 (s.sp - subImm) s.gStackGuard0 := rfl
                                have hSubEq : BitVec.ofNat 64 subImm.toNat = subImm := BitVec.eq_of_toNat_eq (by simp)
                                have hGuardSub : BitVec.ult s.gStackGuard0 (s.sp - subImm) = true := by
                                  simpa [StackPrologueKind.headroomBytes, hSubEq] using hGuard
                                rw [hNz]
                                exact stackGuard_sub_ls_false s.sp s.gStackGuard0 subImm hGuardSub
                              let s4 := advancePc s3
                              have hStep3 : stepArm64 prog ext s3 = s4 :=
                                step_bCond_false prog ext s3 3 prog.size .ls imm19 hRun3 hPc3 rfl h3 hCondFalse
                              have hRun4 : s4.status = .running := by simp [s4, status_advancePc, hRun3]
                              have hPc4 : s4.pcIdx = 4 := by simp [s4, pcIdx_advancePc, hPc3]
                              intro fuel
                              cases fuel with
                              | zero => simp [runArm64, hRun]
                              | succ f1 =>
                                rw [runArm64_succ_running f1 prog ext s hRun, hStep0]
                                cases f1 with
                                | zero => simp [runArm64, hRun1]
                                | succ f2 =>
                                  rw [runArm64_succ_running f2 prog ext s1 hRun1, hStep1]
                                  cases f2 with
                                  | zero => simp [runArm64, hRun2]
                                  | succ f3 =>
                                    rw [runArm64_succ_running f3 prog ext s2 hRun2, hStep2]
                                    cases f3 with
                                    | zero => simp [runArm64, hRun3]
                                    | succ f4 =>
                                      rw [runArm64_succ_running f4 prog ext s3 hRun3, hStep3]
                                      exact bodyInsnStackSafe_run prog ext 3 _ (subImm.toNat + 128) (fun i => prog[i]?) (Nat.le_of_lt hSafeLimLt) hAll f4 s4 hRun4 (by omega) (by rw [hPc4]; omega)
                  all_goals (dsimp only at hOk; cases hOk)
                | _ => dsimp only at hOk; cases hOk
            · simp only [h2, Bool.not_false, ↓reduceIte] at hOk; cases hOk
          · have hRdsF : (rd == 17#5 && rn == 31#5) = false := Bool.eq_false_iff.mpr hRds
            simp only [hRdsF, Bool.not_false, ↓reduceIte] at hOk; cases hOk
        | _ => dsimp only at hOk; cases hOk
  · have h0f : (prog[0]? == some (.ldrStrUImm true false 8 16 16#5 28#5)) = false := Bool.eq_false_iff.mpr h0
    simp only [h0f, Bool.not_false, ↓reduceIte] at hOk; cases hOk

theorem checkStackGuard_sound (prog : Array Arm64Insn) (ext : Nat → Option String)
    (kind : StackPrologueKind) (hOk : checkStackGuard prog ext = .ok kind)
    (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0)
    (hSp : BitVec.ult (BitVec.ofNat 64 (kind.headroomBytes - 1)) s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - BitVec.ofNat 64 kind.headroomBytes) = true) :
    ∀ fuel : Nat, (runArm64 fuel prog ext s).status ≠ .trapMorestack :=
  checkStackGuardWith_sound prog (fun i => prog[i]?) ext kind (fun _ => rfl) hOk s hRun hPc hG hSp hGuard

def isBoundsCond : Arm64Cond → Bool
  | .cs | .cc | .hi | .ls => true
  | _ => false

/-- Non-degenerate subtraction flag-setting comparison (`SUBS`/`CMP`) suitable as a bounds guard.
Rejects additions (`CMN`), tautological same-register comparisons (`CMP Rn, Rn`), comparisons
involving `XZR`/`SP` (`31#5`), and zero-immediate comparisons (`CMP Rn, #0`). -/
def isFlagCmpInsn : Arm64Insn → Bool
  | .addSubReg _ true true _ _ _ rn rm =>
    rn != rm && rn != 31#5 && rm != 31#5
  | .addSubImm _ true true _ rn imm =>
    rn != 31#5 && imm != 0#64
  | .addSubExt _ true true _ _ _ rn rm =>
    rn != rm && rn != 31#5 && rm != 31#5
  | _ => false

def isFlagCmpOpt : Option Arm64Insn → Bool
  | some insn => isFlagCmpInsn insn
  | none => false

def isUncondBOpt : Option Arm64Insn → Bool
  | some (.b _) => true
  | _ => false

private def isCondGeOpt : Option Arm64Insn → Bool
  | some (.bCond .ge _) => true
  | _ => false

/-- Structural ARM64 bounds-check guard pattern (`CMP; B.{cs,cc,hi,ls}` or loop
`CMP; B.ge; B.{cc,hi}; B`) requiring a non-degenerate subtraction comparison (`isFlagCmpInsn`). -/
def isVerifiedBoundsGuard (getInsn : Nat → Option Arm64Insn) (src dst : Nat) : Bool :=
  match getInsn src with
  | some (.bCond c imm19) =>
    let brGuard :=
      isBoundsCond c && ((src : Int) + imm19 == (dst : Int)) &&
      ((src >= 1 && isFlagCmpOpt (getInsn (src - 1))) ||
       (src >= 2 && (match getInsn (src - 1) with | some (.bCond _ _) => true | _ => false) &&
         isFlagCmpOpt (getInsn (src - 2))))
    let fallGuard :=
      (dst == src + 1) && isUncondBOpt (getInsn dst) && (c == .cc || c == .hi) &&
      ((src >= 1 && isFlagCmpOpt (getInsn (src - 1))) ||
       (src >= 2 && isCondGeOpt (getInsn (src - 1)) && isFlagCmpOpt (getInsn (src - 2))))
    brGuard || fallGuard
  | _ => false

def isPanicBoundsSym (sym : String) : Bool :=
  sym == "runtime.panicBounds"

def panicBoundsInsnSafe (progSize : Nat) (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (inPb : Nat → Bool) (idx : Nat) (insn : Arm64Insn) : Bool :=
  let noUnk := insnDecodedValid insn
  let pbCovered :=
    match ext idx with
    | some sym => if isPanicBoundsSym sym then inPb idx else true
    | none => true
  if inPb idx then
    noUnk && pbCovered
  else
    let noOutPb :=
      match insn with
      | .b imm26 | .bl imm26 =>
        if isOutOfBoundsRel progSize idx imm26 then
          match ext idx with | some sym => !isPanicBoundsSym sym | none => true
        else true
      | .bCond _ imm19 | .cbz _ _ _ imm19 =>
        if isOutOfBoundsRel progSize idx imm19 then
          match ext idx with | some sym => !isPanicBoundsSym sym | none => true
        else true
      | .tbz _ _ _ imm14 =>
        if isOutOfBoundsRel progSize idx imm14 then
          match ext idx with | some sym => !isPanicBoundsSym sym | none => true
        else true
      | _ => true
    let edgesGuarded :=
      (insnSuccessors progSize ext idx insn).all fun j =>
        !inPb j || isVerifiedBoundsGuard getInsn idx j
    let fallSafe :=
      if idx + 1 < progSize then
        match insn with
        | .ret _ | .b _ | .bl _ | .unknown _ => true
        | _ => !inPb (idx + 1) || isVerifiedBoundsGuard getInsn idx (idx + 1)
      else true
    noUnk && pbCovered && noOutPb && edgesGuarded && fallSafe

def verifyPanicBoundsCertWith (progSize : Nat) (getInsn : Nat → Option Arm64Insn)
    (insns : List Arm64Insn) (ext : Nat → Option String) (inPb : Nat → Bool) : Except String Unit :=
  if progSize > 0 && inPb 0 then
    .error "entry pcIdx 0 is in panicBounds cold region"
  else if !everyWithIdx 0 insns (panicBoundsInsnSafe progSize getInsn ext inPb) then
    .error "unguarded or fall-through path into runtime.panicBounds"
  else
    .ok ()

/-- Backward fixpoint computing all definitely-panic-bound instruction indices in `prog`. -/
def computeDefPb (prog : Array Arm64Insn) (ext : Nat → Option String) : Array Bool := Id.run do
  let n := prog.size
  let mut defPb : Array Bool := Array.replicate n false
  for i in [0:n] do
    if ext i == some "runtime.panicBounds" then
      defPb := defPb.set! i true
  let mut changed := true
  let mut fuel := n + 1
  while changed && fuel > 0 do
    changed := false
    fuel := fuel - 1
    for i in [0:n] do
      if !defPb[i]! then
        let insn := prog[i]!
        let nonCondFall :=
          match insn with
          | .bCond _ _ | .cbz _ _ _ _ | .tbz _ _ _ _ => false
          | _ => true
        if nonCondFall then
          let succs := insnSuccessors n ext i insn
          if !succs.isEmpty && succs.all (fun j => defPb[j]!) then
            defPb := defPb.set! i true
            changed := true
  return defPb

/-- Executable + verified panicBounds reachability verifier (used by `bin-check` and Go tests). -/
def checkPanicBoundsReachability (prog : Array Arm64Insn) (ext : Nat → Option String) :
    Except String (Array Bool) := do
  let cert := computeDefPb prog ext
  let inPb := fun i => if h : i < cert.size then cert[i] else false
  verifyPanicBoundsCertWith prog.size (fun i => prog[i]?) prog.toList ext inPb
  return cert

private theorem jumpRel_pb_guard (prog : Array Arm64Insn) (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (inPb : Nat → Bool) (isCall : Bool)
    (s : Arm64State) (imm : Int)
    (hRun : s.status = .running)
    (hNoOutPb : isOutOfBoundsRel prog.size s.pcIdx imm = true →
      (match ext s.pcIdx with | some sym => !isPanicBoundsSym sym | none => true) = true)
    (hSuccEdge : ∀ j, 0 ≤ (s.pcIdx : Int) + imm → ((s.pcIdx : Int) + imm).toNat < prog.size →
      j = ((s.pcIdx : Int) + imm).toNat →
      (!inPb j || isVerifiedBoundsGuard getInsn s.pcIdx j) = true) :
    let s' := jumpRel prog.size ext isCall s imm
    s'.status ≠ .trapPanicBounds ∧
      (s'.status = .running → inPb s'.pcIdx = false ∨ isVerifiedBoundsGuard getInsn s.pcIdx s'.pcIdx = true) := by
  unfold jumpRel
  dsimp only
  by_cases hIn : (0 ≤ (s.pcIdx : Int) + imm && ((s.pcIdx : Int) + imm).toNat < prog.size) = true
  · simp only [hIn, ↓reduceIte]
    have hPos : 0 ≤ (s.pcIdx : Int) + imm := by simp at hIn; exact hIn.1
    have hLt : ((s.pcIdx : Int) + imm).toNat < prog.size := by simp at hIn; exact hIn.2
    have hE := hSuccEdge ((s.pcIdx : Int) + imm).toNat hPos hLt rfl
    have hSt : (if isCall = true then writeGpr s 30#5 (s.baseAddr + BitVec.ofNat 64 (4 * (s.pcIdx + 1))) else s).status = s.status := by
      cases isCall <;> simp only [↓reduceIte, Bool.false_eq_true, status_writeGpr_any]
    rw [hSt, hRun]
    refine ⟨(fun h => by cases h), fun _ => ?_⟩
    simpa [Bool.or_eq_true, Bool.not_eq_true'] using hE
  · have hOut : isOutOfBoundsRel prog.size s.pcIdx imm = true := by simp [isOutOfBoundsRel, hIn]
    have hNoP := hNoOutPb hOut
    simp only [hIn, Bool.false_eq_true, ↓reduceIte]
    cases hExt : ext s.pcIdx with
    | none =>
      exact ⟨(fun h => by cases h), fun h => by cases h⟩
    | some sym =>
      rw [hExt] at hNoP
      dsimp only at hNoP
      have hNotP : (sym == "runtime.panicBounds") = false := by
        simpa [isPanicBoundsSym] using hNoP
      dsimp only
      split
      · exact ⟨(fun h => by cases h), fun h => by cases h⟩
      · rw [hNotP]
        simp only [Bool.false_eq_true, ↓reduceIte]
        split
        · exact ⟨(fun h => by cases h), fun h => by cases h⟩
        · exact ⟨(fun h => by cases h), fun h => by cases h⟩

/-- Single-step panic-freedom & guard-dominance theorem for `verifyPanicBoundsCertWith`. -/
theorem checkPanicBoundsReachability_step_guard (prog : Array Arm64Insn)
    (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String) (inPb : Nat → Bool)
    (hBound : ∀ j, prog.size ≤ j → inPb j = false)
    (hOk : verifyPanicBoundsCertWith prog.size getInsn prog.toList ext inPb = .ok ())
    (s : Arm64State) (hRun : s.status = .running) (hSafe : inPb s.pcIdx = false) :
    let s' := stepArm64 prog ext s
    s'.status ≠ .trapPanicBounds ∧
      (s'.status = .running → inPb s'.pcIdx = false ∨ isVerifiedBoundsGuard getInsn s.pcIdx s'.pcIdx = true) := by
  unfold verifyPanicBoundsCertWith at hOk
  by_cases h0 : (prog.size > 0 && inPb 0) = true
  · simp only [h0, ↓reduceIte] at hOk; cases hOk
  · simp only [h0, Bool.false_eq_true, ↓reduceIte] at hOk
    by_cases hAllNeg : (!everyWithIdx 0 prog.toList (panicBoundsInsnSafe prog.size getInsn ext inPb)) = true
    · simp only [hAllNeg, ↓reduceIte] at hOk; cases hOk
    · have hAll : everyWithIdx 0 prog.toList (panicBoundsInsnSafe prog.size getInsn ext inPb) = true := by
        simpa [Bool.not_eq_true'] using hAllNeg
      unfold stepArm64
      have hNotStop : (s.status != .running) = false := by simp [hRun]
      simp only [hNotStop, Bool.false_eq_true, ↓reduceIte]
      by_cases hLt : s.pcIdx < prog.size
      · simp only [hLt, ↓reduceDIte]
        have hAt := everyWithIdx_getElem?_zero prog.toList (panicBoundsInsnSafe prog.size getInsn ext inPb) hAll
        have hGet : prog.toList[s.pcIdx]? = some prog[s.pcIdx] :=
          Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLt)
        have hP := hAt s.pcIdx prog[s.pcIdx] hGet
        unfold panicBoundsInsnSafe at hP
        simp only [hSafe, Bool.false_eq_true, ↓reduceIte, Bool.and_eq_true] at hP
        rcases hP with ⟨⟨⟨⟨hNoUnk, _⟩, hNoOutPb⟩, hEdges⟩, hFall⟩
        have hAdvStep : (s.pcIdx + 1 < prog.size → (!inPb (s.pcIdx + 1) || isVerifiedBoundsGuard getInsn s.pcIdx (s.pcIdx + 1)) = true) →
            inPb (s.pcIdx + 1) = false ∨ isVerifiedBoundsGuard getInsn s.pcIdx (s.pcIdx + 1) = true := by
          intro hF
          by_cases hLt1 : s.pcIdx + 1 < prog.size
          · simpa [Bool.or_eq_true, Bool.not_eq_true'] using hF hLt1
          · left; exact hBound (s.pcIdx + 1) (Nat.le_of_not_lt hLt1)
        cases hInsn : prog[s.pcIdx] with
        | unknown w =>
          rw [hInsn] at hNoUnk
          simp [insnDecodedValid] at hNoUnk
        | ret rn =>
          dsimp only [execArm64Insn]
          split <;> exact ⟨(fun h => by cases h), fun h => by cases h⟩
        | b imm26 =>
          rw [hInsn] at hNoOutPb hEdges
          dsimp only [execArm64Insn]
          apply jumpRel_pb_guard prog getInsn ext inPb false s imm26 hRun
            (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
          intro j hPos hLtT rfl
          have hMem : ((s.pcIdx : Int) + imm26).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.b imm26) := by
            simp [insnSuccessors, hPos, hLtT]
          exact List.all_eq_true.mp hEdges _ hMem
        | bl imm26 =>
          rw [hInsn] at hNoOutPb hEdges
          dsimp only [execArm64Insn]
          apply jumpRel_pb_guard prog getInsn ext inPb true s imm26 hRun
            (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
          intro j hPos hLtT rfl
          have hMem : ((s.pcIdx : Int) + imm26).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.bl imm26) := by
            unfold insnSuccessors
            simp only [hPos, hLtT, decide_true, Bool.and_self, ↓reduceIte]
            split <;> try split <;> simp
          exact List.all_eq_true.mp hEdges _ hMem
        | bCond cond imm19 =>
          rw [hInsn] at hNoOutPb hEdges hFall
          dsimp only [execArm64Insn]
          split
          · apply jumpRel_pb_guard prog getInsn ext inPb false s imm19 hRun
              (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
            intro j hPos hLtT rfl
            have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.bCond cond imm19) := by
              simp [insnSuccessors, hPos, hLtT]
            exact List.all_eq_true.mp hEdges _ hMem
          · simp only [advancePc, hRun]
            refine ⟨(fun h => by cases h), fun _ => ?_⟩
            exact hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
        | cbz is64 isNz rt imm19 =>
          rw [hInsn] at hNoOutPb hEdges hFall
          dsimp only [execArm64Insn]
          cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
          · apply jumpRel_pb_guard prog getInsn ext inPb false s imm19 hRun
              (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
            intro j hPos hLtT rfl
            have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.cbz is64 true rt imm19) := by
              simp [insnSuccessors, hPos, hLtT]
            exact List.all_eq_true.mp hEdges _ hMem
          · simp only [advancePc, hRun]
            refine ⟨(fun h => by cases h), fun _ => ?_⟩
            exact hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
          · apply jumpRel_pb_guard prog getInsn ext inPb false s imm19 hRun
              (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
            intro j hPos hLtT rfl
            have hMem : ((s.pcIdx : Int) + imm19).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.cbz is64 false rt imm19) := by
              simp [insnSuccessors, hPos, hLtT]
            exact List.all_eq_true.mp hEdges _ hMem
          · simp only [advancePc, hRun]
            refine ⟨(fun h => by cases h), fun _ => ?_⟩
            exact hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
        | tbz isNz bit rt imm14 =>
          rw [hInsn] at hNoOutPb hEdges hFall
          dsimp only [execArm64Insn]
          cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
          · apply jumpRel_pb_guard prog getInsn ext inPb false s imm14 hRun
              (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
            intro j hPos hLtT rfl
            have hMem : ((s.pcIdx : Int) + imm14).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.tbz true bit rt imm14) := by
              simp [insnSuccessors, hPos, hLtT]
            exact List.all_eq_true.mp hEdges _ hMem
          · simp only [advancePc, hRun]
            refine ⟨(fun h => by cases h), fun _ => ?_⟩
            exact hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
          · apply jumpRel_pb_guard prog getInsn ext inPb false s imm14 hRun
              (fun h => by simp only [h, ↓reduceIte] at hNoOutPb; exact hNoOutPb)
            intro j hPos hLtT rfl
            have hMem : ((s.pcIdx : Int) + imm14).toNat ∈ insnSuccessors prog.size ext s.pcIdx (.tbz false bit rt imm14) := by
              simp [insnSuccessors, hPos, hLtT]
            exact List.all_eq_true.mp hEdges _ hMem
          · simp only [advancePc, hRun]
            refine ⟨(fun h => by cases h), fun _ => ?_⟩
            exact hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
        | nop | adrp _ _ | movWide _ _ _ _ _ | addSubImm _ _ _ _ _ _ | addSubReg _ _ _ _ _ _ _ _
        | addSubExt _ _ _ _ _ _ _ _ | logicalImm _ _ _ _ _ | logicalReg _ _ _ _ _ _ _ _ | bitfield _ _ _ _ _ _ _
        | condSel _ _ _ _ _ _ | div _ _ _ _ _ | mul3 _ _ _ _ _ | ldrStrPair _ _ _ _ _ _ _
        | ldrStrUImm _ _ _ _ _ _ | ldrStrImm _ _ _ _ _ _ _ | ldrStrReg _ _ _ _ _ _ _ _ _
        | fcmpD _ _ _ | fpIntConv _ _ _ _ _ =>
          rw [hInsn] at hFall
          have hNextSafe : inPb (s.pcIdx + 1) = false ∨ isVerifiedBoundsGuard getInsn s.pcIdx (s.pcIdx + 1) = true :=
            hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
          have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simp [hInsn])
          rw [hInsn] at hLin
          rw [hLin.1, hLin.2, hRun]
          exact ⟨(fun h => by cases h), fun _ => hNextSafe⟩
        | fpUnaryD _ _ _ | fpBinaryD _ _ _ _ =>
          rw [hInsn] at hFall
          have hNextSafe : inPb (s.pcIdx + 1) = false ∨ isVerifiedBoundsGuard getInsn s.pcIdx (s.pcIdx + 1) = true :=
            hAdvStep (fun hLt1 => by simp only [hLt1, ↓reduceIte] at hFall; exact hFall)
          have hLin := execArm64Insn_linear prog.size ext prog[s.pcIdx] s (by simpa [insnDecodedValid, hInsn] using hNoUnk)
          rw [hInsn] at hLin
          rw [hLin.1, hLin.2, hRun]
          exact ⟨(fun h => by cases h), fun _ => hNextSafe⟩
      · simp only [hLt, ↓reduceDIte]
        exact ⟨(fun h => by cases h), fun h => by cases h⟩

/-- Trace hypothesis for structural bounds-guard mediation: states that along a `fuel`-step
execution, every transition across a non-degenerate comparison guard (`isVerifiedBoundsGuard`)
stays outside the `inPb` panicBounds region (i.e. the runtime bounds checks pass). -/
def BoundsGuardsHold (prog : Array Arm64Insn) (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (inPb : Nat → Bool) :
    Nat → Arm64State → Prop
  | 0, _ => True
  | k + 1, s =>
    let s1 := stepArm64 prog ext s
    (s.status = .running → s1.status = .running →
      isVerifiedBoundsGuard getInsn s.pcIdx s1.pcIdx = true → inPb s1.pcIdx = false) ∧
      BoundsGuardsHold prog getInsn ext inPb k s1

/-- Structural bounds-guard mediation theorem: whenever `verifyPanicBoundsCertWith` passes,
every CFG edge into `inPb` is mediated by a non-degenerate comparison guard (`isVerifiedBoundsGuard`),
and `runtime.panicBounds` is unreachable for any `fuel : Nat` whenever those runtime bounds guards pass (`BoundsGuardsHold`). -/
theorem checkPanicBoundsReachability_run_safe (prog : Array Arm64Insn)
    (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String) (inPb : Nat → Bool)
    (hBound : ∀ j, prog.size ≤ j → inPb j = false)
    (hOk : verifyPanicBoundsCertWith prog.size getInsn prog.toList ext inPb = .ok ()) :
    ∀ (fuel : Nat) (s : Arm64State),
      s.status = .running → inPb s.pcIdx = false →
      BoundsGuardsHold prog getInsn ext inPb fuel s →
      (runArm64 fuel prog ext s).status ≠ .trapPanicBounds := by
  intro fuel
  induction fuel with
  | zero =>
    intro s hRun _ _
    simp [runArm64, hRun]
  | succ k ih =>
    intro s hRun hSafe hHold
    rw [runArm64_succ_running k prog ext s hRun]
    have hStep := checkPanicBoundsReachability_step_guard prog getInsn ext inPb hBound hOk s hRun hSafe
    let s1 := stepArm64 prog ext s
    by_cases hRun1 : s1.status = .running
    · have hSafe1 : inPb s1.pcIdx = false := by
        cases hStep.2 hRun1 with
        | inl hNotPb => exact hNotPb
        | inr hGuard => exact hHold.1 hRun hRun1 hGuard
      exact ih s1 hRun1 hSafe1 hHold.2
    · rw [runArm64_of_not_running k prog ext s1 hRun1]
      exact hStep.1

/-- Unconditional `panicBounds` unreachability theorem for routines with zero `runtime.panicBounds`
CFG blocks (`inPb := fun _ => false`): requires NO `BoundsGuardsHold` hypothesis. -/
theorem checkPanicBoundsReachability_run_unconditional (prog : Array Arm64Insn)
    (getInsn : Nat → Option Arm64Insn) (ext : Nat → Option String)
    (hOk : verifyPanicBoundsCertWith prog.size getInsn prog.toList ext (fun _ => false) = .ok ()) :
    ∀ (fuel : Nat) (s : Arm64State),
      s.status = .running →
      (runArm64 fuel prog ext s).status ≠ .trapPanicBounds := by
  intro fuel s hRun
  have hHold : ∀ f st, BoundsGuardsHold prog getInsn ext (fun _ => false) f st := by
    intro f
    induction f with
    | zero => intro _; trivial
    | succ k ih =>
      intro st
      exact ⟨fun _ _ _ => rfl, ih (stepArm64 prog ext st)⟩
  exact checkPanicBoundsReachability_run_safe prog getInsn ext (fun _ => false)
    (fun _ _ => rfl) hOk fuel s hRun rfl (hHold fuel s)

/-! ### Verified Safety Theorems on Actual Go 1.27 Tier 5B ELF Kernels -/

private theorem hasInfChunks_len :
    ∀ i c, hasInfeasibleIntegerEqRowChunks[i]? = some c →
      (i + 1 < hasInfeasibleIntegerEqRowChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all hasInfeasibleIntegerEqRowChunks (by decide)

private theorem augChunks_len :
    ∀ i c, augmentRowScalarChunks[i]? = some c →
      (i + 1 < augmentRowScalarChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all augmentRowScalarChunks (by decide)

private theorem initChunks_len :
    ∀ i c, initReducedRow64Chunks[i]? = some c →
      (i + 1 < initReducedRow64Chunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all initReducedRow64Chunks (by decide)

private theorem refillChunks_len :
    ∀ i c, refillQueueIndirectScalarChunks[i]? = some c →
      (i + 1 < refillQueueIndirectScalarChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all refillQueueIndirectScalarChunks (by decide)

private theorem relaxChunks_len :
    ∀ i c, relaxRowIndirectScalarChunks[i]? = some c →
      (i + 1 < relaxRowIndirectScalarChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all relaxRowIndirectScalarChunks (by decide)

private def chunkDecode (chunks : List (List UInt32)) (idx : Nat) : Option Arm64Insn :=
  (chunkWord chunks 16 idx).map decodeArm64Word

private theorem chunkDecode_hasInf (i : Nat) :
    chunkDecode hasInfeasibleIntegerEqRowChunks i = (decodeArm64Prog hasInfeasibleIntegerEqRowWords)[i]? :=
  (decodeProg_getElem?_chunk hasInfeasibleIntegerEqRowChunks i hasInfChunks_len).symm

private theorem chunkDecode_aug (i : Nat) :
    chunkDecode augmentRowScalarChunks i = (decodeArm64Prog augmentRowScalarWords)[i]? :=
  (decodeProg_getElem?_chunk augmentRowScalarChunks i augChunks_len).symm

private theorem chunkDecode_init (i : Nat) :
    chunkDecode initReducedRow64Chunks i = (decodeArm64Prog initReducedRow64Words)[i]? :=
  (decodeProg_getElem?_chunk initReducedRow64Chunks i initChunks_len).symm

private theorem chunkDecode_refill (i : Nat) :
    chunkDecode refillQueueIndirectScalarChunks i = (decodeArm64Prog refillQueueIndirectScalarWords)[i]? :=
  (decodeProg_getElem?_chunk refillQueueIndirectScalarChunks i refillChunks_len).symm

private theorem chunkDecode_relax (i : Nat) :
    chunkDecode relaxRowIndirectScalarChunks i = (decodeArm64Prog relaxRowIndirectScalarWords)[i]? :=
  (decodeProg_getElem?_chunk relaxRowIndirectScalarChunks i relaxChunks_len).symm

private def memInList (xs : List Nat) (i : Nat) : Bool := xs.contains i

private theorem memInList_of_all_lt (xs : List Nat) (hi j : Nat)
    (hAll : xs.all (fun x => x < hi) = true) (hj : hi ≤ j) :
    memInList xs j = false := by
  induction xs with
  | nil => rfl
  | cons x xs ih =>
    simp only [List.all_cons, Bool.and_eq_true, decide_eq_true_eq] at hAll
    have hNe : (j == x) = false := beq_false_of_ne (by omega)
    unfold memInList
    simp only [List.contains_cons, hNe, Bool.false_or]
    exact ih hAll.2

def hasInfeasiblePbIndices : List Nat := [134, 141, 142, 143, 144, 145, 146, 147]
def augmentRowScalarPbIndices : List Nat := [
  272, 293, 294, 295, 296, 297, 298, 299, 300, 301, 302, 303, 304, 305, 306, 307, 308, 309, 310, 311, 312, 313, 314
]
def initReducedRow64PbIndices : List Nat := [77, 85]
def refillQueuePbIndices : List Nat := [179, 180, 181, 182, 225, 226, 227, 228, 229, 230, 231]
def relaxRowPbIndices : List Nat := [82, 83, 84, 85, 86, 87, 88]

private theorem checkZeroGC_of_chunks (chunks : List (List UInt32)) (ext : Nat → Option String) (sz : Nat)
    (hSz : chunksSumLen chunks = sz)
    (hChunks : everyWithIdxChunks 0 chunks (fun i w => zeroGcInsnOk sz ext i (decodeArm64Word w)) = true) :
    checkZeroGC (decodeArm64Prog ⟨chunks.flatten⟩) ext = .ok () := by
  have hProgSz : (decodeArm64Prog ⟨chunks.flatten⟩).size = sz := decodeChunks_size chunks sz hSz
  have hAll : everyWithIdx 0 (decodeArm64Prog ⟨chunks.flatten⟩).toList (zeroGcInsnOk sz ext) = true := by
    have hToList : (decodeArm64Prog ⟨chunks.flatten⟩).toList = chunks.flatten.map decodeArm64Word := rfl
    rw [hToList, everyWithIdx_map, everyWithIdx_flatten]
    exact hChunks
  unfold checkZeroGC
  rw [hProgSz]
  simp [hAll]

theorem hasInfeasible_zeroGC :
    checkZeroGC (decodeArm64Prog hasInfeasibleIntegerEqRowWords) hasInfeasibleIntegerEqRowTargets = .ok () := by
  apply checkZeroGC_of_chunks hasInfeasibleIntegerEqRowChunks hasInfeasibleIntegerEqRowTargets 154 (by decide)
  decide

theorem aug_zeroGC :
    checkZeroGC (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets = .ok () := by
  apply checkZeroGC_of_chunks augmentRowScalarChunks augmentRowScalarTargets 335 (by decide)
  decide

theorem init_zeroGC :
    checkZeroGC (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets = .ok () := by
  apply checkZeroGC_of_chunks initReducedRow64Chunks initReducedRow64Targets 102 (by decide)
  decide

theorem refill_zeroGC :
    checkZeroGC (decodeArm64Prog refillQueueIndirectScalarWords) refillQueueIndirectScalarTargets = .ok () := by
  apply checkZeroGC_of_chunks refillQueueIndirectScalarChunks refillQueueIndirectScalarTargets 248 (by decide)
  decide

theorem relax_zeroGC :
    checkZeroGC (decodeArm64Prog relaxRowIndirectScalarWords) relaxRowIndirectScalarTargets = .ok () := by
  apply checkZeroGC_of_chunks relaxRowIndirectScalarChunks relaxRowIndirectScalarTargets 111 (by decide)
  decide

private theorem hasInf_stackGuard :
    checkStackGuardWith 154 (chunkDecode hasInfeasibleIntegerEqRowChunks)
      (decodeArm64Prog hasInfeasibleIntegerEqRowWords).toList hasInfeasibleIntegerEqRowTargets =
      .ok (.smallFrame 149 148) := by
  have hAll : everyWithIdx 0 (decodeArm64Prog hasInfeasibleIntegerEqRowWords).toList
      (bodyInsnStackSafe 154 2 148 128 (chunkDecode hasInfeasibleIntegerEqRowChunks) hasInfeasibleIntegerEqRowTargets) = true := by
    change everyWithIdx 0 (hasInfeasibleIntegerEqRowChunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    decide
  unfold checkStackGuardWith
  have hc0 : (chunkDecode hasInfeasibleIntegerEqRowChunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1 : (chunkDecode hasInfeasibleIntegerEqRowChunks 1 == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5)) = true := by decide
  have hc2 : chunkDecode hasInfeasibleIntegerEqRowChunks 2 = some (.bCond .ls 147) := by decide
  have hStub : trapStubReachesMorestack (chunkDecode hasInfeasibleIntegerEqRowChunks) hasInfeasibleIntegerEqRowTargets 154 149 = true := by decide
  have hLim : computeSafeLimit (chunkDecode hasInfeasibleIntegerEqRowChunks) hasInfeasibleIntegerEqRowTargets 149 = 148 := by decide
  simp [hc0, hc1, hc2, hStub, hLim, hAll]

private theorem aug_stackGuard :
    checkStackGuardWith 335 (chunkDecode augmentRowScalarChunks)
      (decodeArm64Prog augmentRowScalarWords).toList augmentRowScalarTargets =
      .ok (.largeFrame 112 316 315) := by
  have hAll : everyWithIdx 0 (decodeArm64Prog augmentRowScalarWords).toList
      (bodyInsnStackSafe 335 3 315 (112 + 128) (chunkDecode augmentRowScalarChunks) augmentRowScalarTargets) = true := by
    change everyWithIdx 0 (augmentRowScalarChunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    decide
  unfold checkStackGuardWith
  have hc0 : (chunkDecode augmentRowScalarChunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1 : chunkDecode augmentRowScalarChunks 1 = some (.addSubImm true true false 17#5 31#5 112#64) := by decide
  have hc2 : (chunkDecode augmentRowScalarChunks 2 == some (.addSubReg true true true .lsl 0 31#5 17#5 16#5)) = true := by decide
  have hc3 : chunkDecode augmentRowScalarChunks 3 = some (.bCond .ls 313) := by decide
  have hStub : trapStubReachesMorestack (chunkDecode augmentRowScalarChunks) augmentRowScalarTargets 335 316 = true := by decide
  have hLim : computeSafeLimit (chunkDecode augmentRowScalarChunks) augmentRowScalarTargets 316 = 315 := by decide
  simp [hc0, hc1, hc2, hc3, hStub, hLim, hAll]

private theorem init_stackGuard :
    checkStackGuardWith 102 (chunkDecode initReducedRow64Chunks)
      (decodeArm64Prog initReducedRow64Words).toList initReducedRow64Targets =
      .ok (.smallFrame 87 86) := by
  have hAll : everyWithIdx 0 (decodeArm64Prog initReducedRow64Words).toList
      (bodyInsnStackSafe 102 2 86 128 (chunkDecode initReducedRow64Chunks) initReducedRow64Targets) = true := by
    change everyWithIdx 0 (initReducedRow64Chunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    decide
  unfold checkStackGuardWith
  have hc0 : (chunkDecode initReducedRow64Chunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1 : (chunkDecode initReducedRow64Chunks 1 == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5)) = true := by decide
  have hc2 : chunkDecode initReducedRow64Chunks 2 = some (.bCond .ls 85) := by decide
  have hStub : trapStubReachesMorestack (chunkDecode initReducedRow64Chunks) initReducedRow64Targets 102 87 = true := by decide
  have hLim : computeSafeLimit (chunkDecode initReducedRow64Chunks) initReducedRow64Targets 87 = 86 := by decide
  simp [hc0, hc1, hc2, hStub, hLim, hAll]

private theorem refill_stackGuard :
    checkStackGuardWith 248 (chunkDecode refillQueueIndirectScalarChunks)
      (decodeArm64Prog refillQueueIndirectScalarWords).toList refillQueueIndirectScalarTargets =
      .ok (.smallFrame 233 232) := by
  have hAll : everyWithIdx 0 (decodeArm64Prog refillQueueIndirectScalarWords).toList
      (bodyInsnStackSafe 248 2 232 128 (chunkDecode refillQueueIndirectScalarChunks) refillQueueIndirectScalarTargets) = true := by
    change everyWithIdx 0 (refillQueueIndirectScalarChunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    decide
  unfold checkStackGuardWith
  have hc0 : (chunkDecode refillQueueIndirectScalarChunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1 : (chunkDecode refillQueueIndirectScalarChunks 1 == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5)) = true := by decide
  have hc2 : chunkDecode refillQueueIndirectScalarChunks 2 = some (.bCond .ls 231) := by decide
  have hStub : trapStubReachesMorestack (chunkDecode refillQueueIndirectScalarChunks) refillQueueIndirectScalarTargets 248 233 = true := by decide
  have hLim : computeSafeLimit (chunkDecode refillQueueIndirectScalarChunks) refillQueueIndirectScalarTargets 233 = 232 := by decide
  simp [hc0, hc1, hc2, hStub, hLim, hAll]

private theorem relax_stackGuard :
    checkStackGuardWith 111 (chunkDecode relaxRowIndirectScalarChunks)
      (decodeArm64Prog relaxRowIndirectScalarWords).toList relaxRowIndirectScalarTargets =
      .ok (.smallFrame 90 89) := by
  have hAll : everyWithIdx 0 (decodeArm64Prog relaxRowIndirectScalarWords).toList
      (bodyInsnStackSafe 111 2 89 128 (chunkDecode relaxRowIndirectScalarChunks) relaxRowIndirectScalarTargets) = true := by
    change everyWithIdx 0 (relaxRowIndirectScalarChunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    decide
  unfold checkStackGuardWith
  have hc0 : (chunkDecode relaxRowIndirectScalarChunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1 : (chunkDecode relaxRowIndirectScalarChunks 1 == some (.addSubExt true true true .uxtx 0 31#5 31#5 16#5)) = true := by decide
  have hc2 : chunkDecode relaxRowIndirectScalarChunks 2 = some (.bCond .ls 88) := by decide
  have hStub : trapStubReachesMorestack (chunkDecode relaxRowIndirectScalarChunks) relaxRowIndirectScalarTargets 111 90 = true := by decide
  have hLim : computeSafeLimit (chunkDecode relaxRowIndirectScalarChunks) relaxRowIndirectScalarTargets 90 = 89 := by decide
  simp [hc0, hc1, hc2, hStub, hLim, hAll]

private theorem verifyPb_of_chunks (chunks : List (List UInt32)) (ext : Nat → Option String)
    (pbList : List Nat) (sz : Nat)
    (hSz : chunksSumLen chunks = sz)
    (hNot0 : (decide (0 < sz) && memInList pbList 0) = false)
    (hChunks : everyWithIdxChunks 0 chunks (fun i w => panicBoundsInsnSafe sz (chunkDecode chunks) ext (memInList pbList) i (decodeArm64Word w)) = true) :
    verifyPanicBoundsCertWith (decodeArm64Prog ⟨chunks.flatten⟩).size (chunkDecode chunks)
      (decodeArm64Prog ⟨chunks.flatten⟩).toList ext (memInList pbList) = .ok () := by
  have hProgSz : (decodeArm64Prog ⟨chunks.flatten⟩).size = sz := decodeChunks_size chunks sz hSz
  have hAll : everyWithIdx 0 (decodeArm64Prog ⟨chunks.flatten⟩).toList
      (panicBoundsInsnSafe sz (chunkDecode chunks) ext (memInList pbList)) = true := by
    change everyWithIdx 0 (chunks.flatten.map decodeArm64Word) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    exact hChunks
  unfold verifyPanicBoundsCertWith
  rw [hProgSz]
  simp only [gt_iff_lt, hNot0, Bool.false_eq_true, ↓reduceIte, hAll, Bool.not_true]

theorem hasInfeasible_pbCert :
    verifyPanicBoundsCertWith (decodeArm64Prog hasInfeasibleIntegerEqRowWords).size
      (chunkDecode hasInfeasibleIntegerEqRowChunks)
      (decodeArm64Prog hasInfeasibleIntegerEqRowWords).toList
      hasInfeasibleIntegerEqRowTargets (memInList hasInfeasiblePbIndices) = .ok () := by
  apply verifyPb_of_chunks hasInfeasibleIntegerEqRowChunks hasInfeasibleIntegerEqRowTargets hasInfeasiblePbIndices 154 (by decide) (by decide)
  decide

theorem aug_pbCert :
    verifyPanicBoundsCertWith (decodeArm64Prog augmentRowScalarWords).size
      (chunkDecode augmentRowScalarChunks)
      (decodeArm64Prog augmentRowScalarWords).toList
      augmentRowScalarTargets (memInList augmentRowScalarPbIndices) = .ok () := by
  apply verifyPb_of_chunks augmentRowScalarChunks augmentRowScalarTargets augmentRowScalarPbIndices 335 (by decide) (by decide)
  decide

theorem init_pbCert :
    verifyPanicBoundsCertWith (decodeArm64Prog initReducedRow64Words).size
      (chunkDecode initReducedRow64Chunks)
      (decodeArm64Prog initReducedRow64Words).toList
      initReducedRow64Targets (memInList initReducedRow64PbIndices) = .ok () := by
  apply verifyPb_of_chunks initReducedRow64Chunks initReducedRow64Targets initReducedRow64PbIndices 102 (by decide) (by decide)
  decide

theorem refill_pbCert :
    verifyPanicBoundsCertWith (decodeArm64Prog refillQueueIndirectScalarWords).size
      (chunkDecode refillQueueIndirectScalarChunks)
      (decodeArm64Prog refillQueueIndirectScalarWords).toList
      refillQueueIndirectScalarTargets (memInList refillQueuePbIndices) = .ok () := by
  apply verifyPb_of_chunks refillQueueIndirectScalarChunks refillQueueIndirectScalarTargets refillQueuePbIndices 248 (by decide) (by decide)
  decide

theorem relax_pbCert :
    verifyPanicBoundsCertWith (decodeArm64Prog relaxRowIndirectScalarWords).size
      (chunkDecode relaxRowIndirectScalarChunks)
      (decodeArm64Prog relaxRowIndirectScalarWords).toList
      relaxRowIndirectScalarTargets (memInList relaxRowPbIndices) = .ok () := by
  apply verifyPb_of_chunks relaxRowIndirectScalarChunks relaxRowIndirectScalarTargets relaxRowPbIndices 111 (by decide) (by decide)
  decide

private theorem aug_stack_headroom_112 (sp sg : BitVec 64)
    (hSp : BitVec.ult 239#64 sp = true)
    (hGuard : BitVec.ult sg (sp - 240#64) = true) :
    BitVec.ult 111#64 sp = true ∧ BitVec.ult sg (sp - 112#64) = true := by
  simp only [BitVec.ult, decide_eq_true_eq] at hSp hGuard ⊢
  constructor <;> bv_omega

/-- Tier 5B theorem: decoded Go 1.27 ARM64 `hasInfeasibleIntegerEqRow` guarantees Zero-GC allocation, stack-headroom safety (all fuels), and structurally CFG-guarded `panicBounds` mediation (`BoundsGuardsHold`). -/
theorem hasInfeasibleIntegerEqRow_elf_safe (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hSp : BitVec.ult 31#64 s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - 32#64) = true)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0) :
    let prog := decodeArm64Prog hasInfeasibleIntegerEqRowWords
    (∀ fuel tgt, (runArm64 fuel prog hasInfeasibleIntegerEqRowTargets s).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel prog hasInfeasibleIntegerEqRowTargets s).status ≠ .trapMorestack) ∧
    (∀ fuel, BoundsGuardsHold prog (fun i => prog[i]?) hasInfeasibleIntegerEqRowTargets (memInList hasInfeasiblePbIndices) fuel s →
      (runArm64 fuel prog hasInfeasibleIntegerEqRowTargets s).status ≠ .trapPanicBounds) := by
  have hProgSz : (decodeArm64Prog hasInfeasibleIntegerEqRowWords).size = 154 :=
    decodeChunks_size hasInfeasibleIntegerEqRowChunks 154 (by decide)
  have hSafe0 : memInList hasInfeasiblePbIndices s.pcIdx = false := by rw [hPc]; decide
  have hBound : ∀ j, (decodeArm64Prog hasInfeasibleIntegerEqRowWords).size ≤ j →
      memInList hasInfeasiblePbIndices j = false := by
    intro j hj
    rw [hProgSz] at hj
    exact memInList_of_all_lt hasInfeasiblePbIndices 154 j (by decide) hj
  have hGet : (fun i => (decodeArm64Prog hasInfeasibleIntegerEqRowWords)[i]?) =
      chunkDecode hasInfeasibleIntegerEqRowChunks :=
    (funext chunkDecode_hasInf).symm
  refine ⟨fun fuel tgt => checkZeroGC_run_sound _ _ hasInfeasible_zeroGC fuel s tgt hRun,
          checkStackGuardWith_sound _ (chunkDecode hasInfeasibleIntegerEqRowChunks) _ (.smallFrame 149 148)
            chunkDecode_hasInf (by rw [hProgSz]; exact hasInf_stackGuard) s hRun hPc hG hSp hGuard,
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecode hasInfeasibleIntegerEqRowChunks) _ _ hBound hasInfeasible_pbCert fuel s hRun hSafe0 (hGet ▸ hHold)⟩

/-- Tier 5B theorem: decoded Go 1.27 ARM64 `augmentRowScalar` and all three helpers (`InitReducedRow64`, `refillQueueIndirectScalar`, `relaxRowIndirectScalar`) guarantee Zero-GC allocation, stack-headroom safety, and structurally CFG-guarded `panicBounds` mediation (`BoundsGuardsHold`). -/
theorem augmentRowScalar_elf_safe (sAug sInit sRefill sRelax : Arm64State)
    (hRunA : sAug.status = .running) (hPcA : sAug.pcIdx = 0)
    (hSpA : BitVec.ult 239#64 sAug.sp = true) (hGuardA : BitVec.ult sAug.gStackGuard0 (sAug.sp - 240#64) = true)
    (hGA : readU64 sAug.mem (sAug.regs 28#5 + 16#64) = sAug.gStackGuard0)
    (hRunI : sInit.status = .running) (hPcI : sInit.pcIdx = 0)
    (hSpI : BitVec.ult 31#64 sInit.sp = true) (hGuardI : BitVec.ult sInit.gStackGuard0 (sInit.sp - 32#64) = true)
    (hGI : readU64 sInit.mem (sInit.regs 28#5 + 16#64) = sInit.gStackGuard0)
    (hRunRf : sRefill.status = .running) (hPcRf : sRefill.pcIdx = 0)
    (hSpRf : BitVec.ult 31#64 sRefill.sp = true) (hGuardRf : BitVec.ult sRefill.gStackGuard0 (sRefill.sp - 32#64) = true)
    (hGRf : readU64 sRefill.mem (sRefill.regs 28#5 + 16#64) = sRefill.gStackGuard0)
    (hRunRx : sRelax.status = .running) (hPcRx : sRelax.pcIdx = 0)
    (hSpRx : BitVec.ult 31#64 sRelax.sp = true) (hGuardRx : BitVec.ult sRelax.gStackGuard0 (sRelax.sp - 32#64) = true)
    (hGRx : readU64 sRelax.mem (sRelax.regs 28#5 + 16#64) = sRelax.gStackGuard0) :
    (∀ fuel tgt, (runArm64 fuel (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets sAug).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel tgt, (runArm64 fuel (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets sInit).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel tgt, (runArm64 fuel (decodeArm64Prog refillQueueIndirectScalarWords) refillQueueIndirectScalarTargets sRefill).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel tgt, (runArm64 fuel (decodeArm64Prog relaxRowIndirectScalarWords) relaxRowIndirectScalarTargets sRelax).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets sAug).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets sInit).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel (decodeArm64Prog refillQueueIndirectScalarWords) refillQueueIndirectScalarTargets sRefill).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel (decodeArm64Prog relaxRowIndirectScalarWords) relaxRowIndirectScalarTargets sRelax).status ≠ .trapMorestack) ∧
    (∀ fuel, BoundsGuardsHold (decodeArm64Prog augmentRowScalarWords) (fun i => (decodeArm64Prog augmentRowScalarWords)[i]?) augmentRowScalarTargets (memInList augmentRowScalarPbIndices) fuel sAug →
      (runArm64 fuel (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets sAug).status ≠ .trapPanicBounds) ∧
    (∀ fuel, BoundsGuardsHold (decodeArm64Prog initReducedRow64Words) (fun i => (decodeArm64Prog initReducedRow64Words)[i]?) initReducedRow64Targets (memInList initReducedRow64PbIndices) fuel sInit →
      (runArm64 fuel (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets sInit).status ≠ .trapPanicBounds) ∧
    (∀ fuel, BoundsGuardsHold (decodeArm64Prog refillQueueIndirectScalarWords) (fun i => (decodeArm64Prog refillQueueIndirectScalarWords)[i]?) refillQueueIndirectScalarTargets (memInList refillQueuePbIndices) fuel sRefill →
      (runArm64 fuel (decodeArm64Prog refillQueueIndirectScalarWords) refillQueueIndirectScalarTargets sRefill).status ≠ .trapPanicBounds) ∧
    (∀ fuel, BoundsGuardsHold (decodeArm64Prog relaxRowIndirectScalarWords) (fun i => (decodeArm64Prog relaxRowIndirectScalarWords)[i]?) relaxRowIndirectScalarTargets (memInList relaxRowPbIndices) fuel sRelax →
      (runArm64 fuel (decodeArm64Prog relaxRowIndirectScalarWords) relaxRowIndirectScalarTargets sRelax).status ≠ .trapPanicBounds) := by
  have hSzA : (decodeArm64Prog augmentRowScalarWords).size = 335 :=
    decodeChunks_size augmentRowScalarChunks 335 (by decide)
  have hSzI : (decodeArm64Prog initReducedRow64Words).size = 102 :=
    decodeChunks_size initReducedRow64Chunks 102 (by decide)
  have hSzRf : (decodeArm64Prog refillQueueIndirectScalarWords).size = 248 :=
    decodeChunks_size refillQueueIndirectScalarChunks 248 (by decide)
  have hSzRx : (decodeArm64Prog relaxRowIndirectScalarWords).size = 111 :=
    decodeChunks_size relaxRowIndirectScalarChunks 111 (by decide)
  have hHeadA := aug_stack_headroom_112 sAug.sp sAug.gStackGuard0 hSpA hGuardA
  have hBdA : ∀ j, (decodeArm64Prog augmentRowScalarWords).size ≤ j → memInList augmentRowScalarPbIndices j = false := by
    intro j hj; rw [hSzA] at hj; exact memInList_of_all_lt augmentRowScalarPbIndices 335 j (by decide) hj
  have hBdI : ∀ j, (decodeArm64Prog initReducedRow64Words).size ≤ j → memInList initReducedRow64PbIndices j = false := by
    intro j hj; rw [hSzI] at hj; exact memInList_of_all_lt initReducedRow64PbIndices 102 j (by decide) hj
  have hBdRf : ∀ j, (decodeArm64Prog refillQueueIndirectScalarWords).size ≤ j → memInList refillQueuePbIndices j = false := by
    intro j hj; rw [hSzRf] at hj; exact memInList_of_all_lt refillQueuePbIndices 248 j (by decide) hj
  have hBdRx : ∀ j, (decodeArm64Prog relaxRowIndirectScalarWords).size ≤ j → memInList relaxRowPbIndices j = false := by
    intro j hj; rw [hSzRx] at hj; exact memInList_of_all_lt relaxRowPbIndices 111 j (by decide) hj
  have hGetA : (fun i => (decodeArm64Prog augmentRowScalarWords)[i]?) = chunkDecode augmentRowScalarChunks :=
    (funext chunkDecode_aug).symm
  have hGetI : (fun i => (decodeArm64Prog initReducedRow64Words)[i]?) = chunkDecode initReducedRow64Chunks :=
    (funext chunkDecode_init).symm
  have hGetRf : (fun i => (decodeArm64Prog refillQueueIndirectScalarWords)[i]?) = chunkDecode refillQueueIndirectScalarChunks :=
    (funext chunkDecode_refill).symm
  have hGetRx : (fun i => (decodeArm64Prog relaxRowIndirectScalarWords)[i]?) = chunkDecode relaxRowIndirectScalarChunks :=
    (funext chunkDecode_relax).symm
  refine ⟨fun fuel tgt => checkZeroGC_run_sound _ _ aug_zeroGC fuel sAug tgt hRunA,
          fun fuel tgt => checkZeroGC_run_sound _ _ init_zeroGC fuel sInit tgt hRunI,
          fun fuel tgt => checkZeroGC_run_sound _ _ refill_zeroGC fuel sRefill tgt hRunRf,
          fun fuel tgt => checkZeroGC_run_sound _ _ relax_zeroGC fuel sRelax tgt hRunRx,
          checkStackGuardWith_sound _ (chunkDecode augmentRowScalarChunks) _ (.largeFrame 112 316 315)
            chunkDecode_aug (by rw [hSzA]; exact aug_stackGuard) sAug hRunA hPcA hGA hHeadA.1 hHeadA.2,
          checkStackGuardWith_sound _ (chunkDecode initReducedRow64Chunks) _ (.smallFrame 87 86)
            chunkDecode_init (by rw [hSzI]; exact init_stackGuard) sInit hRunI hPcI hGI hSpI hGuardI,
          checkStackGuardWith_sound _ (chunkDecode refillQueueIndirectScalarChunks) _ (.smallFrame 233 232)
            chunkDecode_refill (by rw [hSzRf]; exact refill_stackGuard) sRefill hRunRf hPcRf hGRf hSpRf hGuardRf,
          checkStackGuardWith_sound _ (chunkDecode relaxRowIndirectScalarChunks) _ (.smallFrame 90 89)
            chunkDecode_relax (by rw [hSzRx]; exact relax_stackGuard) sRelax hRunRx hPcRx hGRx hSpRx hGuardRx,
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecode augmentRowScalarChunks) _ _ hBdA aug_pbCert fuel sAug hRunA (by rw [hPcA]; decide) (hGetA ▸ hHold),
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecode initReducedRow64Chunks) _ _ hBdI init_pbCert fuel sInit hRunI (by rw [hPcI]; decide) (hGetI ▸ hHold),
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecode refillQueueIndirectScalarChunks) _ _ hBdRf refill_pbCert fuel sRefill hRunRf (by rw [hPcRf]; decide) (hGetRf ▸ hHold),
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecode relaxRowIndirectScalarChunks) _ _ hBdRx relax_pbCert fuel sRelax hRunRx (by rw [hPcRx]; decide) (hGetRx ▸ hHold)⟩

/-- `augmentRowScalar` fast path (`done == nil` at saved stack slot `sp + 456`) unconditionally bypasses `runtime.selectnbrecv` (`pcIdx = 168`) directly to `pcIdx = 181`. -/
theorem augmentRowScalar_done_nil_no_selectnbrecv (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 161)
    (hDoneNil : readU64 s.mem (s.sp + 456#64) = 0#64) :
    let s' := runArm64 2 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets s
    s'.status = .running ∧ s'.pcIdx = 181 ∧ s'.status ≠ .callExternal "runtime.selectnbrecv" := by
  dsimp only
  have hSz : (decodeArm64Prog augmentRowScalarWords).size = 335 :=
    decodeChunks_size augmentRowScalarChunks 335 (by decide)
  have hGet161 : (decodeArm64Prog augmentRowScalarWords)[161]? = some (.ldrStrUImm true false 8 456 1#5 31#5) := by
    rw [← chunkDecode_aug]; decide
  have hGet162 : (decodeArm64Prog augmentRowScalarWords)[162]? = some (.cbz true false 1#5 19) := by
    rw [← chunkDecode_aug]; decide
  let s1 := advancePc (writeGpr s 1#5 (readU64 s.mem (readGprSP s 31#5 + BitVec.ofNat 64 456)))
  have hStep0 : stepArm64 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets s = s1 :=
    step_ldrStrUImm_load64 _ _ s 161 335 456 1#5 31#5 hRun hPc hSz hGet161
  have hRun1 : s1.status = .running := by simp [s1, status_advancePc, status_writeGpr, hRun]
  have hPc1 : s1.pcIdx = 162 := by simp [s1, pcIdx_advancePc, pcIdx_writeGpr, hPc]
  have hR1 : readGpr s1 1#5 = 0#64 := by
    rw [show s1 = advancePc (writeGpr s 1#5 _) from rfl, readGpr_advancePc,
      readGpr_writeGpr_eq _ _ _ (by decide)]
    have hSp : readGprSP s 31#5 = s.sp := rfl
    have hOff : BitVec.ofNat 64 456 = 456#64 := rfl
    rw [hSp, hOff, hDoneNil]
  have hCbzTaken : (if false then readGpr s1 1#5 != 0#64 else readGpr s1 1#5 == 0#64) = true := by
    simp [hR1]
  let s2 := { s1 with pcIdx := 181 }
  have hStep1 : stepArm64 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets s1 = s2 := by
    have hs := step_cbz64_taken (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets s1
      162 335 false 1#5 19 hRun1 hPc1 hSz hGet162 hCbzTaken (by decide) (by decide)
    exact hs
  rw [runArm64_succ_running _ _ _ _ hRun, hStep0, runArm64_succ_running _ _ _ _ hRun1, hStep1]
  simp only [runArm64, s2, hRun1]
  exact ⟨trivial, trivial, fun h => by cases h⟩

/-! ### Concrete Execution Bridges (`WordOverlay` and Exact `Rat` Evaluation) -/

def upsertWord (xs : List (BitVec 64 × BitVec 64)) (k v : BitVec 64) : List (BitVec 64 × BitVec 64) :=
  match xs with
  | [] => [(k, v)]
  | (k', v') :: rest => if k == k' then (k, v) :: rest else (k', v') :: upsertWord rest k v

def lookupWord (xs : List (BitVec 64 × BitVec 64)) (k : BitVec 64) : Option (BitVec 64) :=
  match xs with
  | [] => none
  | (k', v') :: rest => if k == k' then some v' else lookupWord rest k

def evalWordOverlay (base : BitVec 64 → BitVec 8) (ws : List (BitVec 64 × BitVec 64)) (a : BitVec 64) : BitVec 8 :=
  match lookupWord ws (a &&& 0xfffffffffffffff8#64) with
  | some w => (w >>> ((a &&& 7#64) * 8#64)).extractLsb' 0 8
  | none => base a

theorem lookupWord_upsert_eq (xs : List (BitVec 64 × BitVec 64)) (k v : BitVec 64) :
    lookupWord (upsertWord xs k v) k = some v := by
  induction xs with
  | nil => simp [upsertWord, lookupWord]
  | cons p rest ih =>
    rcases p with ⟨k', v'⟩
    by_cases hk : k == k'
    · simp [upsertWord, lookupWord, hk]
    · simp [upsertWord, lookupWord, hk, ih]

theorem lookupWord_upsert_ne (xs : List (BitVec 64 × BitVec 64)) (k v q : BitVec 64) (hne : q ≠ k) :
    lookupWord (upsertWord xs k v) q = lookupWord xs q := by
  induction xs with
  | nil => simp [upsertWord, lookupWord, beq_false_of_ne hne]
  | cons p rest ih =>
    rcases p with ⟨k', v'⟩
    by_cases hk : k == k'
    · have hk' : k = k' := eq_of_beq hk
      subst hk'
      simp [upsertWord, lookupWord, beq_false_of_ne hne]
    · by_cases hqk : q == k'
      · simp [upsertWord, lookupWord, hk, hqk]
      · simp [upsertWord, lookupWord, hk, hqk, ih]

private theorem and_7_toNat (x : BitVec 64) : (x &&& 7#64).toNat = x.toNat % 8 := by
  rw [BitVec.toNat_and]
  exact Nat.and_two_pow_sub_one_eq_mod x.toNat 3

private theorem and_not7_eq_shift (x : BitVec 64) :
    x &&& 0xfffffffffffffff8#64 = (x >>> 3) <<< 3 := by
  have hMask : (0xfffffffffffffff8#64 : BitVec 64) = ~~~(7#64) := by decide
  rw [hMask]
  apply BitVec.eq_of_getLsbD_eq
  intro i hi
  simp only [BitVec.getLsbD_and, BitVec.getLsbD_not, BitVec.getLsbD_shiftLeft,
    BitVec.getLsbD_ushiftRight, hi, decide_true, Bool.true_and]
  by_cases h3 : i < 3
  · match i, h3 with
    | 0, _ => simp
    | 1, _ => simp
    | 2, _ => simp
  · have hNot7 : (7#64 : BitVec 64).getLsbD i = false := by
      change Nat.testBit 7 i = false
      exact Nat.testBit_lt_two_pow (by
        have hPow : 2 ^ 3 ≤ 2 ^ i := Nat.pow_le_pow_right (by decide) (by omega)
        omega)
    have hSub : 3 + (i - 3) = i := by omega
    simp [h3, hNot7, hSub]

private theorem and_not7_toNat (x : BitVec 64) :
    (x &&& 0xfffffffffffffff8#64).toNat = (x.toNat / 8) * 8 := by
  rw [and_not7_eq_shift, BitVec.toNat_shiftLeft, BitVec.toNat_ushiftRight]
  have hlt := x.isLt
  have hShift : (x.toNat >>> 3) <<< 3 = (x.toNat / 8) * 8 := by
    rw [Nat.shiftRight_eq_div_pow, Nat.shiftLeft_eq]
  rw [hShift]
  exact Nat.mod_eq_of_lt (by omega)

private theorem bv64_aligned_bytes (addr x : BitVec 64) (hAligned : addr &&& 7#64 = 0#64) :
    ((x == addr || x == addr + 1#64 || x == addr + 2#64 || x == addr + 3#64 ||
      x == addr + 4#64 || x == addr + 5#64 || x == addr + 6#64 || x == addr + 7#64) = true) ↔
    (x &&& 0xfffffffffffffff8#64) = addr := by
  have hMod : addr.toNat % 8 = 0 := by
    have := congrArg BitVec.toNat hAligned
    rw [and_7_toNat] at this
    exact this
  have hAddrLt := addr.isLt
  have hXLt := x.isLt
  have hAddNat : ∀ k : Nat, k < 8 → (addr + BitVec.ofNat 64 k).toNat = addr.toNat + k := by
    intro k hk
    rw [BitVec.toNat_add, BitVec.toNat_ofNat]
    have hKMod : k % 2 ^ 64 = k := Nat.mod_eq_of_lt (by omega)
    rw [hKMod]
    exact Nat.mod_eq_of_lt (by omega)
  simp only [Bool.or_eq_true, beq_iff_eq]
  constructor
  · intro h
    apply BitVec.eq_of_toNat_eq
    rw [and_not7_toNat]
    rcases h with ((((((rfl | rfl) | rfl) | rfl) | rfl) | rfl) | rfl) | rfl
    · omega
    · have := hAddNat 1 (by decide); change ((addr + BitVec.ofNat 64 1).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 2 (by decide); change ((addr + BitVec.ofNat 64 2).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 3 (by decide); change ((addr + BitVec.ofNat 64 3).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 4 (by decide); change ((addr + BitVec.ofNat 64 4).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 5 (by decide); change ((addr + BitVec.ofNat 64 5).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 6 (by decide); change ((addr + BitVec.ofNat 64 6).toNat / 8) * 8 = addr.toNat; omega
    · have := hAddNat 7 (by decide); change ((addr + BitVec.ofNat 64 7).toNat / 8) * 8 = addr.toNat; omega
  · intro h
    have hDiv : (x.toNat / 8) * 8 = addr.toNat := by
      have := congrArg BitVec.toNat h
      rw [and_not7_toNat] at this
      exact this
    have hRem : x.toNat % 8 < 8 := Nat.mod_lt x.toNat (by decide)
    match hr : x.toNat % 8 with
    | 0 => left; left; left; left; left; left; left; exact BitVec.eq_of_toNat_eq (by omega)
    | 1 => left; left; left; left; left; left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 1 (by decide); change x.toNat = (addr + BitVec.ofNat 64 1).toNat; omega)
    | 2 => left; left; left; left; left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 2 (by decide); change x.toNat = (addr + BitVec.ofNat 64 2).toNat; omega)
    | 3 => left; left; left; left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 3 (by decide); change x.toNat = (addr + BitVec.ofNat 64 3).toNat; omega)
    | 4 => left; left; left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 4 (by decide); change x.toNat = (addr + BitVec.ofNat 64 4).toNat; omega)
    | 5 => left; left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 5 (by decide); change x.toNat = (addr + BitVec.ofNat 64 5).toNat; omega)
    | 6 => left; right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 6 (by decide); change x.toNat = (addr + BitVec.ofNat 64 6).toNat; omega)
    | 7 => right; exact BitVec.eq_of_toNat_eq (by have := hAddNat 7 (by decide); change x.toNat = (addr + BitVec.ofNat 64 7).toNat; omega)
    | _ + 8 => omega

private theorem extractLsb'_shiftRight_zero (v : BitVec 64) (k : Nat) (hk : k ≤ 56) :
    (v >>> (BitVec.ofNat 64 k)).extractLsb' 0 8 = v.extractLsb' k 8 := by
  change (v >>> (BitVec.ofNat 64 k).toNat).extractLsb' 0 8 = v.extractLsb' k 8
  apply BitVec.eq_of_getLsbD_eq
  intro i hi
  simp only [BitVec.getLsbD_extractLsb', BitVec.getLsbD_ushiftRight, hi, decide_true, Bool.true_and,
    BitVec.toNat_ofNat]
  have hMod : k % 2 ^ 64 = k := Nat.mod_eq_of_lt (by omega)
  rw [hMod, Nat.zero_add]

private theorem bv64_byte_extract (addr x v : BitVec 64) (hAligned : addr &&& 7#64 = 0#64)
    (hx : (x &&& 0xfffffffffffffff8#64) = addr) :
    (if x == addr + 7#64 then v.extractLsb' 56 8
     else if x == addr + 6#64 then v.extractLsb' 48 8
     else if x == addr + 5#64 then v.extractLsb' 40 8
     else if x == addr + 4#64 then v.extractLsb' 32 8
     else if x == addr + 3#64 then v.extractLsb' 24 8
     else if x == addr + 2#64 then v.extractLsb' 16 8
     else if x == addr + 1#64 then v.extractLsb' 8 8
     else v.extractLsb' 0 8) =
    (v >>> ((x &&& 7#64) * 8#64)).extractLsb' 0 8 := by
  have hMod : addr.toNat % 8 = 0 := by
    have := congrArg BitVec.toNat hAligned
    rw [and_7_toNat] at this; exact this
  have hDiv : (x.toNat / 8) * 8 = addr.toNat := by
    have := congrArg BitVec.toNat hx
    rw [and_not7_toNat] at this; exact this
  have hAddrLt := addr.isLt
  have hXLt := x.isLt
  have hAddNat : ∀ k : Nat, k < 8 → (addr + BitVec.ofNat 64 k).toNat = addr.toNat + k := by
    intro k hk
    rw [BitVec.toNat_add, BitVec.toNat_ofNat]
    have hKMod : k % 2 ^ 64 = k := Nat.mod_eq_of_lt (by omega)
    rw [hKMod]
    exact Nat.mod_eq_of_lt (by omega)
  have hNe : ∀ k j : Nat, k < 8 → j < 8 → k ≠ j → (addr + BitVec.ofNat 64 k == addr + BitVec.ofNat 64 j) = false := by
    intro k j hk hj hne
    apply beq_false_of_ne
    intro hEq
    have hN := congrArg BitVec.toNat hEq
    rw [hAddNat k hk, hAddNat j hj] at hN
    omega
  have hRemLt : x.toNat % 8 < 8 := Nat.mod_lt x.toNat (by decide)
  match hr : x.toNat % 8 with
  | 0 =>
    have hxEq : x = addr := BitVec.eq_of_toNat_eq (by omega)
    have h7 : x &&& 7#64 = 0#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    have h0 : ∀ j : Nat, 0 < j → j < 8 → (addr == addr + BitVec.ofNat 64 j) = false := by
      intro j hj1 hj2; simpa using hNe 0 j (by decide) hj2 (by omega)
    rw [h7, hxEq]
    simp only [h0 7 (by decide) (by decide), h0 6 (by decide) (by decide), h0 5 (by decide) (by decide),
      h0 4 (by decide) (by decide), h0 3 (by decide) (by decide), h0 2 (by decide) (by decide),
      h0 1 (by decide) (by decide), Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 0 (by decide)).symm
  | 1 =>
    have hxEq : x = addr + 1#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 1 (by decide); change x.toNat = (addr + BitVec.ofNat 64 1).toNat; omega)
    have h7 : x &&& 7#64 = 1#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 1 7 (by decide) (by decide) (by decide), hNe 1 6 (by decide) (by decide) (by decide),
      hNe 1 5 (by decide) (by decide) (by decide), hNe 1 4 (by decide) (by decide) (by decide),
      hNe 1 3 (by decide) (by decide) (by decide), hNe 1 2 (by decide) (by decide) (by decide),
      beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 8 (by decide)).symm
  | 2 =>
    have hxEq : x = addr + 2#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 2 (by decide); change x.toNat = (addr + BitVec.ofNat 64 2).toNat; omega)
    have h7 : x &&& 7#64 = 2#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 2 7 (by decide) (by decide) (by decide), hNe 2 6 (by decide) (by decide) (by decide),
      hNe 2 5 (by decide) (by decide) (by decide), hNe 2 4 (by decide) (by decide) (by decide),
      hNe 2 3 (by decide) (by decide) (by decide), beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 16 (by decide)).symm
  | 3 =>
    have hxEq : x = addr + 3#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 3 (by decide); change x.toNat = (addr + BitVec.ofNat 64 3).toNat; omega)
    have h7 : x &&& 7#64 = 3#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 3 7 (by decide) (by decide) (by decide), hNe 3 6 (by decide) (by decide) (by decide),
      hNe 3 5 (by decide) (by decide) (by decide), hNe 3 4 (by decide) (by decide) (by decide),
      beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 24 (by decide)).symm
  | 4 =>
    have hxEq : x = addr + 4#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 4 (by decide); change x.toNat = (addr + BitVec.ofNat 64 4).toNat; omega)
    have h7 : x &&& 7#64 = 4#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 4 7 (by decide) (by decide) (by decide), hNe 4 6 (by decide) (by decide) (by decide),
      hNe 4 5 (by decide) (by decide) (by decide), beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 32 (by decide)).symm
  | 5 =>
    have hxEq : x = addr + 5#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 5 (by decide); change x.toNat = (addr + BitVec.ofNat 64 5).toNat; omega)
    have h7 : x &&& 7#64 = 5#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 5 7 (by decide) (by decide) (by decide), hNe 5 6 (by decide) (by decide) (by decide),
      beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 40 (by decide)).symm
  | 6 =>
    have hxEq : x = addr + 6#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 6 (by decide); change x.toNat = (addr + BitVec.ofNat 64 6).toNat; omega)
    have h7 : x &&& 7#64 = 6#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [hNe 6 7 (by decide) (by decide) (by decide), beq_self_eq_true, Bool.false_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 48 (by decide)).symm
  | 7 =>
    have hxEq : x = addr + 7#64 := BitVec.eq_of_toNat_eq (by have := hAddNat 7 (by decide); change x.toNat = (addr + BitVec.ofNat 64 7).toNat; omega)
    have h7 : x &&& 7#64 = 7#64 := BitVec.eq_of_toNat_eq (by rw [and_7_toNat, hr]; rfl)
    rw [h7, hxEq]
    simp only [beq_self_eq_true, ↓reduceIte]
    exact (extractLsb'_shiftRight_zero v 56 (by decide)).symm
  | _ + 8 => omega

theorem writeU64_evalWordOverlay (base : BitVec 64 → BitVec 8) (ws : List (BitVec 64 × BitVec 64))
    (addr v : BitVec 64) (hAligned : addr &&& 7#64 = 0#64) :
    writeU64 (evalWordOverlay base ws) addr v = evalWordOverlay base (upsertWord ws addr v) := by
  funext x
  unfold writeU64 writeU8 evalWordOverlay
  by_cases hx : (x &&& 0xfffffffffffffff8#64) = addr
  · have hIn := (bv64_aligned_bytes addr x hAligned).mpr hx
    simp only [Bool.or_eq_true] at hIn
    have hLook : lookupWord (upsertWord ws addr v) (x &&& 0xfffffffffffffff8#64) = some v := by
      rw [hx]; exact lookupWord_upsert_eq ws addr v
    rw [hLook]
    have hExt := bv64_byte_extract addr x v hAligned hx
    by_cases h7 : (x == addr + 7#64) = true
    · simp only [h7, ↓reduceIte] at hExt ⊢; exact hExt
    · by_cases h6 : (x == addr + 6#64) = true
      · simp only [h7, h6, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
      · by_cases h5 : (x == addr + 5#64) = true
        · simp only [h7, h6, h5, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
        · by_cases h4 : (x == addr + 4#64) = true
          · simp only [h7, h6, h5, h4, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
          · by_cases h3 : (x == addr + 3#64) = true
            · simp only [h7, h6, h5, h4, h3, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
            · by_cases h2 : (x == addr + 2#64) = true
              · simp only [h7, h6, h5, h4, h3, h2, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
              · by_cases h1 : (x == addr + 1#64) = true
                · simp only [h7, h6, h5, h4, h3, h2, h1, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
                · have h0 : (x == addr) = true := by
                    simpa [h7, h6, h5, h4, h3, h2, h1] using hIn
                  simp only [h7, h6, h5, h4, h3, h2, h1, h0, Bool.false_eq_true, ↓reduceIte] at hExt ⊢; exact hExt
  · have hNotIn := mt (bv64_aligned_bytes addr x hAligned).mp hx
    have hLook : lookupWord (upsertWord ws addr v) (x &&& 0xfffffffffffffff8#64) =
        lookupWord ws (x &&& 0xfffffffffffffff8#64) :=
      lookupWord_upsert_ne ws addr v (x &&& 0xfffffffffffffff8#64) hx
    rw [hLook]
    have h7 : (x == addr + 7#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h6 : (x == addr + 6#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h5 : (x == addr + 5#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h4 : (x == addr + 4#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h3 : (x == addr + 3#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h2 : (x == addr + 2#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h1 : (x == addr + 1#64) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    have h0 : (x == addr) = false := Bool.eq_false_iff.mpr (fun h => hNotIn (by simp [h]))
    simp only [h7, h6, h5, h4, h3, h2, h1, h0, Bool.false_eq_true, ↓reduceIte]

/-! ### Concrete Overlay Machine Execution on Real Compiled Kernels -/

private theorem arm64State_ext {s1 s2 : Arm64State}
    (hRegs : ∀ r, s1.regs r = s2.regs r)
    (hSp : s1.sp = s2.sp)
    (hNzcv : s1.nzcv = s2.nzcv)
    (hFp : ∀ r, s1.fpRegs r = s2.fpRegs r)
    (hGuard : s1.gStackGuard0 = s2.gStackGuard0)
    (hMem : ∀ a, s1.mem a = s2.mem a)
    (hPc : s1.pcIdx = s2.pcIdx)
    (hBase : s1.baseAddr = s2.baseAddr)
    (hStat : s1.status = s2.status) :
    s1 = s2 := by
  rcases s1 with ⟨r1, sp1, nz1, fp1, g1, m1, p1, b1, st1⟩
  rcases s2 with ⟨r2, sp2, nz2, fp2, g2, m2, p2, b2, st2⟩
  dsimp at hRegs hSp hNzcv hFp hGuard hMem hPc hBase hStat
  have hr : r1 = r2 := funext hRegs
  have hf : fp1 = fp2 := funext hFp
  have hm : m1 = m2 := funext hMem
  subst hr hf hm hSp hNzcv hGuard hPc hBase hStat
  rfl

private theorem gcd_pow2 (m e : Nat) (h : e = 0 ∨ m % 2 = 1) : m.gcd (2 ^ e) = 1 := by
  rcases h with rfl | hOdd
  · simp
  · induction e with
    | zero => simp
    | succ k ih =>
      have hCop : Nat.Coprime m 2 := by
        unfold Nat.Coprime
        rw [Nat.gcd_comm, Nat.gcd_rec, hOdd]
        decide
      rw [Nat.pow_succ]
      exact Nat.Coprime.mul_right ih hCop

private theorem rat_normalize_coprime (num : Int) (den : Nat) (hNz : den ≠ 0) (hg : num.natAbs.gcd den = 1) :
    Rat.normalize num den hNz = ⟨num, den, hNz, hg⟩ := by
  rw [Rat.normalize_eq hNz]
  simp [hg]

private theorem rat_add_odd_pow2 (n1 : Int) (e1 : Nat) (n2 : Int) (e2 : Nat)
    (hLe : e1 ≤ e2)
    (h1 : e1 = 0 ∨ n1.natAbs % 2 = 1) (h2 : e2 = 0 ∨ n2.natAbs % 2 = 1)
    (hNumOdd : e2 = 0 ∨ (n1 * ↑(2 ^ (e2 - e1)) + n2).natAbs % 2 = 1) :
    mkRat n1 (2 ^ e1) + mkRat n2 (2 ^ e2) =
      mkRat (n1 * ↑(2 ^ (e2 - e1)) + n2) (2 ^ e2) := by
  have hp1 : (2 ^ e1 : Nat) ≠ 0 := Nat.ne_of_gt (Nat.pow_pos (by decide))
  have hp2 : (2 ^ e2 : Nat) ≠ 0 := Nat.ne_of_gt (Nat.pow_pos (by decide))
  have hg1 : n1.natAbs.gcd (2 ^ e1) = 1 := gcd_pow2 n1.natAbs e1 h1
  have hg2 : n2.natAbs.gcd (2 ^ e2) = 1 := gcd_pow2 n2.natAbs e2 h2
  have hk1 : mkRat n1 (2 ^ e1) = ⟨n1, 2 ^ e1, hp1, hg1⟩ := by
    rw [Rat.mkRat_def, dite_eq_right (fun h => absurd h hp1)]; exact rat_normalize_coprime _ _ hp1 hg1
  have hk2 : mkRat n2 (2 ^ e2) = ⟨n2, 2 ^ e2, hp2, hg2⟩ := by
    rw [Rat.mkRat_def, dite_eq_right (fun h => absurd h hp2)]; exact rat_normalize_coprime _ _ hp2 hg2
  let a := n1 * ↑(2 ^ (e2 - e1)) + n2
  have hgSum : a.natAbs.gcd (2 ^ e2) = 1 := gcd_pow2 a.natAbs e2 hNumOdd
  have hkSum : mkRat a (2 ^ e2) = ⟨a, 2 ^ e2, hp2, hgSum⟩ := by
    rw [Rat.mkRat_def, dite_eq_right (fun h => absurd h hp2)]; exact rat_normalize_coprime _ _ hp2 hgSum
  rw [hk1, hk2, hkSum, Rat.add_def, Rat.normalize_eq]
  have hPow2 : (↑(2 ^ e2) : Int) = ↑(2 ^ e1) * ↑(2 ^ (e2 - e1)) := by
    have hNat : 2 ^ e2 = 2 ^ e1 * 2 ^ (e2 - e1) := by
      rw [← Nat.pow_add, Nat.add_sub_cancel' hLe]
    exact_mod_cast hNat
  have hNum : n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1) = a * ↑(2 ^ e1) := by
    rw [hPow2]; dsimp [a]
    rw [Int.add_mul, Int.mul_assoc, Int.mul_comm (↑(2 ^ e1) : Int) ↑(2 ^ (e2 - e1)), ← Int.mul_assoc]
  have hNatAbs : (n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1)).natAbs = a.natAbs * 2 ^ e1 := by
    rw [hNum, Int.natAbs_mul]; rfl
  have hDen : 2 ^ e1 * 2 ^ e2 = 2 ^ e2 * 2 ^ e1 := Nat.mul_comm _ _
  have hGcd : (n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1)).natAbs.gcd (2 ^ e1 * 2 ^ e2) = 2 ^ e1 := by
    rw [hNatAbs, hDen, Nat.gcd_mul_right, hgSum, Nat.one_mul]
  congr 1
  · change (n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1)) /
      ↑((n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1)).natAbs.gcd (2 ^ e1 * 2 ^ e2)) = a
    rw [hGcd, hNum]
    exact Int.mul_ediv_cancel a (by exact_mod_cast hp1)
  · change (2 ^ e1 * 2 ^ e2) /
      ((n1 * ↑(2 ^ e2) + n2 * ↑(2 ^ e1)).natAbs.gcd (2 ^ e1 * 2 ^ e2)) = 2 ^ e2
    rw [hGcd, hDen]
    exact Nat.mul_div_left (2 ^ e2) (Nat.pow_pos (by decide : 0 < 2) : 0 < 2 ^ e1)

private theorem rat_half_eq : ((1 : Rat) / 2) = mkRat 1 (2 ^ 1) := by
  have h1 : (1 : Rat) = ⟨1, 1, by decide, by decide⟩ := rfl
  have h2 : (2 : Rat) = ⟨2, 1, by decide, by decide⟩ := rfl
  have hInv : (⟨2, 1, by decide, by decide⟩ : Rat)⁻¹ = ⟨1, 2, by decide, by decide⟩ := by
    rw [Rat.inv_def]; decide
  rw [Rat.div_def, h1, h2, hInv, Rat.mul_def]
  decide

private theorem uint64_ofNat_toNat_eq (a : BitVec 64) : UInt64.ofNat a.toNat = ⟨a⟩ := by
  apply congrArg UInt64.ofBitVec
  rw [BitVec.ofNat_toNat, BitVec.setWidth_eq]

private theorem f64Sub_posZero_right (a : BitVec 64) (q : Rat)
    (hNaN : f64IsNaN a = false) (hPosInf : f64IsPosInf a = false) (hNegInf : f64IsNegInf a = false)
    (hZero : f64IsZero a = false)
    (hRat : float64BitsToRat ⟨a⟩ = some q)
    (hBack : ratToFloat64Bits q false = a) :
    f64Sub a 0#64 = a := by
  unfold f64Sub f64Add
  have hNaN0 : f64IsNaN 0#64 = false := by decide
  have hNeg0 : f64Neg 0#64 = 0x8000000000000000#64 := by decide
  rw [hNeg0, uint64_ofNat_toNat_eq a, uint64_ofNat_toNat_eq 0x8000000000000000#64]
  have hFlags0 : f64IsNaN 0x8000000000000000#64 = false ∧ f64IsPosInf 0x8000000000000000#64 = false ∧
      f64IsNegInf 0x8000000000000000#64 = false := by decide
  have hZ0 : float64BitsToRat ⟨0x8000000000000000#64⟩ = some 0 := by decide
  simp only [hNaN, hNaN0, hPosInf, hNegInf, hFlags0.1, hFlags0.2.1, hFlags0.2.2, hZero,
    Bool.false_and, Bool.or_false, Bool.false_eq_true, ↓reduceIte,
    hRat, hZ0, Rat.add_zero, hBack]

private theorem f64Add_zero_left (b : BitVec 64) (q : Rat)
    (hNaN : f64IsNaN b = false) (hPosInf : f64IsPosInf b = false) (hNegInf : f64IsNegInf b = false)
    (hRat : float64BitsToRat ⟨b⟩ = some q)
    (hNegZ : (f64IsZero 0#64 && f64IsZero b && f64Sign 0#64 && f64Sign b) = false)
    (hBack : ratToFloat64Bits q false = b) :
    f64Add 0#64 b = b := by
  unfold f64Add
  rw [uint64_ofNat_toNat_eq 0#64, uint64_ofNat_toNat_eq b]
  have hFlags0 : f64IsNaN 0#64 = false ∧ f64IsPosInf 0#64 = false ∧ f64IsNegInf 0#64 = false := by decide
  have hZ0 : float64BitsToRat ⟨0#64⟩ = some 0 := by decide
  simp only [hNaN, hPosInf, hNegInf, hFlags0.1, hFlags0.2.1, hFlags0.2.2, Bool.false_and, Bool.or_false,
    Bool.false_eq_true, ↓reduceIte, hZ0, hRat, hNegZ, Rat.zero_add, hBack]

def lookup32 (rs : List (BitVec 64)) (r : BitVec 5) : BitVec 64 := rs.getD r.toNat 0#64

def regListOf (f : BitVec 5 → BitVec 64) : List (BitVec 64) :=
  (List.range 32).map fun i => f (BitVec.ofNat 5 i)

private theorem lookup32_regListOf (f : BitVec 5 → BitVec 64) (r : BitVec 5) :
    lookup32 (regListOf f) r = f r := by
  have hr : r.toNat < 32 := r.isLt
  unfold lookup32 regListOf
  rw [List.getD_eq_getElem?_getD, List.getElem?_map, List.getElem?_range hr]
  simp

private theorem lookup32_set (rs : List (BitVec 64)) (hLen : rs.length = 32) (r : BitVec 5) (v : BitVec 64) :
    ∀ r' : BitVec 5, lookup32 (rs.set r.toNat v) r' = if r' == r then v else lookup32 rs r' := by
  intro r'
  unfold lookup32
  have h1 : r.toNat < rs.length := hLen ▸ r.isLt
  have h2 : r'.toNat < rs.length := hLen ▸ r'.isLt
  by_cases hEq : r' = r
  · subst hEq
    rw [List.getD_eq_getElem?_getD, List.getElem?_set_self h1]
    simp
  · have hNeNat : r.toNat ≠ r'.toNat := fun h => hEq (BitVec.eq_of_toNat_eq h.symm)
    have hBeq : (r' == r) = false := Bool.eq_false_iff.mpr (fun h => hEq (eq_of_beq h))
    rw [List.getD_eq_getElem?_getD, List.getElem?_set_ne hNeNat, List.getD_eq_getElem?_getD, hBeq]
    simp [h2]

private theorem lookup32_set_eq (rs : List (BitVec 64)) (hLen : rs.length = 32) (r : BitVec 5) (v : BitVec 64) :
    lookup32 (rs.set r.toNat v) r = v := by
  rw [lookup32_set rs hLen r v r]; simp

private theorem lookup32_set_ne (rs : List (BitVec 64)) (hLen : rs.length = 32) (r r' : BitVec 5) (v : BitVec 64)
    (hNe : r' ≠ r) :
    lookup32 (rs.set r.toNat v) r' = lookup32 rs r' := by
  have hBeq : (r' == r) = false := Bool.eq_false_iff.mpr (fun h => hNe (eq_of_beq h))
  rw [lookup32_set rs hLen r v r', hBeq]; rfl

private theorem list_set_idem {α : Type} (xs : List α) (i : Nat) (v : α) :
    (xs.set i v).set i v = xs.set i v := by
  induction xs generalizing i with
  | nil => rfl
  | cons x xs ih =>
    cases i with
    | zero => rfl
    | succ k => simp [List.set, ih]

/-- Concrete list-backed ARM64 execution state using explicit sparse word memory overlays. -/
structure OverlayState where
  regs : List (BitVec 64)
  fpRegs : List (BitVec 64)
  sp : BitVec 64
  nzcv : NZCV
  gStackGuard0 : BitVec 64
  ws : List (BitVec 64 × BitVec 64)
  pcIdx : Nat
  baseAddr : BitVec 64
  status : ExecStatus
  deriving DecidableEq, Repr, Inhabited

def OverlayState.toArm64 (base : BitVec 64 → BitVec 8) (c : OverlayState) : Arm64State :=
  { regs := lookup32 c.regs
  , sp := c.sp
  , nzcv := c.nzcv
  , fpRegs := lookup32 c.fpRegs
  , gStackGuard0 := c.gStackGuard0
  , mem := evalWordOverlay base c.ws
  , pcIdx := c.pcIdx
  , baseAddr := c.baseAddr
  , status := c.status }

def OverlayState.ofArm64 (ws : List (BitVec 64 × BitVec 64)) (s : Arm64State) : OverlayState :=
  { regs := regListOf s.regs
  , fpRegs := regListOf s.fpRegs
  , sp := s.sp
  , nzcv := s.nzcv
  , gStackGuard0 := s.gStackGuard0
  , ws := ws
  , pcIdx := s.pcIdx
  , baseAddr := s.baseAddr
  , status := s.status }

theorem toArm64_ofArm64 (base : BitVec 64 → BitVec 8) (ws : List (BitVec 64 × BitVec 64)) (s : Arm64State)
    (hMem : ∀ a, s.mem a = evalWordOverlay base ws a) :
    (OverlayState.ofArm64 ws s).toArm64 base = s := by
  apply arm64State_ext
  · intro r; exact lookup32_regListOf s.regs r
  · rfl
  · rfl
  · intro r; exact lookup32_regListOf s.fpRegs r
  · rfl
  · intro a; exact (hMem a).symm
  · rfl
  · rfl
  · rfl

private theorem toArm64_with_ctrl (base : BitVec 64 → BitVec 8) (c : OverlayState) (p : Nat) (b : BitVec 64) (st : ExecStatus) :
    { c.toArm64 base with pcIdx := p, baseAddr := b, status := st } =
      ({ c with pcIdx := p, baseAddr := b, status := st }).toArm64 base := rfl

def isPureNonStoreInsn : Arm64Insn → Bool
  | .ldrStrPair false .. => false
  | .ldrStrUImm false .. => false
  | .ldrStrImm false .. => false
  | .ldrStrReg false .. => false
  | _ => true

private theorem advancePc_mem (s : Arm64State) : (advancePc s).mem = s.mem := rfl
private theorem writeGpr_mem (s : Arm64State) (r : BitVec 5) (v : BitVec 64) : (writeGpr s r v).mem = s.mem := by
  unfold writeGpr; split <;> rfl
private theorem writeFp_mem (s : Arm64State) (r : BitVec 5) (v : BitVec 64) : (writeFp s r v).mem = s.mem := rfl

private theorem jumpRel_mem (progLen : Nat) (ext : Nat → Option String) (isLink : Bool) (s : Arm64State) (imm : Int) :
    (jumpRel progLen ext isLink s imm).mem = s.mem := by
  unfold jumpRel; dsimp only
  have hLink : (if isLink = true then writeGpr s 30#5 (s.baseAddr + BitVec.ofNat 64 (4 * (s.pcIdx + 1))) else s).mem = s.mem := by
    cases isLink <;> simp only [↓reduceIte, Bool.false_eq_true, writeGpr_mem]
  split
  · exact hLink
  · cases ext s.pcIdx with
    | none => exact hLink
    | some _ =>
      dsimp only
      split <;> try exact hLink
      split <;> try exact hLink
      split <;> exact hLink

private theorem if_jump_adv_mem (take : Bool) (progLen : Nat) (ext : Nat → Option String) (s : Arm64State) (imm : Int) :
    (if take = true then jumpRel progLen ext false s imm else advancePc s).mem = s.mem := by
  cases take <;> simp only [↓reduceIte, Bool.false_eq_true, jumpRel_mem, advancePc_mem]

private theorem loadStep_mem (s : Arm64State) (isFP : Bool) (sz : Nat) (rt : BitVec 5) (v64 v8 : BitVec 64) :
    (if (sz == 8) = true then (if isFP = true then writeFp s rt v64 else writeGpr s rt v64) else writeGpr s rt v8).mem = s.mem := by
  cases (sz == 8) <;> cases isFP <;> simp only [↓reduceIte, Bool.false_eq_true, writeGpr_mem, writeFp_mem]

private theorem pairLoadStep_mem (s : Arm64State) (isFP : Bool) (rt rt2 : BitVec 5) (v1 v2 : BitVec 64) :
    (if isFP = true then writeFp (writeFp s rt v1) rt2 v2 else writeGpr (writeGpr s rt v1) rt2 v2).mem = s.mem := by
  cases isFP <;> simp only [↓reduceIte, Bool.false_eq_true, writeFp_mem, writeGpr_mem]

private theorem mode_mem (s1 : Arm64State) (mode : MemIndexMode) (rn : BitVec 5) (v : BitVec 64)
    {m : BitVec 64 → BitVec 8} (h : s1.mem = m) :
    (match mode with | .offset => s1 | .post | .pre => writeGprSP s1 rn v).mem = m := by
  cases mode <;> simp only [mem_writeGprSP, h]

private theorem execArm64Insn_pure_mem (progLen : Nat) (ext : Nat → Option String)
    (insn : Arm64Insn) (s : Arm64State) (h : isPureNonStoreInsn insn = true) :
    (execArm64Insn progLen ext insn s).mem = s.mem := by
  cases insn
  case ldrStrPair isLoad isFP mode _ rt rt2 rn =>
    cases isLoad
    · simp [isPureNonStoreInsn] at h
    · unfold execArm64Insn; rw [advancePc_mem]; dsimp only; simp only [↓reduceIte]
      exact mode_mem _ mode rn _ (pairLoadStep_mem s isFP rt rt2 _ _)
  case ldrStrUImm isLoad isFP sz _ rt _ =>
    cases isLoad
    · simp [isPureNonStoreInsn] at h
    · unfold execArm64Insn; rw [advancePc_mem]; dsimp only; simp only [↓reduceIte]
      exact loadStep_mem s isFP sz rt _ _
  case ldrStrImm isLoad isFP sz mode _ rt rn =>
    cases isLoad
    · simp [isPureNonStoreInsn] at h
    · unfold execArm64Insn; rw [advancePc_mem]; dsimp only; simp only [↓reduceIte]
      exact mode_mem _ mode rn _ (loadStep_mem s isFP sz rt _ _)
  case ldrStrReg isLoad isSigned isFP sz _ _ rt _ _ =>
    cases isLoad
    · simp [isPureNonStoreInsn] at h
    · unfold execArm64Insn; rw [advancePc_mem]; dsimp only; simp only [↓reduceIte]
      exact loadStep_mem s isFP sz rt _ _
  case b _ => exact jumpRel_mem _ _ _ _ _
  case bl _ => exact jumpRel_mem _ _ _ _ _
  case bCond _ _ => exact if_jump_adv_mem _ _ _ _ _
  case cbz _ _ _ _ => exact if_jump_adv_mem _ _ _ _ _
  case tbz _ _ _ _ => exact if_jump_adv_mem _ _ _ _ _
  case addSubImm _ _ setFlags _ _ _ =>
    unfold execArm64Insn; dsimp only; rw [advancePc_mem]
    cases setFlags <;> simp only [↓reduceIte, Bool.false_eq_true, writeGpr_mem, mem_writeGprSP]
  case addSubExt _ _ setFlags _ _ _ _ _ =>
    unfold execArm64Insn; dsimp only; rw [advancePc_mem]
    cases setFlags <;> simp only [↓reduceIte, Bool.false_eq_true, writeGpr_mem, mem_writeGprSP]
  case addSubReg _ _ setFlags _ _ _ _ _ =>
    unfold execArm64Insn; dsimp only; rw [advancePc_mem]
    cases setFlags <;> simp only [↓reduceIte, Bool.false_eq_true, writeGpr_mem]
  case logicalImm _ opc _ _ _ =>
    unfold execArm64Insn; dsimp only; rw [advancePc_mem]
    by_cases h3 : (opc == 3) = true <;> simp only [h3, ↓reduceIte, Bool.false_eq_true, writeGpr_mem, mem_writeGprSP]
  case logicalReg _ _ opc _ _ _ _ _ =>
    unfold execArm64Insn; dsimp only; rw [advancePc_mem]
    by_cases h3 : (opc == 3) = true <;> simp only [h3, ↓reduceIte, Bool.false_eq_true, writeGpr_mem]
  case fpIntConv toInt isSigned gprToFp _ _ =>
    unfold execArm64Insn; dsimp only
    cases toInt <;> cases gprToFp <;> cases isSigned <;> simp only [advancePc_mem, ↓reduceIte, Bool.false_eq_true, writeGpr_mem, writeFp_mem]
  case div is64 _ _ _ _ =>
    cases is64
    · unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte, advancePc_mem, writeGpr_mem]
    · unfold execArm64Insn; dsimp only; simp only [↓reduceIte, advancePc_mem, writeGpr_mem]
  case nop => rfl
  case ret _ => unfold execArm64Insn; dsimp only; split <;> rfl
  case unknown _ => rfl
  case fpUnaryD _ _ _ => unfold execArm64Insn; dsimp only; split <;> first | rfl | simp only [advancePc_mem, writeFp_mem]
  case fpBinaryD _ _ _ _ => unfold execArm64Insn; dsimp only; split <;> first | rfl | simp only [advancePc_mem, writeFp_mem]
  all_goals
    unfold execArm64Insn; dsimp only; simp only [advancePc_mem, writeGpr_mem]

def stepStoreOverlay (base : BitVec 64 → BitVec 8) (c : OverlayState) (insn : Arm64Insn) : Option OverlayState :=
  let s := c.toArm64 base
  match insn with
  | .ldrStrImm false isFP _ mode imm9 rt rn =>
    let b := readGprSP s rn
    let addr := match mode with | .post => b | .offset | .pre => b + BitVec.ofInt 64 imm9
    if (addr &&& 7#64) == 0#64 then
      let v := if isFP then s.fpRegs rt else readGpr s rt
      let s1 := { s with mem := evalWordOverlay base (upsertWord c.ws addr v) }
      let s2 := match mode with | .offset => s1 | .post | .pre => writeGprSP s1 rn (b + BitVec.ofInt 64 imm9)
      some (OverlayState.ofArm64 (upsertWord c.ws addr v) (advancePc s2))
    else none
  | .ldrStrUImm false isFP sz imm rt rn =>
    if sz == 8 then
      let addr := readGprSP s rn + BitVec.ofNat 64 imm
      if (addr &&& 7#64) == 0#64 then
        let v := if isFP then s.fpRegs rt else readGpr s rt
        let s1 := { s with mem := evalWordOverlay base (upsertWord c.ws addr v) }
        some (OverlayState.ofArm64 (upsertWord c.ws addr v) (advancePc s1))
      else none
    else none
  | .ldrStrReg false _ isFP sz extKind scale rt rn rm =>
    if sz == 8 then
      let addr := calcRegOffsetAddr (readGprSP s rn) (readGpr s rm) extKind scale 8
      if (addr &&& 7#64) == 0#64 then
        let v := if isFP then s.fpRegs rt else readGpr s rt
        let s1 := { s with mem := evalWordOverlay base (upsertWord c.ws addr v) }
        some (OverlayState.ofArm64 (upsertWord c.ws addr v) (advancePc s1))
      else none
    else none
  | _ => none

private theorem stepStoreOverlay_sound (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c c' : OverlayState) (insn : Arm64Insn)
    (hNotPure : isPureNonStoreInsn insn = false)
    (h : stepStoreOverlay base c insn = some c') :
    execArm64Insn progLen ext insn (c.toArm64 base) = c'.toArm64 base := by
  cases insn <;> simp only [isPureNonStoreInsn] at hNotPure <;> try contradiction
  case ldrStrUImm isLoad isFP sz imm rt rn =>
    cases isLoad
    · unfold stepStoreOverlay at h; dsimp only at h
      split at h <;> try contradiction
      split at h <;> try contradiction
      cases h; rename_i hSz hAl
      have hSzEq : sz = 8 := eq_of_beq hSz
      subst hSzEq
      have hAlEq : _ = 0#64 := of_decide_eq_true hAl
      have hW := writeU64_evalWordOverlay base c.ws (readGprSP (c.toArm64 base) rn + BitVec.ofNat 64 imm)
        (if isFP then (c.toArm64 base).fpRegs rt else readGpr (c.toArm64 base) rt) hAlEq
      unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte, beq_self_eq_true]
      have hMemEq : (c.toArm64 base).mem = evalWordOverlay base c.ws := rfl
      rw [hMemEq, hW, toArm64_ofArm64]; intro a; rfl
    · contradiction
  case ldrStrImm isLoad isFP sz mode imm9 rt rn =>
    cases isLoad
    · unfold stepStoreOverlay at h; dsimp only at h
      cases mode <;> dsimp only at h
      · split at h <;> try contradiction
        cases h; rename_i hAl
        have hAlEq : _ = 0#64 := of_decide_eq_true hAl
        have hW := writeU64_evalWordOverlay base c.ws (readGprSP (c.toArm64 base) rn + BitVec.ofInt 64 imm9)
          (if isFP then (c.toArm64 base).fpRegs rt else readGpr (c.toArm64 base) rt) hAlEq
        unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte]
        have hMemEq : (c.toArm64 base).mem = evalWordOverlay base c.ws := rfl
        rw [hMemEq, hW, toArm64_ofArm64]; intro a; rfl
      · split at h <;> try contradiction
        cases h; rename_i hAl
        have hAlEq : _ = 0#64 := of_decide_eq_true hAl
        have hW := writeU64_evalWordOverlay base c.ws (readGprSP (c.toArm64 base) rn)
          (if isFP then (c.toArm64 base).fpRegs rt else readGpr (c.toArm64 base) rt) hAlEq
        unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte]
        have hMemEq : (c.toArm64 base).mem = evalWordOverlay base c.ws := rfl
        rw [hMemEq, hW, toArm64_ofArm64]; intro a; rw [advancePc_mem, mem_writeGprSP]
      · split at h <;> try contradiction
        cases h; rename_i hAl
        have hAlEq : _ = 0#64 := of_decide_eq_true hAl
        have hW := writeU64_evalWordOverlay base c.ws (readGprSP (c.toArm64 base) rn + BitVec.ofInt 64 imm9)
          (if isFP then (c.toArm64 base).fpRegs rt else readGpr (c.toArm64 base) rt) hAlEq
        unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte]
        have hMemEq : (c.toArm64 base).mem = evalWordOverlay base c.ws := rfl
        rw [hMemEq, hW, toArm64_ofArm64]; intro a; rw [advancePc_mem, mem_writeGprSP]
    · contradiction
  case ldrStrReg isLoad isSigned isFP sz extK scale rt rn rm =>
    cases isLoad
    · unfold stepStoreOverlay at h; dsimp only at h
      split at h <;> try contradiction
      split at h <;> try contradiction
      cases h; rename_i hSz hAl
      have hSzEq : sz = 8 := eq_of_beq hSz
      subst hSzEq
      have hAlEq : _ = 0#64 := of_decide_eq_true hAl
      have hW := writeU64_evalWordOverlay base c.ws
        (calcRegOffsetAddr (readGprSP (c.toArm64 base) rn) (readGpr (c.toArm64 base) rm) extK scale 8)
        (if isFP then (c.toArm64 base).fpRegs rt else readGpr (c.toArm64 base) rt) hAlEq
      unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte, beq_self_eq_true]
      have hMemEq : (c.toArm64 base).mem = evalWordOverlay base c.ws := rfl
      rw [hMemEq, hW, toArm64_ofArm64]; intro a; rfl
    · contradiction

def stepOverlay (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) : Option OverlayState :=
  if c.status != .running then some c
  else if c.pcIdx < progLen then
    match chunkWord chunks 16 c.pcIdx with
    | none => some { c with status := .trapIllegal }
    | some w =>
      let insn := decodeArm64Word w
      if isPureNonStoreInsn insn then
        some (OverlayState.ofArm64 c.ws (execArm64Insn progLen ext insn (c.toArm64 base)))
      else
        stepStoreOverlay base c insn
  else some { c with status := .trapIllegal }

def runOverlay (fuel : Nat) (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) : Option OverlayState :=
  match fuel with
  | 0 => some c
  | k + 1 =>
    match stepOverlay chunks progLen ext base c with
    | some c' => runOverlay k chunks progLen ext base c'
    | none => none

theorem stepOverlay_sound (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c c' : OverlayState)
    (h : stepOverlay chunks progLen ext base c = some c') :
    stepChunked chunks 16 progLen ext (c.toArm64 base) = c'.toArm64 base := by
  unfold stepOverlay at h
  unfold stepChunked
  by_cases hRun : (c.status != .running) = true
  · simp [hRun] at h; cases h; change (if c.status != .running then c.toArm64 base else _) = _; simp [hRun]
  · simp only [hRun, Bool.false_eq_true, ↓reduceIte] at h
    have hRunS : ((c.toArm64 base).status != .running) = false := Bool.eq_false_iff.mpr hRun
    simp only [hRunS, Bool.false_eq_true, ↓reduceIte]
    by_cases hLt : c.pcIdx < progLen
    · have hLtS : (c.toArm64 base).pcIdx < progLen := hLt
      simp only [hLt, ↓reduceIte] at h
      simp only [hLtS, ↓reduceIte]
      cases hCW : chunkWord chunks 16 c.pcIdx with
      | none =>
        rw [hCW] at h; cases h
        change (match chunkWord chunks 16 c.pcIdx with | some _ => _ | none => _) = _
        rw [hCW]; rfl
      | some w =>
        rw [hCW] at h; dsimp only at h
        change (match chunkWord chunks 16 c.pcIdx with | some _ => _ | none => _) = _
        rw [hCW]; dsimp only
        by_cases hPure : isPureNonStoreInsn (decodeArm64Word w) = true
        · simp [hPure] at h; cases h
          rw [toArm64_ofArm64]
          intro a
          exact congrFun (execArm64Insn_pure_mem progLen ext (decodeArm64Word w) (c.toArm64 base) hPure) a
        · simp only [hPure, Bool.false_eq_true, ↓reduceIte] at h
          have hNotPure : isPureNonStoreInsn (decodeArm64Word w) = false := Bool.eq_false_iff.mpr hPure
          exact stepStoreOverlay_sound progLen ext base c c' (decodeArm64Word w) hNotPure h
    · simp only [hLt, ↓reduceIte] at h; cases h
      have hLtF : ¬ (c.toArm64 base).pcIdx < progLen := hLt
      simp [hLtF]; rfl

theorem runOverlay_sound (fuel : Nat) (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c c' : OverlayState)
    (h : runOverlay fuel chunks progLen ext base c = some c') :
    runChunked fuel chunks 16 progLen ext (c.toArm64 base) = c'.toArm64 base := by
  induction fuel generalizing c with
  | zero => simp [runOverlay, runChunked] at h ⊢; cases h; rfl
  | succ k ih =>
    unfold runOverlay at h
    cases hStep : stepOverlay chunks progLen ext base c with
    | none => simp [hStep] at h
    | some cMid =>
      simp only [hStep] at h
      rw [runChunked, stepOverlay_sound chunks progLen ext base c cMid hStep]
      exact ih cMid h

private theorem stepChunked_fpBinary (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) (op : Nat) (fd fn fm : BitVec 5) (w : UInt32) (res : BitVec 64)
    (hStat : c.status = .running) (hLt : c.pcIdx < progLen) (hFpLen : c.fpRegs.length = 32)
    (hCW : chunkWord chunks 16 c.pcIdx = some w) (hDec : decodeArm64Word w = .fpBinaryD op fd fn fm)
    (hOp : (op = 2 ∧ f64Add (lookup32 c.fpRegs fn) (lookup32 c.fpRegs fm) = res) ∨
           (op = 3 ∧ f64Sub (lookup32 c.fpRegs fn) (lookup32 c.fpRegs fm) = res)) :
    stepChunked chunks 16 progLen ext (c.toArm64 base) =
      ({ c with fpRegs := c.fpRegs.set fd.toNat res, pcIdx := c.pcIdx + 1 }).toArm64 base := by
  unfold stepChunked
  simp only [OverlayState.toArm64, hStat, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte, hLt, hCW, hDec]
  unfold execArm64Insn; dsimp only
  rcases hOp with ⟨rfl, hVal⟩ | ⟨rfl, hVal⟩ <;> rw [hVal] <;>
    (apply arm64State_ext; intro r; rfl; rfl; rfl; intro r; exact (lookup32_set c.fpRegs hFpLen fd res r).symm; rfl; intro a; rfl; rfl; rfl; rfl)

private theorem runChunked_fpBinary (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) (op : Nat) (fd fn fm : BitVec 5) (fdNat nextPc : Nat)
    (w : UInt32) (res : BitVec 64) (hFd : fd.toNat = fdNat) (hNext : c.pcIdx + 1 = nextPc)
    (hStat : c.status = .running) (hLt : c.pcIdx < progLen) (hFpLen : c.fpRegs.length = 32)
    (hCW : chunkWord chunks 16 c.pcIdx = some w) (hDec : decodeArm64Word w = .fpBinaryD op fd fn fm)
    (hOp : (op = 2 ∧ f64Add (lookup32 c.fpRegs fn) (lookup32 c.fpRegs fm) = res) ∨
           (op = 3 ∧ f64Sub (lookup32 c.fpRegs fn) (lookup32 c.fpRegs fm) = res)) :
    runChunked 1 chunks 16 progLen ext (c.toArm64 base) =
      ({ c with fpRegs := c.fpRegs.set fdNat res, pcIdx := nextPc }).toArm64 base := by
  rw [← hFd, ← hNext]
  exact stepChunked_fpBinary chunks progLen ext base c op fd fn fm w res hStat hLt hFpLen hCW hDec hOp

private theorem runChunked_fpRound (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) (op : Nat) (mode : RoundMode) (fd fn : BitVec 5)
    (fdNat nextPc : Nat) (w : UInt32) (res : BitVec 64)
    (hFd : fd.toNat = fdNat) (hNext : c.pcIdx + 1 = nextPc)
    (hStat : c.status = .running) (hLt : c.pcIdx < progLen) (hFpLen : c.fpRegs.length = 32)
    (hCW : chunkWord chunks 16 c.pcIdx = some w) (hDec : decodeArm64Word w = .fpUnaryD op fd fn)
    (hMode : (op = 9 ∧ mode = .ceil) ∨ (op = 10 ∧ mode = .floor) ∨ (op = 12 ∧ mode = .nearestAway))
    (hVal : f64RoundToInt mode (lookup32 c.fpRegs fn) = res) :
    runChunked 1 chunks 16 progLen ext (c.toArm64 base) =
      ({ c with fpRegs := c.fpRegs.set fdNat res, pcIdx := nextPc }).toArm64 base := by
  rw [← hFd, ← hNext]
  unfold runChunked stepChunked
  simp only [OverlayState.toArm64, hStat, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte, hLt, hCW, hDec]
  unfold execArm64Insn; dsimp only
  rcases hMode with ⟨rfl, rfl⟩ | ⟨rfl, rfl⟩ | ⟨rfl, rfl⟩ <;> rw [hVal] <;>
    (apply arm64State_ext; intro r; rfl; rfl; rfl; intro r; exact (lookup32_set c.fpRegs hFpLen fd res r).symm; rfl; intro a; rfl; rfl; rfl; rfl)

private theorem runChunked_fpToInt64 (chunks : List (List UInt32)) (progLen : Nat) (ext : Nat → Option String)
    (base : BitVec 64 → BitVec 8) (c : OverlayState) (isSigned : Bool) (rd fn : BitVec 5)
    (rdNat nextPc : Nat) (w : UInt32) (res : BitVec 64)
    (hRdNat : rd.toNat = rdNat) (hNext : c.pcIdx + 1 = nextPc)
    (hStat : c.status = .running) (hLt : c.pcIdx < progLen) (hRegLen : c.regs.length = 32)
    (hRd : rd ≠ 31#5)
    (hCW : chunkWord chunks 16 c.pcIdx = some w) (hDec : decodeArm64Word w = .fpIntConv true isSigned false rd fn)
    (hVal : f64ToInt64 (lookup32 c.fpRegs fn) = res) :
    runChunked 1 chunks 16 progLen ext (c.toArm64 base) =
      ({ c with regs := c.regs.set rdNat res, pcIdx := nextPc }).toArm64 base := by
  rw [← hRdNat, ← hNext]
  have hRdB : (rd == 31#5) = false := Bool.eq_false_iff.mpr (fun h => hRd (eq_of_beq h))
  unfold runChunked stepChunked
  simp only [OverlayState.toArm64, hStat, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte, hLt, hCW, hDec]
  unfold execArm64Insn; dsimp only
  simp only [↓reduceIte, hVal]; unfold advancePc writeGpr; rw [hRdB]; dsimp only
  apply arm64State_ext
  · intro r; exact (lookup32_set c.regs hRegLen rd res r).symm
  · rfl
  · rfl
  · intro r; rfl
  · rfl
  · intro a; rfl
  · rfl
  · rfl
  · rfl

def zeroBase : BitVec 64 → BitVec 8 := fun _ => 0#8

def sparse32 (ps : List (Nat × BitVec 64)) : List (BitVec 64) :=
  (List.range 32).map fun i => match ps.find? (fun p => p.1 == i) with | some (_, v) => v | none => 0#64

private theorem f64RoundToInt_ceil (b res : BitVec 64) (q : Rat) (z : Int)
    (hBypass : (f64IsNaN b || f64Exp b == 2047 || f64IsZero b) = false)
    (hRat : float64BitsToRat ⟨b⟩ = some q)
    (hZ : q.ceil = z) (hNz : (z == 0) = false) (hBack : ratToFloat64Bits ↑z false = res) :
    f64RoundToInt .ceil b = res := by
  simp only [Bool.or_eq_false_iff] at hBypass
  unfold f64RoundToInt; rw [uint64_ofNat_toNat_eq b]
  simp only [hBypass.1.1, hBypass.1.2, hBypass.2, Bool.or_false, Bool.false_eq_true, ↓reduceIte, hRat, hZ, hNz, hBack]

private theorem f64RoundToInt_floor (b res : BitVec 64) (q : Rat) (z : Int)
    (hBypass : (f64IsNaN b || f64Exp b == 2047 || f64IsZero b) = false)
    (hRat : float64BitsToRat ⟨b⟩ = some q)
    (hZ : q.floor = z) (hNz : (z == 0) = false) (hBack : ratToFloat64Bits ↑z false = res) :
    f64RoundToInt .floor b = res := by
  simp only [Bool.or_eq_false_iff] at hBypass
  unfold f64RoundToInt; rw [uint64_ofNat_toNat_eq b]
  simp only [hBypass.1.1, hBypass.1.2, hBypass.2, Bool.or_false, Bool.false_eq_true, ↓reduceIte, hRat, hZ, hNz, hBack]

private theorem f64RoundToInt_nearestAway_nonneg (b res : BitVec 64) (q : Rat) (z : Int)
    (hBypass : (f64IsNaN b || f64Exp b == 2047 || f64IsZero b) = false)
    (hRat : float64BitsToRat ⟨b⟩ = some q) (hGe : q ≥ 0)
    (hZ : (q + mkRat 1 2).floor = z) (hNz : (z == 0) = false) (hBack : ratToFloat64Bits ↑z false = res) :
    f64RoundToInt .nearestAway b = res := by
  simp only [Bool.or_eq_false_iff] at hBypass
  unfold f64RoundToInt; rw [uint64_ofNat_toNat_eq b]
  simp only [hBypass.1.1, hBypass.1.2, hBypass.2, Bool.or_false, Bool.false_eq_true, ↓reduceIte, hRat, hGe, rat_half_eq, hZ, hNz, hBack]

private theorem f64ToInt64_exact (b res : BitVec 64) (q : Rat) (z : Int)
    (hNaN : f64IsNaN b = false) (hPosInf : f64IsPosInf b = false) (hNegInf : f64IsNegInf b = false)
    (hRat : float64BitsToRat ⟨b⟩ = some q)
    (hZ : (if q ≥ 0 then q.floor else q.ceil) = z)
    (hHi : ¬ (z ≥ ↑(1 <<< 63))) (hLo : ¬ (z < -↑(1 <<< 63)))
    (hBV : BitVec.ofInt 64 z = res) :
    f64ToInt64 b = res := by
  unfold f64ToInt64
  rw [uint64_ofNat_toNat_eq b]
  simp only [hNaN, hPosInf, hNegInf, Bool.false_eq_true, ↓reduceIte, hRat, hZ, hHi, hLo, hBV]

private theorem f64Add_mkRat_pow2 (a b res : BitVec 64) (na nb : Int) (ea eb : Nat)
    (hNaN : f64IsNaN a = false ∧ f64IsPosInf a = false ∧ f64IsNegInf a = false ∧
            f64IsNaN b = false ∧ f64IsPosInf b = false ∧ f64IsNegInf b = false ∧
            (f64IsZero a && f64IsZero b && f64Sign a && f64Sign b) = false)
    (hA : float64BitsToRat ⟨a⟩ = some (mkRat na (2 ^ ea)))
    (hB : float64BitsToRat ⟨b⟩ = some (mkRat nb (2 ^ eb)))
    (hLe : ea ≤ eb)
    (hO1 : ea = 0 ∨ na.natAbs % 2 = 1) (hO2 : eb = 0 ∨ nb.natAbs % 2 = 1)
    (hOS : eb = 0 ∨ (na * ↑(2 ^ (eb - ea)) + nb).natAbs % 2 = 1)
    (hBack : ratToFloat64Bits (mkRat (na * ↑(2 ^ (eb - ea)) + nb) (2 ^ eb)) false = res) :
    f64Add a b = res := by
  unfold f64Add
  rw [uint64_ofNat_toNat_eq a, uint64_ofNat_toNat_eq b]
  simp only [hNaN.1, hNaN.2.1, hNaN.2.2.1, hNaN.2.2.2.1, hNaN.2.2.2.2.1, hNaN.2.2.2.2.2.1,
    hNaN.2.2.2.2.2.2, Bool.false_and, Bool.or_false, Bool.false_eq_true, ↓reduceIte, hA, hB]
  rw [rat_add_odd_pow2 na ea nb eb hLe hO1 hO2 hOS, hBack]

private theorem f64Sub_1_0 : f64Sub 0x3ff0000000000000#64 0#64 = 0x3ff0000000000000#64 :=
  f64Sub_posZero_right _ 1 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

private theorem f64Sub_5_0 : f64Sub 0x4014000000000000#64 0#64 = 0x4014000000000000#64 :=
  f64Sub_posZero_right _ 5 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

private theorem f64Add_0_1 : f64Add 0#64 0x3ff0000000000000#64 = 0x3ff0000000000000#64 :=
  f64Add_zero_left _ 1 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Sub_3_eps :
    f64Sub 0x4008000000000000#64 0x3eb0c6f7a0b5ed8d#64 = 0x4007ffff79c842fa#64 := by
  have hSub : f64Sub 0x4008000000000000#64 0x3eb0c6f7a0b5ed8d#64 =
      f64Add 0x4008000000000000#64 0xbeb0c6f7a0b5ed8d#64 := by
    unfold f64Sub
    have hN : (f64IsNaN 0x4008000000000000#64 || f64IsNaN 0x3eb0c6f7a0b5ed8d#64) = false := by decide
    have hNeg : f64Neg 0x3eb0c6f7a0b5ed8d#64 = 0xbeb0c6f7a0b5ed8d#64 := by decide
    simp only [hN, Bool.false_eq_true, ↓reduceIte, hNeg]
  rw [hSub]
  exact f64Add_mkRat_pow2 _ _ _ 3 (-4722366482869645) 0 72 (by decide) (by decide)
    (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Add_3_eps :
    f64Add 0x4008000000000000#64 0x3eb0c6f7a0b5ed8d#64 = 0x400800008637bd06#64 :=
  f64Add_mkRat_pow2 _ _ _ 3 4722366482869645 0 72 (by decide) (by decide)
    (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Sub_4_eps :
    f64Sub 0x4010000000000000#64 0x3eb0c6f7a0b5ed8d#64 = 0x400fffff79c842fa#64 := by
  have hSub : f64Sub 0x4010000000000000#64 0x3eb0c6f7a0b5ed8d#64 =
      f64Add 0x4010000000000000#64 0xbeb0c6f7a0b5ed8d#64 := by
    unfold f64Sub
    have hN : (f64IsNaN 0x4010000000000000#64 || f64IsNaN 0x3eb0c6f7a0b5ed8d#64) = false := by decide
    have hNeg : f64Neg 0x3eb0c6f7a0b5ed8d#64 = 0xbeb0c6f7a0b5ed8d#64 := by decide
    simp only [hN, Bool.false_eq_true, ↓reduceIte, hNeg]
  rw [hSub]
  exact f64Add_mkRat_pow2 _ _ _ 4 (-4722366482869645) 0 72 (by decide) (by decide)
    (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Add_4_eps :
    f64Add 0x4010000000000000#64 0x3eb0c6f7a0b5ed8d#64 = 0x40100000431bde83#64 :=
  f64Add_mkRat_pow2 _ _ _ 4 4722366482869645 0 72 (by decide) (by decide)
    (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Round_ceil_3m :
    f64RoundToInt .ceil 0x4007ffff79c842fa#64 = 0x4008000000000000#64 :=
  f64RoundToInt_ceil _ _ (mkRat 3377698594627965 1125899906842624) 3
    (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Round_floor_3p :
    f64RoundToInt .floor 0x400800008637bd06#64 = 0x4008000000000000#64 :=
  f64RoundToInt_floor _ _ (mkRat 3377700846427779 1125899906842624) 3
    (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Round_ceil_4m :
    f64RoundToInt .ceil 0x400fffff79c842fa#64 = 0x4010000000000000#64 :=
  f64RoundToInt_ceil _ _ (mkRat 4503598501470589 1125899906842624) 4
    (by decide) (by decide) (by decide) (by decide) (by decide)

set_option maxRecDepth 8192 in
private theorem f64Round_floor_4p :
    f64RoundToInt .floor 0x40100000431bde83#64 = 0x4010000000000000#64 :=
  f64RoundToInt_floor _ _ (mkRat 4503600753270403 1125899906842624) 4
    (by decide) (by decide) (by decide) (by decide) (by decide)

private theorem f64Round_na_2 :
    f64RoundToInt .nearestAway 0x4000000000000000#64 = 0x4000000000000000#64 :=
  f64RoundToInt_nearestAway_nonneg _ _ 2 2 (by decide) (by decide) (by decide)
    (by rw [show (2 : Rat) = mkRat 2 (2 ^ 0) from by decide, show mkRat 1 2 = mkRat 1 (2 ^ 1) from by decide,
            rat_add_odd_pow2 2 0 1 1 (by decide) (by decide) (by decide) (by decide)]; decide)
    (by decide) (by decide)

private theorem f64Round_na_4 :
    f64RoundToInt .nearestAway 0x4010000000000000#64 = 0x4010000000000000#64 :=
  f64RoundToInt_nearestAway_nonneg _ _ 4 4 (by decide) (by decide) (by decide)
    (by rw [show (4 : Rat) = mkRat 4 (2 ^ 0) from by decide, show mkRat 1 2 = mkRat 1 (2 ^ 1) from by decide,
            rat_add_odd_pow2 4 0 1 1 (by decide) (by decide) (by decide) (by decide)]; decide)
    (by decide) (by decide)

private theorem f64ToInt_2 : f64ToInt64 0x4000000000000000#64 = 2#64 :=
  f64ToInt64_exact _ _ 2 2 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

private theorem f64ToInt_3 : f64ToInt64 0x4008000000000000#64 = 3#64 :=
  f64ToInt64_exact _ _ 3 3 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

private theorem f64ToInt_4 : f64ToInt64 0x4010000000000000#64 = 4#64 :=
  f64ToInt64_exact _ _ 4 4 (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide) (by decide)

def augPairs : List (BitVec 64 × BitVec 64) :=
  [ (0x399d98#64, 0x7ff0000000000000#64)
  , (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64)
  , (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64)
  , (0x25000#64, 0xffffffffffffffff#64), (0x25008#64, 0xffffffffffffffff#64)
  , (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64)
  , (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64)
  , (0x80008#64, 0x25000#64), (0x80010#64, 2#64), (0x80018#64, 2#64)
  , (0x80020#64, 0x26000#64), (0x80028#64, 2#64), (0x80030#64, 2#64)
  , (0x80038#64, 0x27000#64), (0x80040#64, 2#64), (0x80048#64, 2#64)
  , (0x80050#64, 0x28000#64), (0x80058#64, 2#64), (0x80060#64, 2#64) ]

def augOverlayInit : OverlayState :=
  { regs := sparse32 [(0, 0#64), (1, 2#64), (2, 0x21000#64), (3, 4#64), (4, 4#64),
      (5, 0x22000#64), (6, 2#64), (7, 2#64), (8, 0x23000#64), (9, 2#64),
      (10, 2#64), (11, 0x24000#64), (12, 2#64), (13, 2#64), (14, 0#64), (30, 0xdeadbeef#64)]
  , fpRegs := sparse32 []
  , sp := 0x80000#64, nzcv := {}, gStackGuard0 := 0x70000#64
  , ws := augPairs, pcIdx := 0, baseAddr := 0x1f05c0#64, status := .running }

private def augC1 : OverlayState :=
  { regs := sparse32 [(0, 0x24000#64), (1, 0x2#64), (2, 0x2#64), (3, 0x21000#64), (4, 0x2#64), (5, 0x4#64), (6, 0x23000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x21000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x4#64), (13, 0x2#64), (15, 0xfffffffffffffffc#64), (17, 0x7ff90#64), (29, 0x7ff08#64), (30, 0x1f06b0#64)], fpRegs := sparse32 [],
    sp := 0x7ff10#64, nzcv := { n := false, z := false, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0xffffffffffffffff#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64)],
    pcIdx := 59, baseAddr := 0x1f05c0#64, status := ExecStatus.callExternal "internal/simd.InitReducedRow64" }

private def initC38 : OverlayState :=
  { regs := sparse32 [(0, 0x24000#64), (1, 0x2#64), (2, 0xfffffffffffffffe#64), (3, 0x21000#64), (5, 0x4#64), (6, 0x23000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x21000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x4#64), (13, 0x2#64), (15, 0xfffffffffffffffc#64), (17, 0x7ff90#64), (27, 0x399000#64), (29, 0x7fef8#64), (30, 0x1f06b0#64)], fpRegs := sparse32 [(1, 0x7ff0000000000000#64), (2, 0x3ff0000000000000#64), (4, 0x7ff0000000000000#64)],
    sp := 0x7ff00#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0xffffffffffffffff#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64), (0x7ff00#64, 0x1f06b0#64), (0x7fef8#64, 0x7ff08#64), (0x7ff18#64, 0x24000#64), (0x7ff30#64, 0x21000#64), (0x7ff48#64, 0x23000#64)],
    pcIdx := 69, baseAddr := 0x1e43e0#64, status := ExecStatus.running }

private def initC48 : OverlayState :=
  { regs := sparse32 [(0, 0x24000#64), (1, 0x2#64), (2, 0xfffffffffffffffe#64), (3, 0x21000#64), (4, 0x1#64), (5, 0x4#64), (6, 0x23000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x21000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x4#64), (13, 0x2#64), (15, 0xfffffffffffffffc#64), (17, 0x7ff90#64), (27, 0x399000#64), (29, 0x7fef8#64), (30, 0x1f06b0#64)], fpRegs := sparse32 [(1, 0x3ff0000000000000#64), (2, 0x4014000000000000#64), (4, 0x7ff0000000000000#64)],
    sp := 0x7ff00#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0xffffffffffffffff#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64), (0x7ff00#64, 0x1f06b0#64), (0x7fef8#64, 0x7ff08#64), (0x7ff18#64, 0x24000#64), (0x7ff30#64, 0x21000#64), (0x7ff48#64, 0x23000#64), (0x24000#64, 0x3ff0000000000000#64)],
    pcIdx := 69, baseAddr := 0x1e43e0#64, status := ExecStatus.running }

private def initFin : OverlayState :=
  { regs := sparse32 [(0, 0x24000#64), (1, 0x2#64), (2, 0xfffffffffffffffe#64), (3, 0x21000#64), (4, 0x2#64), (5, 0x4#64), (6, 0x23000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x21000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x4#64), (13, 0x2#64), (15, 0xfffffffffffffffc#64), (17, 0x7ff90#64), (27, 0x399000#64), (29, 0x7ff08#64), (30, 0x1f06b0#64)], fpRegs := sparse32 [(0, 0x3ff0000000000000#64), (1, 0x3ff0000000000000#64), (2, 0x4014000000000000#64), (4, 0x7ff0000000000000#64)],
    sp := 0x7ff10#64, nzcv := { n := false, z := false, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0xffffffffffffffff#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64), (0x7ff00#64, 0x1f06b0#64), (0x7fef8#64, 0x7ff08#64), (0x7ff18#64, 0x24000#64), (0x7ff30#64, 0x21000#64), (0x7ff48#64, 0x23000#64), (0x24000#64, 0x3ff0000000000000#64), (0x24008#64, 0x4014000000000000#64)],
    pcIdx := 84, baseAddr := 0x1e43e0#64, status := ExecStatus.returned }

private def aug2C26 : OverlayState :=
  { regs := sparse32 [(1, 0x2#64), (2, 0x22000#64), (3, 0x21000#64), (4, 0x2#64), (5, 0x4#64), (6, 0x24000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x25000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x27000#64), (13, 0x2#64), (15, 0x2#64), (17, 0x28000#64), (20, 0xffffffffffffffff#64), (27, 0x399000#64), (29, 0x7ff08#64), (30, 0x1f06b0#64)], fpRegs := sparse32 [(1, 0x7ff0000000000000#64), (2, 0x3ff0000000000000#64), (4, 0x7ff0000000000000#64)],
    sp := 0x7ff10#64, nzcv := { n := false, z := true, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0xffffffffffffffff#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0xffffffffffffffff#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64), (0x7ff00#64, 0x1f06b0#64), (0x7fef8#64, 0x7ff08#64), (0x7ff18#64, 0x24000#64), (0x7ff30#64, 0x21000#64), (0x7ff48#64, 0x23000#64), (0x24000#64, 0x3ff0000000000000#64), (0x24008#64, 0x4014000000000000#64), (0x28000#64, 0#64)],
    pcIdx := 108, baseAddr := 0x1f05c0#64, status := ExecStatus.running }

private def augFin : OverlayState :=
  { regs := sparse32 [(2, 0x26000#64), (3, 0x21000#64), (4, 0x2#64), (5, 0x4#64), (6, 0x24000#64), (7, 0x2#64), (8, 0x2#64), (9, 0x25000#64), (10, 0x2#64), (11, 0x24000#64), (12, 0x27000#64), (13, 0x2#64), (15, 0x2#64), (17, 0x28000#64), (20, 0xffffffffffffffff#64), (27, 0x399000#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x3ff0000000000000#64), (1, 0x7ff0000000000000#64), (2, 0x3ff0000000000000#64), (4, 0x7ff0000000000000#64)],
    sp := 0x80000#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := [(0x399d98#64, 0x7ff0000000000000#64), (0x21000#64, 0x3ff0000000000000#64), (0x21008#64, 0x4014000000000000#64), (0x21010#64, 0x4000000000000000#64), (0x21018#64, 0x4010000000000000#64), (0x25000#64, 0#64), (0x25008#64, 0xffffffffffffffff#64), (0x26000#64, 0#64), (0x26008#64, 0xffffffffffffffff#64), (0x27000#64, 0#64), (0x27008#64, 0xffffffffffffffff#64), (0x80008#64, 0x25000#64), (0x80010#64, 0x2#64), (0x80018#64, 0x2#64), (0x80020#64, 0x26000#64), (0x80028#64, 0x2#64), (0x80030#64, 0x2#64), (0x80038#64, 0x27000#64), (0x80040#64, 0x2#64), (0x80048#64, 0x2#64), (0x80050#64, 0x28000#64), (0x80058#64, 0x2#64), (0x80060#64, 0x2#64), (0x7ff10#64, 0xdeadbeef#64), (0x7ff08#64, 0#64), (0x80078#64, 0x21000#64), (0x80090#64, 0x22000#64), (0x800a8#64, 0x23000#64), (0x800c0#64, 0x24000#64), (0x80068#64, 0#64), (0x80070#64, 0x2#64), (0x800d8#64, 0#64), (0x80098#64, 0x2#64), (0x800d0#64, 0x2#64), (0x800b8#64, 0x2#64), (0x80088#64, 0x4#64), (0x80080#64, 0x4#64), (0x7ff00#64, 0x1f06b0#64), (0x7fef8#64, 0x7ff08#64), (0x7ff18#64, 0x24000#64), (0x7ff30#64, 0x21000#64), (0x7ff48#64, 0x23000#64), (0x24000#64, 0x3ff0000000000000#64), (0x24008#64, 0x4014000000000000#64), (0x28000#64, 0#64), (0x22000#64, 0x3ff0000000000000#64)],
    pcIdx := 120, baseAddr := 0x1f05c0#64, status := ExecStatus.returned }

def hasInfPairs (rhsBits : BitVec 64) : List (BitVec 64 × BitVec 64) :=
  [ (0x3998f0#64, 0x41cdcd6500000000#64), (0x399a70#64, 0x3e7ad7f29abcaf48#64)
  , (0x399a80#64, 0x3eb0c6f7a0b5ed8d#64), (0x399d40#64, 0x43d0000000000000#64)
  , (0x10248#64, 0x0100#64), (0x10238#64, 1#64)
  , (0x10110#64, 0x11000#64), (0x10118#64, 1#64), (0x11000#64, 0#64)
  , (0x10170#64, 0x12000#64), (0x10178#64, 2#64), (0x12000#64, 0#64), (0x12008#64, 2#64)
  , (0x10128#64, 0x13000#64), (0x10130#64, 1#64), (0x13000#64, rhsBits)
  , (0x10188#64, 0x14000#64), (0x10190#64, 2#64), (0x14000#64, 0#64), (0x14008#64, 1#64)
  , (0x10080#64, 0x15000#64), (0x10088#64, 2#64), (0x15000#64, 0x0101#64)
  , (0x101a0#64, 0x16000#64), (0x101a8#64, 2#64), (0x16000#64, 0x4000000000000000#64), (0x16008#64, 0x4010000000000000#64) ]

def hasInfOverlayInit (rhsBits : BitVec 64) : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (30, 0xdeadbeef#64)]
  , fpRegs := sparse32 []
  , sp := 0x80000#64, nzcv := {}, gStackGuard0 := 0x70000#64
  , ws := hasInfPairs rhsBits, pcIdx := 0, baseAddr := 0x201350#64, status := .running }

private def hi3C46 : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (1, 0x1#64), (3, 0x2#64), (5, 0x16000#64), (7, 0x2#64), (8, 0x2#64), (29, 0x7ffe8#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4000000000000000#64)],
    sp := 0x7fff0#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := hasInfPairs 0x4008000000000000#64 ++ [(0x7fff0#64, 0xdeadbeef#64), (0x7ffe8#64, 0#64)],
    pcIdx := 65, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi3C54 : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (1, 0x1#64), (3, 0x2#64), (5, 0x16000#64), (7, 0x2#64), (8, 0x2#64), (27, 0x399000#64), (29, 0x7ffe8#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4000000000000000#64), (1, 0x41cdcd6500000000#64)],
    sp := 0x7fff0#64, nzcv := { n := false, z := false, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := hi3C46.ws,
    pcIdx := 73, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi3C82 : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (1, 0x1#64), (3, 0x2#64), (4, 0x2#64), (5, 0x16000#64), (6, 0x1#64), (7, 0x2#64), (8, 0x2#64), (29, 0x7ffe8#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4010000000000000#64), (1, 0x41cdcd6500000000#64)],
    sp := 0x7fff0#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := hi3C46.ws,
    pcIdx := 65, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi3C90 : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (1, 0x1#64), (3, 0x2#64), (4, 0x2#64), (5, 0x16000#64), (6, 0x1#64), (7, 0x2#64), (8, 0x2#64), (27, 0x399000#64), (29, 0x7ffe8#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4010000000000000#64), (1, 0x41cdcd6500000000#64)],
    sp := 0x7fff0#64, nzcv := { n := false, z := false, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := hi3C46.ws,
    pcIdx := 73, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi3C123 : OverlayState :=
  { regs := sparse32 [(0, 0x10000#64), (1, 0x1#64), (3, 0x13000#64), (4, 0x2#64), (5, 0x1#64), (6, 0x2#64), (8, 0x2#64), (27, 0x399000#64), (29, 0x7ffe8#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4008000000000000#64), (1, 0x3eb0c6f7a0b5ed8d#64), (2, 0x43d0000000000000#64)],
    sp := 0x7fff0#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := hi3C46.ws,
    pcIdx := 97, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi3Fin : OverlayState :=
  { regs := sparse32 [(0, 0x1#64), (1, 0x1#64), (3, 0x4#64), (4, 0x2#64), (5, 0x3#64), (6, 0x1#64), (8, 0x2#64), (27, 0x1#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4008000000000000#64), (1, 0x3eb0c6f7a0b5ed8d#64), (2, 0x43d0000000000000#64), (3, 0x4008000000000000#64)],
    sp := 0x80000#64, nzcv := { n := true, z := false, c := false, v := false },
    gStackGuard0 := 0x70000#64, ws := hi3C46.ws,
    pcIdx := 133, baseAddr := 0x201350#64, status := ExecStatus.returned }

private def hi4C46 : OverlayState :=
  { regs := hi3C46.regs, fpRegs := hi3C46.fpRegs, sp := 0x7fff0#64, nzcv := hi3C46.nzcv,
    gStackGuard0 := 0x70000#64, ws := hasInfPairs 0x4010000000000000#64 ++ [(0x7fff0#64, 0xdeadbeef#64), (0x7ffe8#64, 0#64)],
    pcIdx := 65, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi4C54 : OverlayState := { hi3C54 with ws := hi4C46.ws }
private def hi4C82 : OverlayState := { hi3C82 with ws := hi4C46.ws }
private def hi4C90 : OverlayState := { hi3C90 with ws := hi4C46.ws }

private def hi4C123 : OverlayState :=
  { regs := hi3C123.regs, fpRegs := sparse32 [(0, 0x4010000000000000#64), (1, 0x3eb0c6f7a0b5ed8d#64), (2, 0x43d0000000000000#64)],
    sp := 0x7fff0#64, nzcv := hi3C123.nzcv, gStackGuard0 := 0x70000#64, ws := hi4C46.ws,
    pcIdx := 97, baseAddr := 0x201350#64, status := ExecStatus.running }

private def hi4Fin : OverlayState :=
  { regs := sparse32 [(1, 0x1#64), (2, 0x1#64), (3, 0x4#64), (4, 0x2#64), (5, 0x4#64), (6, 0x2#64), (8, 0x2#64), (27, 0x399000#64), (30, 0xdeadbeef#64)], fpRegs := sparse32 [(0, 0x4010000000000000#64), (1, 0x3e7ad7f29abcaf48#64), (2, 0x43d0000000000000#64), (3, 0x4010000000000000#64)],
    sp := 0x80000#64, nzcv := { n := false, z := true, c := true, v := false },
    gStackGuard0 := 0x70000#64, ws := hi4C46.ws,
    pcIdx := 129, baseAddr := 0x201350#64, status := ExecStatus.returned }

private theorem runHasInf_shared_mid (rhsBits : BitVec 64)
    (c46 c54 c82 c90 c123 : OverlayState)
    (h1 : runOverlay 46 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase (hasInfOverlayInit rhsBits) = some c46)
    (h2 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { c46 with fpRegs := c46.fpRegs.set 1 0x4000000000000000#64, pcIdx := 66 } = some c54)
    (h3 : runOverlay 27 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { c54 with regs := c54.regs.set 5 2#64, pcIdx := 74 } = some c82)
    (h4 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { c82 with fpRegs := c82.fpRegs.set 1 0x4010000000000000#64, pcIdx := 66 } = some c90)
    (h5 : runOverlay 32 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { c90 with regs := c90.regs.set 5 4#64, pcIdx := 74 } = some c123)
    (hc46S : c46.status = .running ∧ c46.pcIdx = 65 ∧ c46.fpRegs.length = 32 ∧ lookup32 c46.fpRegs 0#5 = 0x4000000000000000#64)
    (hc54S : c54.status = .running ∧ c54.pcIdx = 73 ∧ c54.regs.length = 32 ∧ lookup32 c54.fpRegs 0#5 = 0x4000000000000000#64)
    (hc82S : c82.status = .running ∧ c82.pcIdx = 65 ∧ c82.fpRegs.length = 32 ∧ lookup32 c82.fpRegs 0#5 = 0x4010000000000000#64)
    (hc90S : c90.status = .running ∧ c90.pcIdx = 73 ∧ c90.regs.length = 32 ∧ lookup32 c90.fpRegs 0#5 = 0x4010000000000000#64) :
    runChunked 123 hasInfeasibleIntegerEqRowChunks 16 154 hasInfeasibleIntegerEqRowTargets
      ((hasInfOverlayInit rhsBits).toArm64 zeroBase) = c123.toArm64 zeroBase := by
  have hcw65 : chunkWord hasInfeasibleIntegerEqRowChunks 16 65 = some 0x1e664001 := by decide
  have hdec65 : decodeArm64Word 0x1e664001 = .fpUnaryD 12 1#5 0#5 := by decide
  have hcw73 : chunkWord hasInfeasibleIntegerEqRowChunks 16 73 = some 0x9e780005 := by decide
  have hdec73 : decodeArm64Word 0x9e780005 = .fpIntConv true true false 5#5 0#5 := by decide
  have hr46 := runOverlay_sound 46 _ _ _ zeroBase _ _ h1
  have hr54 := runOverlay_sound 7 _ _ _ zeroBase _ _ h2
  have hr82 := runOverlay_sound 27 _ _ _ zeroBase _ _ h3
  have hr90 := runOverlay_sound 7 _ _ _ zeroBase _ _ h4
  have hr123 := runOverlay_sound 32 _ _ _ zeroBase _ _ h5
  have hs46 := runChunked_fpRound hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c46 12 .nearestAway 1#5 0#5 1 66 0x1e664001 0x4000000000000000#64
    rfl (by omega) hc46S.1 (hc46S.2.1 ▸ (by decide : 65 < 154)) hc46S.2.2.1 (hc46S.2.1 ▸ hcw65) hdec65 (Or.inr (Or.inr ⟨rfl, rfl⟩)) (by rw [hc46S.2.2.2]; exact f64Round_na_2)
  have hs54 := runChunked_fpToInt64 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c54 true 5#5 0#5 5 74 0x9e780005 2#64
    rfl (by omega) hc54S.1 (hc54S.2.1 ▸ (by decide : 73 < 154)) hc54S.2.2.1 (by decide) (hc54S.2.1 ▸ hcw73) hdec73 (by rw [hc54S.2.2.2]; exact f64ToInt_2)
  have hs82 := runChunked_fpRound hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c82 12 .nearestAway 1#5 0#5 1 66 0x1e664001 0x4010000000000000#64
    rfl (by omega) hc82S.1 (hc82S.2.1 ▸ (by decide : 65 < 154)) hc82S.2.2.1 (hc82S.2.1 ▸ hcw65) hdec65 (Or.inr (Or.inr ⟨rfl, rfl⟩)) (by rw [hc82S.2.2.2]; exact f64Round_na_4)
  have hs90 := runChunked_fpToInt64 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c90 true 5#5 0#5 5 74 0x9e780005 4#64
    rfl (by omega) hc90S.1 (hc90S.2.1 ▸ (by decide : 73 < 154)) hc90S.2.2.1 (by decide) (hc90S.2.1 ▸ hcw73) hdec73 (by rw [hc90S.2.2.2]; exact f64ToInt_4)
  rw [show (123 : Nat) = 46 + (1 + (7 + (1 + (27 + (1 + (7 + (1 + 32))))))) by omega]
  rw [runChunked_add, hr46, runChunked_add, hs46, runChunked_add, hr54, runChunked_add, hs54]
  rw [runChunked_add, hr82, runChunked_add, hs82, runChunked_add, hr90, runChunked_add, hs90]
  exact hr123

private theorem runHasInf_tail_6
    (c123 c129 cFin : OverlayState) (fuelTail : Nat) (x3Lo x0Hi x3Res x5Res : BitVec 64)
    (hStat : c123.status = .running ∧ c123.pcIdx = 97 ∧ c123.regs.length = 32 ∧ c123.fpRegs.length = 32 ∧
      lookup32 c123.fpRegs 0#5 = x0Hi ∧ lookup32 c123.fpRegs 1#5 = 0x3eb0c6f7a0b5ed8d#64)
    (hSub : f64Sub x0Hi 0x3eb0c6f7a0b5ed8d#64 = x3Lo)
    (hCeil : f64RoundToInt .ceil x3Lo = x0Hi)
    (hInt1 : f64ToInt64 x0Hi = x3Res)
    (hAdd : f64Add x0Hi 0x3eb0c6f7a0b5ed8d#64 = x5Res)
    (hFloor : f64RoundToInt .floor x5Res = x0Hi)
    (h129Eq : {
      c123 with
        fpRegs := (((c123.fpRegs.set 3 x3Lo).set 3 x0Hi).set 0 x5Res).set 0 x0Hi
        regs := (c123.regs.set 3 x3Res).set 5 x3Res
        pcIdx := 103
      } = c129)
    (hFin : runOverlay fuelTail hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c129 = some cFin) :
    runChunked (6 + fuelTail) hasInfeasibleIntegerEqRowChunks 16 154 hasInfeasibleIntegerEqRowTargets (c123.toArm64 zeroBase) =
      cFin.toArm64 zeroBase := by
  have hcw97 : chunkWord hasInfeasibleIntegerEqRowChunks 16 97 = some 0x1e613803 := by decide
  have hcw98 : chunkWord hasInfeasibleIntegerEqRowChunks 16 98 = some 0x1e64c063 := by decide
  have hcw99 : chunkWord hasInfeasibleIntegerEqRowChunks 16 99 = some 0x9e780063 := by decide
  have hcw100 : chunkWord hasInfeasibleIntegerEqRowChunks 16 100 = some 0x1e612800 := by decide
  have hcw101 : chunkWord hasInfeasibleIntegerEqRowChunks 16 101 = some 0x1e654000 := by decide
  have hcw102 : chunkWord hasInfeasibleIntegerEqRowChunks 16 102 = some 0x9e780005 := by decide
  let sA := { c123 with fpRegs := c123.fpRegs.set 3 x3Lo, pcIdx := 98 }
  let sB := { sA with fpRegs := sA.fpRegs.set 3 x0Hi, pcIdx := 99 }
  let sC := { sB with regs := sB.regs.set 3 x3Res, pcIdx := 100 }
  let sD := { sC with fpRegs := sC.fpRegs.set 0 x5Res, pcIdx := 101 }
  let sE := { sD with fpRegs := sD.fpRegs.set 0 x0Hi, pcIdx := 102 }
  have hs123 := runChunked_fpBinary hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase c123 3 3#5 0#5 1#5 3 98 0x1e613803 x3Lo
    rfl (by omega) hStat.1 (hStat.2.1 ▸ (by decide : 97 < 154)) hStat.2.2.2.1 (hStat.2.1 ▸ hcw97) (by decide) (Or.inr ⟨rfl, by rw [hStat.2.2.2.2.1, hStat.2.2.2.2.2]; exact hSub⟩)
  have hs124 := runChunked_fpRound hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase sA 9 .ceil 3#5 3#5 3 99 0x1e64c063 x0Hi
    rfl rfl hStat.1 (by decide : 98 < 154) (by simp only [sA, List.length_set]; exact hStat.2.2.2.1) hcw98 (by decide) (Or.inl ⟨rfl, rfl⟩)
    (by have hLk : lookup32 sA.fpRegs 3#5 = x3Lo := lookup32_set_eq c123.fpRegs hStat.2.2.2.1 3#5 x3Lo
        rw [hLk]; exact hCeil)
  have hs125 := runChunked_fpToInt64 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase sB true 3#5 3#5 3 100 0x9e780063 x3Res
    rfl rfl hStat.1 (by decide : 99 < 154) hStat.2.2.1 (by decide) hcw99 (by decide)
    (by have hLk : lookup32 sB.fpRegs 3#5 = x0Hi :=
          lookup32_set_eq sA.fpRegs (by simp only [sA, List.length_set]; exact hStat.2.2.2.1) 3#5 x0Hi
        rw [hLk]; exact hInt1)
  have hs126 := runChunked_fpBinary hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase sC 2 0#5 0#5 1#5 0 101 0x1e612800 x5Res
    rfl rfl hStat.1 (by decide : 100 < 154) (by simp only [sC, sB, sA, List.length_set]; exact hStat.2.2.2.1) hcw100 (by decide)
    (Or.inl ⟨rfl, by
      have hl0 : lookup32 sC.fpRegs 0#5 = x0Hi := by
        dsimp [sC, sB, sA]
        rw [show (c123.fpRegs.set 3 x3Lo).set 3 x0Hi = (c123.fpRegs.set (3#5).toNat x3Lo).set (3#5).toNat x0Hi from rfl,
          lookup32_set_ne _ (by simp [hStat.2.2.2.1]) 3#5 0#5 x0Hi (by decide),
          lookup32_set_ne c123.fpRegs hStat.2.2.2.1 3#5 0#5 x3Lo (by decide)]
        exact hStat.2.2.2.2.1
      have hl1 : lookup32 sC.fpRegs 1#5 = 0x3eb0c6f7a0b5ed8d#64 := by
        dsimp [sC, sB, sA]
        rw [show (c123.fpRegs.set 3 x3Lo).set 3 x0Hi = (c123.fpRegs.set (3#5).toNat x3Lo).set (3#5).toNat x0Hi from rfl,
          lookup32_set_ne _ (by simp [hStat.2.2.2.1]) 3#5 1#5 x0Hi (by decide),
          lookup32_set_ne c123.fpRegs hStat.2.2.2.1 3#5 1#5 x3Lo (by decide)]
        exact hStat.2.2.2.2.2
      rw [hl0, hl1]; exact hAdd⟩)
  have hs127 := runChunked_fpRound hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase sD 10 .floor 0#5 0#5 0 102 0x1e654000 x0Hi
    rfl rfl hStat.1 (by decide : 101 < 154) (by simp only [sD, sC, sB, sA, List.length_set]; exact hStat.2.2.2.1) hcw101 (by decide) (Or.inr (Or.inl ⟨rfl, rfl⟩))
    (by have hl0 : lookup32 sD.fpRegs 0#5 = x5Res :=
          lookup32_set_eq sC.fpRegs (by simp only [sC, sB, sA, List.length_set]; exact hStat.2.2.2.1) 0#5 x5Res
        rw [hl0]; exact hFloor)
  have hs128 := runChunked_fpToInt64 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase sE true 5#5 0#5 5 103 0x9e780005 x3Res
    rfl rfl hStat.1 (by decide : 102 < 154) (by simp only [sE, sD, sC, List.length_set]; exact hStat.2.2.1) (by decide) hcw102 (by decide)
    (by have hl0 : lookup32 sE.fpRegs 0#5 = x0Hi :=
          lookup32_set_eq sD.fpRegs (by simp only [sD, sC, sB, sA, List.length_set]; exact hStat.2.2.2.1) 0#5 x0Hi
        rw [hl0]; exact hInt1)
  have hs128' : runChunked 1 hasInfeasibleIntegerEqRowChunks 16 154 hasInfeasibleIntegerEqRowTargets (sE.toArm64 zeroBase) = c129.toArm64 zeroBase := by
    rw [hs128, h129Eq]
  rw [show 6 + fuelTail = 1 + (1 + (1 + (1 + (1 + (1 + fuelTail))))) by omega]
  rw [runChunked_add, hs123, runChunked_add, hs124, runChunked_add, hs125, runChunked_add, hs126, runChunked_add, hs127, runChunked_add, hs128']
  exact runOverlay_sound fuelTail _ _ _ zeroBase _ _ hFin

set_option maxRecDepth 8192 in
/-- Concrete machine execution: decoded ARM64 `hasInfeasibleIntegerEqRow` detects GCD infeasibility
on `2*x0 + 4*x1 = 3` (returning `1`) with stack restored cleanly and zero traps. -/
theorem hasInfeasibleIntegerEqRow_infeasible_gcd_exec :
    let sFin := runArm64 141 (decodeArm64Prog hasInfeasibleIntegerEqRowWords) hasInfeasibleIntegerEqRowTargets ((hasInfOverlayInit 0x4008000000000000#64).toArm64 zeroBase)
    sFin.status = .returned ∧ readGpr sFin 0#5 = 1#64 ∧ sFin.sp = 0x80000#64 := by
  have h1 : runOverlay 46 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase (hasInfOverlayInit 0x4008000000000000#64) = some hi3C46 := by decide
  have h2 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi3C46 with fpRegs := hi3C46.fpRegs.set 1 0x4000000000000000#64, pcIdx := 66 } = some hi3C54 := by decide
  have h3 : runOverlay 27 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi3C54 with regs := hi3C54.regs.set 5 2#64, pcIdx := 74 } = some hi3C82 := by decide
  have h4 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi3C82 with fpRegs := hi3C82.fpRegs.set 1 0x4010000000000000#64, pcIdx := 66 } = some hi3C90 := by decide
  have h5 : runOverlay 32 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi3C90 with regs := hi3C90.regs.set 5 4#64, pcIdx := 74 } = some hi3C123 := by decide
  have hMid := runHasInf_shared_mid 0x4008000000000000#64 hi3C46 hi3C54 hi3C82 hi3C90 hi3C123 h1 h2 h3 h4 h5
    (by decide) (by decide) (by decide) (by decide)
  let hi3C129 : OverlayState := {
    hi3C123 with
      fpRegs := (hi3C123.fpRegs.set 3 0x4008000000000000#64).set 0 0x4008000000000000#64
      regs := (hi3C123.regs.set 3 3#64).set 5 3#64
      pcIdx := 103
    }
  have hTail := runHasInf_tail_6 hi3C123 hi3C129 hi3Fin 12
    0x4007ffff79c842fa#64 0x4008000000000000#64 3#64 0x400800008637bd06#64
    (by decide) f64Sub_3_eps f64Round_ceil_3m f64ToInt_3 f64Add_3_eps f64Round_floor_3p rfl (by decide)
  have hRun : runChunked 141 hasInfeasibleIntegerEqRowChunks 16 154 hasInfeasibleIntegerEqRowTargets
      ((hasInfOverlayInit 0x4008000000000000#64).toArm64 zeroBase) = hi3Fin.toArm64 zeroBase := by
    rw [show (141 : Nat) = 123 + (6 + 12) by omega, runChunked_add, hMid]; exact hTail
  have hEq : runArm64 141 (decodeArm64Prog hasInfeasibleIntegerEqRowWords) hasInfeasibleIntegerEqRowTargets
      ((hasInfOverlayInit 0x4008000000000000#64).toArm64 zeroBase) = hi3Fin.toArm64 zeroBase := by
    unfold hasInfeasibleIntegerEqRowWords
    rw [runChunked_eq_runArm64 141 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets _ hasInfChunks_len (by decide)]
    exact hRun
  dsimp only; rw [hEq]
  refine ⟨rfl, by change lookup32 hi3Fin.regs 0#5 = 1#64; decide, rfl⟩

set_option maxRecDepth 8192 in
/-- Concrete machine execution: decoded ARM64 `hasInfeasibleIntegerEqRow` accepts feasible GCD row
`2*x0 + 4*x1 = 4` (returning `0`) with stack restored cleanly and zero traps. -/
theorem hasInfeasibleIntegerEqRow_feasible_gcd_exec :
    let sFin := runArm64 150 (decodeArm64Prog hasInfeasibleIntegerEqRowWords) hasInfeasibleIntegerEqRowTargets ((hasInfOverlayInit 0x4010000000000000#64).toArm64 zeroBase)
    sFin.status = .returned ∧ readGpr sFin 0#5 = 0#64 ∧ sFin.sp = 0x80000#64 := by
  have h1 : runOverlay 46 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase (hasInfOverlayInit 0x4010000000000000#64) = some hi4C46 := by decide
  have h2 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi4C46 with fpRegs := hi4C46.fpRegs.set 1 0x4000000000000000#64, pcIdx := 66 } = some hi4C54 := by decide
  have h3 : runOverlay 27 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi4C54 with regs := hi4C54.regs.set 5 2#64, pcIdx := 74 } = some hi4C82 := by decide
  have h4 : runOverlay 7 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi4C82 with fpRegs := hi4C82.fpRegs.set 1 0x4010000000000000#64, pcIdx := 66 } = some hi4C90 := by decide
  have h5 : runOverlay 32 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets zeroBase { hi4C90 with regs := hi4C90.regs.set 5 4#64, pcIdx := 74 } = some hi4C123 := by decide
  have hMid := runHasInf_shared_mid 0x4010000000000000#64 hi4C46 hi4C54 hi4C82 hi4C90 hi4C123 h1 h2 h3 h4 h5
    (by decide) (by decide) (by decide) (by decide)
  let hi4C129 : OverlayState := {
    hi4C123 with
      fpRegs := (hi4C123.fpRegs.set 3 0x4010000000000000#64).set 0 0x4010000000000000#64
      regs := (hi4C123.regs.set 3 4#64).set 5 4#64
      pcIdx := 103
    }
  have hTail := runHasInf_tail_6 hi4C123 hi4C129 hi4Fin 21
    0x400fffff79c842fa#64 0x4010000000000000#64 4#64 0x40100000431bde83#64
    (by decide) f64Sub_4_eps f64Round_ceil_4m f64ToInt_4 f64Add_4_eps f64Round_floor_4p rfl (by decide)
  have hRun : runChunked 150 hasInfeasibleIntegerEqRowChunks 16 154 hasInfeasibleIntegerEqRowTargets
      ((hasInfOverlayInit 0x4010000000000000#64).toArm64 zeroBase) = hi4Fin.toArm64 zeroBase := by
    rw [show (150 : Nat) = 123 + (6 + 21) by omega, runChunked_add, hMid]; exact hTail
  have hEq : runArm64 150 (decodeArm64Prog hasInfeasibleIntegerEqRowWords) hasInfeasibleIntegerEqRowTargets
      ((hasInfOverlayInit 0x4010000000000000#64).toArm64 zeroBase) = hi4Fin.toArm64 zeroBase := by
    unfold hasInfeasibleIntegerEqRowWords
    rw [runChunked_eq_runArm64 150 hasInfeasibleIntegerEqRowChunks 154 hasInfeasibleIntegerEqRowTargets _ hasInfChunks_len (by decide)]
    exact hRun
  dsimp only; rw [hEq]
  refine ⟨rfl, by change lookup32 hi4Fin.regs 0#5 = 0#64; decide, rfl⟩

private theorem augChunks_size : augmentRowScalarChunks.flatten.length = 335 := by
  rw [chunksSumLen_eq_flatten_length]; decide

set_option maxRecDepth 8192 in
private theorem augOverlay_c1 :
    runOverlay 60 augmentRowScalarChunks 335 augmentRowScalarTargets zeroBase augOverlayInit = some augC1 := by decide

private theorem initOverlay_c38 :
    runOverlay 38 initReducedRow64Chunks 102 initReducedRow64Targets zeroBase
      { augC1 with pcIdx := 0, baseAddr := 0x1e43e0#64, status := .running } = some initC38 := by decide

private theorem initOverlay_c48 :
    runOverlay 8 initReducedRow64Chunks 102 initReducedRow64Targets zeroBase
      { initC38 with fpRegs := initC38.fpRegs.set 2 0x3ff0000000000000#64, pcIdx := 71 } = some initC48 := by decide

private theorem initOverlay_fin :
    runOverlay 11 initReducedRow64Chunks 102 initReducedRow64Targets zeroBase
      { initC48 with fpRegs := initC48.fpRegs.set 2 0x4014000000000000#64, pcIdx := 71 } = some initFin := by decide

private theorem aug2Overlay_c26 :
    runOverlay 26 augmentRowScalarChunks 335 augmentRowScalarTargets zeroBase
      { initFin with pcIdx := 60, baseAddr := 0x1f05c0#64, status := .running } = some aug2C26 := by decide

private theorem aug2Overlay_fin :
    runOverlay 12 augmentRowScalarChunks 335 augmentRowScalarTargets zeroBase
      { aug2C26 with fpRegs := aug2C26.fpRegs.set 0 0x3ff0000000000000#64, pcIdx := 109 } = some augFin := by decide

/-- Concrete end-to-end composed ARM64 machine execution of `augmentRowScalar` calling
`InitReducedRow64` on a 2x2 cost matrix `[[1, 5], [2, 4]]` (`done == nil`), verifying
exact return status, stack restoration, updated dual `u[0] = 1.0`, and matching `col4row[0] = 0`, `row4col[0] = 0`. -/
theorem augmentRowScalar_concrete_exec :
    let s1 := runArm64 60 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets (augOverlayInit.toArm64 zeroBase)
    let s2 := runArm64 61 (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets { s1 with pcIdx := 0, baseAddr := 0x1e43e0#64, status := .running }
    let sFin := runArm64 39 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets { s2 with pcIdx := 60, baseAddr := 0x1f05c0#64, status := .running }
    s1.status = .callExternal "internal/simd.InitReducedRow64" ∧
      s1.pcIdx = 59 ∧
      s2.status = .returned ∧
      sFin.status = .returned ∧
      sFin.sp = 0x80000#64 ∧
      readU64 sFin.mem 0x22000#64 = 0x3ff0000000000000#64 ∧
      readU64 sFin.mem 0x25000#64 = 0#64 ∧
      readU64 sFin.mem 0x26000#64 = 0#64 := by
  have hs1 : runArm64 60 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets (augOverlayInit.toArm64 zeroBase) = augC1.toArm64 zeroBase := by
    unfold augmentRowScalarWords
    rw [runChunked_eq_runArm64 60 augmentRowScalarChunks 335 augmentRowScalarTargets _ augChunks_len augChunks_size]
    exact runOverlay_sound 60 _ _ _ zeroBase _ _ augOverlay_c1
  have hcw69 : chunkWord initReducedRow64Chunks 16 69 = some 0x1e633842 := by decide
  have hcw70 : chunkWord initReducedRow64Chunks 16 70 = some 0x1e603842 := by decide
  let jA := { initC38 with fpRegs := initC38.fpRegs.set 2 0x3ff0000000000000#64, pcIdx := 70 }
  let jB := { initC48 with fpRegs := initC48.fpRegs.set 2 0x4014000000000000#64, pcIdx := 70 }
  have hs38 := runChunked_fpBinary initReducedRow64Chunks 102 initReducedRow64Targets zeroBase initC38 3 2#5 2#5 3#5 2 70 0x1e633842 0x3ff0000000000000#64
    rfl rfl (by decide) (by decide) (by decide) hcw69 (by decide) (Or.inr ⟨rfl, by change f64Sub 0x3ff0000000000000#64 0#64 = _; exact f64Sub_1_0⟩)
  have hs39 := runChunked_fpBinary initReducedRow64Chunks 102 initReducedRow64Targets zeroBase jA 3 2#5 2#5 0#5 2 71 0x1e603842 0x3ff0000000000000#64
    rfl rfl (by decide) (by decide) (by decide) hcw70 (by decide) (Or.inr ⟨rfl, by change f64Sub 0x3ff0000000000000#64 0#64 = _; exact f64Sub_1_0⟩)
  have hs39' : runChunked 1 initReducedRow64Chunks 16 102 initReducedRow64Targets (jA.toArm64 zeroBase) =
      ({ initC38 with fpRegs := initC38.fpRegs.set 2 0x3ff0000000000000#64, pcIdx := 71 }).toArm64 zeroBase := by
    rw [hs39]; dsimp only [jA]; rw [list_set_idem]
  have hs48 := runChunked_fpBinary initReducedRow64Chunks 102 initReducedRow64Targets zeroBase initC48 3 2#5 2#5 3#5 2 70 0x1e633842 0x4014000000000000#64
    rfl rfl (by decide) (by decide) (by decide) hcw69 (by decide) (Or.inr ⟨rfl, by change f64Sub 0x4014000000000000#64 0#64 = _; exact f64Sub_5_0⟩)
  have hs49 := runChunked_fpBinary initReducedRow64Chunks 102 initReducedRow64Targets zeroBase jB 3 2#5 2#5 0#5 2 71 0x1e603842 0x4014000000000000#64
    rfl rfl (by decide) (by decide) (by decide) hcw70 (by decide) (Or.inr ⟨rfl, by change f64Sub 0x4014000000000000#64 0#64 = _; exact f64Sub_5_0⟩)
  have hs49' : runChunked 1 initReducedRow64Chunks 16 102 initReducedRow64Targets (jB.toArm64 zeroBase) =
      ({ initC48 with fpRegs := initC48.fpRegs.set 2 0x4014000000000000#64, pcIdx := 71 }).toArm64 zeroBase := by
    rw [hs49]; dsimp only [jB]; rw [list_set_idem]
  have hs2 : runArm64 61 (decodeArm64Prog initReducedRow64Words) initReducedRow64Targets
      { augC1.toArm64 zeroBase with pcIdx := 0, baseAddr := 0x1e43e0#64, status := .running } = initFin.toArm64 zeroBase := by
    unfold initReducedRow64Words
    rw [toArm64_with_ctrl, runChunked_eq_runArm64 61 initReducedRow64Chunks 102 initReducedRow64Targets _ initChunks_len (by decide)]
    rw [show (61 : Nat) = 38 + (1 + (1 + (8 + (1 + (1 + 11))))) by omega]
    rw [runChunked_add, runOverlay_sound 38 _ _ _ zeroBase _ _ initOverlay_c38]
    rw [runChunked_add, hs38, runChunked_add, hs39']
    rw [runChunked_add, runOverlay_sound 8 _ _ _ zeroBase _ _ initOverlay_c48]
    rw [runChunked_add, hs48, runChunked_add, hs49']
    exact runOverlay_sound 11 _ _ _ zeroBase _ _ initOverlay_fin
  have hcw108 : chunkWord augmentRowScalarChunks 16 108 = some 0x1e622800 := by decide
  have hs108 := runChunked_fpBinary augmentRowScalarChunks 335 augmentRowScalarTargets zeroBase aug2C26 2 0#5 0#5 2#5 0 109 0x1e622800 0x3ff0000000000000#64
    rfl rfl (by decide) (by decide) (by decide) hcw108 (by decide) (Or.inl ⟨rfl, by change f64Add 0#64 0x3ff0000000000000#64 = _; exact f64Add_0_1⟩)
  have hsFin : runArm64 39 (decodeArm64Prog augmentRowScalarWords) augmentRowScalarTargets
      { initFin.toArm64 zeroBase with pcIdx := 60, baseAddr := 0x1f05c0#64, status := .running } = augFin.toArm64 zeroBase := by
    unfold augmentRowScalarWords
    rw [toArm64_with_ctrl, runChunked_eq_runArm64 39 augmentRowScalarChunks 335 augmentRowScalarTargets _ augChunks_len augChunks_size]
    rw [show (39 : Nat) = 26 + (1 + 12) by omega]
    rw [runChunked_add, runOverlay_sound 26 _ _ _ zeroBase _ _ aug2Overlay_c26]
    rw [runChunked_add, hs108]
    exact runOverlay_sound 12 _ _ _ zeroBase _ _ aug2Overlay_fin
  dsimp only
  rw [hs1, hs2, hsFin]
  refine ⟨rfl, rfl, rfl, rfl, rfl, ?_, ?_, ?_⟩
  · change readU64 (evalWordOverlay zeroBase augFin.ws) 0x22000#64 = 0x3ff0000000000000#64; decide
  · change readU64 (evalWordOverlay zeroBase augFin.ws) 0x25000#64 = 0#64; decide
  · change readU64 (evalWordOverlay zeroBase augFin.ws) 0x26000#64 = 0#64; decide

end GoblinFormal.Track5B
