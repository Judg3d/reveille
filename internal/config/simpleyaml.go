package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type YAMLValue struct {
	scalar string
	list   []string
}

func parseYAMLFile(path string) (map[string]YAMLValue, error) {
	return ParseYAMLFile(path)
}

func ParseYAMLFile(path string) (map[string]YAMLValue, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]YAMLValue{}
	var stack []string
	var currentList string
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := stripComment(scanner.Text())
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := countIndent(raw)
		if indent%2 != 0 {
			return nil, fmt.Errorf("%s:%d: indentation must use multiples of two spaces", path, lineNo)
		}
		level := indent / 2
		text := strings.TrimSpace(raw)
		if strings.HasPrefix(text, "- ") {
			if currentList == "" {
				return nil, fmt.Errorf("%s:%d: list item without list key", path, lineNo)
			}
			values[currentList] = YAMLValue{list: append(values[currentList].list, cleanScalar(strings.TrimPrefix(text, "- ")))}
			continue
		}
		parts := strings.SplitN(text, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: expected key: value", path, lineNo)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		if level < len(stack) {
			stack = stack[:level]
		}
		if level > len(stack) {
			return nil, fmt.Errorf("%s:%d: skipped indentation level", path, lineNo)
		}
		stack = append(stack, key)
		full := strings.Join(stack, ".")
		value := strings.TrimSpace(parts[1])
		currentList = ""
		if value == "" {
			currentList = full
			continue
		}
		values[full] = YAMLValue{scalar: cleanScalar(value)}
		stack = stack[:level]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func str(values map[string]YAMLValue, key string) string {
	return values[key].scalar
}

func list(values map[string]YAMLValue, key string) []string {
	return values[key].list
}

func String(values map[string]YAMLValue, key string) string { return str(values, key) }

func List(values map[string]YAMLValue, key string) []string { return list(values, key) }

func stripComment(line string) string {
	inSingle, inDouble := false, false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func countIndent(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' {
			return n
		}
		n++
	}
	return n
}

func cleanScalar(value string) string {
	return strings.TrimSpace(strings.Trim(value, `"'`))
}
