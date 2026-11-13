package contractsapi

import (
	"math/big"
	"reflect"
	"testing"

	/* START ALPHA CHANGE 468 -- remove imports used only by state sync tests -- */
	/* FINISH ALPHA CHANGE 468 -- remove imports used only by state sync tests -- */
	"github.com/stretchr/testify/require"
)

type method interface {
	EncodeAbi() ([]byte, error)
	DecodeAbi(buf []byte) error
}

func TestEncoding_Method(t *testing.T) {
	t.Parallel()

	cases := []method{
		/* START ALPHA CHANGE 469 -- remove legacy state receiver encoding case -- */
		/* FINISH ALPHA CHANGE 469 -- remove legacy state receiver encoding case -- */
		// empty commit epoch
		&CommitEpochValidatorSetFn{
			ID: big.NewInt(1),
			Epoch: &Epoch{
				StartBlock: big.NewInt(1),
				EndBlock:   big.NewInt(1),
			},
		},
	}

	for _, c := range cases {
		res, err := c.EncodeAbi()
		require.NoError(t, err)

		// use reflection to create another type and decode
		val := reflect.New(reflect.TypeOf(c).Elem()).Interface()
		obj, ok := val.(method)
		require.True(t, ok)

		err = obj.DecodeAbi(res)
		require.NoError(t, err)
		require.Equal(t, obj, c)
	}
}

/* START ALPHA CHANGE 470 -- remove legacy state sync commitment encoding test -- */
/* FINISH ALPHA CHANGE 470 -- remove legacy state sync commitment encoding test -- */

/* START ALPHA CHANGE 456 -- remove L2 state sender bridge event encoding test -- */
/* FINISH ALPHA CHANGE 456 -- remove L2 state sender bridge event encoding test -- */
