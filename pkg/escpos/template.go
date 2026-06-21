/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package escpos

import (
	"regexp"
	"sort"
	"strings"
)

var (
	fieldCommentPattern = regexp.MustCompile(`<!--\s*field:([A-Za-z_][A-Za-z0-9_-]*)\s*([^>]*)-->`)
	placeholderPattern  = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_-]*)\}\}`)
	attributePattern    = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_-]*)="([^"]*)"`)
)

type TemplateField struct {
	Name    string `json:"name"`
	Hint    string `json:"hint,omitempty"`
	Default string `json:"default,omitempty"`
}

func ParseTemplateFields(source string) []TemplateField {
	fieldsByName := map[string]TemplateField{}
	order := []string{}

	add := func(field TemplateField) {
		if field.Name == "" {
			return
		}
		if _, found := fieldsByName[field.Name]; !found {
			order = append(order, field.Name)
		}
		fieldsByName[field.Name] = mergeTemplateField(fieldsByName[field.Name], field)
	}

	for _, match := range fieldCommentPattern.FindAllStringSubmatch(source, -1) {
		attrs := parseTemplateAttributes(match[2])
		add(TemplateField{
			Name:    match[1],
			Hint:    attrs["hint"],
			Default: attrs["default"],
		})
	}

	placeholderNames := map[string]bool{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(source, -1) {
		placeholderNames[match[1]] = true
	}
	extras := []string(nil)
	for name := range placeholderNames {
		if _, found := fieldsByName[name]; !found {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		add(TemplateField{Name: name, Hint: "Value for " + name})
	}

	fields := make([]TemplateField, 0, len(order))
	for _, name := range order {
		fields = append(fields, fieldsByName[name])
	}
	return fields
}

func RenderTemplate(source string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(source, func(token string) string {
		match := placeholderPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		if value, found := values[match[1]]; found {
			return value
		}
		return token
	})
}

func parseTemplateAttributes(source string) map[string]string {
	attrs := map[string]string{}
	for _, match := range attributePattern.FindAllStringSubmatch(source, -1) {
		attrs[strings.ToLower(match[1])] = match[2]
	}
	return attrs
}

func mergeTemplateField(base, next TemplateField) TemplateField {
	if base.Name == "" {
		return next
	}
	if base.Hint == "" {
		base.Hint = next.Hint
	}
	if base.Default == "" {
		base.Default = next.Default
	}
	return base
}
