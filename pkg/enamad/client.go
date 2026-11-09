package enamad

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	baseURL        = "https://enamad.ir"
	userAgent      = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	domainListPath = "/DomainListForMIMT"
	refreshCaptURL = "https://enamad.ir/refreshCapt"
	getDomainList  = "https://enamad.ir/getDomainList"
)

var persianDigits = strings.NewReplacer(
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
	"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
)

type Client struct {
	httpClient *http.Client
}

func NewClient(proxyURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	client := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	c := &Client{httpClient: client}
	if err := c.initSession(); err != nil {
		return nil, fmt.Errorf("failed to initialize session: %w", err)
	}

	return c, nil
}

func (c *Client) initSession() error {
	req, err := http.NewRequest(http.MethodGet, baseURL+domainListPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status when initializing session: %d", resp.StatusCode)
	}
	return nil
}

// FetchCaptcha requests a fresh captcha challenge from Enamad.
// It returns the decoded JPEG image bytes and the cptToken string.
func (c *Client) FetchCaptcha() ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodPost, refreshCaptURL, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Referer", baseURL+domainListPath)
	req.Header.Set("Origin", baseURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var captResp CaptchaResponse
	if err := json.NewDecoder(resp.Body).Decode(&captResp); err != nil {
		return nil, "", fmt.Errorf("failed to decode captcha response: %w", err)
	}

	imgBytes, err := base64.StdEncoding.DecodeString(captResp.CaptchaBase64)
	if err != nil {
		return nil, "", fmt.Errorf("failed to base64 decode captcha image: %w", err)
	}

	return imgBytes, captResp.CptToken, nil
}

// GetDomainList fetches a page of domain results using the solved captcha and token.
func (c *Client) GetDomainList(page int, cptToken, captchaValue string, filter SearchFilter) (*DomainResponse, error) {
	form := url.Values{}
	form.Set("s#ms-domain-address", filter.DomainAddress)
	form.Set("s#ms-persian-name", filter.PersianName)
	form.Set("s#ms-product-service-id-enc", filter.ProductServiceIDEnc)
	form.Set("s#mi-rating", filter.Rating)
	form.Set("s#ms-province-id-enc", filter.ProvinceIDEnc)
	form.Set("s#ms-city-id-enc", filter.CityIDEnc)
	form.Set("Capt", captchaValue)
	form.Set("Csearch", "")
	form.Set("page", strconv.Itoa(page))
	form.Set("token", cptToken)
	form.Set("cptToken", cptToken)
	form.Set("checkcapga", "0")

	req, err := http.NewRequest(http.MethodPost, getDomainList, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", baseURL+domainListPath)
	req.Header.Set("Origin", baseURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var domResp DomainResponse
	if err := json.Unmarshal(bodyBytes, &domResp); err != nil {
		return nil, fmt.Errorf("failed to parse domain response (status %d): %w | body: %s", resp.StatusCode, err, string(bodyBytes))
	}

	if domResp.Result != 1 {
		return &domResp, fmt.Errorf("enamad returned error: code=%d msg=%s", domResp.Result, domResp.ResultMsg)
	}

	return &domResp, nil
}

// FetchTrustsealDetails extracts public verified details directly from the Trustseal profile URL.
func (c *Client) FetchTrustsealDetails(id int, code string) (*TrustsealDetails, error) {
	sealURL := fmt.Sprintf("https://trustseal.enamad.ir/?id=%d&code=%s", id, code)
	return c.FetchTrustsealByURL(sealURL)
}

// FetchTrustsealByURL extracts public details given the full Trustseal URL.
func (c *Client) FetchTrustsealByURL(sealURL string) (*TrustsealDetails, error) {
	req, err := http.NewRequest(http.MethodGet, sealURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", baseURL+domainListPath)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trustseal returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	details := &TrustsealDetails{}

	// Iterate over inner row pairs to match exact field labels
	doc.Find("div.row").Each(func(i int, s *goquery.Selection) {
		cols := s.ChildrenFiltered("div")
		if cols.Length() == 2 {
			rawLabel := strings.TrimSpace(cols.Eq(0).Text())
			rawVal := strings.TrimSpace(cols.Eq(1).Text())

			cleanLabel := strings.TrimSpace(strings.ReplaceAll(rawLabel, " ", ""))
			cleanLabel = strings.TrimSuffix(cleanLabel, ":")
			cleanLabel = strings.TrimSuffix(cleanLabel, "：")

			valNormalized := persianDigits.Replace(rawVal)

			switch cleanLabel {
			case "صاحبامتیاز":
				if details.OwnerName == "" {
					details.OwnerName = rawVal
				}
			case "تلفن":
				cleanedPhone := cleanNumber(valNormalized)
				if cleanedPhone != "" {
					details.Phone = cleanedPhone
					if strings.HasPrefix(cleanedPhone, "09") && len(cleanedPhone) == 11 {
						details.IsMobile = true
					}
				}
			case "آدرس":
				if details.Address == "" {
					details.Address = rawVal
				}
			case "پستالكترونیكی", "پستالکترونیکی", "ایمیل":
				if details.Email == "" {
					details.Email = rawVal
				}
			case "ساعتپاسخگویی":
				if details.ResponseHours == "" {
					details.ResponseHours = rawVal
				}
			}
		}
	})

	return details, nil
}

func cleanNumber(raw string) string {
	raw = strings.TrimSpace(raw)
	var sb strings.Builder
	for _, r := range raw {
		if (r >= '0' && r <= '9') || r == '+' {
			sb.WriteRune(r)
		}
	}
	num := sb.String()
	if strings.HasPrefix(num, "+98") {
		num = "0" + strings.TrimPrefix(num, "+98")
	} else if strings.HasPrefix(num, "0098") {
		num = "0" + strings.TrimPrefix(num, "0098")
	}
	return num
}
