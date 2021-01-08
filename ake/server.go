package ake

import (
	"github.com/bytemare/cryptotools/encoding"
	"github.com/bytemare/opaque/internal"
)

type response func(core *internal.Core, m *internal.Metadata, nonceLen int, sk, pku, req, info2 []byte, enc encoding.Encoding) ([]byte, []byte, error)
type serverFinalize func(core *internal.Core, info3, einfo3, req []byte, enc encoding.Encoding) error

type Server struct {
	id Identifier
	*internal.Core
	sk []byte
	response
	finalize serverFinalize
}

func (s *Server) Identifier() Identifier {
	return s.id
}

func (s *Server) PrivateKey() []byte {
	return s.sk
}

func (s *Server) Response(m *internal.Metadata, nonceLen int, pku, req, info2 []byte, enc encoding.Encoding) ([]byte, []byte, error) {
	return s.response(s.Core, m, nonceLen, s.sk, pku, req, info2, enc)
}

func (s *Server) Finalize(info3, einfo3, req []byte, enc encoding.Encoding) error {
	return s.finalize(s.Core, info3, einfo3, req, enc)
}

func (s *Server) SessionKey() []byte {
	return s.SessionSecret
}