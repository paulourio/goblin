/-
GoblinFormal.Track5D: Tier 5B Binary Safety Extension for Phase 2 LSAP Kernels
(`twoSumAssign`, `clampFinite`, `exactReducedCost`, `hasPotentialCancellation`)
and Functional IEEE-754 SIMD Lane & `Min2` Reduction Equivalence.
-/

import GoblinFormal.Common.Basic
import GoblinFormal.Track5A
import GoblinFormal.Track5B

namespace GoblinFormal.Track5D

open GoblinFormal
open GoblinFormal.Track5A
open GoblinFormal.Track5B

/-!
### Part I: Extracted Stock Go 1.27 `GOARCH=arm64` ELF `.text` Chunks & Relocation Targets
-/

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (8 insns) for `assign.twoSumAssign` in 16-word chunks. -/
def twoSumAssignChunks : List (List UInt32) := [
  [0x1e602822, 0x1e603843, 0x1e633844, 0x1e643804, 0x1e633823, 0x1e632881, 0x1e604040, 0xd65f03c0]
]
def twoSumAssignWords : Array UInt32 := ⟨twoSumAssignChunks.flatten⟩
def twoSumAssignTargets (_ : Nat) : Option String := none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (13 insns) for `assign.clampFinite` in 16-word chunks. -/
def clampFiniteChunks : List (List UInt32) := [
  [0xb00009bb, 0xfd45ef61, 0x1e602020, 0x54000104, 0xb00009bb, 0xfd45f761, 0x1e612000, 0x54000065, 0x1e604020, 0xd65f03c0, 0xd65f03c0, 0x1e604020, 0xd65f03c0]
]
def clampFiniteWords : Array UInt32 := ⟨clampFiniteChunks.flatten⟩
def clampFiniteTargets (_ : Nat) : Option String := none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (63 insns) for `-gcflags=-l` `assign.exactReducedCost` in 16-word chunks. -/
def exactReducedCostNoinlineChunks : List (List UInt32) := [
  [0xf9400b90, 0xeb3063ff, 0x540006c9, 0xf81a0ffe, 0xf81f83fd, 0xd10023fd, 0xfd003fe2, 0x1e614021, 0x97fff860, 0xfd0017e0, 0xfd002be1, 0xfd403fe2, 0x1e614041, 0x97fff85b, 0xfd0013e0, 0xfd402be0],
  [0x97fff858, 0xfd0027e0, 0xfd001fe1, 0x1e604001, 0xfd4013e0, 0x97fff853, 0xfd0023e0, 0xfd001be1, 0xfd4017e0, 0xaa1f03e0, 0x97fbb9aa, 0x36000060, 0xb24003e0, 0x14000004, 0xfd4013e0, 0xaa1f03e0],
  [0x97fbb9a4, 0x37000120, 0xfd4027e0, 0xaa1f03e0, 0x97fbb9a0, 0x370000a0, 0xfd4023e0, 0xaa1f03e0, 0x97fbb99c, 0x360000c0, 0x9e6703e0, 0xaa1f03e0, 0xf85f83fd, 0xf84607fe, 0xd65f03c0, 0xfd401fe1],
  [0xfd401be2, 0x1e622821, 0xfd4023e2, 0x1e622820, 0xb24003e0, 0xf85f83fd, 0xf84607fe, 0xd65f03c0, 0x6d0087e0, 0xfd000fe2, 0xaa1e03e3, 0x97fac749, 0x6d4087e0, 0xfd400fe2, 0x17ffffc2]
]
def exactReducedCostNoinlineWords : Array UInt32 := ⟨exactReducedCostNoinlineChunks.flatten⟩
def exactReducedCostNoinlineTargets (idx : Nat) : Option String :=
  match idx with
  | 8 | 13 | 16 | 21 => some "assign.twoSumAssign"
  | 26 | 32 | 36 | 40 => some "math.IsInf"
  | 59 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (52 insns) for shipping optimized `assign.exactReducedCost` in 16-word chunks. -/
def exactReducedCostChunks : List (List UInt32) := [
  [0x1e614021, 0x1e614042, 0x1e602823, 0x1e603864, 0x1e643865, 0x1e653805, 0x1e643821, 0x1e652821, 0x1e604064, 0x1e632845, 0x1e6338a6, 0x1e6638a7, 0x1e673863, 0x1e663842, 0x1e632842, 0x1e622823],
  [0x1e613866, 0x1e663867, 0x1e673821, 0x1e663842, 0x1e6040a6, 0x1e604067, 0x1e6328a8, 0x1e653909, 0x1e69390a, 0x1e6a38a5, 0x1e693863, 0x1e60c084, 0xd503201f, 0xd503201f, 0x1e622821, 0x1e6328a2],
  [0xb00009bb, 0xfd45ef63, 0x1e642060, 0x54000144, 0x1e60c0c4, 0x1e642060, 0x540000e4, 0x1e60c0e4, 0x1e642060, 0x54000084, 0x1e60c104, 0x1e642060, 0x54000085, 0x9e6703e0, 0xaa1f03e0, 0xd65f03c0],
  [0x1e622821, 0x1e682820, 0xb24003e0, 0xd65f03c0]
]
def exactReducedCostWords : Array UInt32 := ⟨exactReducedCostChunks.flatten⟩
def exactReducedCostTargets (_ : Nat) : Option String := none

/-- Stock Go 1.27 `GOARCH=arm64` ELF `.text` words (1353 insns) for `assign.hasPotentialCancellation` in 16-word chunks. -/
def hasPotentialCancellationChunks : List (List UInt32) := [
  [0xf9400b90, 0xd10b03f1, 0xeb10023f, 0x5400a629, 0xd10d03f4, 0xa93ffa9d, 0x9100029f, 0xd10023fd, 0xf901bbe2, 0xf901c7e5, 0xf901d3e8, 0xf901dfeb, 0xf100041f, 0x5400010d, 0xeb01015f, 0x5400a463],
  [0x9e6703e0, 0xaa1f03f0, 0xf941abf1, 0xf941a7f3, 0x14000007, 0xaa1f03e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x91000610, 0x1e604040, 0xeb10003f, 0x5400048d, 0xfc707901, 0x1e60c022, 0xb0000a3b],
  [0xfd415b63, 0x1e632040, 0x54000348, 0xb0000a3b, 0xfd415364, 0x1e622080, 0x54000244, 0x1e622000, 0x54000044, 0x1e604002, 0xeb00003f, 0x54fffdcd, 0x1e602028, 0x540000ec, 0xeb10023f, 0x5400a009],
  [0xf8707a74, 0x1e602028, 0xb6fffcf4, 0x54fffcc0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0],
  [0xf94021d0, 0xeb10001f, 0x54009d68, 0xf901b3e0, 0xf901b7e1, 0xf901ebee, 0x390f63ef, 0xf901e7ed, 0xf901e3ec, 0xf901dfeb, 0xf901d7e9, 0xf901d3e8, 0xf901cbe6, 0xf901c7e5, 0xf901c3e4, 0xfd00e7e0],
  [0xf900f7f0, 0xf901cfe7, 0xf901dbea, 0xf901bbe2, 0xf901bfe3, 0xf94019c2, 0xf9017be2, 0xd37df001, 0xaa0203e0, 0x97fad35b, 0xf941ebe2, 0xf9402c43, 0xf941b3ec, 0xeb03019f, 0x540099c8, 0xf940244e],
  [0xf941b7e1, 0xeb0c003f, 0x540000ad, 0xd503201f, 0xd503201f, 0xaa1f03ef, 0x140004c3, 0x9108205b, 0xa940436f, 0x8b0c0191, 0xeb0c05ff, 0x54009703, 0xf9411053, 0xeb0c067f, 0x54009643, 0xf9014ff1],
  [0xf90123e3, 0xf900dbef, 0xf900e3f3, 0xf90167f0, 0xf90183ee, 0xf940fc54, 0xf90163f4, 0xaa1f03e0, 0xaa1f03e4, 0x9e6703e0, 0xaa1f03e5, 0xaa1f03e6, 0xaa1f03e7, 0x9e6703e1, 0x14000015, 0xfd40e7e3],
  [0x1e642864, 0xfd4103e5, 0x90000a3b, 0xfd40b766, 0x1f461481, 0x910005d5, 0xf94123e3, 0xf94183ee, 0xf940dbef, 0xaa1403f0, 0xf9414ff1, 0xf940e3f3, 0xf94163f4, 0xaa0003e5, 0xaa0103e6, 0xaa0203e7],
  [0xf941b7e1, 0xf941ebe2, 0xaa1503e0, 0xfd0107e0, 0xf900dfe4, 0x390653e6, 0x390647e5, 0x39064fe7, 0xeb00019f, 0x5400122d, 0xf941e3e8, 0xeb00011f, 0x54009069, 0xf941dfe9, 0xf860792a, 0xeb0a003f],
  [0x540010c9, 0xf941cbeb, 0xeb00017f, 0x54008f69, 0xf941c7ed, 0xfc6079a3, 0x1e60c064, 0x90000a3b, 0xfd415b65, 0x1e652080, 0x54000f08, 0x90000a3b, 0xfd415366, 0x1e6420c0, 0x54000e04, 0x9b012815],
  [0xf941bff6, 0xeb1502df, 0x54008d29, 0xf941bbf7, 0xfc757ae7, 0x1e60c0e8, 0x1e6820a0, 0x54000c64, 0x1e662100, 0x54000c28, 0xf941d7f5, 0xeb0a02bf, 0x54008ba9, 0xf900ffea, 0xfd012be4, 0xfd0127e8],
  [0xf9011fe0, 0xfd0103e1, 0xf941d3e0, 0xfc6a7802, 0xfd00cfe2, 0x1e6040e0, 0x1e604061, 0x97fffe39, 0x360009c0, 0x1e602000, 0x54000981, 0xfd4127e3, 0xfd412be4, 0x1e642865, 0xfd40cfe6, 0x1e60c0c6],
  [0x1e6628a5, 0x1e602068, 0x540000ed, 0x1e6428c6, 0x90000a3b, 0xfd412367, 0x1e6308e3, 0x1e662060, 0x540006c4, 0x1e60c003, 0x90000a3b, 0xfd40af66, 0x1e6508c5, 0x1e6320a0, 0x54000684, 0x1e602008],
  [0x54000345, 0x1e614003, 0xf9411fee, 0xf9417bef, 0xfc2e79e3, 0xf940dff0, 0xf941b3ec, 0xeb0c061f, 0x5400014a, 0x540085a2, 0xf94163f1, 0xf8307a2e, 0xf940fff3, 0xf94167f4, 0xf8307a93, 0x91000604],
  [0x394647e0, 0x14000005, 0xf94163f1, 0xf94167f4, 0xaa1003e4, 0xb24003e0, 0xfd4107e0, 0xb24003e1, 0xb24003e2, 0x17ffff86, 0x540000cd, 0xfd4107e3, 0x90000a3b, 0xfd40cb65, 0x1f450c00, 0x14000002],
  [0xfd4107e0, 0x9a9fd7ee, 0x394653ef, 0xaa0f01ce, 0xf941b3ec, 0xf9417bef, 0xf94163f1, 0xf94167f4, 0xf940dfe4, 0xaa0e03e1, 0x394647e0, 0x39464fe2, 0xf9411fee, 0x17ffff72, 0xb24003e0, 0xa97ffbfd],
  [0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd],
  [0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x1e612801, 0x1e612821, 0x9e620182, 0x90000a3b, 0xfd40f363, 0x1e630843],
  [0xf0000a1b, 0xfd41b364, 0x1f440461, 0x1e60c023, 0x90000a3b, 0xfd415b64, 0x1e642060, 0x54000188, 0xf9411c4d, 0xeb0d019f, 0x54007b68, 0xf941144b, 0xaa1f03f5, 0xaa0603e0, 0xf941cbe6, 0xaa0503e8],
  [0xf941c7e5, 0xfd40e7e3, 0x14000007, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xfc357965, 0x910006b5, 0xeb15019f, 0x5400032d, 0xeb1500df, 0x540078e9, 0xfc7578a5, 0x1e60c0a5, 0x1e652865],
  [0x90000a3b, 0xfd40b366, 0x1f4604a5, 0xf0000a1b, 0xfd423367, 0x90000a3b, 0xfd40cf68, 0x1f481ca5, 0x1e60c0a9, 0x1e692080, 0x540000a4, 0x90000a3b, 0xfd415769, 0x1e6920a0, 0x54fffd29, 0xb24003e0],
  [0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xa95c5855, 0xeb0c06bf, 0x54007563, 0xa95de057, 0xeb0c06ff, 0x540074a3, 0xf940f859, 0xeb0c073f, 0x540073e3, 0xf941405a, 0xf101035f, 0x54007323, 0xf900f3fa],
  [0xd37ef59e, 0x9109405b, 0xa9402b69, 0xf10103df, 0xb27a03fa, 0x9a9ac3da, 0xeb09035f, 0x540071c8, 0xf941345e, 0xeb1e035f, 0x54007108, 0xf90197f8, 0xfd015fe2, 0xfd013be1, 0xf9013ff5, 0xf900fbed],
  [0xf9017feb, 0xf9014bf7, 0xf90173ea, 0xf9019bf6, 0xf90143f9, 0xf900effa, 0xf941204e, 0xf9016fee, 0xf940d84f, 0xf90193ef, 0xf941384f, 0xf90177ef, 0xd503201f, 0xf941dfef, 0xf941e3f0, 0xa900c3ef],
  [0xf941e7ef, 0xa901bbef, 0xa902a7fa, 0xa903ebea, 0xf90027fe, 0xaa0c03e0, 0xf941bbe2, 0xf941bfe3, 0xf941c3e4, 0xf941cfe7, 0xf941d3e8, 0xf941d7e9, 0xf941dbea, 0x97ffad77, 0x37000141, 0xf90137e0],
  [0xaa1f03e1, 0xaa1f03e2, 0xf940dfe3, 0xaa1f03e4, 0x394647e5, 0x394653e6, 0x39464fe7, 0x1400029d, 0xf941dff0, 0xf941e3f1, 0xa900c7f0, 0xf941e7f0, 0xf941a7f1, 0xa901c7f0, 0xf941abf0, 0xf941aff1],
  [0xa902c7f0, 0xf9417bf0, 0xf941b3ec, 0xa903b3f0, 0xf940f7f0, 0xf94183f1, 0xa904c7f0, 0xf94123f0, 0xa905c3ec, 0xf94193f0, 0xf9414ff1, 0xa906c7f0, 0xf9413ff0, 0xf9419bf3, 0xa907cff0, 0xf9414bf0],
  [0xa908c3f1, 0xf94197f0, 0xa909c7f0, 0xf94143f0, 0xf94163f3, 0xa90acff0, 0xf940dbf0, 0xa90bc3f1, 0xf94167f0, 0xa90cc7f0, 0xf940e3f0, 0xf94177f1, 0xa90dc7f0, 0xb27a03f0, 0xf940f3f1, 0xa90ec7f0],
  [0x39464ff0, 0x3903e3f0, 0x394f63f0, 0x3903e7f0, 0xaa0c03e0, 0xf941b7e1, 0xf941bbe2, 0xf941bfe3, 0xf941c3e4, 0xf941c7e5, 0xf941cbe6, 0xf941cfe7, 0xf941d3e8, 0xf941d7e9, 0xf941dbea, 0xf9417feb],
  [0xf940fbed, 0xf940dfee, 0x394647ef, 0xfd413be0, 0x94000368, 0x36000085, 0x394653ef, 0xaa0f008f, 0x14000005, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x3600016f, 0x36000104, 0xaa1f03ef],
  [0xfd4107e0, 0xf941b3ec, 0xf9417beb, 0xf941cbe6, 0xf941c7e5, 0x1400022b, 0xfd4107e0, 0x14000005, 0xaa1f03e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x1e6e1003, 0x90000a3b, 0xfd40b364, 0xfd415fe5],
  [0x1f440ca3, 0x1e630803, 0xf0000a1b, 0xfd41b364, 0x1f440ca3, 0x1e60c064, 0x90000a3b, 0xfd415b65, 0x1e652080, 0x540009a8, 0xf941ebef, 0xa94d45f0, 0xf941b3ec, 0xeb10019f, 0x540041e8, 0xf94075f3],
  [0xeb13019f, 0x54004148, 0xf0000a1b, 0xfd41e364, 0x90000a3b, 0xfd40cb66, 0x1f4310c1, 0xfd413be3, 0x1e612060, 0x54000724, 0xf90187f1, 0xfd012fe1, 0xf900dfe1, 0x390647e3, 0x39064fe4, 0xf94061f4],
  [0xf9018bf4, 0x37000102, 0xaa1f03f0, 0xf94193f3, 0xf94197f5, 0xf9417beb, 0xf9419bf6, 0xaa1f03e2, 0x140001b6, 0xf941a7ef, 0xf941abf5, 0xa900d7ef, 0xf941afef, 0xf9417bf5, 0xa901d7ef, 0xf940f7ef],
  [0xa902bfec, 0xa903b3f4, 0xa904c7f0, 0xa905cfec, 0xf94177ef, 0xb27a03f0, 0xa906c3ef, 0xf940f3ef, 0xf9003fef, 0xaa0c03e0, 0xf941b7e1, 0xf941bbe2, 0xf941bfe3, 0xf941c3e4, 0xf941c7e5, 0xf941cbe6],
  [0xf941cfe7, 0xf941d3e8, 0xf941d7e9, 0xf941dbea, 0xf941dfeb, 0xf941e3ec, 0xf941e7ed, 0xfd40e7e0, 0x394f63ee, 0x940004b3, 0xf941b7ef, 0xf941b3ec, 0xeb0c01ff, 0x370001a1, 0xb24003e0, 0xa97ffbfd],
  [0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x9a8cd590, 0xf941ebf1, 0xf9403a33, 0xeb13021f, 0x54002f88, 0xf9405e33],
  [0xeb13021f, 0x54002ec8, 0xf9403233, 0xf9405634, 0xaa1f03f5, 0x14000005, 0x92800016, 0xf8357a76, 0xf8357a9f, 0x910006b5, 0xeb1002bf, 0x54ffff6b, 0xaa1f03f5, 0xf9418bf6, 0xf94187f7, 0x14000005],
  [0xf8797a98, 0x91000718, 0xf8397a98, 0x910006b5, 0xeb0002bf, 0x540001aa, 0xeb15019f, 0x54002bc9, 0xf8757ad8, 0xf8757af9, 0xeb18021f, 0x54002ae9, 0xf8787a7a, 0xf8357ada, 0xf8387a75, 0xeb19021f],
  [0x54fffe08, 0x1400014e, 0xeb0c01ff, 0x5400010d, 0xaa1f03f5, 0xf941e3f8, 0xf941dff9, 0xf941d7e9, 0xf941d3e8, 0xfd412fe3, 0x1400012c, 0xaa1f03f5, 0x92800018, 0x14000004, 0x91000419, 0xaa1503f8],
  [0xaa1903f5, 0xeb1002bf, 0x5400016a, 0xf8757a99, 0xb50000d9, 0x91000b19, 0xcb1903f9, 0xf8357a99, 0xaa1503e0, 0x17fffff5, 0xaa1503e0, 0xaa1803f5, 0x17fffff2, 0xaa1f03f5, 0xf941e3f9, 0xf941dffa],
  [0xf941d7e9, 0xf941d3e8, 0xfd412fe3, 0x14000004, 0x910006b5, 0xaa0003f1, 0xaa1e03f8, 0xb7f80438, 0xeb10031f, 0x54002122, 0xf8787a9e, 0xaa1103e0, 0x92800031, 0xcb1e023e, 0xeb0c01ff, 0x540000ad],
  [0xeb18019f, 0x54000061, 0xaa1f03f8, 0x140000e1, 0xf8787a78, 0x14000003, 0xf8787ad8, 0xaa0103fe, 0xb7fffd98, 0xeb18019f, 0x54001aa9, 0xf8787ae1, 0xeb01021f, 0x54001a09, 0xf8617a82, 0xd1000442],
  [0xf8217a82, 0xb50000a2, 0x91000bc2, 0xcb0203e2, 0xf8217a82, 0x17fffff1, 0xaa1e03e1, 0x17ffffef, 0xeb1002bf, 0x54000541, 0x39464fe0, 0x36000580, 0x394647e0, 0x370001c0, 0xf9402e21, 0xeb01019f],
  [0x54001788, 0xf9405e21, 0xeb01019f, 0x54001708, 0xf9402621, 0xf9405622, 0xaa1f03e3, 0xf9417be4, 0xf941cbe5, 0xf941c7e6, 0x14000022, 0xf94177f0, 0xb27a03f1, 0xa900c7f0, 0xf940f3f0, 0xf9000ff0],
  [0xaa0c03e0, 0xaa0f03e1, 0xf941bbe2, 0xf941bfe3, 0xf941c3e4, 0xf941c7e5, 0xf941cbe6, 0xf941cfe7, 0xf941dbea, 0xf9417beb, 0xf940f7ed, 0xfd40e7e0, 0x394f63ee, 0x94000533, 0x37000120, 0xb24003e0],
  [0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xaa1f03e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x91000463, 0xeb03019f, 0x5400036d, 0xfc637883, 0x1e602068],
  [0x540002ad, 0xeb0300bf, 0x54001109, 0xfc6378c4, 0x1e633883, 0x90000a3b, 0xfd415b64, 0x1e632080, 0x54000085, 0x90000a3b, 0xfd415b63, 0x14000007, 0x90000a3b, 0xfd416b65, 0x1e652060, 0x54000065],
  [0x90000a3b, 0xfd416b63, 0xfc237823, 0xf823785f, 0x17ffffe7, 0x90000a3b, 0xfd415b64, 0x17ffffe4, 0xf9018fe1, 0xf9016be2, 0xaa1f03e0, 0x1400000c, 0xf9410be3, 0x91000460, 0xf9418fe1, 0xf9416be2],
  [0xf9417be4, 0xf941cbe5, 0xf941c7e6, 0xf941d3e8, 0xf941d7e9, 0xf941b3ec, 0xf941b7ef, 0xf940dfe3, 0xeb03001f, 0x540009aa, 0xeb0c041f, 0x54000ba2, 0xf94163e7, 0xf86078ea, 0xf94167eb, 0xf860796d],
  [0x9b0f354e, 0xf941bff0, 0xeb0e021f, 0x54000a69, 0xf941bbf1, 0xfc6e7a20, 0xeb0d013f, 0x540009c9, 0xfc6d7902, 0xeb0a019f, 0x54000949, 0xf90117ea, 0xfd0153e0, 0xfd00d3e2, 0xf9010be0, 0xfc6a7821],
  [0xfd0133e1, 0x97fffbbf, 0x36000600, 0x1e602000, 0x540005c1, 0x1e602008, 0x54fffac5, 0xf94117e1, 0xf9416be2, 0xf8617843, 0xb5000483, 0xd503201f, 0xfd4133e0, 0x90000a3b, 0xfd416f61, 0x97fbba9d],
  [0x90000a3b, 0xfd415b62, 0x1e602040, 0x54000085, 0x90000a3b, 0xfd415b60, 0x14000007, 0x90000a3b, 0xfd416b63, 0x1e632000, 0x54000065, 0x90000a3b, 0xfd416b60, 0xf94117e0, 0xf9418fe1, 0xfc207820],
  [0xb24003e1, 0xf9416be2, 0xf8207841, 0x1e604001, 0xfd40d3e2, 0xfd4153e0, 0x97fffb9a, 0x36000060, 0x1e602008, 0x54fff66a, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd],
  [0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xaa1f03e2, 0x14000002, 0x91000442, 0xeb02019f, 0x54ffefad, 0xfc627880, 0x1e602008, 0x54ffff6d, 0xfc627820, 0xeb0200bf],
  [0x54000069, 0xfc2278c0, 0x17fffff6, 0x97fad019, 0x97fad018, 0x97fad017, 0xaa1003e0, 0x97fad015, 0xf9414fe1, 0x97fad013, 0x97fad012, 0x97fad011, 0x97fad010, 0xaa1003e0, 0x97fad00e, 0xaa1803e0],
  [0x97fad00c, 0x91000421, 0xaa1803fe, 0xaa0103f8, 0xeb18019f, 0x54ffe1ed, 0xeb18033f, 0x54000309, 0xf8787b41, 0xeb01013f, 0x54000289, 0xfc617904, 0x1e614084, 0x1e632080, 0x540001a8, 0xf8787a81],
  [0xd1000421, 0xf8387a81, 0xb50000c1, 0x91000bc1, 0xcb0103e1, 0xf8387a81, 0xaa1803e1, 0x17ffffea, 0xaa1803e1, 0xaa1e03f8, 0x17ffffe7, 0xaa1803e1, 0xaa1e03f8, 0x17ffffe4, 0x97facfee, 0xaa1803e0],
  [0xaa1903e1, 0x97facfeb, 0xaa1803e0, 0xaa1003e1, 0x97facfe8, 0x910006b5, 0xeb15019f, 0x54ffda8d, 0xeb15031f, 0x54000269, 0xf8757b3a, 0xeb1a013f, 0x540001c9, 0xfc7a7904, 0x1e614084, 0x1e632080],
  [0x54fffea8, 0xeb1002bf, 0x540000a2, 0xf8757a9a, 0x9100075a, 0xf8357a9a, 0x17ffffef, 0xaa1503e0, 0xaa1003e1, 0x97facfd3, 0xaa1a03e0, 0x97facfd1, 0xaa1503e0, 0xaa1803e1, 0x97facfce, 0xaa1903e0],
  [0xaa1003e1, 0x97facfcb, 0xaa1803e0, 0xaa1003e1, 0x97facfc8, 0xaa1503e0, 0x97facfc6, 0xaa1003e0, 0xaa1303e1, 0x97facfc3, 0xaa1003e0, 0xaa1303e1, 0x97facfc0, 0x91000610, 0xeb00021f, 0x540002ea],
  [0xeb0c061f, 0x540006e2, 0xf8707a77, 0xfc707aa3, 0xeb17019f, 0x54000629, 0xfc777964, 0x1e632883, 0x1e632020, 0x54fffe84, 0xeb02019f, 0x540000ed, 0x54000529, 0xf8227a97, 0xf8707ad7, 0xf8227a37],
  [0x91000442, 0x17ffffec, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xf941b7f0, 0xeb0c021f, 0x5400008d, 0xaa1f03f3, 0xf94183f5, 0x14000005, 0xaa1003ef, 0xaa0203e0, 0x17fffe5c, 0x91000673],
  [0xeb13019f, 0x5400022d, 0xfc737aa3, 0xfc737964, 0x1e632883, 0x1e612060, 0x54ffff28, 0xeb02019f, 0x540000cd, 0x54000169, 0xf8227a93, 0xf8227a2c, 0x91000442, 0x17fffff2, 0xb24003e0, 0xa97ffbfd],
  [0x910d03ff, 0xd65f03c0, 0xeb0c021f, 0x17ffffe9, 0x97facf88, 0x97facf87, 0xaa1703e0, 0x97facf85, 0xaa1003e0, 0xf9414fe1, 0x97facf82, 0xaa1303e0, 0x97facf80, 0xaa1003e0, 0x97facf7e, 0x910005ef],
  [0xeb0f019f, 0x54ffbb6d, 0xfc6f7963, 0x1e602068, 0x540001cd, 0xeb0f00df, 0x540002a9, 0xfc6f78a4, 0x1e60c084, 0x90000a3b, 0xfd40a765, 0x1e6408a4, 0x1e642060, 0x54000144, 0x90000a3b, 0xfd40cb64],
  [0x1f440060, 0x17ffffee, 0x90000a3b, 0xfd40cb64, 0x90000a3b, 0xfd40a765, 0x17ffffe9, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0x97facf61, 0xf9410ff9, 0x91000739, 0xaa0003e5, 0xaa0103e6],
  [0xaa0203e7, 0xf94137e0, 0xaa1903e1, 0xaa1703e2, 0xeb01001f, 0x540014cd, 0xf940efe8, 0xeb08003f, 0x54001722, 0xf9416fe9, 0xf861792a, 0xf94173eb, 0xf861796c, 0xf941cbed, 0xeb0a01bf, 0x54001629],
  [0xf941b7ee, 0x9b0e314f, 0xf941c7f0, 0xfc6a7a01, 0xf941bff1, 0xeb0f023f, 0x54001509, 0xf941bbf3, 0xfc6f7a60, 0xf941d7ef, 0xeb0c01ff, 0x54001449, 0xf9011bea, 0xf90113ec, 0xfd00ebe1, 0xfd0157e0],
  [0xf9015be3, 0x39065fe6, 0x39065be5, 0x390657e7, 0xf9010fe1, 0xf90147e2, 0x39064be4, 0xf941d3e0, 0xfc6c7802, 0xfd00d7e2, 0x97fffa96, 0x36000f80, 0x1e602000, 0x54000f41, 0x1e602008, 0x54000585],
  [0xfd40ebe3, 0x1e60c063, 0x1e614004, 0xfd4157e5, 0x1e60c0a5, 0x1e6328a3, 0xfd40d7e5, 0x1e60c0a5, 0x1e652863, 0x90000a3b, 0xfd40af65, 0x1e650863, 0x1e642060, 0x54000bc4, 0xf941b3ec, 0xf9411bef],
  [0xeb0f019f, 0x54000f69, 0xf9417beb, 0xfc6f7963, 0x1e642060, 0x54000045, 0xfc2f7964, 0xf9415bf0, 0xeb0c061f, 0x5400014a, 0x54000de2, 0xf94163f1, 0xf8307a2f, 0xf94113f3, 0xf94167f4, 0xf8307a93],
  [0x91000603, 0x39465be0, 0x14000006, 0xf94163f1, 0xf94113f3, 0xf94167f4, 0xaa1003e3, 0xb24003e0, 0xb24003e1, 0xb24003e2, 0x1400000b, 0xf9417beb, 0xf941b3ec, 0xf9411bef, 0xf94163f1, 0xf94113f3],
  [0xf94167f4, 0xf9415be3, 0x39465fe1, 0x39465be0, 0x394657e2, 0xfd413be3, 0x1e632000, 0x54000568, 0xf941abf0, 0xeb13021f, 0x54000989, 0xf941a7f5, 0xf8737ab3, 0xf941b7f6, 0xeb0c02df, 0xb6f801f3],
  [0x540005e0, 0xeb0f019f, 0x54000869, 0xf94183f3, 0xfc6f7a64, 0x1e642000, 0x54000045, 0xfc2f7a60, 0xf9419bef, 0xf94197f3, 0xf94193f8, 0xf94147f7, 0x39464be4, 0x17ffff7f, 0xeb0f027f, 0x54000480],
  [0xf94147f7, 0xeb0c06ff, 0x5400016a, 0x540005e2, 0xf94193f8, 0xf8377b0f, 0xf9419bef, 0xf83779f3, 0xf94197f3, 0xfc377a60, 0x910006f7, 0x39464be4, 0x17ffff70, 0xf9419bef, 0xf94197f3, 0xf94193f8],
  [0xb24003e4, 0x17ffff6b, 0xf9419bef, 0xf941abf0, 0xf94197f3, 0xf941a7f5, 0xf941b7f6, 0xf94193f8, 0xf94147f7, 0x39464be4, 0x17ffff62, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0],
  [0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xb24003e0, 0xa97ffbfd, 0x910d03ff, 0xd65f03c0, 0xaa0303e1, 0xaa0203e0, 0xaa0603ef, 0xaa0503e3, 0xaa0403e2],
  [0xaa0703e4, 0x17fffcfc, 0xaa1703e0, 0xf9414fe1, 0x97facea8, 0x97facea7, 0xaa1303e0, 0xaa1003e1, 0x97facea4, 0xaa1003e0, 0xf9414fe1, 0x97facea1, 0x97facea0, 0x97face9f, 0xaa1103e0, 0x97face9d],
  [0x97face9c, 0x97face9b, 0xaa1a03e0, 0xaa1e03e1, 0x97face98, 0xaa1a03e0, 0x97face96, 0xaa1a03e0, 0xb27a03e1, 0x97face93, 0xaa1103e0, 0xaa1903e1, 0x97face90, 0xaa1103e0, 0xaa1703e1, 0x97face8d],
  [0xaa1103e0, 0xaa1503e1, 0x97face8a, 0xaa1503e0, 0x97face88, 0x97face87, 0xaa1003e0, 0xf9414fe1, 0x97face84, 0xaa1503e0, 0x97face82, 0xaa1503e0, 0xaa1603e1, 0x97face7f, 0x97face7e, 0x97face7d],
  [0xaa1103e0, 0xaa1303e1, 0x97face7a, 0xaa1103e0, 0x97face78, 0xf0000a1b, 0xfd415f63, 0xfc2f79c3, 0x910005ef, 0xeb0f019f, 0x54ffff6c, 0x17fffb3c, 0x97face70, 0xaa1003e1, 0x97face6e, 0xaa1003e0],
  [0xaa1103e1, 0x97face6b, 0x97face6a, 0xd503201f, 0xa90207e0, 0xa9030fe2, 0xa90417e4, 0xa9051fe6, 0xa90627e8, 0xa9072fea, 0xa90837ec, 0xf9004bee, 0x390263ef, 0xaa1e03e3, 0x97fac626, 0xa94207e0],
  [0xa9430fe2, 0xa94417e4, 0xa9451fe6, 0xa94627e8, 0xa9472fea, 0xa94837ec, 0xf9404bee, 0x394263ef, 0x17fffab8]
]
def hasPotentialCancellationWords : Array UInt32 := ⟨hasPotentialCancellationChunks.flatten⟩
def hasPotentialCancellationTargets (idx : Nat) : Option String :=
  match idx with
  | 89 => some "runtime.memclrNoHeapPointers"
  | 199 | 833 | 870 | 1130 => some "assign.exactReducedCost"
  | 413 => some "internal/simd.ScanNearTightMatrix64Scalar"
  | 484 => some "assign.scanCancelRowsFallback"
  | 585 => some "assign.buildCancelEdgesFallback"
  | 749 => some "assign.projectCancelViolFallback"
  | 847 => some "math.Nextafter"
  | 899 | 900 | 901 | 903 | 905 | 906 | 907 | 908 | 910 | 912 | 942 | 945 | 948 | 969 | 971 | 974 | 977 | 980 | 982 | 985 | 988 | 1044 | 1045 | 1047 | 1050 | 1052 | 1054 | 1083 | 1268 | 1269 | 1272 | 1275 | 1276 | 1277 | 1279 | 1280 | 1281 | 1284 | 1286 | 1289 | 1292 | 1295 | 1298 | 1300 | 1301 | 1304 | 1306 | 1309 | 1310 | 1311 | 1314 | 1316 | 1324 | 1326 | 1329 | 1330 => some "runtime.panicBounds"
  | 1342 => some "runtime.morestack_noctxt.abi0"
  | _ => none

