package utils

import (
	"reflect"
	"testing"
)

func TestStringSliceEqual(t *testing.T) {
	type args struct {
		s1 []string
		s2 []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{s1: []string{"foo", "bar"}, s2: []string{"bar", "foo"}}, want: true},
		{name: "test2", args: args{s1: []string{"foo1", "bar"}, s2: []string{"bar", "foo"}}, want: false},
		{name: "test3", args: args{s1: []string{"a", "a", "b", "c"}, s2: []string{"a", "b", "b", "c"}}, want: false},
		{name: "test4", args: args{s1: []string{"a", "b", "c", "b"}, s2: []string{"a", "b", "b", "c"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StringSliceEqual(tt.args.s1, tt.args.s2); got != tt.want {
				t.Errorf("StringSliceEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveDuplicateElement(t *testing.T) {
	type args struct {
		addrs []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{addrs: []string{"a", "b", "b", "c", "d", "d", "d"}}, want: []string{"a", "b", "c", "d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RemoveDuplicateElement(tt.args.addrs); !StringSliceEqual(got, tt.want) {
				t.Errorf("RemoveDuplicateElement() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSliceTrimSpace(t *testing.T) {
	type args struct {
		input []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{input: []string{"foo"}}, want: []string{"foo"}},
		{name: "test2", args: args{input: []string{"foo", ""}}, want: []string{"foo"}},
		{name: "test3", args: args{input: []string{"foo", "  "}}, want: []string{"foo"}},
		{name: "test4", args: args{input: []string{"foo", "\t"}}, want: []string{"foo"}},
		{name: "test4", args: args{input: []string{"foo", "\n"}}, want: []string{"foo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SliceTrimSpace(tt.args.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SliceTrimSpace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIfThenElse(t *testing.T) {
	type args struct {
		condition bool
		a         interface{}
		b         interface{}
	}
	tests := []struct {
		name string
		args args
		want interface{}
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{condition: false, a: "string", b: "bool"}, want: "bool"},
		{name: "test2", args: args{condition: false, a: "string", b: false}, want: false},
		{name: "test3", args: args{condition: false, a: "false", b: false}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IfThenElse(tt.args.condition, tt.args.a, tt.args.b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IfThenElse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	type args struct {
		input []interface{}
		value interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "test1", args: args{input: []interface{}{"a", "b"}, value: "a"}, want: true, wantErr: false},
		{name: "test2", args: args{input: []interface{}{"a", "b"}, value: false}, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Contains(tt.args.input, tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Contains() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringInSlice(t *testing.T) {
	type args struct {
		input []string
		str   string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
		{
			name: "testOK",
			args: args{
				input: []string{"test", "foo", "bar"},
				str:   "test",
			},
			want: true,
		},
		{
			name: "testNotOK",
			args: args{
				input: []string{"test", "foo", "bar"},
				str:   "notExist",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StringInSlice(tt.args.input, tt.args.str); got != tt.want {
				t.Errorf("StringInSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}
