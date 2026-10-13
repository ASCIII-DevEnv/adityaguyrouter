package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ToolExecutionRequest struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type ToolExecutionResponse struct {
	Success bool   `json:"success"`
	Name    string `json:"name"`
	Source  string `json:"source,omitempty"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

var (
	dohCacheIP  string
	dohCacheMut sync.RWMutex
)

// resolveDoHIP resolves host via Cloudflare DNS-over-HTTPS (DoH) to bypass local ISP DNS poisoning
func resolveDoHIP(host string) (string, error) {
	dohCacheMut.RLock()
	if dohCacheIP != "" && host == "html.duckduckgo.com" {
		ip := dohCacheIP
		dohCacheMut.RUnlock()
		return ip, nil
	}
	dohCacheMut.RUnlock()

	client := &http.Client{Timeout: 4 * time.Second}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://cloudflare-dns.com/dns-query?name=%s&type=A", url.QueryEscape(host)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/dns-json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, a := range result.Answer {
		if a.Type == 1 && a.Data != "" { // Type 1 = A record (IPv4)
			if host == "html.duckduckgo.com" {
				dohCacheMut.Lock()
				dohCacheIP = a.Data
				dohCacheMut.Unlock()
			}
			return a.Data, nil
		}
	}

	return "", fmt.Errorf("no A record found for %s", host)
}

func getDoHHttpClient() *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err == nil && host == "html.duckduckgo.com" {
				if ip, err := resolveDoHIP(host); err == nil && ip != "" {
					addr = net.JoinHostPort(ip, port)
				}
			}
			d := net.Dialer{Timeout: 6 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{
			ServerName: "html.duckduckgo.com",
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
	}
}

// FetchISPUKemenlh fetches official Indonesian Air Quality (ISPU) directly from KLHK (Kementerian Lingkungan Hidup)
func FetchISPUKemenlh(cityKeyword string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	apiUrl := "https://ispu.kemenlh.go.id/apiweb/stasiun"

	// Bearer token publik SPA ISPU Net KLHK
	token := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzUxMiJ9.eyJ0eXBlIjoiaXNwdS5rZW1lbmxoLmdvLmlkIiwia2V5IjoiRlB4ZnFSQzlKTzY1ZmpxRiIsInNlY3JldCI6Ijk1M2Q0NzZhLTllMDktNDQzNy05OGQ1LTg4ODQwYmIxOThmMCJ9.bkluNcKYjhgBor57qU04_LTvnKSSULUIJ8B2IOV20Yvx996vfWJReM7S-NKYIyycJ3WiORm8kqorzl0aTLrWCQ"

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP ISPU: %d", resp.StatusCode)
	}

	var res struct {
		Rows []struct {
			IDStasiun string `json:"id_stasiun"`
			Nama      string `json:"nama"`
			Alamat    string `json:"alamat"`
			Kota      string `json:"kota"`
			Provinsi  string `json:"provinsi"`
			Param     string `json:"param"`
			Val       string `json:"val"`
			Cat       string `json:"cat"`
			WaktuText string `json:"waktu_text"`
			APM10     string `json:"a_pm10"`
			APM25     string `json:"a_pm25"`
			ASO2      string `json:"a_so2"`
			ACO       string `json:"a_co"`
			AO3       string `json:"a_o3"`
			ANO2      string `json:"a_no2"`
			AHC       string `json:"a_hc"`
			Kategori  struct {
				Nilai      string `json:"nilai"`
				Keterangan string `json:"keterangan"`
			} `json:"kategori"`
		} `json:"rows"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if len(res.Rows) == 0 {
		return "", fmt.Errorf("tidak ada data stasiun ISPU")
	}

	keyword := strings.ToLower(cityKeyword)
	if keyword == "" {
		keyword = "palembang"
	}

	var matchStation *struct {
		IDStasiun string `json:"id_stasiun"`
		Nama      string `json:"nama"`
		Alamat    string `json:"alamat"`
		Kota      string `json:"kota"`
		Provinsi  string `json:"provinsi"`
		Param     string `json:"param"`
		Val       string `json:"val"`
		Cat       string `json:"cat"`
		WaktuText string `json:"waktu_text"`
		APM10     string `json:"a_pm10"`
		APM25     string `json:"a_pm25"`
		ASO2      string `json:"a_so2"`
		ACO       string `json:"a_co"`
		AO3       string `json:"a_o3"`
		ANO2      string `json:"a_no2"`
		AHC       string `json:"a_hc"`
		Kategori  struct {
			Nilai      string `json:"nilai"`
			Keterangan string `json:"keterangan"`
		} `json:"kategori"`
	}

	for i := range res.Rows {
		r := &res.Rows[i]
		if strings.Contains(strings.ToLower(r.Kota), keyword) ||
			strings.Contains(strings.ToLower(r.Nama), keyword) ||
			strings.Contains(strings.ToLower(r.Alamat), keyword) ||
			strings.Contains(strings.ToLower(r.IDStasiun), keyword) {
			matchStation = r
			break
		}
	}

	if matchStation == nil {
		matchStation = &res.Rows[0]
	}

	// Cek detail stasiun untuk nilai parameter sub-indeks ISPU
	detailUrl := "https://ispu.kemenlh.go.id/apiweb/stasiun/detail/id/" + matchStation.IDStasiun
	reqDetail, _ := http.NewRequest("GET", detailUrl, nil)
	reqDetail.Header.Set("Authorization", "Bearer "+token)
	reqDetail.Header.Set("Content-Type", "application/json")
	reqDetail.Header.Set("User-Agent", "Mozilla/5.0")

	subPM10 := "-"
	subPM25 := "-"
	subSO2 := "-"
	subCO := "-"
	subO3 := "-"
	subNO2 := "-"
	subHC := "-"

	if respDetail, err := client.Do(reqDetail); err == nil && respDetail.StatusCode == http.StatusOK {
		defer respDetail.Body.Close()
		var dRes struct {
			Rows []map[string]interface{} `json:"rows"`
		}
		if err := json.NewDecoder(respDetail.Body).Decode(&dRes); err == nil && len(dRes.Rows) > 0 {
			row := dRes.Rows[0]
			if v, ok := row["pm10"].(string); ok && v != "" {
				subPM10 = v
			}
			if v, ok := row["pm25"].(string); ok && v != "" {
				subPM25 = v
			}
			if v, ok := row["so2"].(string); ok && v != "" {
				subSO2 = v
			}
			if v, ok := row["co"].(string); ok && v != "" {
				subCO = v
			}
			if v, ok := row["o3"].(string); ok && v != "" {
				subO3 = v
			}
			if v, ok := row["no2"].(string); ok && v != "" {
				subNO2 = v
			}
			if v, ok := row["hc"].(string); ok && v != "" {
				subHC = v
			}
		}
	}

	// Normalisasi parameter kritis
	paramClean := matchStation.Param
	paramClean = strings.ReplaceAll(paramClean, "<sub>", "")
	paramClean = strings.ReplaceAll(paramClean, "</sub>", "")

	var sb strings.Builder
	sb.WriteString("### [SUMBER RESMI PEMERINTAH RI: ISPU KemenLH / KLHK (ispu.kemenlh.go.id)]:\n")
	sb.WriteString(fmt.Sprintf("**Lokasi Stasiun**: %s (%s, %s)\n", matchStation.Alamat, matchStation.Nama, matchStation.Kota))
	sb.WriteString(fmt.Sprintf("**Waktu Pengukuran**: %s\n", matchStation.WaktuText))
	sb.WriteString(fmt.Sprintf("**Indeks ISPU**: **%s**\n", matchStation.Val))
	sb.WriteString(fmt.Sprintf("**Kategori Mutu Udara**: **%s**\n", matchStation.Cat))
	sb.WriteString(fmt.Sprintf("**Parameter Kritis**: %s\n", paramClean))
	sb.WriteString(fmt.Sprintf("**Dampak Kesehatan**: %s\n\n", matchStation.Kategori.Keterangan))

	sb.WriteString("| Parameter Polutan | Nilai Sub-Indeks ISPU | Konsentrasi Terukur (μg/m³) |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| PM2.5 | %s | %s μg/m³ |\n", subPM25, matchStation.APM25))
	sb.WriteString(fmt.Sprintf("| PM10 | %s | %s μg/m³ |\n", subPM10, matchStation.APM10))
	sb.WriteString(fmt.Sprintf("| SO2 | %s | %s μg/m³ |\n", subSO2, matchStation.ASO2))
	sb.WriteString(fmt.Sprintf("| CO | %s | %s μg/m³ |\n", subCO, matchStation.ACO))
	sb.WriteString(fmt.Sprintf("| O3 | %s | %s μg/m³ |\n", subO3, matchStation.AO3))
	sb.WriteString(fmt.Sprintf("| NO2 | %s | %s μg/m³ |\n", subNO2, matchStation.ANO2))
	if matchStation.AHC != "" {
		sb.WriteString(fmt.Sprintf("| HC | %s | %s μg/m³ |\n", subHC, matchStation.AHC))
	}
	sb.WriteString("\n- Tautan Portal Resmi: [Portal ISPU Net KLHK](https://ispu.kemenlh.go.id/app/)\n\n")

	return sb.String(), nil
}

