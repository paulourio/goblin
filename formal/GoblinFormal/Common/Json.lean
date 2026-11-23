/-
GoblinFormal.Common.Json: pure Init total recursive-descent JSON parser
and exact extractors for objects, arrays, integers, UInt64, and Rat.
-/

import GoblinFormal.Common.Basic
import Init.Data.String.Basic
import Init.Data.String.Termination

namespace GoblinFormal

/-- JSON value AST with exact decimal representation for numbers. -/
inductive JsonVal where
  | null
  | bool (b : Bool)
  | num (mantissa : Int) (exp10 : Int) (hasDotOrExp : Bool)
  | str (s : String)
  | arr (elems : Array JsonVal)
  | obj (fields : Array (String × JsonVal))
  deriving Repr, Inhabited

private def jsonValBeq : JsonVal → JsonVal → Bool
  | .null, .null => true
  | .bool a, .bool b => a == b
  | .num m1 e1 d1, .num m2 e2 d2 => m1 == m2 && e1 == e2 && d1 == d2
  | .str a, .str b => a == b
  | .arr xs, .arr ys =>
    if xs.size = ys.size then
      arrBeq xs ys 0
    else
      false
  | .obj xs, .obj ys =>
    if xs.size = ys.size then
      objBeq xs ys 0
    else
      false
  | _, _ => false
termination_by a _ => (sizeOf a, 0)
decreasing_by
  all_goals simp_wf
  · left; omega
  · left; omega
where
  arrBeq (xs ys : Array JsonVal) (i : Nat) : Bool :=
    if h : i < xs.size then
      if h2 : i < ys.size then
        jsonValBeq xs[i] ys[i] && arrBeq xs ys (i + 1)
      else
        false
    else
      true
  termination_by (sizeOf xs, xs.size - i)
  decreasing_by
    all_goals simp_wf
    · left; have := Array.sizeOf_getElem xs i h; omega
    · right; omega
  objBeq (xs ys : Array (String × JsonVal)) (i : Nat) : Bool :=
    if h : i < xs.size then
      if h2 : i < ys.size then
        xs[i].1 == ys[i].1 && jsonValBeq xs[i].2 ys[i].2 && objBeq xs ys (i + 1)
      else
        false
    else
      true
  termination_by (sizeOf xs, xs.size - i)
  decreasing_by
    all_goals simp_wf
    · left
      have h1 : sizeOf xs[i] < sizeOf xs := Array.sizeOf_getElem xs i h
      have h2 : sizeOf (xs[i].1, xs[i].2) = 1 + sizeOf xs[i].1 + sizeOf xs[i].2 :=
        Prod.mk.sizeOf_spec xs[i].1 xs[i].2
      rw [Prod.eta] at h2
      change sizeOf xs[i].2 < sizeOf xs
      omega
    · right; omega

instance : BEq JsonVal := ⟨jsonValBeq⟩

namespace JsonParser

private def isWs (c : Char) : Bool :=
  c == ' ' || c == '\t' || c == '\n' || c == '\r'

private def isDigit (c : Char) : Bool :=
  '0' ≤ c && c ≤ '9'

private def digitVal (c : Char) : Nat :=
  c.toNat - '0'.toNat

private def hexVal? (c : Char) : Option Nat :=
  if '0' ≤ c && c ≤ '9' then some (c.toNat - '0'.toNat)
  else if 'a' ≤ c && c ≤ 'f' then some (10 + (c.toNat - 'a'.toNat))
  else if 'A' ≤ c && c ≤ 'F' then some (10 + (c.toNat - 'A'.toNat))
  else none

private theorem pos_lt_of_le_of_lt {s : String} {p q r : s.Pos}
    (h1 : p ≤ q) (h2 : q < r) : p < r := by
  simp only [String.Pos.lt_iff, String.Pos.le_iff, String.Pos.Raw.lt_iff,
    String.Pos.Raw.le_iff] at h1 h2 ⊢
  omega

private theorem pos_lt_of_lt_of_le {s : String} {p q r : s.Pos}
    (h1 : p < q) (h2 : q ≤ r) : p < r := by
  simp only [String.Pos.lt_iff, String.Pos.le_iff, String.Pos.Raw.lt_iff,
    String.Pos.Raw.le_iff] at h1 h2 ⊢
  omega

