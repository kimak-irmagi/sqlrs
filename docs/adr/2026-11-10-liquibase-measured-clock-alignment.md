# Bound measured-clock alignment without changing product timing

The retained #89 performance merge gate is partially obsolete under the
[2026-11-14 acceptance](2026-11-14-issue89-performance-acceptance-and-merge.md).
Clock alignment, timing definitions and evidence requirements remain valid.

- Conversation approval: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved after critical and repeat
  review. Separate MC01–MC08 inventory subsequently approved before tests/code.
- Question: native CW07 identifies a strict observer refusal for wall/elapsed
  spans 73672499931/73672499940 ns, differing by −9 ns over 73.67 s. Keep exact
  equality, change product timing, remove checks or bound diagnostic alignment?
- Alternatives: retain refusal; measure API elapsed on calendar time; discard
  consistency checks; or apply the already existing report allowance of 10 μs
  explicitly in a separately selected measured path and disclose its uncertainty.
- Decision: user chose the last. Preserve monotonic API elapsed. A measured-only
  scalar validator permits absolute wall-minus-elapsed delta at most 10000 ns
  with positive spans and correct order, rejecting overflow/larger differences.
  The measured summary validates first, then reuses calendar-axis clipping/union
  with monotonic endpoint readings explicitly removed. Strict validators and
  summaries remain strict. Every successful measured summary includes signed
  ClockDeltaNS, including zero. Node has an explicit required-alignment mode;
  it validates/retains that field and uses the implied calendar span. Default
  parsing keeps old shapes. Keep exact error reasons and do not label wall-only
  error metadata as original monotonic evidence. New tests require approval.
- Rationale: nine nanoseconds of clock alignment cannot explain the seconds-long
  product regression. Retain a bounded, visible observation uncertainty without
  modifying either measured binary, elapsed metric or the P01b criterion.

See the [approved components](../architecture/issue89-performance-diagnostic-design.md#approved-bounded-measured-clock-alignment-strict-helpers-unchanged)
and [proposed tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-mc01mc08-bounded-measured-clock-alignment).
Reviewed the localisation, clock-rejection and unavailable-entry ADRs: none is
obsolete. The clock-rejection decision did not itself authorise a tolerance;
this separately approved measured-only boundary does. Its strict helpers,
error/metadata rules, old evidence and the product speed gate remain in force.

## Approve MC01–MC08 before implementation

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: implement the separately reviewed bounded-alignment tests while
  retaining strict helpers and default report contracts?
- Alternatives: retain the observation blocker; or test scalar limits/overflow,
  actual summaries, required report mode and unchanged native protocol first.
- Decision: user explicitly approved MC01–MC08. Repeated old-test audit found
  no required change; write new tests first. No old assertion/fixture changes
  or product timing/performance-criterion change are authorised.
- Rationale: make bounded observation uncertainty explicit without silently
  converting API elapsed time or presenting a small test as native evidence.

## Verification, 2026-11-10

Native MC08 Actions `42802379397`, observer `81cc3a1`, completes all eight
members/16 passes with original binaries, all workload oracles and exact cleanup.
The eight traced deltas are -10/-9/0 ns, all explicitly retained. See the
[unchanged report and attribution limits](../architecture/issue89-diagnostic-results-2026-11-10.md).
This verifies the observation boundary, not a product optimisation or speed waiver.
