package services

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/genai"
)

type AIService struct {
	Client          *genai.Client
	Model           string
	GoogleSearchKey string
	GoogleSearchCX  string
}

// Konservatif sabitler
const (
	MAX_CONTEXT_LENGTH = 20000
	MAX_SEARCH_RESULTS = 2
	MAX_ITERATIONS     = 3
	REQUEST_TIMEOUT    = 3 * time.Minute
)

func NewAIService(apiKey string, model string, googleSearchKey string, googleSearchCX string) (*AIService, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	return &AIService{Client: client, Model: model, GoogleSearchKey: googleSearchKey, GoogleSearchCX: googleSearchCX}, nil
}

func (s *AIService) GenerateTripPlan(prompt models.PromptBody) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()
	return s.twoStageGeneration(ctx, prompt)
}

func (s *AIService) twoStageGeneration(ctx context.Context, prompt models.PromptBody) (string, error) {
	log.Printf("🎯 Starting two-stage generation")
	searchResults, err := s.performManualSearches(prompt)
	if err != nil {
		log.Printf("⚠️ Search failed, continuing without: %v", err)
		searchResults = "Arama yapılamadı, genel bilgilerle plan oluşturulacak."
	} else {
		searchResults = summarizeSearchResults(searchResults, 20) // 🔍 EKLENDİ: Uzunluğu kısıtla
	}
	return s.generatePlanWithSearchResults(ctx, prompt, searchResults)
}

// Manual search yapma
func (s *AIService) performManualSearches(prompt models.PromptBody) (string, error) {
	log.Printf("🔍 Performing manual searches...")
	queries := []string{
		fmt.Sprintf("%s %s kamp alanları", prompt.StartPosition, prompt.EndPosition),
		fmt.Sprintf("%s kamp yerleri koordinat", prompt.StartPosition),
		fmt.Sprintf("%s camping sites", prompt.EndPosition),
	}

	allResults := ""
	for i, query := range queries {
		log.Printf("🔍 Search %d: %s", i+1, query)
		result := s.performSingleSearch(query)
		if result != "" {
			allResults += fmt.Sprintf("\n=== ARAMA %d: %s ===\n%s\n", i+1, query, result)
		}
		time.Sleep(1 * time.Second)
		if len(allResults) > 8000 {
			break
		}
	}

	if allResults == "" {
		return "", fmt.Errorf("no search results found")
	}
	log.Printf("✅ Manual searches completed: %d chars", len(allResults))
	return allResults, nil
}

// 🔍 Basit özetleme fonksiyonu eklendi
func summarizeSearchResults(fullText string, maxLines int) string {
	lines := strings.Split(fullText, "\n")
	var importantLines []string
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "kamp") || strings.Contains(line, "http") {
			importantLines = append(importantLines, line)
		}
		if len(importantLines) >= maxLines {
			break
		}
	}
	return strings.Join(importantLines, "\n")
}



// Tek search yapma
func (s *AIService) performSingleSearch(query string) string {
	searchResults, err := utils.PerformSearch(query, s.GoogleSearchKey, s.GoogleSearchCX)

	if err != nil || searchResults == nil || len(searchResults.Items) == 0 {
		return fmt.Sprintf("'%s' için sonuç bulunamadı", query)
	}

	resultStr := ""
	maxResults := MAX_SEARCH_RESULTS
	if len(searchResults.Items) < maxResults {
		maxResults = len(searchResults.Items)
	}

	for i := 0; i < maxResults; i++ {
		item := searchResults.Items[i]

		title := item.Title
		if len(title) > 80 {
			title = title[:80] + "..."
		}

		snippet := item.Snippet
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}

		resultStr += fmt.Sprintf("• %s\n  %s\n  %s\n\n", title, snippet, item.Link)
	}

	return resultStr
}

