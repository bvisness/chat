package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/bvisness/chat/utils"
)

// A general error to be used when no results are found. This is the error
// returned by QueryOne, and can generally be used by other database helpers
// that fetch a single result but find nothing.
var NotFound = errors.New("not found")

// Performs a SQL query and returns a slice of all the result rows. The query
// is just plain SQL, but make sure to read the package documentation for
// details. You must explicitly provide the type argument - this is how it
// knows what Go type to map the results to, and it cannot be inferred.
//
// Any SQL query may be performed, including INSERT and UPDATE - as long as it
// returns a result set, you can use this. If the query does not return a
// result set, or you simply do not care about the result set, call Exec
// directly on your pgx connection.
//
// This function always returns pointers to the values. This is convenient for
// structs, but for other types, you may wish to use QueryScalar.
func Query[T any](
	ctx context.Context,
	conn *sql.DB,
	query string,
	args ...any,
) ([]*T, error) {
	it, err := QueryIterator[T](ctx, conn, query, args...)
	if err != nil {
		return nil, err
	} else {
		return it.ToSlice()
	}
}

// Identical to Query, but returns only the first result row. If there are no
// rows in the result set, returns NotFound.
func QueryOne[T any](
	ctx context.Context,
	conn *sql.DB,
	query string,
	args ...any,
) (*T, error) {
	it, err := QueryIterator[T](ctx, conn, query, args...)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	result, hasRow := it.Next()
	if !hasRow {
		if readErr := it.Err(); readErr != nil {
			return nil, readErr
		} else {
			return nil, NotFound
		}
	}

	return result, nil
}

// Identical to Query, but returns concrete values instead of pointers. More
// convenient for primitive types.
func QueryScalar[T any](
	ctx context.Context,
	conn *sql.DB,
	query string,
	args ...any,
) ([]T, error) {
	it, err := QueryIterator[T](ctx, conn, query, args...)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	var result []T
	for {
		val, hasRow := it.Next()
		if !hasRow {
			break
		}
		result = append(result, *val)
	}

	if iterErr := it.Err(); iterErr != nil {
		return nil, iterErr
	}
	return result, nil
}

// Identical to QueryScalar, but returns only the first result value. If there
// are no rows in the result set, returns NotFound.
func QueryOneScalar[T any](
	ctx context.Context,
	conn *sql.DB,
	query string,
	args ...any,
) (T, error) {
	it, err := QueryIterator[T](ctx, conn, query, args...)
	if err != nil {
		var zero T
		return zero, err
	}
	defer it.Close()

	result, hasRow := it.Next()
	if !hasRow {
		var zero T
		if readErr := it.Err(); readErr != nil {
			return zero, readErr
		} else {
			return zero, NotFound
		}
	}

	return *result, nil
}

// Identical to Query, but returns the ResultIterator instead of automatically
// converting the results to a slice. The iterator must be closed after use.
func QueryIterator[T any](
	ctx context.Context,
	conn *sql.DB,
	query string,
	args ...any,
) (*Iterator[T], error) {
	compiled := compileQuery[T](query)
	rows, err := conn.QueryContext(ctx, compiled, args...)
	if err != nil {
		return nil, err
	}

	it := &Iterator[T]{
		rows: rows,

		ctx:    ctx,
		closed: make(chan struct{}, 1),
	}

	return it, nil
}

var reColumnsPlaceholder = regexp.MustCompile(`\$columns({(.*?)})?`)

