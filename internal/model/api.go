package model

// ErrorResponse is the response body returned on errors.
type ErrorResponse struct {
	Error string `json:"error" example:"group not found"`
}