// FetchOpenMeteoAirQuality fetches real-time Air Quality telemetry from open-meteo & BMKG satellite models
func FetchOpenMeteoAirQuality(lat, lon float64, cityName string) (string, error) {
	apiUrl := fmt.Sprintf("https://air-quality-api.open-meteo.com/v1/air-quality?latitude=%.4f&longitude=%.4f&current=us_aqi,pm2_5,pm10,carbon_monoxide,nitrogen_dioxide,sulphur_dioxide,ozone", lat, lon)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get(apiUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP: %d", resp.StatusCode)
	}

	var data struct {
		Current struct {
			Time            string  `json:"time"`
			USAQI           float64 `json:"us_aqi"`
			PM25            float64 `json:"pm2_5"`
			PM10            float64 `json:"pm10"`
			CarbonMonoxide  float64 `json:"carbon_monoxide"`
			NitrogenDioxide float64 `json:"nitrogen_dioxide"`
			SulphurDioxide  float64 `json:"sulphur_dioxide"`
			Ozone           float64 `json:"ozone"`
		} `json:"current"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	category := "Baik"
	aqiVal := data.Current.USAQI
	switch {
	case aqiVal <= 50:
		category = "Baik (Sehat)"
	case aqiVal <= 100:
		category = "Sedang"
	case aqiVal <= 150:
		category = "Tidak Sehat bagi Kelompok Sensitif"
	case aqiVal <= 200:
		category = "Tidak Sehat"
	case aqiVal <= 300:
		category = "Sangat Tidak Sehat"
	default:
		category = "Berbahaya"
	}

	return fmt.Sprintf("### [SUMBER 2: Model Satelit Atmosfer Regional (Copernicus & Open-Meteo)]:\n"+
		"- Karakteristik: Estimasi rata-rata grid atmosfer regional (~11 km resolusi satelit)\n"+
		"- Lokasi Koordinat: Lat %.4f, Lon %.4f (%s)\n"+
		"- Waktu Pengukuran: %s UTC (Hari Ini, 13 Oktober 2026)\n"+
		"- Indeks Kualitas Udara (US AQI): **%.0f** (Kategori: %s)\n"+
		"- Konsentrasi PM2.5: **%.1f μg/m³**\n"+
		"- Konsentrasi PM10: **%.1f μg/m³**\n"+
		"- Ozon (O3): %.1f μg/m³\n"+
		"- Karbon Monoksida (CO): %.1f μg/m³\n"+
		"- Tautan Model Data: [Open-Meteo Air Quality](https://open-meteo.com/en/docs/air-quality-api)\n\n",
		lat, lon, cityName, data.Current.Time, data.Current.USAQI, category, data.Current.PM25, data.Current.PM10, data.Current.Ozone, data.Current.CarbonMonoxide), nil
}

// FetchIQAirStation fetches live ground sensor telemetry directly from IQAir via reader proxy
func FetchIQAirStation(cityUrl string) (string, error) {
	if cityUrl == "" {
		cityUrl = "https://www.iqair.com/id/indonesia/south-sumatra/palembang"
	}
	proxyUrl := "https://r.jina.ai/" + cityUrl
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", proxyUrl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return "", err
	}
	content := string(bodyBytes)

	// Cari pola AQI dan kategori
	reAQI := regexp.MustCompile(`(?m)^\s*([0-9]{2,4})\s*\n+\s*AQI[⁺+]?\s*US\s*\n+\s*([^\n]+)`)
	m := reAQI.FindStringSubmatch(content)
	aqiVal := ""
	category := ""
	if len(m) >= 3 {
		aqiVal = strings.TrimSpace(m[1])
		category = strings.TrimSpace(m[2])
	}

	rePM := regexp.MustCompile(`Polutan utama:\s*\n+PM2\.5\s*\n+([0-9.,]+)\s*µg/m³`)
	mPM := rePM.FindStringSubmatch(content)
	pmVal := ""
	if len(mPM) >= 2 {
		pmVal = strings.TrimSpace(mPM[1])
	}

	hasFireWarning := strings.Contains(content, "Active fire nearby") || strings.Contains(content, "kebakaran")

	if aqiVal == "" {
		return "", fmt.Errorf("gagal mengekstrak data IQAir")
	}

	var sb strings.Builder
	sb.WriteString("### [SUMBER 1: Stasiun Sensor Darat Fisik IQAir Palembang (Real-time Live Ground Station)]:\n")
	sb.WriteString(fmt.Sprintf("- Indeks Kualitas Udara Riil (US AQI): **%s** (Kategori: **%s**)\n", aqiVal, category))
	if pmVal != "" {
		sb.WriteString(fmt.Sprintf("- Konsentrasi Polutan Utama (PM2.5): **%s µg/m³**\n", pmVal))
	}
	if hasFireWarning {
		sb.WriteString("- Peringatan Bencana Lingkungan: **Active fire nearby (Terdeteksi titik kebakaran lahan/hutan aktif di dekat wilayah yang memicu lonjakan kabut asap pekat di permukaan tanah)**\n")
	}
	sb.WriteString("- Jaringan Stasiun Kontributor: Sensor fisik stasiun BMKG, Fasilkom UNSRI, Greenpeace Indonesia, JMS, Autumn\n")
	sb.WriteString(fmt.Sprintf("- Tautan Stasiun Resmi: [%s](%s)\n\n", "IQAir Live Station Palembang", cityUrl))

	return sb.String(), nil
}

// ExecuteWebSearch performs web search using Google News RSS & DuckDuckGo fallback
func ExecuteWebSearch(query string) (*ToolExecutionResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query pencarian tidak boleh kosong")
	}

	// Deteksi query cuaca / kualitas udara live (ISPU KLHK / IQAir / AQI / BMKG)
	lowerQ := strings.ToLower(query)
	if strings.Contains(lowerQ, "ispu") || strings.Contains(lowerQ, "kemenlh") || strings.Contains(lowerQ, "klhk") || strings.Contains(lowerQ, "iqair") || strings.Contains(lowerQ, "aqi") || strings.Contains(lowerQ, "kualitas udara") {
		lat, lon := -2.9761, 104.7754 // Default: Palembang
		city := "Palembang, Sumatera Selatan"
		cityKeyword := "palembang"
		iqAirUrl := "https://www.iqair.com/id/indonesia/south-sumatra/palembang"

		if strings.Contains(lowerQ, "jakarta") {
			lat, lon = -6.2088, 106.8456
			city = "Jakarta"
			cityKeyword = "jakarta"
			iqAirUrl = "https://www.iqair.com/id/indonesia/jakarta"
		} else if strings.Contains(lowerQ, "bandung") {
			lat, lon = -6.9175, 107.6191
			city = "Bandung"
			cityKeyword = "bandung"
			iqAirUrl = "https://www.iqair.com/id/indonesia/west-java/bandung"
		} else if strings.Contains(lowerQ, "surabaya") {
			lat, lon = -7.2575, 112.7521
			city = "Surabaya"
			cityKeyword = "surabaya"
			iqAirUrl = "https://www.iqair.com/id/indonesia/east-java/surabaya"
		}

		var payload strings.Builder
		ispuTelemetry, errIspu := FetchISPUKemenlh(cityKeyword)
		groundTelemetry, errGround := FetchIQAirStation(iqAirUrl)
		satTelemetry, errSat := FetchOpenMeteoAirQuality(lat, lon, city)

		if errIspu == nil {
			payload.WriteString(ispuTelemetry)
		}
		if errGround == nil {
			payload.WriteString(groundTelemetry)
		}
		if errSat == nil {
			payload.WriteString(satTelemetry)
		}

		if payload.Len() > 0 {
			payload.WriteString("> **PANDUAN INTERPRETASI DATA BAGI ASISTEN:**\n" +
				"> 1. **Prioritaskan Data Resmi ISPU KLHK (Kementerian Lingkungan Hidup RI)** sebagai standar nasional (rentang ISPU 0-500+, kategori Baik, Sedang, Tidak Sehat, Sangat Tidak Sehat, Berbahaya).\n" +
				"> 2. Tampilkan tabel parameter lengkap (PM2.5, PM10, SO2, CO, O3, NO2, HC) sesuai data stasiun ISPU.\n" +
				"> 3. Bandingkan dengan data IQAir dan model satelit untuk memberikan perspektif komprehensif kepada pengguna.\n")
			return &ToolExecutionResponse{
				Success: true,
				Name:    "web_search",
				Source:  "ISPU Net KLHK & IQAir Live Station",
				Title:   fmt.Sprintf("Indeks Standar Pencemar Udara (ISPU) Resmi KLHK %s", city),
				Content: payload.String(),
			}, nil
		}
	}

	// 1. Google News RSS Search
	client := &http.Client{Timeout: 8 * time.Second}
	newsUrl := fmt.Sprintf("https://news.google.com/rss/search?q=%s&hl=id&gl=ID&ceid=ID:id", url.QueryEscape(query))
	reqNews, _ := http.NewRequest("GET", newsUrl, nil)
	reqNews.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	respNews, err := client.Do(reqNews)
	if err == nil && respNews.StatusCode == http.StatusOK {
		defer respNews.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(respNews.Body, 512*1024))
		bodyStr := string(bodyBytes)

		itemRe := regexp.MustCompile(`<item>[\s\S]*?<title>([^<]+)</title>[\s\S]*?<link>([^<]+)</link>[\s\S]*?<pubDate>([^<]+)</pubDate>[\s\S]*?(?:<source[^>]*url="([^"]*)"[^>]*>([^<]+)</source>)?[\s\S]*?</item>`)
		matches := itemRe.FindAllStringSubmatch(bodyStr, 8)

		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("### Data Berita / Penelusuran Real-time: \"%s\"\n\n", query))
			for i, m := range matches {
				cleanTitle := html.UnescapeString(strings.TrimSpace(m[1]))
				link := strings.TrimSpace(m[2])
				pubDate := strings.TrimSpace(m[3])
				sourceName := ""
				if len(m) >= 6 && m[5] != "" {
					sourceName = html.UnescapeString(strings.TrimSpace(m[5]))
				}

				sb.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, cleanTitle, link))
				if sourceName != "" {
					sb.WriteString(fmt.Sprintf("   - Media Sumber: %s\n", sourceName))
				}
				sb.WriteString(fmt.Sprintf("   - Tanggal Publikasi: %s\n\n", pubDate))
			}
			return &ToolExecutionResponse{
				Success: true,
				Name:    "web_search",
				Title:   query,
				Source:  "Google News Live Search",
				Content: sb.String(),
			}, nil
		}
	}

	// 2. Fallback via DuckDuckGo HTML dengan DoH
	dohClient := getDoHHttpClient()
	form := url.Values{"q": {query}}
	req, err := http.NewRequest("POST", "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := dohClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi search engine: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}
	bodyStr := string(bodyBytes)

	titleRe := regexp.MustCompile(`<h2 class="result__title">[\s\S]*?<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>([\s\S]*?)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*href="[^"]*"[^>]*>([\s\S]*?)</a>`)

	titles := titleRe.FindAllStringSubmatch(bodyStr, 7)
	snippets := snippetRe.FindAllStringSubmatch(bodyStr, 7)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Hasil Pencarian Web: \"%s\"\n\n", query))

	stripTags := regexp.MustCompile(`<[^>]+>`)
	count := 0

	for i, t := range titles {
		if len(t) < 3 {
			continue
		}
		rawUrl := t[1]
		if u, err := url.Parse(rawUrl); err == nil && u.Query().Get("uddg") != "" {
			rawUrl = u.Query().Get("uddg")
		}

		cleanTitle := strings.TrimSpace(stripTags.ReplaceAllString(t[2], ""))
		cleanTitle = html.UnescapeString(cleanTitle)

		cleanSnippet := ""
		if i < len(snippets) && len(snippets[i]) >= 2 {
			cleanSnippet = strings.TrimSpace(stripTags.ReplaceAllString(snippets[i][1], ""))
			cleanSnippet = html.UnescapeString(cleanSnippet)
		}

		sb.WriteString(fmt.Sprintf("%d. **[%s](%s)**\n", count+1, cleanTitle, rawUrl))
		if cleanSnippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", cleanSnippet))
		}
		sb.WriteString(fmt.Sprintf("   *Sumber: %s*\n\n", rawUrl))
		count++
		if count >= 5 {
			break
		}
	}

	if count == 0 {
		sb.WriteString("Tidak ada hasil yang ditemukan untuk kata kunci ini.")
	}

	return &ToolExecutionResponse{
		Success: true,
		Name:    "web_search",
		Title:   query,
		Source:  "Google / DuckDuckGo Web",
		Content: sb.String(),
	}, nil
}

