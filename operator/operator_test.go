package operator

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	op := New(TypesAdd, "", 0, nil)
	op.OID = "sss"
	op.IID = 1001

	fmt.Printf("%+v", op)
}
