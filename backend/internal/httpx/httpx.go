package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

type ErrorBody struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

type HandlerError struct {
	Status  int
	Code    string
	Message string
}

func (e *HandlerError) Error() string { return e.Message }

func Decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func Error(w http.ResponseWriter, r *http.Request, err error) {
	var handlerErr *HandlerError
	if errors.As(err, &handlerErr) {
		JSON(w, handlerErr.Status, ErrorBody{Error: APIError{Code: handlerErr.Code, Message: handlerErr.Message, RequestID: RequestID(r.Context())}})
		return
	}
	slog.Error("request failed", "request_id", RequestID(r.Context()), "error", err)
	JSON(w, http.StatusInternalServerError, ErrorBody{Error: APIError{Code: "internal_error", Message: "服务暂时不可用，请稍后重试。", RequestID: RequestID(r.Context())}})
}
