package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"enamad-crawler/pkg/enamad"
	"enamad-crawler/pkg/scanner"
	"enamad-crawler/pkg/utils"
)

func main() {
	urlFlag := flag.String("url", "", "Extract single Trustseal profile directly by URL (no captcha required)")
	fileFlag := flag.String("file", "", "Path to text file containing Trustseal URLs or domains (one per line, no captcha required for URLs)")
	pagesFlag := flag.Int("pages", 1, "Number of pages to crawl from DomainList (each page contains up to 30 items)")
	domainFlag := flag.String("domain", "", "Filter list by domain address (e.g. digikala.com)")
	nameFlag := flag.String("name", "", "Filter list by Persian business name")
	scanSitesFlag := flag.Bool("scan-sites", false, "Scan domain websites directly to discover mobile numbers and phones")
	skipTrustsealFlag := flag.Bool("skip-trustseal", false, "Skip scraping Trustseal profile for each item (Trustseal is enabled by default)")
	concurrencyFlag := flag.Int("concurrency", 5, "Number of concurrent workers for fetching profiles & scanning domains")
	proxyFlag := flag.String("proxy", "", "Optional HTTP/SOCKS5 proxy URL (e.g. http://127.0.0.1:8080)")
	outPrefixFlag := flag.String("out", "enamad_results", "Output file prefix (will create .json and .csv)")
	loadExistingFlag := flag.Bool("dedup-existing", true, "Load existing output file to avoid collecting previously seen domains/phones")

	flag.Parse()

	client, err := enamad.NewClient(*proxyFlag)
	if err != nil {
		log.Fatalf("خطا در ایجاد کلاینت اینماد: %v", err)
	}

	dedup := enamad.NewDeduplicator()

	// Load existing output to prevent duplicates across multiple runs
	if *loadExistingFlag {
		loadExistingData(dedup, *outPrefixFlag)
	}

	// 1. Direct Single URL Extraction mode (no captcha required!)
	if *urlFlag != "" {
		fmt.Println("=========================================================")
		fmt.Println("       استخراج مستقیم شناسنامه نماد (بدون نیاز به کپچا)   ")
		fmt.Println("=========================================================")
		fmt.Printf("در حال دریافت اطلاعات از: %s\n", *urlFlag)

		details, err := client.FetchTrustsealByURL(*urlFlag)
		if err != nil {
			log.Fatalf("خطا در دریافت اطلاعات شناسنامه نماد: %v", err)
		}

		timeInfo := utils.NowExtractionInfo()
		phoneType := "تلفن ثابت"
		if details.IsMobile {
			phoneType = "موبایل"
		}

		fmt.Println("\nاطلاعات استخراج شده از اینماد:")
		fmt.Printf("• نام صاحب امتیاز: %s\n", details.OwnerName)
		fmt.Printf("• شماره تماس:      %s (%s)\n", details.Phone, phoneType)
		fmt.Printf("• ایمیل:            %s\n", details.Email)
		fmt.Printf("• ساعات پاسخگویی:  %s\n", details.ResponseHours)
		fmt.Printf("• آدرس پستی:        %s\n", details.Address)
		fmt.Printf("• روز و تاریخ:     %s (%s) - ساعت %s\n", timeInfo.DayOfWeek, timeInfo.ShamsiDate, time.Now().Format("15:04:05"))

		bInfo := enamad.BusinessInfo{
			TrustsealURL:   *urlFlag,
			Trustseal:      details,
			ExtractionDate: timeInfo.ShamsiDate,
			ExtractionDay:  timeInfo.DayOfWeek,
			ExtractionTime: time.Now().Format("15:04:05"),
			ExtractedAt:    timeInfo.Timestamp,
		}

		outJSON, _ := json.MarshalIndent(bInfo, "", "  ")
		fmt.Printf("\nخروجی JSON:\n%s\n", string(outJSON))
		return
	}

	// 2. Batch File Mode: Process list of Trustseal URLs from file concurrently (no captcha required!)
	if *fileFlag != "" {
		fmt.Println("=========================================================")
		fmt.Println("       استخراج دسته‌ای از فایل ورودی (بدون نیاز به کپچا)   ")
		fmt.Println("=========================================================")

		lines, err := readLines(*fileFlag)
		if err != nil {
			log.Fatalf("خطا در خواندن فایل: %v", err)
		}

		fmt.Printf("تعداد لینک‌های یافت شده در فایل: %d\n", len(lines))
		results := processBatchURLs(client, lines, *concurrencyFlag, *scanSitesFlag, *proxyFlag, dedup)
		saveOutputs(results, *outPrefixFlag)
		return
	}

	// 3. Full Crawling Mode (List -> Solve Captcha -> Extract each Trustseal)
	nowTimeInfo := utils.NowExtractionInfo()
	fmt.Println("=========================================================")
	fmt.Println("           Enamad Crawler (Domain & Contact Extractor)   ")
	fmt.Println("=========================================================")
	fmt.Printf("زمان شروع فرایند: %s (%s) - ساعت %s\n", nowTimeInfo.DayOfWeek, nowTimeInfo.ShamsiDate, time.Now().Format("15:04:05"))
	fmt.Println("ویژگی‌های فعال:")
	fmt.Println("✓ ثبت روز هفته و تاریخ شمسی/میلادی استخراج در خروجی")
	fmt.Println("✓ فیلتر خودکار موارد تکراری (حذف دامنه‌ها یا شماره تماس‌های مشابه)")
	fmt.Println("✓ ورود خودکار به لینک شناسنامه هر کسب‌وکار و استخراج شماره و نام صاحب امتیاز")
	if *scanSitesFlag {
		fmt.Println("✓ اسکن تکمیلی صفحات داخلی وب‌سایت جهت یافتن شماره موبایل‌های اعلام‌شده")
	}
	fmt.Println("---------------------------------------------------------")

	fmt.Println("در حال دریافت چالش کپچا از سرور اینماد برای واکشی لیست دامنه‌ها...")
	imgBytes, cptToken, err := client.FetchCaptcha()
	if err != nil {
		log.Fatalf("خطا در دریافت تصویر کپچا: %v", err)
	}

	captchaFilePath := "captcha.jpg"
	if err := os.WriteFile(captchaFilePath, imgBytes, 0644); err != nil {
		log.Fatalf("خطا در ذخیره تصویر کپچا: %v", err)
	}

	fmt.Printf("تصویر کد امنیتی در فایل '%s' ذخیره شد.\n", captchaFilePath)
	openImageInViewer(captchaFilePath)

	fmt.Print("لطفاً کد امنیتی مشاهده شده در تصویر را وارد کرده و Enter را بزنید: ")
	reader := bufio.NewReader(os.Stdin)
	captchaInput, _ := reader.ReadString('\n')
	captchaInput = strings.TrimSpace(captchaInput)

	if captchaInput == "" {
		log.Fatalf("کد امنیتی نمی‌تواند خالی باشد.")
	}

	filter := enamad.SearchFilter{
		DomainAddress: *domainFlag,
		PersianName:   *nameFlag,
	}

	contactScanner := scanner.NewDomainContactScanner(*proxyFlag)
	var allBusinesses []enamad.BusinessInfo
	skippedDuplicates := 0

	for page := 1; page <= *pagesFlag; page++ {
		fmt.Printf("\n--- در حال واکشی صفحه %d از %d ---\n", page, *pagesFlag)
		resp, err := client.GetDomainList(page, cptToken, captchaInput, filter)
		if err != nil {
			log.Printf("خطا در واکشی صفحه %d: %v", page, err)
			break
		}

		fmt.Printf("تعداد کل نتایج یافت شده: %d | آیتم‌های این صفحه: %d\n", resp.Count, len(resp.ApplicantDomainsList))
		if len(resp.ApplicantDomainsList) == 0 {
			break
		}

		pageItems := make([]enamad.BusinessInfo, len(resp.ApplicantDomainsList))
		for i, item := range resp.ApplicantDomainsList {
			currentTime := utils.NowExtractionInfo()
			trustsealURL := fmt.Sprintf("https://trustseal.enamad.ir/?id=%d&code=%s", item.ID, item.Code)
			pageItems[i] = enamad.BusinessInfo{
				DomainAddress:  item.DomainAddress,
				PersianName:    item.PersianName,
				Province:       item.Province,
				City:           item.City,
				Rating:         item.Rating,
				TrustsealURL:   trustsealURL,
				ExtractionDate: currentTime.ShamsiDate,
				ExtractionDay:  currentTime.DayOfWeek,
				ExtractionTime: time.Now().Format("15:04:05"),
				ExtractedAt:    currentTime.Timestamp,
			}
		}

		// Perform deep enrichment (trustseal & site scanning) concurrently
		if !*skipTrustsealFlag || *scanSitesFlag {
			fmt.Printf("در حال استخراج اطلاعات شناسنامه نماد (تلفن، موبایل، آدرس) با %d ورکر هم‌روند...\n", *concurrencyFlag)
			var wg sync.WaitGroup
			sem := make(chan struct{}, *concurrencyFlag)

			for i := range pageItems {
				wg.Add(1)
				go func(idx int, raw enamad.ApplicantDomain) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					// Fetch trustseal info
					if !*skipTrustsealFlag {
						details, err := client.FetchTrustsealDetails(raw.ID, raw.Code)
						if err == nil {
							pageItems[idx].Trustseal = details
						}
					}

					// Scan business website for phone & mobile contacts if enabled
					if *scanSitesFlag {
						mobiles, phones := contactScanner.ScanDomain(raw.DomainAddress)
						pageItems[idx].SiteMobiles = dedup.FilterSitePhones(mobiles)
						pageItems[idx].SitePhones = dedup.FilterSitePhones(phones)
					}
				}(i, resp.ApplicantDomainsList[i])
			}
			wg.Wait()
		}

		// Apply Deduplication filtering
		for idx, item := range pageItems {
			primaryPhone := ""
			if item.Trustseal != nil {
				primaryPhone = item.Trustseal.Phone
			}

			// Check domain & phone uniqueness
			isUnique, reason := dedup.CheckAndAdd(item.DomainAddress, primaryPhone)
			if !isUnique {
				skippedDuplicates++
				fmt.Printf("[%d/%d] [تکراری - رد شد] دامنه: %-20s (%s)\n", idx+1, len(pageItems), item.DomainAddress, reason)
				continue
			}

			phoneStr := "-"
			if item.Trustseal != nil && item.Trustseal.Phone != "" {
				if item.Trustseal.IsMobile {
					phoneStr = item.Trustseal.Phone + " (موبایل)"
				} else {
					phoneStr = item.Trustseal.Phone + " (ثابت)"
				}
			}
			fmt.Printf("[%d/%d] [ثبت شد] دامنه: %-25s | نام: %-20s | تلفن اینماد: %s\n", idx+1, len(pageItems), item.DomainAddress, item.PersianName, phoneStr)
			if len(item.SiteMobiles) > 0 || len(item.SitePhones) > 0 {
				fmt.Printf("            -> تماس‌های داخل سایت: موبایل=%v | تلفن=%v\n", item.SiteMobiles, item.SitePhones)
			}
			allBusinesses = append(allBusinesses, item)
		}

		// Delay between page requests
		if page < *pagesFlag {
			time.Sleep(1 * time.Second)
		}
	}

	totalDomains, totalPhones := dedup.Stats()
	fmt.Printf("\nخلاصه وضعیت یکتاسازی:\n")
	fmt.Printf("• کسب‌وکارهای جدید ثبت‌شده: %d\n", len(allBusinesses))
	fmt.Printf("• موارد تکراری رد شده:      %d\n", skippedDuplicates)
	fmt.Printf("• مجموع دامنه‌های یکتا:     %d\n", totalDomains)
	fmt.Printf("• مجموع شماره‌های یکتا:     %d\n", totalPhones)

	if len(allBusinesses) == 0 {
		fmt.Println("\nهیچ اطلاعات جدیدی برای ذخیره وجود ندارد.")
		return
	}

	saveOutputs(allBusinesses, *outPrefixFlag)
}

