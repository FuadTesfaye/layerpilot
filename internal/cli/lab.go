package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/FuadTesfaye/layerpilot/internal/lab"
	"github.com/spf13/cobra"
)

var (
	labRuntimeFlag   string
	labTargetsFlag   string
	labScenariosFlag string
	labOutDirFlag    string
	labNetworkFlag   bool
	labTimeoutFlag   int

	verifyScenarioFlag string
	verifyArtifactFlag string
	verifyOutDirFlag   string
)

var labCmd = &cobra.Command{
	Use:   "lab",
	Short: "Multi-distro app testing matrix and AI-agent context generator",
	Long: `compat lab enables developers to test Linux applications across pinned
distributions (Fedora, Ubuntu, Debian, Arch, openSUSE) from macOS, Windows, or Linux,
and produces structured context bundles for AI coding agents to fix failures in a loop.`,
}

var labDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Detect host container runtimes, virtualization, and prerequisites",
	RunE: func(cmd *cobra.Command, args []string) error {
		doc := lab.NewDoctor()
		rep := doc.Diagnose(cmd.Context())

		if jsonOutput {
			return printJSON(rep)
		}

		fmt.Println("=== Compat Lab Host Diagnostics ===")
		fmt.Printf("OS / Architecture: %s / %s\n", rep.HostOS, rep.HostArch)
		if rep.Virtualization {
			fmt.Println("Virtualization / KVM: Available")
		} else {
			fmt.Println("Virtualization / KVM: Not detected (containers may run without hardware acceleration)")
		}
		if rep.FreeDiskBytes > 0 || rep.FreeRAMBytes > 0 {
			freeDiskGB := float64(rep.FreeDiskBytes) / (1024 * 1024 * 1024)
			freeRAMGB := float64(rep.FreeRAMBytes) / (1024 * 1024 * 1024)
			fmt.Printf("Free Resources:       %.1f GB Disk, %.1f GB RAM\n", freeDiskGB, freeRAMGB)
		}
		fmt.Println()

		fmt.Println("Container Runtimes Detected:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "RUNTIME\tAVAILABLE\tVERSION / STATUS")
		for _, rt := range rep.Runtimes {
			avail := "No"
			if rt.Available {
				avail = "Yes"
			}
			details := rt.Version
			if rt.Details != "" {
				if details != "" {
					details += " (" + rt.Details + ")"
				} else {
					details = rt.Details
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", rt.ID, avail, details)
		}
		w.Flush()
		fmt.Println()

		if rep.RecommendedRuntime != "" {
			fmt.Printf("Recommended Runtime: %s\n", rep.RecommendedRuntime)
		} else {
			fmt.Println("No container runtime available.")
			fmt.Println()
			fmt.Println("Installation instructions:")
			for _, rt := range rep.Runtimes {
				if rt.InstallInstructions != "" {
					fmt.Printf("- %s:\n%s\n\n", rt.Name, rt.InstallInstructions)
				}
			}
		}

		return nil
	},
}

var labRunCmd = &cobra.Command{
	Use:   "run <artifact-file>",
	Short: "Run compatibility test matrix across Linux distributions",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		artifactPath := args[0]

		// 1. Resolve targets
		var targetNames []string
		if labTargetsFlag != "" {
			for _, t := range strings.Split(labTargetsFlag, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					targetNames = append(targetNames, t)
				}
			}
		}
		targets, err := lab.FilterTargets(targetNames)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(3)
		}

		// 2. Resolve scenarios
		var scenarioIDs []string
		if labScenariosFlag != "" {
			for _, s := range strings.Split(labScenariosFlag, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					scenarioIDs = append(scenarioIDs, s)
				}
			}
		}
		scenarios := lab.FilterScenarios(scenarioIDs)

		// 3. Resolve container runtime
		var runtime lab.ContainerRuntime
		if labRuntimeFlag != "" {
			switch strings.ToLower(labRuntimeFlag) {
			case "podman":
				runtime = lab.NewPodmanContainerRuntime()
			case "docker":
				runtime = lab.NewDockerContainerRuntime()
			case "mock":
				runtime = lab.NewMockContainerRuntime()
			default:
				fmt.Fprintf(os.Stderr, "Error: unsupported runtime '%s' (use podman, docker, or mock)\n", labRuntimeFlag)
				os.Exit(3)
			}
		}

		outDir := labOutDirFlag
		if outDir == "" {
			outDir = "compat-report"
		}

		timeout := time.Duration(labTimeoutFlag) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}

		// 4. Run test suite
		report, err := lab.RunLabSuite(cmd.Context(), lab.RunLabOptions{
			ArtifactPath: artifactPath,
			Targets:      targets,
			Scenarios:    scenarios,
			OutputDir:    outDir,
			Runtime:      runtime,
			Network:      labNetworkFlag,
			Timeout:      timeout,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Lab execution error: %v\n", err)
			os.Exit(3)
		}

		if jsonOutput {
			_ = printJSON(report)
		} else {
			printReportTerminalSummary(report, outDir)
		}

		// Determine exit code:
		// 0: all pass
		// 1: any fail
		// 2: inconclusive / not-testable only
		if report.Summary.Failed > 0 {
			os.Exit(1)
		}
		if report.Summary.Inconclusive > 0 || report.Summary.NotTestable > 0 {
			os.Exit(2)
		}
		os.Exit(0)
	},
}

