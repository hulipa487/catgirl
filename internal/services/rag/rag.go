package rag

import (
	"context"
	"fmt"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type RAGService struct {
	repo   *repository.Repository
	llm    *llm.LLMService
	config *config.RAGConfig
	logger zerolog.Logger
}

func NewRAGService(repo *repository.Repository, llmSvc *llm.LLMService, cfg *config.RAGConfig, logger zerolog.Logger) *RAGService {
	return &RAGService{
		repo:   repo,
		llm:    llmSvc,
		config: cfg,
		logger: logger,
	}
}

func (s *RAGService) RetrieveMemories(ctx context.Context, sessionID uuid.UUID, query string, topK int) ([]*models.LongTermMemory, error) {
	if !s.config.Enabled {
		return nil, nil
	}

	if topK == 0 {
		topK = s.config.DefaultTopK
	}

	model := s.llm.GetRandomEmbeddingModel("")
	embedding, err := s.llm.EmbedOne(ctx, model, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	memories, err := s.repo.SearchLongTermMemory(ctx, sessionID, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	for _, mem := range memories {
		if err := s.repo.IncrementMemoryAccessCount(ctx, mem.ID); err != nil {
			s.logger.Warn().Err(err).Str("memory_id", mem.ID.String()).Msg("failed to increment access count")
		}
	}

	return memories, nil
}

func (s *RAGService) ExtractKeyTerms(ctx context.Context, text string, topN int) ([]string, error) {
	if topN == 0 {
		topN = s.config.AutoRetrieve.TopK
	}

	terms := extractKeywords(text, topN)

	s.logger.Debug().
		Strs("terms", terms).
		Msg("extracted key terms for retrieval")

	return terms, nil
}

func (s *RAGService) AutoRetrieve(ctx context.Context, sessionID uuid.UUID, contextText string) ([]*models.LongTermMemory, error) {
	if !s.config.Enabled || !s.config.AutoRetrieve.Enabled {
		return nil, nil
	}

	terms, err := s.ExtractKeyTerms(ctx, contextText, s.config.AutoRetrieve.TopK)
	if err != nil {
		return nil, err
	}

	var allMemories []*models.LongTermMemory
	seen := make(map[string]bool)

	for _, term := range terms {
		memories, err := s.RetrieveMemories(ctx, sessionID, term, s.config.AutoRetrieve.MaxResults)
		if err != nil {
			s.logger.Warn().Err(err).Str("term", term).Msg("failed to retrieve for term")
			continue
		}

		for _, mem := range memories {
			key := mem.ID.String()
			if !seen[key] {
				seen[key] = true
				allMemories = append(allMemories, mem)
			}
		}
	}

	maxResults := s.config.AutoRetrieve.MaxResults
	if len(allMemories) > maxResults {
		allMemories = allMemories[:maxResults]
	}

	return allMemories, nil
}

func (s *RAGService) BuildRetrievalContext(ctx context.Context, sessionID uuid.UUID, query string) (string, error) {
	memories, err := s.RetrieveMemories(ctx, sessionID, query, s.config.DefaultTopK)
	if err != nil {
		return "", err
	}

	if len(memories) == 0 {
		return "", nil
	}

	context := "Relevant context from memory:\n\n"
	for i, mem := range memories {
		context += fmt.Sprintf("[%d] (%s) %s\n", i+1, mem.Tier, mem.Content)
	}

	return context, nil
}

func extractKeywords(text string, topN int) []string {
	words := make([]string, 0)
	current := ""

	for _, ch := range text {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			current += string(ch)
		} else {
			if len(current) > 3 {
				words = append(words, current)
			}
			current = ""
		}
	}

	if len(current) > 3 {
		words = append(words, current)
	}

	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "her": true,
		"was": true, "one": true, "our": true, "out": true, "has": true,
		"have": true, "been": true, "were": true, "they": true, "this": true,
		"that": true, "with": true, "from": true, "your": true, "what": true,
	}

	filtered := make([]string, 0)
	for _, word := range words {
		lower := ""
		for _, ch := range word {
			lower += string(ch + 32)
		}
		if !stopWords[lower] {
			filtered = append(filtered, word)
		}
	}

	if len(filtered) <= topN {
		return filtered
	}
	return filtered[:topN]
}
