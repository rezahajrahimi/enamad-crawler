package enamad

import (
	"strings"
	"sync"
)

type Deduplicator struct {
	mu          sync.Mutex
	seenDomains map[string]bool
	seenPhones  map[string]bool
}

func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		seenDomains: make(map[string]bool),
		seenPhones:  make(map[string]bool),
	}
}

// NormalizeDomain strips protocols, www, and slashes, then lowercases
func NormalizeDomain(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimPrefix(d, "www.")
	d = strings.TrimRight(d, "/")
	return d
}

// NormalizePhone cleans country codes and non-digit chars
func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	var sb strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	num := sb.String()
	if strings.HasPrefix(num, "98") && len(num) > 10 {
		num = "0" + strings.TrimPrefix(num, "98")
	}
	return num
}

// CheckAndAdd checks whether the domain or primary phone is a duplicate.
// If either is already seen, it returns false (duplicate).
// Otherwise, it registers them and returns true.
func (d *Deduplicator) CheckAndAdd(domain string, phone string) (isUnique bool, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	normDomain := NormalizeDomain(domain)
	if normDomain != "" && d.seenDomains[normDomain] {
		return false, "دامنه تکراری: " + normDomain
	}

	normPhone := NormalizePhone(phone)
	if normPhone != "" && d.seenPhones[normPhone] {
		return false, "شماره تلفن تکراری: " + normPhone
	}

	if normDomain != "" {
		d.seenDomains[normDomain] = true
	}
	if normPhone != "" {
		d.seenPhones[normPhone] = true
	}

	return true, ""
}

// FilterSitePhones removes any site numbers that have already been recorded.
func (d *Deduplicator) FilterSitePhones(numbers []string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	var unique []string
	for _, num := range numbers {
		norm := NormalizePhone(num)
		if norm != "" && !d.seenPhones[norm] {
			d.seenPhones[norm] = true
			unique = append(unique, num)
		}
	}
	return unique
}

// Stats returns the total unique domains and unique phones seen
func (d *Deduplicator) Stats() (domainsCount int, phonesCount int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.seenDomains), len(d.seenPhones)
}

