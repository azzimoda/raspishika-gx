package model

// Department represents an educational department scraped from the college website.
type Department struct {
	ID   string `json:"id" example:"28728"`
	Name string `json:"name" example:"АиЭС"`
	Year int    `json:"year" example:"2026"`
}