/-!
### Conservative AArch64 Scalar FP Extension (`FMADDD` & `FMOVD $imm8`) and Tier 5B Safety Proofs
-/

/-- ARMv8-A `VFPExpandImm` expansion of an 8-bit modified immediate into IEEE-754 binary64 bits. -/
def vfpExpandImm64 (imm8 : Nat) : BitVec 64 :=
  let b7 := (imm8 >>> 7) &&& 1
  let b6 := (imm8 >>> 6) &&& 1
  let notB6 := if b6 == 1 then 0 else 1
  let repB6 := if b6 == 1 then 0xff else 0
  let low6 := imm8 &&& 0x3f
  let hi16 := (b7 <<< 15) ||| (notB6 <<< 14) ||| (repB6 <<< 6) ||| low6
  (BitVec.ofNat 64 hi16) <<< 48

/-- Exact IEEE-754 single-rounding scalar binary64 fused multiply-add (`FMADDD`/`FMSUBD`/`FNMADDD`/`FNMSUBD`).
Follows ARMv8 `FPMulAdd(a=ra, b=rn, c=rm)` NaN propagation: sNaNs quiet in `(ra, rn, rm)` order,
qNaN addend with `0 * Inf` produces default qNaN `0x7ff8000000000000`, otherwise qNaNs propagate in `(ra, rn, rm)` order. -/
def f64MulAdd (o1 o0 : Bool) (rn rm ra : BitVec 64) : BitVec 64 :=
  if f64IsSNaN ra then f64QuietNaN ra
  else if f64IsSNaN rn then f64QuietNaN rn
  else if f64IsSNaN rm then f64QuietNaN rm
  else
    let prodInvalid :=
      (f64IsZero rn && (f64IsPosInf rm || f64IsNegInf rm)) ||
      (f64IsZero rm && (f64IsPosInf rn || f64IsNegInf rn))
    if prodInvalid then 0x7ff8000000000000#64
    else if f64IsNaN ra then ra
    else if f64IsNaN rn then rn
    else if f64IsNaN rm then rm
    else
      let aEff := if o1 then f64Neg ra else ra
      let prodNegRaw := f64Sign rn != f64Sign rm
      let prodNeg := if o0 != o1 then !prodNegRaw else prodNegRaw
      let prodInf := f64IsPosInf rn || f64IsNegInf rn || f64IsPosInf rm || f64IsNegInf rm
      if prodInf then
        let pBits := if prodNeg then 0xfff0000000000000#64 else 0x7ff0000000000000#64
        f64Add aEff pBits
      else if f64IsPosInf aEff || f64IsNegInf aEff then
        aEff
      else
        match float64BitsToRat (UInt64.ofNat rn.toNat),
              float64BitsToRat (UInt64.ofNat rm.toNat),
              float64BitsToRat (UInt64.ofNat aEff.toNat) with
        | some qn, some qm, some qa =>
          let qp := if o0 != o1 then -(qn * qm) else qn * qm
          let negZ := f64IsZero aEff && f64Sign aEff && (qn == 0 || qm == 0) && prodNeg
          ratToFloat64Bits (qa + qp) negZ
        | _, _, _ => 0x7ff8000000000000#64

/-- Conservative extension of `Arm64Insn` adding scalar binary64 `FMADD`/`FMSUB`/`FNMADD`/`FNMSUB` and immediate `FMOVD`. -/
inductive Arm64InsnD where
  | base (insn : Arm64Insn)
  | fmaddD (o1 o0 : Bool) (rd rn rm ra : BitVec 5)
  | fmovImmD (rd : BitVec 5) (immBits : BitVec 64)
  deriving DecidableEq, Repr, Inhabited

/-- Decode a 32-bit little-endian AArch64 word into `Arm64InsnD`. -/
def decodeArm64WordD (word : UInt32) : Arm64InsnD :=
  match decodeArm64Word word with
  | .unknown _ =>
    let w : Nat := word.toNat
    let rd : BitVec 5 := BitVec.ofNat 5 (w &&& 0x1f)
    if (w &&& 0xffc00000) == 0x1f400000 then
      let o1 := ((w >>> 21) &&& 1) == 1
      let rm : BitVec 5 := BitVec.ofNat 5 ((w >>> 16) &&& 0x1f)
      let o0 := ((w >>> 15) &&& 1) == 1
      let ra : BitVec 5 := BitVec.ofNat 5 ((w >>> 10) &&& 0x1f)
      let rn : BitVec 5 := BitVec.ofNat 5 ((w >>> 5) &&& 0x1f)
      .fmaddD o1 o0 rd rn rm ra
    else if (w &&& 0xffe01fe0) == 0x1e601000 then
      let imm8 := (w >>> 13) &&& 0xff
      .fmovImmD rd (vfpExpandImm64 imm8)
    else
      .base (.unknown word)
  | insn => .base insn

def decodeArm64ProgD (words : Array UInt32) : Array Arm64InsnD :=
  ⟨words.toList.map decodeArm64WordD⟩

/-- Control-flow-identical `Arm64Insn` projection preserving exact `pcIdx` and `ExecStatus` steps. -/
def toCfgProxy : Arm64InsnD → Arm64Insn
  | .base insn => insn
  | .fmaddD _ _ rd _ _ _ => .fpUnaryD 0 rd rd
  | .fmovImmD rd _ => .fpUnaryD 0 rd rd

def decodeArm64WordCfg (word : UInt32) : Arm64Insn :=
  toCfgProxy (decodeArm64WordD word)

def decodeArm64ProgCfg (words : Array UInt32) : Array Arm64Insn :=
  ⟨words.toList.map decodeArm64WordCfg⟩

/-- Execute one `Arm64InsnD` instruction in `Arm64State`. -/
def execArm64InsnD (progLen : Nat) (extTargets : Nat → Option String)
    (insn : Arm64InsnD) (s : Arm64State) : Arm64State :=
  match insn with
  | .base b => execArm64Insn progLen extTargets b s
  | .fmaddD o1 o0 rd rn rm ra =>
    let res := f64MulAdd o1 o0 (s.fpRegs rn) (s.fpRegs rm) (s.fpRegs ra)
    advancePc (writeFp s rd res)
  | .fmovImmD rd immBits =>
    advancePc (writeFp s rd immBits)

def stepArm64D (prog : Array Arm64InsnD) (extTargets : Nat → Option String)
    (s : Arm64State) : Arm64State :=
  if s.status != .running then s
  else if h : s.pcIdx < prog.size then
    execArm64InsnD prog.size extTargets prog[s.pcIdx] s
  else
    { s with status := .trapIllegal }

def runArm64D (fuel : Nat) (prog : Array Arm64InsnD) (extTargets : Nat → Option String)
    (s : Arm64State) : Arm64State :=
  match fuel with
  | 0 => s
  | fuel' + 1 =>
    if s.status != .running then s
    else runArm64D fuel' prog extTargets (stepArm64D prog extTargets s)

theorem runArm64D_succ_running (fuel : Nat) (prog : Array Arm64InsnD) (ext : Nat → Option String)
    (s : Arm64State) (h : s.status = .running) :
    runArm64D (fuel + 1) prog ext s = runArm64D fuel prog ext (stepArm64D prog ext s) := by
  simp [runArm64D, h]

theorem runArm64D_of_not_running (fuel : Nat) (prog : Array Arm64InsnD) (ext : Nat → Option String)
    (s : Arm64State) (h : s.status ≠ .running) :
    runArm64D fuel prog ext s = s := by
  cases fuel with
  | zero => rfl
  | succ k =>
    have hne : (s.status != .running) = true := bne_iff_ne.mpr h
    simp [runArm64D, hne]

private theorem execArm64InsnD_proxy_status_pc (n : Nat) (ext : Nat → Option String)
    (insn : Arm64InsnD) (s : Arm64State) :
    (execArm64InsnD n ext insn s).status = (execArm64Insn n ext (toCfgProxy insn) s).status ∧
    (execArm64InsnD n ext insn s).pcIdx = (execArm64Insn n ext (toCfgProxy insn) s).pcIdx := by
  cases insn with
  | base b => exact ⟨rfl, rfl⟩
  | fmaddD o1 o0 rd rn rm ra => exact ⟨rfl, rfl⟩
  | fmovImmD rd immBits => exact ⟨rfl, rfl⟩

/-- Exact single-step `status` and `pcIdx` agreement between `stepArm64D` and `stepArm64` on `decodeArm64ProgCfg`. -/
theorem stepArm64D_proxy_status_pc (words : Array UInt32) (ext : Nat → Option String) (s : Arm64State) :
    let sD := stepArm64D (decodeArm64ProgD words) ext s
    let sP := stepArm64 (decodeArm64ProgCfg words) ext s
    sD.status = sP.status ∧ sD.pcIdx = sP.pcIdx := by
  have hSzD : (decodeArm64ProgD words).size = words.toList.length := by
    unfold decodeArm64ProgD; exact List.length_map _
  have hSzP : (decodeArm64ProgCfg words).size = words.toList.length := by
    unfold decodeArm64ProgCfg; exact List.length_map _
  dsimp only [stepArm64D, stepArm64]
  by_cases hStop : (s.status != .running) = true
  · simp [hStop]
  · simp only [hStop, Bool.false_eq_true, ↓reduceIte]
    by_cases hLt : s.pcIdx < words.toList.length
    · have hLtD : s.pcIdx < (decodeArm64ProgD words).size := hSzD ▸ hLt
      have hLtP : s.pcIdx < (decodeArm64ProgCfg words).size := hSzP ▸ hLt
      simp only [hLtD, hLtP, ↓reduceDIte]
      have hElemEq : (decodeArm64ProgCfg words)[s.pcIdx] = toCfgProxy (decodeArm64ProgD words)[s.pcIdx] := by
        have h1 : (decodeArm64ProgCfg words).toList[s.pcIdx]? = some (decodeArm64ProgCfg words)[s.pcIdx] :=
          Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLtP)
        have h2 : (decodeArm64ProgD words).toList[s.pcIdx]? = some (decodeArm64ProgD words)[s.pcIdx] :=
          Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLtD)
        have hMap : (decodeArm64ProgCfg words).toList[s.pcIdx]? = ((decodeArm64ProgD words).toList[s.pcIdx]?).map toCfgProxy := by
          change (words.toList.map decodeArm64WordCfg)[s.pcIdx]? = ((words.toList.map decodeArm64WordD)[s.pcIdx]?).map toCfgProxy
          rw [List.getElem?_map, List.getElem?_map, Option.map_map]
          rfl
        rw [h1, h2] at hMap
        exact Option.some.inj hMap
      have hSzEq : (decodeArm64ProgCfg words).size = (decodeArm64ProgD words).size := hSzP.trans hSzD.symm
      rw [hElemEq, hSzEq]
      exact execArm64InsnD_proxy_status_pc (decodeArm64ProgD words).size ext (decodeArm64ProgD words)[s.pcIdx] s
    · have hNotD : ¬ s.pcIdx < (decodeArm64ProgD words).size := by omega
      have hNotP : ¬ s.pcIdx < (decodeArm64ProgCfg words).size := by omega
      simp [hNotD, hNotP]

/-! ### Bit-Exact Functional Equivalence of `assign.twoSumAssign` -/

/-- Exact IEEE-754 binary64 Knuth `TwoSum` rounding-error specification matching Go 1.27 ARM64 `assign.twoSumAssign` (`FADDD F0, F1, F2` computes `s := b + a`). -/
def twoSumErrSpec (a b : BitVec 64) : BitVec 64 :=
  let s := f64Add b a
  let bb := f64Sub s a
  f64Add (f64Sub a (f64Sub s bb)) (f64Sub b bb)

