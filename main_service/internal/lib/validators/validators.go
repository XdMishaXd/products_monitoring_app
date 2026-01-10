package validators

import (
	"strings"

	"github.com/go-playground/validator/v10"
)

// * RegisterProductURLValidator регистрирует кастомный валидатор для URL товаров
func RegisterProductURLValidator(v *validator.Validate) error {
	return v.RegisterValidation("product_url", validateProductURL)
}

// * validateProductURL проверяет что URL ведет на страницу товара
func validateProductURL(fl validator.FieldLevel) bool {
	urlStr := fl.Field().String()

	if urlStr == "" {
		return false
	}

	lowerURL := strings.ToLower(urlStr)

	// eBay URLs
	if strings.Contains(lowerURL, "ebay.com") {
		return strings.Contains(lowerURL, "/itm/") || strings.Contains(lowerURL, "/p/")
	}

	// Etsy URLs
	if strings.Contains(lowerURL, "etsy.com") {
		return strings.Contains(lowerURL, "/listing/")
	}

	// AliExpress URLs
	if strings.Contains(lowerURL, "aliexpress.com") || strings.Contains(lowerURL, "aliexpress.ru") {
		return strings.Contains(lowerURL, "/item/")
	}

	return false
}
