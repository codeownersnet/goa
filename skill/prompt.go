package skill

import (
	"fmt"
	"strings"
)

func ToPromptXML(skills []*Skill) string {
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		b.WriteString("\t<skill>\n")
		b.WriteString(fmt.Sprintf("\t\t<name>%s</name>\n", xmlEscape(s.Name)))
		b.WriteString(fmt.Sprintf("\t\t<description>%s</description>\n", xmlEscape(s.Description)))
		b.WriteString(fmt.Sprintf("\t\t<location>%s</location>\n", xmlEscape(s.Location)))
		b.WriteString("\t</skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func ToPromptText(skills []*Skill) string {
	var b strings.Builder
	for _, s := range skills {
		b.WriteString(fmt.Sprintf("- %s: %s [%s]\n", s.Name, s.Description, s.Location))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
