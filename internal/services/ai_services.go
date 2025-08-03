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

	// Basit ve net prompt - AI'ın kendi araştırması için
	userPrompt := fmt.Sprintf(`
Kullanıcı Bilgileri:
- ID: %s
- İsim: %s
- Açıklama: %s
- Başlangıç: %s
- Bitiş: %s
- Başlangıç Tarihi: %s
- Bitiş Tarihi: %s

GÖREV: Bu bilgilere göre Türkiye'de kamp rotası planla. Her gün için gerçek kamp alanlarını araştır ve bul.

Sadece JSON formatında yanıt ver.
`, prompt.UserID, prompt.Name, prompt.Description, prompt.StartPosition, prompt.EndPosition, prompt.StartDate, prompt.EndDate)

	systemPrompt, err := utils.LoadPromptFromFile("prompts/system_prompt.txt")
	if err != nil {
		systemPrompt = `Sen kamp rotası planlama uzmanısın. Google Search'ü aktif kullanarak gerçek kamp alanlarını araştır. Her lokasyon için mutlaka arama yap ve doğrula. Sadece JSON yanıt ver.`
	}

	googleSearchTool := genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "performGoogleSearch",
				Description: "Google'da arama yap. Kamp alanları için ZORUNLU.",
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

	// Maksimum 15 iterasyon - AI'ın istediği kadar araştırma yapmasına izin ver
	maxIterations := 15
	iteration := 0

	for iteration < maxIterations {
		iteration++
		log.Printf("🤖 AI Research Iteration %d", iteration)

		resp, err := s.Client.Models.GenerateContent(ctx, s.Model, contents, generationConfig)
		if err != nil {
			log.Printf("❌ API Error (iteration %d): %v", iteration, err)
			return "", fmt.Errorf("AI service error: %w", err)
		}

		if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			log.Printf("⚠️ Empty response (iteration %d)", iteration)
			return "", fmt.Errorf("empty response from AI")
		}

		// Function call kontrolü
		var hasMoreSearches bool
		for _, part := range resp.Candidates[0].Content.Parts {
			if fc := part.FunctionCall; fc != nil && fc.Name == "performGoogleSearch" {
				hasMoreSearches = true
				query, ok := fc.Args["query"].(string)
				if !ok {
					log.Printf("⚠️ Invalid search query")
					continue
				}

				log.Printf("🔍 AI Search Request: %s", query)
				
				// Google Search yap
				searchResults, err := utils.PerformSearch(query, s.GoogleSearchKey, s.GoogleSearchCX)
				
				var resultStr string
				if err != nil || searchResults == nil || len(searchResults.Items) == 0 {
					resultStr = fmt.Sprintf("Arama sonucu bulunamadı: %s", query)
					log.Printf("⚠️ Search failed for: %s", query)
				} else {
					log.Printf("✅ Found %d results for: %s", len(searchResults.Items), query)
					
					// AI'a zengin veri ver
					for i, item := range searchResults.Items {
						if i >= 10 { // İlk 10 sonuç
							break
						}
						resultStr += fmt.Sprintf(`
SONUÇ %d:
Başlık: %s
URL: %s
Açıklama: %s
---
`, i+1, item.Title, item.Link, item.Snippet)
					}
				}

				// AI'a search sonuçlarını gönder
				contents = append(contents, &genai.Content{
					Role: "function",
					Parts: []*genai.Part{
						{
							FunctionResponse: &genai.FunctionResponse{
								Name: "performGoogleSearch",
								Response: map[string]any{
									"results": resultStr,
								},
							},
						},
					},
				})
				break
			}
		}
		
		// AI daha fazla arama yapmıyorsa final response'u al
		if !hasMoreSearches {
			finalResponse := resp.Text()
			log.Printf("🎯 AI Final Response: %s", finalResponse)
			
			// JSON'u temizle
			cleanedResponse := s.cleanJSONResponse(finalResponse)
			if cleanedResponse == "" {
				log.Printf("⚠️ Could not extract JSON from response")
				return "", fmt.Errorf("invalid JSON response")
			}
			
			// JSON validation
			if err := s.validateJSON(cleanedResponse); err != nil {
				log.Printf("⚠️ JSON validation failed: %v", err)
				log.Printf("Raw response: %s", finalResponse)
				return "", fmt.Errorf("invalid JSON format: %w", err)
			}
			
			log.Printf("✅ Valid JSON received after %d iterations", iteration)
			return cleanedResponse, nil
		}
	}

	return "", fmt.Errorf("AI exceeded maximum research iterations (%d)", maxIterations)
}

// JSON temizleme - minimal müdahale
func (s *AIService) cleanJSONResponse(response string) string {
	if response == "" {
		return ""
	}

	// Sadece markdown block'ları temizle
	response = strings.ReplaceAll(response, "```json", "")
	response = strings.ReplaceAll(response, "```", "")
	response = strings.TrimSpace(response)
	
	// JSON başlangıç ve bitiş bul
	startIndex := strings.Index(response, "{")
	endIndex := strings.LastIndex(response, "}")
	
	if startIndex == -1 || endIndex == -1 || endIndex <= startIndex {
		return ""
	}
	
	return response[startIndex : endIndex+1]
}

// JSON validation
func (s *AIService) validateJSON(jsonStr string) error {
	var jsonTest interface{}
	return json.Unmarshal([]byte(jsonStr), &jsonTest)
}