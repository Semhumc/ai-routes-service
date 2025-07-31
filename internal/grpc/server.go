package grpc

import (
	"ai-routes-service/internal/models"
	"ai-routes-service/internal/services"
	"context"
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
		return nil, err
	}

	response := &proto.TripPlanResponse{
		Trip: &proto.Trip{
			UserId:        req.UserId,
			Name:          req.Name,
			Description:   req.Description,
			StartPosition: req.StartPosition,
			EndPosition:   req.EndPosition,
			StartDate:     req.StartDate,
			EndDate:       req.EndDate,
			TotalDays:     7, // Calculate based on dates
			RouteSummary:  result,
		},
		DailyPlan: []*proto.DailyPlan{

		},
	}
	return response,nil
		
}

func StartGRPCServer(aiService *services.AIService, port string) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	
	aiGrpcServer := NewAIGrpcServer(aiService)
	proto.RegisterAIServiceServer(s, aiGrpcServer)

	log.Printf("gRPC server listening on port %s", port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
