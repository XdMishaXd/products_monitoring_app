package authMiddlware

import (
	"context"
	"log/slog"
	"net/http"

	resp "main_service/internal/lib/api/response"
	"main_service/internal/lib/jwt"

	"github.com/go-chi/render"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func New(log *slog.Logger, jwtParser *jwt.JWTParser) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const op = "middleware.auth.New"

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				log.Warn("Missing authorization header",
					slog.String("op", op),
					slog.String("path", r.URL.Path),
				)

				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("Missing authorization"))

				return
			}

			userID, err := jwtParser.ParseToken(authHeader)
			if err != nil {
				log.Warn("Invalid token",
					slog.String("op", op),
					slog.String("path", r.URL.Path),
					slog.String("error", err.Error()),
				)

				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, resp.Error("Invalid token"))

				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)

			log.Debug("User authenticated",
				slog.String("op", op),
				slog.Int64("user_id", userID),
				slog.String("path", r.URL.Path),
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
