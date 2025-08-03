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

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type AIService struct {
	Client          *genai.Client
	Model           string
	GoogleSearchKey string
	GoogleSearchCX  string
}

const (
	MAX_CONTEXT_LENGTH = 20000
	MAX_SEARCH_RESULTS = 3
	MAX_ITERATIONS     = 5
	REQUEST_TIMEOUT    = 3 * time.Minute
)

func NewAIService(apiKey string, model string, googleSearchKey string, googleSearchCX string) (*AIService, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("genai client oluşturulamadı: %w", err)
	}
	return &AIService{
		Client:          client,
		Model:          model,
		GoogleSearchKey: googleSearchKey,
		GoogleSearchCX:  googleSearchCX,
	}, nil
}

func (s *AIService) GenerateTripPlan(prompt models.PromptBody) (string, error) {
	return s.generatePlanWithFunctionCalls(prompt)
}

func (s *AIService) generatePlanWithFunctionCalls(prompt models.PromptBody) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT)
	defer cancel()

	log.Printf("🤖 %s -> %s rotası için plan oluşturuluyor", prompt.StartPosition, prompt.EndPosition)

	systemPrompt := `Sen, Google Search aracını kullanarak gerçek verilere dayalı kamp rotaları oluşturan bir seyahat planlama asistanısın.

GÖREV AKIŞI:
1. **Rota Analizi:** Başlangıç ve bitiş noktaları arasındaki ana şehirleri belirle
2. **Kamp Alanı Arama:** Her durak için gerçek kamp alanları ara
3. **Detay Doğrulama:** Kamp alanlarının GPS koordinatları ve web sitelerini bul
4. **JSON Oluşturma:** Tüm bilgileri JSON formatında birleştir

KURALLAR:
- Her gün coğrafi olarak mantıklı bir ilerleme olmalı
- Her kamp alanının adı, adresi, çalışan web sitesi ve GPS koordinatları olmalı
- Eksik bilgi varsa yeni arama yap, bulamazsan o kamp alanını kullanma

JSON ÇIKTI FORMATI:
{
  "trip": {
    "user_id": "string",
    "name": "string", 
    "description": "string",
    "start_position": "string",
    "end_position": "string",
    "start_date": "string",
    "end_date": "string",
    "total_days": number
  },
  "daily_plan": [
    {
      "day": number,
      "location": {
        "name": "string",
        "address": "string", 
        "site_url": "string",
        "latitude": number,
        "longitude": number
      }
    }
  ]
}`

	userPrompt := fmt.Sprintf(`Bu bilgilere göre kamp rotası planı oluştur:
- Kullanıcı ID: %s
- Seyahat Adı: %s  
- Rota: %s → %s
- Tarihler: %s - %s
- Açıklama: %s`,
		prompt.UserID, prompt.Name, prompt.StartPosition, 
		prompt.EndPosition, prompt.StartDate, prompt.EndDate, prompt.Description)

	// Tool tanımı - eski SDK syntax'ı ile
	googleSearchTool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "performGoogleSearch",
				Description: "Google'da arama yapar ve sonuçları döndürür",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "Aranacak sorgu",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}

	// Model oluşturma - eski SDK syntax'ı
	model := s.Client.GenerativeModel(s.Model)
	
	// System instruction ayarlama
	model.SetSystemInstruction(genai.Text(systemPrompt))
	
	// Tools ayarlama
	model.Tools = []*genai.Tool{googleSearchTool}

	// Chat session başlatma
	session := model.StartChat()

	return s.managedConversation(ctx, session, userPrompt)
}

func (s *AIService) managedConversation(ctx context.Context, session *genai.ChatSession, userPrompt string) (string, error) {
	// İlk kullanıcı mesajını gönder
	resp, err := session.SendMessage(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", fmt.Errorf("ilk mesaj gönderilemedi: %w", err)
	}

	for iteration := 1; iteration <= MAX_ITERATIONS; iteration++ {
		log.Printf("🤖 İterasyon %d/%d", iteration, MAX_ITERATIONS)

		if len(session.History) > MAX_CONTEXT_LENGTH/100 { // Rough estimate
			log.Printf("⚠️ Context limiti aşıldı, durduruluyor")
			break
		}

		if resp == nil || len(resp.Candidates) == 0 {
			log.Printf("⚠️ Boş yanıt alındı")
			break
		}

		candidate := resp.Candidates[0]
		if candidate.Content == nil {
			log.Printf("⚠️ Content boş")
			break
		}

		// Function call var mı kontrol et
		var functionCall *genai.FunctionCall
		for _, part := range candidate.Content.Parts {
			if fc, ok := part.(*genai.FunctionCall); ok && fc != nil {
				functionCall = fc
				break
			}
		}

		if functionCall != nil {
			if functionCall.Name == "performGoogleSearch" {
				query, ok := functionCall.Args["query"].(string)
				if !ok {
					log.Printf("⚠️ Query parse edilemedi")
					continue
				}

				log.Printf("🔍 Arama: '%s'", query)
				searchResult := s.performSingleSearch(query)

				// Function response gönder
				resp, err = session.SendMessage(ctx, genai.FunctionResponse{
					Name: "performGoogleSearch",
					Response: map[string]any{
						"results": searchResult,
					},
				})

				if err != nil {
					log.Printf("❌ Function response hatası: %v", err)
					break
				}

				time.Sleep(1 * time.Second) // Rate limiting
			}
		} else {
			// Final response - text içeriği kontrol et
			if len(candidate.Content.Parts) > 0 {
				if textPart, ok := candidate.Content.Parts[0].(genai.Text); ok {
					finalResponse := string(textPart)
					log.Printf("✅ Final yanıt alındı")

					cleanedJSON := s.cleanJSONResponse(finalResponse)
					if cleanedJSON == "" {
						log.Printf("⚠️ JSON temizlenemedi, fallback kullanılıyor")
						return s.generateFallback(userPrompt), nil
					}
					return cleanedJSON, nil
				}
			}

			// Eğer text part bulunamazsa, candidate.Content.Parts içindeki diğer partları kontrol et
			var finalResponse string
			for _, part := range candidate.Content.Parts {
				if textPart, ok := part.(genai.Text); ok {
					finalResponse = string(textPart)
					break
				}
			}
			if finalResponse != "" {
				log.Printf("✅ Final yanıt alındı (fallback part scan)")
				cleanedJSON := s.cleanJSONResponse(finalResponse)
				if cleanedJSON == "" {
					log.Printf("⚠️ JSON temizlenemedi, fallback kullanılıyor")
					return s.generateFallback(userPrompt), nil
				}
				return cleanedJSON, nil
			}
			
			log.Printf("⚠️ Text content bulunamadı")
			break
		}
	}

	log.Printf("⚠️ Max iterasyon sayısına ulaşıldı")
	return s.generateFallback(userPrompt), nil
}

