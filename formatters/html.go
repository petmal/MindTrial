// Copyright (C) 2025 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package formatters

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"path/filepath"

	"github.com/petmal/mindtrial/runners"
	"github.com/petmal/mindtrial/version"
)

const templateFile = "templates/html.tmpl"

//go:embed templates/*.tmpl
var templatesFS embed.FS

var currentVersionData = VersionData{
	Name:    version.Name,
	Version: version.GetVersion(),
	Source:  version.GetSource(),
}

// VersionData contains version information included in formatted output.
type VersionData struct {
	// Name is the application name.
	Name string
	// Version is the application version string.
	Version string
	// Source is the application source code URL.
	Source string
}

// NewHTMLFormatter creates a new formatter that outputs results as an HTML document.
func NewHTMLFormatter() Formatter {
	templ := template.Must(template.New(filepath.Base(templateFile)).Funcs(template.FuncMap{
		"ToStatus":                ToStatus,
		"FormatAnswer":            FormatAnswer,
		"SortResultsByProvider":   SortedKeys[string, []runners.RunResult],
		"SortResultsByRunAndKind": SortedKeys[string, map[runners.ResultKind][]runners.RunResult],
		"CountByKind":             CountByKind,
		"TotalDuration":           TotalDuration,
		"RoundToMS":               RoundToMS,
		"Timestamp":               Timestamp,
		"TextToHTML":              TextToHTML,
		"SafeHTML": func(s string) template.HTML {
			return template.HTML(s) //nolint:gosec
		},
	}).ParseFS(templatesFS, templateFile))
	return &htmlFormatter{
		templ: templ,
	}
}

type htmlFormatter struct {
	templ *template.Template
}

func (f htmlFormatter) FileExt() string {
	return "html"
}

func (f htmlFormatter) Write(results runners.Results, out io.Writer) error {
	if err := f.templ.Execute(out, struct {
		ResultsData runners.Results
		VersionData VersionData
	}{
		ResultsData: results,
		VersionData: currentVersionData,
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrPrintResults, err)
	}
	return nil
}
