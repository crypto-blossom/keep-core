// Package dkg conatins code that implements Distributed Key Generation protocol
// described in [GJKR 99].
//
// See http://docs.keep.network/cryptography/beacon_dkg.html#_protocol
//
//     [GJKR 99]: Gennaro R., Jarecki S., Krawczyk H., Rabin T. (1999) Secure
//         Distributed Key Generation for Discrete-Log Based Cryptosystems. In:
//         Stern J. (eds) Advances in Cryptology — EUROCRYPT ’99. EUROCRYPT 1999.
//         Lecture Notes in Computer Science, vol 1592. Springer, Berlin, Heidelberg
//         http://groups.csail.mit.edu/cis/pubs/stasio/vss.ps.gz
package dkg

import (
	"math/big"
)

// calculateShare calculates a share for given memberID.
//
// It calculates `Σ a_j * z^j mod q`for j in [0..T], where:
// - `a_j` is j coefficient
// - `z` is memberID
// - `T` is threshold
func calculateShare(memberID *big.Int, coefficients []*big.Int, mod *big.Int) *big.Int {
	result := big.NewInt(0)
	for j, a := range coefficients {
		result = new(big.Int).Mod(
			new(big.Int).Add(
				result,
				new(big.Int).Mul(
					a,
					new(big.Int).Exp(memberID, big.NewInt(int64(j)), mod),
				),
			),
			mod,
		)
	}
	return result
}
