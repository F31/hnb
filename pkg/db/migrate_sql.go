package db

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func validateMigrationFiles(files []MigrationFile) error {
	if len(files) == 0 {
		return fmt.Errorf("migration set must not be empty")
	}
	var previous int64
	for _, f := range files {
		prefix, _, ok := strings.Cut(f.Name, "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		sum := sha256.Sum256([]byte(f.SQL))
		if !ok || err != nil || version != f.Version || f.Version <= previous || filepath.Base(f.Name) != f.Name || !strings.HasSuffix(f.Name, ".sql") || strings.HasSuffix(f.Name, ".rollback.sql") || strings.TrimSpace(f.SQL) == "" || f.Checksum != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("migration set must be ordered, unique and match the actual SQL bytes")
		}
		previous = f.Version
	}
	return nil
}

// Split only outside PostgreSQL strings, dollar-quoted function bodies and
// comments. This is not a SQL interpreter: PostgreSQL validates the resulting
// DDL. It prevents script-level COMMIT from escaping the ledger transaction.
func sqlStatements(body string) ([]string, error) {
	var statements []string
	start, block := 0, 0
	var quote byte
	dollar := ""
	line, escape := false, false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if line {
			if c == '\n' {
				line = false
			}
			continue
		}
		if block > 0 {
			if strings.HasPrefix(body[i:], "/*") {
				block++
				i++
			} else if strings.HasPrefix(body[i:], "*/") {
				block--
				i++
			}
			continue
		}
		if dollar != "" {
			if strings.HasPrefix(body[i:], dollar) {
				i += len(dollar) - 1
				dollar = ""
			}
			continue
		}
		if quote != 0 {
			if escape && c == '\\' {
				i++
				continue
			}
			if c == quote {
				if i+1 < len(body) && body[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		if strings.HasPrefix(body[i:], "--") {
			line = true
			i++
			continue
		}
		if strings.HasPrefix(body[i:], "/*") {
			block = 1
			i++
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			escape = c == '\'' && i > 0 && (body[i-1] == 'e' || body[i-1] == 'E')
			continue
		}
		if c == '$' {
			j := i + 1
			for j < len(body) && ((body[j] >= 'a' && body[j] <= 'z') || (body[j] >= 'A' && body[j] <= 'Z') || body[j] == '_' || (j > i+1 && body[j] >= '0' && body[j] <= '9')) {
				j++
			}
			if j < len(body) && body[j] == '$' {
				dollar = body[i : j+1]
				i = j
			}
			continue
		}
		if c == ';' {
			part := body[start:i]
			if leadingSQL(part) != "" {
				statements = append(statements, part)
			}
			start = i + 1
		}
	}
	if quote != 0 || block != 0 || dollar != "" {
		return nil, fmt.Errorf("unterminated SQL literal or comment")
	}
	if part := body[start:]; leadingSQL(part) != "" {
		statements = append(statements, part)
	}
	return statements, nil
}

func leadingSQL(part string) string {
	for {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "--") {
			if end := strings.IndexByte(part, '\n'); end >= 0 {
				part = part[end+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(part, "/*") {
			depth, i := 1, 2
			for i < len(part) && depth > 0 {
				if strings.HasPrefix(part[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(part[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth > 0 {
				return ""
			}
			part = part[i:]
			continue
		}
		return strings.ToUpper(part)
	}
}

func transactionalBody(body string) (string, error) {
	statements, err := sqlStatements(body)
	if err != nil || len(statements) == 0 {
		return "", fmt.Errorf("invalid migration SQL")
	}
	first, last := leadingSQL(statements[0]), leadingSQL(statements[len(statements)-1])
	if first == "BEGIN" || first == "BEGIN TRANSACTION" || first == "BEGIN WORK" {
		if len(statements) < 3 || (last != "COMMIT" && last != "COMMIT WORK" && last != "COMMIT TRANSACTION") {
			return "", fmt.Errorf("incomplete outer transaction")
		}
		statements = statements[1 : len(statements)-1]
	}
	for _, statement := range statements {
		fields := strings.Fields(leadingSQL(statement))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT", "SAVEPOINT", "RELEASE", "START", "PREPARE":
			return "", fmt.Errorf("embedded transaction control is unsupported")
		}
	}
	return strings.Join(statements, ";\n") + ";", nil
}
