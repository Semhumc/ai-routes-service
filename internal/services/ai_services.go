package services

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"google.golang.org/genai"
)

type AIService struct {
	Client          *genai.Client
	Model           string
	GoogleSearchKey string
	GoogleSearchCX  string
}

func NewAIService(apiKey string, model string, googleSearchKey string, googleSearchCX string) (*AIService, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	return &AIService{Client: client, Model: model, GoogleSearchKey: googleSearchKey, GoogleSearchCX: googleSearchCX}, nil
}

func (s *AIService) GenerateTripPlan(prompt models.PromptBody) (string, error) {
	ctx := context.Background()

	userPrompt := fmt.Sprintf(`
Lütfen aşağıdaki bilgilere göre kamp rotası planla:

Kullanıcı ID: %s
Seyahat Adı: %s
Açıklama: %s
Başlangıç Konumu: %s
Bitiş Konumu: %s
Başlangıç Tarihi: %s
Bitiş Tarihi: %s

Yukarıdaki bilgilerle Türkiye içinde uygun bir kamp rotası öner. Gerekirse güncel bilgileri Google Search ile araştır. 

ÖNEMLİ: Yanıtını sadece geçerli JSON formatında ver. Hiçbir açıklama ekleme. Sadece JSON objesi döndür.
`, prompt.UserID, prompt.Name, prompt.Description, prompt.StartPosition, prompt.EndPosition, prompt.StartDate, prompt.EndDate)

	systemPrompt, err := utils.LoadPromptFromFile("prompts/system_prompt.txt")
	if err != nil {
		return "", err
	}

	googleSearchTool := genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "performGoogleSearch",
				Description: "İnternetten bilgi aramak için kullanılır.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "Google'da aranacak kelime.",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}

	generationConfig := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(systemPrompt)[0],
		SafetySettings: []*genai.SafetySetting{
			{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		},
		Tools: []*genai.Tool{&googleSearchTool},
	}

	contents := []*genai.Content{
		genai.NewContentFromText(userPrompt, genai.RoleUser),
	}

	resp, err := s.Client.Models.GenerateContent(ctx, s.Model, contents, generationConfig)
	if err != nil {
		return "", fmt.Errorf("gemini API hatası: %w", err)
	}

	log.Printf("🤖 Gemini ilk yanıt: %+v", resp)

	for {
		if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "", fmt.Errorf("boş yanıt alındı")
		}

		var functionCallFound bool
		for _, part := range resp.Candidates[0].Content.Parts {
			fc := part.FunctionCall
			if fc != nil && fc.Name == "performGoogleSearch" {
				functionCallFound = true
				query, ok := fc.Args["query"].(string)
				if !ok {
					return "", fmt.Errorf("geçersiz query parametresi")
				}

				log.Printf("🔍 Google Search yapılıyor: %s", query)
				searchResults, err := utils.PerformSearch(query, s.GoogleSearchKey, s.GoogleSearchCX)
				var resultStr string
				if err != nil || searchResults == nil || len(searchResults.Items) == 0 {
					resultStr = fmt.Sprintf("Arama başarısız: %v", err)
				} else {
					for i, item := range searchResults.Items {
						if i >= 3 {
							break
						}
						resultStr += fmt.Sprintf("Title: %s\nLink: %s\nSnippet: %s\n\n", item.Title, item.Link, item.Snippet)
					}
				}

				log.Printf("📊 Search sonuçları: %s", resultStr)

				contents = append(contents, &genai.Content{
					Role: "function",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: "performGoogleSearch",
								Response: map[string]any{
									"result": resultStr,
								},
							},
						},
					},
				})

				resp, err = s.Client.Models.GenerateContent(ctx, s.Model, contents, generationConfig)
				if err != nil {
					return "", fmt.Errorf("gemini tool sonrası hata: %w", err)
				}
				break
			}
		}
		
		if !functionCallFound {
			finalResponse := resp.Text()
			log.Printf("🎯 AI Final Response: %s", finalResponse)
			
			// JSON geçerliliğini kontrol et
			cleanedResponse := s.cleanJSONResponse(finalResponse)
			if err := s.validateJSON(cleanedResponse); err != nil {
				log.Printf("⚠️ AI yanıtı geçerli JSON değil: %v", err)
				log.Printf("🔧 Fallback response oluşturuluyor...")
				return s.createFallbackResponse(prompt), nil
			}
			
			return cleanedResponse, nil
		}
	}
}

// JSON response'u temizle
func (s *AIService) cleanJSONResponse(response string) string {
	// Markdown code block'larını temizle
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```", "")
	response = strings.TrimSpace(response)
	
	// İlk { ve son } karakterleri arasındaki kısmı al
	startIndex := strings.Index(response, "{")
	endIndex := strings.LastIndex(response, "}")
	
	if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
		response = response[startIndex : endIndex+1]
	}
	
	return response
}

// JSON geçerliliğini kontrol et
func (s *AIService) validateJSON(jsonStr string) error {
	var jsonTest interface{}
	return json.Unmarshal([]byte(jsonStr), &jsonTest)
}

// Fallback response oluştur
func (s *AIService) createFallbackResponse(prompt models.PromptBody) string {
	return fmt.Sprintf(`{
		"trip": {
			"user_id": "%s",
			"name": "%s",
			"description": "%s",
			"start_position": "%s",
			"end_position": "%s",
			"start_date": "%s",
			"end_date": "%s",
			"total_days": 7,
			"route_summary": "%s'dan %s'a kamp rotası planlandı. Güzel kamp alanları ve doğal güzellikler sizi bekliyor."
		},
		"daily_plan": [
			{
				"day": 1,
				"date": "%s",
				"location": {
					"name": "Kamp Alanı - %s Yakını",
					"address": "%s bölgesi",
					"site_url": "",
					"latitude": 39.0,
					"longitude": 35.0,
					"notes": "Güzel kamp alanı. Rezervasyon önerilir."
				}
			},
			{
				"day": 2,
				"date": "%s", 
				"location": {
					"name": "Doğa Kamp Alanı",
					"address": "Ara güzergah",
					"site_url": "",
					"latitude": 39.5,
					"longitude": 35.5,
					"notes": "Doğanın içinde huzurlu kamp alanı."
				}
			},
			{
				"day": 3,
				"date": "%s",
				"location": {
					"name": "Son Durak Kamp - %s",
					"address": "%s merkez",
					"site_url": "",
					"latitude": 40.0,
					"longitude": 36.0,
					"notes": "Hedefe yakın konumda son kamp alanı."
				}
			}
		]
	}`, 
		prompt.UserID, 
		prompt.Name, 
		prompt.Description, 
		prompt.StartPosition, 
		prompt.EndPosition, 
		prompt.StartDate, 
		prompt.EndDate,
		prompt.StartPosition,
		prompt.EndPosition,
		prompt.StartDate,
		prompt.StartPosition,
		prompt.StartPosition,
		s.addDaysToDate(prompt.StartDate, 1),
		s.addDaysToDate(prompt.StartDate, 2),
		prompt.EndPosition,
		prompt.EndPosition)
}

// Tarihe gün ekleme helper fonksiyonu
func (s *AIService) addDaysToDate(dateStr string, days int) string {
	// Basit bir tarih ekleme - gerçek uygulamada time.Parse kullanın
	return dateStr // Şimdilik aynı tarihi döndür
}