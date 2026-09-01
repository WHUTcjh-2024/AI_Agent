package llm

import (
	"errors"
	"fmt"
)

type ProviderError struct {
	Code       string
	StatusCode int
	Retryable  bool
	Cause      error
}

func (e *ProviderError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("llm provider error: %s", e.Code)
	}
	return fmt.Sprintf("llm provider error: %s: %v", e.Code, e.Cause)
}

func (e *ProviderError) Unwrap() error { return e.Cause }

func errorCode(err error) string {
	var providerError *ProviderError
	if errors.As(err, &providerError) && providerError.Code != "" {
		return providerError.Code
	}
	return "provider_error"
}
