package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/crush/internal/acpagent"
	"github.com/spf13/cobra"
	acp "github.com/zed-industries/agent-client-protocol/go"
)

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Start the crush agent in ACP mode",
	Long: `Start the crush agent in ACP mode

	Allows crush to be connected with ACP compliant clients such as zed

	`,
	RunE: func(cmd *cobra.Command, args []string) error {

		app, err := setupApp(cmd)
		if err != nil {
			return err
		}
		defer app.Shutdown()

		if !app.Config().IsConfigured() {
			return fmt.Errorf("no providers configured - please run 'crush' to set up a provider interactively")
		}

		ag, err := acpagent.NewACPAgent(app)
		if err != nil {
			return err
		}
		asc := acp.NewAgentSideConnection(ag, os.Stdout, os.Stdin)
		ag.SetAgentConnection(asc)

		// Block until the peer disconnects (stdin closes).
		<-asc.Done()

		return nil
	},
}
