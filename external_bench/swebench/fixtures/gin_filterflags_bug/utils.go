package gin

func filterFlags(content string) string {
	for i, char := range content {
		if char == ';' {
			return content[:i]
		}
	}
	return content
}
