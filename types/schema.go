package types

import (
	"strings"

	"github.com/xeals/signal-back/signal"
)

type ColumnType int

const (
	CT_None ColumnType = iota
	CT_Text
	CT_Integer
	CT_Real
	CT_Blob
)

func columnTypeFromString(s string) ColumnType {
	switch s {
	case "TEXT":    return CT_Text
	case "INTEGER": return CT_Integer
	case "REAL":    return CT_Real
	case "BLOB":    return CT_Blob
	default:        return CT_None
	}
}

type Schema struct {
	Index map[string]int
	Type  []ColumnType
}

func NewSchema(statement_params string) *Schema {
	// remove parentheses, then split by commas
	cols := strings.Split(Unwrap(statement_params, "()"), ",")

	s := Schema{
		Index: make(map[string]int),
		Type:  make([]ColumnType, len(cols)),
	}

	// Directives like "UNIQUE(field, field)" get split by commas, too.
	// Handle this by skipping opening through closing parentheses.
	inParen := false
	j := 0

	// convert each text description into Schema entries
	for i, desc := range cols {
		trimmed := strings.TrimSpace(desc)
		parts := strings.SplitN(trimmed, " ", 3)
		// ignore parts[3:], optional tags like "DEFAULT" or "PRIMARY"

		name := parts[0]
		if strings.Index(name, "(") != -1 {
			inParen = true
		}
		if inParen {
			if strings.Index(name, ")") != -1 {
				inParen = false
			} else {
				j++
			}
			continue
		}

		// Map column names to their index number
		s.Index[name] = i - j

		if len(parts) > 1 {
			s.Type[i] = columnTypeFromString(parts[1])
		}
	}
	return &s
}

func (s *Schema) Field(row []*signal.SqlStatement_SqlParameter, column string) interface{} {
	i, ok := s.Index[column]
	t     := s.Type[i]
	if !ok {
		panic("Field not found: " + column)
	}
	return ParameterValue(row[i], t)
}

func (s *Schema) RowValues(row []*signal.SqlStatement_SqlParameter) []interface{} {
	pv := make([]interface{}, len(row))
	for i, v := range row {
		pv[i] = ParameterValue(v, s.Type[i])
	}
	return pv
}

// Backups.proto declares IntegerParameter as uint64
//   but there are values of -1 in the database.
// We must treat all IntegerParameter as int64 to avoid this error during db.Exec
//   "sql: converting argument $32 type: uint64 values with high bit set are not supported"

func signed(u *uint64) *int64 {
	if u == nil {
		return nil
	}
	s := int64(*u)
	return &s
}

func ParameterValue(p *signal.SqlStatement_SqlParameter, typ ColumnType) interface{} {
	// https://www.sqlite.org/datatype3.html#type_affinity
	//     "The type affinity of a column is the recommended type for data stored
	//     in that column. The important idea here is that the type is recommended,
	//     not required. Any column can still store any type of data."

	if         p.StringParameter != nil {
		return p.StringParameter
	} else if  p.IntegerParameter != nil {
		return signed(p.IntegerParameter)
	} else if  p.DoubleParameter != nil {
		return p.DoubleParameter
	} else if  p.BlobParameter != nil {
		return p.BlobParameter
	}

	// return nil value of specific type if possible
	switch typ {
	case CT_Text:       return p.StringParameter
	case CT_Integer:    return signed(p.IntegerParameter)
	case CT_Real:       return p.DoubleParameter
	case CT_Blob:       return p.BlobParameter
	}

	return nil
}
