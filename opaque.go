package opaque

import (
	"errors"
	"fmt"

	"github.com/bytemare/cryptotools/hash"
	"github.com/bytemare/cryptotools/mhf"
	"github.com/bytemare/voprf"

	"github.com/bytemare/opaque/internal"
	"github.com/bytemare/opaque/internal/encoding"
	"github.com/bytemare/opaque/internal/envelope"
	"github.com/bytemare/opaque/message"
)

// Mode designates OPAQUE's envelope mode.
type Mode byte

const (
	// Internal designates the internal mode.
	Internal Mode = iota + 1

	// External designates the external mode.
	External
)

// Group identifies the prime-order group with hash-to-curve capability to use in OPRF and AKE.
type Group byte

const (
	// RistrettoSha512 identifies the Ristretto255 group and SHA-512.
	RistrettoSha512 = Group(voprf.RistrettoSha512)

	// decaf448Shake256 identifies the Decaf448 group and Shake-256.
	// decaf448Shake256 = 2.

	// P256Sha256 identifies the NIST P-256 group and SHA-256.
	P256Sha256 = Group(voprf.P256Sha256)

	// P384Sha512 identifies the NIST P-384 group and SHA-512.
	P384Sha512 = Group(voprf.P384Sha512)

	// P521Sha512 identifies the NIST P-512 group and SHA-512.
	P521Sha512 = Group(voprf.P521Sha512)
)

// String implements the Stringer() interface for the Group.
func (g *Group) String() string {
	return voprf.Ciphersuite(*g).String()
}

// Credentials holds the client and server ids (will certainly disappear in next versions°.
type Credentials struct {
	Client, Server              []byte
	TestEnvNonce, TestMaskNonce []byte
}

// Configuration represents an OPAQUE configuration. Note that OprfGroup and AKEGroup are recommended to be the same,
// as well as KDF, MAC, Hash should be the same.
type Configuration struct {
	// OprfGroup identifies the OPRF ciphersuite to be used.
	OprfGroup Group `json:"oprf"`

	// KDF identifies the hash function to be used for key derivation (e.g. HKDF).
	// Identifiers are defined in github.com/bytemare/cryptotools/hash.
	KDF hash.Hashing `json:"kdf"`

	// MAC identifies the hash function to be used for message authentication (e.g. HMAC).
	// Identifiers are defined in github.com/bytemare/cryptotools/hash.
	MAC hash.Hashing `json:"mac"`

	// Hash identifies the hash function to be used for hashing, as defined in github.com/bytemare/cryptotools/hash.
	Hash hash.Hashing `json:"hash"`

	// MHF identifies the memory-hard function for expensive key derivation on the client,
	// defined in github.com/bytemare/cryptotools/mhf.
	MHF mhf.Identifier `json:"mhf"`

	// Mode identifies the envelope mode to be used.
	Mode Mode `json:"mode"`

	// AKEGroup identifies the prime-order group to use in the AKE.
	AKEGroup Group `json:"group"`

	// NonceLen identifies the length to use for nonces. 32 is the recommended value.
	NonceLen int `json:"nn"`
}

func (c *Configuration) toInternal() *internal.Parameters {
	g := voprf.Ciphersuite(c.OprfGroup)
	cs := g.Group()

	ip := &internal.Parameters{
		OprfCiphersuite: g,
		KDF:             &internal.KDF{H: c.KDF.Get()},
		MAC:             &internal.Mac{H: c.MAC.Get()},
		Hash:            &internal.Hash{H: c.Hash.Get()},
		MHF:             &internal.MHF{MHF: c.MHF.Get()},
		AKEGroup:        cs,
		NonceLen:        c.NonceLen,
		EnvelopeSize:    envelope.Size(envelope.Mode(c.Mode), c.NonceLen, c.MAC.Size(), cs),
	}
	ip.Init()

	return ip
}

// Serialize returns the byte encoding of the Configuration structure.
func (c *Configuration) Serialize() []byte {
	b := make([]byte, 8)
	b[0] = byte(c.OprfGroup)
	b[1] = byte(c.KDF)
	b[2] = byte(c.MAC)
	b[3] = byte(c.Hash)
	b[4] = byte(c.MHF)
	b[5] = byte(c.Mode)
	b[6] = byte(c.AKEGroup)
	b[7] = encoding.I2OSP(c.NonceLen, 1)[0]

	return b
}

// Client returns a newly instantiated Client from the Configuration.
func (c *Configuration) Client() *Client {
	return NewClient(c)
}

// Server returns a newly instantiated Server from the Configuration.
func (c *Configuration) Server() *Server {
	return NewServer(c)
}

// String returns a string representation of the parameter set.
func (c *Configuration) String() string {
	return fmt.Sprintf("%s-%s-%s-%s-%s-%v-%s-%d",
		c.OprfGroup.String(), c.KDF, c.MAC, c.Hash, c.MHF, c.Mode, c.AKEGroup.String(), c.NonceLen)
}

var errInvalidLength = errors.New("invalid length")

// DeserializeConfiguration decodes the input and returns a Parameter structure. This assumes that the encoded parameters
// are valid, and will not be checked.
func DeserializeConfiguration(encoded []byte) (*Configuration, error) {
	if len(encoded) != 8 {
		return nil, errInvalidLength
	}

	return &Configuration{
		OprfGroup: Group(encoded[0]),
		KDF:       hash.Hashing(encoded[1]),
		MAC:       hash.Hashing(encoded[2]),
		Hash:      hash.Hashing(encoded[3]),
		MHF:       mhf.Identifier(encoded[4]),
		Mode:      Mode(encoded[5]),
		AKEGroup:  Group(encoded[6]),
		NonceLen:  encoding.OS2IP(encoded[7:]),
	}, nil
}

// DefaultConfiguration returns a default configuration with strong parameters.
func DefaultConfiguration() *Configuration {
	return &Configuration{
		OprfGroup: RistrettoSha512,
		KDF:       hash.SHA512,
		MAC:       hash.SHA512,
		Hash:      hash.SHA512,
		MHF:       mhf.Scrypt,
		Mode:      Internal,
		AKEGroup:  RistrettoSha512,
		NonceLen:  32,
	}
}

// ClientRecord is a server-side structure enabling the storage of user relevant information.
type ClientRecord struct {
	CredentialIdentifier []byte
	ClientIdentity       []byte
	*message.RegistrationUpload

	// testing
	TestMaskNonce []byte
}
