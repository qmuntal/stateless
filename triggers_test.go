package stateless

import (
	"testing"
)

func Test_invocationInfo_String(t *testing.T) {
	tests := []struct {
		name string
		inv  invocationInfo
		want string
	}{
		{"empty", invocationInfo{}, "<nil>"},
		{"named", invocationInfo{Method: "aaa"}, "aaa"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inv.String(); got != tt.want {
				t.Errorf("invocationInfo.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
