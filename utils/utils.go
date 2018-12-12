package utils

import (
	"fmt"
	"reflect"
	"strings"
)

// IfThenElse evaluates a condition, if true returns the first parameter,
// Otherwise the second.
func IfThenElse(condition bool, a interface{}, b interface{}) interface{} {
	if condition {
		return a
	}

	return b
}

// Contains check if a interface in a interface slice.
func Contains(input []interface{}, value interface{}) (bool, error) {
	if value == nil || len(input) == 0 {
		return false, nil
	}

	if reflect.TypeOf(input[0]) != reflect.TypeOf(value) {
		return false, fmt.Errorf("interface type not equals")
	}

	switch v := value.(type) {
	case int, int64, float64, string:
		for _, v := range input {
			if v == value {
				return true, nil
			}
		}
		return false, nil
	// TODO: add more types
	default:
		r := reflect.TypeOf(v)
		return false, fmt.Errorf("Not support: %s", r)
	}
}

// StringInSlice checks if a string in the slice.
func StringInSlice(input []string, str string) bool {
	if str == "" || len(input) == 0 {
		return false
	}

	result := make([]interface{}, len(input))
	for i, v := range input {
		result[i] = v
	}

	exists, _ := Contains(result, str)
	return exists
}

// StringSliceEqual compare two string slice, ignore the order.
// we also should consider if there has duplicate items in slice.
func StringSliceEqual(s1, s2 []string) bool {
	if s1 == nil && s2 == nil {
		return true
	}
	if s1 == nil || s2 == nil {
		return false
	}
	if len(s1) != len(s2) {
		return false
	}
	// mapKeys to remember keys that exist in s1
	mapKeys := map[string]int{}
	// first list all items in s1
	for _, v := range s1 {
		mapKeys[v]++
	}
	// second list all items in s2
	for _, v := range s2 {
		mapKeys[v]--
		// we may get -1 in two cases:
		// 1. the item exists in the s2, but not in the s1;
		// 2. the item exists both in s1 and s2, but has different copies.
		// Under the condition that the length of slices are equals,
		// so we can quickly return false.
		if mapKeys[v] < 0 {
			return false
		}
	}
	return true
}

// RemoveDuplicateElement delete duplicate item from slice
func RemoveDuplicateElement(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	temp := map[string]struct{}{}
	for _, item := range addrs {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// SliceTrimSpace delete empty item, like " ", "\t", "\n" from slice
func SliceTrimSpace(input []string) []string {
	output := []string{}
	for _, item := range input {
		str := strings.TrimSpace(item)
		if str != "" {
			output = append(output, str)
		}
	}

	return output
}
