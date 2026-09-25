package model

import (
	"fmt"
	"path"
	"strings"

	"github.com/maago-web/maago/internal/pipeline"
)

// Files returns the generated files for apps: a models_gen.go for each app
// with fields that are not columns, listing them and why (D-18, D-79). An
// app without such fields gets no file, so gen removes an old one.
func Files(apps []*App) map[string][]byte {
	files := map[string][]byte{}
	for _, app := range apps {
		var lines []string
		for _, m := range app.Models {
			for _, u := range m.Unsupported {
				lines = append(lines, fmt.Sprintf("//   - %s.%s: %s\n", m.Name, u.Name, u.Reason))
			}
		}
		if len(lines) == 0 {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n\npackage %s\n\n// Fields that are not columns (D-18):\n//\n", pipeline.Marker, app.Name)
		for _, l := range lines {
			b.WriteString(l)
		}
		files[path.Join(app.Dir, "models_gen.go")] = []byte(b.String())
	}
	return files
}
