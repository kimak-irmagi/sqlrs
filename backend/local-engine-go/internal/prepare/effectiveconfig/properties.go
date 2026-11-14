package effectiveconfig

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ParseProperties reads the supported Java properties grammar used by
// Liquibase defaults files. Unknown escapes and malformed Unicode fail
// closed, so an admitted value cannot be interpreted differently at launch.
func ParseProperties(reader io.Reader) (map[string]string, error) {
	return parseProperties(reader, false)
}

// parseProperties shares admission grammar and original-byte line limits;
// the opt-in file adapter changes only the named file-bearing values.
func parseProperties(reader io.Reader, fileBearing bool) (map[string]string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	properties := make(map[string]string)
	var logical string
	continued := false
	for scanner.Scan() {
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return nil, fmt.Errorf("Liquibase properties are not UTF-8")
		}
		if continued {
			line = strings.TrimLeft(line, " \t\f")
		}
		logical += line
		if trailingBackslashes(logical)%2 == 1 {
			logical = logical[:len(logical)-1]
			continued = true
			continue
		}
		continued = false
		line = strings.TrimLeft(logical, " \t\f")
		logical = ""
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		separator := len(line)
		escaped := false
		for i := 0; i < len(line); i++ {
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			if line[i] == '=' || line[i] == ':' || line[i] == ' ' || line[i] == '\t' || line[i] == '\f' {
				separator = i
				break
			}
		}
		key, err := unescapeProperty(line[:separator])
		if err != nil || invalidPropertiesKey(key) || protectedProperty(key) {
			return nil, fmt.Errorf("unsupported or protected Liquibase property key")
		}
		value := ""
		if separator < len(line) {
			value = strings.TrimLeft(line[separator:], " \t\f")
			value = strings.TrimLeft(value, "=:")
			value = strings.TrimLeft(value, " \t\f")
		}
		adaptFile := fileBearing && fileBearingProperty(key)
		if adaptFile {
			var mapped strings.Builder
			for _, b := range []byte(value) {
				mapped.WriteRune(rune(b))
			}
			value = mapped.String()
		}
		value, err = unescapeProperty(value)
		if err != nil {
			return nil, fmt.Errorf("invalid Liquibase property value: %w", err)
		}
		if adaptFile {
			value = strings.TrimFunc(value, func(r rune) bool { return r <= 0x20 })
		}
		properties[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Liquibase properties: %w", err)
	}
	if continued {
		return nil, fmt.Errorf("unfinished Liquibase property continuation")
	}
	return properties, nil
}

func trailingBackslashes(value string) int {
	count := 0
	for i := len(value) - 1; i >= 0 && value[i] == '\\'; i-- {
		count++
	}
	return count
}

func unescapeProperty(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		i++
		if i == len(value) {
			return "", fmt.Errorf("trailing escape")
		}
		switch value[i] {
		case '\\', '=', ':', ' ', '#', '!':
			out.WriteByte(value[i])
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'n':
			out.WriteByte('\n')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(value) {
				return "", fmt.Errorf("short Unicode escape")
			}
			code, err := strconv.ParseUint(value[i+1:i+5], 16, 16)
			if err != nil || code >= 0xD800 && code <= 0xDFFF {
				return "", fmt.Errorf("invalid Unicode escape")
			}
			out.WriteRune(rune(code))
			i += 4
		default:
			return "", fmt.Errorf("unsupported escape")
		}
	}
	return out.String(), nil
}
