package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type capacityTestResponse struct {
	From   string               `json:"from"`
	To     string               `json:"to"`
	Weeks  []capacityTestWeek   `json:"weeks"`
	People []capacityTestPerson `json:"people"`
}

type capacityTestWeek struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type capacityTestPerson struct {
	ID          int                      `json:"id"`
	Name        string                   `json:"name"`
	WeeklyHours float64                  `json:"weeklyHours"`
	Weeks       []capacityTestPersonWeek `json:"weeks"`
}

type capacityTestPersonWeek struct {
	WeekStart       string  `json:"weekStart"`
	AllocatedHours  float64 `json:"allocatedHours"`
	CapacityHours   float64 `json:"capacityHours"`
	IsOverallocated bool    `json:"isOverallocated"`
}

func newTestServer(t *testing.T) *server {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	return &server{db: pool}
}

func newEmptyRosterTestServer(t *testing.T) *server {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is required for integration tests")
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	statements := []string{
		`CREATE TEMP TABLE people (id int, name text, weekly_hours numeric)`,
		`CREATE TEMP TABLE assignments (person_id int, start_date date, end_date date, hours_per_day numeric)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("create empty-roster fixture: %v", err)
		}
	}

	return &server{db: pool}
}

func personWithID(t *testing.T, response capacityTestResponse, id int) capacityTestPerson {
	t.Helper()

	for _, person := range response.People {
		if person.ID == id {
			return person
		}
	}
	t.Fatalf("person %d not found", id)
	return capacityTestPerson{}
}

func TestHandleCapacityRequiresDateRange(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity", nil)

	(&server{}).handleCapacity(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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

func TestHandleCapacityRejectsMalformedFromDate(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-1-05&to=2026-01-09", nil)

	(&server{}).handleCapacity(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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

func TestHandleCapacityRejectsInvalidToDate(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-05&to=2026-02-30", nil)

	(&server{}).handleCapacity(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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

func TestHandleCapacityRejectsReversedRange(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-10&to=2026-01-09", nil)

	(&server{}).handleCapacity(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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

func TestHandleCapacityReturnsMondayBasedWeeksForInclusiveRange(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-07&to=2026-01-12", nil)

	newTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.From != "2026-01-07" || response.To != "2026-01-12" {
		t.Fatalf("range = %s..%s, want 2026-01-07..2026-01-12", response.From, response.To)
	}
	wantWeeks := []capacityTestWeek{
		{Start: "2026-01-05", End: "2026-01-11"},
		{Start: "2026-01-12", End: "2026-01-18"},
	}
	if len(response.Weeks) != len(wantWeeks) {
		t.Fatalf("weeks = %#v, want %#v", response.Weeks, wantWeeks)
	}
	for i, want := range wantWeeks {
		if response.Weeks[i] != want {
			t.Errorf("week %d = %#v, want %#v", i, response.Weeks[i], want)
		}
	}
}

func TestHandleCapacityReturnsWeeksForEmptyRoster(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-07&to=2026-01-12", nil)

	newEmptyRosterTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.People) != 0 {
		t.Fatalf("people = %#v, want empty roster", response.People)
	}
	wantWeeks := []capacityTestWeek{
		{Start: "2026-01-05", End: "2026-01-11"},
		{Start: "2026-01-12", End: "2026-01-18"},
	}
	if len(response.Weeks) != len(wantWeeks) {
		t.Fatalf("weeks = %#v, want %#v", response.Weeks, wantWeeks)
	}
	for i, want := range wantWeeks {
		if response.Weeks[i] != want {
			t.Errorf("week %d = %#v, want %#v", i, response.Weeks[i], want)
		}
	}
}

func TestHandleCapacityReturnsDenseStableNameOrderedPersonWeekMatrix(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-05&to=2026-01-09", nil)

	newTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.People) != 500 {
		t.Fatalf("people count = %d, want 500", len(response.People))
	}
	for _, person := range response.People {
		if len(person.Weeks) != 1 {
			t.Fatalf("person %d week count = %d, want 1", person.ID, len(person.Weeks))
		}
		if person.Weeks[0].WeekStart != "2026-01-05" {
			t.Fatalf("person %d week start = %q, want 2026-01-05", person.ID, person.Weeks[0].WeekStart)
		}
	}
	wantFirstPeople := []struct {
		id   int
		name string
	}{
		{id: 1, name: "Ana Ferreira"},
		{id: 2, name: "Bo Lindqvist"},
		{id: 3, name: "Cem Aydin"},
		{id: 4, name: "Dee Okafor"},
		{id: 5, name: "Eli Nakamura"},
		{id: 314, name: "Fatima Al-Rashid"},
	}
	for i, want := range wantFirstPeople {
		got := response.People[i]
		if got.ID != want.id || got.Name != want.name {
			t.Fatalf("person %d = %d %q, want %d %q", i, got.ID, got.Name, want.id, want.name)
		}
	}
	if got := response.People[0].WeeklyHours; got != 40 {
		t.Fatalf("Ana weekly hours = %v, want 40", got)
	}
}

func TestHandleCapacityProratesCapacityForPartialWeeks(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-07&to=2026-01-12", nil)

	newTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	ana := personWithID(t, response, 1)
	if len(ana.Weeks) != 2 {
		t.Fatalf("Ana week count = %d, want 2", len(ana.Weeks))
	}
	if got := ana.Weeks[0].CapacityHours; got != 24 {
		t.Errorf("first partial week capacity = %v, want 24", got)
	}
	if got := ana.Weeks[1].CapacityHours; got != 8 {
		t.Errorf("second partial week capacity = %v, want 8", got)
	}
	cem := personWithID(t, response, 3)
	if got := cem.Weeks[0].CapacityHours; got != 12 {
		t.Errorf("part-time first partial week capacity = %v, want 12", got)
	}
	if got := cem.Weeks[1].CapacityHours; got != 4 {
		t.Errorf("part-time second partial week capacity = %v, want 4", got)
	}
}

func TestHandleCapacityAddsWeekdayAllocationsAndFlagsOvercommitment(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-05&to=2026-01-09", nil)

	newTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	tests := []struct {
		personID       int
		allocatedHours float64
		capacityHours  float64
		overallocated  bool
	}{
		{personID: 1, allocatedHours: 0, capacityHours: 40, overallocated: false},
		{personID: 2, allocatedHours: 32, capacityHours: 40, overallocated: false},
		{personID: 3, allocatedHours: 4, capacityHours: 20, overallocated: false},
		{personID: 4, allocatedHours: 45, capacityHours: 40, overallocated: true},
		{personID: 5, allocatedHours: 20, capacityHours: 0, overallocated: true},
	}
	for _, test := range tests {
		person := personWithID(t, response, test.personID)
		week := person.Weeks[0]
		if week.AllocatedHours != test.allocatedHours {
			t.Errorf("person %d allocated hours = %v, want %v", test.personID, week.AllocatedHours, test.allocatedHours)
		}
		if week.CapacityHours != test.capacityHours {
			t.Errorf("person %d capacity hours = %v, want %v", test.personID, week.CapacityHours, test.capacityHours)
		}
		if week.IsOverallocated != test.overallocated {
			t.Errorf("person %d overallocated = %v, want %v", test.personID, week.IsOverallocated, test.overallocated)
		}
	}
}

func TestHandleCapacityExcludesWeekendDaysFromAllocationAndCapacity(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/capacity?from=2026-01-09&to=2026-01-12", nil)

	newTestServer(t).handleCapacity(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response capacityTestResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	bo := personWithID(t, response, 2)
	if got := bo.Weeks[0].AllocatedHours + bo.Weeks[1].AllocatedHours; got != 16 {
		t.Errorf("allocation across Friday through Monday = %v, want 16", got)
	}
	if got := bo.Weeks[0].CapacityHours + bo.Weeks[1].CapacityHours; got != 16 {
		t.Errorf("capacity across Friday through Monday = %v, want 16", got)
	}
}
