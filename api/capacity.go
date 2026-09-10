package main

import (
	"net/http"
	"time"
)

type capacityResponse struct {
	From   string           `json:"from"`
	To     string           `json:"to"`
	Weeks  []capacityWeek   `json:"weeks"`
	People []personCapacity `json:"people"`
}

type capacityWeek struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type personCapacity struct {
	ID          int                  `json:"id"`
	Name        string               `json:"name"`
	WeeklyHours float64              `json:"weeklyHours"`
	Weeks       []personCapacityWeek `json:"weeks"`
}

type personCapacityWeek struct {
	WeekStart       string  `json:"weekStart"`
	AllocatedHours  float64 `json:"allocatedHours"`
	CapacityHours   float64 `json:"capacityHours"`
	IsOverallocated bool    `json:"isOverallocated"`
}

func weeksInRange(from, to time.Time) []capacityWeek {
	daysSinceMonday := (int(from.Weekday()) + 6) % 7
	weekStart := from.AddDate(0, 0, -daysSinceMonday)
	weeks := make([]capacityWeek, 0)
	for !weekStart.After(to) {
		weeks = append(weeks, capacityWeek{
			Start: weekStart.Format("2006-01-02"),
			End:   weekStart.AddDate(0, 0, 6).Format("2006-01-02"),
		})
		weekStart = weekStart.AddDate(0, 0, 7)
	}
	return weeks
}

// handleCapacity serves GET /api/capacity?from=YYYY-MM-DD&to=YYYY-MM-DD
//
// It returns every person and every Monday-Sunday week touched by the inclusive
// range, with weekday allocation and prorated capacity totals.
func (s *server) handleCapacity(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from and to are required"})
		return
	}
	fromDate, err := time.Parse("2006-01-02", from)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must use YYYY-MM-DD"})
		return
	}
	toDate, err := time.Parse("2006-01-02", to)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to must use YYYY-MM-DD"})
		return
	}
	if fromDate.After(toDate) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be on or before to"})
		return
	}

	rows, err := s.db.Query(r.Context(), `
		WITH range_bounds AS (
			SELECT $1::date AS date_from, $2::date AS date_to
		),
		weeks AS (
			SELECT week_start::date AS week_start
			FROM range_bounds r
			CROSS JOIN LATERAL generate_series(
				date_trunc('week', r.date_from),
				date_trunc('week', r.date_to),
				interval '1 week'
			) AS week_start
		),
		working_days AS (
			SELECT
				work_day::date AS work_day,
				date_trunc('week', work_day)::date AS week_start
			FROM range_bounds r
			CROSS JOIN LATERAL generate_series(
				r.date_from,
				r.date_to,
				interval '1 day'
			) AS work_day
			WHERE extract(isodow FROM work_day) <= 5
		),
		week_capacity AS (
			SELECT
				w.week_start,
				count(wd.work_day) AS working_day_count
			FROM weeks w
			LEFT JOIN working_days wd USING (week_start)
			GROUP BY w.week_start
		),
		assignment_allocations AS (
			SELECT
				p.id AS person_id,
				w.week_start,
				coalesce(sum(a.hours_per_day), 0) AS allocated_hours
			FROM people p
			CROSS JOIN weeks w
			LEFT JOIN working_days wd USING (week_start)
			LEFT JOIN assignments a
				ON a.person_id = p.id
				AND wd.work_day BETWEEN a.start_date AND a.end_date
			GROUP BY p.id, w.week_start
		)
		SELECT
			p.id,
			p.name,
			p.weekly_hours,
			w.week_start,
			aa.allocated_hours,
			p.weekly_hours * wc.working_day_count / 5 AS capacity_hours
		FROM people p
		CROSS JOIN weeks w
		JOIN week_capacity wc USING (week_start)
		JOIN assignment_allocations aa
			ON aa.person_id = p.id
			AND aa.week_start = w.week_start
		ORDER BY p.name, p.id, w.week_start
	`, fromDate, toDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load capacity"})
		return
	}
	defer rows.Close()

	response := capacityResponse{
		From:   from,
		To:     to,
		Weeks:  weeksInRange(fromDate, toDate),
		People: make([]personCapacity, 0),
	}
	for rows.Next() {
		var personID int
		var personName string
		var weeklyHours float64
		var weekStart time.Time
		var allocatedHours float64
		var capacityHours float64
		if err := rows.Scan(
			&personID,
			&personName,
			&weeklyHours,
			&weekStart,
			&allocatedHours,
			&capacityHours,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load capacity"})
			return
		}

		weekStartString := weekStart.Format("2006-01-02")
		if len(response.People) == 0 || response.People[len(response.People)-1].ID != personID {
			response.People = append(response.People, personCapacity{
				ID:          personID,
				Name:        personName,
				WeeklyHours: weeklyHours,
				Weeks:       make([]personCapacityWeek, 0, len(response.Weeks)),
			})
		}
		person := &response.People[len(response.People)-1]
		person.Weeks = append(person.Weeks, personCapacityWeek{
			WeekStart:       weekStartString,
			AllocatedHours:  allocatedHours,
			CapacityHours:   capacityHours,
			IsOverallocated: allocatedHours > capacityHours,
		})
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load capacity"})
		return
	}

	writeJSON(w, http.StatusOK, response)
}
