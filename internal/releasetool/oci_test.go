package releasetool

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyCenterOCI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "center.oci.tar")
	writeTestOCI(t, path, "amd64", "0.1.0-rc.1", testRevision, false)
	info, err := VerifyCenterOCI(path, "amd64", "0.1.0-rc.1", testRevision)
	if err != nil {
		t.Fatal(err)
	}
	if info.Arch != "amd64" || info.Reference != "ghcr.io/ipchronicle/ipchronicle-center:v0.1.0-rc.1" {
		t.Fatalf("unexpected OCI image info: %#v", info)
	}
}

func TestVerifyCenterOCIRejectsWrongMetadataAndTamperedBlob(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "center.oci.tar")
		writeTestOCI(t, path, "arm64", "0.1.0", testRevision, true)
		if _, err := VerifyCenterOCI(path, "arm64", "0.1.0", testRevision); err == nil {
			t.Fatal("incorrect OCI label unexpectedly accepted")
		}
	})
	t.Run("platform", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "center.oci.tar")
		writeTestOCI(t, path, "amd64", "0.1.0", testRevision, false)
		if _, err := VerifyCenterOCI(path, "arm64", "0.1.0", testRevision); err == nil {
			t.Fatal("incorrect OCI platform unexpectedly accepted")
		}
	})
}

func writeTestOCI(t *testing.T, path, architecture, version, revision string, wrongLabel bool) {
	t.Helper()
	layer := []byte("test layer contents")
	layerDigest := digestBytes(layer)
	labels := map[string]string{
		"org.opencontainers.image.licenses": "AGPL-3.0-only",
		"org.opencontainers.image.revision": revision,
		"org.opencontainers.image.source":   "https://github.com/ipchronicle/ipchronicle/tree/v" + version,
		"org.opencontainers.image.url":      "https://github.com/ipchronicle/ipchronicle/tree/v" + version,
		"org.opencontainers.image.version":  version,
	}
	if wrongLabel {
		labels["org.opencontainers.image.revision"] = "wrong"
	}
	config := map[string]any{
		"architecture": architecture,
		"os":           "linux",
		"config": map[string]any{
			"User":       "ipchronicle",
			"Entrypoint": []string{"/usr/local/bin/ipchronicle-center"},
			"Labels":     labels,
		},
		"rootfs": map[string]any{"type": "layers", "diff_ids": []string{layerDigest}},
	}
	configJSON := marshalTestJSON(t, config)
	configDigest := digestBytes(configJSON)
	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     ociManifestMediaType,
		Config: ociDescriptor{
			MediaType: ociConfigMediaType, Digest: configDigest, Size: int64(len(configJSON)),
		},
		Layers: []ociDescriptor{{
			MediaType: "application/vnd.oci.image.layer.v1.tar", Digest: layerDigest, Size: int64(len(layer)),
		}},
	}
	manifestJSON := marshalTestJSON(t, manifest)
	manifestDigest := digestBytes(manifestJSON)
	reference := "ghcr.io/ipchronicle/ipchronicle-center:v" + version
	index := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociDescriptor{{
			MediaType: ociManifestMediaType, Digest: manifestDigest, Size: int64(len(manifestJSON)),
			Platform: &ociPlatform{Architecture: architecture, OS: "linux"},
			Annotations: map[string]string{
				"io.containerd.image.name":          reference,
				"org.opencontainers.image.ref.name": "v" + version,
			},
		}},
	}
	indexJSON := marshalTestJSON(t, index)

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	entries := []struct {
		name string
		data []byte
	}{
		{name: "oci-layout", data: []byte(`{"imageLayoutVersion":"1.0.0"}`)},
		{name: "index.json", data: indexJSON},
		{name: "blobs/sha256/" + configDigest[len("sha256:"):], data: configJSON},
		{name: "blobs/sha256/" + manifestDigest[len("sha256:"):], data: manifestJSON},
		{name: "blobs/sha256/" + layerDigest[len("sha256:"):], data: layer},
	}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.data)), Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func digestBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func marshalTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