// Search sonuçlarıyla plan oluşturma
func (s *AIService) generatePlanWithSearchResults(ctx context.Context, prompt models.PromptBody, searchResults string) (string, error) {
	log.Printf("🎯 Generating plan with search results...")

	systemPrompt := `# Kamp Rotası Planlama AI - Tam Dinamik Sistem

Sen akıllı bir kamp rotası planlama uzmanısın. Kullanıcının verdiği bilgilere göre **tamamen araştırma bazlı** kamp rotası oluşturacaksın.


## SENİN GÖREVİN:

### 1. ROTA ANALİZ ET
- Başlangıç ve bitiş noktalarını analiz et
- Tarih aralığını hesapla (kaç gün)
- Mantıklı bir güzergah planla

### 2. HER GÜN İÇİN ARAŞTIRMA YAP
Sen kendi başına karar ver hangi aramaları yapacağına. Örnek stratejiler:

**İlk Araştırma:**

[başlangıç şehri] [bitiş şehri] arası kamp rotası güzergah


**Detay Araştırmaları:**

[şehir] kamp alanları adres web sitesi
[kamp alanı adı] koordinat konum 


**Koordinat Araştırması:**

[kamp alanı adı] GPS koordinat latitude longitude
[kamp alanı adı] Google Maps konum


## ÇIKTI FORMATI:

json
{
  "trip": {
    "user_id": "user_id",
    "name": "kullanıcının_girdiği_isim",
    "description": "kullanıcının_açıklaması",
    "start_position": "başlangıç",
    "end_position": "bitiş", 
    "start_date": "2024-08-01",
    "end_date": "2024-08-07",
    "total_days": 7,
  },
  "daily_plan": [
    {
      "day": 1,
      "date": "2024-08-01", 
      "location": {
        "name": "ARAŞTIRDIĞIN_GERÇEK_KAMP_ALANI",
        "address": "TAM_ADRES_BİLGİSİ_MAH_CAD_NO_İLÇE_İL",
        "site_url": "https://gerçek-web-sitesi.com",
        "latitude": 37.123456,
        "longitude": 27.654321,
      }
    }
  ]
}


## KRİTİK KURALLAR:

## Rota Planlama Kuralları:
- İlk gün start_position'dan başla
- Son gün end_position'da veya yakınında bitir
- Ara günlerde mantıklı bir rota izle (çok fazla geri dönüş yapma)
- Coğrafi yakınlığı göz önünde bulundur

## Önemli Notlar:
- start_position ve end_position'ı dikkate alarak mantıklı bir rota oluştur
- Sezon durumlarını kontrol et (kapalı kamp alanları önerme)

## Kalite Kontrol:
- Tüm kamp alanlarının gerçek ve aktif olduğundan emin ol
- Web sitesi linklerinin çalıştığını kontrol et
- Adres bilgilerinin doğru olduğunu doğrula
-Koordinatlar çok önemli.
- Rota mantığının doğru olduğunu kontrol et (start_position → end_position)


BAŞLA VE ARAŞTIR!`

	userPrompt := fmt.Sprintf(`KAMP ROTASI BİLGİLERİ:
ID: %s
İsim: %s
Açıklama: %s
Başlangıç: %s → Bitiş: %s
Tarih: %s - %s

ARAMA SONUÇLARI:
%s

Bu bilgileri kullanarak JSON formatında kamp rotası planı oluştur.`,
		prompt.UserID, prompt.Name, prompt.Description,
		prompt.StartPosition, prompt.EndPosition,
		prompt.StartDate, prompt.EndDate,
		searchResults)

	// Basit konfigürasyon - function call YOK
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(systemPrompt)[0],
		MaxOutputTokens:   4096,
		SafetySettings: []*genai.SafetySetting{
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		},
	}

	contents := []*genai.Content{
		genai.NewContentFromText(userPrompt, genai.RoleUser),
	}

	// Tek seferde response al
	resp, err := s.Client.Models.GenerateContent(ctx, s.Model, contents, config)
	if err != nil {
		log.Printf("❌ Generation failed: %v", err)
		// Fallback response döndür
		return s.generateFallbackWithSearch(prompt, searchResults), nil
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		log.Printf("⚠️ Empty response received")
		return s.generateFallbackWithSearch(prompt, searchResults), nil
	}

	response := resp.Text()
	log.Printf("✅ Plan generation successful: %d chars", len(response))

	cleaned := s.cleanJSONResponse(response)
	if cleaned == "" {
		log.Printf("⚠️ JSON cleaning failed")
		return s.generateFallbackWithSearch(prompt, searchResults), nil
	}

	return cleaned, nil
}