func compileQuery[T any](query string) string {
	columnsMatch := reColumnsPlaceholder.FindStringSubmatch(query)
	hasColumnsPlaceholder := columnsMatch != nil

	if hasColumnsPlaceholder {
		// The presence of the $columns placeholder means that the destination type
		// must be a struct, and we will plonk that struct's fields into the query.

		tDest := reflect.TypeFor[T]()
		if tDest.Kind() != reflect.Struct {
			panic("$columns can only be used when querying into a struct")
		}

		var prefix []string
		prefixText := columnsMatch[2]
		if prefixText != "" {
			prefix = []string{prefixText}
		}

		columnNames := getColumnNames[T](prefix)

		columns := make([]string, 0, len(columnNames))
		for _, strSlice := range columnNames {
			tableName := strings.Join(strSlice[0:len(strSlice)-1], "_")
			fullName := strSlice[len(strSlice)-1]
			if tableName != "" {
				fullName = tableName + "." + fullName
			}
			columns = append(columns, fullName)
		}

		columnNamesString := strings.Join(columns, ", ")
		query = reColumnsPlaceholder.ReplaceAllString(query, columnNamesString)
	}

	return query
}

// Column names are generated from `db` tags on struct fields. Nested structs
// will cause a given column name to have more than one entry, which will need
// to be splatted down to a single string for the final query.
type columnName []string

func getColumnNames[T any](prefix []string) []columnName {
	var res []columnName

	tStruct := reflect.TypeFor[T]()
	utils.Assert(tStruct.Kind() == reflect.Struct)

	// for f := range dbFields(tStruct) {
	// 	if f.
	// }

	return res
}

type Iterator[T any] struct {
	rows *sql.Rows

	ctx     context.Context
	closed  chan struct{}
	scanErr error
}

func (it *Iterator[T]) Next() (*T, bool) {
	// TODO(ben): What happens if this panics? Does it leak resources? Do we need
	// to put a recover() here and close the rows?

	if it.ctx.Err() != nil {
		it.Close()
		return nil, false
	}

	hasNext := it.rows.Next()
	if !hasNext {
		return nil, false
	}

	result := new(T)
	scanArgs := generateScanArgs(result)
	if err := it.rows.Scan(scanArgs...); err != nil {
		it.scanErr = err
		return nil, false
	}
	AssertAllComplete(scanArgs)

	return result, true
}

func (it *Iterator[T]) Err() error {
	return errors.Join(it.rows.Err(), it.ctx.Err(), it.scanErr)
}

func (it *Iterator[T]) Close() {
	it.rows.Close()
	select {
	case it.closed <- struct{}{}:
	default:
	}
}