private def twoSumProg : Array Arm64Insn := #[
  .fpBinaryD 2 2#5 1#5 0#5,
  .fpBinaryD 3 3#5 2#5 0#5,
  .fpBinaryD 3 4#5 2#5 3#5,
  .fpBinaryD 3 4#5 0#5 4#5,
  .fpBinaryD 3 3#5 1#5 3#5,
  .fpBinaryD 2 1#5 4#5 3#5,
  .fpUnaryD 0 0#5 2#5,
  .ret 30#5 ]

private theorem fpRegs_adv_writeFp (s : Arm64State) (rd q : BitVec 5) (v : BitVec 64) :
    (advancePc (writeFp s rd v)).fpRegs q = if q == rd then v else s.fpRegs q := rfl

private theorem exec_faddD (sz : Nat) (ext : Nat → Option String) (rd rn rm : BitVec 5) (s : Arm64State) :
    execArm64Insn sz ext (.fpBinaryD 2 rd rn rm) s =
      advancePc (writeFp s rd (f64Add (s.fpRegs rn) (s.fpRegs rm))) := rfl

private theorem exec_fsubD (sz : Nat) (ext : Nat → Option String) (rd rn rm : BitVec 5) (s : Arm64State) :
    execArm64Insn sz ext (.fpBinaryD 3 rd rn rm) s =
      advancePc (writeFp s rd (f64Sub (s.fpRegs rn) (s.fpRegs rm))) := rfl

private theorem exec_fmovD (sz : Nat) (ext : Nat → Option String) (rd rn : BitVec 5) (s : Arm64State) :
    execArm64Insn sz ext (.fpUnaryD 0 rd rn) s =
      advancePc (writeFp s rd (s.fpRegs rn)) := rfl

private theorem adv_writeFp_status (st : Arm64State) (rd : BitVec 5) (v : BitVec 64) :
    (advancePc (writeFp st rd v)).status = st.status := rfl

private theorem adv_writeFp_pcIdx (st : Arm64State) (rd : BitVec 5) (v : BitVec 64) :
    (advancePc (writeFp st rd v)).pcIdx = st.pcIdx + 1 := rfl

private theorem adv_writeFp_sp (st : Arm64State) (rd : BitVec 5) (v : BitVec 64) :
    (advancePc (writeFp st rd v)).sp = st.sp := rfl

private theorem fpRegs_adv_eq (st : Arm64State) (rd : BitVec 5) (v : BitVec 64) :
    (advancePc (writeFp st rd v)).fpRegs rd = v := by
  simp only [fpRegs_adv_writeFp, beq_self_eq_true, ↓reduceIte]

private theorem fpRegs_adv_ne (st : Arm64State) (rd q : BitVec 5) (v : BitVec 64)
    (hne : (q == rd) = false) :
    (advancePc (writeFp st rd v)).fpRegs q = st.fpRegs q := by
  simp only [fpRegs_adv_writeFp, hne, Bool.false_eq_true, ↓reduceIte]

private theorem twoSumProg_step8 (s0 : Arm64State) (a b : BitVec 64)
    (addOp subOp : BitVec 64 → BitVec 64 → BitVec 64)
    (hAdd : addOp = f64Add) (hSub : subOp = f64Sub)
    (hRun : s0.status = .running) (hPc : s0.pcIdx = 0)
    (hF0 : s0.fpRegs 0#5 = a) (hF1 : s0.fpRegs 1#5 = b) :
    let s' := runArm64 8 twoSumProg twoSumAssignTargets s0
    let sum := addOp b a
    let bb := subOp sum a
    let err := addOp (subOp a (subOp sum bb)) (subOp b bb)
    s'.status = .returned ∧
      s'.fpRegs 0#5 = sum ∧
      s'.fpRegs 1#5 = err ∧
      s'.sp = s0.sp := by
  dsimp only
  obtain ⟨sum, hSum⟩ : ∃ x, addOp b a = x := ⟨_, rfl⟩
  obtain ⟨bb, hBb⟩ : ∃ x, subOp sum a = x := ⟨_, rfl⟩
  obtain ⟨sbb, hSbb⟩ : ∃ x, subOp sum bb = x := ⟨_, rfl⟩
  obtain ⟨da, hDa⟩ : ∃ x, subOp a sbb = x := ⟨_, rfl⟩
  obtain ⟨db, hDb⟩ : ∃ x, subOp b bb = x := ⟨_, rfl⟩
  obtain ⟨err, hErr⟩ : ∃ x, addOp da db = x := ⟨_, rfl⟩
  rw [hSum, hBb, hSbb, hDa, hDb, hErr]
  obtain ⟨s1, hs1d⟩ : ∃ x, advancePc (writeFp s0 2#5 sum) = x := ⟨_, rfl⟩
  have hs1 : stepArm64 twoSumProg twoSumAssignTargets s0 = s1 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s0 0 (.fpBinaryD 2 2#5 1#5 0#5) hRun hPc (by decide),
        exec_faddD, hF1, hF0, ← hAdd, hSum, hs1d]
  have hR1 : s1.status = .running := by rw [← hs1d, adv_writeFp_status, hRun]
  have hP1 : s1.pcIdx = 1 := by rw [← hs1d, adv_writeFp_pcIdx, hPc]
  obtain ⟨s2, hs2d⟩ : ∃ x, advancePc (writeFp s1 3#5 bb) = x := ⟨_, rfl⟩
  have hs2 : stepArm64 twoSumProg twoSumAssignTargets s1 = s2 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s1 1 (.fpBinaryD 3 3#5 2#5 0#5) hR1 hP1 (by decide),
        exec_fsubD, ← hSub]
    have hf2 : s1.fpRegs 2#5 = sum := by rw [← hs1d, fpRegs_adv_eq]
    have hf0 : s1.fpRegs 0#5 = a := by rw [← hs1d, fpRegs_adv_ne _ _ _ _ (by decide), hF0]
    rw [hf2, hf0, hBb, hs2d]
  have hR2 : s2.status = .running := by rw [← hs2d, adv_writeFp_status, hR1]
  have hP2 : s2.pcIdx = 2 := by rw [← hs2d, adv_writeFp_pcIdx, hP1]
  obtain ⟨s3, hs3d⟩ : ∃ x, advancePc (writeFp s2 4#5 sbb) = x := ⟨_, rfl⟩
  have hs3 : stepArm64 twoSumProg twoSumAssignTargets s2 = s3 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s2 2 (.fpBinaryD 3 4#5 2#5 3#5) hR2 hP2 (by decide),
        exec_fsubD, ← hSub]
    have hf2 : s2.fpRegs 2#5 = sum := by
      rw [← hs2d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs1d, fpRegs_adv_eq]
    have hf3 : s2.fpRegs 3#5 = bb := by rw [← hs2d, fpRegs_adv_eq]
    rw [hf2, hf3, hSbb, hs3d]
  have hR3 : s3.status = .running := by rw [← hs3d, adv_writeFp_status, hR2]
  have hP3 : s3.pcIdx = 3 := by rw [← hs3d, adv_writeFp_pcIdx, hP2]
  obtain ⟨s4, hs4d⟩ : ∃ x, advancePc (writeFp s3 4#5 da) = x := ⟨_, rfl⟩
  have hs4 : stepArm64 twoSumProg twoSumAssignTargets s3 = s4 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s3 3 (.fpBinaryD 3 4#5 0#5 4#5) hR3 hP3 (by decide),
        exec_fsubD, ← hSub]
    have hf0 : s3.fpRegs 0#5 = a := by
      rw [← hs3d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs2d, fpRegs_adv_ne _ _ _ _ (by decide),
          ← hs1d, fpRegs_adv_ne _ _ _ _ (by decide), hF0]
    have hf4 : s3.fpRegs 4#5 = sbb := by rw [← hs3d, fpRegs_adv_eq]
    rw [hf0, hf4, hDa, hs4d]
  have hR4 : s4.status = .running := by rw [← hs4d, adv_writeFp_status, hR3]
  have hP4 : s4.pcIdx = 4 := by rw [← hs4d, adv_writeFp_pcIdx, hP3]
  obtain ⟨s5, hs5d⟩ : ∃ x, advancePc (writeFp s4 3#5 db) = x := ⟨_, rfl⟩
  have hs5 : stepArm64 twoSumProg twoSumAssignTargets s4 = s5 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s4 4 (.fpBinaryD 3 3#5 1#5 3#5) hR4 hP4 (by decide),
        exec_fsubD, ← hSub]
    have hf1 : s4.fpRegs 1#5 = b := by
      rw [← hs4d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs3d, fpRegs_adv_ne _ _ _ _ (by decide),
          ← hs2d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs1d, fpRegs_adv_ne _ _ _ _ (by decide), hF1]
    have hf3 : s4.fpRegs 3#5 = bb := by
      rw [← hs4d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs3d, fpRegs_adv_ne _ _ _ _ (by decide),
          ← hs2d, fpRegs_adv_eq]
    rw [hf1, hf3, hDb, hs5d]
  have hR5 : s5.status = .running := by rw [← hs5d, adv_writeFp_status, hR4]
  have hP5 : s5.pcIdx = 5 := by rw [← hs5d, adv_writeFp_pcIdx, hP4]
  obtain ⟨s6, hs6d⟩ : ∃ x, advancePc (writeFp s5 1#5 err) = x := ⟨_, rfl⟩
  have hs6 : stepArm64 twoSumProg twoSumAssignTargets s5 = s6 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s5 5 (.fpBinaryD 2 1#5 4#5 3#5) hR5 hP5 (by decide),
        exec_faddD, ← hAdd]
    have hf4 : s5.fpRegs 4#5 = da := by
      rw [← hs5d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs4d, fpRegs_adv_eq]
    have hf3 : s5.fpRegs 3#5 = db := by rw [← hs5d, fpRegs_adv_eq]
    rw [hf4, hf3, hErr, hs6d]
  have hR6 : s6.status = .running := by rw [← hs6d, adv_writeFp_status, hR5]
  have hP6 : s6.pcIdx = 6 := by rw [← hs6d, adv_writeFp_pcIdx, hP5]
  obtain ⟨s7, hs7d⟩ : ∃ x, advancePc (writeFp s6 0#5 sum) = x := ⟨_, rfl⟩
  have hs7 : stepArm64 twoSumProg twoSumAssignTargets s6 = s7 := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s6 6 (.fpUnaryD 0 0#5 2#5) hR6 hP6 (by decide),
        exec_fmovD]
    have hf2 : s6.fpRegs 2#5 = sum := by
      rw [← hs6d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs5d, fpRegs_adv_ne _ _ _ _ (by decide),
          ← hs4d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs3d, fpRegs_adv_ne _ _ _ _ (by decide),
          ← hs2d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs1d, fpRegs_adv_eq]
    rw [hf2, hs7d]
  have hR7 : s7.status = .running := by rw [← hs7d, adv_writeFp_status, hR6]
  have hP7 : s7.pcIdx = 7 := by rw [← hs7d, adv_writeFp_pcIdx, hP6]
  have hs8 : stepArm64 twoSumProg twoSumAssignTargets s7 = { s7 with status := .returned } := by
    rw [stepArm64_at twoSumProg twoSumAssignTargets s7 7 (.ret 30#5) hR7 hP7 (by decide)]
    rfl
  rw [runArm64_succ_running 7 _ _ s0 hRun, hs1,
      runArm64_succ_running 6 _ _ s1 hR1, hs2,
      runArm64_succ_running 5 _ _ s2 hR2, hs3,
      runArm64_succ_running 4 _ _ s3 hR3, hs4,
      runArm64_succ_running 3 _ _ s4 hR4, hs5,
      runArm64_succ_running 2 _ _ s5 hR5, hs6,
      runArm64_succ_running 1 _ _ s6 hR6, hs7,
      runArm64_succ_running 0 _ _ s7 hR7, hs8]
  dsimp only [runArm64]
  refine ⟨rfl, ?_, ?_, ?_⟩
  · rw [← hs7d, fpRegs_adv_eq]
  · rw [← hs7d, fpRegs_adv_ne _ _ _ _ (by decide), ← hs6d, fpRegs_adv_eq]
  · rw [← hs7d, adv_writeFp_sp, ← hs6d, adv_writeFp_sp, ← hs5d, adv_writeFp_sp,
        ← hs4d, adv_writeFp_sp, ← hs3d, adv_writeFp_sp, ← hs2d, adv_writeFp_sp,
        ← hs1d, adv_writeFp_sp]

/-- Decoded ARM64 `assign.twoSumAssign` computes `(f64Add b a, twoSumErrSpec a b)` in 8 instructions with zero traps. -/
theorem twoSumAssign_arm64_equiv (s : Arm64State) (a b : BitVec 64)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hF0 : s.fpRegs 0#5 = a) (hF1 : s.fpRegs 1#5 = b) :
    let s' := runArm64 8 (decodeArm64Prog twoSumAssignWords) twoSumAssignTargets s
    s'.status = .returned ∧
      s'.fpRegs 0#5 = f64Add b a ∧
      s'.fpRegs 1#5 = twoSumErrSpec a b ∧
      s'.sp = s.sp := by
  dsimp only
  have hProg : decodeArm64Prog twoSumAssignWords = twoSumProg := by decide
  rw [hProg]
  exact twoSumProg_step8 s a b f64Add f64Sub rfl rfl hRun hPc hF0 hF1

/-! ### Frameless Leaf Safety Analyzer (`twoSumAssign`, `clampFinite`, `exactReducedCost`) -/

def framelessLeafInsnOk (progSize : Nat) (idx : Nat) (insn : Arm64Insn) : Bool :=
  if !insnDecodedValid insn then false
  else
    match insn with
    | .unknown _ | .bl _ => false
    | .b imm26 => !isOutOfBoundsRel progSize idx imm26
    | .bCond _ imm19 | .cbz _ _ _ imm19 => !isOutOfBoundsRel progSize idx imm19
    | .tbz _ _ _ imm14 => !isOutOfBoundsRel progSize idx imm14
    | _ => true

def checkFramelessLeaf (prog : Array Arm64Insn) : Bool :=
  everyWithIdx 0 prog.toList (framelessLeafInsnOk prog.size)

private theorem jumpRel_inBounds_running (progLen : Nat) (ext : Nat → Option String)
    (isCall : Bool) (s : Arm64State) (imm : Int)
    (hRun : s.status = .running)
    (hIn : isOutOfBoundsRel progLen s.pcIdx imm = false) :
    (jumpRel progLen ext isCall s imm).status = .running := by
  unfold jumpRel
  unfold isOutOfBoundsRel at hIn
  have hCond : (0 <= (s.pcIdx : Int) + imm && ((s.pcIdx : Int) + imm).toNat < progLen) = true := by
    simpa [Bool.not_eq_false'] using hIn
  simp only [hCond, ↓reduceIte]
  cases isCall <;> simp [status_writeGpr, hRun]

private theorem checkFramelessLeaf_step (prog : Array Arm64Insn) (ext : Nat → Option String)
    (hOk : checkFramelessLeaf prog = true) (s : Arm64State) (hRun : s.status = .running) :
    (stepArm64 prog ext s).status = .running ∨ (stepArm64 prog ext s).status = .returned ∨
    (stepArm64 prog ext s).status = .trapIllegal := by
  have hAt := everyWithIdx_getElem?_zero prog.toList (framelessLeafInsnOk prog.size) hOk
  unfold stepArm64
  have hNotStop : (s.status != .running) = false := by simp [hRun]
  simp only [hNotStop, Bool.false_eq_true, ↓reduceIte]
  by_cases hLt : s.pcIdx < prog.size
  · simp only [hLt, ↓reduceDIte]
    have hGet : prog.toList[s.pcIdx]? = some prog[s.pcIdx] :=
      Array.getElem?_toList.trans (Array.getElem?_eq_getElem hLt)
    have hI := hAt s.pcIdx prog[s.pcIdx] hGet
    cases hInsn : prog[s.pcIdx] with
    | unknown _ | bl _ => rw [hInsn] at hI; simp [framelessLeafInsnOk, insnDecodedValid] at hI
    | ret rn =>
      unfold execArm64Insn; dsimp only
      split
      · right; left; rfl
      · right; right; rfl
    | b imm26 =>
      rw [hInsn] at hI
      left
      exact jumpRel_inBounds_running prog.size ext false s imm26 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
    | bCond cond imm19 =>
      rw [hInsn] at hI
      left
      dsimp only [execArm64Insn]
      split
      · exact jumpRel_inBounds_running prog.size ext false s imm19 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
      · simp [advancePc, hRun]
    | cbz is64 isNz rt imm19 =>
      rw [hInsn] at hI
      left
      dsimp only [execArm64Insn]
      cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
      · exact jumpRel_inBounds_running prog.size ext false s imm19 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
      · simp [advancePc, hRun]
      · exact jumpRel_inBounds_running prog.size ext false s imm19 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
      · simp [advancePc, hRun]
    | tbz isNz bit rt imm14 =>
      rw [hInsn] at hI
      left
      dsimp only [execArm64Insn]
      cases isNz <;> simp only [↓reduceIte, Bool.false_eq_true] <;> split
      · exact jumpRel_inBounds_running prog.size ext false s imm14 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
      · simp [advancePc, hRun]
      · exact jumpRel_inBounds_running prog.size ext false s imm14 hRun (by simpa [framelessLeafInsnOk, insnDecodedValid] using hI)
      · simp [advancePc, hRun]
    | nop => left; exact hRun
    | adrp _ _ | movWide _ _ _ _ _ | bitfield _ _ _ _ _ _ _
    | condSel _ _ _ _ _ _ | mul3 _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; simp only [advancePc, status_writeGpr, hRun]
    | div is64 _ _ _ _ =>
      cases is64
      · left; unfold execArm64Insn; dsimp only; simp only [Bool.false_eq_true, ↓reduceIte, advancePc, status_writeGpr, hRun]
      · left; unfold execArm64Insn; dsimp only; simp only [↓reduceIte, advancePc, status_writeGpr, hRun]
    | addSubImm _ _ _ _ _ _ | addSubExt _ _ _ _ _ _ _ _ | logicalImm _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; split <;> simp only [advancePc, status_writeGpr, status_writeGprSP, hRun]
    | addSubReg _ _ _ _ _ _ _ _ | logicalReg _ _ _ _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; split <;> simp only [advancePc, status_writeGpr, hRun]
    | ldrStrPair _ _ _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; split <;> split <;> try split
      all_goals simp only [advancePc, status_writeGpr, status_writeGprSP, writeFp, hRun]
    | ldrStrUImm _ _ _ _ _ _ | ldrStrReg _ _ _ _ _ _ _ _ _ | fpIntConv _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; split <;> try split <;> try split
      all_goals simp only [advancePc, status_writeGpr, writeFp, hRun]
    | ldrStrImm _ _ _ _ _ _ _ =>
      left; unfold execArm64Insn; dsimp only; split <;> try split <;> try split <;> try split
      all_goals simp only [advancePc, status_writeGpr, status_writeGprSP, writeFp, hRun]
    | fcmpD _ _ _ => left; exact hRun
    | fpUnaryD op _ _ =>
      unfold execArm64Insn; dsimp only
      split
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · right; right; rfl
    | fpBinaryD op _ _ _ =>
      unfold execArm64Insn; dsimp only
      split
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · left; exact hRun
      · right; right; rfl
  · right; right; simp [hLt]

/-- Soundness of `checkFramelessLeaf`: any frameless leaf routine executes with zero external calls, zero `morestack` traps, and zero `panicBounds` traps for all fuels and stack pointers. -/
theorem checkFramelessLeaf_run_sound (prog : Array Arm64Insn) (ext : Nat → Option String)
    (hOk : checkFramelessLeaf prog = true) :
    ∀ (fuel : Nat) (s : Arm64State),
      s.status = .running →
      (∀ tgt, (runArm64 fuel prog ext s).status = .callExternal tgt → isForbiddenAllocSym tgt = false) ∧
      (runArm64 fuel prog ext s).status ≠ .trapMorestack ∧
      (runArm64 fuel prog ext s).status ≠ .trapPanicBounds := by
  intro fuel
  induction fuel with
  | zero =>
    intro s hRun
    simp [runArm64, hRun]
  | succ k ih =>
    intro s hRun
    rw [runArm64_succ_running k prog ext s hRun]
    rcases checkFramelessLeaf_step prog ext hOk s hRun with hRun1 | hRet1 | hTrap1
    · exact ih (stepArm64 prog ext s) hRun1
    · have hStop : (stepArm64 prog ext s).status ≠ .running := by rw [hRet1]; decide
      rw [runArm64_of_not_running k prog ext _ hStop, hRet1]
      exact ⟨(fun _ h => by cases h), by decide, by decide⟩
    · have hStop : (stepArm64 prog ext s).status ≠ .running := by rw [hTrap1]; decide
      rw [runArm64_of_not_running k prog ext _ hStop, hTrap1]
      exact ⟨(fun _ h => by cases h), by decide, by decide⟩

/-! ### Chunked Safety Certificates & Theorems for Track 5D Routines -/

private def chunkDecodeCfg (chunks : List (List UInt32)) (idx : Nat) : Option Arm64Insn :=
  (chunkWord chunks 16 idx).map decodeArm64WordCfg

private theorem decodeCfgProg_size_eq (xs : List UInt32) :
    (decodeArm64ProgCfg ⟨xs⟩).size = xs.length := by
  unfold decodeArm64ProgCfg
  exact List.length_map decodeArm64WordCfg

private theorem decodeCfgChunks_size (chunks : List (List UInt32)) (sz : Nat)
    (h : chunksSumLen chunks = sz) :
    (decodeArm64ProgCfg ⟨chunks.flatten⟩).size = sz := by
  rw [decodeCfgProg_size_eq, chunksSumLen_eq_flatten_length, h]

private theorem decodeCfgProg_getElem?_chunk (chunks : List (List UInt32)) (idx : Nat)
    (hLen : ∀ i c, chunks[i]? = some c → (i + 1 < chunks.length → c.length = 16) ∧ c.length ≤ 16) :
    (decodeArm64ProgCfg ⟨chunks.flatten⟩)[idx]? = chunkDecodeCfg chunks idx := by
  have h1 : (decodeArm64ProgCfg ⟨chunks.flatten⟩)[idx]? = (chunks.flatten.map decodeArm64WordCfg)[idx]? := by
    simp [decodeArm64ProgCfg]
  rw [h1, List.getElem?_map, flatten_getElem?_chunk chunks 16 idx (by decide) hLen]
  unfold chunkDecodeCfg chunkWord
  cases chunks[idx / 16]? <;> rfl

private theorem checkZeroGC_cfg_of_chunks (chunks : List (List UInt32)) (ext : Nat → Option String) (sz : Nat)
    (hSz : chunksSumLen chunks = sz)
    (hChunks : everyWithIdxChunks 0 chunks (fun i w => zeroGcInsnOk sz ext i (decodeArm64WordCfg w)) = true) :
    checkZeroGC (decodeArm64ProgCfg ⟨chunks.flatten⟩) ext = .ok () := by
  have hProgSz : (decodeArm64ProgCfg ⟨chunks.flatten⟩).size = sz := decodeCfgChunks_size chunks sz hSz
  have hAll : everyWithIdx 0 (decodeArm64ProgCfg ⟨chunks.flatten⟩).toList (zeroGcInsnOk sz ext) = true := by
    have hToList : (decodeArm64ProgCfg ⟨chunks.flatten⟩).toList = chunks.flatten.map decodeArm64WordCfg := rfl
    rw [hToList, everyWithIdx_map, everyWithIdx_flatten]
    exact hChunks
  unfold checkZeroGC
  rw [hProgSz]
  simp [hAll]

private theorem verifyPb_cfg_of_chunks (chunks : List (List UInt32)) (ext : Nat → Option String)
    (inPb : Nat → Bool) (sz : Nat)
    (hSz : chunksSumLen chunks = sz)
    (hNot0 : (decide (0 < sz) && inPb 0) = false)
    (hChunks : everyWithIdxChunks 0 chunks (fun i w => panicBoundsInsnSafe sz (chunkDecodeCfg chunks) ext inPb i (decodeArm64WordCfg w)) = true) :
    verifyPanicBoundsCertWith (decodeArm64ProgCfg ⟨chunks.flatten⟩).size (chunkDecodeCfg chunks)
      (decodeArm64ProgCfg ⟨chunks.flatten⟩).toList ext inPb = .ok () := by
  have hProgSz : (decodeArm64ProgCfg ⟨chunks.flatten⟩).size = sz := decodeCfgChunks_size chunks sz hSz
  have hAll : everyWithIdx 0 (decodeArm64ProgCfg ⟨chunks.flatten⟩).toList
      (panicBoundsInsnSafe sz (chunkDecodeCfg chunks) ext inPb) = true := by
    change everyWithIdx 0 (chunks.flatten.map decodeArm64WordCfg) _ = true
    rw [everyWithIdx_map, everyWithIdx_flatten]
    exact hChunks
  unfold verifyPanicBoundsCertWith
  rw [hProgSz]
  simp only [gt_iff_lt, hNot0, Bool.false_eq_true, ↓reduceIte, hAll, Bool.not_true]

/-- Tier 5D safety theorem for frameless `assign.twoSumAssign`, `assign.clampFinite`, and shipping optimized `assign.exactReducedCost`. -/
theorem exactReducedCost_and_leaves_elf_safe (sTwo sClamp sExact : Arm64State)
    (hRunT : sTwo.status = .running)
    (hRunC : sClamp.status = .running)
    (hRunE : sExact.status = .running) :
    let pTwo := decodeArm64Prog twoSumAssignWords
    let pClamp := decodeArm64Prog clampFiniteWords
    let pExact := decodeArm64Prog exactReducedCostWords
    (∀ fuel tgt, (runArm64 fuel pTwo twoSumAssignTargets sTwo).status = .callExternal tgt → isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel pTwo twoSumAssignTargets sTwo).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel pTwo twoSumAssignTargets sTwo).status ≠ .trapPanicBounds) ∧
    (∀ fuel tgt, (runArm64 fuel pClamp clampFiniteTargets sClamp).status = .callExternal tgt → isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel pClamp clampFiniteTargets sClamp).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel pClamp clampFiniteTargets sClamp).status ≠ .trapPanicBounds) ∧
    (∀ fuel tgt, (runArm64 fuel pExact exactReducedCostTargets sExact).status = .callExternal tgt → isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel pExact exactReducedCostTargets sExact).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel pExact exactReducedCostTargets sExact).status ≠ .trapPanicBounds) := by
  have hOkT : checkFramelessLeaf (decodeArm64Prog twoSumAssignWords) = true := by decide
  have hOkC : checkFramelessLeaf (decodeArm64Prog clampFiniteWords) = true := by decide
  have hOkE : checkFramelessLeaf (decodeArm64Prog exactReducedCostWords) = true := by decide
  refine ⟨fun fuel => (checkFramelessLeaf_run_sound _ _ hOkT fuel sTwo hRunT).1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkT fuel sTwo hRunT).2.1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkT fuel sTwo hRunT).2.2,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkC fuel sClamp hRunC).1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkC fuel sClamp hRunC).2.1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkC fuel sClamp hRunC).2.2,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkE fuel sExact hRunE).1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkE fuel sExact hRunE).2.1,
          fun fuel => (checkFramelessLeaf_run_sound _ _ hOkE fuel sExact hRunE).2.2⟩

