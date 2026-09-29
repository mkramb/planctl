// Package plan manages plan identity and the local plan lifecycle.
package plan

import (
	"fmt"
	"strings"
	"unicode"
)

type Plan struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	Base          string `json:"base"`
	WorkspacePath string `json:"workspace_path"`
	AbsolutePath  string `json:"absolute_path"`
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func Slug(title string) (string, error) {
	var result strings.Builder
	separator := false
	for _, char := range title {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			if separator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(unicode.ToLower(char))
			separator = false
		} else {
			separator = true
		}
	}
	slug := result.String()
	if slug == "" || len(slug) > 100 {
		return "", &Error{Code: "invalid_title", Message: "title must produce a slug of 1 to 100 bytes containing letters or numbers"}
	}
	// These filenames cannot be used on Windows, even with a .md extension.
	reserved := slug == "con" || slug == "prn" || slug == "aux" || slug == "nul"
	if len(slug) == 4 && (strings.HasPrefix(slug, "com") || strings.HasPrefix(slug, "lpt")) && slug[3] >= '1' && slug[3] <= '9' {
		reserved = true
	}
	if reserved {
		return "", &Error{Code: "invalid_title", Message: fmt.Sprintf("title produces the reserved filename %q; choose a more descriptive title", slug)}
	}
	return slug, nil
}

func template(title string) string {
	return "# " + title + "\n\n## Context\n\n## Proposed Approach\n\n## Implementation\n\n## Testing\n\n## Open Questions\n"
}
