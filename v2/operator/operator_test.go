package operator

import (
	"fmt"
	"testing"
)

func TestNew(t *testing.T) {
	op := &Operator{
		OID:  "sss",
		IID:  1001,
		Type: Types_Add,
	}

	//op.String()
	//b, _ := json.Marshal(op)
	fmt.Printf("%v", op.String())
}
