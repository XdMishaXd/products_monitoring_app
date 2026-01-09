package parsers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"parsing_service/internal/models"

	"github.com/PuerkitoBio/goquery"
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrPriceNotFound   = errors.New("price not found")
	ErrInvalidURL      = errors.New("invalid URL")
)

var (
	numberPattern       = regexp.MustCompile(`(\d+\.?\d*)`)
	exactPricePattern   = regexp.MustCompile(`^\$[\d,]+\.\d{2}$`)
	extractPricePattern = regexp.MustCompile(`\$([\d,]+\.\d{2})`)
)

var (
	priceSelectors = []string{
		"div.x-price-primary[data-testid='x-price-primary'] span.ux-textspans",
		"div[data-testid='x-price-primary'] span.ux-textspans",
		".x-bin-price__content .x-price-primary span.ux-textspans",
		".x-price-primary span.ux-textspans",

		"div.x-price-primary span",
		"[data-testid='x-price-primary'] span",
	}

	oldSelectors = []string{
		".mainPrice span.ux-textspans",
		".mainPrice .ux-textspans",
		"span[itemprop='price']",
		".x-price-approx span",
	}

	metaSelectors = []string{
		"meta[property='og:price:amount']",
		"meta[property='product:price:amount']",
		"meta[name='twitter:data1']",
	}

	replacements = []string{
		"US", "EUR", "GBP", "USD",
		"$", "€", "£", "¥",
		"Price:", "price:", "PRICE:",
		"approximately", "approx", "~",
	}
)

type EbayParser struct {
	client *http.Client
}

func NewEbayParser() *EbayParser {
	return &EbayParser{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  false,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

// * Parse парсит страницу товара на eBay и возвращает цену и наличие
func (p *EbayParser) Parse(ctx context.Context, product models.Product) (*models.ParsedProduct, error) {
	const op = "parsers.EbayParser.Parse"

	if !strings.Contains(product.URL, "ebay.com") {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, ErrInvalidURL)
	}

	// * Формирование запроса к ebay.com
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, product.URL, nil)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to create request: %w", op, err)
	}

	// * Заголовки для имитации браузера
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)

	resp, err := p.client.Do(req)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to fetch page: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ParsedProduct{}, fmt.Errorf("%s: unexpected status code: %d", op, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to read body: %w", op, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to parse HTML: %w", op, err)
	}

	// Парсим цену
	price, err := p.parsePrice(doc)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, err)
	}

	info := &models.ParsedProduct{
		ID:    product.ID,
		Price: price,
	}

	// Парсим наличие
	info.InStock = p.parseAvailability(doc)

	return info, nil
}

// * ParseWithRetry парсит с повторными попытками
func (p *EbayParser) ParseWithRetry(
	ctx context.Context,
	product models.Product,
	maxRetries int,
) (*models.ParsedProduct, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		info, err := p.Parse(ctx, product)
		if err == nil {
			return info, nil
		}

		lastErr = err

		if errors.Is(err, ErrInvalidURL) || errors.Is(err, ErrProductNotFound) {
			return nil, err
		}

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * time.Duration(i+1)):
				// Экспоненциальная задержка
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// * parsePrice извлекает цену из документа
func (p *EbayParser) parsePrice(doc *goquery.Document) (float32, error) {
	var priceText string

	for _, selector := range priceSelectors {
		if priceText != "" {
			break
		}

		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			text := strings.TrimSpace(s.Text())

			if exactPricePattern.MatchString(text) {
				priceText = text
			}
		})
	}

	if priceText == "" {
		for _, selector := range oldSelectors {
			if priceText != "" {
				break
			}

			s := doc.Find(selector).First()
			if s.Length() > 0 {
				text := strings.TrimSpace(s.Text())

				if exactPricePattern.MatchString(text) {
					priceText = text
				} else if match := extractPricePattern.FindString(text); match != "" {
					priceText = match
				}
			}
		}
	}

	if priceText == "" {
		for _, selector := range metaSelectors {
			if content, exists := doc.Find(selector).First().Attr("content"); exists {
				if strings.Contains(content, ".") {
					testPrice := strings.ReplaceAll(content, ",", "")

					if price, err := strconv.ParseFloat(testPrice, 64); err == nil {
						if price >= 0.01 && price <= 1000000 {
							priceText = content
							break
						}
					}
				}
			}
		}
	}

	if priceText == "" {
		return 0, ErrPriceNotFound
	}

	return cleanAndParsePrice(priceText)
}

// * parseAvailability проверяет наличие товара
func (p *EbayParser) parseAvailability(doc *goquery.Document) bool {
	// * Индикаторы отсутствия товара
	unavailableIndicators := []string{
		"out of stock",
		"sold out",
		"unavailable",
		"no longer available",
		"this listing has ended",
		"this item is out of stock",
	}

	pageText := strings.ToLower(doc.Text())

	for _, indicator := range unavailableIndicators {
		if strings.Contains(pageText, indicator) {
			return false
		}
	}

	// Проверка кнопок "Add to cart" и "Buy It Now"
	buyButtonSelectors := []string{
		"a[data-testid='ux-call-to-action']",
		"a.ux-call-to-action",
		".vim-btn-primary",
		"[data-testid='x-atc-cta-btn']",
	}

	for _, selector := range buyButtonSelectors {
		if doc.Find(selector).Length() > 0 {
			return true
		}
	}

	// * Проверка количества доступных товаров
	quantitySelectors := []string{
		"span[data-testid='qtySubTxt'] span.ux-textspans--BOLD",
		".qtyTxt .ux-textspans--BOLD",
		".vi-qty-pur-lnk",
	}

	for _, selector := range quantitySelectors {
		qtyText := doc.Find(selector).First().Text()

		if qtyText != "" {
			qtyText = strings.ToLower(strings.TrimSpace(qtyText))

			if strings.Contains(qtyText, "available") ||
				strings.Contains(qtyText, "left") ||
				strings.Contains(qtyText, "in stock") {

				return true
			}
		}
	}

	// * По умолчанию товар в наличии, если не найдено явных признаков отсутствия
	return true
}

// * cleanAndParsePrice очищает строку цены и конвертирует в float32
func cleanAndParsePrice(priceText string) (float32, error) {
	priceText = strings.TrimSpace(priceText)

	for _, r := range replacements {
		priceText = strings.ReplaceAll(priceText, r, "")
	}

	priceText = strings.ReplaceAll(priceText, " ", "")
	priceText = strings.ReplaceAll(priceText, "\n", "")
	priceText = strings.ReplaceAll(priceText, "\t", "")

	priceText = strings.ReplaceAll(priceText, ",", "")

	priceText = strings.TrimSpace(priceText)

	matches := numberPattern.FindString(priceText)

	if matches == "" {
		return 0, fmt.Errorf("no valid number found in price text: '%s'", priceText)
	}

	priceFloat, err := strconv.ParseFloat(matches, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price '%s': %w", matches, err)
	}

	if priceFloat < 0.01 || priceFloat > 1000000 {
		return 0, fmt.Errorf("price out of reasonable range: %.2f", priceFloat)
	}

	return float32(priceFloat), nil
}
