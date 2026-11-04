package config

import (
	"errors"
	"path/filepath"
	"strings"
)

// AnalysisConfig configures the optional LLM summarization of SQL hotspots. The
// analyzer is an external read-only command (typically qoder-mcp/analyze.py)
// that receives a natural-language prompt on stdin and prints its findings to
// stdout; sql-monitor forwards those findings to Feishu or the web UI.
type AnalysisConfig struct {
	Enabled        bool     `json:"enabled"`
	Command        []string `json:"command"`
	CWD            string   `json:"cwd,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	MaxHotspots    int      `json:"max_hotspots,omitempty"`
}

// Validate fills defaults and rejects configuration that would make the
// analyzer unsafe or useless. Command[0] may be a PATH executable (e.g.
// "python3"); remaining arguments should be absolute paths so the analyzer
// runs independently of the monitor's working directory.
func (c *AnalysisConfig) Validate() error {
	if c == nil || !c.Enabled {
		return nil
	}
	if len(c.Command) == 0 {
		return errors.New("analysis requires command, e.g. [\"python3\", \"/absolute/path/to/qoder-mcp/analyze.py\"]")
	}
	for i := range c.Command {
		c.Command[i] = strings.TrimSpace(c.Command[i])
		if c.Command[i] == "" {
			return errors.New("analysis command must not contain empty arguments")
		}
	}
	if c.CWD != "" {
		c.CWD = strings.TrimSpace(c.CWD)
		if !filepath.IsAbs(c.CWD) {
			return errors.New("analysis cwd must be an absolute path")
		}
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 600
	}
	if c.TimeoutSeconds < 10 || c.TimeoutSeconds > 3600 {
		return errors.New("analysis timeout_seconds must be 10..3600")
	}
	if c.MaxHotspots == 0 {
		c.MaxHotspots = 5
	}
	if c.MaxHotspots < 1 || c.MaxHotspots > 20 {
		return errors.New("analysis max_hotspots must be 1..20")
	}
	return nil
}
