package toolbuiltin

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"

	"github.com/saker-ai/saker/pkg/model"
)

// SupportedImageMediaTypes is the canonical whitelist of image MIME types we
// will base64-encode and surface to the LLM as ContentBlockImage.
var SupportedImageMediaTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
	"image/bmp":  {},
}

// EncodeImageBytes converts already-loaded bytes into a model.ContentBlock,
// detecting the MIME type and rejecting unsupported formats. Callers that have
// already enforced size limits/whitelists can use this directly; tools reading
// from disk should prefer LoadImageBlockFromFile.
func EncodeImageBytes(data []byte) (*model.ContentBlock, error) {
	if len(data) == 0 {
		return nil, errors.New("image data is empty")
	}
	mediaType := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if semi := strings.Index(mediaType, ";"); semi >= 0 {
		mediaType = strings.TrimSpace(mediaType[:semi])
	}
	if _, ok := SupportedImageMediaTypes[mediaType]; !ok {
		return nil, fmt.Errorf("unsupported image media type %q", mediaType)
	}
	return &model.ContentBlock{
		Type:      model.ContentBlockImage,
		MediaType: mediaType,
		Data:      base64.StdEncoding.EncodeToString(data),
	}, nil
}

// LoadImageBlockFromFile reads an image from disk and returns a populated
// ContentBlock plus its detected media type. Pass maxBytes <= 0 to disable
// the size cap. When the file exceeds maxBytes, it is automatically
// downsampled (decoded → resized → JPEG re-encoded) so that the LLM can
// still inspect the image instead of receiving an opaque error.
func LoadImageBlockFromFile(path string, maxBytes int64) (*model.ContentBlock, string, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, "", 0, fmt.Errorf("%s is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read %s: %w", path, err)
	}

	if maxBytes > 0 && int64(len(data)) > maxBytes {
		resized, resizeErr := downsampleToFit(data, maxBytes)
		if resizeErr != nil {
			return nil, "", 0, fmt.Errorf("image %d bytes exceeds %d byte limit and could not be downsampled: %w", len(data), maxBytes, resizeErr)
		}
		data = resized
	}

	block, err := EncodeImageBytes(data)
	if err != nil {
		return nil, "", 0, err
	}
	return block, block.MediaType, len(data), nil
}

// downsampleMaxWidth is the initial target width for downsampling.
const downsampleMaxWidth = 1920

// downsampleToFit decodes the image, progressively scales it down, and
// re-encodes as JPEG until the result fits within maxBytes.
func downsampleToFit(data []byte, maxBytes int64) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode failed: %w", err)
	}

	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	targetW := downsampleMaxWidth
	qualities := []int{85, 70, 55, 40}

	for _, q := range qualities {
		newW := targetW
		if newW >= srcW {
			newW = srcW
		}
		newH := srcH * newW / srcW
		if newH < 1 {
			newH = 1
		}

		var img image.Image
		if newW == srcW && newH == srcH {
			img = src
		} else {
			dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
			for y := 0; y < newH; y++ {
				srcY := y * srcH / newH
				for x := 0; x < newW; x++ {
					srcX := x * srcW / newW
					dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
				}
			}
			img = dst
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, fmt.Errorf("jpeg encode failed: %w", err)
		}
		if int64(buf.Len()) <= maxBytes {
			return buf.Bytes(), nil
		}

		targetW = targetW * 3 / 4
		if targetW < 320 {
			targetW = 320
		}
	}

	// Last resort: 320px wide, quality 30.
	newW := 320
	if newW > srcW {
		newW = srcW
	}
	newH := srcH * newW / srcW
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		srcY := y * srcH / newH
		for x := 0; x < newW; x++ {
			srcX := x * srcW / newW
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 30}); err != nil {
		return nil, fmt.Errorf("jpeg encode failed: %w", err)
	}
	if int64(buf.Len()) <= maxBytes {
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("image %d bytes still exceeds limit after maximum downsampling", buf.Len())
}
