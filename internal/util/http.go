package util

import (
	"errors"
	"io"
	"net/http"
)

func ReadBodyLimited(r *http.Request, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("invalid max body size")
	}
	defer r.Body.Close()

	lr := io.LimitReader(r.Body, maxBytes+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, errors.New("payload too large")
	}
	return b, nil
}
