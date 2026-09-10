## What did the spec not tell you?

  - Weeks run from Monday to Sunday, but capacity only counts weekdays. Partial weeks are prorated.
  - After changing someone’s weekly hours, the grid reloads the data from the API. This keeps the calculation in one place.
  - Filtering happens in the browser. The over-capacity total always represents the whole team, even when only some rows are visible.
  - People are ordered by name and then by ID.
  - If there are no people, the API still returns the requested weeks.

  ## What did you notice that looked wrong?

  - Eli Nakamura has 0 weekly hours but 20 assigned hours. I kept the data visible and marked the person as over capacity.
  - 41 of 500 people are over capacity. I checked several records and the result matches the seed data.

  ## What did the AI get wrong that you caught?

  The first version looked correct but became slow with all 500 people. Every keystroke in the weekly-hours field re-rendered every row.

  I noticed the typing delay while testing the full dataset. Moving each row into a memoised component fixed it.

  Earlier versions also ended weeks on Friday instead of Sunday and sorted people by ID instead of name.

  ## What would you do differently with a week?

  - Virtualise the table so showing all 500 rows is faster.
  - Add frontend tests.
  - Return the updated person’s capacity from PATCH instead of reloading all 500 people.
  - Let the user choose the date range instead of hardcoding it.