// Search sonuçlarıyla fallback
func (s *AIService) generateFallbackWithSearch(prompt models.PromptBody, searchResults string) string {
	log.Printf("🔄 Generating fallback with search results")

	// Search sonuçlarından kamp alanı ismi çıkarmaya çalış
	campName := "Genel Kamp Alanı"
	if strings.Contains(searchResults, "kamp") {
		lines := strings.Split(searchResults, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(strings.ToLower(line), "kamp") && strings.Contains(line, "•") {
				// İlk kamp alanını al
				parts := strings.Split(line, "•")
				if len(parts) > 1 {
					name := strings.TrimSpace(parts[1])
					if len(name) > 5 && len(name) < 100 {
						campName = name
						break
					}
				}
			}
		}
	}

	return fmt.Sprintf(`{
  "trip": {
    "user_id": "%s",
    "name": "%s", 
    "description": "%s",
    "start_position": "%s",
    "end_position": "%s",
    "start_date": "%s",
    "end_date": "%s",
    "total_days": 1,
    "route_summary": "Arama sonuçları kullanılarak oluşturulan kamp rotası planı."
  },
  "daily_plan": [
    {
      "day": 1,
      "date": "%s",
      "location": {
        "name": "%s",
        "address": "%s bölgesi",
        "site_url": "",
        "latitude": 39.9334,
        "longitude": 32.8597,
        "notes": "Arama sonuçlarından alınan bilgiler. Detaylı bilgi için araştırma yapılması önerilir."
      }
    }
  ]
}`, prompt.UserID, prompt.Name, prompt.Description,
		prompt.StartPosition, prompt.EndPosition,
		prompt.StartDate, prompt.EndDate,
		prompt.StartDate, campName, prompt.StartPosition)
}

// Gelişmiş function call versiyonu (alternatif)
func (s *AIService) GenerateTripPlanWithFunctionCalls(prompt models.PromptBody) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	log.Printf("🤖 Starting function call generation")

	systemPrompt := `Sen kamp rotası uzmanısın. Google Search kullanarak gerçek kamp alanları araştır.

ARAŞTIRMA STRATEJİSİ:
1. "[başlangıç] [bitiş] kamp alanları"
2. "[şehir] camping koordinat"
3. Gerçek kamp alanı bilgileri bul

JSON ÇıKTı:
{
  "trip": {...},
  "daily_plan": [{"day": 1, "location": {"name": "GERÇEK_ALAN", ...}}]
}`

	userPrompt := fmt.Sprintf(`Kamp rotası planla:
%s → %s (%s - %s)
İsim: %s

Gerçek kamp alanları araştır ve JSON planı oluştur.`,
		prompt.StartPosition, prompt.EndPosition,
		prompt.StartDate, prompt.EndDate, prompt.Name)

	// Google Search tool
	googleSearchTool := genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "performGoogleSearch",
				Description: "Google'da arama yap",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "Arama sorgusu",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(systemPrompt)[0],
		Tools:             []*genai.Tool{&googleSearchTool},

		MaxOutputTokens: 3072,
	}

	contents := []*genai.Content{
		genai.NewContentFromText(userPrompt, genai.RoleUser),
	}

	return s.managedConversation(ctx, contents, config, prompt)
}

