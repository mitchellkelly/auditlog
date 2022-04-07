package mux

import (
	"net/http"
)

type HttpError struct {
	Code        int    `json:"-"`
	Description string `json:"description"`
}

func (self HttpError) Error() string {
	return self.Description
}

func DefaultHttpError(statusCode int) HttpError {
	return HttpError{
		Code:        statusCode,
		Description: http.StatusText(statusCode),
	}
}
