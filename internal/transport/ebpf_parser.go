package transport

import (
	"bytes"
)

type streamBuffer struct {
	buf bytes.Buffer
}




func (s *streamBuffer) extractJSON() []string {
	var messages []string

	for {
		data := s.buf.Bytes()
		if len(data) == 0 {
			break
		}

		
		startIdx := bytes.IndexAny(data, "{[")
		if startIdx == -1 {

			s.buf.Reset()
			break
		}
		if startIdx > 0 {
			s.buf.Next(startIdx)
			data = s.buf.Bytes()
		}

		depth := 0
		inString := false
		escape := false
		found := false

		for i, b := range data {
			if escape {
				escape = false
				continue
			}

			switch b {
			case '\\':
				escape = true
			case '"':
				inString = !inString
			case '{', '[':
				if !inString {
					depth++
				}
			case '}', ']':
				if !inString {
					depth--
					if depth == 0 {
						messages = append(messages, string(data[:i+1]))
						s.buf.Next(i + 1)
						found = true
						goto NextScan
					}
				}
			}
		}

		if !found {

			break
		}
	NextScan:
	}

	return messages
}

func (s *streamBuffer) extractLines() []string {
	var lines []string

	for {
		data := s.buf.Bytes()
		if len(data) == 0 {
			break
		}

		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			break
		}

		line := string(data[:idx])
		s.buf.Next(idx + 1)

		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		lines = append(lines, line)
	}

	return lines
}
