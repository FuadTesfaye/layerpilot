package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/app"
	"github.com/FuadTesfaye/layerpilot/internal/doctor"
	"github.com/spf13/cobra"
)

var autoApprove bool
var logLines int
var dryRun bool

func init() {
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(supportCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(launchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(engineCmd)
	rootCmd.AddCommand(cleanupCmd)

	installCmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Automatically approve proposed plan")
	uninstallCmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "Automatically approve uninstallation")
	logsCmd.Flags().IntVarP(&logLines, "lines", "n", 50, "Number of log lines to retrieve")
	cleanupCmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview resources eligible for cleanup without deleting")

	engineCmd.AddCommand(engineListCmd)
	engineCmd.AddCommand(engineDoctorCmd)
}

func getWorkflow() (*app.Workflow, error) {
	return app.NewDefaultWorkflow()
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <package-file>",
	Short: "Inspect package format, architecture, and metadata without installing",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		info, err := wf.Inspect(context.Background(), args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(info)
		}

		fmt.Println("Package Inspection:")
		fmt.Printf("  Name:         %s\n", info.Name)
		fmt.Printf("  Format:       %s\n", info.Format)
		fmt.Printf("  Architecture: %s\n", info.Architecture)
		fmt.Printf("  Type:         %s\n", info.PackageType)
		fmt.Printf("  Size:         %d bytes\n", info.SizeBytes)
		fmt.Printf("  SHA256:       %s\n", info.SHA256)
		if info.Version != "" {
			fmt.Printf("  Version:      %s\n", info.Version)
		}
		if info.Description != "" {
			fmt.Printf("  Description:  %s\n", info.Description)
		}
		if len(info.Dependencies) > 0 {
			fmt.Printf("  Dependencies: %s\n", strings.Join(info.Dependencies, ", "))
		}
		fmt.Println("\nNo system changes were made.")
		return nil
	},
}

var supportCmd = &cobra.Command{
	Use:   "support <package-file>",
	Short: "Show applicable compatibility strategies, support tier, and limitations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		decision, err := wf.Support(context.Background(), args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(decision)
		}

		fmt.Println("Compatibility Support Evaluation:")
		fmt.Printf("  Support Tier:     %s\n", decision.SupportTier)
		fmt.Printf("  Candidate Engine: %s\n", decision.EngineID)
		if len(decision.ReasonCodes) > 0 {
			fmt.Printf("  Reason Codes:     %s\n", strings.Join(decision.ReasonCodes, ", "))
		}
		if len(decision.Limitations) > 0 {
			fmt.Println("  Limitations:")
			for _, lim := range decision.Limitations {
				fmt.Printf("    - %s\n", lim)
			}
		}
		if len(decision.RequiredActions) > 0 {
			fmt.Println("  Required Actions:")
			for _, act := range decision.RequiredActions {
				fmt.Printf("    - %s\n", act)
			}
		}
		return nil
	},
}

var planCmd = &cobra.Command{
	Use:   "plan <package-file>",
	Short: "Build an explicit previewable action plan without applying changes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		plan, err := wf.Plan(context.Background(), args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(plan)
		}

		fmt.Printf("Proposed Installation Plan for %s:\n", plan.Package.Name)
		fmt.Printf("  Plan ID:      %s\n", plan.PlanID)
		fmt.Printf("  App ID:       %s\n", plan.AppID)
		fmt.Printf("  Engine:       %s\n", plan.Decision.EngineID)
		fmt.Printf("  Support Tier: %s\n\n", plan.Decision.SupportTier)

		fmt.Println("Planned Actions:")
		for _, step := range plan.Steps {
			fmt.Printf("  %d. [%s] %s\n", step.Order, step.Name, step.Description)
			if step.HostImpact != "" {
				fmt.Printf("     Impact: %s\n", step.HostImpact)
			}
		}

		if len(plan.HostChanges) > 0 {
			fmt.Println("\nHost Changes:")
			for _, hc := range plan.HostChanges {
				fmt.Printf("  - %s\n", hc)
			}
		}

		fmt.Println("\nPrivileged Operations: None requested")
		fmt.Println("Run 'compat install <file>' to preview and apply this plan.")
		return nil
	},
}