func processBatchURLs(client *enamad.Client, urls []string, concurrency int, scanSites bool, proxy string, dedup *enamad.Deduplicator) []enamad.BusinessInfo {
	var results []enamad.BusinessInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	contactScanner := scanner.NewDomainContactScanner(proxy)
	sem := make(chan struct{}, concurrency)

	for i, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, targetURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			details, err := client.FetchTrustsealByURL(targetURL)
			if err != nil {
				log.Printf("[%d/%d] خطا در واکشی %s: %v", idx+1, len(urls), targetURL, err)
				return
			}

			// Deduplicate by URL/Domain and Phone
			phone := details.Phone
			isUnique, reason := dedup.CheckAndAdd(targetURL, phone)
			if !isUnique {
				fmt.Printf("[%d/%d] [تکراری - رد شد] %s (%s)\n", idx+1, len(urls), targetURL, reason)
				return
			}

			timeInfo := utils.NowExtractionInfo()
			bInfo := enamad.BusinessInfo{
				TrustsealURL:   targetURL,
				Trustseal:      details,
				ExtractionDate: timeInfo.ShamsiDate,
				ExtractionDay:  timeInfo.DayOfWeek,
				ExtractionTime: time.Now().Format("15:04:05"),
				ExtractedAt:    timeInfo.Timestamp,
			}

			phoneStr := details.Phone
			if details.IsMobile {
				phoneStr += " (موبایل)"
			} else if details.Phone != "" {
				phoneStr += " (ثابت)"
			}
			fmt.Printf("[%d/%d] [ثبت شد] صاحب امتیاز: %-20s | تلفن: %s\n", idx+1, len(urls), details.OwnerName, phoneStr)

			if scanSites && details.Email != "" {
				parts := strings.Split(details.Email, "@")
				if len(parts) == 2 {
					mobiles, phones := contactScanner.ScanDomain(parts[1])
					bInfo.SiteMobiles = dedup.FilterSitePhones(mobiles)
					bInfo.SitePhones = dedup.FilterSitePhones(phones)
				}
			}

			mu.Lock()
			results = append(results, bInfo)
			mu.Unlock()
		}(i, u)
	}

	wg.Wait()
	return results
}

