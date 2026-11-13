/* START ALPHA CHANGE 746 -- add MDBX blockchain storage batch writer -- */
package mdbx

import (
	mdbxgo "github.com/erigontech/mdbx-go/mdbx"
	"github.com/innovativebioresearch/innbc-chain/blockchain/storage"
)

var _ storage.Batch = (*batchMDBX)(nil)

type batchMDBX struct {
	env          *mdbxgo.Env
	dbi          mdbxgo.DBI
	keysToDelete [][]byte
	valuesToPut  [][2][]byte
}

func NewBatchMDBX(env *mdbxgo.Env, dbi mdbxgo.DBI) *batchMDBX {
	return &batchMDBX{
		env: env,
		dbi: dbi,
	}
}

func (b *batchMDBX) Delete(key []byte) {
	b.keysToDelete = append(b.keysToDelete, copyBytes(key))
}

func (b *batchMDBX) Put(k []byte, v []byte) {
	b.valuesToPut = append(b.valuesToPut, [2][]byte{
		copyBytes(k),
		copyBytes(v),
	})
}

func (b *batchMDBX) Write() error {
	return b.env.Update(func(txn *mdbxgo.Txn) error {
		for _, key := range b.keysToDelete {
			if err := txn.Del(b.dbi, key, nil); err != nil && !mdbxgo.IsNotFound(err) {
				return err
			}
		}

		for _, value := range b.valuesToPut {
			if err := txn.Put(b.dbi, value[0], value[1], 0); err != nil {
				return err
			}
		}

		return nil
	})
}

/* FINISH ALPHA CHANGE 746 -- add MDBX blockchain storage batch writer -- */
