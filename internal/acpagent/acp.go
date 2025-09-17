package acpagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/llm/agent"
	"github.com/charmbracelet/crush/internal/message"
	acp "github.com/zed-industries/agent-client-protocol/go"
)

type ACPAgent struct {
	app  *app.App
	conn *acp.AgentSideConnection
}

func NewACPAgent(app *app.App) (*ACPAgent, error) {
	return &ACPAgent{
		app: app,
	}, nil
}

func (a *ACPAgent) SetAgentConnection(conn *acp.AgentSideConnection) { a.conn = conn }

func (a *ACPAgent) Initialize(ctx context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false,
			PromptCapabilities: acp.PromptCapabilities{
				EmbeddedContext: false,
			},
		},
	}, nil
}

func (a *ACPAgent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	sesh, err := a.app.Sessions.Create(ctx, "new acp session")
	if err != nil {
		return acp.NewSessionResponse{}, err
	}
	return acp.NewSessionResponse{SessionId: acp.SessionId(sesh.ID)}, nil
}

func (a *ACPAgent) LoadSession(ctx context.Context, lsr acp.LoadSessionRequest) error {
	return nil
}

func (a *ACPAgent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	_, err := a.app.Sessions.Get(ctx, string(params.SessionId))
	if err != nil {
		return err
	}

	if a.app.CoderAgent != nil {
		a.app.CoderAgent.Cancel(string(params.SessionId))
	}

	return nil
}

func (a *ACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	_, err := a.app.Sessions.Get(ctx, string(params.SessionId))
	if err != nil {
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", string(params.SessionId))
	}

	content := ""
	for _, cont := range params.Prompt {
		if cont.Text != nil {
			content += cont.Text.Text + " "
		}
	}

	done, err := a.app.CoderAgent.Run(context.Background(), string(params.SessionId), content)
	if err != nil {
		return acp.PromptResponse{}, err
	}

	messageEvents := a.app.Messages.Subscribe(ctx)

	// Track sent content to only send deltas
	var lastTextSent string
	var lastThinkingSent string

	for {
		select {
		case result := <-done:
			if result.Error != nil {
				if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, agent.ErrRequestCancelled) {
					slog.Info("agent processing cancelled", "session_id", params.SessionId)
					return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
				}
				return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
			}
		case event, ok := <-messageEvents:
			if !ok {
				// Stream closed, agent finished
				return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
			}

			switch event.Payload.Role {
			case message.Assistant:
				for _, part := range event.Payload.Parts {
					switch part := part.(type) {
					case message.ReasoningContent:
						// Only send the delta (new thinking content)
						if len(part.Thinking) > len(lastThinkingSent) {
							delta := part.Thinking[len(lastThinkingSent):]
							if a.conn != nil && delta != "" {
								if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
									SessionId: params.SessionId,
									Update: acp.SessionUpdate{
										AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
											SessionUpdate: "agent_thought_chunk",
											Content: acp.ContentBlock{
												Text: &acp.ContentBlockText{
													Text: delta,
													Type: "text",
												},
											},
										},
									},
								}); err != nil {
									slog.Error("error sending agent thought chunk", err)
									continue
								}
							}
							lastThinkingSent = part.Thinking
						}
					case message.BinaryContent:
					case message.ImageURLContent:
					case message.Finish:
					case message.TextContent:
						// Only send the delta (new text content)
						if len(part.Text) > len(lastTextSent) {
							delta := part.Text[len(lastTextSent):]
							if a.conn != nil && delta != "" {
								if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
									SessionId: params.SessionId,
									Update: acp.SessionUpdate{
										AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
											SessionUpdate: "agent_message_chunk",
											Content: acp.ContentBlock{
												Text: &acp.ContentBlockText{
													Text: delta,
													Type: "text",
												},
											},
										},
									},
								}); err != nil {
									slog.Error("error sending agent text chunk", err)
									continue
								}
							}
							lastTextSent = part.Text
						}
					case message.ToolCall:
					case message.ToolResult:
					}
				}
			case message.System:
			case message.Tool:
			case message.User:
			}
		}
	}

}

func (a *ACPAgent) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) error { return nil }
