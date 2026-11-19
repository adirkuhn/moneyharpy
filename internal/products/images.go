package products

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/attachments"
	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// ErrNoImage reports a product without an image.
var ErrNoImage = errors.New("product has no image")

// imageTypes are the files a product image can be, as Moneybird's checkout
// page shows them.
var imageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// entityType is what product images are kept under.
const entityType = "Product"

// Image is the picture of a product, for a checkout page. Moneybird's API
// has none; it is a MoneyHarpy extension.
type Image struct {
	ID          resource.ID
	Filename    string
	ContentType string
	Size        int64
}

// Images keeps the files of product images; the attachments service is one.
type Images interface {
	Add(ctx context.Context, admin resource.ID, entityType string, entity resource.ID, filename, contentType string, data []byte) (attachments.Attachment, error)
	Of(ctx context.Context, admin resource.ID, entityType string, entities []resource.ID) (map[resource.ID][]attachments.Attachment, error)
	Open(ctx context.Context, admin resource.ID, entityType string, entity, id resource.ID) (attachments.Attachment, io.ReadCloser, error)
	DeleteAll(ctx context.Context, admin resource.ID, entityType string, entity resource.ID) error
}

// UseImages lets products have an image.
func (s *Service) UseImages(i Images) { s.images = i }

func imageOf(a attachments.Attachment) *Image {
	return &Image{ID: a.ID, Filename: a.Filename, ContentType: a.ContentType, Size: a.Size}
}

// withImages fills in the image of each product.
func (s *Service) withImages(ctx context.Context, admin resource.ID, list []Product) error {
	if s.images == nil || len(list) == 0 {
		return nil
	}
	ids := make([]resource.ID, len(list))
	for i, p := range list {
		ids[i] = p.ID
	}
	files, err := s.images.Of(ctx, admin, entityType, ids)
	if err != nil {
		return fmt.Errorf("load images: %w", err)
	}
	for i := range list {
		if f := files[list[i].ID]; len(f) > 0 {
			list[i].Image = imageOf(f[len(f)-1])
		}
	}
	return nil
}

// SetImage gives the product an image, in place of the one it had.
func (s *Service) SetImage(ctx context.Context, admin, id, user resource.ID, filename, contentType string, data []byte) (Product, error) {
	p, err := s.Product(ctx, admin, id, user)
	if err != nil {
		return Product{}, err
	}
	if s.images == nil {
		return Product{}, ErrNoImage
	}
	if !slices.Contains(imageTypes, sniff(contentType, data)) {
		var verr resource.ValidationError
		verr.Add("image", "is not a PNG, JPEG, GIF or WebP image")
		return Product{}, &verr
	}
	if err := s.images.DeleteAll(ctx, admin, entityType, id); err != nil {
		return Product{}, fmt.Errorf("remove old image: %w", err)
	}
	a, err := s.images.Add(ctx, admin, entityType, id, filename, contentType, data)
	if err != nil {
		return Product{}, err
	}
	p.Image = imageOf(a)
	return p, nil
}

// Image opens the product's image.
func (s *Service) Image(ctx context.Context, admin, id, user resource.ID) (Image, io.ReadCloser, error) {
	p, err := s.Product(ctx, admin, id, user)
	if err != nil {
		return Image{}, nil, err
	}
	if p.Image == nil {
		return Image{}, nil, ErrNoImage
	}
	a, body, err := s.images.Open(ctx, admin, entityType, id, p.Image.ID)
	if err != nil {
		return Image{}, nil, err
	}
	return *imageOf(a), body, nil
}

// DeleteImage removes the product's image.
func (s *Service) DeleteImage(ctx context.Context, admin, id, user resource.ID) (Product, error) {
	p, err := s.Product(ctx, admin, id, user)
	if err != nil {
		return Product{}, err
	}
	if s.images != nil {
		if err := s.images.DeleteAll(ctx, admin, entityType, id); err != nil {
			return Product{}, fmt.Errorf("remove image: %w", err)
		}
	}
	p.Image = nil
	return p, nil
}

// sniff is the file's type: the one given, or else what its bytes say.
func sniff(contentType string, data []byte) string {
	t, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(contentType)), ";")
	if t == "" || t == "application/octet-stream" {
		t, _, _ = strings.Cut(http.DetectContentType(data), ";")
	}
	return t
}