func printReportTerminalSummary(report *lab.LabReport, outDir string) {
	fmt.Println("=== Compat Lab Test Matrix Results ===")
	fmt.Printf("Artifact:   %s (%d bytes, SHA-256: %s...)\n", filepath.Base(report.ArtifactPath), report.ArtifactSize, report.ArtifactSHA256[:12])
	fmt.Printf("Runtime:    %s (Emulation: %v)\n", report.RuntimeID, report.IsEmulated)
	fmt.Printf("Total Tests: %d (Passed: %d, Failed: %d, Inconclusive: %d, Not Testable: %d)\n\n",
		report.Summary.TotalScenarios, report.Summary.Passed, report.Summary.Failed, report.Summary.Inconclusive, report.Summary.NotTestable)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TARGET\tSCENARIO\tTIER\tVERDICT\tCAUSE / NOTE")
	for _, r := range report.Results {
		cause := r.Cause
		if len(cause) > 60 {
			cause = cause[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.TargetID, r.ScenarioID, r.Tier, strings.ToUpper(string(r.Verdict)), cause)
	}
	w.Flush()
	fmt.Println()

	if report.Summary.Failed > 0 {
		fmt.Printf("❌ Failed scenarios: %d. Inspection reports and AI context bundle written to:\n", report.Summary.Failed)
	} else if report.Summary.Inconclusive > 0 || report.Summary.NotTestable > 0 {
		fmt.Printf("⚠️  Inconclusive/Untestable scenarios: %d. Details written to:\n", report.Summary.Inconclusive+report.Summary.NotTestable)
	} else {
		fmt.Println("✅ All compatibility scenarios passed cleanly! Context bundle written to:")
	}
	fmt.Printf("   Bundle:        %s/\n", outDir)
	fmt.Printf("   Human Report:  %s/REPORT.md\n", outDir)
	fmt.Printf("   AI Brief:      %s/AGENT_CONTEXT.md\n\n", outDir)
	fmt.Printf("Hand this bundle to an AI agent:\n   compat lab context %s | <your-agent-cli>\n", outDir)
}

var labReportCmd = &cobra.Command{
	Use:   "report [dir]",
	Short: "Display compatibility lab report summary from an output directory",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "compat-report"
		if len(args) > 0 {
			dir = args[0]
		}

		reportPath := dir
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			reportPath = filepath.Join(dir, "report.json")
		}

		data, err := os.ReadFile(reportPath)
		if err != nil {
			return fmt.Errorf("failed reading report from %s: %w", reportPath, err)
		}

		var rep lab.LabReport
		if err := json.Unmarshal(data, &rep); err != nil {
			return fmt.Errorf("failed parsing report.json: %w", err)
		}

		if jsonOutput {
			return printJSON(rep)
		}

		printReportTerminalSummary(&rep, dir)
		return nil
	},
}

