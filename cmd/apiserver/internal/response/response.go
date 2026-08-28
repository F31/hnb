package response

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Code int

const (
	CodeOK                 Code = 0
	CodeBadRequest         Code = 40000
	CodeValidation         Code = 40001
	CodeUnauthorized       Code = 40100
	CodeTokenExpired       Code = 40101
	CodeForbidden          Code = 40300
	CodeNotFound           Code = 40400
	CodeConflict           Code = 40900
	CodeRateLimit          Code = 42900
	CodeInternal           Code = 50000
	CodeServiceUnavailable Code = 50300
	CodeUpstreamTimeout    Code = 50400
)

var codeToHTTP = map[Code]int{
	CodeOK:                 200,
	CodeBadRequest:         400,
	CodeValidation:         422,
	CodeUnauthorized:       401,
	CodeTokenExpired:       401,
	CodeForbidden:          403,
	CodeNotFound:           404,
	CodeConflict:           409,
	CodeRateLimit:          429,
	CodeInternal:           500,
	CodeServiceUnavailable: 503,
	CodeUpstreamTimeout:    504,
}

type APIResponse struct {
	Code      Code   `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data,omitempty"`
	Total     int    `json:"total,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type ProblemDetail struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	HNBCode  string `json:"hnb_code,omitempty"`
	Code     Code   `json:"code"`
	Message  string `json:"message"`
}

type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type ValidationError struct {
	ProblemDetail
	Errors []FieldError `json:"errors,omitempty"`
}

func Success(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Code:    CodeOK,
		Message: "success",
		Data:    data,
	})
}

func SuccessWithTotal(w http.ResponseWriter, data any, total int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Code:    CodeOK,
		Message: "success",
		Data:    data,
		Total:   total,
	})
}

func Created(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{
		Code:    CodeOK,
		Message: "created",
		Data:    data,
	})
}

func Error(w http.ResponseWriter, httpStatus int, code Code, message string, details ...any) {
	prob := ProblemDetail{
		Type:    fmt.Sprintf("https://hnb.example/problems/%d", code),
		Title:   http.StatusText(httpStatus),
		Status:  httpStatus,
		Detail:  message,
		Code:    code,
		Message: message,
	}

	if len(details) > 0 {
		if fieldErrors, ok := details[0].([]FieldError); ok {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(httpStatus)
			json.NewEncoder(w).Encode(ValidationError{
				ProblemDetail: prob,
				Errors:        fieldErrors,
			})
			return
		}
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(prob)
}

func BadRequest(w http.ResponseWriter, message string, details ...any) {
	Error(w, http.StatusBadRequest, CodeBadRequest, message, details...)
}

func ValidationFailed(w http.ResponseWriter, errors []FieldError) {
	Error(w, http.StatusUnprocessableEntity, CodeValidation, "validation failed", errors)
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, CodeUnauthorized, message)
}

func Forbidden(w http.ResponseWriter, message string) {
	Error(w, http.StatusForbidden, CodeForbidden, message)
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, CodeNotFound, message)
}

func InternalError(w http.ResponseWriter, message string) {
	Error(w, http.StatusInternalServerError, CodeInternal, message)
}

func ServiceUnavailable(w http.ResponseWriter, message string) {
	Error(w, http.StatusServiceUnavailable, CodeServiceUnavailable, message)
}

func init() {
	_ = fmt.Sprintf
}
