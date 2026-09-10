package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type updatePersonTestResponse struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	WeeklyHours float64 `json:"weeklyHours"`
}

func serveUpdatePerson(s *server, id, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/people/{id}", s.handleUpdatePerson)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/people/"+id, strings.NewReader(body))
	mux.ServeHTTP(recorder, request)
	return recorder
}

func newPeopleTestServer(t *testing.T) *server {
	t.Helper()

	s := newEmptyRosterTestServer(t)
	if _, err := s.db.Exec(context.Background(), `
		INSERT INTO people (id, name, weekly_hours)
		VALUES (1, 'Ana Ferreira', 40), (2, 'Bo Lindqvist', 32)
	`); err != nil {
		t.Fatalf("seed people fixture: %v", err)
	}
	return s
}

func TestHandleUpdatePersonRejectsInvalidID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"0", "-1", "not-a-number"} {
		t.Run(id, func(t *testing.T) {
			recorder := serveUpdatePerson(&server{}, id, `{"weeklyHours":40}`)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			var response map[string]string
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response["error"] == "" {
				t.Fatal("error response is empty")
			}
		})
	}
}

func TestHandleUpdatePersonRejectsInvalidBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "missing weeklyHours", body: `{}`},
		{name: "negative weeklyHours", body: `{"weeklyHours":-0.5}`},
		{name: "wrong type", body: `{"weeklyHours":"40"}`},
		{name: "unknown property", body: `{"weeklyHours":40,"note":"full time"}`},
		{name: "duplicate property", body: `{"weeklyHours":40,"weeklyHours":30}`},
		{name: "malformed JSON", body: `{"weeklyHours":`},
		{name: "trailing JSON", body: `{"weeklyHours":40}{}`},
		{name: "non-finite number", body: `{"weeklyHours":1e999}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := serveUpdatePerson(&server{}, "1", test.body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}

			var response map[string]string
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response["error"] == "" {
				t.Fatal("error response is empty")
			}
		})
	}
}

func TestHandleUpdatePersonUpdatesAndReturnsCanonicalPerson(t *testing.T) {
	s := newPeopleTestServer(t)

	for _, weeklyHours := range []string{"17.5", "0"} {
		recorder := serveUpdatePerson(s, "1", `{"weeklyHours":`+weeklyHours+`}`)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		var response updatePersonTestResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.ID != 1 || response.Name != "Ana Ferreira" {
			t.Fatalf("person = %#v, want id 1 and name Ana Ferreira", response)
		}
		var want float64
		if _, err := fmt.Sscan(weeklyHours, &want); err != nil {
			t.Fatalf("parse test weekly hours: %v", err)
		}
		if response.WeeklyHours != want {
			t.Fatalf("weeklyHours = %v, want %v", response.WeeklyHours, want)
		}
	}
}

func TestHandleUpdatePersonReturnsNotFoundForUnknownPerson(t *testing.T) {
	recorder := serveUpdatePerson(newPeopleTestServer(t), "999", `{"weeklyHours":40}`)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var response map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] == "" {
		t.Fatal("error response is empty")
	}
}

func TestHandleUpdatePersonReturnsGenericJSONForDatabaseFailure(t *testing.T) {
	s := newPeopleTestServer(t)
	if _, err := s.db.Exec(context.Background(), `ALTER TABLE people DROP COLUMN weekly_hours`); err != nil {
		t.Fatalf("break people fixture: %v", err)
	}

	recorder := serveUpdatePerson(s, "1", `{"weeklyHours":40}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var response map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["error"] != "update person" {
		t.Fatalf("error = %q, want generic update person", response["error"])
	}
}

func TestHandleUpdatePersonKeepsCapacityGridConsistent(t *testing.T) {
	s := newPeopleTestServer(t)
	if _, err := s.db.Exec(context.Background(), `
		INSERT INTO assignments (person_id, start_date, end_date, hours_per_day)
		VALUES
			(1, '2026-01-05', '2026-01-09', 4),
			(2, '2026-01-05', '2026-01-09', 3)
	`); err != nil {
		t.Fatalf("seed assignment fixture: %v", err)
	}

	loadCapacity := func() capacityTestResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-05&to=2026-01-09", nil)
		s.handleCapacity(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("capacity status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		var response capacityTestResponse
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatalf("decode capacity response: %v", err)
		}
		return response
	}

	before := loadCapacity()
	recorder := serveUpdatePerson(s, "1", `{"weeklyHours":10}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	after := loadCapacity()

	anaBefore := personWithID(t, before, 1)
	anaAfter := personWithID(t, after, 1)
	if anaAfter.WeeklyHours != 10 || anaAfter.Weeks[0].CapacityHours != 10 {
		t.Fatalf("updated Ana = %#v, want weekly and range capacity of 10", anaAfter)
	}
	if anaAfter.Weeks[0].AllocatedHours != anaBefore.Weeks[0].AllocatedHours {
		t.Fatalf("Ana allocation changed from %v to %v", anaBefore.Weeks[0].AllocatedHours, anaAfter.Weeks[0].AllocatedHours)
	}

	boBefore := personWithID(t, before, 2)
	boAfter := personWithID(t, after, 2)
	if boAfter.WeeklyHours != boBefore.WeeklyHours || boAfter.Weeks[0] != boBefore.Weeks[0] {
		t.Fatalf("unrelated person changed from %#v to %#v", boBefore, boAfter)
	}
}
