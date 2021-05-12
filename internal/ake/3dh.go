// Package ake provides high-level functions for the 3DH AKE.
package ake

import (
	"errors"
	"fmt"

	"github.com/bytemare/cryptotools/group"
	"github.com/bytemare/cryptotools/group/ciphersuite"
	"github.com/bytemare/cryptotools/utils"

	"github.com/bytemare/opaque/internal"
	"github.com/bytemare/opaque/internal/encoding"
	"github.com/bytemare/opaque/message"
)

var errInvalidSelector = errors.New("invalid selector (must be either client or server)")

type selector bool

const (
	client selector = true
	server selector = false
)

// KeyGen returns private and public keys in the group.
func KeyGen(id ciphersuite.Identifier) (sk, pk []byte) {
	g := id.Get(nil)
	scalar := g.NewScalar().Random()
	publicKey := g.Base().Mult(scalar)

	return internal.SerializeScalar(scalar, id), internal.SerializePoint(publicKey, id)
}

type keys struct {
	serverMacKey, clientMacKey []byte
	handshakeSecret            []byte
	handshakeEncryptKey        []byte
}

// setValues - testing: integrated to support testing, to force values.
// There's no effect if esk, epk, and nonce have already been set in a previous call.
func setValues(g group.Group, scalar group.Scalar, nonce []byte, nonceLen int) (s group.Scalar, n []byte) {
	if scalar != nil {
		s = scalar
	} else {
		s = g.NewScalar().Random()
	}

	if len(nonce) == 0 {
		nonce = utils.RandomBytes(nonceLen)
	}

	return s, nonce
}

func buildLabel(length int, label, context []byte) []byte {
	return utils.Concatenate(0,
		encoding.I2OSP(length, 2),
		encoding.EncodeVectorLen(append([]byte(internal.LabelPrefix), label...), 1),
		encoding.EncodeVectorLen(context, 1))
}

func expand(h *internal.KDF, secret, hkdfLabel []byte) []byte {
	return h.Expand(secret, hkdfLabel, h.Size())
}

func expandLabel(h *internal.KDF, secret, label, context []byte) []byte {
	hkdfLabel := buildLabel(h.Size(), label, context)
	return expand(h, secret, hkdfLabel)
}

func deriveSecret(h *internal.KDF, secret, label, context []byte) []byte {
	return expandLabel(h, secret, label, context)
}

func newInfo(h *internal.Hash, ke1 *message.KE1, idu, ids, response, nonceS, epks []byte) {
	cp := encoding.EncodeVectorLen(idu, 2)
	sp := encoding.EncodeVectorLen(ids, 2)
	h.Write(utils.Concatenate(0, []byte(internal.Tag3DH), cp, ke1.Serialize(), sp, response, nonceS, epks))
}

func deriveKeys(h *internal.KDF, ikm, context []byte) (k *keys, sessionSecret []byte) {
	prk := h.Extract(nil, ikm)
	k = &keys{}
	k.handshakeSecret = deriveSecret(h, prk, []byte(internal.TagHandshake), context)
	sessionSecret = deriveSecret(h, prk, []byte(internal.TagSession), context)
	k.serverMacKey = expandLabel(h, k.handshakeSecret, []byte(internal.TagMacServer), nil)
	k.clientMacKey = expandLabel(h, k.handshakeSecret, []byte(internal.TagMacClient), nil)
	k.handshakeEncryptKey = expandLabel(h, k.handshakeSecret, []byte(internal.TagEncServer), nil)

	return k, sessionSecret
}

func decodeKeys(g group.Group, secret, peerEpk, peerPk []byte) (sk group.Scalar, epk, pk group.Element, err error) {
	sk, err = g.NewScalar().Decode(secret)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decoding secret key: %w", err)
	}

	epk, err = g.NewElement().Decode(peerEpk)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decoding peer ephemeral public key: %w", err)
	}

	pk, err = g.NewElement().Decode(peerPk)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decoding peer public key: %w", err)
	}

	return sk, epk, pk, nil
}

func k3dh(p1 group.Element, s1 group.Scalar, p2 group.Element, s2 group.Scalar, p3 group.Element, s3 group.Scalar) []byte {
	e1 := p1.Mult(s1)
	e2 := p2.Mult(s2)
	e3 := p3.Mult(s3)

	return utils.Concatenate(0, e1.Bytes(), e2.Bytes(), e3.Bytes())
}

func ikm(s selector, g group.Group, esk group.Scalar, secretKey, peerEpk, peerPublicKey []byte) ([]byte, error) {
	sk, epk, gpk, err := decodeKeys(g, secretKey, peerEpk, peerPublicKey)
	if err != nil {
		return nil, err
	}

	switch s {
	case client:
		return k3dh(epk, esk, gpk, esk, epk, sk), nil
	case server:
		return k3dh(epk, esk, epk, sk, gpk, esk), nil
	}

	panic(errInvalidSelector)
}

func cryptInfo(p *internal.Parameters, key, info []byte) (out []byte) {
	if len(info) != 0 {
		pad := p.KDF.Expand(key, []byte(internal.EncryptionTag), len(info))
		out = internal.Xor(pad, info)
	}

	return out
}

func getServerMac(p *internal.Parameters, key, einfo []byte) []byte {
	p.Hash.Write(encoding.EncodeVector(einfo))
	return p.MAC.MAC(key, p.Hash.Sum()) // transcript2
}

type output struct {
	info, serverMac, clientMac []byte
}

func core3DH(s selector, p *internal.Parameters, esk group.Scalar, secretKey, peerEpk, peerPublicKey,
	epks, idu, ids, nonceS, credResp, info []byte, ke1 *message.KE1) (*output, []byte, error) {
	ikm, err := ikm(s, p.AKEGroup.Get(nil), esk, secretKey, peerEpk, peerPublicKey)
	if err != nil {
		return nil, nil, err
	}

	newInfo(p.Hash, ke1, idu, ids, credResp, nonceS, epks)
	keys, sessionSecret := deriveKeys(p.KDF, ikm, p.Hash.Sum())

	st := &output{}
	st.info = cryptInfo(p, keys.handshakeEncryptKey, info)

	switch s {
	case client:
		st.serverMac = getServerMac(p, keys.serverMacKey, info)
	case server:
		st.serverMac = getServerMac(p, keys.serverMacKey, st.info)
	default:
		panic(errInvalidSelector)
	}

	p.Hash.Write(st.serverMac)
	transcript3 := p.Hash.Sum()
	st.clientMac = p.MAC.MAC(keys.clientMacKey, transcript3)

	return st, sessionSecret, nil
}