func loadExistingData(dedup *enamad.Deduplicator, prefix string) {
	csvPath := prefix + ".csv"
	file, err := os.Open(csvPath)
	if err != nil {
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil || len(records) <= 1 {
		return
	}

	count := 0
	for _, row := range records[1:] {
		domain := ""
		phone := ""
		if len(row) > 0 {
			domain = row[0]
		}
		if len(row) > 7 {
			phone = row[7]
		}
		dedup.CheckAndAdd(domain, phone)
		count++
	}

	if count > 0 {
		fmt.Printf("اطلاعات %d کسب‌وکار قبلی از فایل %s جهت جلوگیری از تکرار بارگذاری شد.\n", count, csvPath)
	}
}

func readLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func saveOutputs(businesses []enamad.BusinessInfo, prefix string) {
	// Export to JSON
	jsonFileName := prefix + ".json"
	jsonData, err := json.MarshalIndent(businesses, "", "  ")
	if err == nil {
		_ = os.WriteFile(jsonFileName, jsonData, 0644)
		fmt.Printf("\nنتایج کامل در فایل JSON ذخیره شد: %s\n", jsonFileName)
	}

	// Export to CSV
	csvFileName := prefix + ".csv"
	csvFile, err := os.Create(csvFileName)
	if err == nil {
		defer csvFile.Close()
		writer := csv.NewWriter(csvFile)
		defer writer.Flush()

		_ = writer.Write([]string{
			"Domain",
			"BusinessName",
			"Province",
			"City",
			"Rating",
			"TrustsealURL",
			"OwnerName",
			"Phone",
			"IsMobile",
			"Address",
			"Email",
			"ResponseHours",
			"SiteMobiles",
			"SitePhones",
			"ExtractionDate",
			"ExtractionDay",
			"ExtractionTime",
			"ExtractedAt",
		})

		for _, b := range businesses {
			owner := ""
			phone := ""
			isMobile := "false"
			address := ""
			email := ""
			hours := ""
			if b.Trustseal != nil {
				owner = b.Trustseal.OwnerName
				phone = b.Trustseal.Phone
				if b.Trustseal.IsMobile {
					isMobile = "true"
				}
				address = b.Trustseal.Address
				email = b.Trustseal.Email
				hours = b.Trustseal.ResponseHours
			}

			_ = writer.Write([]string{
				b.DomainAddress,
				b.PersianName,
				b.Province,
				b.City,
				strconv.Itoa(b.Rating),
				b.TrustsealURL,
				owner,
				phone,
				isMobile,
				address,
				email,
				hours,
				strings.Join(b.SiteMobiles, "; "),
				strings.Join(b.SitePhones, "; "),
				b.ExtractionDate,
				b.ExtractionDay,
				b.ExtractionTime,
				b.ExtractedAt,
			})
		}
		fmt.Printf("نتایج به تفکیک ستون‌ها در فایل CSV ذخیره شد: %s\n", csvFileName)
	}

	fmt.Printf("عملیات با موفقیت پایان یافت. مجموعاً %d کسب‌وکار ثبت گردید.\n", len(businesses))
}

func openImageInViewer(filePath string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
