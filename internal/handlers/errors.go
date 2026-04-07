package handlers

// ErrorDetail represents a single error in the error response
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse represents the standardized error response format
type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

// NewErrorResponse creates a new error response with a single error
func NewErrorResponse(code, message string) *ErrorResponse {
	return &ErrorResponse{
		Errors: []ErrorDetail{
			{Code: code, Message: message},
		},
	}
}

// MultiErrorResponse creates a new error response with multiple errors
func MultiErrorResponse(errors []ErrorDetail) *ErrorResponse {
	return &ErrorResponse{
		Errors: errors,
	}
}
