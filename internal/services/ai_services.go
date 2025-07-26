package services

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

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
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, err
	}

	return &AIService{
		Client:          client,
		Model:           model,
		GoogleSearchKey: googleSearchKey,
		GoogleSearchCX:  googleSearchCX,
	}, nil
}

func (s *AIService) performGoogleSearch(query string) (string, error) {
	baseURL := "https://www.googleapis.com/customsearch/v1"
	params := url.Values{}
	params.Add("key", s.GoogleSearchKey)
	params.Add("cx", s.GoogleSearchCX)
	params.Add("q", query)
	params.Add("num", "5") // Limit to 5 results

	resp, err := http.Get(baseURL + "?" + params.Encode())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var searchResult struct {
		Items []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"items"`
	}

	if err := json.Unmarshal(body, &searchResult); err != nil {
		return "", err
	}

	// Format results
	result := "Search Results:\n"
	for i, item := range searchResult.Items {
		result += fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, item.Title, item.Snippet, item.Link)
	}

	return result, nil
}

func (s *AIService) GenerateTripPlan(prompt models.PromptBody) (string, error) {
	ctx := context.Background()

	// Create a detailed user prompt that includes all the data
	userPrompt := fmt.Sprintf(`
Lütfen aşağıdaki bilgilere göre kamp rotası planla:

Kullanıcı ID: %s
Seyahat Adı: %s
Açıklama: %s
Başlangıç Konumu: %s
Bitiş Konumu: %s
Başlangıç Tarihi: %s
Bitiş Tarihi: %s

Bu bilgileri kullanarak Türkiye içinde %s konumundan %s konumuna kadar %s tarihleri arasında bir kamp rotası planla. JSON formatında detaylı bir plan hazırla.
`,
		prompt.UserID,
		prompt.Name,
		prompt.Description,
		prompt.StartPosition,
		prompt.EndPosition,
		prompt.StartDate,
		prompt.EndDate,
		prompt.StartPosition,
		prompt.EndPosition,
		prompt.StartDate+" - "+prompt.EndDate,
	)

	systemPrompt, err := utils.LoadPromptFromFile("system_prompt.txt")
	if err != nil {
		return "", err
	}

	tools := []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "googleSearch",
					Description: "A tool to perform a web search using Google Search.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query": {
								Type:        genai.TypeString,
								Description: "The search query for Google Search.",
							},
						},
						Required: []string{"query"},
					},
				},
			},
		},
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.Text(systemPrompt)[0],
		Tools:             tools,
	}

	result, err := s.Client.Models.GenerateContent(ctx, s.Model, genai.Text(userPrompt), config)
	if err != nil {
		return "", err
	}

	// Handle function calls
	for {
		if len(result.Candidates) == 0 {
			break
		}

		candidate := result.Candidates[0]
		var functionResponses []*genai.Content
		hasFunctionCalls := false

		for _, part := range candidate.Content.Parts {
			if fnCall := part.FunctionCall; fnCall != nil {
				hasFunctionCalls = true

				if fnCall.Name == "googleSearch" {
					query, ok := fnCall.Args["query"].(string)
					if !ok {
						continue
					}

					searchResult, err := s.performGoogleSearch(query)
					if err != nil {
						searchResult = "Error performing search: " + err.Error()
					}

					functionResponses = append(functionResponses, genai.Text(searchResult)...)
				}
			}
		}

		if !hasFunctionCalls {
			break
		}

		// Send function responses back to the model
		if len(functionResponses) > 0 {
			result, err = s.Client.Models.GenerateContent(ctx, s.Model, functionResponses, config)
			if err != nil {
				return "", err
			}
		}
	}

	return result.Text(), nil
}
