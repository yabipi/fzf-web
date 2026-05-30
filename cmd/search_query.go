package main

import "strings"

// splitSearchTerms 按空白切分查询词，过滤空项。
func splitSearchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	return strings.Fields(query)
}

// buildFzfFilterQuery 将用户输入转为 fzf extended 过滤表达式。
// 多个词时用 '词' 做子串精确匹配并以空格 AND；单词仍走 fzf 默认模糊匹配。
func buildFzfFilterQuery(query string) string {
	terms := splitSearchTerms(query)
	if len(terms) == 0 {
		return ""
	}
	if len(terms) == 1 {
		return terms[0]
	}
	parts := make([]string, len(terms))
	for i, term := range terms {
		term = strings.ReplaceAll(term, "'", "")
		parts[i] = "'" + term + "'"
	}
	return strings.Join(parts, " ")
}

// matchesAllTerms 判断 text 是否包含 terms 中的每一个词（大小写不敏感）。
func matchesAllTerms(text string, terms []string) bool {
	if len(terms) == 0 {
		return false
	}
	lower := strings.ToLower(text)
	for _, term := range terms {
		if !strings.Contains(lower, strings.ToLower(term)) {
			return false
		}
	}
	return true
}
