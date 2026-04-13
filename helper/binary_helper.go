package helper

import (
	"fmt"
)

//IntToBinary returns the binary of the interger in string format in reversed order
//otherwise index of a location in binary will give wrong result.
//For e.g. inBinary[10] will start from left but in binary 10 postion should check from right
//When the binary is in reverse inBinary[10] is valid.
func IntToBinary(value int) string {
	// if value > 0 {
	// 	return reverse(fmt.Sprintf("%032b", value))
	// }
	// return reverse(bInt(int64(value)))
	return reverse(fmt.Sprintf("%032b", value))
}

// func bInt(n int64) string {
// 	return strconv.FormatUint(*(*uint64)(unsafe.Pointer(&n)), 2)
// }

func reverse(str string) (result string) {
	// fmt.Println("orig", str)
	for _, v := range str {
		result = string(v) + result
	}
	return

	// r := []rune(str)
	// for i, j := 0, len(r)-1; i < len(r)/2; i, j = i+1, j-1 {
	// 	r[i], r[j] = r[j], r[i]
	// }
	// return string(r)
}
