package models

type PromptBody struct {
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	StartPosition string `json:"start_position"`
	EndPosition   string `json:"end_position"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
}

type ReqBody struct {
	Prompt PromptBody `json:"prompt"`
}
