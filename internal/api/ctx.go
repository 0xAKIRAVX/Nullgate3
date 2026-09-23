package api

import (
        "context"
        "net/http"
)

func contextWith(r *http.Request, key ctxKey, val int64) context.Context {
        return context.WithValue(r.Context(), key, val)
}
