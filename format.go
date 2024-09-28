// TODO: MIT

// TODO: doc
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

type Formatter struct {
	HideUnexported bool   // do not display unexported fields
	HideZero       bool   // do not display fields that have their zero value
	MaxWidth       int    // maximum columns, but not breaking words
	Compact        bool   // as few lines as possible, observing MaxWidth
	Indent         string // ignored if Compact; default is 4 spaces
	MaxDepth       int    // max recursion depth; default is 100
	// MaxElements    int
}

func Sprint(x any) string { return (Formatter{}).Sprint(x) }

func Fprint(w io.Writer, x any) error { return (Formatter{}).Fprint(w, x) }

func Print(x any) error { return (Formatter{}).Print(x) }

func (f Formatter) Sprint(x any) string {
	var buf bytes.Buffer
	_ = f.Fprint(&buf, x)
	return buf.String()
}

func (f Formatter) Print(x any) error {
	return f.Fprint(os.Stdout, x)
}

func (f Formatter) Fprint(w io.Writer, x any) error {
	s := &state{
		Formatter: f,
		w:         w,
		seen:      map[any]bool{},
		depth:     -1,
	}
	if f.Indent == "" {
		s.Indent = "    "
	}
	if f.MaxDepth <= 0 {
		f.MaxDepth = 100
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
	Formatter
	w     io.Writer
	seen  map[any]bool
	depth int
	col   int
	err   error
}

func (s *state) print(v reflect.Value) {
	s.depth++
	defer func() { s.depth-- }()
	if s.depth > s.MaxDepth {
		s.pr("<maxdepth>")
		return
	}

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

	switch kind := v.Kind(); kind {
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
		s.print(v.Elem())

	case reflect.Pointer:
		s.pr("*")
		// TODO: no linebreak between * and the rest.
		// We also shouldn't increment depth; maybe s.printWithPrefix?
		s.print(v.Elem())

	case reflect.Array, reflect.Slice:
		if kind == reflect.Array {
			s.prf("[%d]{", v.Len())
		} else {
			s.pr("[]{")
		}
		for i := range v.Len() {
			if i > 0 && s.Compact {
				s.pr(", ")
			}
			s.print(v.Index(i))
			if !s.Compact {
				s.pr(",\n")
			}
		}
		s.pr("}")

	case reflect.Map:
		keys := v.MapKeys()
		slices.SortFunc(keys, compareValues)
		// TODO: use mapiter for NaNs.
		s.pr("{")
		for i, key := range keys {
			if i > 0 && s.Compact {
				s.pr(", ")
			}
			val := v.MapIndex(key)
			s.print(key)
			s.pr(": ")
			s.print(val)
		}
		s.pr("}")

	case reflect.Struct:
		s.pr("STRUCT")
	default:
		s.pr("<unknown reflect kind>")
	}
}

func (s *state) prf(format string, args ...any) {
	s.pr(fmt.Sprintf(format, args...))
}

func (s *state) pr(str string) {
	// Observe MaxWidth
	if s.MaxWidth > 0 && s.col+len(str) >= s.MaxWidth {
		s.write("\n")
		s.col = 0
	}

	// Observe indent.
	if s.col == 0 {
		for range s.depth {
			s.write(s.Indent)
		}
		s.col = s.depth * len(s.Indent)
	}

	s.write(str)
	s.col += len(str) // assume one column per character

	// If we wrote a newline, adjust col.
	if i := strings.LastIndex(str, "\n"); i > 0 {
		s.col = len(str) - i
	}
}

func (s *state) write(str string) {
	if s.err != nil {
		return
	}
	_, s.err = io.WriteString(s.w, str)
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
	return cmp.Compare(fmt.Sprint(v1), fmt.Sprint(v2))
}

// isOrdered reports whether values of type t can be compare with <, >, etc.
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
