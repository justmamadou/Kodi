package httpx

import (
	"context"
	"net/http"
)

func withoutCancel(r *http.Request) context.Context { return context.WithoutCancel(r.Context()) }
