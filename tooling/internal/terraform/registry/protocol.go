package registry

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/gravitational/trace"
	"golang.org/x/crypto/openpgp/armor"
)

const (
	versionsFilename = "versions"
)

func VersionsFilePath(prefix, namespace, name string) string {
	return path.Join(prefix, namespace, name, versionsFilename)
}

// Versions is a Go representation of the Terraform Registry Protocol
// `versions` file
type Versions struct {
	Versions []*Version `json:"versions"`
}

func NewVersions() Versions {
	return Versions{Versions: []*Version{}}
}

func (idx *Versions) Save(filename string) error {
	indexFile, err := os.Create(filename)
	if err != nil {
		return trace.Wrap(err)
	}
	defer indexFile.Close()

	encoder := json.NewEncoder(indexFile)
	encoder.SetIndent("", "  ")

	return encoder.Encode(idx)
}

func LoadVersionsFile(filename string) (Versions, error) {
	f, err := os.Open(filename)
	if err != nil {
		return Versions{}, trace.Wrap(err, "failed opening versions file")
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	var idx Versions
	if err = decoder.Decode(&idx); err != nil {
		return Versions{}, trace.Wrap(err, "failed decoding versions file")
	}

	return idx, nil
}

type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Version struct {
	Version   string     `json:"version"`
	Protocols []string   `json:"protocols"`
	Platforms []Platform `json:"platforms"`
}

type GpgPublicKey struct {
	KeyID          string `json:"key_id"`
	ASCIIArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
	Source         string `json:"source,omitempty"`
	SourceURL      string `json:"source_url,omitempty"`
}

type SigningKeys struct {
	GpgPublicKeys []GpgPublicKey `json:"gpg_public_keys"`
}

type Download struct {
	Protocols    []string    `json:"protocols"`
	OS           string      `json:"os"`
	Arch         string      `json:"arch"`
	Filename     string      `json:"filename"`
	DownloadURL  string      `json:"download_url"`
	ShaURL       string      `json:"shasums_url"`
	SignatureURL string      `json:"shasums_signature_url"`
	Sha          string      `json:"shasum"`
	SigningKeys  SigningKeys `json:"signing_keys"`
}

func (dl *Download) Save(indexDir, namespace, name, version string) error {
	filename := filepath.Join(indexDir, namespace, name, version, "download", dl.OS, dl.Arch)

	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return trace.Wrap(err, "Creating download dir")
	}

	f, err := os.Create(filename)
	if err != nil {
		return trace.Wrap(err, "Failed opening file")
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")

	return encoder.Encode(dl)
}

func NewDownloadFromRepackResult(info *RepackResult, protocols []string, objectStoreURL string) (*Download, error) {

	keyText, err := formatPublicKey(info.SigningEntity)
	if err != nil {
		return nil, trace.Wrap(err, "failed formatting public key")
	}

	return &Download{
		Protocols:    protocols,
		OS:           info.OS,
		Arch:         info.Arch,
		Filename:     info.Zip,
		Sha:          info.Sha256String(),
		DownloadURL:  objectStoreURL + info.Zip,
		ShaURL:       objectStoreURL + info.Sum,
		SignatureURL: objectStoreURL + info.Sig,
		SigningKeys: SigningKeys{
			GpgPublicKeys: []GpgPublicKey{
				{
					KeyID:      info.SigningEntity.PrivateKey.PublicKey.KeyIdString(),
					ASCIIArmor: keyText,
				},
			},
		},
	}, nil
}

func formatPublicKey(signingKey *openpgp.Entity) (string, error) {
	var text bytes.Buffer
	writer, err := armor.Encode(&text, openpgp.PublicKeyType, nil)
	if err != nil {
		return "", trace.Wrap(err)
	}

	err = signingKey.Serialize(writer)
	if err != nil {
		return "", trace.Wrap(err)
	}

	if err = writer.Close(); err != nil {
		return "", trace.Wrap(err)
	}

	return text.String(), nil
}