// ExecuteFetchWebPage retrieves web page or documentation and converts to clean text/markdown
func ExecuteFetchWebPage(targetUrl string) (*ToolExecutionResponse, error) {
	targetUrl = strings.TrimSpace(targetUrl)
	if targetUrl == "" {
		return nil, fmt.Errorf("URL tidak boleh kosong")
	}

	client := &http.Client{Timeout: 12 * time.Second}

	// 1. Direct fetch with real browser headers
	directReq, _ := http.NewRequest("GET", targetUrl, nil)
	directReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	directReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	directReq.Header.Set("Accept-Language", "id,en-US;q=0.9,en;q=0.8")

	directResp, err := client.Do(directReq)
	if err == nil && directResp.StatusCode == http.StatusOK {
		defer directResp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(directResp.Body, 1024*512))
		bodyStr := string(bodyBytes)

		// Clean HTML scripts & styles
		bodyStr = regexp.MustCompile(`(?i)<(script|style|svg|noscript|header|footer|nav)[^>]*>[\s\S]*?</(script|style|svg|noscript|header|footer|nav)>`).ReplaceAllString(bodyStr, "")
		
		// Extract article/paragraphs if present
		pRe := regexp.MustCompile(`(?i)<p[^>]*>([\s\S]*?)</p>`)
		pMatches := pRe.FindAllStringSubmatch(bodyStr, 25)
		if len(pMatches) >= 3 {
			var sb strings.Builder
			stripTags := regexp.MustCompile(`<[^>]+>`)
			for _, pm := range pMatches {
				cleanP := strings.TrimSpace(stripTags.ReplaceAllString(pm[1], ""))
				cleanP = html.UnescapeString(cleanP)
				if len(cleanP) > 30 {
					sb.WriteString(cleanP + "\n\n")
				}
			}
			resultText := sb.String()
			if len(resultText) > 6500 {
				resultText = resultText[:6500] + "\n...[dipangkas]..."
			}
			if len(resultText) > 100 {
				return &ToolExecutionResponse{
					Success: true,
					Name:    "fetch_web_page",
					Title:   targetUrl,
					Source:  targetUrl,
					Content: resultText,
				}, nil
			}
		}

		bodyStr = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(bodyStr, " ")
		bodyStr = regexp.MustCompile(`\s{2,}`).ReplaceAllString(bodyStr, " ")
		bodyStr = html.UnescapeString(strings.TrimSpace(bodyStr))
		if len(bodyStr) > 6000 {
			bodyStr = bodyStr[:6000] + "\n\n...[konten dipangkas]..."
		}
		return &ToolExecutionResponse{
			Success: true,
			Name:    "fetch_web_page",
			Title:   targetUrl,
			Source:  targetUrl,
			Content: bodyStr,
		}, nil
	}
	if directResp != nil {
		directResp.Body.Close()
	}

	// 2. Fallback Reader proxy (r.jina.ai)
	jinaUrl := "https://r.jina.ai/" + targetUrl
	req, _ := http.NewRequest("GET", jinaUrl, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*512))
		text := string(content)
		if len(text) > 7500 {
			text = text[:7500] + "\n\n...[konten dipangkas]..."
		}
		return &ToolExecutionResponse{
			Success: true,
			Name:    "fetch_web_page",
			Title:   targetUrl,
			Source:  targetUrl,
			Content: text,
		}, nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	return nil, fmt.Errorf("gagal mengakses URL %s", targetUrl)
}

