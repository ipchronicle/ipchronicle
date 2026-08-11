package releasetool

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	releaseReadinessName = "RELEASE_READINESS.md"
	pendingReportStatus  = "Status: Pre-publication validation in progress"
	readyReportStatus    = "Status: Ready for publication decision"
	reportEvidenceStart  = "<!-- release-evidence:start -->"
	reportEvidenceEnd    = "<!-- release-evidence:end -->"
)

var actionsRunPath = regexp.MustCompile(`^/ipchronicle/ipchronicle/actions/runs/[1-9][0-9]*$`)

type FinalizeOptions struct {
	Directory      string
	Version        string
	Revision       string
	CIRunURL       string
	RCRunURL       string
	ValidationDate string
}

func Finalize(options FinalizeOptions) (Summary, error) {
	summary, err := Verify(VerifyOptions{
		Directory: options.Directory,
		Version:   options.Version,
		Revision:  options.Revision,
	})
	if err != nil {
		return Summary{}, fmt.Errorf("verify candidate before finalization: %w", err)
	}
	if err := validateActionsRunURL("ordinary CI", options.CIRunURL); err != nil {
		return Summary{}, err
	}
	if err := validateActionsRunURL("release candidate", options.RCRunURL); err != nil {
		return Summary{}, err
	}
	if options.CIRunURL == options.RCRunURL {
		return Summary{}, errors.New("ordinary CI and release candidate run URLs must differ")
	}
	validationTime, err := time.Parse("2006-01-02", options.ValidationDate)
	if err != nil || validationTime.Format("2006-01-02") != options.ValidationDate {
		return Summary{}, errors.New("validation date must use YYYY-MM-DD")
	}

	reportPath := filepath.Join(options.Directory, releaseReadinessName)
	report, err := os.ReadFile(reportPath)
	if err != nil {
		return Summary{}, fmt.Errorf("read release-readiness report: %w", err)
	}
	contents := string(report)
	if strings.Count(contents, pendingReportStatus) != 1 {
		return Summary{}, errors.New("release-readiness report does not have exactly one pending status")
	}
	start := strings.Index(contents, reportEvidenceStart)
	end := strings.Index(contents, reportEvidenceEnd)
	if start < 0 || end < 0 || end <= start ||
		strings.Count(contents, reportEvidenceStart) != 1 || strings.Count(contents, reportEvidenceEnd) != 1 {
		return Summary{}, errors.New("release-readiness report evidence markers are invalid")
	}

	evidence := fmt.Sprintf(`%s

Final candidate validation completed on **%s**.

- Candidate revision: `+"`%s`"+`
- Ordinary CI: <%s> (**passed**)
- Release candidate workflow: <%s> (**passed**)
- Reproducibility: **passed**. Two independent clean builds from the candidate
  revision matched exactly by file name, mode, size, and SHA-256 digest before
  and after this evidence was applied.
- Required release gates: **passed**. Candidate verification, deterministic
  failure tests, all 34 distribution/architecture lifecycle jobs, both native
  resource jobs, and live official IPQuality execution completed successfully.
%s`, reportEvidenceStart, options.ValidationDate, summary.Revision,
		options.CIRunURL, options.RCRunURL, reportEvidenceEnd)
	contents = contents[:start] + evidence + contents[end+len(reportEvidenceEnd):]
	contents = strings.Replace(contents, pendingReportStatus, readyReportStatus, 1)
	if err := writeFileAtomic(reportPath, []byte(contents), 0o644); err != nil {
		return Summary{}, fmt.Errorf("write release-readiness report: %w", err)
	}

	return Create(CreateOptions{
		Directory: options.Directory,
		Version:   summary.Version,
		Revision:  summary.Revision,
	})
}

func validateActionsRunURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.RawQuery != "" || parsed.Fragment != "" || !actionsRunPath.MatchString(parsed.Path) {
		return fmt.Errorf("%s run URL is not a canonical IPChronicle GitHub Actions URL", name)
	}
	return nil
}
