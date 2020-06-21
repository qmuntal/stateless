// +build gofuzz

package stateless

import "bytes"

func Fuzz(data []byte) int {
    if _, err := Decode(bytes.NewReader(data)); err != nil {
      return 0
    }
    return 1
}