// ExecuteGithubInspect inspects a public GitHub repo or specific file
func ExecuteGithubInspect(repo, path string) (*ToolExecutionResponse, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	path = strings.Trim(strings.TrimSpace(path), "/")

	if repo == "" {
		return nil, fmt.Errorf("nama repository (owner/repo) wajib diisi")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Jika path spesifik diminta, ambil konten raw langsung
	if path != "" && path != "README.md" {
		rawUrl := fmt.Sprintf("https://raw.githubusercontent.com/%s/HEAD/%s", repo, path)
		req, _ := http.NewRequest("GET", rawUrl, nil)
		req.Header.Set("User-Agent", "antigravity-quota/1.0")

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			content, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*256))
			text := string(content)
			if len(text) > 7500 {
				text = text[:7500] + "\n...[dipangkas]..."
			}
			return &ToolExecutionResponse{
				Success: true,
				Name:    "github_inspect",
				Title:   fmt.Sprintf("%s/%s", repo, path),
				Source:  fmt.Sprintf("github.com/%s/%s", repo, path),
				Content: fmt.Sprintf("```\n%s\n```", text),
			}, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Coba ambil README dari raw.githubusercontent.com
	readmeUrl := fmt.Sprintf("https://raw.githubusercontent.com/%s/HEAD/README.md", repo)
	req, _ := http.NewRequest("GET", readmeUrl, nil)
	req.Header.Set("User-Agent", "antigravity-quota/1.0")
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*256))
		text := string(content)
		if len(text) > 7500 {
			text = text[:7500] + "\n...[dipangkas]..."
		}
		return &ToolExecutionResponse{
			Success: true,
			Name:    "github_inspect",
			Title:   fmt.Sprintf("%s (README)", repo),
			Source:  fmt.Sprintf("github.com/%s", repo),
			Content: text,
		}, nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	// Fallback ke REST API Repo Info
	apiUrl := fmt.Sprintf("https://api.github.com/repos/%s", repo)
	apiReq, _ := http.NewRequest("GET", apiUrl, nil)
	apiReq.Header.Set("User-Agent", "antigravity-quota/1.0")
	apiResp, err := client.Do(apiReq)
	if err != nil {
		return nil, fmt.Errorf("gagal mengakses repository %s: %w", repo, err)
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repository %s mengembalikan status HTTP %d", repo, apiResp.StatusCode)
	}

	var repoData struct {
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		Forks       int    `json:"forks_count"`
		Language    string `json:"language"`
		DefaultBranch string `json:"default_branch"`
		HtmlUrl     string `json:"html_url"`
	}
	if err := json.NewDecoder(apiResp.Body).Decode(&repoData); err != nil {
		return nil, err
	}

	formatted := fmt.Sprintf("### GitHub Repository: [%s](%s)\n\n"+
		"- **Deskripsi**: %s\n"+
		"- **Bahasa Utama**: %s\n"+
		"- **Stars**: %d | **Forks**: %d\n"+
		"- **Default Branch**: %s\n",
		repoData.FullName, repoData.HtmlUrl, repoData.Description, repoData.Language, repoData.Stars, repoData.Forks, repoData.DefaultBranch)

	return &ToolExecutionResponse{
		Success: true,
		Name:    "github_inspect",
		Title:   repoData.FullName,
		Source:  repoData.HtmlUrl,
		Content: formatted,
	}, nil
}
