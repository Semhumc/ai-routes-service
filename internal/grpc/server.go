package grpc

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/services"
	"context"
	"encoding/json"
	"log"
	"net"

	"github.com/Semhumc/grpc-proto/proto"
	"google.golang.org/grpc"
)

type AIGrpcServer struct {
	proto.UnimplementedAIServiceServer
	AIService *services.AIService
}

func NewAIGrpcServer(aiService *services.AIService) *AIGrpcServer {
	return &AIGrpcServer{
		AIService: aiService,
	}
}

func (s *AIGrpcServer) GeneratePlan(ctx context.Context, req *proto.PromptRequest) (*proto.TripPlanResponse, error) {
	log.Printf("📥 gRPC Request alındı: %+v", req)
	
	promptBody := models.PromptBody{
		UserID:        req.UserId,
		Name:          req.Name,
		Description:   req.Description,
		StartPosition: req.StartPosition,
		EndPosition:   req.EndPosition,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
	}

	result, err := s.AIService.GenerateTripPlan(promptBody)
	if err != nil {
		log.Printf("❌ AI Service hatası: %v", err)
		return nil, err
	}

	log.Printf("📤 AI Service sonucu: %s", result)

	// AI Response struct'ını tanımla
	type AIResponseLocation struct {
		Name      string  `json:"name"`
		Address   *string `json:"address"`
		SiteURL   *string `json:"site_url"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Notes     *string `json:"notes"`
	}

	type AIResponseDailyPlan struct {
		Day      int32              `json:"day"`
		Date     string             `json:"date"`
		Location AIResponseLocation `json:"location"`
	}

	type AIResponseTrip struct {
		UserID        string `json:"user_id"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		StartPosition string `json:"start_position"`
		EndPosition   string `json:"end_position"`
		StartDate     string `json:"start_date"`
		EndDate       string `json:"end_date"`
		TotalDays     int32  `json:"total_days"`
		RouteSummary  string `json:"route_summary"`
	}

	type AIResponse struct {
		Trip      AIResponseTrip        `json:"trip"`
		DailyPlan []AIResponseDailyPlan `json:"daily_plan"`
	}

	// JSON parse et
	var aiResponse AIResponse
	if err := json.Unmarshal([]byte(result), &aiResponse); err != nil {
		log.Printf("❌ JSON parse hatası: %v", err)
		log.Printf("🔧 Fallback response oluşturuluyor...")
		
		// Fallback response
		return &proto.TripPlanResponse{
			Trip: &proto.Trip{
				UserId:        req.UserId,
				Name:          req.Name,
				Description:   req.Description,
				StartPosition: req.StartPosition,
				EndPosition:   req.EndPosition,
				StartDate:     req.StartDate,
				EndDate:       req.EndDate,
				TotalDays:     7,
				RouteSummary:  "Kamp rotası planlandı. Detaylar için sistem yöneticisi ile iletişime geçin.",
			},
			DailyPlan: []*proto.DailyPlan{
				{
					Day:  1,
					Date: req.StartDate,
					Location: &proto.Location{
						Name:      "Kamp Alanı 1",
						Address:   req.StartPosition + " yakını",
						SiteUrl:   "",
						Latitude:  39.0,
						Longitude: 35.0,
						Notes:     "Güzel kamp alanı",
					},
				},
			},
		}, nil
	}

	log.Printf("✅ JSON başarıyla parse edildi. Daily plans sayısı: %d", len(aiResponse.DailyPlan))

	// Parsed response'u proto'ya çevir
	var dailyPlans []*proto.DailyPlan
	for i, daily := range aiResponse.DailyPlan {
		log.Printf("📍 Day %d: %s - %s", daily.Day, daily.Date, daily.Location.Name)
		
		dailyPlan := &proto.DailyPlan{
			Day:  daily.Day,
			Date: daily.Date,
			Location: &proto.Location{
				Name:      daily.Location.Name,
				Address:   getStringValue(daily.Location.Address),
				SiteUrl:   getStringValue(daily.Location.SiteURL),
				Latitude:  daily.Location.Latitude,
				Longitude: daily.Location.Longitude,
				Notes:     getStringValue(daily.Location.Notes),
			},
		}
		dailyPlans = append(dailyPlans, dailyPlan)
		
		log.Printf("✅ Daily plan %d eklendi", i+1)
	}

	response := &proto.TripPlanResponse{
		Trip: &proto.Trip{
			UserId:        aiResponse.Trip.UserID,
			Name:          aiResponse.Trip.Name,
			Description:   aiResponse.Trip.Description,
			StartPosition: aiResponse.Trip.StartPosition,
			EndPosition:   aiResponse.Trip.EndPosition,
			StartDate:     aiResponse.Trip.StartDate,
			EndDate:       aiResponse.Trip.EndDate,
			TotalDays:     aiResponse.Trip.TotalDays,
			RouteSummary:  aiResponse.Trip.RouteSummary,
		},
		DailyPlan: dailyPlans,
	}

	log.Printf("🎯 Final gRPC response hazırlandı. Daily plans: %d", len(response.DailyPlan))
	return response, nil
}

// Helper function - pointer string'i normal string'e çevir
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func StartGRPCServer(aiService *services.AIService, port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()

	aiGrpcServer := NewAIGrpcServer(aiService)
	proto.RegisterAIServiceServer(s, aiGrpcServer)

	log.Printf("🚀 gRPC server listening on port %s", port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}