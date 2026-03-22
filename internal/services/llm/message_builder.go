package llm

// MessageBuilder helps construct OpenAI-compliant chat message arrays
type MessageBuilder struct {
	messages []ChatMessage
}

// NewMessageBuilder creates a new message builder with an optional system prompt
func NewMessageBuilder(systemPrompt string) *MessageBuilder {
	m := &MessageBuilder{
		messages: make([]ChatMessage, 0),
	}
	if systemPrompt != "" {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: systemPrompt})
	}
	return m
}

// AddUserMessage adds a user message
func (m *MessageBuilder) AddUserMessage(content string) *MessageBuilder {
	m.messages = append(m.messages, ChatMessage{Role: "user", Content: content})
	return m
}

// AddAssistantMessage adds an assistant message with optional tool calls
func (m *MessageBuilder) AddAssistantMessage(content string, toolCalls []ToolCall) *MessageBuilder {
	msg := ChatMessage{Role: "assistant", Content: content}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	m.messages = append(m.messages, msg)
	return m
}

// AddToolResult adds a tool result message
func (m *MessageBuilder) AddToolResult(toolCallID, result string) *MessageBuilder {
	m.messages = append(m.messages, ChatMessage{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    result,
	})
	return m
}

// AddMessages appends existing messages (for history reconstruction)
func (m *MessageBuilder) AddMessages(messages []ChatMessage) *MessageBuilder {
	m.messages = append(m.messages, messages...)
	return m
}

// Build returns the constructed message array
func (m *MessageBuilder) Build() []ChatMessage {
	return m.messages
}

// Len returns the number of messages
func (m *MessageBuilder) Len() int {
	return len(m.messages)
}