// Pulls all the remaining values into a slice, and closes the iterator.
func (it *Iterator[T]) ToSlice() ([]*T, error) {
	defer it.Close()
	var result []*T
	for {
		row, ok := it.Next()
		if !ok {
			break
		}
		result = append(result, row)
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Given a pointer to a struct, generates a slice of pointers to
// Scan-compatible values. Basically this destructures structs according to the
// `db` tags on each field.
//
// The result of this function can be passed to Rows.Scan.
func generateScanArgs(outStruct any) []any {
	var res []any
	appendScanArgs(outStruct, &res)
	return res
}

func appendScanArgs(outStruct any, args *[]any) {
	utils.Assert(reflect.TypeOf(outStruct).Kind() == reflect.Pointer, "generateScanArgs requires a pointer to a struct")
	utils.Assert(reflect.TypeOf(outStruct).Elem().Kind() == reflect.Struct, "generateScanArgs requires a pointer to a struct")

	vStruct := reflect.ValueOf(outStruct).Elem()
	tStruct := vStruct.Type()

	for field := range dbFields(tStruct) {
		vField := vStruct.FieldByIndex(field.Index)
		tField := field.Type

		if tField.Implements(reflect.TypeFor[sql.Scanner]()) {
			// If the type implements sql.Scanner, then we always use the Scanner
			// implementation.
			*args = append(*args, vField.Addr().Interface())
		} else if typeIsScannablePrimitive(tField) {
			// Primitives supported by Rows.Scan are fine as is.
			*args = append(*args, vField.Addr().Interface())
		} else if tField.Kind() == reflect.Pointer && typeIsScannablePrimitive(tField.Elem()) {
			// Pointers to primitives are also ok, but we use our own special
			// wrapper instead of sql.NullString and friends.
			*args = append(*args, &NullPrimitive{vPtrToNullableValue: vField.Addr()})
		} else if tField.Kind() == reflect.Struct {
			// Structs will be recursed into.
			appendScanArgs(vField.Addr().Interface(), args)
		} else if tField.Kind() == reflect.Pointer && tField.Elem().Kind() == reflect.Struct {
			// Pointers to structs will likewise be recursed, but a different kind
			// of wrapper is yet again required.
			wrapper, n := NewNullStruct(vField.Addr().Interface())
			for range n {
				*args = append(*args, wrapper)
			}
		} else {
			panic(fmt.Errorf("type %s is not compatible with sql.Rows.Scan", tField))
		}
	}
}

// Types are taken straight from the documentation for Rows.Scan:
// https://pkg.go.dev/database/sql#Rows.Scan
//
// But also, we add time.Time to this because it's supported by driver.Value
// and special-cased inside Rows.Scan, which is dumb.
//
// interface{} (any) is omitted because I think it's ugly to just return
// driver-specific values. It's not hard to just implement Scanner for anything
// you care about, and to handle driver-specific complexity there.
//
// RawBytes is omitted because we don't need that kind of implementation
// complexity for an API that always expects you to populate a struct field of
// type []byte.
//
// Rows is omitted because it doesn't fit the API design of this package.
func typeIsScannablePrimitive(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Bool,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		// []byte is the only "primitive" slice type
		return t.Elem().Kind() == reflect.Uint8 // type byte = uint8
	default:
		if t == reflect.TypeFor[time.Time]() {
			return true
		}
		return false
	}
}

// A function that replicates the default behavior of [sql.Rows.Scan]; that is,
// it copies a value from the database into the given field as if you had
// simply passed that field to [sql.Rows.Scan]. It supports primitives like
// *string and *int, as well as anything implementing [sql.Scanner].
//
// Some types are omitted because they are a poor fit for this package's API or
// simply unnecessary; see [typeIsScannablePrimitive].
func DefaultScan(src any, out any) error {
	// Basically, this function is trying to mirror the behavior of
	// sql.convertAssign, which is not exported. Rather than copy-paste the
	// logic, we just use [sql.NullString] and friends, but if we ever get a null
	// from the database, we return an error.

	// First get [sql.Scanner] out of the way.
	if asScanner, ok := out.(sql.Scanner); ok {
		return asScanner.Scan(src)
	}

	// Then handle all the primitives we care to handle.
	vResult := reflect.ValueOf(out)
	utils.Assert(vResult.Kind() == reflect.Pointer, "DefaultScan requires a pointer")
	tResult := vResult.Type().Elem()
	if tResult == reflect.TypeFor[time.Time]() {
		var v sql.Null[time.Time]
		if err := v.Scan(src); err != nil {
			return err
		} else if !v.Valid {
			return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
		} else {
			vResult.Elem().Set(reflect.ValueOf(v.V))
		}
	} else {
		switch tResult.Kind() {
		case reflect.String:
			var v sql.NullString
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else {
				vResult.Elem().SetString(v.String)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var v sql.NullInt64
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else if vResult.Elem().OverflowInt(v.Int64) {
				return fmt.Errorf("converting driver.Value type %T (\"%d\") to a %s: value out of range", src, v.Int64, tResult.Kind())
			} else {
				vResult.Elem().SetInt(v.Int64)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			var v sql.NullInt64
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else if v.Int64 < 0 {
				return fmt.Errorf("converting driver.Value type %T (\"%d\") to a %s: invalid syntax", src, v.Int64, tResult.Kind())
			} else if vResult.Elem().OverflowUint(uint64(v.Int64)) {
				return fmt.Errorf("converting driver.Value type %T (\"%d\") to a %s: value out of range", src, v.Int64, tResult.Kind())
			} else {
				vResult.Elem().SetUint(uint64(v.Int64))
			}
		case reflect.Float32, reflect.Float64:
			var v sql.NullFloat64
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else {
				vResult.Elem().SetFloat(v.Float64)
			}
		case reflect.Bool:
			var v sql.NullBool
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else {
				vResult.Elem().SetBool(v.Bool)
			}
		case reflect.Slice: // guaranteed to be []byte
			var v sql.NullString
			if err := v.Scan(src); err != nil {
				return err
			} else if !v.Valid {
				return fmt.Errorf("converting NULL to %s is unsupported", tResult.Kind())
			} else {
				vResult.Elem().Set(reflect.ValueOf([]byte(v.String)))
			}
		default:
			return fmt.Errorf("DefaultScan does not support type %s", tResult)
		}
	}

	return nil
}

// A wrapper around a pointer to a primitive, implementing [sql.Scanner].
// Assigns nil if the db field is nil; otherwise, does exactly what
// [sql.Rows.Scan] would do on a non-nullable version of that field.
type NullPrimitive struct {
	// The address of the nullable (pointer) field, i.e. **value.
	vPtrToNullableValue reflect.Value
}

// Implements [sql.Scanner].
func (p *NullPrimitive) Scan(src any) error {
	// If the incoming value is null, we set the field to nil.
	if src == nil {
		p.vPtrToNullableValue.Elem().SetZero()
		return nil
	}

	// Otherwise we invoke the default behavior of [sql.Rows.Scan].
	tPrimitive := p.vPtrToNullableValue.Type().Elem().Elem()
	vResult := reflect.New(tPrimitive)
	if err := DefaultScan(src, vResult.Interface()); err == nil {
		p.vPtrToNullableValue.Elem().Set(vResult)
		return nil
	} else {
		return err
	}
}

// A wrapper around a pointer to a struct, implementing [sql.Scanner]. It
// should be passed to [sql.Rows.Scan] once for every `db` field in the struct
// type and all its children. For example, suppose you have the following
// types, called like so:
//
//	type Inner2 struct {
//		C int `db:"c"`
//	}
//	type Inner1 struct {
//		B int `db:"b"`
//		Inner2 *Inner2 `db:"inner2"`
//	}
//	type Result struct {
//		A int `db:"a"`
//		Inner1 *Inner1 `db:"inner1"`
//	}
//
// The resulting NullStruct should be passed to [sql.Rows.Scan] three times,
// more or less like so:
//
//	var result *Result
//	s, _ := newNullStruct(&result)
//	rows.Scan(s, s, s)
//
// Each call corresponds to one of the `db` fields, and will allocate
// structs as necessary on the way down to the actual field. In the example
// above, the three fields would correspond to columns `a`, `inner1.b`, and
// `inner1.inner2.c`. (Such deeply-nested structs are probably of limited
// utility in typical flavors of SQL, but it's not really a problem to support
// them here.)
//
// To complete the example: suppose your query returned NULL, 3, NULL. The
// first Scan would do nothing, leaving `result` nil. The second would allocate
// new structs for `result` and `result.Inner1`, finally assigning
// `result.Inner1.B = 3`. The third would again do nothing, leaving
// `result.Inner1.Inner2` nil.
//
// Calling the Scan method too many times will panic. Calling it too few times
// will do nothing, since it doesn't know if you're done scanning yet, but you
// may call [AssertAllComplete] on all your Scan args at the end to assert this
// case yourself.
type NullStruct struct {
	vPtrToNullableStruct reflect.Value
	tStruct              reflect.Type
	hasAllocated         bool

	expectedScanCount int
	scanCount         int
	scanArgs          []any
}

func NewNullStruct(ptrToNullableStruct any) (*NullStruct, int) {
	vPtrToNullableStruct := reflect.ValueOf(ptrToNullableStruct)
	tStruct := vPtrToNullableStruct.Type().Elem().Elem()
	expectedScanCount := countScans(tStruct)
	return &NullStruct{
		vPtrToNullableStruct: vPtrToNullableStruct,
		tStruct:              tStruct,
		expectedScanCount:    expectedScanCount,
	}, expectedScanCount
}

func countScans(tStruct reflect.Type) int {
	var res int
	for field := range dbFields(tStruct) {
		tField := field.Type

		// The logic here mirrors [appendScanArgs] and should be kept in sync.
		if tField.Implements(reflect.TypeFor[sql.Scanner]()) {
			res += 1
		} else if typeIsScannablePrimitive(tField) {
			res += 1
		} else if tField.Kind() == reflect.Pointer && typeIsScannablePrimitive(tField.Elem()) {
			res += 1
		} else if tField.Kind() == reflect.Struct {
			res += countScans(tField)
		} else if tField.Kind() == reflect.Pointer && tField.Elem().Kind() == reflect.Struct {
			res += countScans(tField.Elem())
		} else {
			panic(fmt.Errorf("type %s is not compatible with sql.Rows.Scan", tField))
		}
	}
	return res
}

// Implements [sql.Scanner].
func (s *NullStruct) Scan(src any) error {
	s.scanCount += 1
	if s.scanCount > s.expectedScanCount {
		panic(fmt.Errorf("for type %s: Scan was called too many times (expected %d times)", s.tStruct, s.expectedScanCount))
	}

	// On first scan, explicitly nil out the destination for sanity.
	if s.scanCount == 1 {
		s.vPtrToNullableStruct.Elem().SetZero()
	}

	// If we see a NULL from the db, there is nothing to do. (If we allocated a
	// new struct, all pointer fields will already be nil.)
	if src == nil {
		return nil
	}

	// Otherwise, we need to allocate a new struct if necessary, and then
	// recursively Scan into the appropriate field.
	if !s.hasAllocated {
		s.hasAllocated = true

		vOut := reflect.New(s.tStruct)
		s.vPtrToNullableStruct.Elem().Set(vOut)
		s.scanArgs = generateScanArgs(vOut.Interface())
	}
	if err := DefaultScan(src, s.scanArgs[s.scanCount-1]); err != nil {
		return err
	}

	return nil
}

// Checks if Scan was called the correct number of times. See [NullStruct].
func (s *NullStruct) CheckComplete() error {
	// Sanity check: if we ever allocated, we should have a number of scanArgs
	// equal to our expectedScanCount.
	utils.Assert(!s.hasAllocated || len(s.scanArgs) == s.expectedScanCount)

	if s.scanCount != s.expectedScanCount {
		return fmt.Errorf("for type %s: expected Scan to be called %d times, but was scanned %d times", s.tStruct, s.expectedScanCount, s.scanCount)
	}
	return nil
}

// Checks if all arguments to [sql.Rows.Scan] were called the correct number of
// times. If not, returns an error indicating which args were incorrect.
func CheckAllComplete(scanArgs []any) error {
	var errs []error
	for _, arg := range scanArgs {
		if nullStruct, ok := arg.(*NullStruct); ok {
			if err := nullStruct.CheckComplete(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Same as [CheckAllComplete], but panicky.
func AssertAllComplete(scanArgs []any) {
	if err := CheckAllComplete(scanArgs); err != nil {
		panic(err)
	}
}

// Given a struct type, returns an iterator over all exported struct fields
// with a `db` tag.
func dbFields(tStruct reflect.Type) iter.Seq[reflect.StructField] {
	utils.Assert(tStruct.Kind() == reflect.Struct, "DBFields requires a struct type")
	return func(yield func(reflect.StructField) bool) {
		for _, field := range reflect.VisibleFields(tStruct) {
			if !field.IsExported() || field.Anonymous {
				continue
			}

			if columnName := field.Tag.Get("db"); columnName != "" {
				// Validate that anything with a tag has parent fields with a tag
				if len(field.Index) > 1 {
					for i := 0; i < len(field.Index)-1; i++ {
						parentField := tStruct.FieldByIndex(field.Index[:i])
						if parentField.Tag.Get("db") == "" {
							panic(fmt.Errorf("for field %s of type %s: parent embedding type %s has no db tag", field.Name, tStruct, parentField.Type))
						}
					}
				}

				if !yield(field) {
					return
				}
			}
		}
	}
}
