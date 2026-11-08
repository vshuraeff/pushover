package listen

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlComment   = regexp.MustCompile(`<!--[\s\S]*?-->`)
	htmlLineBreak = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	// a tag starts with a letter after "<" or "</", so a literal "<" in text survives,
	// and quoted attribute values may contain ">".
	htmlTag = regexp.MustCompile(`</?[A-Za-z](?:[^>"']|"[^"]*"|'[^']*')*>`)
	// href must follow whitespace, so attributes such as data-href never match.
	htmlLink = regexp.MustCompile(`(?i)<a(?:\s(?:[^>"']|"[^"]*"|'[^']*')*?)?\shref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+))`)
)

// renderHTML renders a pushover html message body as plain text for desktop notifications,
// which cannot display markup: comments and tags are dropped, line breaks become newlines and
// entities are decoded. it also returns the target of the first link, or "" when there is none.
func renderHTML(body string) (text, link string) {
	body = htmlComment.ReplaceAllString(body, "")
	if match := htmlLink.FindStringSubmatch(body); match != nil {
		link = strings.TrimSpace(html.UnescapeString(match[1] + match[2] + match[3]))
	}
	text = htmlLineBreak.ReplaceAllString(body, "\n")
	text = htmlTag.ReplaceAllString(text, "")
	return strings.TrimSpace(html.UnescapeString(text)), link
}
