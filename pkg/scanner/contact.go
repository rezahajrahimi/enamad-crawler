package scanner

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	// Matches Iranian mobile numbers: 0912..., 0935...
	mobileRegex = regexp.MustCompile(`(?:(?:\+|00)98|0)?(9\d{9})\b`)
	// Matches Iranian landline numbers: 021..., 031..., 051...
	phoneRegex = regexp.MustCompile(`(?:(?:\+|00)98|0)([1-8]\d{1,2}\d{7,8})\b`)

	persianDigits = strings.NewReplacer(
		"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
		"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
		"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
		"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	)
)

type DomainContactScanner struct {
	client *http.Client
}

func NewDomainContactScanner(proxyURL string) *DomainContactScanner {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &DomainContactScanner{
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// NormalizeText converts Persian/Arabic numerals to standard ASCII digits
func NormalizeText(input string) string {
	return persianDigits.Replace(input)
}

// ScanDomain attempts to scrape the homepage and contact page of the given domain
// to extract public phone and mobile numbers that the business displays.
func (s *DomainContactScanner) ScanDomain(domain string) (mobiles []string, phones []string) {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimRight(domain, "/")

	if domain == "" {
		return nil, nil
	}

	targets := []string{
		"https://" + domain,
		"https://" + domain + "/contact",
		"https://" + domain + "/contact-us",
		"https://" + domain + "/about-us",
		"http://" + domain,
	}

	mobileMap := make(map[string]bool)
	phoneMap := make(map[string]bool)

	for _, targetURL := range targets {
		body, err := s.fetchPage(targetURL)
		if err != nil || body == "" {
			continue
		}

		normalizedBody := NormalizeText(body)

		for _, match := range mobileRegex.FindAllStringSubmatch(normalizedBody, -1) {
			if len(match) > 1 {
				num := "0" + match[1]
				mobileMap[num] = true
			}
		}

		for _, match := range phoneRegex.FindAllStringSubmatch(normalizedBody, -1) {
			if len(match) > 1 {
				num := "0" + match[1]
				phoneMap[num] = true
			}
		}

		// If contacts already found, skip probing subsequent paths
		if len(mobileMap) > 0 && len(phoneMap) > 0 {
			break
		}
	}

	for m := range mobileMap {
		mobiles = append(mobiles, m)
	}
	for p := range phoneMap {
		phones = append(phones, p)
	}

	return mobiles, phones
}

func (s *DomainContactScanner) fetchPage(targetURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil
	}

	lr := io.LimitReader(resp.Body, 512*1024) // limit response reading to 512KB
	b, err := io.ReadAll(lr)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
