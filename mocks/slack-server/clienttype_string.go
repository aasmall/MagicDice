// Code generated by "stringer -type=ClientType"; DO NOT EDIT.

package main

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Normal-0]
	_ = x[Snooper-1]
}

const _ClientType_name = "NormalSnooper"

var _ClientType_index = [...]uint8{0, 6, 13}

func (i ClientType) String() string {
	if i < 0 || i >= ClientType(len(_ClientType_index)-1) {
		return "ClientType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _ClientType_name[_ClientType_index[i]:_ClientType_index[i+1]]
}