var labContextCmd = &cobra.Command{
	Use:   "context [dir]",
	Short: "Output strict 7-section AGENT_CONTEXT.md for AI coding agent handoff",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "compat-report"
		if len(args) > 0 {
			dir = args[0]
		}

		contextPath := dir
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			contextPath = filepath.Join(dir, "AGENT_CONTEXT.md")
		}

		data, err := os.ReadFile(contextPath)
		if err != nil {
			return fmt.Errorf("failed reading agent context from %s: %w", contextPath, err)
		}

		fmt.Print(string(data))
		return nil
	},
}

var labVerifyFixCmd = &cobra.Command{
	Use:   "verify-fix <report-or-dir>",
	Short: "Verify whether an AI agent fix succeeded without regressing other scenarios",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		origReport := args[0]

		if verifyArtifactFlag == "" {
			fmt.Fprintln(os.Stderr, "Error: --artifact flag is required to specify the candidate fixed build")
			os.Exit(3)
		}

		res, err := lab.VerifyFix(cmd.Context(), lab.VerificationRequest{
			OriginalReportPath: origReport,
			NewArtifactPath:    verifyArtifactFlag,
			TargetScenarioID:   verifyScenarioFlag,
			OutputDir:          verifyOutDirFlag,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Verification runtime error: %v\n", err)
			os.Exit(3)
		}

		if jsonOutput {
			_ = printJSON(res)
		} else {
			fmt.Println("=== Compat Lab Fix Verification ===")
			fmt.Printf("Target Scenario:   %s\n", res.ScenarioID)
			fmt.Printf("Original Verdict:  %s\n", res.OriginalVerdict)
			fmt.Printf("New Verdict:       %s\n", res.NewVerdict)
			fmt.Printf("Regressions:       %d\n", res.RegressionsCount)
			for _, reg := range res.RegressionDetails {
				fmt.Printf("  - %s\n", reg)
			}
			fmt.Println()
			fmt.Println(res.SummaryMessage)
		}

		if res.Success {
			os.Exit(0)
		}
		os.Exit(1)
	},
}

func init() {
	labCmd.AddCommand(labDoctorCmd)
	labCmd.AddCommand(labRunCmd)
	labCmd.AddCommand(labReportCmd)
	labCmd.AddCommand(labContextCmd)
	labCmd.AddCommand(labVerifyFixCmd)

	labRunCmd.Flags().StringVar(&labRuntimeFlag, "runtime", "", "Container runtime backend to use (podman, docker, mock)")
	labRunCmd.Flags().StringVar(&labTargetsFlag, "targets", "", "Comma-separated list of target distros (fedora:42, ubuntu:24.04, debian:12, archlinux, opensuse:15.6)")
	labRunCmd.Flags().StringVar(&labScenariosFlag, "scenarios", "", "Comma-separated list of scenario IDs to run")
	labRunCmd.Flags().StringVar(&labOutDirFlag, "out", "compat-report", "Directory where reports, logs, and screenshots are written")
	labRunCmd.Flags().BoolVar(&labNetworkFlag, "network", false, "Enable network connectivity in test containers (default: off)")
	labRunCmd.Flags().IntVar(&labTimeoutFlag, "timeout", 30, "Scenario execution timeout in seconds")

	labVerifyFixCmd.Flags().StringVar(&verifyScenarioFlag, "scenario", "", "Scenario ID that originally failed")
	labVerifyFixCmd.Flags().StringVar(&verifyArtifactFlag, "artifact", "", "Path to newly built candidate artifact to verify")
	labVerifyFixCmd.Flags().StringVar(&verifyOutDirFlag, "out", "", "Output directory for verification report")
}
