package main

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type updatedPerson struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	WeeklyHours float64 `json:"weeklyHours"`
}

// handleUpdatePerson serves PATCH /api/people/{id}
//
// It should update the person's weekly hours. What it returns is yours to
// design — the grid is the consumer, and it has state to keep honest.
func (s *server) handleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || personID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id must be a positive integer"})
		return
	}
	decoder := json.NewDecoder(r.Body)
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') || !decoder.More() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain only a numeric weeklyHours property"})
		return
	}
	property, err := decoder.Token()
	if err != nil || property != "weeklyHours" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain only a numeric weeklyHours property"})
		return
	}
	var weeklyHours *float64
	if err := decoder.Decode(&weeklyHours); err != nil || weeklyHours == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain only a numeric weeklyHours property"})
		return
	}
	if decoder.More() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain exactly one property"})
		return
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain one JSON object"})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain one JSON object"})
		return
	}
	if *weeklyHours < 0 || math.IsNaN(*weeklyHours) || math.IsInf(*weeklyHours, 0) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weeklyHours must be a non-negative finite number"})
		return
	}

	var person updatedPerson
	err = s.db.QueryRow(r.Context(), `
		UPDATE people
		SET weekly_hours = $1
		WHERE id = $2
		RETURNING id, name, weekly_hours
	`, *weeklyHours, personID).Scan(&person.ID, &person.Name, &person.WeeklyHours)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "person not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update person"})
		return
	}

	writeJSON(w, http.StatusOK, person)
}
