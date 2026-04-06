package ai

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIAdapter implements Provider using OpenAI-compatible APIs.
type OpenAIAdapter struct {
	client *openai.Client
	model  string
}

// NewOpenAIAdapter creates a new OpenAI adapter.
func NewOpenAIAdapter(apiKey, model, baseURL string) *OpenAIAdapter {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL + "/v1"
	}

	if model == "" {
		model = "gpt-4o"
	}

	return &OpenAIAdapter{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

const systemPrompt = `You are a code merge assistant. Your task is to merge upstream changes while preserving the user's local modifications.

Rules:
1. Preserve the user's local changes/patches
2. Accept upstream changes that don't conflict with local changes
3. If you cannot determine which version to use for a conflict block, mark it with [NEEDS_MANUAL_REVIEW]
4. Output the COMPLETE merged file content
5. After the merged content, add a brief explanation of your changes`

func buildUserPrompt(req ConflictRequest) string {
	langInfo := ""
	if req.Language != "" {
		langInfo = fmt.Sprintf("\nLanguage: %s", req.Language)
	}

	diffInfo := ""
	if req.UserDiff != "" {
		diffInfo = fmt.Sprintf("\n\nUser's local changes (diff):\n%s", req.UserDiff)
	}

	return fmt.Sprintf(`File: %s%s

Conflict file content:
%s%s

Please output the complete resolved file content, followed by a brief explanation of your changes.`,
		req.FilePath, langInfo, req.ConflictContent, diffInfo)
}

// ResolveConflicts sends conflict content to OpenAI for resolution.
func (a *OpenAIAdapter) ResolveConflicts(ctx context.Context, req ConflictRequest) (*ConflictResolution, error) {
	resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: a.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: buildUserPrompt(req)},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("openai api error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai returned no choices")
	}

	content := resp.Choices[0].Message.Content

	// Check for manual review markers
	needsReview := strings.Contains(content, "[NEEDS_MANUAL_REVIEW]")

	// Validate: no conflict markers should remain
	if strings.Contains(content, "<<<<<<<") ||
		(strings.Contains(content, "=======") && strings.Contains(content, ">>>>>>>")) {
		return nil, fmt.Errorf("AI output still contains conflict markers")
	}

	// Separate merged content from explanation
	// The AI is asked to output merged content then explanation
	// We'll treat the whole thing as merged content for now
	return &ConflictResolution{
		MergedContent: content,
		Explanation:   "",
		NeedsReview:   needsReview,
	}, nil
}