private def skipWs {s : String} (p : s.Pos) : { q : s.Pos // p ≤ q } :=
  if h : ¬p.IsAtEnd then
    if isWs (p.get h) then
      let res := skipWs (p.next h)
      ⟨res.1, String.Pos.le_trans (String.Pos.le_of_lt p.lt_next) res.2⟩
    else
      ⟨p, String.Pos.le_refl p⟩
  else
    ⟨p, String.Pos.le_refl p⟩
termination_by p

private def readHex4 {s : String} (p : s.Pos) :
    Except String (Nat × { q : s.Pos // p < q }) := do
  let readOne (cur : s.Pos) : Except String (Nat × { q : s.Pos // cur < q }) :=
    if h : ¬cur.IsAtEnd then
      match hexVal? (cur.get h) with
      | some d => .ok (d, ⟨cur.next h, cur.lt_next⟩)
      | none => .error "invalid hex digit in \\u escape"
    else
      .error "truncated \\u escape"
  let (d0, ⟨p1, h1⟩) ← readOne p
  let (d1, ⟨p2, h2⟩) ← readOne p1
  let (d2, ⟨p3, h3⟩) ← readOne p2
  let (d3, ⟨p4, h4⟩) ← readOne p3
  let code := (((d0 * 16 + d1) * 16 + d2) * 16 + d3)
  let h14 : p < p4 := String.Pos.lt_trans h1 (String.Pos.lt_trans h2 (String.Pos.lt_trans h3 h4))
  .ok (code, ⟨p4, h14⟩)

private def parseEscape {s : String} (p : s.Pos) :
    Except String (Char × { q : s.Pos // p < q }) :=
  if h : ¬p.IsAtEnd then
    let c := p.get h
    let p1 := p.next h
    let hp1 : p < p1 := p.lt_next
    match c with
    | '"' => .ok ('"', ⟨p1, hp1⟩)
    | '\\' => .ok ('\\', ⟨p1, hp1⟩)
    | '/' => .ok ('/', ⟨p1, hp1⟩)
    | 'b' => .ok (Char.ofNat 0x08, ⟨p1, hp1⟩)
    | 'f' => .ok (Char.ofNat 0x0c, ⟨p1, hp1⟩)
    | 'n' => .ok ('\n', ⟨p1, hp1⟩)
    | 'r' => .ok ('\r', ⟨p1, hp1⟩)
    | 't' => .ok ('\t', ⟨p1, hp1⟩)
    | 'u' => do
      let (cp, ⟨p2, hp2⟩) ← readHex4 p1
      let hp02 : p < p2 := String.Pos.lt_trans hp1 hp2
      if 0xD800 ≤ cp && cp ≤ 0xDBFF then
        -- High surrogate must be followed by a low surrogate `\uXXXX`.
        if h2 : ¬p2.IsAtEnd then
          if p2.get h2 == '\\' then
            let p3 := p2.next h2
            if h3 : ¬p3.IsAtEnd then
              if p3.get h3 == 'u' then
                let p4 := p3.next h3
                let (lo, ⟨p5, hp5⟩) ← readHex4 p4
                if 0xDC00 ≤ lo && lo ≤ 0xDFFF then
                  let full := 0x10000 + ((cp - 0xD800) <<< 10) + (lo - 0xDC00)
                  let hp05 : p < p5 :=
                    String.Pos.lt_trans hp02
                      (String.Pos.lt_trans p2.lt_next (String.Pos.lt_trans p3.lt_next hp5))
                  .ok (Char.ofNat full, ⟨p5, hp05⟩)
                else
                  .error "invalid low surrogate in \\u escape"
              else
                .error "unpaired high surrogate in \\u escape"
            else
              .error "truncated surrogate pair in \\u escape"
          else
            .error "unpaired high surrogate in \\u escape"
        else
          .error "unpaired high surrogate at end of string"
      else if 0xDC00 ≤ cp && cp ≤ 0xDFFF then
        .error "unexpected low surrogate in \\u escape"
      else
        .ok (Char.ofNat cp, ⟨p2, hp02⟩)
    | _ => .error s!"invalid JSON escape sequence \\{c}"
  else
    .error "unterminated escape sequence in string literal"

private def parseStringBody {s : String} (p : s.Pos) (acc : String) :
    Except String (String × { q : s.Pos // p < q }) :=
  if h : ¬p.IsAtEnd then
    let c := p.get h
    let p1 := p.next h
    let hp1 : p < p1 := p.lt_next
    if c == '"' then
      .ok (acc, ⟨p1, hp1⟩)
    else if c == '\\' then
      match parseEscape p1 with
      | .error msg => .error msg
      | .ok (ch, ⟨p2, hp2⟩) =>
        match parseStringBody p2 (acc.push ch) with
        | .error msg => .error msg
        | .ok (res, ⟨q, hq⟩) =>
          .ok (res, ⟨q, String.Pos.lt_trans hp1 (String.Pos.lt_trans hp2 hq)⟩)
    else if c.toNat < 0x20 then
      .error "unescaped control character in JSON string"
    else
      match parseStringBody p1 (acc.push c) with
      | .error msg => .error msg
      | .ok (res, ⟨q, hq⟩) => .ok (res, ⟨q, String.Pos.lt_trans hp1 hq⟩)
  else
    .error "unterminated JSON string"
termination_by p
decreasing_by
  · exact String.Pos.lt_trans hp1 hp2
  · exact hp1

private def readDigits {s : String} (p : s.Pos) (acc count : Nat) :
    Nat × Nat × { q : s.Pos // p ≤ q } :=
  if h : ¬p.IsAtEnd then
    let c := p.get h
    if isDigit c then
      let p1 := p.next h
      let (acc', count', ⟨q, hq⟩) := readDigits p1 (acc * 10 + digitVal c) (count + 1)
      (acc', count', ⟨q, String.Pos.le_trans (String.Pos.le_of_lt p.lt_next) hq⟩)
    else
      (acc, count, ⟨p, String.Pos.le_refl p⟩)
  else
    (acc, count, ⟨p, String.Pos.le_refl p⟩)
termination_by p

private def parseNumber {s : String} (p : s.Pos) (h : ¬p.IsAtEnd) :
    Except String (JsonVal × { q : s.Pos // p < q }) := do
  let (neg, ⟨pInt, hpInt⟩) : Bool × { q : s.Pos // p ≤ q } :=
    if p.get h == '-' then
      (true, ⟨p.next h, String.Pos.le_of_lt p.lt_next⟩)
    else
      (false, ⟨p, String.Pos.le_refl p⟩)
  if hInt : ¬pInt.IsAtEnd then
    let c0 := pInt.get hInt
    let (intMag, ⟨pAfterInt, hpAfterInt⟩) : Nat × { q : s.Pos // p < q } ←
      if c0 == '0' then
        let p1 := pInt.next hInt
        let hp01 : p < p1 := pos_lt_of_le_of_lt hpInt pInt.lt_next
        if h1 : ¬p1.IsAtEnd then
          if isDigit (p1.get h1) then
            .error "leading zero in JSON number"
          else
            .ok (0, ⟨p1, hp01⟩)
        else
          .ok (0, ⟨p1, hp01⟩)
      else if '1' ≤ c0 && c0 ≤ '9' then
        let p1 := pInt.next hInt
        let (mag, _, ⟨pAfter, hpAfter⟩) := readDigits p1 (digitVal c0) 1
        let hp0 : p < pAfter :=
          pos_lt_of_lt_of_le (pos_lt_of_le_of_lt hpInt pInt.lt_next) hpAfter
        .ok (mag, ⟨pAfter, hp0⟩)
      else
        .error "expected digit in JSON number"
    let (fullMag, fracLen, hasDot, ⟨pAfterFrac, hpAfterFrac⟩) :
        Nat × Nat × Bool × { q : s.Pos // p < q } ←
      if hF : ¬pAfterInt.IsAtEnd then
        if pAfterInt.get hF == '.' then
          let p1 := pAfterInt.next hF
          let (mag, cnt, ⟨p2, hp2⟩) := readDigits p1 intMag 0
          if cnt == 0 then
            .error "expected digit after decimal point"
          else
            let hp02 : p < p2 :=
              String.Pos.lt_trans hpAfterInt (pos_lt_of_lt_of_le pAfterInt.lt_next hp2)
            .ok (mag, cnt, true, ⟨p2, hp02⟩)
        else
          .ok (intMag, 0, false, ⟨pAfterInt, hpAfterInt⟩)
      else
        .ok (intMag, 0, false, ⟨pAfterInt, hpAfterInt⟩)
    let (expVal, hasExp, ⟨pEnd, hpEnd⟩) : Int × Bool × { q : s.Pos // p < q } ←
      if hE : ¬pAfterFrac.IsAtEnd then
        let ce := pAfterFrac.get hE
        if ce == 'e' || ce == 'E' then
          let p1 := pAfterFrac.next hE
          let (expNeg, ⟨p2, hp12⟩) : Bool × { q : s.Pos // p1 ≤ q } :=
            if hSign : ¬p1.IsAtEnd then
              let cs := p1.get hSign
              if cs == '-' then (true, ⟨p1.next hSign, String.Pos.le_of_lt p1.lt_next⟩)
              else if cs == '+' then (false, ⟨p1.next hSign, String.Pos.le_of_lt p1.lt_next⟩)
              else (false, ⟨p1, String.Pos.le_refl p1⟩)
            else
              (false, ⟨p1, String.Pos.le_refl p1⟩)
          let (eMag, eCnt, ⟨p3, hp23⟩) := readDigits p2 0 0
          if eCnt == 0 then
            .error "expected digit in JSON exponent"
          else
            let ev : Int := if expNeg then -(eMag : Int) else (eMag : Int)
            let hp03 : p < p3 :=
              String.Pos.lt_trans hpAfterFrac
                (pos_lt_of_lt_of_le pAfterFrac.lt_next (String.Pos.le_trans hp12 hp23))
            .ok (ev, true, ⟨p3, hp03⟩)
        else
          .ok (0, false, ⟨pAfterFrac, hpAfterFrac⟩)
      else
        .ok (0, false, ⟨pAfterFrac, hpAfterFrac⟩)
    let signedMant : Int := if neg then -(fullMag : Int) else (fullMag : Int)
    let exp10 : Int := expVal - (fracLen : Int)
    .ok (.num signedMant exp10 (hasDot || hasExp), ⟨pEnd, hpEnd⟩)
  else
    .error "unexpected end of input after '-'"

private def expectChar {s : String} (p : s.Pos) (expected : Char) :
    Option { q : s.Pos // p < q } :=
  if h : ¬p.IsAtEnd then
    if p.get h == expected then some ⟨p.next h, p.lt_next⟩ else none
  else
    none

private def parseTrue {s : String} (p : s.Pos) : Option { q : s.Pos // p < q } := do
  let ⟨p1, h1⟩ ← expectChar p 't'
  let ⟨p2, h2⟩ ← expectChar p1 'r'
  let ⟨p3, h3⟩ ← expectChar p2 'u'
  let ⟨p4, h4⟩ ← expectChar p3 'e'
  some ⟨p4, String.Pos.lt_trans h1 (String.Pos.lt_trans h2 (String.Pos.lt_trans h3 h4))⟩

private def parseFalse {s : String} (p : s.Pos) : Option { q : s.Pos // p < q } := do
  let ⟨p1, h1⟩ ← expectChar p 'f'
  let ⟨p2, h2⟩ ← expectChar p1 'a'
  let ⟨p3, h3⟩ ← expectChar p2 'l'
  let ⟨p4, h4⟩ ← expectChar p3 's'
  let ⟨p5, h5⟩ ← expectChar p4 'e'
  some ⟨p5, String.Pos.lt_trans h1 (String.Pos.lt_trans h2 (String.Pos.lt_trans h3 (String.Pos.lt_trans h4 h5)))⟩

private def parseNull {s : String} (p : s.Pos) : Option { q : s.Pos // p < q } := do
  let ⟨p1, h1⟩ ← expectChar p 'n'
  let ⟨p2, h2⟩ ← expectChar p1 'u'
  let ⟨p3, h3⟩ ← expectChar p2 'l'
  let ⟨p4, h4⟩ ← expectChar p3 'l'
  some ⟨p4, String.Pos.lt_trans h1 (String.Pos.lt_trans h2 (String.Pos.lt_trans h3 h4))⟩

private theorem remBytes_lt_of_lt {s : String} {p q : s.Pos} (h : p < q) :
    q.remainingBytes < p.remainingBytes :=
  (String.Pos.lt_iff_remainingBytes_lt p q).mp h

private theorem remBytes_le_of_le {s : String} {p q : s.Pos} (h : p ≤ q) :
    q.remainingBytes ≤ p.remainingBytes :=
  (String.Pos.le_iff_remainingBytes_le p q).mp h

mutual
  private def parseVal (s : String) (p : s.Pos) :
      Except String (JsonVal × { q : s.Pos // p < q }) :=
    let ⟨p1, hp1⟩ := skipWs p
    if h1 : ¬p1.IsAtEnd then
      let c := p1.get h1
      if c == '{' then
        let p2 := p1.next h1
        match parseObjStart s p2 with
        | .error msg => .error msg
        | .ok (v, ⟨q, hq⟩) =>
          .ok (v, ⟨q, pos_lt_of_le_of_lt hp1 (String.Pos.lt_trans p1.lt_next hq)⟩)
      else if c == '[' then
        let p2 := p1.next h1
        match parseArrStart s p2 with
        | .error msg => .error msg
        | .ok (v, ⟨q, hq⟩) =>
          .ok (v, ⟨q, pos_lt_of_le_of_lt hp1 (String.Pos.lt_trans p1.lt_next hq)⟩)
      else if c == '"' then
        match parseStringBody (p1.next h1) "" with
        | .error msg => .error msg
        | .ok (str, ⟨q, hq⟩) =>
          .ok (.str str, ⟨q, pos_lt_of_le_of_lt hp1 (String.Pos.lt_trans p1.lt_next hq)⟩)
      else if c == 't' then
        match parseTrue p1 with
        | some ⟨q, hq⟩ => .ok (.bool true, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
        | none => .error "invalid literal starting with 't'"
      else if c == 'f' then
        match parseFalse p1 with
        | some ⟨q, hq⟩ => .ok (.bool false, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
        | none => .error "invalid literal starting with 'f'"
      else if c == 'n' then
        match parseNull p1 with
        | some ⟨q, hq⟩ => .ok (.null, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
        | none => .error "invalid literal starting with 'n'"
      else if c == '-' || isDigit c then
        match parseNumber p1 h1 with
        | .error msg => .error msg
        | .ok (v, ⟨q, hq⟩) => .ok (v, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
      else
        .error s!"unexpected character '{c}' in JSON"
    else
      .error "unexpected end of JSON input"
  termination_by (p.remainingBytes, (0 : Nat))
  decreasing_by
    all_goals simp_wf
    · left; exact remBytes_lt_of_lt (pos_lt_of_le_of_lt hp1 p1.lt_next)
    · left; exact remBytes_lt_of_lt (pos_lt_of_le_of_lt hp1 p1.lt_next)

  private def parseArrStart (s : String) (p : s.Pos) :
      Except String (JsonVal × { q : s.Pos // p < q }) :=
    let ⟨p1, hp1⟩ := skipWs p
    if h1 : ¬p1.IsAtEnd then
      if p1.get h1 == ']' then
        .ok (.arr #[], ⟨p1.next h1, pos_lt_of_le_of_lt hp1 p1.lt_next⟩)
      else
        match parseArrLoop s p1 #[] with
        | .error msg => .error msg
        | .ok (v, ⟨q, hq⟩) => .ok (v, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
    else
      .error "unterminated JSON array"
  termination_by (p.remainingBytes, (2 : Nat))
  decreasing_by
    simp_wf
    have hle := remBytes_le_of_le hp1
    omega

  private def parseArrLoop (s : String) (p : s.Pos) (acc : Array JsonVal) :
      Except String (JsonVal × { q : s.Pos // p < q }) :=
    match parseVal s p with
    | .error msg => .error msg
    | .ok (elem, ⟨p1, hp1⟩) =>
      let acc' := acc.push elem
      let ⟨p2, hp2⟩ := skipWs p1
      if h2 : ¬p2.IsAtEnd then
        let c := p2.get h2
        if c == ']' then
          let q := p2.next h2
          .ok (.arr acc', ⟨q, String.Pos.lt_trans hp1 (pos_lt_of_le_of_lt hp2 p2.lt_next)⟩)
        else if c == ',' then
          let p3 := p2.next h2
          have hp03 : p < p3 := String.Pos.lt_trans hp1 (pos_lt_of_le_of_lt hp2 p2.lt_next)
          match parseArrLoop s p3 acc' with
          | .error msg => .error msg
          | .ok (v, ⟨q, hq⟩) => .ok (v, ⟨q, String.Pos.lt_trans hp03 hq⟩)
        else
          .error s!"expected ',' or ']' in JSON array, got '{c}'"
      else
        .error "unterminated JSON array"
  termination_by (p.remainingBytes, (1 : Nat))
  decreasing_by
    all_goals simp_wf
    · right; omega
    · left; exact remBytes_lt_of_lt hp03

  private def parseObjStart (s : String) (p : s.Pos) :
      Except String (JsonVal × { q : s.Pos // p < q }) :=
    let ⟨p1, hp1⟩ := skipWs p
    if h1 : ¬p1.IsAtEnd then
      if p1.get h1 == '}' then
        .ok (.obj #[], ⟨p1.next h1, pos_lt_of_le_of_lt hp1 p1.lt_next⟩)
      else
        match parseObjLoop s p1 #[] with
        | .error msg => .error msg
        | .ok (v, ⟨q, hq⟩) => .ok (v, ⟨q, pos_lt_of_le_of_lt hp1 hq⟩)
    else
      .error "unterminated JSON object"
  termination_by (p.remainingBytes, (2 : Nat))
  decreasing_by
    simp_wf
    have hle := remBytes_le_of_le hp1
    omega

  private def parseObjLoop (s : String) (p : s.Pos) (acc : Array (String × JsonVal)) :
      Except String (JsonVal × { q : s.Pos // p < q }) :=
    let ⟨p1, hp1⟩ := skipWs p
    if h1 : ¬p1.IsAtEnd then
      if p1.get h1 == '"' then
        match parseStringBody (p1.next h1) "" with
        | .error msg => .error msg
        | .ok (key, ⟨p2, hp2⟩) =>
          let ⟨p3, hp3⟩ := skipWs p2
          if h3 : ¬p3.IsAtEnd then
            if p3.get h3 == ':' then
              let p4 := p3.next h3
              have hp04 : p < p4 :=
                pos_lt_of_le_of_lt hp1
                  (String.Pos.lt_trans p1.lt_next
                    (String.Pos.lt_trans hp2 (pos_lt_of_le_of_lt hp3 p3.lt_next)))
              match parseVal s p4 with
              | .error msg => .error msg
              | .ok (val, ⟨p5, hp5⟩) =>
                let acc' := acc.push (key, val)
                let ⟨p6, hp6⟩ := skipWs p5
                if h6 : ¬p6.IsAtEnd then
                  let c := p6.get h6
                  if c == '}' then
                    let q := p6.next h6
                    have hp0q : p < q :=
                      String.Pos.lt_trans hp04
                        (String.Pos.lt_trans hp5 (pos_lt_of_le_of_lt hp6 p6.lt_next))
                    .ok (.obj acc', ⟨q, hp0q⟩)
                  else if c == ',' then
                    let p7 := p6.next h6
                    have hp07 : p < p7 :=
                      String.Pos.lt_trans hp04
                        (String.Pos.lt_trans hp5 (pos_lt_of_le_of_lt hp6 p6.lt_next))
                    match parseObjLoop s p7 acc' with
                    | .error msg => .error msg
                    | .ok (res, ⟨q, hq⟩) => .ok (res, ⟨q, String.Pos.lt_trans hp07 hq⟩)
                  else
                    .error s!"expected ',' or '}}' in JSON object, got '{c}'"
                else
                  .error "unterminated JSON object"
            else
              .error s!"expected ':' after JSON object key, got '{p3.get h3}'"
          else
            .error "expected ':' after JSON object key"
      else
        .error s!"expected string key in JSON object, got '{p1.get h1}'"
    else
      .error "unterminated JSON object"
  termination_by (p.remainingBytes, (1 : Nat))
  decreasing_by
    all_goals simp_wf
    · left; exact remBytes_lt_of_lt hp04
    · left; exact remBytes_lt_of_lt hp07
end

end JsonParser

/-- Parses a complete JSON value from `s`, rejecting trailing tokens. -/
def parseJson (s : String) : Except String JsonVal := do
  let (v, ⟨pEnd, _⟩) ← JsonParser.parseVal s s.startPos
  let ⟨pTrail, _⟩ := JsonParser.skipWs pEnd
  if h : ¬pTrail.IsAtEnd then
    .error s!"unexpected trailing character '{pTrail.get h}' in JSON"
  else
    .ok v

namespace JsonVal

private def parseSignedIntStr (raw : String) : Except String Int :=
  if raw.isEmpty then
    .error "empty integer string"
  else
    let chars := raw.toList
    let (neg, digits) :=
      match chars with
      | '-' :: rest => (true, rest)
      | _ => (false, chars)
    if digits.isEmpty then
      .error s!"invalid integer string '{raw}'"
    else
      let parsed := digits.foldl (fun acc c =>
        match acc with
        | none => none
        | some n =>
          if '0' ≤ c && c ≤ '9' then some (n * 10 + (c.toNat - '0'.toNat))
          else none) (some (0 : Nat))
      match parsed with
      | none => .error s!"invalid integer string '{raw}'"
      | some mag => .ok (if neg then -(mag : Int) else (mag : Int))

private def parsePosNatStr (raw : String) : Except String Nat := do
  let n ← parseSignedIntStr raw
  if 0 < n then
    .ok n.toNat
  else
    .error s!"expected positive denominator, got '{raw}'"

/-- Looks up a field by key in a JSON object. -/
def getField? : JsonVal → String → Option JsonVal
  | .obj fields, key =>
    (fields.findRev? (fun kv => kv.1 == key)).map Prod.snd
  | _, _ => none

/-- Looks up a required field by key in a JSON object. -/
def getField (v : JsonVal) (key : String) : Except String JsonVal :=
  match v.getField? key with
  | some fv => .ok fv
  | none =>
    match v with
    | .obj _ => .error s!"missing required JSON field '{key}'"
    | _ => .error s!"expected JSON object when accessing field '{key}'"

/-- Extracts an array from a JSON value. -/
def asArray : JsonVal → Except String (Array JsonVal)
  | .arr xs => .ok xs
  | _ => .error "expected JSON array"

/-- Extracts an object's key-value pairs from a JSON value. -/
def asObj : JsonVal → Except String (Array (String × JsonVal))
  | .obj xs => .ok xs
  | _ => .error "expected JSON object"

/-- Extracts a boolean from a JSON value. -/
def asBool : JsonVal → Except String Bool
  | .bool b => .ok b
  | _ => .error "expected JSON boolean"

/-- Extracts a string from a JSON value. -/
def asString : JsonVal → Except String String
  | .str s => .ok s
  | _ => .error "expected JSON string"

/-- Extracts an exact arbitrary-precision `Int` from a JSON integer number. -/
def asInt : JsonVal → Except String Int
  | .num m 0 false => .ok m
  | .num _ _ _ => .error "expected JSON integer without fraction or exponent"
  | _ => .error "expected JSON integer"

/-- Extracts an arbitrary-precision `Nat` from a JSON non-negative integer. -/
def asNat (v : JsonVal) : Except String Nat := do
  let i ← v.asInt
  if 0 ≤ i then
    .ok i.toNat
  else
    .error s!"expected non-negative integer, got {i}"

/-- Extracts a `UInt64` from a JSON integer in `[0, 2^64 - 1]`. -/
def asUInt64 (v : JsonVal) : Except String UInt64 := do
  let n ← v.asNat
  if n < UInt64.size then
    .ok (UInt64.ofNat n)
  else
    .error s!"integer {n} exceeds UInt64 range"

private def asIntOrStr (v : JsonVal) : Except String Int :=
  match v with
  | .num m 0 false => .ok m
  | .str s => parseSignedIntStr s
  | _ => .error "expected integer or integer string"

private def asPosNatOrStr (v : JsonVal) : Except String Nat := do
  let n ← asIntOrStr v
  if 0 < n then
    .ok n.toNat
  else
    .error s!"expected positive denominator, got {n}"

/-- Extracts an exact `Rat` from `{"num": <int|str>, "den": <pos_nat|str>}`,
a JSON number (integer or decimal/scientific literal), or a `"num/den"` string. -/
def asRat : JsonVal → Except String Rat
  | .num m e _ =>
    if e.natAbs > 10000 then
      .error s!"JSON number exponent {e} exceeds exact conversion bound"
    else if 0 ≤ e then
      .ok ((m * ((10 ^ e.toNat : Nat) : Int) : Int) : Rat)
    else
      .ok (mkRat m (10 ^ (-e).toNat))
  | .str s =>
    match s.splitOn "/" with
    | [numStr] => do
      let n ← parseSignedIntStr numStr
      .ok (n : Rat)
    | [numStr, denStr] => do
      let n ← parseSignedIntStr numStr
      let d ← parsePosNatStr denStr
      .ok (mkRat n d)
    | _ => .error s!"invalid rational string '{s}'"
  | .obj fields =>
    let v := JsonVal.obj fields
    do
      let numVal ← v.getField "num"
      let denVal ← v.getField "den"
      let n ← asIntOrStr numVal
      let d ← asPosNatOrStr denVal
      .ok (mkRat n d)
  | _ => .error "expected rational object, number, or 'num/den' string"

/-- Looks up field `key` and decodes it as `Array JsonVal`. -/
def getArray (v : JsonVal) (key : String) : Except String (Array JsonVal) :=
  v.getField key >>= asArray

/-- Looks up field `key` and decodes it as `Bool`. -/
def getBool (v : JsonVal) (key : String) : Except String Bool :=
  v.getField key >>= asBool

/-- Looks up field `key` and decodes it as `String`. -/
def getString (v : JsonVal) (key : String) : Except String String :=
  v.getField key >>= asString

/-- Looks up field `key` and decodes it as `Int`. -/
def getInt (v : JsonVal) (key : String) : Except String Int :=
  v.getField key >>= asInt

/-- Looks up field `key` and decodes it as `Nat`. -/
def getNat (v : JsonVal) (key : String) : Except String Nat :=
  v.getField key >>= asNat

/-- Looks up field `key` and decodes it as `UInt64`. -/
def getUInt64 (v : JsonVal) (key : String) : Except String UInt64 :=
  v.getField key >>= asUInt64

/-- Looks up field `key` and decodes it as `Rat`. -/
def getRat (v : JsonVal) (key : String) : Except String Rat :=
  v.getField key >>= asRat

private def findDupSmall (fields : Array (String × JsonVal)) : Option String :=
  let rec inner (ki : String) (j : Nat) : Option String :=
    if hj : j < fields.size then
      if fields[j].1 == ki then some ki else inner ki (j + 1)
    else
      none
  termination_by fields.size - j
  let rec outer (i : Nat) : Option String :=
    if hi : i < fields.size then
      match inner fields[i].1 (i + 1) with
      | some dup => some dup
      | none => outer (i + 1)
    else
      none
  termination_by fields.size - i
  outer 0

private def mergeStrings : List String → List String → List String
  | [], ys => ys
  | xs, [] => xs
  | x :: xs, y :: ys =>
    if x ≤ y then x :: mergeStrings xs (y :: ys)
    else y :: mergeStrings (x :: xs) ys
termination_by xs ys => xs.length + ys.length

private def sortStrings (xs : List String) : List String :=
  if h : xs.length ≤ 1 then
    xs
  else
    let half := xs.length / 2
    let left := xs.take half
    let right := xs.drop half
    mergeStrings (sortStrings left) (sortStrings right)
termination_by xs.length
decreasing_by
  all_goals simp_wf
  · omega
  · omega

private def findDupAdjacent : List String → Option String
  | [] => none
  | [_] => none
  | x :: y :: rest =>
    if x == y then some x else findDupAdjacent (y :: rest)

private def findDupInFields (fields : Array (String × JsonVal)) : Option String :=
  if fields.size ≤ 1 then
    none
  else if fields.size ≤ 16 then
    findDupSmall fields
  else
    let keys := (fields.map Prod.fst).toList
    findDupAdjacent (sortStrings keys)

/-- Recursively searches a JSON value for any object containing duplicate keys,
returning the first duplicate key encountered. -/
def findDuplicateKey? : JsonVal → Option String
  | .arr elems => findInArr elems 0
  | .obj fields =>
    match findDupInFields fields with
    | some k => some k
    | none => findInObj fields 0
  | _ => none
termination_by v => (sizeOf v, 0)
decreasing_by
  all_goals simp_wf
  · left; omega
  · left; omega
where
  findInArr (xs : Array JsonVal) (i : Nat) : Option String :=
    if h : i < xs.size then
      match findDuplicateKey? xs[i] with
      | some k => some k
      | none => findInArr xs (i + 1)
    else
      none
  termination_by (sizeOf xs, xs.size - i)
  decreasing_by
    all_goals simp_wf
    · left; have := Array.sizeOf_getElem xs i h; omega
    · right; omega
  findInObj (xs : Array (String × JsonVal)) (i : Nat) : Option String :=
    if h : i < xs.size then
      match findDuplicateKey? xs[i].2 with
      | some k => some k
      | none => findInObj xs (i + 1)
    else
      none
  termination_by (sizeOf xs, xs.size - i)
  decreasing_by
    all_goals simp_wf
    · left
      have h1 : sizeOf xs[i] < sizeOf xs := Array.sizeOf_getElem xs i h
      have h2 : sizeOf (xs[i].1, xs[i].2) = 1 + sizeOf xs[i].1 + sizeOf xs[i].2 :=
        Prod.mk.sizeOf_spec xs[i].1 xs[i].2
      rw [Prod.eta] at h2
      change sizeOf xs[i].2 < sizeOf xs
      omega
    · right; omega

end JsonVal

/-- Parses a complete JSON value from `s`, rejecting trailing tokens and
recursively rejecting any object containing duplicate keys. -/
def parseJsonStrict (s : String) : Except String JsonVal := do
  let v ← parseJson s
  match v.findDuplicateKey? with
  | some dup => .error s!"duplicate JSON object key '{dup}'"
  | none => .ok v

end GoblinFormal
