# Worklog

Running notes on how this got built — decisions, assumptions, dead ends, and anything
left unfinished. Append as you go; a line or two per entry is right.

---

- 2026-09-10: Capacity ranges are inclusive and grouped into Monday-based weeks; week labels remain Monday-Friday while capacity and allocation are clipped to requested weekdays. This keeps partial weeks comparable without counting weekends.
- 2026-09-10: Capacity rows use explicit person-ID/week ordering so the seeded showcase people remain first and the 500-person response is deterministic.
- 2026-09-10 clarification: Spec review corrected week metadata to Monday-Sunday and person ordering to name then ID; calculations remain clipped to requested weekdays.
- 2026-09-10 clarification: Week metadata is derived from the validated range, not roster rows, so an empty team still returns its requested weeks.
- 2026-09-10: Person updates accept one strict `weeklyHours` JSON property, including zero and fractions; the API returns the canonical updated person and the UI can refetch capacity to recompute every affected cell from the database.
- 2026-09-10: The grid renders the full roster in a semantic, horizontally scrollable ledger. Edits refetch the canonical range; failed refreshes retain the prior grid and explicitly warn that it may be stale.
- 2026-09-10: There is no frontend test runner in the starter, so UI TDD uses the running page as its observable red/green seam, backed by TypeScript/build checks and desktop/narrow browser interaction rather than adding a heavy test dependency.
- 2026-09-10: Typing in a weekly-hours field cost ~70ms per keystroke — drafts and save states are one object each on the grid, so every key re-rendered all 500 rows. Extracted a memoised `PersonRow` and made the two handlers stable; measured back down to one frame (~16ms).
- 2026-09-10 unfinished: There is no name filter or over-allocated-only view, so finding one person means scrolling 500 rows. The `41 over capacity` count is the only way to know they exist.
