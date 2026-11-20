package main

import (
	"encoding/hex"
	"fmt"

	"qqtang/internal/accountauth"
)

func main() {
	key := accountauth.DeriveKey([]byte("123456"), []byte("0123456789abcdef"), 1000)
	fmt.Println(hex.EncodeToString(key))
}
