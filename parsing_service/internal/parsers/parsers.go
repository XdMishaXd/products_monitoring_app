package parsers

import (
	"errors"
	"regexp"
	"strings"
)

const (
	CurrencyUSD = "USD" // Доллар США
	CurrencyEUR = "EUR" // Евро
	CurrencyGBP = "GBP" // Фунт стерлингов
	CurrencyJPY = "JPY" // Японская иена
	CurrencyCNY = "CNY" // Китайский юань
	CurrencyRUB = "RUB" // Российский рубль
	CurrencyMDL = "MDL" // Молдавский лей
)

var CurrencySymbolMap = map[string]string{
	"$":   "USD",
	"€":   "EUR",
	"£":   "GBP",
	"¥":   "JPY",
	"₽":   "RUB",
	"lei": "MDL",
	"L":   "MDL",
}

var (
	CurrencyCodePattern = regexp.MustCompile(`"priceCurrency"\s*:\s*"([A-Z]{3})"`)
	CurrencyTextPattern = regexp.MustCompile(`\b(USD|EUR|GBP|JPY|CNY|RUB|MDL)\b`)
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrPriceNotFound   = errors.New("price not found")
	ErrInvalidURL      = errors.New("invalid URL")
)

// * variables for eBay
var (
	NumberPatternEbay               = regexp.MustCompile(`(\d+\.?\d*)`)
	ExactPricePatternEbay           = regexp.MustCompile(`^\$[\d,]+\.\d{2}$`)
	ExtractPricePatternEbay         = regexp.MustCompile(`\$([\d,]+\.\d{2})`)
	DataTestIDPriceEbay             = regexp.MustCompile(`\$([\d,]+\.\d{2})`)
	AvailabilityPatternEbay         = regexp.MustCompile(`\d+\s*available`)
	MoreThanAvailabilityPatternEbay = regexp.MustCompile(`more than \d+ available`)
)

var (
	PriceSelectorsEbay = []string{
		"div.x-price-primary[data-testid='x-price-primary'] span.ux-textspans",
		"div[data-testid='x-price-primary'] span.ux-textspans",
		".x-bin-price__content .x-price-primary span.ux-textspans",
		".x-price-primary span.ux-textspans",
		"div.x-price-primary span",
		"[data-testid='x-price-primary'] span",
	}

	OldSelectorsEbay = []string{
		".mainPrice span.ux-textspans",
		".mainPrice .ux-textspans",
		"span[itemprop='price']",
		".x-price-approx span",
	}

	MetaSelectorsEbay = []string{
		"meta[property='og:price:amount']",
		"meta[property='product:price:amount']",
		"meta[name='twitter:data1']",
	}

	ReplacementsEbay = []string{
		"US", "EUR", "GBP", "USD",
		"$", "€", "£", "¥",
		"Price:", "price:", "PRICE:",
		"approximately", "approx", "~",
	}

	BuyButtonSelectorsEbay = []string{
		"a[data-testid='ux-call-to-action']",
		"a.ux-call-to-action",
		"button[data-testid='ux-call-to-action']",
		".vim-btn-primary",
		"[data-testid='x-atc-cta-btn']",
		"a[href*='AddToCart']",
		"a[href*='BuyItNow']",
		"button:contains('Add to cart')",
		"a:contains('Buy It Now')",
	}

	QuantityInputSelectorsEbay = []string{
		"input[type='text'][aria-label*='quantity']",
		"select[aria-label*='Quantity']",
		"input[id*='qtyTextBox']",
		".qtyInput",
	}

	PriceContainersEbay = []string{
		".x-price-section",
		"[data-testid='x-price-section']",
		".vim.x-price-section",
	}

	AvailabilityContainersEbay = []string{
		"div[data-testid='x-item-availability']",
		".vim-availability",
		".d-quantity__availability",
		"[class*='availability']",
	}

	CriticalIndicatorsEbay = []string{
		"this listing has ended",
		"no longer available",
		"item is no longer available",
	}

	PositiveIndicatorsEbay = []string{
		"add to cart",
		"buy it now",
		"ships",
	}

	JsonPatternsEbay = []*regexp.Regexp{
		regexp.MustCompile(`"price"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"value"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"lowPrice"\s*:\s*"?([\d,]+\.?\d*)"?`),
	}
)