func (s *AIService) performSingleSearch(query string) string {
	searchResults, err := utils.PerformSearch(query, s.GoogleSearchKey, s.GoogleSearchCX)
	if err != nil {
		log.Printf("Arama hatası: %v", err)
		return fmt.Sprintf("'%s' için arama yapılamadı: %v", query, err)
	}

	if searchResults == nil || len(searchResults.Items) == 0 {
		return fmt.Sprintf("'%s' için sonuç bulunamadı", query)
	}

	var results strings.Builder
	maxResults := MAX_SEARCH_RESULTS
	if len(searchResults.Items) < maxResults {
		maxResults = len(searchResults.Items)
	}

	for i := 0; i < maxResults; i++ {
		item := searchResults.Items[i]
		results.WriteString(fmt.Sprintf("Başlık: %s\nÖzet: %s\nLink: %s\n\n", 
			item.Title, item.Snippet, item.Link))
	}

	return results.String()
}

func (s *AIService) generateFallback(userPrompt string) string {
	log.Printf("🔄 Fallback yanıt oluşturuluyor")

	// Extract basic info from user prompt if possible
	lines := strings.Split(userPrompt, "\n")
	userID, name, startPos, endPos, startDate, endDate := "", "", "", "", "", ""
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- Kullanıcı ID:") {
			userID = strings.TrimSpace(strings.TrimPrefix(line, "- Kullanıcı ID:"))
		} else if strings.HasPrefix(line, "- Seyahat Adı:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "- Seyahat Adı:"))
		} else if strings.HasPrefix(line, "- Rota:") {
			rota := strings.TrimSpace(strings.TrimPrefix(line, "- Rota:"))
			parts := strings.Split(rota, "→")
			if len(parts) == 2 {
				startPos = strings.TrimSpace(parts[0])
				endPos = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "- Tarihler:") {
			tarihler := strings.TrimSpace(strings.TrimPrefix(line, "- Tarihler:"))
			parts := strings.Split(tarihler, "-")
			if len(parts) == 2 {
				startDate = strings.TrimSpace(parts[0])
				endDate = strings.TrimSpace(parts[1])
			}
		}
	}

	fallbackData := map[string]interface{}{
		"trip": map[string]interface{}{
			"user_id":        userID,
			"name":          name + " (Ön Plan)",
			"description":   "Otomatik plan oluşturulamadı. Lütfen manuel araştırma yapın.",
			"start_position": startPos,
			"end_position":   endPos,
			"start_date":     startDate,
			"end_date":       endDate,
			"total_days":     1,
		},
		"daily_plan": []interface{}{},
		"notes":      "Detaylı plan oluşturmak için yeterli bilgi bulunamadı.",
	}

	jsonBytes, err := json.Marshal(fallbackData)
	if err != nil {
		log.Printf("Fallback JSON oluşturulamadı: %v", err)
		return `{"error": "Plan oluşturulamadı"}`
	}

	return string(jsonBytes)
}

func (s *AIService) cleanJSONResponse(response string) string {
	if response == "" {
		return ""
	}

	log.Printf("🔧 JSON temizleniyor...")
	
	// Trim whitespace
	response = strings.TrimSpace(response)
	
	// Remove markdown code blocks
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
	}
	
	response = strings.TrimSpace(response)

	// Find JSON boundaries
	startIndex := strings.Index(response, "{")
	endIndex := strings.LastIndex(response, "}")
	
	if startIndex == -1 || endIndex == -1 || endIndex < startIndex {
		log.Printf("⚠️ Geçerli JSON sınırları bulunamadı")
		return ""
	}

	result := response[startIndex : endIndex+1]

	// Validate JSON
	var test interface{}
	if err := json.Unmarshal([]byte(result), &test); err != nil {
		log.Printf("⚠️ JSON validation hatası: %v", err)
		return ""
	}

	log.Printf("✅ JSON başarıyla temizlendi")
	return result
}