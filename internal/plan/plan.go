// Package plan manages the file-publish review lifecycle: selecting files,
// staging them into an isolated worktree, and driving the GitHub review.
package plan

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Plan identifies one review: a set of files published to a branch and PR.
type Plan struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Branch        string   `json:"branch"`
	Base          string   `json:"base"`
	Files         []string `json:"files"`
	WorkspacePath string   `json:"workspace_path"`
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

// Slug derives a branch/identity slug from a file name. The extension is kept
// so "a.md" and "a.txt" do not collide.
func Slug(name string) (string, error) {
	var result strings.Builder
	separator := false
	for _, char := range name {
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
		return "", &Error{Code: "invalid_title", Message: "file name must produce a slug of 1 to 100 bytes containing letters or numbers"}
	}
	// These filenames cannot be used on Windows.
	reserved := slug == "con" || slug == "prn" || slug == "aux" || slug == "nul"
	if len(slug) == 4 && (strings.HasPrefix(slug, "com") || strings.HasPrefix(slug, "lpt")) && slug[3] >= '1' && slug[3] <= '9' {
		reserved = true
	}
	if reserved {
		return "", &Error{Code: "invalid_title", Message: fmt.Sprintf("file name produces the reserved slug %q; rename the file or pass --title", slug)}
	}
	return slug, nil
}

// slugFor returns the review slug for the first selected file, falling back to
// a stable placeholder when the base name cannot be slugified.
func slugFor(file string) string {
	name := filepath.Base(file)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	if slug, err := Slug(name); err == nil {
		return slug
	}
	return "review"
}
