package evilginx

import (
	"context"
	"fmt"
	"strings"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

const defaultAIConversationTitle = "New conversation"

// AIProviderStatus - whether a provider has credentials configured server-side.
type AIProviderStatus struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// AIConfigStatus - sanitized server-side AI configuration. No API keys.
type AIConfigStatus struct {
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinking_level"`
	Valid         bool   `json:"valid"`
	Error         string `json:"error,omitempty"`
	SystemPrompt  string `json:"system_prompt,omitempty"`
}

// AIProviderSummary - provider availability plus the effective server config.
type AIProviderSummary struct {
	Providers []AIProviderStatus `json:"providers"`
	Config    AIConfigStatus     `json:"config"`
}

// AIConversationSummary - stable JSON surface for a conversation row.
type AIConversationSummary struct {
	ID                  string `json:"id"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
	OperatorName        string `json:"operator_name"`
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Title               string `json:"title"`
	Summary             string `json:"summary"`
	TurnState           string `json:"turn_state"`
	ActiveTurnID        string `json:"active_turn_id"`
	TargetSessionID     string `json:"target_session_id"`
	TargetBeaconID      string `json:"target_beacon_id"`
	ThinkingLevel       string `json:"thinking_level"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	ContextWindowTokens int64  `json:"context_window_tokens"`
	MessageCount        int    `json:"message_count"`
}

// AIConversationMessageSummary - one persisted message (chat, reasoning, tool).
type AIConversationMessageSummary struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Sequence       uint32 `json:"sequence"`
	Kind           string `json:"kind"`
	Visibility     string `json:"visibility"`
	State          string `json:"state"`
	TurnID         string `json:"turn_id"`
	FinishReason   string `json:"finish_reason,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ToolArguments  string `json:"tool_arguments,omitempty"`
	ToolResult     string `json:"tool_result,omitempty"`
	ErrorText      string `json:"error_text,omitempty"`
}

// GetAIProviders - configured providers + effective (keyless) config summary.
func (s *SliverBridge) GetAIProviders() (*AIProviderSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetAIProviders(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := &AIProviderSummary{Providers: []AIProviderStatus{}}
	for _, p := range resp.GetProviders() {
		out.Providers = append(out.Providers, AIProviderStatus{
			Name:       p.GetName(),
			Configured: p.GetConfigured(),
		})
	}
	if cfg := resp.GetConfig(); cfg != nil {
		out.Config = AIConfigStatus{
			Provider:      cfg.GetProvider(),
			Model:         cfg.GetModel(),
			ThinkingLevel: cfg.GetThinkingLevel(),
			Valid:         cfg.GetValid(),
			Error:         cfg.GetError(),
			SystemPrompt:  cfg.GetSystemPrompt(),
		}
	}
	return out, nil
}

// ListAIConversations - all persisted conversations (metadata only).
func (s *SliverBridge) ListAIConversations() ([]AIConversationSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetAIConversations(context.Background(), &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]AIConversationSummary, 0, len(resp.GetConversations()))
	for _, conv := range resp.GetConversations() {
		out = append(out, toAIConversationSummary(conv))
	}
	return out, nil
}

// GetAIConversation - a single conversation's metadata.
func (s *SliverBridge) GetAIConversation(id string) (*AIConversationSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	conv, err := c.GetAIConversation(context.Background(), &clientpb.AIConversationReq{ID: id})
	if err != nil {
		return nil, err
	}
	out := toAIConversationSummary(conv)
	return &out, nil
}

// CreateAIConversation - create a conversation using the server-configured
// provider/model (the dashboard never handles provider keys).
func (s *SliverBridge) CreateAIConversation(title, systemPrompt string) (*AIConversationSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}

	provider, model := "", ""
	if providers, perr := s.GetAIProviders(); perr == nil {
		provider = providers.Config.Provider
		model = providers.Config.Model
	}
	if strings.TrimSpace(provider) == "" {
		return nil, fmt.Errorf("sliver AI is not configured: set the provider and API key with the sliver console `ai-config` command")
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = defaultAIConversationTitle
	}

	conv, err := c.SaveAIConversation(context.Background(), &clientpb.AIConversation{
		Provider:     provider,
		Model:        model,
		Title:        title,
		SystemPrompt: strings.TrimSpace(systemPrompt),
	})
	if err != nil {
		return nil, err
	}
	out := toAIConversationSummary(conv)
	return &out, nil
}

// DeleteAIConversation - remove a conversation and its messages.
func (s *SliverBridge) DeleteAIConversation(id string) error {
	c, err := s.requireClient()
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("conversation id is required")
	}
	_, err = c.DeleteAIConversation(context.Background(), &clientpb.AIConversationReq{ID: id})
	return err
}

// GetAIConversationMessages - the full message list for a conversation. The
// server runs the model completion asynchronously after a user message is
// submitted, so callers poll this until an assistant reply appears.
func (s *SliverBridge) GetAIConversationMessages(id string) ([]AIConversationMessageSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	resp, err := c.GetAIConversationMessages(context.Background(), &clientpb.AIConversationReq{ID: id})
	if err != nil {
		return nil, err
	}
	out := make([]AIConversationMessageSummary, 0, len(resp.GetMessages()))
	for _, m := range resp.GetMessages() {
		out = append(out, toAIConversationMessageSummary(m))
	}
	return out, nil
}

// SendAIConversationMessage - persist a user message, which triggers the
// server-side agentic completion. Returns the saved user message.
func (s *SliverBridge) SendAIConversationMessage(id, content string) (*AIConversationMessageSummary, error) {
	c, err := s.requireClient()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("conversation id is required")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("message content is required")
	}
	saved, err := c.SaveAIConversationMessage(context.Background(), &clientpb.AIConversationMessage{
		ConversationID: id,
		Role:           "user",
		Content:        content,
	})
	if err != nil {
		return nil, err
	}
	out := toAIConversationMessageSummary(saved)
	return &out, nil
}

// --- converters ---

func toAIConversationSummary(conv *clientpb.AIConversation) AIConversationSummary {
	out := AIConversationSummary{
		ID:              conv.GetID(),
		CreatedAt:       conv.GetCreatedAt(),
		UpdatedAt:       conv.GetUpdatedAt(),
		OperatorName:    conv.GetOperatorName(),
		Provider:        conv.GetProvider(),
		Model:           conv.GetModel(),
		Title:           conv.GetTitle(),
		Summary:         conv.GetSummary(),
		TurnState:       conv.GetTurnState().String(),
		ActiveTurnID:    conv.GetActiveTurnID(),
		TargetSessionID: conv.GetTargetSessionID(),
		TargetBeaconID:  conv.GetTargetBeaconID(),
		ThinkingLevel:   conv.GetThinkingLevel(),
		MessageCount:    len(conv.GetMessages()),
	}
	if usage := conv.GetContextWindowUsage(); usage != nil {
		out.InputTokens = usage.GetInputTokens()
		out.OutputTokens = usage.GetOutputTokens()
		out.TotalTokens = usage.GetTotalTokens()
		out.ContextWindowTokens = usage.GetContextWindowTokens()
	}
	return out
}

func toAIConversationMessageSummary(m *clientpb.AIConversationMessage) AIConversationMessageSummary {
	return AIConversationMessageSummary{
		ID:             m.GetID(),
		ConversationID: m.GetConversationID(),
		CreatedAt:      m.GetCreatedAt(),
		UpdatedAt:      m.GetUpdatedAt(),
		Role:           m.GetRole(),
		Content:        m.GetContent(),
		Provider:       m.GetProvider(),
		Model:          m.GetModel(),
		Sequence:       m.GetSequence(),
		Kind:           m.GetKind().String(),
		Visibility:     m.GetVisibility().String(),
		State:          m.GetState().String(),
		TurnID:         m.GetTurnID(),
		FinishReason:   m.GetFinishReason(),
		ToolName:       m.GetToolName(),
		ToolArguments:  m.GetToolArguments(),
		ToolResult:     m.GetToolResult(),
		ErrorText:      m.GetErrorText(),
	}
}
