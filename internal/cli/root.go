package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/FuadTesfaye/layerpilot/internal/version"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	verbose    bool
	configPath string
)

var rootCmd = &cobra.Command{
	Use:   "compat",
	Short: "Universal Application Compatibility Manager",
	Long: `compat is a reliable, cross-platform application compatibility manager built in Go.
It evaluates host environments, selects verified execution strategies, manages application lifecycles,
and provides transparent failure diagnostics.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute executes the root CLI command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output results in machine-readable JSON format")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose diagnostic logs")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to custom configuration file")

	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information and compatibility catalogue version",
	RunE: func(cmd *cobra.Command, args []string) error {
		info := version.Get()
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(info)
		}
		fmt.Println(info.String())
		return nil
	},
}