var installCmd = &cobra.Command{
	Use:   "install <package-file>",
	Short: "Install a package through the selected compatibility engine",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		ctx := context.Background()

		// Preview plan first
		plan, err := wf.Plan(ctx, args[0])
		if err != nil {
			return err
		}

		if !autoApprove {
			fmt.Printf("Proposed Plan for %s via %s:\n", plan.Package.Name, plan.Decision.EngineID)
			for _, step := range plan.Steps {
				fmt.Printf("  %d. %s\n", step.Order, step.Description)
			}
			fmt.Printf("\nApprove installation? [y/N]: ")
			var response string
			_, _ = fmt.Scanln(&response)
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Println("Installation cancelled by user.")
				return nil
			}
		}

		fmt.Printf("Applying installation plan via %s...\n", plan.Decision.EngineID)
		record, err := wf.Install(ctx, args[0], true)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(record)
		}

		fmt.Printf("\n✓ Successfully installed %s (ID: %s)\n", record.DisplayName, record.AppID)
		fmt.Printf("  Engine:   %s\n", record.EngineID)
		fmt.Printf("  Status:   %s\n", record.Status)
		if record.LauncherPath != "" {
			fmt.Printf("  Launcher: %s\n", record.LauncherPath)
		}
		fmt.Printf("\nTo launch: compat launch %s\n", record.AppID)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all managed applications and their status",
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		apps, err := wf.List(context.Background())
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(apps)
		}

		if len(apps) == 0 {
			fmt.Println("No applications currently managed by compat.")
			fmt.Println("Install an application using 'compat install <package-file>'")
			return nil
		}

		fmt.Printf("%-24s %-16s %-10s %-12s %-12s\n", "APP ID", "NAME", "FORMAT", "ENGINE", "STATUS")
		fmt.Println(strings.Repeat("-", 78))
		for _, a := range apps {
			fmt.Printf("%-24s %-16s %-10s %-12s %-12s\n", a.AppID, a.DisplayName, a.PackageFormat, a.EngineID, a.Status)
		}
		return nil
	},
}

var launchCmd = &cobra.Command{
	Use:   "launch <app-id> [-- <args>...]",
	Short: "Launch an installed application and pass application arguments",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		appID := args[0]
		appArgs := args[1:]

		return wf.Launch(context.Background(), appID, appArgs)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status <app-id>",
	Short: "Report an application's current installation and runtime status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		status, err := wf.Status(context.Background(), args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]string{
				"app_id": args[0],
				"status": string(status),
			})
		}

		fmt.Printf("Application: %s\nStatus:      %s\n", args[0], status)
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs <app-id>",
	Short: "Display application and engine runtime logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		logs, err := wf.Logs(context.Background(), args[0], logLines)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(logs)
		}

		for _, line := range logs {
			fmt.Println(line)
		}
		return nil
	},
}

