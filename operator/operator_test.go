package operator

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	op := &Operator{
		OID:   "sss",
		IID:   1001,
		OType: TypesAdd,
	}

	fmt.Printf("%+v", op)
}
