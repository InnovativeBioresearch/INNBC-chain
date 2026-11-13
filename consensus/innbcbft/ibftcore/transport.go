/* START ALPHA CHANGE 1853 -- move IBFT transport interface into INNBC ownership -- */
package ibftcore

/* FINISH ALPHA CHANGE 1853 -- move IBFT transport interface into INNBC ownership -- */

/* START ALPHA CHANGE 1863 -- use INNBC-owned IBFT wire types -- */
import proto "github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/ibftproto"

/* FINISH ALPHA CHANGE 1863 -- use INNBC-owned IBFT wire types -- */

// Transport defines an interface
// the node uses to communicate with other peers
type Transport interface {
	// Multicast multicasts the message to other peers
	Multicast(message *proto.Message)
}