var explainCmd = &cobra.Command{
	Use:   "explain <app-id>",
	Short: "Explain the selected compatibility strategy, environment, and limitations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		app, err := wf.Explain(context.Background(), args[0])
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(app)
		}

		fmt.Printf("Application Explanation for %s:\n", app.DisplayName)
		fmt.Printf("  App ID:          %s\n", app.AppID)
		fmt.Printf("  Format:          %s (%s)\n", app.PackageFormat, app.PackageArch)
		fmt.Printf("  Engine:          %s\n", app.EngineID)
		fmt.Printf("  Host Distro:     %s (%s)\n", app.HostDistro, app.HostOS)
		fmt.Printf("  Status:          %s\n", app.Status)
		fmt.Printf("  Installed At:    %s\n", app.InstalledAt)
		if app.LastLaunchedAt != nil {
			fmt.Printf("  Last Launched:   %s\n", app.LastLaunchedAt)
		}
		return nil
	},
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall <app-id>",
	Short: "Remove an application and all owned resources",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		appID := args[0]
		if !autoApprove {
			fmt.Printf("Are you sure you want to uninstall '%s' and remove all associated resources? [y/N]: ", appID)
			var resp string
			_, _ = fmt.Scanln(&resp)
			if strings.ToLower(resp) != "y" && strings.ToLower(resp) != "yes" {
				fmt.Println("Uninstall cancelled.")
				return nil
			}
		}

		if err := wf.Uninstall(context.Background(), appID, true); err != nil {
			return err
		}

		fmt.Printf("✓ Successfully uninstalled %s and cleaned up all owned resources.\n", appID)
		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose host environment, engines, permissions, and storage readiness",
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		report := wf.Doctor(context.Background())
		if jsonOutput {
			return printJSON(report)
		}

		fmt.Println("Compat Diagnostic Doctor:")
		fmt.Println("=========================")
		for _, c := range report.Checks {
			icon := "[✓]"
			if c.Status == doctor.StatusWarn {
				icon = "[!]"
			} else if c.Status == doctor.StatusFail {
				icon = "[✗]"
			}
			fmt.Printf("%s %-12s: %s\n", icon, c.Name, c.Message)
			if c.Impact != "" {
				fmt.Printf("    Impact:     %s\n", c.Impact)
			}
			if c.Suggestion != "" {
				fmt.Printf("    Suggestion: %s\n", c.Suggestion)
			}
		}

		if report.Healthy {
			fmt.Println("\n✓ System is healthy and ready for compatibility workflows.")
		} else {
			fmt.Println("\n✗ Issues detected that may block specific compatibility workflows.")
		}
		return nil
	},
}

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Manage and inspect compatibility engines",
}

var engineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered engines and their availability",
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		reports := wf.EngineList(context.Background())
		if jsonOutput {
			return printJSON(reports)
		}

		fmt.Printf("%-10s %-36s %-12s %s\n", "ENGINE ID", "NAME", "AVAILABLE", "DETAILS")
		fmt.Println(strings.Repeat("-", 80))
		for id, r := range reports {
			avail := "No"
			if r.Available {
				avail = "Yes"
			}
			details := r.Version
			if !r.Available && len(r.MissingPrerequisites) > 0 {
				details = strings.Join(r.MissingPrerequisites, "; ")
			}
			fmt.Printf("%-10s %-36s %-12s %s\n", id, r.EngineName, avail, details)
		}
		return nil
	},
}

var engineDoctorCmd = &cobra.Command{
	Use:   "doctor <engine-id>",
	Short: "Diagnose one engine in detail",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		reports := wf.EngineList(context.Background())
		r, ok := reports[args[0]]
		if !ok {
			return fmt.Errorf("engine '%s' not recognized", args[0])
		}

		if jsonOutput {
			return printJSON(r)
		}

		fmt.Printf("Engine Doctor: %s (%s)\n", r.EngineName, r.EngineID)
		fmt.Printf("  Available: %v\n", r.Available)
		if r.Version != "" {
			fmt.Printf("  Version:   %s\n", r.Version)
		}
		if len(r.SupportedFormats) > 0 {
			var fmts []string
			for _, f := range r.SupportedFormats {
				fmts = append(fmts, string(f))
			}
			fmt.Printf("  Formats:   %s\n", strings.Join(fmts, ", "))
		}
		if len(r.MissingPrerequisites) > 0 {
			fmt.Println("  Missing Prerequisites:")
			for _, p := range r.MissingPrerequisites {
				fmt.Printf("    - %s\n", p)
			}
		}
		return nil
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Preview or clean unused resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		wf, err := getWorkflow()
		if err != nil {
			return err
		}
		defer wf.Close()

		actions, err := wf.Cleanup(context.Background(), dryRun)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(actions)
		}

		for _, a := range actions {
			fmt.Println(a)
		}
		return nil
	},
}
