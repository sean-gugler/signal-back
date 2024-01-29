package cmd

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

var snakeCase *strings.Replacer

func makeReplacer() *strings.Replacer {
	r := make([]string, 0, 26*2)
	for ch := 'a'; ch <= 'z'; ch++ {
		CH := ch - 'a' + 'A'
		r = append(r, string(CH))
		r = append(r, "_" + string(ch))
	}
	return strings.NewReplacer(r...)
}

// Convert names of struct members into snake_case
func names(fields []reflect.StructField) []string {
	if snakeCase == nil {
		snakeCase = makeReplacer()
	}
	s := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Name == "ID" {
			// special case, exported struct members cannot begin with _
			s = append(s, "_id")
		} else {
			s = append(s, snakeCase.Replace(f.Name)[1:])
		}
	}
	return s
}

var sqlColumns = make(map[reflect.Type]string)

func cachedFieldNames(typ reflect.Type) string {
	fields, ok := sqlColumns[typ]
	if !ok {
		// Construct and cache query string
		vf := reflect.VisibleFields(typ)
		fields = strings.Join(names(vf), ", ")
		sqlColumns[typ] = fields
	}
	return fields
}

//TODO: upgrade project to support generics [T any]

// Read all rows from table, but only columns that are named as struct members.
// WordCase members are automatically matched with snake_case columns of the same name.
func SelectStructFromTable(db *sql.DB, record interface{}, table string) ([]interface{}, error) {
	var result []interface{}

	typ := reflect.TypeOf(record)
	n := typ.NumField()

	// Perform SELECT query
	q := fmt.Sprintf("SELECT %s FROM %s", cachedFieldNames(typ), table)

	rows, err := db.Query(q)
	if err != nil {
		return nil, errors.Wrap(err, q)
	}
	defer rows.Close()

	// Scan rows into new array of same type as 'record'
	for rows.Next() {
		data := reflect.New(typ)
		val := data.Elem()

		I := make([]interface{}, n)
		for i := 0; i < n; i++ {
			I[i] = val.Field(i).Addr().Interface()
		}

		if err = rows.Scan(I...); err != nil {
			return nil, errors.Wrap(err, "scan")
		}

		result = append(result, data.Interface())
	}
	return result, nil
}

func SelectEntireTable(db *sql.DB, table string) (columnNames []string, records [][]interface{}, result error) {
	q := fmt.Sprintf("SELECT * FROM %s", table)

	rows, err := db.Query(q)
	if err != nil {
		return nil, nil, errors.Wrap(err, q)
	}
	defer rows.Close()

	// For convenience, return column names
	// (useful for CSV header row and JSON field names)
	columnNames, err = rows.Columns()
	if err != nil {
		return nil, nil, errors.Wrap(err, q)
	}

	// Scan rows into generic arrays of type interface{}
	for rows.Next() {
		columns, err := rows.ColumnTypes()
		if err != nil {
			return nil, nil, errors.Wrap(err, q)
		}

		cols := make([]interface{}, 0, len(columns))
		for _, col := range columns {
			cols = append(cols, ScanType(col))
		}

		if err = rows.Scan(cols...); err != nil {
			return nil, nil, errors.Wrap(err, "scan")
		}

		// Fixup null values back to nil
		for i, col := range cols {
			if _, ok := col.(*sql.NullBool); ok {
				cols[i] = nil
			}
		}

		records = append(records, cols)
	}

	return columnNames, records, nil
}

// Accommodate deficiencies in the driver's ScanType()
func ScanType(col *sql.ColumnType) interface{} {
	typ := col.ScanType()
	if typ == nil {
		// Driver always chooses a pointer to a /primitive/ data type
		// or 'nil' when the VALUE is null. This is unfortunate because
		// nil will cause Scan to panic.
		//
		// It would be nice if col.ScanType returned an
		// appropriate sql.Null* type but it never does.
		// We must substitute nil with one of those Null types (it doesn't
		// matter which one, since we know type.Valid will always be false).
		return &sql.NullBool{}

	} else if col.DatabaseTypeName() == "BLOB" {
		// Driver chooses inappropriate type *[][]byte
		// Replace with *[]byte
		return &[]byte{}

	} else {
		return reflect.New(typ).Interface()
	}
}

// Convert results from SelectEntireTable into strings
func StringifyRows(vrows [][]interface{}) [][]string {
	srows := [][]string{}
	for _, vrow := range vrows {
		ss := make([]string, 0, len(vrow))
		for _, v := range vrow {
			s := ""
			if v != nil {
				if vb, ok := v.(*[]byte); ok {
					s = base64.StdEncoding.EncodeToString(*vb)
				} else {
					ptr := reflect.ValueOf(v)
					s = fmt.Sprintf("%v", ptr.Elem())
				}
			}
			ss = append(ss, s)
		}

		srows = append(srows, ss)
	}
	return srows
}
