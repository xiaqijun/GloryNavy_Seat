// Package reviewqueue defines the read-only contract between approval sources.
// Source modules retain ownership of SQL, authorization and all mutations.
package reviewqueue

import (
	"context"
	"encoding/json"
	"time"
)

type Filter struct {
	View        string `json:"view"`
	Sort        string `json:"sort"`
	Kind        string `json:"kind"`
	Corporation string `json:"corporation"`
	Account     string `json:"account"`
	Search      string `json:"search"`
	Status      string `json:"status"`
	From        string `json:"from"`
	Until       string `json:"until"`
	Mine        bool   `json:"mine"`
	ID          int64  `json:"-"`
}
type Position struct {
	Time   time.Time `json:"time"`
	Source string    `json:"source"`
	ID     int64     `json:"id"`
}
type Item struct {
	Source      string          `json:"source"`
	ID          int64           `json:"id,string"`
	Version     int64           `json:"version,string"`
	Account     string          `json:"account_id"`
	Applicant   string          `json:"applicant"`
	Corporation string          `json:"corporation_id"`
	Kind        string          `json:"kind"`
	State       string          `json:"state"`
	Status      string          `json:"status"`
	Recipient   string          `json:"recipient"`
	Title       string          `json:"title"`
	Reference   string          `json:"reference"`
	Amount      int64           `json:"amount_minor"`
	Unit        string          `json:"unit"`
	Time        time.Time       `json:"time"`
	Action      string          `json:"action"`
	Actions     []string        `json:"actions"`
	Payload     json.RawMessage `json:"payload"`
}
type Option struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Access struct {
	Allowed      bool     `json:"allowed"`
	Corporations []Option `json:"corporations"`
}
type Page struct {
	Items  []Item           `json:"items"`
	Counts map[string]int64 `json:"counts"`
}
type Source struct {
	ID     string
	Access func(context.Context, string) (Access, error)
	Query  func(context.Context, string, Filter, Position, int) (Page, error)
	People func(context.Context, string) ([]string, error)
}
