package token

import (
	"fmt"
	"testing"
)

func TestToken(test *testing.T) {
	t := NewToken(EXP_1DAY)
	t.Contents["username"] = "dfindley"
	webtoken, err := t.Sign([]byte("secret"))
	if err != nil {
		test.Fatal(err)
	}
	fmt.Println(string(webtoken))

	// Attempt to validate token
	vt, err := Validate(webtoken, []byte("secret"))
	if err != nil {
		test.Fatal(err)
	}
	fmt.Printf("%v\n", vt)
}
