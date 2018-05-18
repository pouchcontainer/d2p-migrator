package utils

import (
	"reflect"
	"testing"
)

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
		// TODO: Add test cases
		{name: "test1", args: args{condition: 1 == 1, a: "Yes", b: "No"}, want: "Yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IfThenElse(tt.args.condition, tt.args.a, tt.args.b); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("IfThenElse() = %v, want %v", got, tt.want)
			}
		})
	}
}
