// Copyright (c) 2024 Jonathan Amsterdam
// Use of this source code is governed by the license in the LICENSE file.

// TODO: unexported values; https://stackoverflow.com/questions/42664837/how-to-access-unexported-struct-fields/43918797#43918797
// TODO: doc
// TODO: named slice/array/map types
// TODO: unnamed struct types

package format

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
)

// A Formatter formats Go values.
// It follows pointers recursively, detecting cycles.
// Configure a Formatter by setting the exported fields before
// calling a formatting method.
// The defaults are designed to work well in tests.
type Formatter struct {
	// ShowUnexported bool   // display unexported fields
	ShowZero     bool   // display struct fields that have their zero value
	MaxWidth     int    // maximum columns, but not breaking words
	Compact      bool   // as few lines as possible, observing MaxWidth
	Indent       string // ignored if Compact; default is 4 spaces
	MaxDepth     int    // max recursion depth; default is 100
	MaxElements  int    // max array, slice or map elements to print
	OmitPackage  bool   // don't print package in type names
	ignoreFields map[reflect.Type][]string
}

// New returns a new default Formatter.
func New() *Formatter {
	return &Formatter{}
}

// IgnoreFields causes f to skip printing of the named fields of values of structval's type.
// Structval must be a struct or a pointer to a struct.
// It returns its receiver.
func (f *Formatter) IgnoreFields(structval any, fields ...string) *Formatter {
	t := reflect.TypeOf(structval)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("%#v is not a struct or pointer to struct", structval))
	}
	if f.ignoreFields == nil {
		f.ignoreFields = map[reflect.Type][]string{}
	}
	f.ignoreFields[t] = append(f.ignoreFields[t], fields...)
	return f
}

// Sprint calls [Formatter.Sprint] with the default Formatter.
func Sprint(x any) string { return New().Sprint(x) }

// Print calls [Formatter.Print] with the default Formatter.
func Print(x any) error { return New().Print(x) }

// Fprint calls [Formatter.Fprint] with the default Formatter.
func Fprint(w io.Writer, x any) error { return New().Fprint(w, x) }

// Sprint formats x and returns a string.
func (f *Formatter) Sprint(x any) string {
	var buf bytes.Buffer
	_ = f.Fprint(&buf, x)
	return buf.String()
}

// Print formats x and writes to the standard output.
func (f *Formatter) Print(x any) error {
	return f.Fprint(os.Stdout, x)
}

