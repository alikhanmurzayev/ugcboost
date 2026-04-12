package domain

// APIResponse is the unified JSON envelope for all API responses.
// Used by middleware layer (auth, recovery) for error responses.
type APIResponse struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

// APIError represents the error part of the API response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
