package storage

import (
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/fastrlp"
)

type Batch interface {
	Delete(key []byte)
	Write() error
	Put(k []byte, v []byte)
}

type BatchWriter struct {
	batch Batch
}

func NewBatchWriter(storage Storage) *BatchWriter {
	return &BatchWriter{batch: storage.NewBatch()}
}

/* START ALPHA CHANGE 1554 -- select the physical storage segment by block number -- */
func NewBatchWriterForBlock(storage Storage, blockNumber uint64) *BatchWriter {
	if blockStorage, ok := storage.(BlockBatchStorage); ok {
		return &BatchWriter{batch: blockStorage.NewBatchForBlock(blockNumber)}
	}

	return NewBatchWriter(storage)
}

/* FINISH ALPHA CHANGE 1554 -- select the physical storage segment by block number -- */

func (b *BatchWriter) PutHeader(h *types.Header) {
	b.putRlp(HEADER, h.Hash.Bytes(), h)
}

func (b *BatchWriter) PutBody(hash types.Hash, body *types.Body) {
	/* START ALPHA CHANGE 1471 -- omit reconstructible empty block body records -- */
	if body != nil && len(body.Transactions) == 0 && len(body.Uncles) == 0 {
		return
	}
	/* FINISH ALPHA CHANGE 1471 -- omit reconstructible empty block body records -- */

	b.putRlp(BODY, hash.Bytes(), body)
}

func (b *BatchWriter) PutHeadHash(h types.Hash) {
	b.putWithPrefix(HEAD, HASH, h.Bytes())
}

func (b *BatchWriter) PutTxLookup(hash types.Hash, blockHash types.Hash) {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes()).MarshalTo(nil)

	b.putWithPrefix(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

func (b *BatchWriter) PutHeadNumber(n uint64) {
	b.putWithPrefix(HEAD, NUMBER, common.EncodeUint64ToBytes(n))
}

func (b *BatchWriter) PutReceipts(hash types.Hash, receipts []*types.Receipt) {
	/* START ALPHA CHANGE 1472 -- omit reconstructible empty receipt records -- */
	if len(receipts) == 0 {
		return
	}
	/* FINISH ALPHA CHANGE 1472 -- omit reconstructible empty receipt records -- */

	rr := types.Receipts(receipts)

	b.putRlp(RECEIPTS, hash.Bytes(), &rr)
}

/* START ALPHA CHANGE 1587 -- remove superseded typed history-record deletes -- */
/* FINISH ALPHA CHANGE 1587 -- remove superseded typed history-record deletes -- */

/* START ALPHA CHANGE 776 -- remove total-difficulty and fork writes from canonical header storage -- */
func (b *BatchWriter) PutCanonicalHeader(h *types.Header) {
	b.PutHeader(h)
	b.PutHeadHash(h.Hash)
	b.PutHeadNumber(h.Number)
	b.PutCanonicalHash(h.Number, h.Hash)
}

func (b *BatchWriter) PutCanonicalHash(n uint64, hash types.Hash) {
	b.putWithPrefix(CANONICAL, common.EncodeUint64ToBytes(n), hash.Bytes())
}

/* FINISH ALPHA CHANGE 776 -- remove total-difficulty and fork writes from canonical header storage -- */

func (b *BatchWriter) putRlp(p, k []byte, raw types.RLPMarshaler) {
	var data []byte

	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	b.putWithPrefix(p, k, data)
}

func (b *BatchWriter) putWithPrefix(p, k, data []byte) {
	fullKey := append(append(make([]byte, 0, len(p)+len(k)), p...), k...)

	b.batch.Put(fullKey, data)
}

/* START ALPHA CHANGE 1588 -- remove unused prefixed-delete helper -- */
/* FINISH ALPHA CHANGE 1588 -- remove unused prefixed-delete helper -- */

func (b *BatchWriter) WriteBatch() error {
	return b.batch.Write()
}
