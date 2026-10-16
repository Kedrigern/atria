package ebooks

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrEPUBCoverNotFound indicates that an EPUB does not declare a cover image.
var ErrEPUBCoverNotFound = errors.New("EPUB cover image not found")

// Limits on decompressed sizes, so a crafted archive (zip bomb) cannot
// exhaust disk or memory.
const (
	maxEPUBCoverBytes = 20 << 20
	maxEPUBXMLBytes   = 4 << 20
)

// EPUBMetadata contains descriptive metadata read from an EPUB package document.
type EPUBMetadata struct {
	Title       string
	Author      string
	Language    string
	Description string
}

// EPUBCover describes a cover image extracted from an EPUB archive.
type EPUBCover struct {
	// Filename is the cover image filename declared by the EPUB package document.
	Filename string
	// Path is the local path of the extracted image.
	Path string
}

// ReadEPUBMetadata reads title, creator, language, and description from an EPUB file.
func ReadEPUBMetadata(epubPath string) (*EPUBMetadata, error) {
	archive, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("open EPUB archive: %w", err)
	}
	defer archive.Close()

	packageDocument, err := readPackageDocument(archive)
	if err != nil {
		return nil, err
	}
	return &packageDocument.metadata, nil
}

// ExtractEPUBCover extracts an EPUB cover image into destinationDir and returns its local path.
// destinationDir must already exist and should be a temporary directory owned by the caller.
func ExtractEPUBCover(epubPath, destinationDir string) (*EPUBCover, error) {
	archive, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("open EPUB archive: %w", err)
	}
	defer archive.Close()

	packageDocument, err := readPackageDocument(archive)
	if err != nil {
		return nil, err
	}
	if packageDocument.coverPath == "" {
		return nil, ErrEPUBCoverNotFound
	}

	coverFile := findArchiveFile(archive, packageDocument.coverPath)
	if coverFile == nil {
		return nil, fmt.Errorf("find EPUB cover %q: %w", packageDocument.coverPath, ErrEPUBCoverNotFound)
	}
	if coverFile.FileInfo().IsDir() {
		return nil, fmt.Errorf("EPUB cover %q is a directory", packageDocument.coverPath)
	}

	extension := filepath.Ext(filepath.Base(packageDocument.coverPath))
	temporaryFile, err := os.CreateTemp(destinationDir, "epub-cover-*"+extension)
	if err != nil {
		return nil, fmt.Errorf("create cover file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer func() {
		if temporaryFile != nil {
			temporaryFile.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	if coverFile.UncompressedSize64 > maxEPUBCoverBytes {
		return nil, fmt.Errorf("EPUB cover %q is too large", packageDocument.coverPath)
	}
	source, err := coverFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open EPUB cover: %w", err)
	}
	defer source.Close()
	written, err := io.Copy(temporaryFile, io.LimitReader(source, maxEPUBCoverBytes+1))
	if err != nil {
		return nil, fmt.Errorf("extract EPUB cover: %w", err)
	}
	if written > maxEPUBCoverBytes {
		return nil, fmt.Errorf("EPUB cover %q is too large", packageDocument.coverPath)
	}
	if err := temporaryFile.Close(); err != nil {
		return nil, fmt.Errorf("close extracted EPUB cover: %w", err)
	}
	temporaryFile = nil

	return &EPUBCover{
		Filename: filepath.Base(packageDocument.coverPath),
		Path:     temporaryPath,
	}, nil
}

type epubPackageDocument struct {
	metadata  EPUBMetadata
	coverPath string
}

type epubManifestItem struct {
	id         string
	href       string
	properties string
}

func readPackageDocument(archive *zip.ReadCloser) (*epubPackageDocument, error) {
	container := findArchiveFile(archive, "META-INF/container.xml")
	if container == nil {
		return nil, fmt.Errorf("read EPUB container: META-INF/container.xml not found")
	}

	rootfilePath, err := parseContainerRootfile(container)
	if err != nil {
		return nil, err
	}
	packageFile := findArchiveFile(archive, rootfilePath)
	if packageFile == nil {
		return nil, fmt.Errorf("read EPUB package document: %q not found", rootfilePath)
	}
	return parsePackageDocument(packageFile, rootfilePath)
}

func parseContainerRootfile(file *zip.File) (string, error) {
	reader, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open EPUB container: %w", err)
	}
	defer reader.Close()

	decoder := xml.NewDecoder(io.LimitReader(reader, maxEPUBXMLBytes))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse EPUB container: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "rootfile" {
			continue
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "full-path" {
				rootfilePath, err := cleanArchivePath(attribute.Value)
				if err != nil {
					return "", fmt.Errorf("invalid EPUB rootfile path: %w", err)
				}
				return rootfilePath, nil
			}
		}
	}
	return "", fmt.Errorf("parse EPUB container: rootfile is missing")
}

