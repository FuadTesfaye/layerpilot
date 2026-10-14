package planner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
	"github.com/FuadTesfaye/layerpilot/internal/engine"
	"github.com/FuadTesfaye/layerpilot/internal/matrix"
)

// Planner orchestrates deterministic compatibility planning
type Planner struct {
	matrix   *matrix.PolicyEngine
	registry *engine.Registry
}

// New creates a new Planner instance
func New(pe *matrix.PolicyEngine, reg *engine.Registry) *Planner {
	return &Planner{
		matrix:   pe,
		registry: reg,
	}
}

// Plan builds a proposed InstallPlan for a package without applying any system changes
func (p *Planner) Plan(ctx context.Context, pkg *domain.PackageInfo, host *domain.HostProfile) (*domain.InstallPlan, error) {
	availableEngines := p.registry.AvailableEngineIDs(ctx, host)
	decision := p.matrix.Evaluate(pkg, host, availableEngines)

	appID := generateAppID(pkg.Name, pkg.SHA256)
	planID := fmt.Sprintf("plan-%s", appID)

	plan := &domain.InstallPlan{
		PlanID:           planID,
		AppID:            appID,
		Package:          *pkg,
		Decision:         *decision,
		Steps:            []domain.PlanStep{},
		HostChanges:      []string{},
		RequiresApproval: true,
	}

	if decision.SupportTier == domain.Tier4Unsupported {
		return plan, domain.NewError(
			domain.ErrCodeExecutionRefused,
			fmt.Sprintf("compatibility policy refused execution for %s (%s)", pkg.Name, pkg.Format),
			strings.Join(decision.RequiredActions, "; "),
			strings.Join(decision.Limitations, "; "),
		)
	}

	eng, ok := p.registry.Get(decision.EngineID)
	if !ok {
		return plan, domain.NewError(
			domain.ErrCodeEngineUnavailable,
			fmt.Sprintf("selected engine '%s' is not registered", decision.EngineID),
			"Check registered engines using 'compat engine list'",
			"Planning aborted",
		)
	}

	if err := eng.Plan(ctx, plan); err != nil {
		return plan, err
	}

	return plan, nil
}

func generateAppID(name, hash string) string {
	cleanName := strings.ToLower(name)
	cleanName = strings.ReplaceAll(cleanName, "_", "-")
	cleanName = strings.ReplaceAll(cleanName, " ", "-")

	shortHash := "local"
	if len(hash) >= 8 {
		shortHash = hash[:8]
	} else if len(hash) > 0 {
		shortHash = hash
	} else {
		h := sha256.Sum256([]byte(name))
		shortHash = hex.EncodeToString(h[:])[:8]
	}

	return fmt.Sprintf("%s-%s", cleanName, shortHash)
}
