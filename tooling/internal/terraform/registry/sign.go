package registry

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/gravitational/teleport-plugins/tooling/internal/filename"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// FileNames describes the location of a registry-compatible zipfile and its
// associated sidecar files
type FileNames struct {
	Zip string
	Sum string
	Sig string
}

func IsProviderTarball(fn string) bool {
	info, err := filename.Parse(fn)
	if err != nil {
		return false
	}

	return info.Type == "terraform-provider"
}

func makeFileNames(info filename.Info) FileNames {
	zipFileName := info.Filename(".zip")
	return FileNames{
		Zip: zipFileName,
		Sum: zipFileName + ".sums",
		Sig: zipFileName + ".sums.sig",
	}
}

// RepackResult describes a fully-repacked provider and all of its sidecar files
type RepackResult struct {
	filename.Info
	FileNames
	Sha256        []byte
	SigningEntity *openpgp.Entity
}

func (r *RepackResult) Sha256String() string {
	return hex.EncodeToString(r.Sha256)
}

// RepackProvider takes a provider tarball and repacks it as a zipfile compatible
// with a terraform provider registry, generating all the required sidecar files
// as well. Returns a `RepackResult` instance containing the location of the
// generated files and information about the packed plugin
func RepackProvider(dstDir string, srcFileName string, signingEntity *openpgp.Entity) (*RepackResult, error) {
	info, err := filename.Parse(srcFileName)
	if err != nil {
		return nil, trace.Wrap(err, "Bad filename")
	}

	log.Infof("Provider platform: %s/%s/%s\n", info.Version, info.OS, info.Arch)

	src, err := os.Open(srcFileName)
	if err != nil {
		return nil, trace.Wrap(err, "failed opening source file")
	}
	defer src.Close()

	// Create the zip archive in memory in order to make it easier to
	// hash and sign
	var zipArchive bytes.Buffer

	err = repack(&zipArchive, src)
	if err != nil {
		return nil, trace.Wrap(err, "failed repacking provider")
	}

	result := &RepackResult{
		Info:          info,
		FileNames:     makeFileNames(info),
		SigningEntity: signingEntity,
	}

	// compute sha256 and format the sha file as per sha256sum
	var sums bytes.Buffer
	result.Sha256, err = sha256Sum(&sums, result.Zip, zipArchive.Bytes())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// sign the sums with our private key and generate a signature file
	var sig bytes.Buffer
	err = openpgp.DetachSign(&sig, signingEntity, bytes.NewReader(sums.Bytes()), nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Write everything out to the dstdir
	err = writeOutput(dstDir, result, zipArchive.Bytes(), sums.Bytes(), sig.Bytes())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return result, nil
}

func writeOutput(dstDir string, entry *RepackResult, zip, sums, sig []byte) error {
	zipFileName := filepath.Join(dstDir, entry.Zip)
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return trace.Wrap(err, "opening zipfile failed")
	}
	defer zipFile.Close()

	_, err = zipFile.Write(zip)
	if err != nil {
		return trace.Wrap(err, "writing zipfile failed")
	}

	sumFileName := filepath.Join(dstDir, entry.Sum)
	sumFile, err := os.Create(sumFileName)
	if err != nil {
		return trace.Wrap(err, "opening sum file failed")
	}
	defer sumFile.Close()

	_, err = sumFile.Write(sums)
	if err != nil {
		return trace.Wrap(err, "writing sumfile failed")
	}

	sigFileName := filepath.Join(dstDir, entry.Sig)
	sigFile, err := os.Create(sigFileName)
	if err != nil {
		return trace.Wrap(err, "opening sig file failed")
	}
	defer sigFile.Close()

	_, err = sigFile.Write(sig)
	if err != nil {
		return trace.Wrap(err, "writing sigFile failed")
	}

	return nil
}
