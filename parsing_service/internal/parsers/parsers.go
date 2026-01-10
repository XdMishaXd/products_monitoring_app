package parsers

import (
	"errors"
	"regexp"
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
	ExtractPatternEbay              = regexp.MustCompile(`\$([\d,]+\.\d{2})`)
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