// * variables for etsy
var (
	NumberPatternEtsy            = regexp.MustCompile(`(\d+\.?\d*)`)
	PricePatternEtsy             = regexp.MustCompile(`[$€£¥₽]\s*[\d,]+\.?\d{0,2}`)
	QuantityAvailablePatternEtsy = regexp.MustCompile(`\d+\s*(?:in stock|available)`)
	OnlyFewLeftPatternEtsy       = regexp.MustCompile(`only \d+ left`)
)

var (
	// Основные селекторы цены Etsy
	PriceSelectorsEtsy = []string{
		"p[data-buy-box-region='price']",
		"div[data-buy-box-region='price']",
		"p.wt-text-title-03",
		".listing-page-price",
		"[data-price-container] p",
		"div.listing-page-price p",
		".wt-text-title-03",
		"p[class*='price']",
	}

	// Старые селекторы (для обратной совместимости)
	OldSelectorsEtsy = []string{
		"span.currency-value",
		"p.text-largest",
		"span[itemprop='price']",
		".price-display",
	}

	// Meta теги для извлечения цены
	MetaSelectorsEtsy = []string{
		"meta[property='og:price:amount']",
		"meta[property='product:price:amount']",
		"meta[name='twitter:data1']",
		"meta[property='etsymarketplace:price']",
	}

	// Паттерны для JSON-LD
	JsonPatternsEtsy = []*regexp.Regexp{
		regexp.MustCompile(`"price"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"value"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"amount"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"lowPrice"\s*:\s*"?([\d,]+\.?\d*)"?`),
		regexp.MustCompile(`"price_usd"\s*:\s*"?([\d,]+\.?\d*)"?`),
	}

	// Замены для очистки цены
	ReplacementsEtsy = []string{
		"US", "EUR", "GBP", "USD", "CAD", "AUD",
		"$", "€", "£", "¥", "₽",
		"Price:", "price:", "PRICE:",
		"From", "from", "FROM",
		"Sale price", "Original price",
	}

	// Селекторы кнопки "Add to cart"
	AddToCartSelectorsEtsy = []string{
		"button[data-buy-box-region='add-to-cart']",
		"button[aria-label*='Add to cart']",
		"button.add-to-cart-button",
		".add-to-cart-btn",
		"button[type='submit'][name='add_to_cart']",
		"button:contains('Add to cart')",
		"button:contains('Add to bag')",
	}

	// Селекторы поля количества
	QuantitySelectorsEtsy = []string{
		"select[name='quantity']",
		"input[name='quantity']",
		"select[aria-label*='Quantity']",
		"input[aria-label*='Quantity']",
		"[data-selector='quantity-select']",
	}

	// Селекторы контейнеров с информацией о наличии
	AvailabilitySelectorsEtsy = []string{
		"[data-buy-box-region='quantity']",
		".wt-display-flex-xs.wt-align-items-center",
		".listing-page-availability",
		"p[class*='availability']",
		".inventory-message",
	}

	// Критические индикаторы отсутствия товара
	CriticalIndicatorsEtsy = []string{
		"this item is no longer available",
		"this listing has been removed",
		"no longer available",
		"item unavailable",
		"listing not found",
	}

	// Позитивные индикаторы наличия товара
	PositiveIndicatorsEtsy = []string{
		"add to cart",
		"add to bag",
		"buy now",
		"in stock",
		"available",
		"ready to ship",
		"made to order",
	}
)

// * extractCurrencyFromText извлекает валюту из текста
func ExtractCurrencyFromText(text string) string {
	// Проверяем символы валют
	for symbol, code := range CurrencySymbolMap {
		if strings.Contains(text, symbol) {
			return code
		}
	}

	// Проверяем текстовые коды валют
	if match := CurrencyTextPattern.FindString(text); match != "" {
		return match
	}

	return ""
}

// * detectCurrencyByDomain определяет валюту по домену или содержимому страницы
func DetectCurrencyByDomain(htmlBody string) string {
	lowerBody := strings.ToLower(htmlBody)

	currencyMentions := map[string]int{
		"usd": 0,
		"eur": 0,
		"gbp": 0,
		"jpy": 0,
		"cny": 0,
		"rub": 0,
		"mdl": 0,
	}

	for currency := range currencyMentions {
		currencyMentions[currency] = strings.Count(lowerBody, currency)
	}

	maxCount := 0
	detectedCurrency := "USD"

	for currency, count := range currencyMentions {
		if count > maxCount {
			maxCount = count
			detectedCurrency = strings.ToUpper(currency)
		}
	}

	return detectedCurrency
}
