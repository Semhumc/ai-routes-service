package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	googleSearchAPIURL = "https://www.googleapis.com/customsearch/v1"
)

// SearchResult Google Custom Search API yanıtını temsil eden struct
type SearchResult struct {
	Items []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"items"`
}

// PerformSearch Google Custom Search API'sini çağırır ve sonuçları döndürür.
func PerformSearch(query, apiKey, cx string) (*SearchResult, error) {
	params := url.Values{}
	params.Add("key", apiKey)
	params.Add("cx", cx)
	params.Add("q", query) // Aranacak terim

	resp, err := http.Get(googleSearchAPIURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("HTTP isteği gönderilirken hata oluştu: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Hata yanıtı okunurken hata oluştu: %w", err)
		}
		return nil, fmt.Errorf("Google Search API hatası: Durum kodu %d, Mesaj: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Yanıt gövdesi okunurken hata oluştu: %w", err)
	}

	var result SearchResult
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("JSON ayrıştırılırken hata oluştu: %w", err)
	}

	return &result, nil
}
