// Copyright (c) 2024 Jonathan Amsterdam
// Use of this source code is governed by the license in the LICENSE file.

package format

import (
	"fmt"
	"reflect"
	"testing"

	"golang.org/x/tools/txtar"
)

func TestSprint(t *testing.T) {
	ar, err := txtar.ParseFile("uncompact.txt")
	if err != nil {
		t.Fatal(err)
	}
	uwants := map[string]string{}
	for _, f := range ar.Files {
		uwants[f.Name] = string(f.Data)
	}

	for _, test := range []struct {
		f             Formatter
		in            any
		want          string
		wantUncompact string // txtar file name
		unprintable   bool
	}{
		// {in: nil, want: "nil"},
		// {in: 123, want: "123"},
		// {in: true, want: "true", wantUncompact: "bool"},
		// {in: 1.5, want: "1.5"},
		// {in: 3 - 4i, want: "(3-4i)"},
		// {in: ptr(5), want: "&5"},
		{
			in:            []int{2, 3, 4},
			want:          "[]{2, 3, 4}",
			wantUncompact: "intslice1",
		},
		{
			in:            []int{2, 3, 4, 5, 6, 99, 100},
			want:          "[]{2, 3, 4, 5, 6, ...}",
			wantUncompact: "intslice2",
		},
		{
			in:            [...]string{"", "x"},
			want:          `[2]{"", "x"}`,
			wantUncompact: "array",
		},
		{
			in:   []*int{ptr(4), ptr(5)},
			want: "[]{&4, &5}",
		},
		{
			in:            map[string]int{"b": 2, "a": 1},
			want:          `{"a": 1, "b": 2}`,
			wantUncompact: "map1",
		},
		{
			in:   map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 99: 99},
			want: `{1: 1, 2: 2, 3: 3, 4: 4, 5: 5, ...}`,
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
			want:          "[]{1, &[]{1, <cycle>}}",
			wantUncompact: "sliceCycle",
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
		{
			in:   Player{"Al", 11, true},
			want: `format.Player{Name: "Al", Score: 11}`,
		},
		{
			in:   Player{"Al", 0, true},
			want: `format.Player{Name: "Al"}`, // zeroes elided
		},
		{
			in:   Player{},
			want: `format.Player{}`, // zeroes elided
		},
		{
			in:   &node{1, &node{2, nil}},
			want: "&format.node{I: 1, Next: &format.node{I: 2}}",
		},
		{
			in: func() any {
				n := &node{I: 1}
				n.Next = n
				return n
			}(),
			want: "&format.node{I: 1, Next: <cycle>}",
		},
		// { doesn't work too well, but maybe we only care about Compact=false
		// 	f:    Formatter{MaxWidth: 20},
		// 	in:   []int{1000, 2000, 3000, 4000},
		// 	want: "[]{1000, 2000, 3000}",
		// },
	} {
		for _, c := range []bool{ /*true, */ false} {
			if !c && test.wantUncompact == "" {
				continue
			}
			test.f.Compact = c
			want := test.want
			if !c {
				var ok bool
				want, ok = uwants[test.wantUncompact]
				if !ok {
					t.Fatalf("txtar file missing %q", test.wantUncompact)
				}
			}
			test.f.MaxDepth = 5
			test.f.MaxElements = 5
			got := test.f.Sprint(test.in)
			if got != want {
				in := "<unprintable>"
				if !test.unprintable {
					in = fmt.Sprintf("%+v", test.in)
				}
				t.Errorf("%+v.Sprint(%s):\ngot\n%q\nwant\n%q", test.f, in, got, want)
			}
		}
	}
}

type Player struct {
	Name   string
	Score  int
	hidden bool
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
	I    int
	Next *node
}
