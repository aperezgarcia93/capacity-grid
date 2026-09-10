# Worklog

Running notes on how this got built — decisions, assumptions, dead ends, and anything
left unfinished. Append as you go; a line or two per entry is right.

---

- 2026-09-10: Capacity ranges are inclusive and grouped into Monday-based weeks; week labels remain Monday-Friday while capacity and allocation are clipped to requested weekdays. This keeps partial weeks comparable without counting weekends.
- 2026-09-10: Capacity rows use explicit person-ID/week ordering so the seeded showcase people remain first and the 500-person response is deterministic.
- 2026-09-10 clarification: Spec review corrected week metadata to Monday-Sunday and person ordering to name then ID; calculations remain clipped to requested weekdays.
- 2026-09-10 clarification: Week metadata is derived from the validated range, not roster rows, so an empty team still returns its requested weeks.
