package releasetool

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"strings"
)

const (
	ociManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	ociConfigMediaType   = "application/vnd.oci.image.config.v1+json"
	maximumOCIJSONBytes  = 4 * 1024 * 1024
)

type OCIImageInfo struct {
	Arch      string `json:"arch"`
	Reference string `json:"reference"`
}

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    *ociPlatform      `json:"platform,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

type ociIndex struct {
	SchemaVersion int               `json:"schemaVersion"`
	Manifests     []ociDescriptor   `json:"manifests"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type ociLayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	MediaType     string          `json:"mediaType"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociImageConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		User       string            `json:"User"`
		Entrypoint []string          `json:"Entrypoint"`
		Labels     map[string]string `json:"Labels"`
	} `json:"config"`
	RootFS struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

type ociBlob struct {
	size int64
	data []byte
}

func VerifyCenterOCI(path, architecture, version, revision string) (OCIImageInfo, error) {
	if architecture != "amd64" && architecture != "arm64" {
		return OCIImageInfo{}, errors.New("unsupported Center architecture")
	}
	archive, err := os.Open(path)
	if err != nil {
		return OCIImageInfo{}, fmt.Errorf("open Center OCI archive: %w", err)
	}
	defer archive.Close()
	blobs := make(map[string]ociBlob)
	files := make(map[string][]byte)
	reader := tar.NewReader(archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return OCIImageInfo{}, fmt.Errorf("read Center OCI archive: %w", err)
		}
		name := header.Name
		cleanName := name
		if header.Typeflag == tar.TypeDir {
			cleanName = strings.TrimSuffix(cleanName, "/")
		}
		if cleanName == "" || pathpkg.IsAbs(cleanName) || pathpkg.Clean(cleanName) != cleanName || strings.HasPrefix(cleanName, "../") {
			return OCIImageInfo{}, errors.New("Center OCI archive contains an unsafe path")
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return OCIImageInfo{}, fmt.Errorf("Center OCI entry %q is not a regular file", name)
		}
		if _, exists := files[name]; exists {
			return OCIImageInfo{}, fmt.Errorf("Center OCI archive repeats %q", name)
		}
		hash := sha256.New()
		var captured bytes.Buffer
		output := io.Writer(hash)
		if header.Size <= maximumOCIJSONBytes {
			output = io.MultiWriter(hash, &captured)
		}
		written, err := io.Copy(output, reader)
		if err != nil || written != header.Size {
			return OCIImageInfo{}, fmt.Errorf("read Center OCI entry %q: %w", name, err)
		}
		files[name] = captured.Bytes()
		if strings.HasPrefix(name, "blobs/sha256/") {
			digest := strings.TrimPrefix(name, "blobs/sha256/")
			actual := hex.EncodeToString(hash.Sum(nil))
			if digest != actual || len(digest) != 64 {
				return OCIImageInfo{}, fmt.Errorf("Center OCI blob %q has an invalid digest", name)
			}
			blobs["sha256:"+digest] = ociBlob{size: written, data: captured.Bytes()}
		}
	}
	var layout ociLayout
	if err := decodeJSON(files["oci-layout"], &layout); err != nil || layout.ImageLayoutVersion != "1.0.0" {
		return OCIImageInfo{}, errors.New("Center archive is not an OCI image layout 1.0.0")
	}
	var index ociIndex
	if err := decodeJSON(files["index.json"], &index); err != nil {
		return OCIImageInfo{}, fmt.Errorf("decode Center OCI index: %w", err)
	}
	if index.SchemaVersion != 2 || len(index.Manifests) != 1 {
		return OCIImageInfo{}, errors.New("Center OCI index must contain exactly one image manifest")
	}
	manifestDescriptor := index.Manifests[0]
	if manifestDescriptor.MediaType != ociManifestMediaType || manifestDescriptor.Platform == nil ||
		manifestDescriptor.Platform.OS != "linux" || manifestDescriptor.Platform.Architecture != architecture {
		return OCIImageInfo{}, errors.New("Center OCI index platform does not match its artifact")
	}
	wantReference := "ghcr.io/ipchronicle/ipchronicle-center:v" + version
	reference := manifestDescriptor.Annotations["io.containerd.image.name"]
	if reference != wantReference || manifestDescriptor.Annotations["org.opencontainers.image.ref.name"] != "v"+version {
		return OCIImageInfo{}, errors.New("Center OCI image reference does not match its release")
	}
	manifestBlob, err := resolveDescriptor(blobs, manifestDescriptor)
	if err != nil {
		return OCIImageInfo{}, err
	}
	var manifest ociManifest
	if err := decodeJSON(manifestBlob.data, &manifest); err != nil {
		return OCIImageInfo{}, fmt.Errorf("decode Center OCI manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.MediaType != ociManifestMediaType ||
		manifest.Config.MediaType != ociConfigMediaType || len(manifest.Layers) == 0 {
		return OCIImageInfo{}, errors.New("Center OCI manifest structure is invalid")
	}
	configBlob, err := resolveDescriptor(blobs, manifest.Config)
	if err != nil {
		return OCIImageInfo{}, err
	}
	var config ociImageConfig
	if err := decodeJSON(configBlob.data, &config); err != nil {
		return OCIImageInfo{}, fmt.Errorf("decode Center OCI config: %w", err)
	}
	if config.OS != "linux" || config.Architecture != architecture || config.Config.User != "ipchronicle" ||
		len(config.Config.Entrypoint) != 1 || config.Config.Entrypoint[0] != "/usr/local/bin/ipchronicle-center" ||
		config.RootFS.Type != "layers" || len(config.RootFS.DiffIDs) != len(manifest.Layers) {
		return OCIImageInfo{}, errors.New("Center OCI runtime configuration is invalid")
	}
	wantLabels := map[string]string{
		"org.opencontainers.image.licenses": "AGPL-3.0-only",
		"org.opencontainers.image.revision": revision,
		"org.opencontainers.image.source":   "https://github.com/ipchronicle/ipchronicle",
		"org.opencontainers.image.url":      "https://github.com/ipchronicle/ipchronicle/tree/v" + version,
		"org.opencontainers.image.version":  version,
	}
	for name, value := range wantLabels {
		if config.Config.Labels[name] != value {
			return OCIImageInfo{}, fmt.Errorf("Center OCI label %q does not match its release", name)
		}
	}
	for _, layer := range manifest.Layers {
		if _, err := resolveDescriptor(blobs, layer); err != nil {
			return OCIImageInfo{}, err
		}
	}
	return OCIImageInfo{Arch: architecture, Reference: reference}, nil
}

func resolveDescriptor(blobs map[string]ociBlob, descriptor ociDescriptor) (ociBlob, error) {
	if !strings.HasPrefix(descriptor.Digest, "sha256:") || len(descriptor.Digest) != len("sha256:")+64 || descriptor.Size < 1 {
		return ociBlob{}, errors.New("Center OCI descriptor identity is invalid")
	}
	blob, ok := blobs[descriptor.Digest]
	if !ok || blob.size != descriptor.Size {
		return ociBlob{}, fmt.Errorf("Center OCI descriptor %q is missing or has the wrong size", descriptor.Digest)
	}
	return blob, nil
}

func decodeJSON(encoded []byte, target any) error {
	if len(encoded) == 0 || len(encoded) > maximumOCIJSONBytes {
		return errors.New("JSON document size is outside the supported range")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON document contains multiple values")
		}
		return err
	}
	return nil
}
