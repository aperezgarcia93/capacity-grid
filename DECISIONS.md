## What did the spec not tell you?

  - Weeks run from Monday to Sunday, but capacity only counts weekdays. Partial weeks are prorated.
  - After changing someone’s weekly hours, the grid reloads the data from the API. This keeps the calculation in one place.
  - Filtering happens in the browser. The over-capacity total always represents the whole team, even when only some rows are visible.
  - People are ordered by name and then by ID.
  - If there are no people, the API still returns the requested weeks.

  ## What did you notice that looked wrong?

  - Eli Nakamura has 0 weekly hours but 20 assigned hours. I kept the data visible and marked the person as over capacity.
  - 41 of 500 people are over capacity. I checked several records and the result matches the seed data.
  - Enabling the over-capacity filter was instant, but turning it off took about
    half a second. Enabling only removes rows; disabling rebuilds 459 of them.
    I deferred the table render so the control answers immediately (~40ms) and
    the rows arrive behind it, with the table faded while it catches up.
    Virtualising is the real fix, not this.

  ## What did the AI get wrong that you caught?

  The first version looked correct but became slow with all 500 people. Every keystroke in the weekly-hours field re-rendered every row.

  I noticed the typing delay while testing the full dataset. Moving each row into a memoised component fixed it.

  Earlier versions also ended weeks on Friday instead of Sunday and sorted people by ID instead of name.

  Also, the hours field was seeded from the display formatter, which rounds to one decimal. Someone stored at 37.25 showed 37.3, and pressing Save without editing anything wrote 37.3 back — a display format leaking into a write path.

  ## What would you do differently with a week?

  - Virtualise the table so showing all 500 rows is faster.
  - Add frontend tests.
  - Return the updated person’s capacity from PATCH instead of reloading all 500 people.
  - Let the user choose the date range instead of hardcoding it.
  - Add clearer loading and error states, especially when updating weekly hours.
  - Cover exahustively invalid ranges, partial weeks, zero-hour capacity, and assignments crossing week boundaries.
  - Check performance with larger datasets — 500 people works, but I would test the API and grid with more realistic production volumes.
  - Support optimistic updates — update the edited row immediately and roll it back if the request fails.
  - Add database indexes and measure the query — make sure the capacity calculation stays fast with much larger datasets.
  - Add concurrent-edit protection — prevent one user from silently overwriting another user’s change.