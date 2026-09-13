# Proposal 001: Effective Point Counter

Add an internal template-selection cost calculation for template accounting, while keeping existing container display math unchanged.

## Details

- Keep `Trait.AdjustedPoints()` unchanged for editing, display, and character-sheet costs, including existing Alternative Abilities behavior.
- Keep picker validation semantics unchanged: `Count` counts selected children, while `Points` compares their point values.
- Add an internal `TemplateSelectionCost` path for template picker and template-total accounting.
- For a point-based picker, use this precedence:
  1. An explicit container-level point value, when present.
  2. A value calculated from the selected children.
  3. The existing child-sum behavior as a fallback.
- Apply Alternative Abilities reductions exactly once, at the container that owns the Alternative Abilities rule. Parent calculations must not reapply reductions already included in a child calculation.
- Define ownership and precedence for nested picker containers so each selected branch is counted once.

## Why this helps

- Addresses the real issue (template cost tracking accuracy).
- Avoids adding UI/config complexity for a new container type.
- Minimizes regression risk by not changing how containers are presented to users.

## Suggested acceptance criteria

- Displayed container values remain unchanged from current behavior.
- Template picker validation continues to honor `Count` and `Points` semantics.
- Template totals can use `TemplateSelectionCost` and reflect selected-choice costs correctly.
- Explicit container-level values take precedence over selected-child values.
- No double-counting occurs with nested containers or Alternative Abilities plus pickers.
- Regression tests cover:
  - exact and range-based point pickers
  - explicit container-level values
  - selected-choice-derived values
  - ordinary nested containers
  - Alternative Abilities containing pickers
  - pickers containing Alternative Abilities
  - disabled, negative, and fractional-cost children

## Notes

- `TemplateSelectionCost` is primarily for internal template accounting and must not alter values shown in trait tables, editors, character sheets, or existing `AdjustedPoints()` callers.
- The name `TemplateSelectionCost` makes the scope explicit: this is the value used to account for selected template content, not a replacement for displayed trait points.