// Fprint formats x and writes to w.
func (f *Formatter) Fprint(w io.Writer, x any) error {
	if f.Indent == "" {
		f.Indent = "    "
	}
	if f.MaxDepth <= 0 {
		f.MaxDepth = 100
	}
	s := &state{
		Formatter: f,
		w:         w,
		seen:      map[any]bool{},
		depth:     -1,
	}
	s.print(reflect.ValueOf(x))
	if s.err != nil {
		return s.err
	}
	if s.col != 0 && !f.Compact {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

type state struct {
	*Formatter
	w     io.Writer
	seen  map[any]bool
	depth int
	col   int
	err   error
}

func (s *state) deeper(f func()) {
	s.depth++
	defer func() { s.depth-- }()
	if s.depth > s.MaxDepth {
		s.pr("<maxdepth>")
		return
	}
	f()
}

func (s *state) print(v reflect.Value) {
	s.deeper(func() {
		s.printSameDepth(v)
	})
}

func (s *state) printSameDepth(v reflect.Value) {
	if !v.IsValid() {
		s.pr("nil")
		return
	}

	value := v.Interface()

	if v.Kind() == reflect.Pointer || v.Kind() == reflect.UnsafePointer {
		if s.seen[value] {
			s.prf("<cycle>")
			return
		} else {
			s.seen[value] = true
			defer func() { delete(s.seen, value) }()
		}
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.UnsafePointer,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.Bool:
		s.prf("%v", value)

	case reflect.String:
		s.prf("%q", v)

	case reflect.Interface:
		s.printSameDepth(v.Elem())

	case reflect.Pointer:
		s.pr("&")
		// TODO: no linebreak between & and the rest.
		s.printSameDepth(v.Elem())

	case reflect.Array, reflect.Slice:
		s.printSlice(v)

	case reflect.Map:
		s.printMap(v)

	case reflect.Struct:
		s.printStruct(v)

	case reflect.Func, reflect.Chan:
		s.prf("%s(%[1]v)", s.typeName(v.Type()))

	default:
		s.prf("<unknown reflect kind:%s>", v.Kind())
	}
}

// print slice or array
func (s *state) printSlice(v reflect.Value) {
	if v.Kind() == reflect.Array {
		s.prf("[%d]{", v.Len())
	} else {
		s.pr("[]{")
	}
	if !s.Compact {
		s.pr("\n")
	}
	for i := range v.Len() {
		if s.MaxElements > 0 && i >= s.MaxElements {
			if s.Compact {
				s.pr("...")
			} else {
				s.depth++
				s.pr("...\n")
				s.depth--
			}
			break
		}
		s.print(v.Index(i))
		if !s.Compact || i != v.Len()-1 {
			s.after(",")
		}
	}
	s.pr("}")
}

func (s *state) printMap(v reflect.Value) {
	keys := v.MapKeys()
	slices.SortFunc(keys, compareValues)
	// TODO: use mapiter for NaNs?
	s.pr("{")
	if !s.Compact {
		s.pr("\n")
	}
	for i, key := range keys {
		if s.MaxElements > 0 && i >= s.MaxElements {
			s.pr("...")
			break
		}
		val := v.MapIndex(key)
		s.print(key)
		s.between(":")
		s.print(val)
		if !s.Compact || i != len(keys)-1 {
			s.after(",")
		}
	}
	s.pr("}")
}

func (s *state) printStruct(v reflect.Value) {
	t := v.Type()
	ignore := s.ignoreFields[t]
	s.prf("%s{", s.typeName(t))
	if !s.Compact {
		s.pr("\n")
	}
	first := true
	for i := range t.NumField() {
		sf := t.Field(i)
		if slices.Contains(ignore, sf.Name) {
			continue
		}
		if len(sf.Index) != 1 {
			panic("len(index) != 1")
		}

		if !sf.IsExported() {
			continue
		}
		val := v.Field(i)
		if !s.ShowZero && val.IsZero() {
			continue
		}
		if !first && s.Compact {
			s.pr(", ")
		}
		s.deeper(func() { s.pr(sf.Name) })
		s.between(":")
		s.print(val)
		first = false
		if !s.Compact {
			s.pr("\n")
		}
	}
	s.pr("}")
}

func (s *state) typeName(t reflect.Type) string {
	n := t.String()
	if !s.OmitPackage {
		return n
	}
	if i := strings.LastIndex(n, "."); i > 0 {
		return n[i+1:]
	}
	return n
}

func (s *state) after(str string) {
	s.write(str)
	s.checkWidth("")
	if s.col != 0 {
		if s.Compact {
			s.write(" ")
		} else {
			s.write("\n")
		}
	}
}

func (s *state) between(str string) {
	s.write(str)
	s.checkWidth("")
	if s.col != 0 {
		s.write(" ")
	}
}

// Observe MaxWidth.
func (s *state) checkWidth(str string) {
	if s.MaxWidth > 0 && s.col+len(str) >= s.MaxWidth {
		s.write("\n")
	}
}

func (s *state) prf(format string, args ...any) {
	s.pr(fmt.Sprintf(format, args...))
}

func (s *state) pr(str string) {
	if s.err != nil {
		return
	}
	s.checkWidth(str)

	// Observe indent.
	if !s.Compact && s.col == 0 {
		for range s.depth {
			s.write(s.Indent)
		}
	}
	s.write(str)
}

func (s *state) write(str string) {
	if s.err != nil {
		return
	}
	_, s.err = io.WriteString(s.w, str)
	// Adjust col. Assume one column per character.
	if i := strings.LastIndex(str, "\n"); i >= 0 {
		s.col = len(str) - i - 1
	} else {
		s.col += len(str)
	}
}

// TODO: call Equal method if any.
// TODO: recurse into slices, arrays, pointers?
func compareValues(v1, v2 reflect.Value) int {
	if !v1.IsValid() && !v2.IsValid() {
		return 0
	}
	if !v1.IsValid() {
		return -1
	}
	if !v2.IsValid() {
		return 1
	}

	if v1.Kind() == reflect.Interface {
		v1 = v1.Elem()
	}
	if v2.Kind() == reflect.Interface {
		v2 = v2.Elem()
	}

	if t1, t2 := v1.Type(), v2.Type(); t1 != t2 {
		return cmp.Compare(t1.String(), t2.String())
	}
	if v1.CanInt() {
		return cmp.Compare(v1.Int(), v2.Int())
	}
	if v1.CanUint() {
		return cmp.Compare(v1.Uint(), v2.Uint())
	}
	if v1.CanFloat() {
		return cmp.Compare(v1.Float(), v2.Float())
	}
	// Either string or not cmp.Ordered; do our best.
	// TODO: prevent Sprint from blowing stack on non-pointer cycles.
	return cmp.Compare(fmt.Sprint(v1), fmt.Sprint(v2))
}

// isOrdered reports whether values of type t can be compared with <, >, etc.
func isOrdered(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

// type ptr struct {
// 	p    any
// 	next *ptr
// }

//	func (s *state) sawPtr(p any) bool {
//		for n := s.seen; n != nil; n = n.next {
//			if n.p == p {
//				return true
//			}
//		}
//		return false
//	}
// func (c Formatter) fprint(w io.Writer, v reflect.Value, level int) (err error) {
// 	pl := func(format string, args ...any) bool {
// 		if err != nil {
// 			return false
// 		}
// 		if level > 0 {
// 			if _, err = fmt.Fprintf(w, "%*s", level, "    "); err != nil {
// 				return false
// 			}
// 		}
// 		if _, err = fmt.Fprintf(w, format, args...); err != nil {
// 			return false
// 		}
// 		_, err = fmt.Fprintln(w)
// 		return err == nil
// 	}

// 	switch v.Kind() {
// 	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
// 		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
// 		reflect.Uintptr, reflect.UnsafePointer,
// 		reflect.Float32, reflect.Float64,
// 		reflect.Complex64, reflect.Complex128,
// 		reflect.Bool:
// 		pl("%v", v)
// 	case reflect.String:
// 		pl("%q", v)
// 	case reflect.Pointer, reflect.Interface:
// 		return c.fprint(w, v.Elem(), level)
// 	case reflect.Array, reflect.Slice:
// 		if !pl("[%d]%s{", v.Len(), v.Type().Elem()) {
// 			return err
// 		}
// 		for i := 0; i < v.Len(); i++ {
// 			if err := c.fprint(w, v.Index(i), level+1); err != nil {
// 				return err
// 			}
// 		}
// 		pl("}")
// 	case reflect.Map:
// 		pl("MAP")
// 	case reflect.Struct:
// 		pl("STRUCT")
// 	default:
// 		pl("%v", v)
// 	}
// 	return err
// }
