package ebooks

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"atria/internal/core"
)

// maxCoverPixels limits the size of a decoded cover image (a crafted small
// file can declare a huge image).
const maxCoverPixels = 40_000_000

// Cover is a cover image, encoded for an EPUB.
type Cover struct {
	Data      []byte
	MediaType string
	Extension string
}

// DecodeCover reads a JPEG, PNG or GIF image for a cover.
func DecodeCover(imagePath string) (image.Image, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	config, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, core.NewUserError("the cover must be a JPEG, PNG or GIF image")
	}
	if config.Width*config.Height > maxCoverPixels {
		return nil, core.NewUserError("the cover image is too large (%d × %d pixels)", config.Width, config.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, core.NewUserError("cannot read the %s cover image: %v", format, err)
	}
	return img, nil
}

// encodeCover encodes img in the media type that the EPUB declares for its
// cover, so the file name and the manifest stay valid. Other types become JPEG.
func encodeCover(img image.Image, mediaType string) (*Cover, error) {
	var buf bytes.Buffer
	switch mediaType {
	case "image/png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encode PNG cover: %w", err)
		}
		return &Cover{Data: buf.Bytes(), MediaType: "image/png", Extension: ".png"}, nil
	case "image/gif":
		if err := gif.Encode(&buf, img, nil); err != nil {
			return nil, fmt.Errorf("encode GIF cover: %w", err)
		}
		return &Cover{Data: buf.Bytes(), MediaType: "image/gif", Extension: ".gif"}, nil
	default:
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode JPEG cover: %w", err)
		}
		return &Cover{Data: buf.Bytes(), MediaType: "image/jpeg", Extension: ".jpg"}, nil
	}
}

// RewriteEPUB copies the EPUB src to dst with new metadata and a new cover.
// A nil metadata keeps the metadata of the file; a nil cover keeps its cover.
// It returns the cover that it wrote, or nil.
//
// The metadata goes into the first dc:title, dc:creator, dc:language and
// dc:description of the package document. The new cover replaces the bytes of
// the existing cover image at the same path. If the EPUB has no cover, the
// rewrite adds one.
func RewriteEPUB(src, dst string, metadata *EPUBMetadata, cover image.Image) (*Cover, error) {
	archive, err := zip.OpenReader(src)
	if err != nil {
		return nil, fmt.Errorf("open EPUB archive: %w", err)
	}
	defer archive.Close()

	document, err := readPackageDocument(archive)
	if err != nil {
		return nil, err
	}
	packageFile := findArchiveFile(archive, document.packagePath)
	opfBytes, err := readLimited(packageFile, maxEPUBXMLBytes)
	if err != nil {
		return nil, fmt.Errorf("read EPUB package document: %w", err)
	}
	opf := string(opfBytes)

	if metadata != nil {
		opf = setMetadataElement(opf, "title", metadata.Title, true)
		opf = setMetadataElement(opf, "creator", metadata.Author, false)
		opf = setMetadataElement(opf, "language", metadata.Language, true)
		opf = setMetadataElement(opf, "description", metadata.Description, false)
	}

	var written *Cover
	coverPath := document.coverPath
	if cover != nil {
		written, err = encodeCover(cover, document.coverMediaType)
		if err != nil {
			return nil, err
		}
		if coverPath == "" || findArchiveFile(archive, coverPath) == nil {
			// The EPUB has no cover: add an image next to the package document.
			href := "atria-cover" + written.Extension
			coverPath = path.Join(path.Dir(document.packagePath), href)
			opf = insertBefore(opf, "manifest", fmt.Sprintf(`<item id="atria-cover" href="%s" media-type="%s" properties="cover-image"/>`, href, written.MediaType))
			opf = insertBefore(opf, "metadata", `<meta name="cover" content="atria-cover"/>`)
		} else if written.MediaType != document.coverMediaType {
			opf = setCoverMediaType(opf, path.Base(coverPath), written.MediaType)
		}
	}

	out, err := os.Create(dst)
	if err != nil {
		return nil, fmt.Errorf("create EPUB: %w", err)
	}
	defer out.Close()
	writer := zip.NewWriter(out)

	// The "mimetype" entry must come first and must not be compressed.
	mimetypeWriter, err := writer.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, fmt.Errorf("write EPUB mimetype: %w", err)
	}
	if _, err := io.WriteString(mimetypeWriter, "application/epub+zip"); err != nil {
		return nil, fmt.Errorf("write EPUB mimetype: %w", err)
	}

	coverDone := false
	for _, file := range archive.File {
		switch file.Name {
		case "mimetype":
			continue
		case document.packagePath:
			if err := writeEntry(writer, file.Name, []byte(opf)); err != nil {
				return nil, err
			}
		case coverPath:
			if written == nil {
				if err := writer.Copy(file); err != nil {
					return nil, fmt.Errorf("copy %s: %w", file.Name, err)
				}
				continue
			}
			if err := writeEntry(writer, file.Name, written.Data); err != nil {
				return nil, err
			}
			coverDone = true
		default:
			if err := writer.Copy(file); err != nil {
				return nil, fmt.Errorf("copy %s: %w", file.Name, err)
			}
		}
	}
	if written != nil && !coverDone {
		if err := writeEntry(writer, coverPath, written.Data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish EPUB: %w", err)
	}
	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("close EPUB: %w", err)
	}
	return written, nil
}

func readLimited(file *zip.File, limit int64) ([]byte, error) {
	if file.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("%s is too large", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, limit))
}

func writeEntry(writer *zip.Writer, name string, data []byte) error {
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if _, err := entry.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// setMetadataElement sets the text of the first dc:<name> element. An empty
// value removes the elements, unless the element is required: then the file
// keeps its value. A missing element is added at the end of the metadata.
func setMetadataElement(opf, name, value string, required bool) string {
	element := regexp.MustCompile(`(?s)<dc:` + name + `\b([^>]*?)(?:/>|>.*?</dc:` + name + `>)`)
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return opf
		}
		return element.ReplaceAllString(opf, "")
	}
	escaped := html.EscapeString(value)
	done := false
	opf = element.ReplaceAllStringFunc(opf, func(match string) string {
		if done {
			return match
		}
		done = true
		attributes := element.FindStringSubmatch(match)[1]
		return "<dc:" + name + attributes + ">" + escaped + "</dc:" + name + ">"
	})
	if !done {
		opf = insertBefore(opf, "metadata", "<dc:"+name+">"+escaped+"</dc:"+name+">")
	}
	return opf
}

// insertBefore puts text before the closing tag of an element, with or
// without a namespace prefix, for example </metadata> or </opf:metadata>.
func insertBefore(opf, element, text string) string {
	closing := regexp.MustCompile(`</(?:[A-Za-z0-9]+:)?` + element + `>`)
	loc := closing.FindStringIndex(opf)
	if loc == nil {
		return opf
	}
	return opf[:loc[0]] + text + "\n" + opf[loc[0]:]
}

// setCoverMediaType changes the media-type of the manifest item of the cover.
func setCoverMediaType(opf, coverFile, mediaType string) string {
	item := regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?item\b[^>]*>`)
	mediaTypeAttribute := regexp.MustCompile(`media-type="[^"]*"`)
	return item.ReplaceAllStringFunc(opf, func(match string) string {
		if !strings.Contains(match, `/`+coverFile+`"`) && !strings.Contains(match, `"`+coverFile+`"`) {
			return match
		}
		return mediaTypeAttribute.ReplaceAllString(match, `media-type="`+mediaType+`"`)
	})
}
