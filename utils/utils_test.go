package utils

import (
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
