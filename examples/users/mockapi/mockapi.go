// Package mockapi implements an in-memory users API that the example
// workflows run against.
package mockapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
)

// New returns a handler serving the users API:
//
//	POST /api/users                 create a user from {"name", "age"} (201)
//	POST /api/users/{id}/followers  add the user {"user_id"} as a follower (204)
//	GET  /api/users/{id}/followers  list the followers (200)
func New() http.Handler {
	s := &store{followers: map[int][]user{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/users", s.createUser)
	mux.HandleFunc("POST /api/users/{id}/followers", s.follow)
	mux.HandleFunc("GET /api/users/{id}/followers", s.listFollowers)
	return mux
}

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type store struct {
	mu        sync.Mutex
	users     []user
	followers map[int][]user
}

func (s *store) createUser(w http.ResponseWriter, r *http.Request) {
	var u user
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil || u.Name == "" || u.Age < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user"})
		return
	}
	s.mu.Lock()
	u.ID = len(s.users) + 1
	s.users = append(s.users, u)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, u)
}

func (s *store) follow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID int `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.find(r.PathValue("id"))
	if !ok || body.UserID < 1 || body.UserID > len(s.users) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	s.followers[id] = append(s.followers[id], s.users[body.UserID-1])
	w.WriteHeader(http.StatusNoContent)
}

func (s *store) listFollowers(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.find(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	list := s.followers[id]
	if list == nil {
		list = []user{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *store) find(v string) (int, bool) {
	id, err := strconv.Atoi(v)
	return id, err == nil && id >= 1 && id <= len(s.users)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
