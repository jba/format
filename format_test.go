// MIT

package format

import (
	"fmt"
	"reflect"
	"testing"
)

func TestCompact(t *testing.T) {
	for _, test := range []struct {
		f           Formatter
		in          any
		want        string
		unprintable bool
	}{
		{in: nil, want: "nil"},
		{in: 123, want: "123"},
		{in: true, want: "true"},
		{in: 1.5, want: "1.5"},
		{in: 3 - 4i, want: "(3-4i)"},
		{in: ptr(5), want: "*5"},
		{
			in:   []int{2, 3, 4},
			want: "[]{2, 3, 4}",
		},
		{
			in:   [...]string{"", "x"},
			want: `[2]{"", "x"}`,
		},
		{
			in:   []*int{ptr(4), ptr(5)},
			want: "[]{*4, *5}",
		},
		{
			in:   map[string]int{"b": 2, "a": 1},
			want: `{"a": 1, "b": 2}`,
		},
		{
			in:   map[any]int{"b": 2, 13: 1, 3 + 5i: 3, 7i: 4, 21: 5},
			want: `{(0+7i): 4, (3+5i): 3, 13: 1, 21: 5, "b": 2}`,
		},
		{
			in: func() any {
				s := []any{1, nil}
				s[1] = &s
				return s
			}(),
			want: "[]{1, *[]{1, <cycle>}}",
		},
		{
			// MaxDepth handles this case.
			// fmt.Print goes into infinite recursion on it.
			in: func() any {
				s := []any{1, nil}
				s[1] = s
				return s
			}(),
			want:        "[]{1, []{1, []{<maxdepth>, <maxdepth>}}}",
			unprintable: true,
		},
		// {
		// 	in: &node{1, &node{2, &node{3, nil}}},
		// 	// want:
		// }
	} {
		test.f.Compact = true
		test.f.MaxDepth = 5
		got := test.f.Sprint(test.in)
		if got != test.want {
			in := "<unprintable>"
			if !test.unprintable {
				in = fmt.Sprintf("%+v", test.in)
			}
			t.Errorf("%+v.Sprint(%s):\ngot  %s\nwant %s", test.f, in, got, test.want)
		}
	}
}

func TestCompareValues(t *testing.T) {
	for _, test := range []struct {
		a, b any
		want int
	}{
		{nil, nil, 0},
		{nil, 1, -1},
		{1, 2, -1},
		{1, 1, 0},
		{uint(3), uint(4), -1},
		{1.5, -5.0, 1},
		{1.5, -5, -1}, // "float64" < "int"
		{"a", "b", -1},
		{1 + 2i, 7i, 1}, // "(0+7i)" < "(1+2i)"
		{
			[]any{1 + 2i},
			[]any{7i},
			1,
		},
		// {ptr(1), ptr(2), 0}, // will vary with pointer value
	} {
		va := reflect.ValueOf(test.a)
		if va.Kind() == reflect.Slice {
			va = va.Index(0)
		}
		vb := reflect.ValueOf(test.b)
		if vb.Kind() == reflect.Slice {
			vb = vb.Index(0)
		}
		got := compareValues(va, vb)
		if got != test.want {
			t.Errorf("compareValues(%v, %v) = %d, want %d", test.a, test.b, got, test.want)
		}
		if test.want != 0 {
			got = compareValues(vb, va)
			if got != -test.want {
				t.Errorf("compareValues(%v, %v) = %d, want %d", test.b, test.a, got, -test.want)
			}
		}
	}
}

func ptr[T any](t T) *T { return &t }

type node struct {
	i    int
	next *node
}
