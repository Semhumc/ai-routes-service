package services

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/utils"
	"context"
	"fmt"
	

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

Yukarıdaki bilgilerle Türkiye içinde uygun bir kamp rotası öner. Gerekirse güncel bilgileri Google Search ile araştır. JSON olarak dön.
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
		return "", fmt.Errorf("gemini aPI hatası: %w", err)
	}
	fmt.Println("gemini:", resp)

	for {
		if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return "", fmt.Errorf("boş yanıt alındı.")
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

				fmt.Println("search", searchResults)

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

				fmt.Println("contents", contents)

				resp, err = s.Client.Models.GenerateContent(ctx, s.Model, contents, generationConfig)
				if err != nil {
					return "", fmt.Errorf("gemini tool sonrası hata: %w", err)
				}
				break
			}
		}
		if !functionCallFound {
			return resp.Text(), nil
		}
	}
}