// Basit conversation management
func (s *AIService) managedConversation(ctx context.Context, contents []*genai.Content, config *genai.GenerateContentConfig, prompt models.PromptBody) (string, error) {

	for iteration := 1; iteration <= MAX_ITERATIONS; iteration++ {
		log.Printf("🤖 Iteration %d/%d", iteration, MAX_ITERATIONS)

		// Context kontrolü
		if s.getContextLength(contents) > MAX_CONTEXT_LENGTH {
			log.Printf("⚠️ Context too long, stopping")
			break
		}

		resp, err := s.Client.Models.GenerateContent(ctx, s.Model, contents, config)
		if err != nil {
			log.Printf("❌ API Error: %v", err)
			return s.generateFallbackWithSearch(prompt, "API hatası nedeniyle arama yapılamadı"), nil
		}

		if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			log.Printf("⚠️ Empty response")
			break
		}

		// Response'u contents'e ekle
		contents = append(contents, resp.Candidates[0].Content)

		// Function call kontrolü
		hasSearchRequest := false
		for _, part := range resp.Candidates[0].Content.Parts {
			if fc := part.FunctionCall; fc != nil && fc.Name == "performGoogleSearch" {
				hasSearchRequest = true
				query, ok := fc.Args["query"].(string)
				if !ok {
					continue
				}

				log.Printf("🔍 Search: %s", query)
				searchResult := s.performSingleSearch(query)

				// Search response ekle
				contents = append(contents, &genai.Content{
					Role: "function",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: "performGoogleSearch",
								Response: map[string]any{
									"results": searchResult,
								},
							},
						},
					},
				})

				// Rate limiting
				time.Sleep(2 * time.Second)
				break // Sadece ilk search'ü işle
			}
		}

		// Final response?
		if !hasSearchRequest {
			finalResponse := resp.Text()
			log.Printf("🎯 Final response: %d chars", len(finalResponse))

			cleaned := s.cleanJSONResponse(finalResponse)
			if cleaned == "" {
				return finalResponse, nil
			}

			return cleaned, nil
		}
	}

	return s.generateFallbackWithSearch(prompt, "Maksimum iterasyon sayısına ulaşıldı"), nil
}

// Context uzunluğu hesaplama
func (s *AIService) getContextLength(contents []*genai.Content) int {
	totalLength := 0
	for _, content := range contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				totalLength += len(part.Text)
			}
			if part.FunctionResponse != nil {
				if results, ok := part.FunctionResponse.Response["results"].(string); ok {
					totalLength += len(results)
				}
			}
		}
	}
	return totalLength
}

// JSON temizleme
func (s *AIService) cleanJSONResponse(response string) string {
	if response == "" {
		return ""
	}

	log.Printf("🔧 Cleaning JSON...")

	// Markdown temizle
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```JSON", "")
	response = strings.ReplaceAll(response, "```", "")
	response = strings.TrimSpace(response)

	// JSON boundaries bul
	startIndex := strings.Index(response, "{")
	if startIndex == -1 {
		return ""
	}

	braceCount := 0
	endIndex := -1

	for i := startIndex; i < len(response); i++ {
		if response[i] == '{' {
			braceCount++
		} else if response[i] == '}' {
			braceCount--
			if braceCount == 0 {
				endIndex = i
				break
			}
		}
	}

	if endIndex == -1 {
		return ""
	}

	result := response[startIndex : endIndex+1]

	// Validation
	var test interface{}
	if err := json.Unmarshal([]byte(result), &test); err != nil {
		log.Printf("⚠️ JSON validation failed: %v", err)
		return ""
	}

	log.Printf("✅ JSON cleaned successfully")
	return result
}

// Test fonksiyonu
func (s *AIService) TestConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("🔍 Testing API connection...")

	config := &genai.GenerateContentConfig{
		MaxOutputTokens: 50,
	}

	contents := []*genai.Content{
		genai.NewContentFromText("Test mesajı. Sadece 'OK' yanıtını ver.", genai.RoleUser),
	}

	resp, err := s.Client.Models.GenerateContent(ctx, s.Model, contents, config)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	if resp != nil && len(resp.Candidates) > 0 {
		log.Printf("✅ API test successful: %s", resp.Text())
		return nil
	}

	return fmt.Errorf("empty test response")
}
