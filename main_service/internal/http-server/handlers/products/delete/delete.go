package deleteProduct

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	resp "main_service/internal/lib/api/response"
	sl "main_service/internal/lib/logger"
	authMiddlware "main_service/internal/middleware/auth"
	"main_service/internal/storage"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
)

type Response struct {
	resp.Response
}

type ProductsRemover interface {
	DeleteProduct(ctx context.Context, productID, userID int64) error
}

// New godoc
// @Summary      Удалить товар из отслеживания
// @Description  ## Описание
// @Description  Удаляет товар из списка отслеживаемых для текущего пользователя.
// @Description
// @Description  ### Процесс удаления:
// @Description  1. Извлечение product_id из query параметра `id`
// @Description  2. Валидация ID (должен быть положительным числом)
// @Description  3. Проверка авторизации (JWT токен)
// @Description  4. Извлечение user_id из токена
// @Description  5. Проверка что товар принадлежит пользователю
// @Description  6. Удаление товара из базы данных
// @Description  7. Остановка мониторинга цены
// @Description
// @Description  ### Что удаляется:
// @Description  - Запись товара из таблицы products
// @Description  - История изменений цен
// @Description  - Настроенные уведомления для этого товара
// @Description  - Планировщик мониторинга цены
// @Description
// @Description  ### Безопасность:
// @Description  - Пользователь может удалить только **свои** товары
// @Description  - Попытка удалить чужой товар вернет 404 (не раскрываем существование)
// @Description  - Требуется валидный JWT токен
// @Description
// @Description  ### Важно:
// @Description  - Удаление **необратимо**
// @Description  - История цен также удаляется
// @Description  - Для восстановления нужно добавить товар заново
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "ID товара для удаления"  minimum(1)  example(42)
// @Success      200  {object}  object{status=string}  "Товар успешно удален из отслеживания"  example({"status": "ok"})
// @Failure      400  {object}  object{status=string,error=string}  "Некорректный ID: отсутствует, не число или отрицательное значение"  example({"status": "error", "error": "Invalid id"})
// @Failure      401  {object}  object{status=string,error=string}  "Требуется авторизация: отсутствует или невалидный JWT токен"  example({"status": "error", "error": "Unauthorized"})
// @Failure      404  {object}  object{status=string,error=string}  "Товар не найден или не принадлежит пользователю"  example({"status": "error", "error": "Product not found"})
// @Failure      500  {object}  object{status=string,error=string}  "Внутренняя ошибка сервера"  example({"status": "error", "error": "Internal error"})
// @Router       /product [delete]
// @x-order      3
func New(
	log *slog.Logger,
	prodOp ProductsRemover,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.products.delete.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		productID := parseProductID(r)
		if productID == -1 {
			log.Error("Invalid id")

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("Invalid id"))

			return
		}

		userID, ok := r.Context().Value(authMiddlware.UserIDKey).(int64)
		if !ok {
			log.Error("User ID not found in context")

			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("Unauthorized"))

			return
		}

		if userID <= 0 {
			log.Error("Invalid user ID", slog.Int64("user_id", userID))

			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("Unauthorized"))

			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		err := prodOp.DeleteProduct(ctx, productID, userID)
		if err != nil {
			if errors.Is(err, storage.ErrProductNotFound) {
				log.Info("Product not found")

				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("Product not found"))

				return
			}

			log.Error("Failed to delete product",
				sl.Err(err),
				slog.Int64("user_id", userID),
				slog.Int64("productID", productID),
			)

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("Internal error"))

			return
		}

		log.Info("Product deleted successfully",
			slog.Int64("product_id", productID),
			slog.Int64("user_id", userID),
		)

		ResponseOK(w, r)
	}
}

func ResponseOK(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, Response{
		Response: resp.OK(),
	})
}

func parseProductID(r *http.Request) int64 {
	productIDStr := r.URL.Query().Get("id")
	if productIDStr == "" {
		return -1
	}

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID < 0 {
		return -1
	}

	return productID
}
