package core

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const fmSeparator = "---"

// ParseFrontmatter splits a YAML frontmatter block from the start of body.
// It returns the parsed frontmatter map, the body with the frontmatter removed,
// and any parse error. If there is no frontmatter, fm is nil and cleanBody == body.
func ParseFrontmatter(body string) (fm map[string]any, cleanBody string, err error) {
	// Frontmatter must start at the very beginning of the file.
	if !strings.HasPrefix(body, fmSeparator+"\n") && body != fmSeparator {
		return nil, body, nil
	}

	// Find the closing "---" delimiter.
	rest := body[len(fmSeparator)+1:] // skip opening "---\n"
	end := strings.Index(rest, "\n"+fmSeparator)
	if end < 0 {
		// Unclosed frontmatter — treat entire body as content, no frontmatter.
		return nil, body, nil
	}

	yamlBlock := rest[:end]
	after := rest[end+1+len(fmSeparator):]
	// Trim a single leading newline from the body that follows the closing delimiter.
	if strings.HasPrefix(after, "\n") {
		after = after[1:]
	}

	fm = make(map[string]any)
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return nil, body, err
	}
	return fm, after, nil
}

// SerializeFrontmatter prepends a YAML frontmatter block to body.
// If fm is nil or empty, body is returned unchanged.
func SerializeFrontmatter(fm map[string]any, body string) (string, error) {
	if len(fm) == 0 {
		return body, nil
	}
	out, err := yaml.Marshal(fm)
	if err != nil {
		return "", err
	}
	return fmSeparator + "\n" + string(out) + fmSeparator + "\n" + body, nil
}

// tagsFromFrontmatter extracts the "tags" field from parsed frontmatter.
func tagsFromFrontmatter(fm map[string]any) []string {
	if fm == nil {
		return nil
	}
	raw, ok := fm["tags"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		tags := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				tags = append(tags, s)
			}
		}
		return tags
	case []string:
		return v
	case string:
		return []string{v}
	}
	return nil
}