func parsePackageDocument(file *zip.File, packagePath string) (*epubPackageDocument, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open EPUB package document: %w", err)
	}
	defer reader.Close()

	var metadata EPUBMetadata
	var coverID string
	var manifest []epubManifestItem
	decoder := xml.NewDecoder(io.LimitReader(reader, maxEPUBXMLBytes))
	inMetadata := false
	inManifest := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse EPUB package document: %w", err)
		}
		if end, ok := token.(xml.EndElement); ok {
			switch end.Name.Local {
			case "metadata":
				inMetadata = false
			case "manifest":
				inManifest = false
			}
			continue
		}

		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "metadata":
			inMetadata = true
		case "manifest":
			inManifest = true
		case "title", "creator", "language", "description":
			if !inMetadata {
				continue
			}
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return nil, fmt.Errorf("parse EPUB metadata: %w", err)
			}
			value = strings.TrimSpace(value)
			switch start.Name.Local {
			case "title":
				if metadata.Title == "" {
					metadata.Title = value
				}
			case "creator":
				if metadata.Author == "" {
					metadata.Author = value
				}
			case "language":
				if metadata.Language == "" {
					metadata.Language = value
				}
			case "description":
				if metadata.Description == "" {
					metadata.Description = value
				}
			}
		case "meta":
			if !inMetadata {
				continue
			}
			if attributeValue(start, "name") == "cover" && coverID == "" {
				coverID = attributeValue(start, "content")
			}
		case "item":
			if !inManifest {
				continue
			}
			manifest = append(manifest, epubManifestItem{
				id:         attributeValue(start, "id"),
				href:       attributeValue(start, "href"),
				properties: attributeValue(start, "properties"),
			})
		}
	}

	coverItem := findCoverManifestItem(manifest, coverID)
	var coverPath string
	if coverItem != nil {
		coverPath, err = resolveArchivePath(packagePath, coverItem.href)
		if err != nil {
			return nil, fmt.Errorf("resolve EPUB cover path: %w", err)
		}
	}
	return &epubPackageDocument{metadata: metadata, coverPath: coverPath}, nil
}

func findCoverManifestItem(items []epubManifestItem, legacyCoverID string) *epubManifestItem {
	for index := range items {
		for _, property := range strings.Fields(items[index].properties) {
			if property == "cover-image" {
				return &items[index]
			}
		}
	}
	for index := range items {
		if items[index].id == legacyCoverID {
			return &items[index]
		}
	}
	return nil
}

func attributeValue(element xml.StartElement, name string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

func findArchiveFile(archive *zip.ReadCloser, archivePath string) *zip.File {
	for _, file := range archive.File {
		if file.Name == archivePath {
			return file
		}
	}
	return nil
}

func resolveArchivePath(basePath, reference string) (string, error) {
	reference = strings.SplitN(strings.SplitN(reference, "#", 2)[0], "?", 2)[0]
	decodedReference, err := url.PathUnescape(reference)
	if err != nil {
		return "", fmt.Errorf("decode path %q: %w", reference, err)
	}
	if strings.HasPrefix(decodedReference, "/") {
		return "", fmt.Errorf("absolute path %q", reference)
	}
	return cleanArchivePath(path.Join(path.Dir(basePath), decodedReference))
}

func cleanArchivePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("path %q is not relative", value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the EPUB archive", value)
	}
	return cleaned, nil
}
