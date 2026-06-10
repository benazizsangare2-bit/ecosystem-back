package utils

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
)

const (
	MaxUploadSize  = 5 << 20 // 5MB
	MinImageWidth  = 200
	MinImageHeight = 200
	ThumbMaxWidth  = 400
)

var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

func SaveReportPhoto(fileHeader *multipart.FileHeader, uploadDir string) (photoPath, thumbPath string, err error) {
	if fileHeader.Size > MaxUploadSize {
		return "", "", fmt.Errorf("file exceeds maximum size of 5MB")
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedExtensions[ext] {
		return "", "", fmt.Errorf("only .jpg, .jpeg, and .png files are allowed")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	img, format, err := image.Decode(src)
	if err != nil {
		return "", "", fmt.Errorf("invalid image file")
	}
	if format != "jpeg" && format != "png" {
		return "", "", fmt.Errorf("unsupported image format")
	}

	bounds := img.Bounds()
	if bounds.Dx() < MinImageWidth || bounds.Dy() < MinImageHeight {
		return "", "", fmt.Errorf("image must be at least 200x200 pixels")
	}

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", "", err
	}
	thumbDir := filepath.Join(uploadDir, "thumbs")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return "", "", err
	}

	filename := uuid.New().String() + ext
	photoPath = filepath.Join(uploadDir, filename)

	if ext == ".png" {
		if err := imaging.Save(img, photoPath); err != nil {
			return "", "", err
		}
	} else {
		if err := imaging.Save(img, photoPath, imaging.JPEGQuality(90)); err != nil {
			return "", "", err
		}
	}

	thumb := imaging.Fit(img, ThumbMaxWidth, ThumbMaxWidth, imaging.Lanczos)
	thumbFilename := uuid.New().String() + ext
	thumbPath = filepath.Join(thumbDir, thumbFilename)
	if ext == ".png" {
		err = imaging.Save(thumb, thumbPath)
	} else {
		err = imaging.Save(thumb, thumbPath, imaging.JPEGQuality(85))
	}
	if err != nil {
		return "", "", err
	}

	return photoPath, thumbPath, nil
}
