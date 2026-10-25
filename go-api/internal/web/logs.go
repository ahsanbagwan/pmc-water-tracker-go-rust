package web

import (
	"log/slog"
	"net/http"
)

func logWarn(r *http.Request, msg string, args ...any) { slog.WarnContext(r.Context(), msg, args...) }
func logInfo(r *http.Request, msg string, args ...any) { slog.InfoContext(r.Context(), msg, args...) }
