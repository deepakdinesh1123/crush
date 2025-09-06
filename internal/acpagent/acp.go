package acpagent

import (
	"context"
	"fmt"

	"github.com/charmbracelet/crush/internal/app"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/pubsub"
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
				EmbeddedContext: true,
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

func (a *ACPAgent) LoadSession(ctx context.Context, lsr acp.LoadSessionRequest) error { return nil }

func (a *ACPAgent) Cancel(ctx context.Context, params acp.CancelNotification) error {
	_, err := a.app.Sessions.Get(ctx, string(params.SessionId))
	if err != nil {
		return err
	} else {
		a.app.Sessions.Delete(ctx, string(params.SessionId))
	}
	return nil
}

func (a *ACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	_, err := a.app.Sessions.Get(ctx, string(params.SessionId))
	if err != nil {
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", string(params.SessionId))
	}

	// cancel any previous turn
	// if s.cancel != nil {
	// 	s.cancel()
	// }
	ctx = context.Background()
	// s.cancel = cancel

	content := ""

	for _, cont := range params.Prompt {
		content = content + cont.Text.Text + " "
	}
	_, err = a.app.CoderAgent.Run(ctx, string(params.SessionId), content)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	for {
		select {
		case <-a.app.EventsCtx.Done():
			return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
		case event := <-a.app.Events:
			switch event := event.(type) {
			case pubsub.Event[message.Message]:
				a.conn.SessionUpdate(ctx, acp.SessionNotification{
					SessionId: params.SessionId,
					Update: acp.SessionUpdate{
						UserMessageChunk: &acp.SessionUpdateUserMessageChunk{
							SessionUpdate: "agent_message_chunk",
							Content: acp.ContentBlock{
								Text: &acp.ContentBlockText{
									Text: event.Payload.Content().Text,
									Type: "text",
								},
							},
						},
					},
				})
			}
		default:
		}
	}

}

func (a *ACPAgent) Authenticate(ctx context.Context, _ acp.AuthenticateRequest) error { return nil }
