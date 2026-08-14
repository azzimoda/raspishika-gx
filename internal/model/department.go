package model

// Department represents an educational department scraped from the college website.
type Department struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
