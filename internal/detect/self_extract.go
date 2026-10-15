package detect

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

func (d *Detector) inspectSelfExtract(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatSelfExtract
	base := filepath.Base(info.Path)
	info.Name = strings.TrimSuffix(strings.TrimSuffix(base, ".run"), ".sh")

	// Read script header (first 50 lines) safely
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return info, nil
	}

	scanner := bufio.NewScanner(io.LimitReader(f, 64*1024))
	var headerLines []string
	linesRead := 0

	for scanner.Scan() && linesRead < 50 {
		line := scanner.Text()
		linesRead++
		// Skip binary non-ascii lines if script transitions to binary payload
		if isBinaryLine(line) {
			headerLines = append(headerLines, fmt.Sprintf("[... binary archive payload attached at line %d ...]", linesRead))
			break
		}
		headerLines = append(headerLines, line)
	}

	headerText := strings.Join(headerLines, "\n")
	info.SelfExtractWarning = fmt.Sprintf(
		"Self-extracting script installer detected (%s).\n"+
			"CAUTION: Self-extracting scripts run arbitrary unconfined commands on the host.\n"+
			"Tier 3 experimental rating. Script header preview:\n\n%s",
		base, headerText,
	)

	info.Metadata["script_header"] = headerText
	info.PackageType = domain.PackageTypeCLI

	return info, nil
}

func isSelfExtractingScript(name string, magic []byte) bool {
	lower := strings.ToLower(name)
	// Check shebang magic '#!'
	if len(magic) >= 2 && magic[0] == '#' && magic[1] == '!' {
		if strings.HasSuffix(lower, ".run") || strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bin") {
			return true
		}
	}
	if strings.HasSuffix(lower, ".run") {
		return true
	}
	return false
}

func isBinaryLine(line string) bool {
	nonPrintable := 0
	for i := 0; i < len(line); i++ {
		b := line[i]
		if b < 0x20 && b != '\t' && b != '\r' && b != '\n' {
			nonPrintable++
		}
	}
	return nonPrintable > 3
}