private instance {ε α : Type} [DecidableEq ε] [DecidableEq α] : DecidableEq (Except ε α)
  | .ok a, .ok b => if h : a = b then isTrue (h ▸ rfl) else isFalse (by intro hEq; cases hEq; exact h rfl)
  | .error e1, .error e2 => if h : e1 = e2 then isTrue (h ▸ rfl) else isFalse (by intro hEq; cases hEq; exact h rfl)
  | .ok _, .error _ => isFalse (by intro h; cases h)
  | .error _, .ok _ => isFalse (by intro h; cases h)

/-- Direct `Track5B` safety theorem (`checkZeroGC_run_sound`, `checkStackGuard_sound`, `checkPanicBoundsReachability_run_unconditional`)
on `-gcflags=-l` `assign.exactReducedCost`, which has zero `runtime.panicBounds` blocks (`inPb = fun _ => false`). -/
theorem exactReducedCostNoinline_elf_safe (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hSp : BitVec.ult 31#64 s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - 32#64) = true)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0) :
    let prog := decodeArm64Prog exactReducedCostNoinlineWords
    (∀ fuel tgt, (runArm64 fuel prog exactReducedCostNoinlineTargets s).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel prog exactReducedCostNoinlineTargets s).status ≠ .trapMorestack) ∧
    (∀ fuel, (runArm64 fuel prog exactReducedCostNoinlineTargets s).status ≠ .trapPanicBounds) := by
  have hEqProg : decodeArm64Prog exactReducedCostNoinlineWords = decodeArm64ProgCfg exactReducedCostNoinlineWords := by decide
  rw [hEqProg]
  have hLen : ∀ i c, exactReducedCostNoinlineChunks[i]? = some c →
      (i + 1 < exactReducedCostNoinlineChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
    chunks16_len_of_all exactReducedCostNoinlineChunks (by decide)
  have hGet : ∀ i, chunkDecodeCfg exactReducedCostNoinlineChunks i =
      (decodeArm64ProgCfg exactReducedCostNoinlineWords)[i]? :=
    fun i => (decodeCfgProg_getElem?_chunk exactReducedCostNoinlineChunks i hLen).symm
  have hGetFun : (fun i => (decodeArm64ProgCfg exactReducedCostNoinlineWords)[i]?) =
      chunkDecodeCfg exactReducedCostNoinlineChunks := (funext hGet).symm
  have hSz : (decodeArm64ProgCfg exactReducedCostNoinlineWords).size = 63 :=
    decodeCfgChunks_size exactReducedCostNoinlineChunks 63 (by decide)
  have hZeroGC : checkZeroGC (decodeArm64ProgCfg exactReducedCostNoinlineWords) exactReducedCostNoinlineTargets = .ok () := by
    apply checkZeroGC_cfg_of_chunks exactReducedCostNoinlineChunks exactReducedCostNoinlineTargets 63 (by decide)
    decide
  have hStack : checkStackGuard (decodeArm64ProgCfg exactReducedCostNoinlineWords) exactReducedCostNoinlineTargets =
      .ok (.smallFrame 56 56) := by
    unfold checkStackGuard
    rw [hSz, hGetFun]
    change checkStackGuardWith 63 (chunkDecodeCfg exactReducedCostNoinlineChunks)
      (exactReducedCostNoinlineChunks.flatten.map decodeArm64WordCfg) exactReducedCostNoinlineTargets =
      .ok (.smallFrame 56 56)
    decide
  have hPb : verifyPanicBoundsCertWith (decodeArm64ProgCfg exactReducedCostNoinlineWords).size
      (chunkDecodeCfg exactReducedCostNoinlineChunks)
      (decodeArm64ProgCfg exactReducedCostNoinlineWords).toList
      exactReducedCostNoinlineTargets (fun _ => false) = .ok () := by
    apply verifyPb_cfg_of_chunks exactReducedCostNoinlineChunks exactReducedCostNoinlineTargets (fun _ => false) 63 (by decide) (by decide)
    decide
  refine ⟨fun fuel tgt => checkZeroGC_run_sound _ _ hZeroGC fuel s tgt hRun,
          checkStackGuard_sound _ _ (.smallFrame 56 56) hStack s hRun hPc hG hSp hGuard,
          fun fuel => checkPanicBoundsReachability_run_unconditional _ (chunkDecodeCfg exactReducedCostNoinlineChunks) _ hPb fuel s hRun⟩

/-- Contiguous O(1) cold `runtime.panicBounds` region predicate for the 1353-instruction `hasPotentialCancellation` ELF routine. -/
def isCancelPbIdx (i : Nat) : Bool :=
  i == 641 ||
  (899 <= i && i <= 912) ||
  (942 <= i && i <= 948) ||
  (967 <= i && i <= 988) ||
  (1044 <= i && i <= 1054) ||
  i == 1083 ||
  (1266 <= i && i <= 1316) ||
  (1324 <= i && i <= 1330)

private theorem isCancelPbIdx_ge_1353 (j : Nat) (hj : 1353 ≤ j) : isCancelPbIdx j = false := by
  unfold isCancelPbIdx
  simp only [Bool.or_eq_false_iff, Bool.and_eq_false_iff, beq_eq_false_iff_ne, decide_eq_false_iff_not]
  omega

private theorem cancelChunks_len :
    ∀ i c, hasPotentialCancellationChunks[i]? = some c →
      (i + 1 < hasPotentialCancellationChunks.length → c.length = 16) ∧ c.length ≤ 16 :=
  chunks16_len_of_all hasPotentialCancellationChunks (by decide)

private theorem chunkDecodeCfg_cancel (i : Nat) :
    chunkDecodeCfg hasPotentialCancellationChunks i =
      (decodeArm64ProgCfg hasPotentialCancellationWords)[i]? :=
  (decodeCfgProg_getElem?_chunk hasPotentialCancellationChunks i cancelChunks_len).symm

theorem hasPotentialCancellation_zeroGC :
    checkZeroGC (decodeArm64ProgCfg hasPotentialCancellationWords) hasPotentialCancellationTargets = .ok () := by
  apply checkZeroGC_cfg_of_chunks hasPotentialCancellationChunks hasPotentialCancellationTargets 1353 (by decide)
  decide

theorem hasPotentialCancellation_stackGuard :
    checkStackGuard (decodeArm64ProgCfg hasPotentialCancellationWords) hasPotentialCancellationTargets =
      .ok (.largeFrame 704 1332 1331) := by
  have hSz : (decodeArm64ProgCfg hasPotentialCancellationWords).size = 1353 :=
    decodeCfgChunks_size hasPotentialCancellationChunks 1353 (by decide)
  have hGetFun : (fun i => (decodeArm64ProgCfg hasPotentialCancellationWords)[i]?) =
      chunkDecodeCfg hasPotentialCancellationChunks := (funext chunkDecodeCfg_cancel).symm
  have hAll : (!everyWithIdx 0 (decodeArm64ProgCfg hasPotentialCancellationWords).toList
      (bodyInsnStackSafe 1353 3 1331 (704 + 128) (chunkDecodeCfg hasPotentialCancellationChunks) hasPotentialCancellationTargets)) = false := by
    have hT : everyWithIdx 0 (decodeArm64ProgCfg hasPotentialCancellationWords).toList
        (bodyInsnStackSafe 1353 3 1331 (704 + 128) (chunkDecodeCfg hasPotentialCancellationChunks) hasPotentialCancellationTargets) = true := by
      change everyWithIdx 0 (hasPotentialCancellationChunks.flatten.map decodeArm64WordCfg) _ = true
      rw [everyWithIdx_map, everyWithIdx_flatten]
      decide
    rw [hT]; rfl
  unfold checkStackGuard
  rw [hSz, hGetFun]
  unfold checkStackGuardWith
  have hc0 : (chunkDecodeCfg hasPotentialCancellationChunks 0 == some (.ldrStrUImm true false 8 16 16#5 28#5)) = true := by decide
  have hc1f : chunkDecodeCfg hasPotentialCancellationChunks 1 = some (.addSubImm true true false 17#5 31#5 704#64) := by decide
  have hc2 : (chunkDecodeCfg hasPotentialCancellationChunks 2 == some (.addSubReg true true true .lsl 0 31#5 17#5 16#5)) = true := by decide
  have hc3 : chunkDecodeCfg hasPotentialCancellationChunks 3 = some (.bCond .ls 1329) := by decide
  simp only [hc0, hc1f, hc2, hc3, Bool.not_true, Bool.false_eq_true, ↓reduceIte]
  split
  · rename_i h; exact absurd h (by decide)
  · split
    · rename_i h; exact absurd h (by decide)
    · split
      · rename_i h; exact absurd h (by decide)
      · split
        · rename_i h; exact absurd h (by decide)
        · split
          · rename_i h; exact absurd h (by decide)
          · split
            · rename_i h; exact absurd h (by decide)
            · split
              · rename_i h; exact Bool.noConfusion (h.symm.trans hAll)
              · rfl

theorem hasPotentialCancellation_pbCert :
    verifyPanicBoundsCertWith (decodeArm64ProgCfg hasPotentialCancellationWords).size
      (chunkDecodeCfg hasPotentialCancellationChunks)
      (decodeArm64ProgCfg hasPotentialCancellationWords).toList
      hasPotentialCancellationTargets isCancelPbIdx = .ok () := by
  apply verifyPb_cfg_of_chunks hasPotentialCancellationChunks hasPotentialCancellationTargets isCancelPbIdx 1353 (by decide) (by decide)
  decide

private theorem cancel_stack_headroom_832 (sp sg : BitVec 64)
    (hSp : BitVec.ult 831#64 sp = true)
    (hGuard : BitVec.ult sg (sp - 832#64) = true) :
    BitVec.ult 703#64 sp = true ∧ BitVec.ult sg (sp - 704#64) = true := by
  simp only [BitVec.ult, decide_eq_true_eq] at hSp hGuard ⊢
  constructor <;> bv_omega

/-- Tier 5D binary theorem for the 1353-instruction zero-allocation Kahn DAG cycle checker `assign.hasPotentialCancellation`: unconditional Zero-GC and stack-headroom safety (`checkZeroGC_run_sound`, `checkStackGuard_sound`) plus structurally CFG-guarded `panicBounds` mediation (`BoundsGuardsHold` via `checkPanicBoundsReachability_run_safe`). -/
theorem hasPotentialCancellation_elf_safe (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0)
    (hSp : BitVec.ult 831#64 s.sp = true)
    (hGuard : BitVec.ult s.gStackGuard0 (s.sp - 832#64) = true)
    (hG : readU64 s.mem (s.regs 28#5 + 16#64) = s.gStackGuard0) :
    let prog := decodeArm64ProgCfg hasPotentialCancellationWords
    (∀ fuel tgt, (runArm64 fuel prog hasPotentialCancellationTargets s).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, (runArm64 fuel prog hasPotentialCancellationTargets s).status ≠ .trapMorestack) ∧
    (∀ fuel, BoundsGuardsHold prog (fun i => prog[i]?) hasPotentialCancellationTargets isCancelPbIdx fuel s →
      (runArm64 fuel prog hasPotentialCancellationTargets s).status ≠ .trapPanicBounds) := by
  have hSz : (decodeArm64ProgCfg hasPotentialCancellationWords).size = 1353 :=
    decodeCfgChunks_size hasPotentialCancellationChunks 1353 (by decide)
  have hHead := cancel_stack_headroom_832 s.sp s.gStackGuard0 hSp hGuard
  have hBd : ∀ j, (decodeArm64ProgCfg hasPotentialCancellationWords).size ≤ j → isCancelPbIdx j = false := by
    intro j hj; rw [hSz] at hj; exact isCancelPbIdx_ge_1353 j hj
  have hGetFun : (fun i => (decodeArm64ProgCfg hasPotentialCancellationWords)[i]?) =
      chunkDecodeCfg hasPotentialCancellationChunks := (funext chunkDecodeCfg_cancel).symm
  refine ⟨fun fuel tgt => checkZeroGC_run_sound _ _ hasPotentialCancellation_zeroGC fuel s tgt hRun,
          checkStackGuard_sound _ _ (.largeFrame 704 1332 1331) hasPotentialCancellation_stackGuard
            s hRun hPc hG hHead.1 hHead.2,
          fun fuel hHold => checkPanicBoundsReachability_run_safe _ (chunkDecodeCfg hasPotentialCancellationChunks) _ _
            hBd hasPotentialCancellation_pbCert fuel s hRun (by rw [hPc]; decide) (hGetFun ▸ hHold)⟩

/-- Bounds-guard trace invariant over the exact extended `Arm64InsnD` interpreter (`FMADDD` + `FMOVD $imm8`). -/
def BoundsGuardsHoldD (words : Array UInt32) (getInsn : Nat → Option Arm64Insn)
    (ext : Nat → Option String) (inPb : Nat → Bool) :
    Nat → Arm64State → Prop
  | 0, _ => True
  | k + 1, s =>
    let s1 := stepArm64D (decodeArm64ProgD words) ext s
    (s.status = .running → s1.status = .running →
      isVerifiedBoundsGuard getInsn s.pcIdx s1.pcIdx = true → inPb s1.pcIdx = false) ∧
      BoundsGuardsHoldD words getInsn ext inPb k s1

/-- All-fuel unconditional Zero-GC allocation safety and structurally CFG-guarded `panicBounds` mediation (`BoundsGuardsHoldD`) lifted directly to the exact `runArm64D` interpreter on `hasPotentialCancellation`. -/
theorem hasPotentialCancellation_exactD_zeroGC_and_pb (s : Arm64State)
    (hRun : s.status = .running) (hPc : s.pcIdx = 0) :
    let progD := decodeArm64ProgD hasPotentialCancellationWords
    let progCfg := decodeArm64ProgCfg hasPotentialCancellationWords
    (∀ fuel tgt, (runArm64D fuel progD hasPotentialCancellationTargets s).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false) ∧
    (∀ fuel, BoundsGuardsHoldD hasPotentialCancellationWords (fun i => progCfg[i]?)
      hasPotentialCancellationTargets isCancelPbIdx fuel s →
      (runArm64D fuel progD hasPotentialCancellationTargets s).status ≠ .trapPanicBounds) := by
  have hSz : (decodeArm64ProgCfg hasPotentialCancellationWords).size = 1353 :=
    decodeCfgChunks_size hasPotentialCancellationChunks 1353 (by decide)
  have hBd : ∀ j, (decodeArm64ProgCfg hasPotentialCancellationWords).size ≤ j → isCancelPbIdx j = false := by
    intro j hj; rw [hSz] at hj; exact isCancelPbIdx_ge_1353 j hj
  have hGetFun : (fun i => (decodeArm64ProgCfg hasPotentialCancellationWords)[i]?) =
      chunkDecodeCfg hasPotentialCancellationChunks := (funext chunkDecodeCfg_cancel).symm
  have hZeroD : ∀ (fuel : Nat) (st : Arm64State) (tgt : String),
      st.status = .running →
      (runArm64D fuel (decodeArm64ProgD hasPotentialCancellationWords) hasPotentialCancellationTargets st).status = .callExternal tgt →
      isForbiddenAllocSym tgt = false := by
    intro fuel
    induction fuel with
    | zero => intro st tgt hR hC; simp [runArm64D, hR] at hC
    | succ k ih =>
      intro st tgt hR hC
      rw [runArm64D_succ_running k _ _ st hR] at hC
      let st1 := stepArm64D (decodeArm64ProgD hasPotentialCancellationWords) hasPotentialCancellationTargets st
      have hSim := stepArm64D_proxy_status_pc hasPotentialCancellationWords hasPotentialCancellationTargets st
      by_cases hR1 : st1.status = .running
      · exact ih st1 tgt hR1 hC
      · rw [runArm64D_of_not_running k _ _ st1 hR1] at hC
        have hCP : (stepArm64 (decodeArm64ProgCfg hasPotentialCancellationWords) hasPotentialCancellationTargets st).status = .callExternal tgt :=
          hSim.1.symm.trans hC
        exact checkZeroGC_sound _ _ hasPotentialCancellation_zeroGC st tgt hR hCP
  have hPbD : ∀ (fuel : Nat) (st : Arm64State),
      st.status = .running → isCancelPbIdx st.pcIdx = false →
      BoundsGuardsHoldD hasPotentialCancellationWords (chunkDecodeCfg hasPotentialCancellationChunks)
        hasPotentialCancellationTargets isCancelPbIdx fuel st →
      (runArm64D fuel (decodeArm64ProgD hasPotentialCancellationWords) hasPotentialCancellationTargets st).status ≠ .trapPanicBounds := by
    intro fuel
    induction fuel with
    | zero => intro st hR _ _; simp [runArm64D, hR]
    | succ k ih =>
      intro st hR hSafe hHold
      rw [runArm64D_succ_running k _ _ st hR]
      let st1 := stepArm64D (decodeArm64ProgD hasPotentialCancellationWords) hasPotentialCancellationTargets st
      have hSim := stepArm64D_proxy_status_pc hasPotentialCancellationWords hasPotentialCancellationTargets st
      have hStepP := checkPanicBoundsReachability_step_guard _ (chunkDecodeCfg hasPotentialCancellationChunks)
        hasPotentialCancellationTargets isCancelPbIdx hBd hasPotentialCancellation_pbCert st hR hSafe
      by_cases hR1 : st1.status = .running
      · have hRP1 : (stepArm64 (decodeArm64ProgCfg hasPotentialCancellationWords) hasPotentialCancellationTargets st).status = .running :=
          hSim.1.symm.trans hR1
        have hSafe1 : isCancelPbIdx st1.pcIdx = false := by
          rcases hStepP.2 hRP1 with hNotPb | hGuard
          · rwa [hSim.2]
          · rw [← hSim.2] at hGuard
            exact hHold.1 hR hR1 hGuard
        exact ih st1 hR1 hSafe1 hHold.2
      · rw [runArm64D_of_not_running k _ _ st1 hR1]
        intro hTrap
        exact hStepP.1 (hSim.1.symm.trans hTrap)
  exact ⟨fun fuel tgt => hZeroD fuel s tgt hRun,
         fun fuel hHold => hPbD fuel s hRun (by rw [hPc]; decide) (hGetFun ▸ hHold)⟩


/-!
### Part II: Functional IEEE-754 SIMD Lane & `Min2` Reduction Equivalence
-/

/-- Canonical IEEE-754 binary64 bit patterns (`+0.0`, `-0.0`, `+Inf`, `-Inf`, subnormals, `minPosNormal`). -/
def f64PosZero : BitVec 64 := 0x0000000000000000#64
def f64NegZero : BitVec 64 := 0x8000000000000000#64
def f64PosInf : BitVec 64 := 0x7ff0000000000000#64
def f64NegInf : BitVec 64 := 0xfff0000000000000#64
def f64MinPosSubnormal : BitVec 64 := 0x0000000000000001#64
def f64MaxPosSubnormal : BitVec 64 := 0x000fffffffffffff#64
def f64MinNegSubnormal : BitVec 64 := 0x8000000000000001#64
def f64MaxNegSubnormal : BitVec 64 := 0x800fffffffffffff#64
def f64MinPosNormal : BitVec 64 := 0x0010000000000000#64

/-- Non-NaN predicate over `BitVec 64` (`VCMPPD_ORD_Q` lane check). -/
def f64NotNaN (x : BitVec 64) : Bool := !f64IsNaN x

/-- IEEE-754 `totalOrder` sign-magnitude bijection mapping 64-bit float words into unsigned `BitVec 64` order (`-0.0 < +0.0`). -/
def f64TotalKey (x : BitVec 64) : BitVec 64 :=
  if x.msb then ~~~x else (x ^^^ 0x8000000000000000#64)

/-- Canonical signed-zero normalization (`if x == 0 { x = +0.0 }`, matching `internal/simd/kernels_amd64.go:713,783`). -/
def canonZero64 (x : BitVec 64) : BitVec 64 :=
  if f64IsZero x then f64PosZero else x

/-- IEEE-754 numeric float64 order key on non-NaN inputs (identifies `-0.0` and `+0.0`). -/
def f64NumKey (x : BitVec 64) : BitVec 64 :=
  f64TotalKey (canonZero64 x)

/-- Hardware IEEE-754 binary64 strict `<` lane comparison (`VCMPPD_LT_OQ` / scalar `<`) on `BitVec 64`. -/
def f64Lt (a b : BitVec 64) : Bool :=
  if f64IsNaN a || f64IsNaN b then false
  else if f64IsZero a && f64IsZero b then false
  else if a.msb != b.msb then a.msb
  else if a.msb then
    BitVec.ult (b &&& 0x7fffffffffffffff#64) (a &&& 0x7fffffffffffffff#64)
  else
    BitVec.ult (a &&& 0x7fffffffffffffff#64) (b &&& 0x7fffffffffffffff#64)

/-- Hardware IEEE-754 binary64 numeric equality (`VCMPPD_EQ_OQ` / scalar `==`) on `BitVec 64`. -/
def f64Eq (a b : BitVec 64) : Bool :=
  if f64IsNaN a || f64IsNaN b then false
  else (a == b) || (f64IsZero a && f64IsZero b)

/-- Hardware IEEE-754 binary64 `<=` (`VCMPPD_LE_OQ` / scalar `<=`) on `BitVec 64`. -/
def f64Le (a b : BitVec 64) : Bool :=
  f64Lt a b || f64Eq a b

/-- Hardware IEEE-754 binary64 `>=` (`VCMPPD_GE_OQ` / `archsimd.Float64x4.GreaterEqual`) on `BitVec 64`. -/
def f64Ge (a b : BitVec 64) : Bool :=
  f64Le b a

/-- x86 `VMINPD` lane semantics (`(a < b) ? a : b`; returns second operand `b` on `-0.0` vs `+0.0` ties). -/
def vminpdLane (a b : BitVec 64) : BitVec 64 :=
  if f64Lt a b then a else b

private theorem bv64_and_abs_toNat (x : BitVec 64) :
    (x &&& 0x7fffffffffffffff#64).toNat = x.toNat % 2 ^ 63 := by
  rw [BitVec.toNat_and]
  exact Nat.and_two_pow_sub_one_eq_mod x.toNat 63

private theorem bv64_and_mant_toNat (x : BitVec 64) :
    (x &&& 0x000fffffffffffff#64).toNat = x.toNat % 2 ^ 52 := by
  rw [BitVec.toNat_and]
  exact Nat.and_two_pow_sub_one_eq_mod x.toNat 52

private theorem bv64_and_exp_toNat (x : BitVec 64) :
    (x &&& 0x7ff0000000000000#64).toNat = (x.toNat % 2 ^ 63 / 2 ^ 52) * 2 ^ 52 := by
  have hEq : x &&& 0x7ff0000000000000#64 = ((x &&& 0x7fffffffffffffff#64) >>> 52) <<< 52 := by
    apply BitVec.eq_of_getLsbD_eq
    intro i hi
    simp only [BitVec.getLsbD_and, BitVec.getLsbD_shiftLeft, BitVec.getLsbD_ushiftRight, hi,
      decide_true, Bool.true_and]
    by_cases h52 : i < 52
    · have hM : (0x7ff0000000000000#64 : BitVec 64).getLsbD i = false := by
        change Nat.testBit (0x7ff * 2 ^ 52) i = false
        rw [Nat.testBit_eq_decide_div_mod_eq]
        have hP : 2 ^ 52 = 2 ^ i * 2 ^ (52 - i) := by rw [← Nat.pow_add]; congr 1; omega
        have hDiv : (0x7ff * 2 ^ 52) / 2 ^ i = 0x7ff * 2 ^ (52 - i) := by
          rw [hP, ← Nat.mul_assoc, Nat.mul_right_comm, Nat.mul_div_cancel _ (Nat.two_pow_pos i)]
        rw [hDiv]
        have hSub : 52 - i = (51 - i) + 1 := by omega
        rw [hSub, Nat.pow_succ]
        exact decide_eq_false (by omega)
      simp [h52, hM]
    · by_cases h63 : i < 63
      · have hM : (0x7ff0000000000000#64 : BitVec 64).getLsbD i = true := by
          change Nat.testBit (0x7ff * 2 ^ 52) i = true
          rw [Nat.testBit_eq_decide_div_mod_eq]
          have hP : 2 ^ i = 2 ^ 52 * 2 ^ (i - 52) := by rw [← Nat.pow_add]; congr 1; omega
          have hDiv : (0x7ff * 2 ^ 52) / 2 ^ i = 0x7ff / 2 ^ (i - 52) := by
            rw [hP, Nat.mul_comm 0x7ff (2 ^ 52), Nat.mul_div_mul_left _ _ (Nat.two_pow_pos 52)]
          rw [hDiv]
          have hRange : i - 52 < 11 := by omega
          have hTb : Nat.testBit 0x7ff (i - 52) = true := by
            have hk : 0x7ff = 2 ^ 11 - 1 := by decide
            rw [hk, Nat.testBit_two_pow_sub_one]; exact decide_eq_true hRange
          rwa [Nat.testBit_eq_decide_div_mod_eq] at hTb
        have hA : (0x7fffffffffffffff#64 : BitVec 64).getLsbD i = true := by
          change Nat.testBit (2 ^ 63 - 1) i = true
          rw [Nat.testBit_two_pow_sub_one]; exact decide_eq_true h63
        have hAdd : 52 + (i - 52) = i := by omega
        simp [h52, hM, hAdd, hA]
      · have hi63 : i = 63 := by omega
        subst hi63
        simp
  rw [hEq, BitVec.toNat_shiftLeft, BitVec.toNat_ushiftRight, BitVec.toNat_and]
  have hMod := Nat.and_two_pow_sub_one_eq_mod x.toNat 63
  change (((x.toNat &&& (2 ^ 63 - 1)) >>> 52) <<< 52) % 2 ^ 64 = (x.toNat % 2 ^ 63 / 2 ^ 52) * 2 ^ 52
  rw [hMod, Nat.shiftRight_eq_div_pow, Nat.shiftLeft_eq]
  have : x.toNat % 2 ^ 63 < 2 ^ 63 := Nat.mod_lt _ (by decide)
  exact Nat.mod_eq_of_lt (by omega)

private theorem bv64_msb_eq (x : BitVec 64) :
    x.msb = decide (2 ^ 63 ≤ x.toNat) := by
  have hlt := x.isLt
  rw [BitVec.msb_eq_getLsbD_last]
  change Nat.testBit x.toNat 63 = decide (2 ^ 63 ≤ x.toNat)
  rw [Nat.testBit_eq_decide_div_mod_eq]
  by_cases h : 2 ^ 63 ≤ x.toNat
  · have hDiv : x.toNat / 2 ^ 63 = 1 := by omega
    simp [h, hDiv]
  · have hDiv : x.toNat / 2 ^ 63 = 0 := by omega
    simp [h, hDiv]

private theorem bv64_xor_sign_toNat (x : BitVec 64) (hx : x.toNat < 2 ^ 63) :
    (x ^^^ 0x8000000000000000#64).toNat = x.toNat + 2 ^ 63 := by
  rw [BitVec.toNat_xor]
  change x.toNat ^^^ 2 ^ 63 = x.toNat + 2 ^ 63
  apply Nat.eq_of_testBit_eq
  intro i
  rw [Nat.testBit_xor, Nat.testBit_two_pow]
  by_cases hlt : i < 63
  · have hne : decide (63 = i) = false := decide_eq_false (by omega)
    rw [hne, Bool.xor_false]
    rw [Nat.testBit_eq_decide_div_mod_eq, Nat.testBit_eq_decide_div_mod_eq]
    have hPow : 2 ^ 63 = 2 ^ i * 2 ^ (63 - i) := by rw [← Nat.pow_add]; congr 1; omega
    rw [hPow, Nat.add_mul_div_left _ _ (Nat.two_pow_pos i)]
    have hSub : 63 - i = (62 - i) + 1 := by omega
    rw [hSub, Nat.pow_succ, Nat.add_mul_mod_self_right]
  · by_cases heq : i = 63
    · subst heq
      have h0 : Nat.testBit x.toNat 63 = false := Nat.testBit_lt_two_pow hx
      rw [h0, Nat.testBit_eq_decide_div_mod_eq, show (x.toNat + 2 ^ 63) / 2 ^ 63 = 1 by omega]
      rfl
    · have hne : decide (63 = i) = false := decide_eq_false (by omega)
      rw [hne, Bool.xor_false]
      have hPow : 2 ^ 64 ≤ 2 ^ i := Nat.pow_le_pow_right (by decide) (by omega)
      rw [Nat.testBit_lt_two_pow (by omega), Nat.testBit_lt_two_pow (by omega)]

private theorem f64TotalKey_toNat (x : BitVec 64) :
    (f64TotalKey x).toNat =
      if 2 ^ 63 ≤ x.toNat then 2 ^ 64 - 1 - x.toNat else x.toNat + 2 ^ 63 := by
  unfold f64TotalKey
  rw [bv64_msb_eq]
  by_cases h : 2 ^ 63 ≤ x.toNat
  · simp only [h, decide_true, ↓reduceIte, BitVec.toNat_not]
  · simp only [h, decide_false, Bool.false_eq_true, ↓reduceIte]
    exact bv64_xor_sign_toNat x (by omega)

private theorem f64IsZero_eq_decide (x : BitVec 64) :
    f64IsZero x = decide (x.toNat = 0 ∨ x.toNat = 2 ^ 63) := by
  have hlt := x.isLt
  unfold f64IsZero
  by_cases h : x.toNat = 0 ∨ x.toNat = 2 ^ 63
  · rw [decide_eq_true h]
    apply beq_iff_eq.mpr
    apply BitVec.eq_of_toNat_eq
    rw [bv64_and_abs_toNat]
    change x.toNat % 2 ^ 63 = 0
    omega
  · rw [decide_eq_false h]
    apply beq_false_of_ne
    intro hEq
    have hN := congrArg BitVec.toNat hEq
    rw [bv64_and_abs_toNat] at hN
    change x.toNat % 2 ^ 63 = 0 at hN
    omega

private theorem f64NumKey_toNat (x : BitVec 64) :
    (f64NumKey x).toNat =
      if x.toNat = 0 ∨ x.toNat = 2 ^ 63 then 2 ^ 63
      else if 2 ^ 63 ≤ x.toNat then 2 ^ 64 - 1 - x.toNat
      else x.toNat + 2 ^ 63 := by
  unfold f64NumKey canonZero64 f64PosZero
  rw [f64IsZero_eq_decide]
  by_cases hz : x.toNat = 0 ∨ x.toNat = 2 ^ 63
  · simp only [hz, decide_true, ↓reduceIte]; rfl
  · simp only [hz, decide_false, Bool.false_eq_true, ↓reduceIte, f64TotalKey_toNat]

theorem f64NotNaN_iff_abs_ule_posInf (x : BitVec 64) :
    f64NotNaN x = BitVec.ule (x &&& 0x7fffffffffffffff#64) f64PosInf := by
  unfold f64NotNaN f64IsNaN f64PosInf
  rw [BitVec.ule_eq_decide, bv64_and_abs_toNat]
  change (!((x &&& 0x7ff0000000000000#64 == 0x7ff0000000000000#64) &&
            (x &&& 0x000fffffffffffff#64 != 0#64))) = decide (x.toNat % 2 ^ 63 ≤ 0x7ff0000000000000)
  have hExp : (x &&& 0x7ff0000000000000#64 == 0x7ff0000000000000#64) =
      decide (x.toNat % 2 ^ 63 / 2 ^ 52 = 0x7ff) := by
    by_cases h : x.toNat % 2 ^ 63 / 2 ^ 52 = 0x7ff
    · rw [decide_eq_true h]; apply beq_iff_eq.mpr; apply BitVec.eq_of_toNat_eq
      rw [bv64_and_exp_toNat, h]; rfl
    · rw [decide_eq_false h]; apply beq_false_of_ne; intro hEq
      have hN := congrArg BitVec.toNat hEq; rw [bv64_and_exp_toNat] at hN; change _ = 0x7ff0000000000000 at hN; omega
  have hMant : (x &&& 0x000fffffffffffff#64 != 0#64) = decide (x.toNat % 2 ^ 52 ≠ 0) := by
    by_cases h : x.toNat % 2 ^ 52 = 0
    · have hEq : x &&& 0x000fffffffffffff#64 = 0#64 := BitVec.eq_of_toNat_eq (by rw [bv64_and_mant_toNat, h]; rfl)
      simp [h, hEq]
    · have hNe : (x &&& 0x000fffffffffffff#64 == 0#64) = false := beq_false_of_ne fun hEq => by
        have hN := congrArg BitVec.toNat hEq; rw [bv64_and_mant_toNat] at hN; change x.toNat % 2 ^ 52 = 0 at hN; exact h hN
      simp [bne, hNe, h]
  rw [hExp, hMant]
  have hMod : x.toNat % 2 ^ 63 % 2 ^ 52 = x.toNat % 2 ^ 52 := by
    have hDvd : 2 ^ 52 ∣ 2 ^ 63 := Nat.pow_dvd_pow 2 (by decide)
    exact Nat.mod_mod_of_dvd x.toNat hDvd
  have hLt63 : x.toNat % 2 ^ 63 < 2 ^ 63 := Nat.mod_lt _ (by decide)
  by_cases h1 : x.toNat % 2 ^ 63 / 2 ^ 52 = 0x7ff <;> by_cases h2 : x.toNat % 2 ^ 52 = 0 <;> simp [h1, h2] <;> omega

theorem f64TotalKey_inj (a b : BitVec 64) :
    (f64TotalKey a == f64TotalKey b) = (a == b) := by
  have ha := a.isLt; have hb := b.isLt
  by_cases hab : a = b
  · subst hab; simp
  · rw [beq_false_of_ne hab]
    apply beq_false_of_ne
    intro hEq
    have hN := congrArg BitVec.toNat hEq
    rw [f64TotalKey_toNat, f64TotalKey_toNat] at hN
    apply hab; apply BitVec.eq_of_toNat_eq
    split at hN <;> split at hN <;> omega

theorem f64NumKey_eq_iff (a b : BitVec 64) :
    (f64NumKey a == f64NumKey b) = ((a == b) || (f64IsZero a && f64IsZero b)) := by
  unfold f64NumKey
  rw [f64TotalKey_inj]
  unfold canonZero64 f64PosZero
  rw [f64IsZero_eq_decide a, f64IsZero_eq_decide b]
  have ha := a.isLt; have hb := b.isLt
  by_cases hza : a.toNat = 0 ∨ a.toNat = 2 ^ 63 <;> by_cases hzb : b.toNat = 0 ∨ b.toNat = 2 ^ 63
  · simp [hza, hzb]
  · simp only [hza, hzb, decide_true, decide_false, ↓reduceIte, Bool.false_eq_true, Bool.and_false, Bool.or_false]
    have h1 : (0#64 == b) = false := beq_false_of_ne (fun h => hzb (Or.inl (by simpa using (congrArg BitVec.toNat h).symm)))
    have h2 : (a == b) = false := beq_false_of_ne (fun h => hzb (h ▸ hza))
    rw [h1, h2]
  · simp only [hza, hzb, decide_true, decide_false, ↓reduceIte, Bool.false_eq_true, Bool.false_and, Bool.or_false]
    have h1 : (a == 0#64) = false := beq_false_of_ne (fun h => hza (Or.inl (by simpa using congrArg BitVec.toNat h)))
    have h2 : (a == b) = false := beq_false_of_ne (fun h => hza (h ▸ hzb))
    rw [h1, h2]
  · simp [hza, hzb]

theorem f64Lt_iff_ult_numKey (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64Lt a b = BitVec.ult (f64NumKey a) (f64NumKey b) := by
  have hNa : f64IsNaN a = false := by unfold f64NotNaN at ha; simpa using ha
  have hNb : f64IsNaN b = false := by unfold f64NotNaN at hb; simpa using hb
  unfold f64Lt
  simp only [hNa, hNb, Bool.or_self, Bool.false_eq_true, ↓reduceIte]
  rw [f64IsZero_eq_decide a, f64IsZero_eq_decide b, bv64_msb_eq a, bv64_msb_eq b,
      BitVec.ult_eq_decide, BitVec.ult_eq_decide, BitVec.ult_eq_decide,
      bv64_and_abs_toNat a, bv64_and_abs_toNat b, f64NumKey_toNat a, f64NumKey_toNat b]
  have hla := a.isLt; have hlb := b.isLt
  by_cases hza : a.toNat = 0 ∨ a.toNat = 2 ^ 63 <;>
    by_cases hzb : b.toNat = 0 ∨ b.toNat = 2 ^ 63 <;>
    by_cases hma : 2 ^ 63 ≤ a.toNat <;>
    by_cases hmb : 2 ^ 63 ≤ b.toNat <;>
    simp [hza, hzb, hma, hmb] <;>
    omega

theorem f64Eq_iff_eq_numKey (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64Eq a b = (f64NumKey a == f64NumKey b) := by
  have hNa : f64IsNaN a = false := by unfold f64NotNaN at ha; simpa using ha
  have hNb : f64IsNaN b = false := by unfold f64NotNaN at hb; simpa using hb
  unfold f64Eq
  simp only [hNa, hNb, Bool.or_self, Bool.false_eq_true, ↓reduceIte]
  exact (f64NumKey_eq_iff a b).symm

theorem f64Le_iff_ule_numKey (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    f64Le a b = BitVec.ule (f64NumKey a) (f64NumKey b) := by
  unfold f64Le
  rw [f64Lt_iff_ult_numKey a b ha hb, f64Eq_iff_eq_numKey a b ha hb, BitVec.ult_eq_decide, BitVec.ule_eq_decide]
  by_cases hEq : f64NumKey a = f64NumKey b
  · rw [hEq]; simp
  · rw [beq_false_of_ne hEq, Bool.or_false]
    have hNeNat : (f64NumKey a).toNat ≠ (f64NumKey b).toNat := fun h => hEq (BitVec.eq_of_toNat_eq h)
    by_cases hLt : (f64NumKey a).toNat < (f64NumKey b).toNat <;> simp [hLt] <;> omega

/-- Soundness of the AVX2/AVX-512 `Min2ReducedRow64` lane filter (`!(c >= u2) == (c < u2)` on non-NaN lanes). -/
theorem not_f64Ge_eq_f64Lt (a b : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) :
    (!f64Ge a b) = f64Lt a b := by
  unfold f64Ge
  rw [f64Le_iff_ule_numKey b a hb ha, f64Lt_iff_ult_numKey a b ha hb, BitVec.ule_eq_decide, BitVec.ult_eq_decide]
  by_cases h : (f64NumKey b).toNat ≤ (f64NumKey a).toNat <;> simp [h] <;> omega

/-- `+Inf` (`0x7ff0000000000000`) is the strict top sentinel of `f64NumKey` over all non-NaN float64 lanes. -/
theorem f64PosInf_top_of_notNaN (x : BitVec 64) (hx : f64NotNaN x = true) :
    BitVec.ule (f64NumKey x) (f64NumKey f64PosInf) = true ∧
      f64Ge f64PosInf x = true ∧
      f64Lt f64PosInf x = false ∧
      f64Lt x f64PosInf = (x != f64PosInf) ∧
      (f64NumKey x == f64NumKey f64PosInf) = (x == f64PosInf) := by
  have hInf : f64NotNaN f64PosInf = true := by decide
  have hAbs : x.toNat % 2 ^ 63 ≤ 0x7ff0000000000000 := by
    have h := f64NotNaN_iff_abs_ule_posInf x
    rw [hx, BitVec.ule_eq_decide, bv64_and_abs_toNat] at h
    exact of_decide_eq_true h.symm
  have hInfKey : (f64NumKey f64PosInf).toNat = 0x7ff0000000000000 + 2 ^ 63 := by decide
  have hUle : BitVec.ule (f64NumKey x) (f64NumKey f64PosInf) = true := by
    rw [BitVec.ule_eq_decide, f64NumKey_toNat x, hInfKey]
    have hlt := x.isLt
    split <;> try split
    all_goals exact decide_eq_true (by omega)
  have hEq : (f64NumKey x == f64NumKey f64PosInf) = (x == f64PosInf) := by
    rw [f64NumKey_eq_iff x f64PosInf]
    have hInfNz : f64IsZero f64PosInf = false := by decide
    simp [hInfNz]
  have hGe : f64Ge f64PosInf x = true := by
    unfold f64Ge; rw [f64Le_iff_ule_numKey x f64PosInf hx hInf]; exact hUle
  have hLtInfX : f64Lt f64PosInf x = false := by
    rw [← not_f64Ge_eq_f64Lt f64PosInf x hInf hx]
    unfold f64Ge; rw [f64Le_iff_ule_numKey x f64PosInf hx hInf, hUle]; rfl
  have hLtXInf : f64Lt x f64PosInf = (x != f64PosInf) := by
    rw [f64Lt_iff_ult_numKey x f64PosInf hx hInf, BitVec.ult_eq_decide]
    have hUleNat : (f64NumKey x).toNat ≤ (f64NumKey f64PosInf).toNat := by
      rw [BitVec.ule_eq_decide] at hUle; exact of_decide_eq_true hUle
    by_cases hX : x = f64PosInf
    · subst hX; simp
    · have hNe : (f64NumKey x == f64NumKey f64PosInf) = false := by rw [hEq]; exact beq_false_of_ne hX
      have hNeNat : (f64NumKey x).toNat ≠ (f64NumKey f64PosInf).toNat := fun h => by
        have := beq_iff_eq.mpr (BitVec.eq_of_toNat_eq h); rw [hNe] at this; cases this
      simp [bne, beq_false_of_ne hX, show (f64NumKey x).toNat < (f64NumKey f64PosInf).toNat by omega]
  exact ⟨hUle, hGe, hLtInfX, hLtXInf, hEq⟩

/-- Exact bridge between `BitVec 64` `f64Lt` and `Track5A.f64Cmp`'s negative flag `N`. -/
theorem f64Lt_eq_f64Cmp_n (a b : BitVec 64) :
    f64Lt a b = (f64Cmp a b).n := by
  unfold f64Lt f64Cmp f64Sign
  by_cases hNaN : (f64IsNaN a || f64IsNaN b) = true
  · simp [hNaN]
  · simp only [hNaN, Bool.false_eq_true, ↓reduceIte]
    by_cases hZero : (f64IsZero a && f64IsZero b) = true
    · simp [hZero]
    · simp only [hZero, Bool.false_eq_true, ↓reduceIte]
      by_cases hSign : (a.msb != b.msb) = true
      · simp only [hSign, ↓reduceIte]
        cases a.msb <;> rfl
      · simp only [hSign, Bool.false_eq_true, ↓reduceIte]
        have hmA : (a &&& 0x7fffffffffffffff#64).toNat = a.toNat &&& 0x7fffffffffffffff := by
          simp [BitVec.toNat_and]
        have hmB : (b &&& 0x7fffffffffffffff#64).toNat = b.toNat &&& 0x7fffffffffffffff := by
          simp [BitVec.toNat_and]
        rw [BitVec.ult_eq_decide, BitVec.ult_eq_decide, hmA, hmB]
        by_cases hEqM : (a.toNat &&& 0x7fffffffffffffff == b.toNat &&& 0x7fffffffffffffff) = true
        · have hEqNat : a.toNat &&& 0x7fffffffffffffff = b.toNat &&& 0x7fffffffffffffff := beq_iff_eq.mp hEqM
          simp [hEqNat]
        · simp only [hEqM, Bool.false_eq_true, ↓reduceIte]
          cases a.msb
          · by_cases hlt : a.toNat &&& 0x7fffffffffffffff < b.toNat &&& 0x7fffffffffffffff <;> simp [hlt]
          · by_cases hlt : b.toNat &&& 0x7fffffffffffffff < a.toNat &&& 0x7fffffffffffffff <;> simp [hlt]

/-- Explicit audit of `-0.0` vs `+0.0`, subnormal ordering (`±2^-1074` .. `±maxSubnormal`), normal boundary, and `+Inf`. -/
theorem signed_zero_and_subnormal_audit :
    f64Eq f64NegZero f64PosZero = true ∧
    BitVec.ult (f64TotalKey f64NegZero) (f64TotalKey f64PosZero) = true ∧
    f64Lt f64NegInf f64MaxNegSubnormal = true ∧
    f64Lt f64MaxNegSubnormal f64MinNegSubnormal = true ∧
    f64Lt f64MinNegSubnormal f64NegZero = true ∧
    f64Lt f64PosZero f64MinPosSubnormal = true ∧
    f64Lt f64MinPosSubnormal f64MaxPosSubnormal = true ∧
    f64Lt f64MaxPosSubnormal f64MinPosNormal = true ∧
    f64Lt f64MinPosNormal f64PosInf = true := by
  decide

/-- Formal witness of x86 `VMINPD` signed-zero operand-order asymmetry (`MinNoNaN64` in `internal/simd/assign_amd64.go:219-227`). -/
theorem vminpdLane_signed_zero_asymmetry :
    vminpdLane f64PosZero f64NegZero = f64NegZero ∧
    vminpdLane f64NegZero f64PosZero = f64PosZero := by
  decide

/-- `vminpdLane` is commutative and associative up to IEEE-754 numeric equivalence (`f64NumKey`) and bit-exact after `canonZero64`. -/
theorem vminpdLane_numKey_assoc_comm (a b c : BitVec 64)
    (ha : f64NotNaN a = true) (hb : f64NotNaN b = true) (hc : f64NotNaN c = true) :
    f64NumKey (vminpdLane a b) = f64NumKey (vminpdLane b a) ∧
    canonZero64 (vminpdLane a b) = canonZero64 (vminpdLane b a) ∧
    f64NumKey (vminpdLane (vminpdLane a b) c) = f64NumKey (vminpdLane a (vminpdLane b c)) ∧
    canonZero64 (vminpdLane (vminpdLane a b) c) = canonZero64 (vminpdLane a (vminpdLane b c)) := by
  have hCanonInj : ∀ x y : BitVec 64, f64NumKey x = f64NumKey y → canonZero64 x = canonZero64 y := by
    intro x y h
    have hEq := beq_iff_eq.mpr h
    unfold f64NumKey at hEq
    rw [f64TotalKey_inj] at hEq
    exact beq_iff_eq.mp hEq
  have hMinKey : ∀ x y : BitVec 64, f64NotNaN x = true → f64NotNaN y = true →
      (f64NumKey (vminpdLane x y)).toNat = min (f64NumKey x).toNat (f64NumKey y).toNat := by
    intro x y hx hy
    unfold vminpdLane
    rw [f64Lt_iff_ult_numKey x y hx hy, BitVec.ult_eq_decide]
    by_cases hlt : (f64NumKey x).toNat < (f64NumKey y).toNat <;> simp [hlt, Nat.min_def] <;> omega
  have hNotNaNMin : ∀ x y : BitVec 64, f64NotNaN x = true → f64NotNaN y = true →
      f64NotNaN (vminpdLane x y) = true := by
    intro x y hx hy
    unfold vminpdLane
    split <;> assumption
  have hComm : f64NumKey (vminpdLane a b) = f64NumKey (vminpdLane b a) := by
    apply BitVec.eq_of_toNat_eq
    rw [hMinKey a b ha hb, hMinKey b a hb ha, Nat.min_comm]
  have hAssoc : f64NumKey (vminpdLane (vminpdLane a b) c) = f64NumKey (vminpdLane a (vminpdLane b c)) := by
    apply BitVec.eq_of_toNat_eq
    rw [hMinKey _ c (hNotNaNMin a b ha hb) hc, hMinKey a b ha hb,
        hMinKey a _ ha (hNotNaNMin b c hb hc), hMinKey b c hb hc, Nat.min_assoc]
  exact ⟨hComm, hCanonInj _ _ hComm, hAssoc, hCanonInj _ _ hAssoc⟩


/-! ### 4-Lane (AVX2) & 8-Lane (AVX-512) Masked `+Inf` Tail & `Min2` Reduction Equivalence -/

/-- Value-index lane pair (`idx = none` denotes `+Inf` sentinel or masked tail lane). -/
structure LaneEntry where
  val : BitVec 64
  idx : Option Nat
  deriving DecidableEq, Repr

/-- Canonical `+Inf` sentinel lane (`0x7ff0000000000000`, `idx = none`). -/
def infEntry : LaneEntry := { val := f64PosInf, idx := none }

/-- Tag active column `j` with `val := x` (mapping `+Inf` to `infEntry`, matching `!(r >= u2)` sentinel skip). -/
def mkLaneEntry (j : Nat) (x : BitVec 64) : LaneEntry :=
  if x == f64PosInf then infEntry else { val := x, idx := some j }

/-- Option-index tie-breaker (`some i < some j` iff `i < j`; active `some _` precedes sentinel `none`). -/
def optIdxLt : Option Nat → Option Nat → Bool
  | some i, some j => decide (i < j)
  | some _, none => true
  | none, _ => false

/-- Lexicographic strict linear order on `LaneEntry`: `(f64NumKey val, idx, f64TotalKey val)`. -/
def entryLt (a b : LaneEntry) : Bool :=
  let ka := (f64NumKey a.val).toNat
  let kb := (f64NumKey b.val).toNat
  if ka != kb then decide (ka < kb)
  else if a.idx != b.idx then optIdxLt a.idx b.idx
  else decide ((f64TotalKey a.val).toNat < (f64TotalKey b.val).toNat)

def entryLe (a b : LaneEntry) : Bool :=
  entryLt a b || a == b

private theorem optIdxLt_irrefl (x : Option Nat) : optIdxLt x x = false := by
  cases x <;> simp [optIdxLt]

private theorem optIdxLt_trans {x y z : Option Nat}
    (h1 : optIdxLt x y = true) (h2 : optIdxLt y z = true) : optIdxLt x z = true := by
  cases x <;> cases y <;> cases z <;> simp [optIdxLt] at h1 h2 ⊢ <;> omega

private theorem optIdxLt_trichotomy (x y : Option Nat) (hne : x ≠ y) :
    optIdxLt x y = true ∨ optIdxLt y x = true := by
  cases x <;> cases y <;> simp [optIdxLt] at hne ⊢ <;> omega

private theorem f64TotalKey_toNat_inj {a b : BitVec 64}
    (h : (f64TotalKey a).toNat = (f64TotalKey b).toNat) : a = b := by
  have hbv : f64TotalKey a = f64TotalKey b := BitVec.eq_of_toNat_eq h
  have hbeq : (a == b) = true := by
    rw [← f64TotalKey_inj a b]
    exact beq_iff_eq.mpr hbv
  exact beq_iff_eq.mp hbeq

theorem entryLt_irrefl (a : LaneEntry) : entryLt a a = false := by
  simp [entryLt]

theorem entryLt_trans {a b c : LaneEntry}
    (hab : entryLt a b = true) (hbc : entryLt b c = true) : entryLt a c = true := by
  unfold entryLt at hab hbc ⊢
  by_cases h1 : ((f64NumKey a.val).toNat != (f64NumKey b.val).toNat) = true <;>
  by_cases h2 : ((f64NumKey b.val).toNat != (f64NumKey c.val).toNat) = true
  · simp only [h1, h2, ↓reduceIte, decide_eq_true_eq] at hab hbc
    have h3 : ((f64NumKey a.val).toNat != (f64NumKey c.val).toNat) = true := bne_iff_ne.mpr (by omega)
    simp only [h3, ↓reduceIte, decide_eq_true_eq]; omega
  · have hk2 : (f64NumKey b.val).toNat = (f64NumKey c.val).toNat := by simpa [bne_iff_ne] using h2
    simp only [h1, ↓reduceIte, decide_eq_true_eq] at hab
    have h3 : ((f64NumKey a.val).toNat != (f64NumKey c.val).toNat) = true := bne_iff_ne.mpr (by omega)
    simp only [h3, ↓reduceIte, decide_eq_true_eq]; omega
  · have hk1 : (f64NumKey a.val).toNat = (f64NumKey b.val).toNat := by simpa [bne_iff_ne] using h1
    simp only [h2, ↓reduceIte, decide_eq_true_eq] at hbc
    have h3 : ((f64NumKey a.val).toNat != (f64NumKey c.val).toNat) = true := bne_iff_ne.mpr (by omega)
    simp only [h3, ↓reduceIte, decide_eq_true_eq]; omega
  · have hk1 : (f64NumKey a.val).toNat = (f64NumKey b.val).toNat := by simpa [bne_iff_ne] using h1
    have hk2 : (f64NumKey b.val).toNat = (f64NumKey c.val).toNat := by simpa [bne_iff_ne] using h2
    have h3 : ((f64NumKey a.val).toNat != (f64NumKey c.val).toNat) = false := by simp [hk1, hk2]
    simp only [h1, h2, h3, Bool.false_eq_true, ↓reduceIte] at hab hbc ⊢
    by_cases hi1 : (a.idx != b.idx) = true <;> by_cases hi2 : (b.idx != c.idx) = true
    · simp only [hi1, hi2, ↓reduceIte] at hab hbc
      have ht := optIdxLt_trans hab hbc
      have hi3 : (a.idx != c.idx) = true := bne_iff_ne.mpr (fun heq => by rw [heq, optIdxLt_irrefl] at ht; cases ht)
      simp only [hi3, ↓reduceIte]; exact ht
    · have heq2 : b.idx = c.idx := by simpa [bne_iff_ne] using hi2
      simp only [hi1, ↓reduceIte] at hab
      rw [← heq2]; simp only [hi1, ↓reduceIte]; exact hab
    · have heq1 : a.idx = b.idx := by simpa [bne_iff_ne] using hi1
      simp only [hi2, ↓reduceIte] at hbc
      rw [heq1]; simp only [hi2, ↓reduceIte]; exact hbc
    · have heq1 : a.idx = b.idx := by simpa [bne_iff_ne] using hi1
      have heq2 : b.idx = c.idx := by simpa [bne_iff_ne] using hi2
      have hi3 : (a.idx != c.idx) = false := by simp [heq1, heq2]
      simp only [hi1, hi2, hi3, Bool.false_eq_true, ↓reduceIte, decide_eq_true_eq] at hab hbc ⊢
      omega

theorem entryLt_trichotomy (a b : LaneEntry) :
    entryLt a b = true ∨ a = b ∨ entryLt b a = true := by
  unfold entryLt
  by_cases hk : (f64NumKey a.val).toNat = (f64NumKey b.val).toNat
  · simp only [hk, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte]
    by_cases hi : a.idx = b.idx
    · simp only [hi, bne_self_eq_false, Bool.false_eq_true, ↓reduceIte, decide_eq_true_eq]
      by_cases ht : (f64TotalKey a.val).toNat = (f64TotalKey b.val).toNat
      · right; left
        have hv := f64TotalKey_toNat_inj ht
        cases a; cases b; simp_all
      · rcases Nat.lt_or_gt_of_ne ht with hlt | hgt
        · left; exact hlt
        · right; right; exact hgt
    · have hne1 : (a.idx != b.idx) = true := bne_iff_ne.mpr hi
      have hne2 : (b.idx != a.idx) = true := bne_iff_ne.mpr (Ne.symm hi)
      simp only [hne1, hne2, ↓reduceIte]
      rcases optIdxLt_trichotomy a.idx b.idx hi with h1 | h2
      · left; exact h1
      · right; right; exact h2
  · have hne1 : ((f64NumKey a.val).toNat != (f64NumKey b.val).toNat) = true := bne_iff_ne.mpr hk
    have hne2 : ((f64NumKey b.val).toNat != (f64NumKey a.val).toNat) = true := bne_iff_ne.mpr (Ne.symm hk)
    simp only [hne1, hne2, ↓reduceIte, decide_eq_true_eq]
    rcases Nat.lt_or_gt_of_ne hk with hlt | hgt
    · left; exact hlt
    · right; right; exact hgt

theorem entryLt_asymm {a b : LaneEntry} (h : entryLt a b = true) : entryLt b a = false := by
  by_cases hba : entryLt b a = true
  · have hc := entryLt_trans h hba
    rw [entryLt_irrefl] at hc; cases hc
  · exact Bool.eq_false_iff.mpr hba

private theorem entryLe_of_not_lt {a b : LaneEntry} (h : entryLt a b = false) :
    entryLe b a = true := by
  unfold entryLe
  rcases entryLt_trichotomy a b with hlt | rfl | hgt
  · rw [h] at hlt; cases hlt
  · simp
  · simp [hgt]

private theorem entryLt_of_lt_of_le {a b c : LaneEntry}
    (hab : entryLt a b = true) (hbc : entryLe b c = true) : entryLt a c = true := by
  unfold entryLe at hbc
  simp only [Bool.or_eq_true, beq_iff_eq] at hbc
  rcases hbc with hlt | rfl
  · exact entryLt_trans hab hlt
  · exact hab

private theorem entryLe_trans {a b c : LaneEntry}
    (hab : entryLe a b = true) (hbc : entryLe b c = true) : entryLe a c = true := by
  unfold entryLe at hab hbc ⊢
  simp only [Bool.or_eq_true, beq_iff_eq] at hab hbc ⊢
  rcases hab with h1 | rfl
  · rcases hbc with h2 | rfl
    · left; exact entryLt_trans h1 h2
    · left; exact h1
  · exact hbc

private theorem not_entryLt_of_le {a b : LaneEntry} (h : entryLe a b = true) :
    entryLt b a = false := by
  unfold entryLe at h
  simp only [Bool.or_eq_true, beq_iff_eq] at h
  rcases h with hlt | rfl
  · exact entryLt_asymm hlt
  · exact entryLt_irrefl a

private theorem entryLt_of_le_of_lt {a b c : LaneEntry}
    (hab : entryLe a b = true) (hbc : entryLt b c = true) : entryLt a c = true := by
  unfold entryLe at hab
  simp only [Bool.or_eq_true, beq_iff_eq] at hab
  rcases hab with hlt | rfl
  · exact entryLt_trans hlt hbc
  · exact hbc

/-- Running `(min1, min2)` pair over `LaneEntry`. -/
abbrev Min2Pair := LaneEntry × LaneEntry

def initMin2 : Min2Pair := (infEntry, infEntry)

def insertEntry (e : LaneEntry) (p : Min2Pair) : Min2Pair :=
  if entryLt e p.1 then (e, p.1)
  else if entryLt e p.2 then (p.1, e)
  else p

def singleMin2 (e : LaneEntry) : Min2Pair :=
  insertEntry e initMin2

/-- Associative SIMD pair-combine operator over ordered `(min1, min2)` summaries. -/
def mergeMin2 (p q : Min2Pair) : Min2Pair :=
  insertEntry q.2 (insertEntry q.1 p)

def foldEntries (es : List LaneEntry) (st : Min2Pair) : Min2Pair :=
  es.foldl (fun acc e => insertEntry e acc) st

theorem insertEntry_ordered (e : LaneEntry) {p : Min2Pair} (hp : entryLe p.1 p.2 = true) :
    entryLe (insertEntry e p).1 (insertEntry e p).2 = true := by
  unfold insertEntry
  by_cases h1 : entryLt e p.1 = true
  · simp [h1, entryLe]
  · have h1f : entryLt e p.1 = false := Bool.eq_false_iff.mpr h1
    simp only [h1f, Bool.false_eq_true, ↓reduceIte]
    by_cases h2 : entryLt e p.2 = true
    · simp only [h2, ↓reduceIte]; exact entryLe_of_not_lt h1f
    · simp [h2, hp]

private theorem insertEntry_comm_lt {a b : LaneEntry} {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true) (hab : entryLt a b = true) :
    insertEntry a (insertEntry b p) = insertEntry b (insertEntry a p) := by
  have hba : entryLt b a = false := entryLt_asymm hab
  unfold insertEntry
  by_cases hb1 : entryLt b p.1 = true
  · have ha1 : entryLt a p.1 = true := entryLt_trans hab hb1
    simp [hb1, ha1, hab, hba]
  · have hb1f : entryLt b p.1 = false := Bool.eq_false_iff.mpr hb1
    by_cases hb2 : entryLt b p.2 = true
    · have ha2 : entryLt a p.2 = true := entryLt_trans hab hb2
      by_cases ha1 : entryLt a p.1 = true
      · simp [hb1f, hb2, ha1, hba]
      · have ha1f : entryLt a p.1 = false := Bool.eq_false_iff.mpr ha1
        simp [hb1f, hb2, ha1f, ha2, hab, hba]
    · have hb2f : entryLt b p.2 = false := Bool.eq_false_iff.mpr hb2
      by_cases ha1 : entryLt a p.1 = true
      · have hbp1 := hp
        simp [hb1f, hb2f, ha1, hba]
      · have ha1f : entryLt a p.1 = false := Bool.eq_false_iff.mpr ha1
        by_cases ha2 : entryLt a p.2 = true
        · simp [hb1f, hb2f, ha1f, ha2, hba]
        · have ha2f : entryLt a p.2 = false := Bool.eq_false_iff.mpr ha2
          simp [hb1f, hb2f, ha1f, ha2f]

theorem insertEntry_comm (a b : LaneEntry) {p : Min2Pair} (hp : entryLe p.1 p.2 = true) :
    insertEntry a (insertEntry b p) = insertEntry b (insertEntry a p) := by
  rcases entryLt_trichotomy a b with hab | rfl | hba
  · exact insertEntry_comm_lt hp hab
  · rfl
  · exact (insertEntry_comm_lt hp hba).symm

private theorem insertEntry_first_le {b c : LaneEntry} (p : Min2Pair) (hbc : entryLe b c = true) :
    entryLe (insertEntry b p).1 c = true := by
  unfold insertEntry
  by_cases hb1 : entryLt b p.1 = true
  · simp [hb1, hbc]
  · have hb1f : entryLt b p.1 = false := Bool.eq_false_iff.mpr hb1
    simp only [hb1f, Bool.false_eq_true, ↓reduceIte]
    have hp1b : entryLe p.1 b = true := entryLe_of_not_lt hb1f
    split <;> exact entryLe_trans hp1b hbc

private theorem insertEntry_both_le {a c : LaneEntry} {p : Min2Pair}
    (hac : entryLe a c = true) (h1 : entryLe p.1 c = true) :
    entryLe (insertEntry a p).1 c = true ∧ entryLe (insertEntry a p).2 c = true := by
  unfold insertEntry
  by_cases ha1 : entryLt a p.1 = true
  · simp [ha1, hac, h1]
  · have ha1f : entryLt a p.1 = false := Bool.eq_false_iff.mpr ha1
    simp only [ha1f, Bool.false_eq_true, ↓reduceIte]
    by_cases ha2 : entryLt a p.2 = true
    · simp [ha2, h1, hac]
    · have ha2f : entryLt a p.2 = false := Bool.eq_false_iff.mpr ha2
      simp only [ha2f, Bool.false_eq_true, ↓reduceIte]
      exact ⟨h1, entryLe_trans (entryLe_of_not_lt ha2f) hac⟩

private theorem insertEntry_noop_of_both_le {c : LaneEntry} {q : Min2Pair}
    (h1 : entryLe q.1 c = true) (h2 : entryLe q.2 c = true) :
    insertEntry c q = q := by
  unfold insertEntry
  rw [not_entryLt_of_le h1, not_entryLt_of_le h2]
  rfl

theorem insertEntry_dropThird {a b c : LaneEntry} {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true) (hac : entryLe a c = true) (hbc : entryLe b c = true) :
    insertEntry a (insertEntry b (insertEntry c p)) = insertEntry a (insertEntry b p) := by
  rw [insertEntry_comm b c hp, insertEntry_comm a c (insertEntry_ordered b hp)]
  have h1 := insertEntry_first_le p hbc
  have h12 := insertEntry_both_le hac h1
  exact insertEntry_noop_of_both_le h12.1 h12.2

theorem mergeMin2_ordered {p q : Min2Pair} (hp : entryLe p.1 p.2 = true) :
    entryLe (mergeMin2 p q).1 (mergeMin2 p q).2 = true :=
  insertEntry_ordered q.2 (insertEntry_ordered q.1 hp)

theorem mergeMin2_insert (e : LaneEntry) {p q : Min2Pair}
    (hp : entryLe p.1 p.2 = true) (hq : entryLe q.1 q.2 = true) :
    mergeMin2 p (insertEntry e q) = insertEntry e (mergeMin2 p q) := by
  have hIns : insertEntry e q =
      (if entryLt e q.1 then (e, q.1) else if entryLt e q.2 then (q.1, e) else q) := rfl
  rw [hIns]
  by_cases he1 : entryLt e q.1 = true
  · simp only [he1, ↓reduceIte]
    unfold mergeMin2
    rw [insertEntry_comm q.2 q.1 hp]
    have he2 : entryLe e q.2 = true := by
      have hlt : entryLt e q.2 = true := entryLt_of_lt_of_le he1 hq
      unfold entryLe; simp [hlt]
    rw [insertEntry_dropThird hp he2 hq, insertEntry_comm q.1 e hp]
  · have he1f : entryLt e q.1 = false := Bool.eq_false_iff.mpr he1
    simp only [he1f, Bool.false_eq_true, ↓reduceIte]
    by_cases he2 : entryLt e q.2 = true
    · simp only [he2, ↓reduceIte]
      unfold mergeMin2
      rw [insertEntry_comm q.2 q.1 hp]
      have he2le : entryLe e q.2 = true := by unfold entryLe; simp [he2]
      rw [insertEntry_dropThird hp he2le hq]
    · have he2f : entryLt e q.2 = false := Bool.eq_false_iff.mpr he2
      simp only [he2f, Bool.false_eq_true, ↓reduceIte]
      unfold mergeMin2
      rw [insertEntry_comm e q.2 (insertEntry_ordered q.1 hp),
          insertEntry_comm e q.1 hp,
          insertEntry_dropThird hp (entryLe_of_not_lt he2f) (entryLe_of_not_lt he1f)]

theorem mergeMin2_foldEntries (es : List LaneEntry) {p q : Min2Pair}
    (hp : entryLe p.1 p.2 = true) (hq : entryLe q.1 q.2 = true) :
    mergeMin2 p (foldEntries es q) = foldEntries es (mergeMin2 p q) := by
  induction es generalizing q with
  | nil => rfl
  | cons e rest ih =>
    change mergeMin2 p (foldEntries rest (insertEntry e q)) =
      foldEntries rest (insertEntry e (mergeMin2 p q))
    rw [ih (insertEntry_ordered e hq), mergeMin2_insert e hp hq]

/-- 4-lane (AVX2) balanced binary tree `Min2` reduction `((0, 1) + (2, 3))`. -/
def vec4TreeMin2 (e0 e1 e2 e3 : LaneEntry) : Min2Pair :=
  mergeMin2 (mergeMin2 (singleMin2 e0) (singleMin2 e1))
            (mergeMin2 (singleMin2 e2) (singleMin2 e3))

/-- 8-lane (AVX-512) balanced binary tree `Min2` reduction `(((0,1)+(2,3)) + ((4,5)+(6,7)))`. -/
def vec8TreeMin2 (e0 e1 e2 e3 e4 e5 e6 e7 : LaneEntry) : Min2Pair :=
  mergeMin2 (vec4TreeMin2 e0 e1 e2 e3) (vec4TreeMin2 e4 e5 e6 e7)

private theorem initMin2_ordered : entryLe initMin2.1 initMin2.2 = true := by decide

private theorem singleMin2_ordered (e : LaneEntry) :
    entryLe (singleMin2 e).1 (singleMin2 e).2 = true :=
  insertEntry_ordered e initMin2_ordered

private theorem entryLe_inf_of_wf {e : LaneEntry}
    (hNaN : f64NotNaN e.val = true) (hInf : e.val = f64PosInf → e = infEntry) :
    entryLe e infEntry = true := by
  have hTop := f64PosInf_top_of_notNaN e.val hNaN
  by_cases hEq : e.val = f64PosInf
  · rw [hInf hEq]; decide
  · have hNeKey : (f64NumKey e.val == f64NumKey f64PosInf) = false := by
      rw [hTop.2.2.2.2]; exact beq_false_of_ne hEq
    have hNeNat : (f64NumKey e.val).toNat ≠ (f64NumKey f64PosInf).toNat := by
      intro hN
      have hB := BitVec.eq_of_toNat_eq hN
      rw [beq_iff_eq.mpr hB] at hNeKey; cases hNeKey
    have hLeNat : (f64NumKey e.val).toNat ≤ (f64NumKey f64PosInf).toNat := by
      have hU := hTop.1; rw [BitVec.ule_eq_decide] at hU; exact of_decide_eq_true hU
    have hLt : entryLt e infEntry = true := by
      unfold entryLt infEntry
      have hBne : ((f64NumKey e.val).toNat != (f64NumKey f64PosInf).toNat) = true := bne_iff_ne.mpr hNeNat
      simp only [hBne, ↓reduceIte, decide_eq_true_eq]; omega
    unfold entryLe; simp [hLt]

private theorem mkLaneEntry_le_inf (j : Nat) {x : BitVec 64} (hx : f64NotNaN x = true) :
    entryLe (mkLaneEntry j x) infEntry = true := by
  unfold mkLaneEntry
  by_cases h : (x == f64PosInf) = true
  · simp [h]; decide
  · simp only [h, Bool.false_eq_true, ↓reduceIte]
    exact entryLe_inf_of_wf hx (fun hEq => absurd (beq_iff_eq.mpr hEq) (by simp [h]))

private theorem mergeMin2_init_left {p : Min2Pair}
    (h1 : entryLe p.1 infEntry = true) (h2 : entryLe p.2 infEntry = true) :
    mergeMin2 p initMin2 = p := by
  unfold mergeMin2 initMin2
  rw [insertEntry_noop_of_both_le h1 h2, insertEntry_noop_of_both_le h1 h2]

private theorem mergeMin2_fold_init (es : List LaneEntry) {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true)
    (h1 : entryLe p.1 infEntry = true) (h2 : entryLe p.2 infEntry = true) :
    mergeMin2 p (foldEntries es initMin2) = foldEntries es p := by
  rw [mergeMin2_foldEntries es hp initMin2_ordered, mergeMin2_init_left h1 h2]

private theorem foldEntries_ordered (es : List LaneEntry) {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true) :
    entryLe (foldEntries es p).1 (foldEntries es p).2 = true := by
  induction es generalizing p with
  | nil => exact hp
  | cons e rest ih => exact ih (insertEntry_ordered e hp)

private theorem foldEntries_le_inf (es : List LaneEntry) {p : Min2Pair}
    (h1 : entryLe p.1 infEntry = true) (h2 : entryLe p.2 infEntry = true)
    (hes : ∀ e ∈ es, entryLe e infEntry = true) :
    entryLe (foldEntries es p).1 infEntry = true ∧ entryLe (foldEntries es p).2 infEntry = true := by
  induction es generalizing p with
  | nil => exact ⟨h1, h2⟩
  | cons e rest ih =>
    have he := hes e (by simp)
    have hb := insertEntry_both_le he h1
    exact ih hb.1 hb.2 (fun y hy => hes y (by simp [hy]))

/-- 4-lane tree reduction equals sequential 4-lane `foldEntries` for all bounded entries. -/
theorem vec4TreeMin2_eq_init {e0 e1 e2 e3 : LaneEntry}
    (he0 : entryLe e0 infEntry = true) (he1 : entryLe e1 infEntry = true)
    (he2 : entryLe e2 infEntry = true) :
    vec4TreeMin2 e0 e1 e2 e3 = foldEntries [e0, e1, e2, e3] initMin2 := by
  have hs : ∀ x, singleMin2 x = foldEntries [x] initMin2 := fun _ => rfl
  have hm01 : mergeMin2 (singleMin2 e0) (singleMin2 e1) = foldEntries [e0, e1] initMin2 := by
    rw [hs e0, hs e1]
    have hb := foldEntries_le_inf [e0] (p := initMin2) (by decide) (by decide)
      (by intro y hy; simp at hy; subst hy; exact he0)
    rw [mergeMin2_fold_init [e1] (foldEntries_ordered [e0] initMin2_ordered) hb.1 hb.2]
    rfl
  have hm23 : mergeMin2 (singleMin2 e2) (singleMin2 e3) = foldEntries [e2, e3] initMin2 := by
    rw [hs e2, hs e3]
    have hb := foldEntries_le_inf [e2] (p := initMin2) (by decide) (by decide)
      (by intro y hy; simp at hy; subst hy; exact he2)
    rw [mergeMin2_fold_init [e3] (foldEntries_ordered [e2] initMin2_ordered) hb.1 hb.2]
    rfl
  unfold vec4TreeMin2
  rw [hm01, hm23]
  have hb01 := foldEntries_le_inf [e0, e1] (p := initMin2) (by decide) (by decide)
    (by intro y hy; simp at hy; rcases hy with rfl | rfl <;> assumption)
  rw [mergeMin2_fold_init [e2, e3] (foldEntries_ordered [e0, e1] initMin2_ordered) hb01.1 hb01.2]
  rfl

theorem vec4TreeMin2_eq_fold {e0 e1 e2 e3 : LaneEntry} {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true)
    (hp1 : entryLe p.1 infEntry = true) (hp2 : entryLe p.2 infEntry = true)
    (he0 : entryLe e0 infEntry = true) (he1 : entryLe e1 infEntry = true)
    (he2 : entryLe e2 infEntry = true) :
    mergeMin2 p (vec4TreeMin2 e0 e1 e2 e3) = foldEntries [e0, e1, e2, e3] p := by
  rw [vec4TreeMin2_eq_init he0 he1 he2, mergeMin2_fold_init [e0, e1, e2, e3] hp hp1 hp2]

/-- 8-lane tree reduction equals sequential 8-lane `foldEntries` for all bounded entries. -/
theorem vec8TreeMin2_eq_fold {e0 e1 e2 e3 e4 e5 e6 e7 : LaneEntry} {p : Min2Pair}
    (hp : entryLe p.1 p.2 = true)
    (hp1 : entryLe p.1 infEntry = true) (hp2 : entryLe p.2 infEntry = true)
    (he0 : entryLe e0 infEntry = true) (he1 : entryLe e1 infEntry = true)
    (he2 : entryLe e2 infEntry = true) (he3 : entryLe e3 infEntry = true)
    (he4 : entryLe e4 infEntry = true) (he5 : entryLe e5 infEntry = true)
    (he6 : entryLe e6 infEntry = true) :
    mergeMin2 p (vec8TreeMin2 e0 e1 e2 e3 e4 e5 e6 e7) =
      foldEntries [e0, e1, e2, e3, e4, e5, e6, e7] p := by
  have hv8 : vec8TreeMin2 e0 e1 e2 e3 e4 e5 e6 e7 =
      foldEntries [e0, e1, e2, e3, e4, e5, e6, e7] initMin2 := by
    unfold vec8TreeMin2
    rw [vec4TreeMin2_eq_init he0 he1 he2, vec4TreeMin2_eq_init he4 he5 he6]
    have hb03 := foldEntries_le_inf [e0, e1, e2, e3] (p := initMin2) (by decide) (by decide)
      (by intro y hy; simp at hy; rcases hy with rfl | rfl | rfl | rfl <;> assumption)
    rw [mergeMin2_fold_init [e4, e5, e6, e7] (foldEntries_ordered [e0, e1, e2, e3] initMin2_ordered) hb03.1 hb03.2]
    rfl
  rw [hv8, mergeMin2_fold_init [e0, e1, e2, e3, e4, e5, e6, e7] hp hp1 hp2]


/-- Build indexed `LaneEntry` list starting at column `base`. -/
def mkEntries (base : Nat) : List (BitVec 64) → List LaneEntry
  | [] => []
  | x :: xs => mkLaneEntry base x :: mkEntries (base + 1) xs

/-- Sequential left-to-right scalar `Min2` step matching `internal/simd/kernels_amd64.go:881-925`. -/
def scalarMin2Step (st : Min2Pair) (j : Nat) (r : BitVec 64) : Min2Pair :=
  if !f64Ge r st.2.val then
    if f64Lt r st.1.val then
      ({ val := r, idx := some j }, st.1)
    else
      (st.1, { val := r, idx := some j })
  else
    st

/-- Sequential left-to-right scalar `Min2` fold over `xs` starting at column `base`. -/
def scalarMin2Fold (base : Nat) : List (BitVec 64) → Min2Pair → Min2Pair
  | [], st => st
  | x :: xs, st => scalarMin2Fold (base + 1) xs (scalarMin2Step st base x)

/-- 8-lane / 4-lane padded lane fetch (`off < chunk.length` active, else masked `+Inf` sentinel). -/
def paddedLane (base : Nat) (chunk : List (BitVec 64)) (off : Nat) : LaneEntry :=
  match chunk[off]? with
  | some x => mkLaneEntry (base + off) x
  | none => infEntry

/-- 4-lane (AVX2) chunked tree reduction with masked `+Inf` (`infEntry`) tail padding. -/
def simdTree4Aux : Nat → Nat → List (BitVec 64) → Min2Pair → Min2Pair
  | 0, _, _, st => st
  | fuel + 1, base, xs, st =>
    match xs with
    | [] => st
    | _ :: _ =>
      let c := xs.take 4
      let v := vec4TreeMin2 (paddedLane base c 0) (paddedLane base c 1)
                            (paddedLane base c 2) (paddedLane base c 3)
      simdTree4Aux fuel (base + 4) (xs.drop 4) (mergeMin2 st v)

def simdTree4Reduce (xs : List (BitVec 64)) : Min2Pair :=
  simdTree4Aux xs.length 0 xs initMin2

/-- 8-lane (AVX-512) chunked tree reduction with masked `+Inf` (`infEntry`) tail padding. -/
def simdTree8Aux : Nat → Nat → List (BitVec 64) → Min2Pair → Min2Pair
  | 0, _, _, st => st
  | fuel + 1, base, xs, st =>
    match xs with
    | [] => st
    | _ :: _ =>
      let c := xs.take 8
      let v := vec8TreeMin2 (paddedLane base c 0) (paddedLane base c 1) (paddedLane base c 2) (paddedLane base c 3)
                            (paddedLane base c 4) (paddedLane base c 5) (paddedLane base c 6) (paddedLane base c 7)
      simdTree8Aux fuel (base + 8) (xs.drop 8) (mergeMin2 st v)

def simdTree8Reduce (xs : List (BitVec 64)) : Min2Pair :=
  simdTree8Aux xs.length 0 xs initMin2

/-- Prior-entry validity invariant before column `k`. -/
def ValidBefore (k : Nat) (e : LaneEntry) : Prop :=
  f64NotNaN e.val = true ∧ (e = infEntry ∨ ∃ i < k, e.idx = some i ∧ e.val ≠ f64PosInf)

private theorem validBefore_mono {k k' : Nat} {e : LaneEntry} (hle : k ≤ k') (h : ValidBefore k e) :
    ValidBefore k' e := by
  rcases h with ⟨hN, rfl | ⟨i, hi, hidx, hne⟩⟩
  · exact ⟨hN, Or.inl rfl⟩
  · exact ⟨hN, Or.inr ⟨i, Nat.lt_of_lt_of_le hi hle, hidx, hne⟩⟩

private theorem validBefore_le_inf {k : Nat} {e : LaneEntry} (h : ValidBefore k e) :
    entryLe e infEntry = true := by
  rcases h with ⟨hN, hCases⟩
  apply entryLe_inf_of_wf hN
  intro hEq
  rcases hCases with rfl | ⟨_, _, _, hne⟩
  · rfl
  · exact absurd hEq hne

private theorem mkLaneEntry_validBefore (k : Nat) {x : BitVec 64} (hx : f64NotNaN x = true) :
    ValidBefore (k + 1) (mkLaneEntry k x) := by
  unfold mkLaneEntry
  by_cases h : (x == f64PosInf) = true
  · simp only [h, ↓reduceIte]; exact ⟨by decide, Or.inl rfl⟩
  · simp only [h, Bool.false_eq_true, ↓reduceIte]
    exact ⟨hx, Or.inr ⟨k, Nat.lt_succ_self k, rfl, fun heq => absurd (beq_iff_eq.mpr heq) (by simp [h])⟩⟩

private theorem entryLt_active_eq_f64Lt {k : Nat} {x : BitVec 64} {e : LaneEntry}
    (hx : f64NotNaN x = true) (hne : x ≠ f64PosInf) (he : ValidBefore k e) :
    entryLt { val := x, idx := some k } e = f64Lt x e.val := by
  rw [f64Lt_iff_ult_numKey x e.val hx he.1, BitVec.ult_eq_decide]
  unfold entryLt
  dsimp only
  by_cases hk : ((f64NumKey x).toNat != (f64NumKey e.val).toNat) = true
  · simp [hk]
  · have hkEq : (f64NumKey x).toNat = (f64NumKey e.val).toNat := by simpa [bne_iff_ne] using hk
    have hBvEq : f64NumKey x = f64NumKey e.val := BitVec.eq_of_toNat_eq hkEq
    rcases he.2 with rfl | ⟨i, hi, hidx, _⟩
    · have hTop := (f64PosInf_top_of_notNaN x hx).2.2.2.2
      have hIsInf : (x == f64PosInf) = true := hTop ▸ beq_iff_eq.mpr hBvEq
      exact absurd (beq_iff_eq.mp hIsInf) hne
    · have hiBne : (some k != some i) = true := bne_iff_ne.mpr (by intro h; injection h; omega)
      rw [hidx]
      simp only [hk, Bool.false_eq_true, ↓reduceIte, hiBne, optIdxLt]
      rw [hkEq]
      have hNotKI : ¬ (k < i) := by omega
      simp [hNotKI]

private theorem entryLe_imp_ule_numKey {a b : LaneEntry} (h : entryLe a b = true) :
    BitVec.ule (f64NumKey a.val) (f64NumKey b.val) = true := by
  rw [BitVec.ule_eq_decide]
  unfold entryLe entryLt at h
  simp only [Bool.or_eq_true] at h
  rcases h with hlt | heq
  · by_cases hk : ((f64NumKey a.val).toNat != (f64NumKey b.val).toNat) = true
    · simp only [hk, ↓reduceIte, decide_eq_true_eq] at hlt
      exact decide_eq_true (Nat.le_of_lt hlt)
    · have hkEq : (f64NumKey a.val).toNat = (f64NumKey b.val).toNat := by simpa [bne_iff_ne] using hk
      exact decide_eq_true (Nat.le_of_eq hkEq)
  · have hEq : a = b := beq_iff_eq.mp heq
    rw [hEq]; exact decide_eq_true (Nat.le_refl _)

/-- Single-step equivalence: `insertEntry (mkLaneEntry k x)` is bit-identical to `scalarMin2Step`. -/
theorem insertEntry_mkLane_eq_scalarStep (k : Nat) {x : BitVec 64} {st : Min2Pair}
    (hx : f64NotNaN x = true)
    (hOrd : entryLe st.1 st.2 = true)
    (hv1 : ValidBefore k st.1) (hv2 : ValidBefore k st.2) :
    insertEntry (mkLaneEntry k x) st = scalarMin2Step st k x := by
  unfold mkLaneEntry scalarMin2Step
  by_cases hInf : (x == f64PosInf) = true
  · have hxEq : x = f64PosInf := beq_iff_eq.mp hInf
    subst hxEq
    simp only [beq_self_eq_true, ↓reduceIte]
    have hGe : f64Ge f64PosInf st.2.val = true := (f64PosInf_top_of_notNaN st.2.val hv2.1).2.1
    rw [insertEntry_noop_of_both_le (validBefore_le_inf hv1) (validBefore_le_inf hv2), hGe]
    rfl
  · have hne : x ≠ f64PosInf := fun heq => absurd (beq_iff_eq.mpr heq) (by simp [hInf])
    simp only [hInf, Bool.false_eq_true, ↓reduceIte]
    rw [not_f64Ge_eq_f64Lt x st.2.val hx hv2.1]
    unfold insertEntry
    rw [entryLt_active_eq_f64Lt hx hne hv1, entryLt_active_eq_f64Lt hx hne hv2]
    by_cases hlt1 : f64Lt x st.1.val = true
    · have hlt2 : f64Lt x st.2.val = true := by
        have h12 : BitVec.ule (f64NumKey st.1.val) (f64NumKey st.2.val) = true := entryLe_imp_ule_numKey hOrd
        rw [f64Lt_iff_ult_numKey x st.1.val hx hv1.1, BitVec.ult_eq_decide] at hlt1
        rw [f64Lt_iff_ult_numKey x st.2.val hx hv2.1, BitVec.ult_eq_decide]
        rw [BitVec.ule_eq_decide] at h12
        exact decide_eq_true (Nat.lt_of_lt_of_le (of_decide_eq_true hlt1) (of_decide_eq_true h12))
      simp [hlt1, hlt2]
    · simp only [hlt1, Bool.false_eq_true, ↓reduceIte]

private theorem insertEntry_preserves_valid {k : Nat} {e : LaneEntry} {st : Min2Pair}
    (he : ValidBefore k e) (hv1 : ValidBefore k st.1) (hv2 : ValidBefore k st.2) :
    ValidBefore k (insertEntry e st).1 ∧ ValidBefore k (insertEntry e st).2 := by
  unfold insertEntry
  split
  · exact ⟨he, hv1⟩
  · split
    · exact ⟨hv1, he⟩
    · exact ⟨hv1, hv2⟩

/-- Full list equivalence between `foldEntries` and sequential `scalarMin2Fold`. -/
theorem foldEntries_mkEntries_eq_scalarMin2Fold (base : Nat) (xs : List (BitVec 64)) (st : Min2Pair)
    (hAll : xs.all f64NotNaN = true)
    (hOrd : entryLe st.1 st.2 = true)
    (hv1 : ValidBefore base st.1) (hv2 : ValidBefore base st.2) :
    foldEntries (mkEntries base xs) st = scalarMin2Fold base xs st := by
  induction xs generalizing base st with
  | nil => rfl
  | cons x rest ih =>
    simp only [List.all_cons, Bool.and_eq_true] at hAll
    simp only [mkEntries, foldEntries, List.foldl_cons, scalarMin2Fold]
    have hStep := insertEntry_mkLane_eq_scalarStep base hAll.1 hOrd hv1 hv2
    rw [← hStep]
    have hvNext := insertEntry_preserves_valid (mkLaneEntry_validBefore base hAll.1)
      (validBefore_mono (Nat.le_succ base) hv1) (validBefore_mono (Nat.le_succ base) hv2)
    exact ih (base + 1) _ hAll.2 (insertEntry_ordered _ hOrd) hvNext.1 hvNext.2

private theorem foldEntries_append_inf (es : List LaneEntry) (m : Nat) {p : Min2Pair}
    (hp1 : entryLe p.1 infEntry = true) (hp2 : entryLe p.2 infEntry = true)
    (hes : ∀ e ∈ es, entryLe e infEntry = true) :
    foldEntries (es ++ List.replicate m infEntry) p = foldEntries es p := by
  simp only [foldEntries, List.foldl_append]
  have hb := foldEntries_le_inf es hp1 hp2 hes
  change List.foldl (fun acc e => insertEntry e acc) (foldEntries es p) (List.replicate m infEntry) = foldEntries es p
  induction m with
  | zero => rfl
  | succ k ih =>
    rw [List.replicate_succ, List.foldl_cons, insertEntry_noop_of_both_le hb.1 hb.2]
    exact ih

private theorem paddedLane_le_inf (base : Nat) (c : List (BitVec 64)) (off : Nat)
    (hc : c.all f64NotNaN = true) :
    entryLe (paddedLane base c off) infEntry = true := by
  unfold paddedLane
  cases h : c[off]? with
  | none => dsimp only; decide
  | some x =>
    dsimp only
    have hMem : x ∈ c := List.mem_of_getElem? h
    exact mkLaneEntry_le_inf (base + off) (List.all_eq_true.mp hc x hMem)

private theorem mkEntries_le_inf_all (base : Nat) (c : List (BitVec 64))
    (hc : c.all f64NotNaN = true) :
    ∀ e ∈ mkEntries base c, entryLe e infEntry = true := by
  induction c generalizing base with
  | nil => intro e he; cases he
  | cons y ys ih =>
    simp only [List.all_cons, Bool.and_eq_true] at hc
    intro e he; simp only [mkEntries, List.mem_cons] at he
    rcases he with rfl | hmem
    · exact mkLaneEntry_le_inf base hc.1
    · exact ih (base + 1) hc.2 e hmem

private theorem padded4_eq_mkEntries_pad (base : Nat) (c : List (BitVec 64))
    (hne : c ≠ []) (hlen : c.length ≤ 4) :
    [paddedLane base c 0, paddedLane base c 1, paddedLane base c 2, paddedLane base c 3] =
      mkEntries base c ++ List.replicate (4 - c.length) infEntry := by
  rcases c with _ | ⟨x0, _ | ⟨x1, _ | ⟨x2, _ | ⟨x3, rest⟩⟩⟩⟩
  · exact absurd rfl hne
  · simp [paddedLane, mkEntries]
  · simp [paddedLane, mkEntries]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · rcases rest with _ | ⟨_, _⟩
    · simp [paddedLane, mkEntries, Nat.add_assoc]
    · simp only [List.length_cons] at hlen; omega

private theorem padded8_eq_mkEntries_pad (base : Nat) (c : List (BitVec 64))
    (hne : c ≠ []) (hlen : c.length ≤ 8) :
    [paddedLane base c 0, paddedLane base c 1, paddedLane base c 2, paddedLane base c 3,
     paddedLane base c 4, paddedLane base c 5, paddedLane base c 6, paddedLane base c 7] =
      mkEntries base c ++ List.replicate (8 - c.length) infEntry := by
  rcases c with _ | ⟨x0, _ | ⟨x1, _ | ⟨x2, _ | ⟨x3, _ | ⟨x4, _ | ⟨x5, _ | ⟨x6, _ | ⟨x7, rest⟩⟩⟩⟩⟩⟩⟩⟩
  · exact absurd rfl hne
  · simp [paddedLane, mkEntries]
  · simp [paddedLane, mkEntries]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · simp [paddedLane, mkEntries, Nat.add_assoc]
  · rcases rest with _ | ⟨_, _⟩
    · simp [paddedLane, mkEntries, Nat.add_assoc]
    · simp only [List.length_cons] at hlen; omega

private theorem mkEntries_append (base : Nat) (a b : List (BitVec 64)) :
    mkEntries base (a ++ b) = mkEntries base a ++ mkEntries (base + a.length) b := by
  induction a generalizing base with
  | nil => simp [mkEntries]
  | cons x rest ih =>
    simp only [List.cons_append, mkEntries, List.length_cons, ih (base + 1)]
    have hAdd : base + 1 + rest.length = base + (rest.length + 1) := by omega
    rw [hAdd]

private theorem simdTree4Aux_eq_fold (fuel base : Nat) (xs : List (BitVec 64)) (st : Min2Pair)
    (hFuel : xs.length ≤ fuel)
    (hAll : xs.all f64NotNaN = true)
    (hOrd : entryLe st.1 st.2 = true)
    (hs1 : entryLe st.1 infEntry = true) (hs2 : entryLe st.2 infEntry = true) :
    simdTree4Aux fuel base xs st = foldEntries (mkEntries base xs) st := by
  induction fuel generalizing base xs st with
  | zero =>
    have hNil : xs = [] := List.eq_nil_of_length_eq_zero (Nat.eq_zero_of_le_zero hFuel)
    subst hNil; rfl
  | succ k ih =>
    cases xs with
    | nil => rfl
    | cons x0 rest =>
      let xs' := x0 :: rest
      let c := xs'.take 4
      have hcAll : c.all f64NotNaN = true := by
        rw [List.all_eq_true] at hAll ⊢; intro y hy; exact hAll y (List.mem_of_mem_take hy)
      have hdAll : (xs'.drop 4).all f64NotNaN = true := by
        rw [List.all_eq_true] at hAll ⊢; intro y hy; exact hAll y (List.mem_of_mem_drop hy)
      simp only [simdTree4Aux]
      rw [vec4TreeMin2_eq_fold hOrd hs1 hs2 (paddedLane_le_inf base c 0 hcAll)
          (paddedLane_le_inf base c 1 hcAll) (paddedLane_le_inf base c 2 hcAll)]
      rw [padded4_eq_mkEntries_pad base c (by simp [c, xs']) (List.length_take_le 4 xs')]
      have hcLe := mkEntries_le_inf_all base c hcAll
      rw [foldEntries_append_inf (mkEntries base c) (4 - c.length) hs1 hs2 hcLe]
      have hdFuel : (xs'.drop 4).length ≤ k := by
        simp only [xs', List.length_drop, List.length_cons] at hFuel ⊢; omega
      have hbC := foldEntries_le_inf (mkEntries base c) hs1 hs2 hcLe
      rw [ih (base + 4) (xs'.drop 4) (foldEntries (mkEntries base c) st) hdFuel hdAll
          (foldEntries_ordered _ hOrd) hbC.1 hbC.2]
      let d := xs'.drop 4
      have hBase4 : mkEntries (base + 4) d = mkEntries (base + c.length) d := by
        cases hD : d with
        | nil => rfl
        | cons _ _ =>
          have hNotNil : ¬ xs'.drop 4 = [] := by simp [d] at hD; simp [hD]
          have hc4 : c.length = 4 := by
            simp only [List.drop_eq_nil_iff, Nat.not_le] at hNotNil
            simp only [c, List.length_take]; omega
          rw [hc4]
      change foldEntries (mkEntries (base + 4) d) (foldEntries (mkEntries base c) st) =
        foldEntries (mkEntries base xs') st
      rw [hBase4]
      have hSplit : xs' = c ++ d := (List.take_append_drop 4 xs').symm
      have hRhs : foldEntries (mkEntries base xs') st =
          foldEntries (mkEntries (base + c.length) d) (foldEntries (mkEntries base c) st) := by
        conv => lhs; rw [hSplit]
        rw [mkEntries_append base c d]
        simp [foldEntries, List.foldl_append]
      exact hRhs.symm

/-- Main 4-lane (AVX2) tree + masked `+Inf` tail equivalence to sequential left-to-right `scalarMin2Fold`. -/
theorem simdTree4Reduce_eq_scalarMin2Fold (xs : List (BitVec 64))
    (hAll : xs.all f64NotNaN = true) :
    simdTree4Reduce xs = scalarMin2Fold 0 xs initMin2 := by
  unfold simdTree4Reduce
  rw [simdTree4Aux_eq_fold xs.length 0 xs initMin2 (Nat.le_refl _) hAll initMin2_ordered (by decide) (by decide)]
  exact foldEntries_mkEntries_eq_scalarMin2Fold 0 xs initMin2 hAll initMin2_ordered
    ⟨by decide, Or.inl rfl⟩ ⟨by decide, Or.inl rfl⟩

private theorem simdTree8Aux_eq_fold (fuel base : Nat) (xs : List (BitVec 64)) (st : Min2Pair)
    (hFuel : xs.length ≤ fuel)
    (hAll : xs.all f64NotNaN = true)
    (hOrd : entryLe st.1 st.2 = true)
    (hs1 : entryLe st.1 infEntry = true) (hs2 : entryLe st.2 infEntry = true) :
    simdTree8Aux fuel base xs st = foldEntries (mkEntries base xs) st := by
  induction fuel generalizing base xs st with
  | zero =>
    have hNil : xs = [] := List.eq_nil_of_length_eq_zero (Nat.eq_zero_of_le_zero hFuel)
    subst hNil; rfl
  | succ k ih =>
    cases xs with
    | nil => rfl
    | cons x0 rest =>
      let xs' := x0 :: rest
      let c := xs'.take 8
      have hcAll : c.all f64NotNaN = true := by
        rw [List.all_eq_true] at hAll ⊢; intro y hy; exact hAll y (List.mem_of_mem_take hy)
      have hdAll : (xs'.drop 8).all f64NotNaN = true := by
        rw [List.all_eq_true] at hAll ⊢; intro y hy; exact hAll y (List.mem_of_mem_drop hy)
      simp only [simdTree8Aux]
      rw [vec8TreeMin2_eq_fold hOrd hs1 hs2
          (paddedLane_le_inf base c 0 hcAll) (paddedLane_le_inf base c 1 hcAll)
          (paddedLane_le_inf base c 2 hcAll) (paddedLane_le_inf base c 3 hcAll)
          (paddedLane_le_inf base c 4 hcAll) (paddedLane_le_inf base c 5 hcAll)
          (paddedLane_le_inf base c 6 hcAll)]
      rw [padded8_eq_mkEntries_pad base c (by simp [c, xs']) (List.length_take_le 8 xs')]
      have hcLe := mkEntries_le_inf_all base c hcAll
      rw [foldEntries_append_inf (mkEntries base c) (8 - c.length) hs1 hs2 hcLe]
      have hdFuel : (xs'.drop 8).length ≤ k := by
        simp only [xs', List.length_drop, List.length_cons] at hFuel ⊢; omega
      have hbC := foldEntries_le_inf (mkEntries base c) hs1 hs2 hcLe
      rw [ih (base + 8) (xs'.drop 8) (foldEntries (mkEntries base c) st) hdFuel hdAll
          (foldEntries_ordered _ hOrd) hbC.1 hbC.2]
      let d := xs'.drop 8
      have hBase8 : mkEntries (base + 8) d = mkEntries (base + c.length) d := by
        cases hD : d with
        | nil => rfl
        | cons _ _ =>
          have hNotNil : ¬ xs'.drop 8 = [] := by simp [d] at hD; simp [hD]
          have hc8 : c.length = 8 := by
            simp only [List.drop_eq_nil_iff, Nat.not_le] at hNotNil
            simp only [c, List.length_take]; omega
          rw [hc8]
      change foldEntries (mkEntries (base + 8) d) (foldEntries (mkEntries base c) st) =
        foldEntries (mkEntries base xs') st
      rw [hBase8]
      have hSplit : xs' = c ++ d := (List.take_append_drop 8 xs').symm
      have hRhs : foldEntries (mkEntries base xs') st =
          foldEntries (mkEntries (base + c.length) d) (foldEntries (mkEntries base c) st) := by
        conv => lhs; rw [hSplit]
        rw [mkEntries_append base c d]
        simp [foldEntries, List.foldl_append]
      exact hRhs.symm

/-- Main 8-lane (AVX-512) tree + masked `+Inf` tail equivalence to sequential left-to-right `scalarMin2Fold`. -/
theorem simdTree8Reduce_eq_scalarMin2Fold (xs : List (BitVec 64))
    (hAll : xs.all f64NotNaN = true) :
    simdTree8Reduce xs = scalarMin2Fold 0 xs initMin2 := by
  unfold simdTree8Reduce
  rw [simdTree8Aux_eq_fold xs.length 0 xs initMin2 (Nat.le_refl _) hAll initMin2_ordered (by decide) (by decide)]
  exact foldEntries_mkEntries_eq_scalarMin2Fold 0 xs initMin2 hAll initMin2_ordered
    ⟨by decide, Or.inl rfl⟩ ⟨by decide, Or.inl rfl⟩

private theorem mem_mkEntries_at (base j : Nat) (xs : List (BitVec 64)) (hj : j < xs.length) :
    mkLaneEntry (base + j) xs[j] ∈ mkEntries base xs := by
  induction xs generalizing base j with
  | nil => cases hj
  | cons x rest ih =>
    cases j with
    | zero => simp [mkEntries]
    | succ k =>
      simp only [List.length_cons] at hj
      have hk : k < rest.length := by omega
      simp only [mkEntries, List.getElem_cons_succ, List.mem_cons]
      right
      have hAdd : base + (k + 1) = (base + 1) + k := by omega
      rw [hAdd]
      exact ih (base + 1) k hk

private theorem mem_mkEntries_inv (base : Nat) (xs : List (BitVec 64)) (e : LaneEntry)
    (h : e ∈ mkEntries base xs) :
    ∃ j, ∃ (hj : j < xs.length), e = mkLaneEntry (base + j) xs[j] := by
  induction xs generalizing base with
  | nil => cases h
  | cons x rest ih =>
    simp only [mkEntries, List.mem_cons] at h
    rcases h with rfl | hRest
    · exact ⟨0, by simp, by simp⟩
    · rcases ih (base + 1) hRest with ⟨k, hk, rfl⟩
      refine ⟨k + 1, by simp; omega, ?_⟩
      have hAdd : base + 1 + k = base + (k + 1) := by omega
      rw [hAdd]; rfl

private theorem foldEntries_provenance (es : List LaneEntry) (st : Min2Pair) (pool : List LaneEntry)
    (h1 : st.1 ∈ pool) (h2 : st.2 ∈ pool) :
    (foldEntries es st).1 ∈ pool ++ es ∧ (foldEntries es st).2 ∈ pool ++ es := by
  induction es generalizing st pool with
  | nil => simp [foldEntries, h1, h2]
  | cons x rest ih =>
    change (foldEntries rest (insertEntry x st)).1 ∈ pool ++ (x :: rest) ∧
           (foldEntries rest (insertEntry x st)).2 ∈ pool ++ (x :: rest)
    have hIns : (insertEntry x st).1 ∈ pool ++ [x] ∧ (insertEntry x st).2 ∈ pool ++ [x] := by
      unfold insertEntry
      split
      · exact ⟨by simp, by simp [h1]⟩
      · split
        · exact ⟨by simp [h1], by simp⟩
        · exact ⟨by simp [h1], by simp [h2]⟩
    have hSub := ih (insertEntry x st) (pool ++ [x]) hIns.1 hIns.2
    simpa [List.append_assoc] using hSub

private theorem entryLe_refl (e : LaneEntry) : entryLe e e = true := by
  unfold entryLe; simp

/-- Witness & global-minimality invariants of `scalarMin2Fold` (and hence 4-lane/8-lane `simdTreeReduce`). -/
theorem scalarMin2Fold_witness_and_minimality (xs : List (BitVec 64))
    (hAll : xs.all f64NotNaN = true) :
    let res := scalarMin2Fold 0 xs initMin2
    entryLe res.1 res.2 = true ∧
    (∀ j, (hj : j < xs.length) → f64Le res.1.val xs[j] = true) ∧
    (∀ j1, res.1.idx = some j1 → j1 < xs.length ∧ xs[j1]? = some res.1.val ∧ res.1.val ≠ f64PosInf) := by
  dsimp only
  rw [← foldEntries_mkEntries_eq_scalarMin2Fold 0 xs initMin2 hAll initMin2_ordered
      ⟨by decide, Or.inl rfl⟩ ⟨by decide, Or.inl rfl⟩]
  let res := foldEntries (mkEntries 0 xs) initMin2
  have hOrd : entryLe res.1 res.2 = true := foldEntries_ordered _ initMin2_ordered
  have hProv := foldEntries_provenance (mkEntries 0 xs) initMin2 [infEntry] (by simp [initMin2]) (by simp [initMin2])
  have hRes1NaN : f64NotNaN res.1.val = true := by
    have hMem1 : res.1 ∈ infEntry :: mkEntries 0 xs := by simpa [res] using hProv.1
    simp only [List.mem_cons] at hMem1
    rcases hMem1 with hEq | hIn
    · rw [hEq]; decide
    · rcases mem_mkEntries_inv 0 xs res.1 hIn with ⟨j, hj, hEq⟩
      rw [hEq]; unfold mkLaneEntry
      split
      · decide
      · exact List.all_eq_true.mp hAll xs[j] (List.getElem_mem hj)
  have hStepLe : ∀ (y : LaneEntry) (p : Min2Pair), entryLe (insertEntry y p).1 p.1 = true := by
    intro y p
    unfold insertEntry
    by_cases h : entryLt y p.1 = true
    · simp [h, entryLe]
    · simp only [h, Bool.false_eq_true, ↓reduceIte]
      split <;> exact entryLe_refl p.1
  have hMono : ∀ (ls : List LaneEntry) (p : Min2Pair), entryLe (foldEntries ls p).1 p.1 = true := by
    intro ls
    induction ls with
    | nil => intro p; exact entryLe_refl p.1
    | cons y ys ihl =>
      intro p
      change entryLe (foldEntries ys (insertEntry y p)).1 p.1 = true
      exact entryLe_trans (ihl (insertEntry y p)) (hStepLe y p)
  have hMemLe : ∀ (es : List LaneEntry) (st : Min2Pair) (e : LaneEntry),
      e ∈ es → entryLe (foldEntries es st).1 e = true := by
    intro es
    induction es with
    | nil => intro _ _ h; cases h
    | cons x rest ih =>
      intro st e he
      change entryLe (foldEntries rest (insertEntry x st)).1 e = true
      simp only [List.mem_cons] at he
      rcases he with rfl | hIn
      · exact entryLe_trans (hMono rest (insertEntry e st)) (insertEntry_first_le st (entryLe_refl e))
      · exact ih (insertEntry x st) e hIn
  refine ⟨hOrd, ?_, ?_⟩
  · intro j hj
    have hIn : mkLaneEntry j xs[j] ∈ mkEntries 0 xs := by simpa using mem_mkEntries_at 0 j xs hj
    have hLeEntry : entryLe res.1 (mkLaneEntry j xs[j]) = true := hMemLe (mkEntries 0 xs) initMin2 _ hIn
    have hUle := entryLe_imp_ule_numKey hLeEntry
    have hjNaN : f64NotNaN xs[j] = true := List.all_eq_true.mp hAll xs[j] (List.getElem_mem hj)
    rw [f64Le_iff_ule_numKey res.1.val xs[j] hRes1NaN hjNaN]
    unfold mkLaneEntry at hUle
    by_cases hI : (xs[j] == f64PosInf) = true
    · rw [beq_iff_eq.mp hI]; simpa [hI, infEntry] using hUle
    · simpa [hI] using hUle
  · intro j1 hIdx
    have hMem1 : res.1 ∈ infEntry :: mkEntries 0 xs := by simpa [res] using hProv.1
    simp only [List.mem_cons] at hMem1
    rcases hMem1 with hInf | hIn
    · rw [hInf] at hIdx; cases hIdx
    · rcases mem_mkEntries_inv 0 xs res.1 hIn with ⟨j, hj, hEq⟩
      simp only [Nat.zero_add] at hEq
      unfold mkLaneEntry at hEq
      by_cases hI : (xs[j] == f64PosInf) = true
      · simp only [hI, ↓reduceIte] at hEq
        rw [hEq] at hIdx; cases hIdx
      · simp only [hI, Bool.false_eq_true, ↓reduceIte] at hEq
        have hIdxJ : res.1.idx = some j := by rw [hEq]
        have hjEq : j = j1 := Option.some.inj (hIdxJ.symm.trans hIdx)
        subst hjEq
        have hVal : res.1.val = xs[j] := by rw [hEq]
        refine ⟨hj, ?_, ?_⟩
        · rw [hVal, List.getElem?_eq_getElem hj]
        · rw [hVal]; exact fun heq => absurd (beq_iff_eq.mpr heq) (by simp [hI])

/-- `min2Scan` `GreaterEqual` filter soundness (`internal/simd/kernels_amd64.go:953-1023`): skipping lanes where `f64Ge r u2 = true` is bit-identical to `scalarMin2Step`. -/
def min2FilteredStep (st : Min2Pair) (j : Nat) (r : BitVec 64) : Min2Pair :=
  if f64Ge r st.2.val then st else scalarMin2Step st j r

theorem min2FilteredStep_eq_scalarMin2Step (st : Min2Pair) (j : Nat) (r : BitVec 64) :
    min2FilteredStep st j r = scalarMin2Step st j r := by
  unfold min2FilteredStep scalarMin2Step
  cases f64Ge r st.2.val <;> rfl

/-! ### PC-Relative / RODATA Normalization for Extended Track 5D ELF Verification -/

/-- Canonicalized extended ARM64 instruction form (`Arm64InsnD`) for validating Track 5D ELF binaries
with full fidelity on `FMADDD`/`FMSUBD`/`FNMADDD`/`FNMSUBD` and `FMOVD $imm8`. -/
inductive NormArm64InsnD where
  | exact (insn : Arm64InsnD)
  | blExt (target : String)
  | bExt (target : String)
  | adrpNorm (rd : BitVec 5)
  | adrpLoad64Norm (isFP : Bool) (rt rn : BitVec 5)
  | adrpAdd64Norm (rd rn : BitVec 5)
  deriving DecidableEq, Repr, Inhabited

/-- Normalize a decoded `Arm64InsnD` program given external call targets indexed by `pcIdx`. -/
def normalizeArm64ProgD (insns : Array Arm64InsnD) (extTargets : Nat → Option String) :
    Except String (Array NormArm64InsnD) := do
  let mut out : Array NormArm64InsnD := #[]
  let n := insns.size
  for i in [0:n] do
    let insn := insns[i]!
    let prevIsAdrp : Option (BitVec 5) :=
      if i > 0 then
        match insns[i - 1]! with
        | .base (.adrp rd _) => some rd
        | _ => none
      else none
    match insn with
    | .base (.unknown w) =>
      throw s!"unknown ARM64 instruction word at idx {i}: {w}"
    | .base (.bl imm26) =>
      let tgtInt : Int := (i : Int) + imm26
      if 0 <= tgtInt && tgtInt.toNat < n then
        out := out.push (.exact insn)
      else
        match extTargets i with
        | some sym => out := out.push (.blExt sym)
        | none => throw s!"missing external BL target metadata at idx {i}"
    | .base (.b imm26) =>
      let tgtInt : Int := (i : Int) + imm26
      if 0 <= tgtInt && tgtInt.toNat < n then
        out := out.push (.exact insn)
      else
        match extTargets i with
        | some sym => out := out.push (.bExt sym)
        | none => throw s!"missing external B target metadata at idx {i}"
    | .base (.adrp rd _) =>
      out := out.push (.adrpNorm rd)
    | .base (.ldrStrUImm true isFP 8 _ rt rn) =>
      if prevIsAdrp == some rn then
        out := out.push (.adrpLoad64Norm isFP rt rn)
      else
        out := out.push (.exact insn)
    | .base (.addSubImm true false false rd rn _) =>
      if prevIsAdrp == some rn then
        out := out.push (.adrpAdd64Norm rd rn)
      else
        out := out.push (.exact insn)
    | _ =>
      out := out.push (.exact insn)
  return out

/-- Compare freshly extracted ELF words + external targets against a verified embedded Track 5D routine. -/
def arm64ProgramMatchesD (expectedWords : Array UInt32) (expectedExt : Nat → Option String)
    (liveWords : Array UInt32) (liveExt : Nat → Option String) : Except String Unit := do
  let expNorm ← normalizeArm64ProgD (decodeArm64ProgD expectedWords) expectedExt
  let liveNorm ← normalizeArm64ProgD (decodeArm64ProgD liveWords) liveExt
  if expNorm.size != liveNorm.size then
    throw s!"instruction count mismatch: expected {expNorm.size}, got {liveNorm.size}"
  for i in [0:expNorm.size] do
    if expNorm[i]! != liveNorm[i]! then
      throw s!"normalized ARM64D mismatch at idx {i}: expected {repr (expNorm[i]!)}, got {repr (liveNorm[i]!)}"
  return ()

end GoblinFormal.Track5D
